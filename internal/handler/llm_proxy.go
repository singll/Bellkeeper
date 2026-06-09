package handler

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/auth"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/singll/bellkeeper/internal/service"
)

type LLMProxyHandler struct {
	svc            *service.LLMProxyService
	pricer         *service.Pricer
	tokenRepo      *repository.LLMTokenRepository
	tokenUsageRepo *repository.LLMTokenUsageRepository
	pricingRepo    *repository.LLMModelPricingRepository
}

func NewLLMProxyHandler(
	svc *service.LLMProxyService,
	pricer *service.Pricer,
	tokenRepo *repository.LLMTokenRepository,
	tokenUsageRepo *repository.LLMTokenUsageRepository,
	pricingRepo *repository.LLMModelPricingRepository,
) *LLMProxyHandler {
	return &LLMProxyHandler{
		svc:            svc,
		pricer:         pricer,
		tokenRepo:      tokenRepo,
		tokenUsageRepo: tokenUsageRepo,
		pricingRepo:    pricingRepo,
	}
}

// Proxy handles OpenAI-compatible proxy requests.
// Route: Any /api/llm/v1/*path
func (h *LLMProxyHandler) Proxy(c *gin.Context) {
	path := "/v1" + c.Param("path")
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.BadRequest(c, "failed to read request body")
		return
	}

	// Use authenticated identity from middleware, not the client-supplied X-Caller-ID header.
	// This prevents spoofing: only the token auth middleware can set this.
	identity := auth.GetCallerIdentity(c)
	callerID := identity.CallerID
	tokenID := identity.TokenID
	if callerID == "" {
		callerID = "unknown"
	}

	if h.svc.IsStreamRequest(body) {
		h.proxyStream(c, path, body, callerID, tokenID)
	} else {
		h.proxyBuffered(c, path, body, callerID, tokenID)
	}
}

// proxyBuffered handles non-streaming proxy requests (original behavior).
func (h *LLMProxyHandler) proxyBuffered(c *gin.Context, path string, body []byte, callerID string, tokenID uint) {
	statusCode, respBody, respHeaders, err := h.svc.ProxyRequest(
		c.Request.Method, path, c.Request.Header, body, callerID, tokenID,
	)

	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// Forward upstream content-type
	if ct := respHeaders.Get("Content-Type"); ct != "" {
		c.Header("Content-Type", ct)
	}

	c.Data(statusCode, "application/json", respBody)
}

// proxyStream handles streaming proxy requests.
func (h *LLMProxyHandler) proxyStream(c *gin.Context, path string, body []byte, callerID string, tokenID uint) {
	result, err := h.svc.ProxyStreamRequest(
		c.Request.Method, path, c.Request.Header, body, callerID, tokenID,
	)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	defer result.BodyReader.Close()
	defer h.svc.FinalizeStream(result, path, callerID, tokenID)

	// Non-200 responses: read body fully and return as JSON error
	if result.StatusCode != 200 {
		errBody, _ := io.ReadAll(result.BodyReader)
		if ct := result.RespHeaders.Get("Content-Type"); ct != "" {
			c.Header("Content-Type", ct)
		}
		c.Data(result.StatusCode, "application/json", errBody)
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	if result.ProviderType == "anthropic" {
		h.streamAnthropicToOpenAI(c, result.BodyReader)
	} else {
		h.streamPassthrough(c, result.BodyReader)
	}
}

// streamPassthrough transparently forwards SSE data from an OpenAI-compatible upstream.
func (h *LLMProxyHandler) streamPassthrough(c *gin.Context, bodyReader io.ReadCloser) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.InternalError(c, "streaming not supported")
		return
	}

	buf := make([]byte, 4096)
	for {
		n, err := bodyReader.Read(buf)
		if n > 0 {
			c.Writer.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			break
		}
	}
}

// streamAnthropicToOpenAI converts Anthropic SSE events to OpenAI SSE format on the fly.
func (h *LLMProxyHandler) streamAnthropicToOpenAI(c *gin.Context, bodyReader io.ReadCloser) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.InternalError(c, "streaming not supported")
		return
	}

	converter := service.NewAnthropicSSEConverter()
	scanner := bufio.NewScanner(bodyReader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var eventType string
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			output := converter.ConvertEvent(eventType, data)
			if output != "" {
				c.Writer.Write([]byte(output))
				flusher.Flush()
			}
			eventType = ""
			continue
		}
	}
}

// ChannelsStatus returns current status of all channels.
func (h *LLMProxyHandler) ChannelsStatus(c *gin.Context) {
	response.Success(c, h.svc.GetChannelsStatus())
}

// Stats returns aggregated proxy statistics.
func (h *LLMProxyHandler) Stats(c *gin.Context) {
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours <= 0 {
		hours = 24
	}
	stats, err := h.svc.GetStats(hours)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, stats)
}

// Logs returns recent proxy request logs.
func (h *LLMProxyHandler) Logs(c *gin.Context) {
	channel := c.Query("channel")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 {
		limit = 50
	}
	logs, err := h.svc.GetRecentLogs(channel, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, logs)
}

// RateLimitEvents returns recent rate-limit events.
func (h *LLMProxyHandler) RateLimitEvents(c *gin.Context) {
	channel := c.Query("channel")
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours <= 0 {
		hours = 24
	}
	events, err := h.svc.GetRateLimitEvents(hours, channel)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, events)
}

// ListAlertEvents returns recent aggregated alert events (circuit-open, quota,
// balance, session) for the alerts page. Route: GET /api/llm/alerts.
func (h *LLMProxyHandler) ListAlertEvents(c *gin.Context) {
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	severity := c.Query("severity")
	alertType := c.Query("type")
	events, err := h.svc.ListAlertEvents(severity, alertType, hours, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, events)
}

// HealthStatus returns health status of all channels including circuit breaker state.
func (h *LLMProxyHandler) HealthStatus(c *gin.Context) {
	response.Success(c, h.svc.GetHealthStatus())
}

// GroupsStatus returns status of all virtual model groups.
func (h *LLMProxyHandler) GroupsStatus(c *gin.Context) {
	response.Success(c, h.svc.GetGroupsStatus())
}

// ClearGroupSticky clears all sticky bindings for the named model group.
func (h *LLMProxyHandler) ClearGroupSticky(c *gin.Context) {
	name := c.Param("name")
	cleared := h.svc.ClearGroupSticky(name)
	if cleared < 0 {
		response.NotFound(c, "model group not found: "+name)
		return
	}
	response.Success(c, gin.H{"cleared": cleared})
}

// ResetChannelCircuit resets the circuit breaker for the named channel.
func (h *LLMProxyHandler) ResetChannelCircuit(c *gin.Context) {
	name := c.Param("name")
	if ok := h.svc.ResetChannelCircuit(name); !ok {
		response.NotFound(c, "channel not found: "+name)
		return
	}
	response.Message(c, "circuit breaker reset for channel: "+name)
}

// --- Channel & Group Config CRUD ---

func (h *LLMProxyHandler) ListChannels(c *gin.Context) {
	channels, err := h.svc.ListChannelConfigs()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, channels)
}

func (h *LLMProxyHandler) CreateChannel(c *gin.Context) {
	var ch model.LLMChannel
	if err := c.ShouldBindJSON(&ch); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.CreateChannel(&ch); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, ch)
}

func (h *LLMProxyHandler) UpdateChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var ch model.LLMChannel
	if err := c.ShouldBindJSON(&ch); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	ch.ID = uint(id)
	if err := h.svc.UpdateChannel(&ch); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, ch)
}

func (h *LLMProxyHandler) DeleteChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.svc.DeleteChannel(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "channel deleted")
}

// --- Channel credentials (encrypted at rest, Tier 6) ---

// ListChannelCredentials returns the masked credentials for a channel.
// Route: GET /api/llm/config/channels/:id/credentials
func (h *LLMProxyHandler) ListChannelCredentials(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	creds, err := h.svc.ListChannelCredentials(uint(id))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, creds)
}

// CreateChannelCredential stores a new encrypted credential for a channel.
// Route: POST /api/llm/config/channels/:id/credentials
func (h *LLMProxyHandler) CreateChannelCredential(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var req struct {
		Purpose      string `json:"purpose"`
		Source       string `json:"source"`
		EnvVarName   string `json:"env_var_name"`
		ProviderType string `json:"provider_type"`
		Label        string `json:"label"`
		Credential   string `json:"credential"`
		Priority     int    `json:"priority"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	view, err := h.svc.CreateChannelCredential(uint(id), req.Purpose, req.Source, req.EnvVarName, req.ProviderType, req.Label, req.Credential, req.Priority)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, view)
}

// UpdateChannelCredential updates metadata and optionally rotates the secret.
// Route: PUT /api/llm/config/credentials/:id
func (h *LLMProxyHandler) UpdateChannelCredential(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var req struct {
		ProviderType string `json:"provider_type"`
		Status       string `json:"status"`
		Credential   string `json:"credential"`
		Purpose      string `json:"purpose"`
		Source       string `json:"source"`
		EnvVarName   string `json:"env_var_name"`
		Label        string `json:"label"`
		Priority     *int   `json:"priority"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	view, err := h.svc.UpdateChannelCredential(uint(id), req.ProviderType, req.Status, req.Credential, req.Purpose, req.Source, req.EnvVarName, req.Label, req.Priority)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, view)
}

// DeleteChannelCredential removes a stored credential.
// Route: DELETE /api/llm/config/credentials/:id
func (h *LLMProxyHandler) DeleteChannelCredential(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.svc.DeleteChannelCredential(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "credential deleted")
}

// ChannelBalanceHistory returns recent balance snapshots for a channel, newest first.
// Route: GET /api/llm/channels/:name/balance/history
func (h *LLMProxyHandler) ChannelBalanceHistory(c *gin.Context) {
	name := c.Param("name")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	snaps, err := h.svc.ChannelBalanceHistory(name, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, snaps)
}

func (h *LLMProxyHandler) ListGroups(c *gin.Context) {
	groups, err := h.svc.ListGroupConfigs()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, groups)
}

func (h *LLMProxyHandler) CreateGroup(c *gin.Context) {
	var g model.LLMModelGroup
	if err := c.ShouldBindJSON(&g); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.CreateGroup(&g); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, g)
}

func (h *LLMProxyHandler) UpdateGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var g model.LLMModelGroup
	if err := c.ShouldBindJSON(&g); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	g.ID = uint(id)
	if err := h.svc.UpdateGroup(&g); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, g)
}

func (h *LLMProxyHandler) DeleteGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.svc.DeleteGroup(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "group deleted")
}

func (h *LLMProxyHandler) ReloadConfig(c *gin.Context) {
	if err := h.svc.Reload(); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "configuration reloaded")
}

// --- Balance ---

func (h *LLMProxyHandler) ChannelBalance(c *gin.Context) {
	name := c.Param("name")
	info := h.svc.GetChannelBalance(name)
	if info == nil {
		response.NotFound(c, "no balance info for channel: "+name)
		return
	}
	response.Success(c, info)
}

func (h *LLMProxyHandler) AllBalances(c *gin.Context) {
	infos := h.svc.GetAllBalances()
	if infos == nil {
		response.Success(c, map[string]interface{}{})
		return
	}
	response.Success(c, infos)
}

func (h *LLMProxyHandler) RefreshBalances(c *gin.Context) {
	h.svc.RefreshBalances()
	response.Message(c, "balance refresh triggered")
}

// --- Token CRUD ---

func (h *LLMProxyHandler) ListTokens(c *gin.Context) {
	tokens, err := h.tokenRepo.List()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	// Don't expose key_hash
	for i := range tokens {
		tokens[i].KeyHash = ""
	}
	response.Success(c, tokens)
}

func (h *LLMProxyHandler) CreateToken(c *gin.Context) {
	var req struct {
		Name                  string   `json:"name"`
		CallerID              string   `json:"caller_id"`
		AllowedModels         []string `json:"allowed_models"`
		AllowedGroups         []string `json:"allowed_groups"`
		QuotaRequestsDaily    int      `json:"quota_requests_daily"`
		QuotaTokensDaily      int      `json:"quota_tokens_daily"`
		QuotaCostMonthlyCents int      `json:"quota_cost_monthly_cents"`
		ExpiresAt             *string  `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Name == "" || req.CallerID == "" {
		response.BadRequest(c, "name and caller_id are required")
		return
	}

	// Generate key: sk-bk-<random 32 hex chars>
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		response.InternalError(c, "failed to generate key")
		return
	}
	rawKey := "sk-bk-" + hex.EncodeToString(b)

	token := model.LLMToken{
		Name:                  req.Name,
		KeyHash:               model.HashKey(rawKey),
		KeyPrefix:             rawKey[:min(8, len(rawKey))],
		CallerID:              req.CallerID,
		QuotaRequestsDaily:    req.QuotaRequestsDaily,
		QuotaTokensDaily:      req.QuotaTokensDaily,
		QuotaCostMonthlyCents: req.QuotaCostMonthlyCents,
	}
	token.SetAllowedModels(req.AllowedModels)
	token.SetAllowedGroups(req.AllowedGroups)
	if req.ExpiresAt != nil {
		if t, err := time.Parse(time.RFC3339, *req.ExpiresAt); err == nil {
			token.ExpiresAt = &t
		}
	}

	if err := h.tokenRepo.Create(&token); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// Return the raw key ONLY on creation
	response.Success(c, gin.H{
		"token": token,
		"key":   rawKey,
	})
}

func (h *LLMProxyHandler) UpdateToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var req struct {
		Name                  string   `json:"name"`
		AllowedModels         []string `json:"allowed_models"`
		AllowedGroups         []string `json:"allowed_groups"`
		QuotaRequestsDaily    int      `json:"quota_requests_daily"`
		QuotaTokensDaily      int      `json:"quota_tokens_daily"`
		QuotaCostMonthlyCents int      `json:"quota_cost_monthly_cents"`
		Enabled               *bool    `json:"enabled"`
		ExpiresAt             *string  `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	token, err := h.tokenRepo.Get(uint(id))
	if err != nil {
		response.NotFound(c, "token not found")
		return
	}

	if req.Name != "" {
		token.Name = req.Name
	}
	token.SetAllowedModels(req.AllowedModels)
	token.SetAllowedGroups(req.AllowedGroups)
	token.QuotaRequestsDaily = req.QuotaRequestsDaily
	token.QuotaTokensDaily = req.QuotaTokensDaily
	token.QuotaCostMonthlyCents = req.QuotaCostMonthlyCents
	if req.Enabled != nil {
		token.Enabled = *req.Enabled
	}
	if req.ExpiresAt != nil {
		if t, err := time.Parse(time.RFC3339, *req.ExpiresAt); err == nil {
			token.ExpiresAt = &t
		} else {
			token.ExpiresAt = nil
		}
	}

	if err := h.tokenRepo.Update(token); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	token.KeyHash = ""
	response.Success(c, token)
}

func (h *LLMProxyHandler) DeleteToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.tokenRepo.Delete(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "token deleted")
}

func (h *LLMProxyHandler) RegenerateTokenKey(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	token, err := h.tokenRepo.Get(uint(id))
	if err != nil {
		response.NotFound(c, "token not found")
		return
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		response.InternalError(c, "failed to generate key")
		return
	}
	rawKey := "sk-bk-" + hex.EncodeToString(b)
	token.KeyHash = model.HashKey(rawKey)
	token.KeyPrefix = rawKey[:min(8, len(rawKey))]

	if err := h.tokenRepo.Update(token); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"key": rawKey})
}

func (h *LLMProxyHandler) GetTokenUsage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	daysStr := c.DefaultQuery("days", "7")
	days, _ := strconv.Atoi(daysStr)
	if days <= 0 {
		days = 7
	}
	if days > 90 {
		days = 90
	}

	to := time.Now()
	from := to.AddDate(0, 0, -days)
	usages, err := h.tokenUsageRepo.ListByToken(uint(id), from, to)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, usages)
}

// --- Pricing CRUD ---

func (h *LLMProxyHandler) ListPricing(c *gin.Context) {
	pricing, err := h.pricingRepo.List()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, pricing)
}

func (h *LLMProxyHandler) CreatePricing(c *gin.Context) {
	var p model.LLMModelPricing
	if err := c.ShouldBindJSON(&p); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if p.ChannelName == "" || p.Model == "" {
		response.BadRequest(c, "channel_name and model are required")
		return
	}
	if err := h.pricingRepo.Create(&p); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, p)
}

func (h *LLMProxyHandler) UpdatePricing(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var p model.LLMModelPricing
	if err := c.ShouldBindJSON(&p); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	p.ID = uint(id)
	if err := h.pricingRepo.Update(&p); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, p)
}

func (h *LLMProxyHandler) DeletePricing(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.pricingRepo.Delete(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "pricing deleted")
}

func (h *LLMProxyHandler) TestPricingCalc(c *gin.Context) {
	var req struct {
		ChannelName      string `json:"channel_name"`
		Model            string `json:"model"`
		PromptTokens     int    `json:"prompt_tokens"`
		CompletionTokens int    `json:"completion_tokens"`
		CachedTokens     int    `json:"cached_tokens"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	cost, err := h.pricer.Calc(req.ChannelName, req.Model, service.Usage{
		PromptTokens:     req.PromptTokens,
		CompletionTokens: req.CompletionTokens,
		CachedTokens:     req.CachedTokens,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{
		"cost_cents": cost,
		"cost_usd":   fmt.Sprintf("%.4f", float64(cost)/100),
	})
}

// --- Conversations ---

func (h *LLMProxyHandler) ListConversations(c *gin.Context) {
	bindings := h.svc.GetConversations()
	response.Success(c, bindings)
}

func (h *LLMProxyHandler) DeleteConversation(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		response.BadRequest(c, "conversation id required")
		return
	}
	if err := h.svc.DeleteConversation(convID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "conversation binding deleted")
}

// --- Usage / Billing ---

func (h *LLMProxyHandler) GetUsage(c *gin.Context) {
	groupBy := c.DefaultQuery("group_by", "date")
	fromStr := c.Query("from")
	toStr := c.Query("to")

	from := time.Now().AddDate(0, 0, -30)
	to := time.Now()
	if fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
	}

	data, err := h.svc.GetUsageAggregates(groupBy, from, to)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, data)
}

// --- Rate Limits ---

func (h *LLMProxyHandler) ListRateLimits(c *gin.Context) {
	data, err := h.svc.GetRateLimits()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, data)
}

func (h *LLMProxyHandler) ResetRateLimit(c *gin.Context) {
	channelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid channel_id")
		return
	}
	modelName := c.Param("model")
	if modelName == "" {
		response.BadRequest(c, "model required")
		return
	}
	if err := h.svc.ResetRateLimit(uint(channelID), modelName); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "rate limit reset")
}

func (h *LLMProxyHandler) LockRateLimit(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var req struct {
		Locked bool `json:"locked"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.LockRateLimit(uint(id), req.Locked); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "rate limit lock updated")
}

// --- Coding Strategy ---

func (h *LLMProxyHandler) GetCodingStrategy(c *gin.Context) {
	response.Success(c, gin.H{"strategy": h.svc.GetCodingStrategy()})
}

func (h *LLMProxyHandler) SetCodingStrategy(c *gin.Context) {
	var req struct {
		Strategy string `json:"strategy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Strategy != "free_first" && req.Strategy != "quality_first" && req.Strategy != "complexity_aware" {
		response.BadRequest(c, "strategy must be one of: free_first, quality_first, complexity_aware")
		return
	}
	h.svc.SetCodingStrategy(req.Strategy)
	response.Message(c, "coding strategy updated")
}
