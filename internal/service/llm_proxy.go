package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

// --- Token Bucket Rate Limiter ---

// TokenBucket implements a token-bucket rate limiter with daily counter.
// It proactively limits request rate on the client side to avoid triggering
// upstream 429 responses.
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time

	dailyCount int
	dailyLimit int
	dayStart   time.Time
}

func NewTokenBucket(rpm, rpd, defaultBucketRPM int) *TokenBucket {
	maxTokens := float64(rpm)
	if maxTokens == 0 {
		maxTokens = float64(defaultBucketRPM)
	}
	refillRate := maxTokens / 60.0

	return &TokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
		dailyLimit: rpd,
		dayStart:   startOfDay(time.Now()),
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// TryAcquire attempts to consume one token without blocking.
// Returns (allowed, suggestedWaitTime).
func (tb *TokenBucket) TryAcquire() (bool, time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	// Reset daily counter at midnight
	today := startOfDay(now)
	if today.After(tb.dayStart) {
		tb.dailyCount = 0
		tb.dayStart = today
	}

	// Check daily limit
	if tb.dailyLimit > 0 && tb.dailyCount >= tb.dailyLimit {
		nextDay := tb.dayStart.Add(24 * time.Hour)
		return false, nextDay.Sub(now)
	}

	// Refill tokens
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		tb.dailyCount++
		return true, 0
	}

	wait := time.Duration((1.0 - tb.tokens) / tb.refillRate * float64(time.Second))
	return false, wait
}

// Status returns current bucket state without consuming tokens.
func (tb *TokenBucket) Status() map[string]interface{} {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	currentTokens := tb.tokens + elapsed*tb.refillRate
	if currentTokens > tb.maxTokens {
		currentTokens = tb.maxTokens
	}

	return map[string]interface{}{
		"available_tokens":  int(currentTokens),
		"max_tokens":        int(tb.maxTokens),
		"daily_used":        tb.dailyCount,
		"daily_limit":       tb.dailyLimit,
		"refill_rate_per_s": fmt.Sprintf("%.2f", tb.refillRate),
	}
}

// --- Channel ---

// Channel represents a single upstream LLM API endpoint with its own rate limiter
// and health tracker.
type Channel struct {
	Config config.ChannelConfig
	Bucket *TokenBucket
	Client *http.Client
	Health *ChannelHealth
}

// --- LLM Proxy Service ---

// LLMProxyService provides a rate-limited, multi-channel OpenAI-compatible proxy
// with virtual model groups, sticky routing, and circuit breaker support.
type LLMProxyService struct {
	mu          sync.RWMutex
	cfg         config.LLMProxyConfig
	channels    map[string]*Channel      // name -> channel
	modelMap    map[string][]*Channel    // model -> sorted channels (by priority)
	modelGroups map[string]*ModelGroup   // group name -> model group
	repo        *repository.LLMProxyRepository
	channelRepo *repository.LLMChannelRepository
	groupRepo   *repository.LLMModelGroupRepository
	stopChans   []chan struct{}           // cleanup goroutine stop channels
}

func NewLLMProxyService(cfg config.LLMProxyConfig, repo *repository.LLMProxyRepository, channelRepo *repository.LLMChannelRepository, groupRepo *repository.LLMModelGroupRepository) *LLMProxyService {
	svc := &LLMProxyService{
		cfg:         cfg,
		channels:    make(map[string]*Channel),
		modelMap:    make(map[string][]*Channel),
		modelGroups: make(map[string]*ModelGroup),
		repo:        repo,
		channelRepo: channelRepo,
		groupRepo:   groupRepo,
	}

	if err := svc.loadFromDB(); err != nil {
		log.Printf("llm-proxy: failed to load config from DB: %v", err)
	}

	return svc
}

// loadFromDB loads channel and model group configuration from the database.
func (s *LLMProxyService) loadFromDB() error {
	dbChannels, err := s.channelRepo.ListEnabled()
	if err != nil {
		return fmt.Errorf("load channels: %w", err)
	}

	channels := make(map[string]*Channel)
	modelMap := make(map[string][]*Channel)

	for _, dbCh := range dbChannels {
		chCfg := dbChannelToConfig(dbCh)
		ch := &Channel{
			Config: chCfg,
			Bucket: NewTokenBucket(chCfg.RPM, chCfg.RPD, s.cfg.DefaultBucketRPM),
			Client: &http.Client{
				Timeout: time.Duration(s.cfg.DefaultTimeout) * time.Second,
			},
			Health: NewChannelHealth(s.cfg.CircuitBreaker),
		}
		channels[chCfg.Name] = ch

		for _, m := range chCfg.Models {
			modelMap[m] = append(modelMap[m], ch)
		}
	}

	// Sort channels per model by priority (lower = higher priority)
	for m := range modelMap {
		chs := modelMap[m]
		sort.Slice(chs, func(i, j int) bool {
			return chs[i].Config.Priority < chs[j].Config.Priority
		})
	}

	// Load model groups
	dbGroups, err := s.groupRepo.List()
	if err != nil {
		return fmt.Errorf("load groups: %w", err)
	}

	modelGroups := make(map[string]*ModelGroup)
	var stopChans []chan struct{}

	for _, dbGroup := range dbGroups {
		gCfg := dbGroupToConfig(dbGroup)
		group, err := NewModelGroup(gCfg, channels)
		if err != nil {
			log.Printf("llm-proxy: error initializing model group %q: %v", gCfg.Name, err)
			continue
		}
		if len(group.Members) == 0 {
			log.Printf("llm-proxy: warning: model group %q has no valid members, skipping", gCfg.Name)
			continue
		}
		modelGroups[gCfg.Name] = group

		stop := group.StartCleanup(1 * time.Minute)
		stopChans = append(stopChans, stop)

		log.Printf("llm-proxy: initialized model group %q with %d members (strategy=%s, sticky_ttl=%ds)",
			gCfg.Name, len(group.Members), gCfg.Strategy, gCfg.StickyTTLSeconds)
	}

	s.channels = channels
	s.modelMap = modelMap
	s.modelGroups = modelGroups
	s.stopChans = stopChans

	log.Printf("llm-proxy: loaded %d channels for %d models, %d model groups from DB",
		len(channels), len(modelMap), len(modelGroups))
	return nil
}

// Reload reloads all channels and model groups from the database,
// preserving health and rate-limiter state for unchanged channels.
func (s *LLMProxyService) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop old cleanup goroutines
	for _, stop := range s.stopChans {
		close(stop)
	}

	// Preserve existing channel state
	oldChannels := s.channels

	if err := s.loadFromDB(); err != nil {
		return err
	}

	// Restore health + bucket state for channels that still exist
	for name, newCh := range s.channels {
		if oldCh, ok := oldChannels[name]; ok {
			newCh.Health = oldCh.Health
			newCh.Bucket = oldCh.Bucket
		}
	}

	return nil
}

func dbChannelToConfig(ch model.LLMChannel) config.ChannelConfig {
	return config.ChannelConfig{
		Name:      ch.Name,
		BaseURL:   ch.BaseURL,
		APIKey:    os.Getenv(ch.APIKeyEnv),
		RPM:       ch.RPM,
		RPD:       ch.RPD,
		Priority:  ch.Priority,
		Models:    ch.GetModels(),
		IsEnabled: ch.IsEnabled,
		IsFree:    ch.IsFree,
	}
}

func dbGroupToConfig(g model.LLMModelGroup) config.ModelGroupConfig {
	members := make([]config.ModelGroupMember, len(g.Members))
	for i, m := range g.Members {
		members[i] = config.ModelGroupMember{
			Channel: m.ChannelName,
			Model:   m.Model,
			Weight:  m.Weight,
		}
	}
	return config.ModelGroupConfig{
		Name:             g.Name,
		Description:      g.Description,
		Strategy:         g.Strategy,
		StickyTTLSeconds: g.StickyTTLSeconds,
		Members:          members,
	}
}

// --- Channel & Group CRUD (delegates to repo, auto-reloads) ---

func (s *LLMProxyService) ListChannelConfigs() ([]model.LLMChannel, error) {
	return s.channelRepo.List()
}

func (s *LLMProxyService) GetChannelConfig(id uint) (*model.LLMChannel, error) {
	return s.channelRepo.Get(id)
}

func (s *LLMProxyService) CreateChannel(ch *model.LLMChannel) error {
	if err := s.channelRepo.Create(ch); err != nil {
		return err
	}
	return s.Reload()
}

func (s *LLMProxyService) UpdateChannel(ch *model.LLMChannel) error {
	if err := s.channelRepo.Update(ch); err != nil {
		return err
	}
	return s.Reload()
}

func (s *LLMProxyService) DeleteChannel(id uint) error {
	if err := s.channelRepo.Delete(id); err != nil {
		return err
	}
	return s.Reload()
}

func (s *LLMProxyService) ListGroupConfigs() ([]model.LLMModelGroup, error) {
	return s.groupRepo.List()
}

func (s *LLMProxyService) GetGroupConfig(id uint) (*model.LLMModelGroup, error) {
	return s.groupRepo.Get(id)
}

func (s *LLMProxyService) CreateGroup(g *model.LLMModelGroup) error {
	if err := s.groupRepo.Create(g); err != nil {
		return err
	}
	return s.Reload()
}

func (s *LLMProxyService) UpdateGroup(g *model.LLMModelGroup) error {
	if err := s.groupRepo.Update(g); err != nil {
		return err
	}
	return s.Reload()
}

func (s *LLMProxyService) DeleteGroup(id uint) error {
	if err := s.groupRepo.Delete(id); err != nil {
		return err
	}
	return s.Reload()
}

// ProxyRequest forwards an OpenAI-compatible request through rate-limited channels.
func (s *LLMProxyService) ProxyRequest(
	method, path string,
	headers http.Header,
	body []byte,
	callerID string,
) (statusCode int, respBody []byte, respHeaders http.Header, err error) {
	modelName := extractModelFromBody(body)

	// Snapshot current routing state for lock-free proxy operation
	s.mu.RLock()
	modelGroups := s.modelGroups
	modelMap := s.modelMap
	s.mu.RUnlock()

	// Derive task key for sticky routing
	taskKey := headers.Get("X-Task-Key")
	if taskKey == "" && callerID != "" && callerID != "unknown" {
		taskKey = callerID + ":" + modelName
	}

	// Check if model matches a virtual model group
	if group, ok := modelGroups[modelName]; ok {
		return s.proxyViaGroup(group, taskKey, method, path, headers, body, callerID)
	}

	// Direct channel matching (existing logic)
	channels := findChannelsInMap(modelMap, modelName)
	if len(channels) == 0 {
		errMsg := fmt.Sprintf("no channel available for model: %s", modelName)
		return 400, []byte(`{"error":"` + errMsg + `"}`), nil, fmt.Errorf("%s", errMsg)
	}

	// Filter out circuit-broken channels
	healthy := s.filterHealthy(channels)
	if len(healthy) == 0 {
		return 503, []byte(`{"error":"all channels circuit-broken for model: ` + modelName + `"}`), nil, nil
	}

	var lastErr error
	for _, ch := range healthy {
		statusCode, respBody, respHeaders, err = s.tryChannel(ch, method, path, headers, body, callerID)
		if err == nil && statusCode < 500 && statusCode != 429 {
			ch.Health.RecordSuccess()
			return statusCode, respBody, respHeaders, nil
		}
		ch.Health.RecordFailure(classifyError(statusCode, err))
		lastErr = err
		log.Printf("llm-proxy: channel %s returned %d for model %s, trying next",
			ch.Config.Name, statusCode, modelName)
	}

	return statusCode, respBody, respHeaders, lastErr
}

// proxyViaGroup routes a request through a virtual model group with sticky binding.
func (s *LLMProxyService) proxyViaGroup(
	group *ModelGroup,
	taskKey, method, path string,
	headers http.Header,
	body []byte,
	callerID string,
) (int, []byte, http.Header, error) {
	modelName := extractModelFromBody(body) // original virtual model name for logging
	maxAttempts := len(group.Members)
	tried := map[string]bool{}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		ch, realModel := group.SelectChannel(taskKey)
		if ch == nil {
			break
		}
		if tried[ch.Config.Name] {
			// Already tried this channel, clear its sticky binding to try another
			if taskKey != "" && group.Sticky != nil {
				group.Sticky.Remove(taskKey)
			}
			continue
		}
		tried[ch.Config.Name] = true

		// Rewrite the model name in request body to the real model name
		rewrittenBody := rewriteModel(body, realModel)

		statusCode, respBody, respHeaders, err := s.tryChannel(
			ch, method, path, headers, rewrittenBody, callerID,
		)

		// Log with original virtual model name for traceability
		s.logGroupRequest(ch.Config.Name, modelName, realModel, path, statusCode, err, callerID)

		if err == nil && statusCode < 500 && statusCode != 429 {
			ch.Health.RecordSuccess()
			if taskKey != "" && group.Sticky != nil {
				group.Sticky.Renew(taskKey, group.Config.StickyTTLSeconds)
			}
			return statusCode, respBody, respHeaders, nil
		}

		// Failure: update health, clear sticky binding, try next
		ch.Health.RecordFailure(classifyError(statusCode, err))
		if taskKey != "" && group.Sticky != nil {
			group.Sticky.Remove(taskKey)
		}
		log.Printf("llm-proxy: group %q channel %s (model %s) failed with %d, trying next member",
			group.Config.Name, ch.Config.Name, realModel, statusCode)
	}

	return 503, []byte(`{"error":"all group members exhausted for: ` + modelName + `"}`), nil, nil
}

// filterHealthy filters out channels whose circuit breaker is open.
func (s *LLMProxyService) filterHealthy(channels []*Channel) []*Channel {
	var healthy []*Channel
	for _, ch := range channels {
		if ch.Health.IsAvailable() {
			healthy = append(healthy, ch)
		}
	}
	return healthy
}

func (s *LLMProxyService) tryChannel(
	ch *Channel,
	method, path string,
	headers http.Header,
	body []byte,
	callerID string,
) (int, []byte, http.Header, error) {
	maxRetries := s.cfg.MaxRetries
	modelName := extractModelFromBody(body)
	var lastStatusCode int
	var lastBody []byte
	var lastHeaders http.Header

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Client-side rate limiting
		allowed, waitTime := ch.Bucket.TryAcquire()
		if !allowed {
			if attempt < maxRetries {
				maxWait := time.Duration(s.cfg.MaxWaitSeconds) * time.Second
				if waitTime > maxWait {
					waitTime = maxWait
				}
				log.Printf("llm-proxy: bucket throttle on %s, wait %v (attempt %d/%d)",
					ch.Config.Name, waitTime, attempt+1, maxRetries)
				time.Sleep(waitTime)
				continue
			}
			s.logRequest(ch.Config.Name, modelName, path, 429, true, attempt, 0, 0, 0,
				"client rate limit exhausted", callerID)
			return 429, []byte(`{"error":"rate limit: bucket exhausted after retries"}`), nil, nil
		}

		// Forward request upstream
		start := time.Now()
		targetURL := strings.TrimRight(ch.Config.BaseURL, "/") + path
		req, err := http.NewRequest(method, targetURL, bytes.NewReader(body))
		if err != nil {
			return 0, nil, nil, fmt.Errorf("create request: %w", err)
		}

		// Copy caller headers, override auth
		for k, vv := range headers {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
		req.Header.Set("Authorization", "Bearer "+ch.Config.APIKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ch.Client.Do(req)
		durationMs := int(time.Since(start).Milliseconds())

		if err != nil {
			s.logRequest(ch.Config.Name, modelName, path, 0, false, attempt, durationMs, 0, 0,
				err.Error(), callerID)
			return 0, nil, nil, fmt.Errorf("upstream request: %w", err)
		}

		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		lastStatusCode = resp.StatusCode
		lastBody = respBytes
		lastHeaders = resp.Header

		isRateLimit := resp.StatusCode == 429
		promptTokens, compTokens := extractTokenUsage(respBytes)

		s.logRequest(ch.Config.Name, modelName, path, resp.StatusCode, isRateLimit,
			attempt, durationMs, promptTokens, compTokens, "", callerID)

		if resp.StatusCode == 429 && attempt < maxRetries {
			backoff := s.calculateBackoff(attempt)
			log.Printf("llm-proxy: upstream 429 on %s, backoff %v (attempt %d/%d)",
				ch.Config.Name, backoff, attempt+1, maxRetries)
			time.Sleep(backoff)
			continue
		}

		return resp.StatusCode, respBytes, resp.Header, nil
	}

	return lastStatusCode, lastBody, lastHeaders, nil
}

// calculateBackoff returns exponential backoff with jitter, using configured cap and jitter factor.
func (s *LLMProxyService) calculateBackoff(attempt int) time.Duration {
	base := math.Pow(2, float64(attempt+1))
	cap := float64(s.cfg.BackoffCapSeconds)
	if base > cap {
		base = cap
	}
	jitter := base * s.cfg.BackoffJitter * rand.Float64()
	return time.Duration((base + jitter) * float64(time.Second))
}

func findChannelsInMap(modelMap map[string][]*Channel, modelName string) []*Channel {
	// Exact match
	if chs, ok := modelMap[modelName]; ok {
		return chs
	}
	// Case-insensitive match
	lower := strings.ToLower(modelName)
	for m, chs := range modelMap {
		if strings.ToLower(m) == lower {
			return chs
		}
	}
	// Substring match
	for m, chs := range modelMap {
		if strings.Contains(strings.ToLower(m), lower) || strings.Contains(lower, strings.ToLower(m)) {
			return chs
		}
	}
	return nil
}

func extractModelFromBody(body []byte) string {
	var req struct {
		Model string `json:"model"`
	}
	json.Unmarshal(body, &req)
	return req.Model
}

func extractTokenUsage(body []byte) (prompt, comp int) {
	var resp struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	json.Unmarshal(body, &resp)
	return resp.Usage.PromptTokens, resp.Usage.CompletionTokens
}

func (s *LLMProxyService) logRequest(channelName, modelName, path string, statusCode int,
	isRateLimit bool, retryCount, durationMs, promptTokens, compTokens int,
	errMsg, callerID string) {
	if s.repo == nil {
		return
	}
	go func() {
		entry := &model.LLMProxyLog{
			ChannelName:  channelName,
			Model:        modelName,
			RequestPath:  path,
			StatusCode:   statusCode,
			IsRateLimit:  isRateLimit,
			RetryCount:   retryCount,
			DurationMs:   durationMs,
			PromptTokens: promptTokens,
			CompTokens:   compTokens,
			ErrorMessage: errMsg,
			CallerID:     callerID,
			CreatedAt:    time.Now(),
		}
		if err := s.repo.CreateLog(entry); err != nil {
			log.Printf("warn: failed to log llm proxy request: %v", err)
		}
	}()
}

// logGroupRequest logs a model group proxy attempt for traceability.
func (s *LLMProxyService) logGroupRequest(channelName, virtualModel, realModel, path string,
	statusCode int, err error, callerID string) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	isRateLimit := statusCode == 429
	// Log with virtual model name so operators can trace group requests
	s.logRequest(channelName, virtualModel+"→"+realModel, path, statusCode, isRateLimit,
		0, 0, 0, 0, errMsg, callerID)
}

// --- Management APIs ---

// GetChannelsStatus returns current status of all channels including bucket state and health.
func (s *LLMProxyService) GetChannelsStatus() []map[string]interface{} {
	s.mu.RLock()
	channels := s.channels
	s.mu.RUnlock()

	var result []map[string]interface{}
	for name, ch := range channels {
		status := ch.Bucket.Status()
		status["name"] = name
		status["base_url"] = ch.Config.BaseURL
		status["models"] = ch.Config.Models
		status["priority"] = ch.Config.Priority
		status["rpm_limit"] = ch.Config.RPM
		status["rpd_limit"] = ch.Config.RPD
		status["is_free"] = ch.Config.IsFree
		status["health"] = ch.Health.Status()
		result = append(result, status)
	}
	return result
}

// GetHealthStatus returns health status of all channels.
func (s *LLMProxyService) GetHealthStatus() []map[string]interface{} {
	s.mu.RLock()
	channels := s.channels
	s.mu.RUnlock()

	var result []map[string]interface{}
	for name, ch := range channels {
		entry := map[string]interface{}{
			"name":     name,
			"models":   ch.Config.Models,
			"priority": ch.Config.Priority,
			"is_free":  ch.Config.IsFree,
			"health":   ch.Health.Status(),
		}
		result = append(result, entry)
	}
	return result
}

// GetGroupsStatus returns status of all virtual model groups.
func (s *LLMProxyService) GetGroupsStatus() []map[string]interface{} {
	s.mu.RLock()
	modelGroups := s.modelGroups
	s.mu.RUnlock()

	var result []map[string]interface{}
	for _, group := range modelGroups {
		result = append(result, group.Status())
	}
	return result
}

// ClearGroupSticky clears all sticky bindings for the named model group.
// Returns the number of bindings cleared, or -1 if the group doesn't exist.
func (s *LLMProxyService) ClearGroupSticky(groupName string) int {
	s.mu.RLock()
	group, ok := s.modelGroups[groupName]
	s.mu.RUnlock()

	if !ok {
		return -1
	}
	if group.Sticky == nil {
		return 0
	}
	return group.Sticky.Clear()
}

// ResetChannelCircuit resets the circuit breaker for the named channel.
// Returns false if the channel doesn't exist.
func (s *LLMProxyService) ResetChannelCircuit(channelName string) bool {
	s.mu.RLock()
	ch, ok := s.channels[channelName]
	s.mu.RUnlock()

	if !ok {
		return false
	}
	ch.Health.Reset()
	return true
}

// GetStats returns aggregated statistics for the given time window.
func (s *LLMProxyService) GetStats(hours int) (map[string]interface{}, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	stats, err := s.repo.GetStats(since)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"channels": stats, "since": since}, nil
}

// GetRecentLogs returns recent proxy request logs.
func (s *LLMProxyService) GetRecentLogs(channelName string, limit int) ([]model.LLMProxyLog, error) {
	return s.repo.GetRecentLogs(channelName, limit)
}

// GetRateLimitEvents returns recent rate-limit events.
func (s *LLMProxyService) GetRateLimitEvents(hours int, channelName string) ([]model.LLMProxyLog, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	return s.repo.GetRateLimitEvents(since, channelName)
}
