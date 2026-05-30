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
	"go.uber.org/zap"
)

// LLMTokenStore abstracts token storage to avoid import cycles.
type LLMTokenStore interface {
	GetByKeyHash(hash string) (*model.LLMToken, error)
	CountRequestsToday(tokenID uint) (int, error)
	UpdateLastUsed(tokenID uint) error
}

const LLMTokenContextKey = "llm_token"

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

		// Backward compatibility: server-level API key bypasses token table
		if key == serverAPIKey {
			c.Set(LLMTokenContextKey, (*model.LLMToken)(nil)) // nil = internal/admin
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

		// Check allowed models (only for chat/completions and similar endpoints)
		allowedModels := token.GetAllowedModels()
		if len(allowedModels) > 0 {
			modelName := extractModelFromContext(c)
			if modelName != "" && !containsString(allowedModels, modelName) {
				response.Forbidden(c, "model not allowed for this token")
				c.Abort()
				return
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

		// Update last_used_at
		_ = tokenRepo.UpdateLastUsed(token.ID)

		c.Set(LLMTokenContextKey, token)
		c.Set("caller_id", token.CallerID)
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
