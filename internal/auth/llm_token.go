package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/response"
)

// LLMTokenStore abstracts token storage to avoid import cycles.
type LLMTokenStore interface {
	GetByKeyHash(hash string) (*model.LLMToken, error)
	IsModelGroupName(name string) (bool, error)
	CountRequestsToday(tokenID uint) (int, error)
	TokensUsedToday(tokenID uint) (int, error)
	CostThisMonthCents(tokenID uint) (int, error)
	UpdateLastUsed(tokenID uint) error
}

const (
	LLMTokenContextKey    = "llm_token"
	LLMCallerIDContextKey = "caller_id"
	LLMTokenIDContextKey  = "token_id"
)

// CallerIdentity holds the validated identity for a request.
// Used to pass authenticated info from middleware to service without token pointer.
type CallerIdentity struct {
	CallerID string
	TokenID  uint
}

// GetCallerIdentity retrieves the validated caller identity from gin context.
func GetCallerIdentity(c *gin.Context) CallerIdentity {
	var id CallerIdentity
	if v, exists := c.Get(LLMCallerIDContextKey); exists {
		if s, ok := v.(string); ok {
			id.CallerID = s
		}
	}
	if v, exists := c.Get(LLMTokenIDContextKey); exists {
		if u, ok := v.(uint); ok {
			id.TokenID = u
		}
	}
	return id
}

// LLMTokenAuth middleware validates Bearer tokens against the llm_tokens table.
// It supports the existing server.api_key for backward compatibility (bypasses token check).
func LLMTokenAuth(tokenRepo LLMTokenStore, serverAPIKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			response.Unauthorized(c, "missing Authorization header")
			c.Abort()
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			response.Unauthorized(c, "invalid Authorization format")
			c.Abort()
			return
		}
		key := parts[1]

		// Backward compatibility: the server-level API key authenticates internal
		// callers. If a matching token row exists (the seeded "default" token keyed
		// off the same api_key), resolve to it so the traffic is billed (token_id != 0);
		// otherwise fall back to the legacy admin bypass (token_id 0, unbilled).
		if serverAPIKey != "" && key == serverAPIKey {
			if token, err := tokenRepo.GetByKeyHash(model.HashKey(key)); err == nil && token != nil && token.Enabled {
				_ = tokenRepo.UpdateLastUsed(token.ID)
				c.Set(LLMTokenContextKey, token)
				c.Set(LLMCallerIDContextKey, token.CallerID)
				c.Set(LLMTokenIDContextKey, token.ID)
				c.Next()
				return
			}
			c.Set(LLMTokenContextKey, (*model.LLMToken)(nil)) // nil = internal/admin
			c.Set(LLMCallerIDContextKey, "server")
			c.Set(LLMTokenIDContextKey, uint(0))
			c.Next()
			return
		}

		// Validate against token table
		token, err := tokenRepo.GetByKeyHash(model.HashKey(key))
		if err != nil || token == nil {
			response.Unauthorized(c, "invalid API key")
			c.Abort()
			return
		}

		if !token.Enabled {
			response.Forbidden(c, "token disabled")
			c.Abort()
			return
		}

		if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
			response.Forbidden(c, "token expired")
			c.Abort()
			return
		}

		// Check model/group scopes for chat/completions and similar endpoints.
		// Request model names may be either real upstream models or virtual model
		// groups; groups are authorized through allowed_groups so admins can scope
		// routing pools without exposing their member models directly.
		modelName := extractModelFromContext(c)
		if modelName != "" {
			isGroup, err := tokenRepo.IsModelGroupName(modelName)
			if err != nil {
				response.InternalError(c, "failed to validate model scope")
				c.Abort()
				return
			}

			if isGroup {
				allowedGroups := token.GetAllowedGroups()
				if len(allowedGroups) > 0 && !containsString(allowedGroups, modelName) {
					response.Forbidden(c, "model group not allowed for this token")
					c.Abort()
					return
				}
			} else {
				allowedModels := token.GetAllowedModels()
				if len(allowedModels) > 0 && !containsString(allowedModels, modelName) {
					response.Forbidden(c, "model not allowed for this token")
					c.Abort()
					return
				}
			}
		}

		// Check daily request quota
		if token.QuotaRequestsDaily > 0 {
			used, err := tokenRepo.CountRequestsToday(token.ID)
			if err == nil && used >= token.QuotaRequestsDaily {
				resetTime := time.Now().AddDate(0, 0, 1).Truncate(24 * time.Hour)
				c.Header("X-Quota-Reset", resetTime.Format(time.RFC3339))
				c.Header("Retry-After", strconv.Itoa(int(resetTime.Sub(time.Now()).Seconds())))
				response.TooManyRequests(c, "daily request quota exceeded")
				c.Abort()
				return
			}
		}

		// Check daily token quota (prompt + completion tokens consumed today)
		if token.QuotaTokensDaily > 0 {
			used, err := tokenRepo.TokensUsedToday(token.ID)
			if err == nil && used >= token.QuotaTokensDaily {
				resetTime := time.Now().AddDate(0, 0, 1).Truncate(24 * time.Hour)
				c.Header("X-Quota-Reset", resetTime.Format(time.RFC3339))
				c.Header("Retry-After", strconv.Itoa(int(resetTime.Sub(time.Now()).Seconds())))
				response.TooManyRequests(c, "daily token quota exceeded")
				c.Abort()
				return
			}
		}

		// Check monthly cost quota (month-to-date cost in cents)
		if token.QuotaCostMonthlyCents > 0 {
			used, err := tokenRepo.CostThisMonthCents(token.ID)
			if err == nil && used >= token.QuotaCostMonthlyCents {
				now := time.Now()
				resetTime := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, 1, 0)
				c.Header("X-Quota-Reset", resetTime.Format(time.RFC3339))
				c.Header("Retry-After", strconv.Itoa(int(resetTime.Sub(time.Now()).Seconds())))
				response.TooManyRequests(c, "monthly cost quota exceeded")
				c.Abort()
				return
			}
		}

		// Update last_used_at
		_ = tokenRepo.UpdateLastUsed(token.ID)

		c.Set(LLMTokenContextKey, token)
		c.Set(LLMCallerIDContextKey, token.CallerID)
		c.Set(LLMTokenIDContextKey, token.ID)
		c.Next()
	}
}

// GetLLMToken retrieves the validated token from gin context.
// Returns nil if the request used the server API key (admin/internal).
func GetLLMToken(c *gin.Context) *model.LLMToken {
	v, exists := c.Get(LLMTokenContextKey)
	if !exists {
		return nil
	}
	if token, ok := v.(*model.LLMToken); ok {
		return token
	}
	return nil
}

func extractModelFromContext(c *gin.Context) string {
	// Try to read model from request body for POST/PUT
	if c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPut {
		body, err := c.GetRawData()
		if err == nil && len(body) > 0 {
			var req struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal(body, &req); err == nil && req.Model != "" {
				// Restore body for downstream handlers
				c.Request.Body = io.NopCloser(bytes.NewReader(body))
				return req.Model
			}
			// Restore body
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
		}
	}
	return ""
}

func containsString(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
}
