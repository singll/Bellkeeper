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

func NewTokenBucket(rpm, rpd int) *TokenBucket {
	maxTokens := float64(rpm)
	if maxTokens == 0 {
		maxTokens = 1000 // effectively unlimited
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

// Channel represents a single upstream LLM API endpoint with its own rate limiter.
type Channel struct {
	Config config.ChannelConfig
	Bucket *TokenBucket
	Client *http.Client
}

// --- LLM Proxy Service ---

// LLMProxyService provides a rate-limited, multi-channel OpenAI-compatible proxy.
type LLMProxyService struct {
	cfg      config.LLMProxyConfig
	channels map[string]*Channel   // name -> channel
	modelMap map[string][]*Channel // model -> sorted channels (by priority)
	repo     *repository.LLMProxyRepository
}

func NewLLMProxyService(cfg config.LLMProxyConfig, repo *repository.LLMProxyRepository) *LLMProxyService {
	svc := &LLMProxyService{
		cfg:      cfg,
		channels: make(map[string]*Channel),
		modelMap: make(map[string][]*Channel),
		repo:     repo,
	}

	for _, chCfg := range cfg.Channels {
		if !chCfg.IsEnabled {
			continue
		}
		ch := &Channel{
			Config: chCfg,
			Bucket: NewTokenBucket(chCfg.RPM, chCfg.RPD),
			Client: &http.Client{
				Timeout: time.Duration(cfg.DefaultTimeout) * time.Second,
			},
		}
		svc.channels[chCfg.Name] = ch

		for _, m := range chCfg.Models {
			svc.modelMap[m] = append(svc.modelMap[m], ch)
		}
	}

	// Sort channels per model by priority (lower = higher priority)
	for m := range svc.modelMap {
		chs := svc.modelMap[m]
		sort.Slice(chs, func(i, j int) bool {
			return chs[i].Config.Priority < chs[j].Config.Priority
		})
	}

	log.Printf("llm-proxy: initialized %d channels for %d models", len(svc.channels), len(svc.modelMap))
	return svc
}

// ProxyRequest forwards an OpenAI-compatible request through rate-limited channels.
func (s *LLMProxyService) ProxyRequest(
	method, path string,
	headers http.Header,
	body []byte,
	callerID string,
) (statusCode int, respBody []byte, respHeaders http.Header, err error) {
	modelName := extractModelFromBody(body)

	channels := s.findChannels(modelName)
	if len(channels) == 0 {
		errMsg := fmt.Sprintf("no channel available for model: %s", modelName)
		return 400, []byte(`{"error":"` + errMsg + `"}`), nil, fmt.Errorf("%s", errMsg)
	}

	var lastErr error
	for _, ch := range channels {
		statusCode, respBody, respHeaders, err = s.tryChannel(ch, method, path, headers, body, callerID)
		if err == nil && statusCode < 500 && statusCode != 429 {
			return statusCode, respBody, respHeaders, nil
		}
		lastErr = err
		log.Printf("llm-proxy: channel %s returned %d for model %s, trying next",
			ch.Config.Name, statusCode, modelName)
	}

	return statusCode, respBody, respHeaders, lastErr
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
				if waitTime > 30*time.Second {
					waitTime = 30 * time.Second
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
			backoff := calculateBackoff(attempt)
			log.Printf("llm-proxy: upstream 429 on %s, backoff %v (attempt %d/%d)",
				ch.Config.Name, backoff, attempt+1, maxRetries)
			time.Sleep(backoff)
			continue
		}

		return resp.StatusCode, respBytes, resp.Header, nil
	}

	return lastStatusCode, lastBody, lastHeaders, nil
}

// calculateBackoff returns exponential backoff with jitter: 2^(attempt+1) * (1 + 0~50% jitter), capped at 60s.
func calculateBackoff(attempt int) time.Duration {
	base := math.Pow(2, float64(attempt+1))
	if base > 60 {
		base = 60
	}
	jitter := base * 0.5 * rand.Float64()
	return time.Duration((base + jitter) * float64(time.Second))
}

func (s *LLMProxyService) findChannels(modelName string) []*Channel {
	// Exact match
	if chs, ok := s.modelMap[modelName]; ok {
		return chs
	}
	// Case-insensitive match
	lower := strings.ToLower(modelName)
	for m, chs := range s.modelMap {
		if strings.ToLower(m) == lower {
			return chs
		}
	}
	// Substring match
	for m, chs := range s.modelMap {
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

// --- Management APIs ---

// GetChannelsStatus returns current status of all channels including bucket state.
func (s *LLMProxyService) GetChannelsStatus() []map[string]interface{} {
	var result []map[string]interface{}
	for name, ch := range s.channels {
		status := ch.Bucket.Status()
		status["name"] = name
		status["base_url"] = ch.Config.BaseURL
		status["models"] = ch.Config.Models
		status["priority"] = ch.Config.Priority
		status["rpm_limit"] = ch.Config.RPM
		status["rpd_limit"] = ch.Config.RPD
		result = append(result, status)
	}
	return result
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
