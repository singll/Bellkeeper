package service

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llm/balance"
	"github.com/singll/bellkeeper/internal/llm/converter"
	llmerrors "github.com/singll/bellkeeper/internal/llm/errors"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/httpclient"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
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

// Resize adjusts the bucket's RPM (max tokens + refill rate) while preserving the
// current token level and daily counter. Used when adaptive rate-limit learning
// updates the safe RPM and the config is reloaded.
func (tb *TokenBucket) Resize(rpm, defaultBucketRPM int) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	maxTokens := float64(rpm)
	if maxTokens == 0 {
		maxTokens = float64(defaultBucketRPM)
	}
	tb.maxTokens = maxTokens
	tb.refillRate = maxTokens / 60.0
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
}

// --- Channel ---

// Channel represents a single upstream LLM API endpoint with its own rate limiter
// and health tracker.
type Channel struct {
	Config      config.ChannelConfig
	Bucket      *TokenBucket
	Client      *httpclient.Client
	Health      *ChannelHealth
	ewmaLatency float64 // ms, exponential weighted moving average
	mu          sync.Mutex
}

// RecordLatency updates the channel's EWMA latency.
func (ch *Channel) RecordLatency(durationMs int) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.ewmaLatency == 0 {
		ch.ewmaLatency = float64(durationMs)
	} else {
		const alpha = 0.1
		ch.ewmaLatency = alpha*float64(durationMs) + (1-alpha)*ch.ewmaLatency
	}
}

// EWMALatency returns the current EWMA latency in milliseconds.
func (ch *Channel) EWMALatency() float64 {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.ewmaLatency
}

// --- LLM Proxy Service ---

// LLMProxyService provides a rate-limited, multi-channel OpenAI-compatible proxy
// with virtual model groups, sticky routing, and circuit breaker support.
type LLMProxyService struct {
	mu             sync.RWMutex
	cfg            config.LLMProxyConfig
	channels       map[string]*Channel      // name -> channel
	modelMap       map[string][]*Channel    // model -> sorted channels (by priority)
	modelGroups    map[string]*ModelGroup   // group name -> model group
	repo           *repository.LLMProxyRepository
	channelRepo    *repository.LLMChannelRepository
	groupRepo      *repository.LLMModelGroupRepository
	pricer         *Pricer
	tokenUsageRepo *repository.LLMTokenUsageRepository
	balanceMgr     *balance.Manager
	rateLimitLearner *RateLimitLearner
	taskRouter     *TaskRouter
	convMgr        *ConversationBindingManager
	tokenRepo      *repository.LLMTokenRepository
	credentialRepo      *repository.LLMChannelCredentialRepository
	balanceSnapshotRepo *repository.LLMChannelBalanceSnapshotRepository
	alertAggregator *AlertAggregator
	groupStopChans []chan struct{} // per-load group-cleanup goroutines (recreated on reload)
	bgStopChans    []chan struct{} // long-lived background loops (conv cleanup, archival, quota)
}

func NewLLMProxyService(cfg config.LLMProxyConfig, repo *repository.LLMProxyRepository, channelRepo *repository.LLMChannelRepository, groupRepo *repository.LLMModelGroupRepository, pricer *Pricer, tokenUsageRepo *repository.LLMTokenUsageRepository, rateLimitRepo *repository.LLMRateLimitRepository, convRepo *repository.ConversationBindingRepository, tokenRepo *repository.LLMTokenRepository, credentialRepo *repository.LLMChannelCredentialRepository, balanceSnapshotRepo *repository.LLMChannelBalanceSnapshotRepository) *LLMProxyService {
	svc := &LLMProxyService{
		cfg:            cfg,
		channels:       make(map[string]*Channel),
		modelMap:       make(map[string][]*Channel),
		modelGroups:    make(map[string]*ModelGroup),
		repo:           repo,
		channelRepo:    channelRepo,
		groupRepo:      groupRepo,
		pricer:         pricer,
		tokenUsageRepo: tokenUsageRepo,
		tokenRepo:      tokenRepo,
		credentialRepo:      credentialRepo,
		balanceSnapshotRepo: balanceSnapshotRepo,
	}

	// Initialize the rate-limit learner BEFORE loading channels so learned-safe RPM
	// is available to size token buckets at first load (not just after a reload).
	svc.rateLimitLearner = NewRateLimitLearner(rateLimitRepo)
	_ = svc.rateLimitLearner.LoadCache()
	svc.rateLimitLearner.Start()

	if err := svc.loadFromDB(); err != nil {
		middleware.GetLogger().Error("failed to load llm-proxy config from DB", zap.Error(err))
	}

	// Initialize balance manager
	if cfg.BalanceSyncInterval > 0 {
		svc.balanceMgr = balance.NewManager(time.Duration(cfg.BalanceSyncInterval) * time.Second)
		svc.registerBalanceProviders()
		svc.balanceMgr.Start()
	}

	// Initialize task router
	svc.taskRouter = NewTaskRouter(cfg.CodingRoutingStrategy)

	// Alert aggregator (nil notifier until Matrix infra wires one in via
	// SetAlertNotifier — until then it still buffers + persists events to DB).
	svc.alertAggregator = NewAlertAggregator(repo, nil)
	svc.alertAggregator.Start()

	// Initialize conversation binding manager
	svc.convMgr = NewConversationBindingManager(convRepo, 24*time.Hour)
	_ = svc.convMgr.LoadFromDB()

	// Long-lived background loops — their stop channels live in bgStopChans and are
	// NOT touched by Reload() (which only cycles group-cleanup goroutines).
	convStop := make(chan struct{})
	svc.bgStopChans = append(svc.bgStopChans, convStop)
	go svc.convCleanupLoop(convStop)

	archiveStop := make(chan struct{})
	svc.bgStopChans = append(svc.bgStopChans, archiveStop)
	svc.startLogArchival(archiveStop)

	if tokenRepo != nil {
		quotaStop := make(chan struct{})
		svc.bgStopChans = append(svc.bgStopChans, quotaStop)
		go svc.quotaAlertLoop(quotaStop)
	}

	// Balance snapshot loop — persists a history row per channel whenever the
	// balance manager publishes a fresh fetch (deduped by FetchedAt). Long-lived,
	// so its stop channel lives in bgStopChans (not cycled by Reload).
	if svc.balanceMgr != nil && balanceSnapshotRepo != nil {
		snapStop := make(chan struct{})
		svc.bgStopChans = append(svc.bgStopChans, snapStop)
		go svc.balanceSnapshotLoop(snapStop)
	}

	// Kimi Code probe loop — re-probes quota_exhausted channels after their breakdown
	// window elapses (§2.6.4). Long-lived → bgStopChans.
	probeStop := make(chan struct{})
	svc.bgStopChans = append(svc.bgStopChans, probeStop)
	go svc.kimiCodeProbeLoop(probeStop)

	return svc
}

func (s *LLMProxyService) registerBalanceProviders() {
	if s.balanceMgr == nil {
		return
	}
	for name, ch := range s.channels {
		if ch.Config.BalanceProviderType == "" {
			continue
		}
		apiKey := ch.Config.APIKey
		if apiKey == "" {
			continue
		}
		if err := s.balanceMgr.Register(name, ch.Config.BalanceProviderType, ch.Config.BaseURL, apiKey, ch.Config.BalanceConfigJSON); err != nil {
			middleware.GetLogger().Warn("failed to register balance provider",
				zap.String("channel", name), zap.Error(err))
		}
	}
}

// GetChannelBalance returns the last fetched balance for a channel.
func (s *LLMProxyService) GetChannelBalance(channelName string) *balance.Info {
	if s.balanceMgr == nil {
		return nil
	}
	return s.balanceMgr.Get(channelName)
}

// GetAllBalances returns all channel balances.
func (s *LLMProxyService) GetAllBalances() map[string]*balance.Info {
	if s.balanceMgr == nil {
		return nil
	}
	return s.balanceMgr.GetAll()
}

// RefreshBalances triggers an immediate balance refresh.
func (s *LLMProxyService) RefreshBalances() {
	if s.balanceMgr == nil {
		return
	}
	s.balanceMgr.RefreshAll()
}

// balanceSnapshotLoop persists a balance-history row per channel each time the
// balance manager publishes a fresh fetch. It ticks at the balance sync interval
// and dedupes by FetchedAt so a channel that hasn't refreshed isn't re-recorded.
func (s *LLMProxyService) balanceSnapshotLoop(stopCh <-chan struct{}) {
	interval := time.Duration(s.cfg.BalanceSyncInterval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastSnapped := make(map[string]time.Time) // channel -> last persisted FetchedAt
	for {
		select {
		case <-ticker.C:
			s.snapshotBalances(lastSnapped)
		case <-stopCh:
			return
		}
	}
}

// snapshotBalances reads the latest balances and persists one history row per
// channel whose fetch timestamp advanced since the previous tick. Failed fetches
// (info.Error set) are skipped so the history stays a clean record of real balances.
func (s *LLMProxyService) snapshotBalances(lastSnapped map[string]time.Time) {
	if s.balanceMgr == nil || s.balanceSnapshotRepo == nil {
		return
	}
	all := s.balanceMgr.GetAll()
	if len(all) == 0 {
		return
	}
	// Resolve channel name -> DB id from the in-memory channel table (one RLock,
	// no per-channel DB round-trip).
	ids := make(map[string]uint, len(all))
	s.mu.RLock()
	for name, ch := range s.channels {
		ids[name] = ch.Config.ID
	}
	s.mu.RUnlock()

	for name, info := range all {
		if info == nil || info.Error != "" || info.FetchedAt.IsZero() {
			continue
		}
		if prev, ok := lastSnapped[name]; ok && !info.FetchedAt.After(prev) {
			continue // already captured this fetch
		}
		raw, err := json.Marshal(info)
		if err != nil {
			middleware.GetLogger().Warn("balance snapshot: marshal failed",
				zap.String("channel", name), zap.Error(err))
			continue
		}
		snap := &model.LLMChannelBalanceSnapshot{
			ChannelID:    ids[name],
			ChannelName:  name,
			BalanceUSD:   info.Balance,
			Currency:     info.Currency,
			TotalGranted: info.TotalGranted,
			TotalUsed:    info.TotalUsed,
			BalanceRaw:   string(raw),
			FetchedAt:    info.FetchedAt,
		}
		if err := s.balanceSnapshotRepo.Create(snap); err != nil {
			middleware.GetLogger().Warn("balance snapshot: persist failed",
				zap.String("channel", name), zap.Error(err))
			continue
		}
		lastSnapped[name] = info.FetchedAt
	}
}

// kimiCodeProbeLoop periodically re-probes channels stuck in a long quota_exhausted
// breakdown (notably Kimi Code, whose quota resets on a ~5h/7d cycle). When a
// channel's breakdown window has elapsed but no live traffic has reopened it, a
// minimal 1-token probe checks whether the upstream quota actually recovered; on
// success the circuit is reset, otherwise the breakdown is re-armed so probes stay
// ~5h apart rather than firing every tick. Long-lived → registered in bgStopChans.
func (s *LLMProxyService) kimiCodeProbeLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.probeQuotaExhausted()
		case <-stopCh:
			return
		}
	}
}

// probeQuotaExhausted sends a recovery probe to each channel whose circuit is open
// with a quota_exhausted breakdown whose window has elapsed.
func (s *LLMProxyService) probeQuotaExhausted() {
	s.mu.RLock()
	channels := make([]*Channel, 0, len(s.channels))
	for _, ch := range s.channels {
		channels = append(channels, ch)
	}
	s.mu.RUnlock()

	now := time.Now()
	for _, ch := range channels {
		state, class, until := ch.Health.BreakdownInfo()
		if state == CircuitClosed || class != string(llmerrors.QuotaExhausted) {
			continue
		}
		if until.IsZero() || now.Before(until) {
			continue // still within the breakdown window — wait
		}
		statusCode, respBody, _, err := s.probeChannel(ch)
		if err == nil && statusCode >= 200 && statusCode < 400 {
			ch.Health.Reset()
			middleware.GetLogger().Info("quota-exhausted channel recovered via probe",
				zap.String("channel", ch.Config.Name), zap.Int("status", statusCode))
			continue
		}
		// Still down — re-arm the breakdown silently (no alert) so the next probe is
		// ~5h out, not on the next tick.
		result := llmerrors.Classify(statusCode, string(respBody), ch.Config.ProviderType)
		ch.Health.RecordClassifiedFailure(classifyError(statusCode, err), string(result.Class),
			llmerrors.BreakdownDuration(result.BreakdownUntil))
		middleware.GetLogger().Info("quota-exhausted channel still down after probe",
			zap.String("channel", ch.Config.Name), zap.Int("status", statusCode))
	}
}

// probeChannel sends a minimal 1-token chat completion to verify upstream recovery.
// It uses tokenID 0 (skips billing) and a synthetic caller id, and goes through
// tryChannel so provider auth/bucket/logging apply identically to real traffic.
func (s *LLMProxyService) probeChannel(ch *Channel) (int, []byte, http.Header, error) {
	model := ch.Config.Name
	if len(ch.Config.Models) > 0 {
		model = ch.Config.Models[0]
	}
	body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}],"max_tokens":1}`, model))
	return s.tryChannel(ch, "POST", "/v1/chat/completions", http.Header{}, body, "kimi-code-probe", 0)
}

// Stop halts background services (balance manager, learner, aggregator, loops).
func (s *LLMProxyService) Stop() {
	s.mu.Lock()
	for _, stop := range s.groupStopChans {
		close(stop)
	}
	s.groupStopChans = nil
	s.mu.Unlock()
	for _, stop := range s.bgStopChans {
		close(stop)
	}
	s.bgStopChans = nil
	if s.alertAggregator != nil {
		s.alertAggregator.Stop()
	}
	if s.balanceMgr != nil {
		s.balanceMgr.Stop()
	}
	if s.rateLimitLearner != nil {
		s.rateLimitLearner.Stop()
	}
}

// SetAlertNotifier wires the delivery notifier into the alert aggregator. Called
// from app setup once the Matrix notification service is available.
func (s *LLMProxyService) SetAlertNotifier(n AlertNotifier) {
	if s.alertAggregator != nil {
		s.alertAggregator.SetNotifier(n)
	}
}

// recordAlert buffers an alert event (no-op if the aggregator isn't initialized).
func (s *LLMProxyService) recordAlert(alertType, severity, channel, msg string) {
	if s.alertAggregator != nil {
		s.alertAggregator.Record(alertType, severity, channel, msg)
	}
}

// alertForClass maps a semantic upstream error class to an aggregated alert so
// operators learn about quota/balance/session/auth failures (§2.6.6).
func (s *LLMProxyService) alertForClass(channel string, class llmerrors.Class) {
	switch class {
	case llmerrors.QuotaExhausted:
		s.recordAlert("quota_exhausted", "error", channel, fmt.Sprintf("channel %s quota exhausted", channel))
	case llmerrors.BalanceZero:
		s.recordAlert("balance_zero", "error", channel, fmt.Sprintf("channel %s balance depleted — please top up", channel))
	case llmerrors.SessionExpired:
		s.recordAlert("session_expired", "error", channel, fmt.Sprintf("channel %s session expired — re-import credentials", channel))
	case llmerrors.AuthFailed:
		s.recordAlert("auth_failed", "critical", channel, fmt.Sprintf("channel %s auth failed — API key invalid", channel))
	case llmerrors.SubscriptionInvalid:
		s.recordAlert("subscription_invalid", "error", channel, fmt.Sprintf("channel %s subscription validation failed", channel))
	}
}

// ListAlertEvents returns recent alert events for the alerts UI.
func (s *LLMProxyService) ListAlertEvents(severity, alertType string, hours, limit int) ([]model.LLMAlertEvent, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("proxy repo not initialized")
	}
	if hours <= 0 {
		hours = 24
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	return s.repo.ListAlertEvents(since, severity, alertType, limit)
}

// quotaAlertLoop runs hourly, comparing each token's usage against its quotas and
// emitting 80% / 95% / 100% alerts through the aggregator (§2.6.2).
func (s *LLMProxyService) quotaAlertLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.checkQuotas()
		case <-stopCh:
			return
		}
	}
}

func (s *LLMProxyService) checkQuotas() {
	if s.tokenRepo == nil || s.tokenUsageRepo == nil || s.alertAggregator == nil {
		return
	}
	tokens, err := s.tokenRepo.List()
	if err != nil {
		middleware.GetLogger().Warn("quota check: list tokens failed", zap.Error(err))
		return
	}
	nowUTC := time.Now().UTC()
	today := time.Now().Truncate(24 * time.Hour)
	monthStart := time.Date(nowUTC.Year(), nowUTC.Month(), 1, 0, 0, 0, 0, time.UTC)
	for _, tok := range tokens {
		if !tok.Enabled {
			continue
		}
		if tok.QuotaRequestsDaily > 0 || tok.QuotaTokensDaily > 0 {
			usages, _ := s.tokenUsageRepo.ListByToken(tok.ID, today, today)
			reqs, toks := 0, 0
			for _, u := range usages {
				reqs += u.Requests
				toks += u.PromptTokens + u.CompletionTokens
			}
			if tok.QuotaRequestsDaily > 0 {
				s.emitQuotaAlert(tok.Name, "daily requests", reqs, tok.QuotaRequestsDaily)
			}
			if tok.QuotaTokensDaily > 0 {
				s.emitQuotaAlert(tok.Name, "daily tokens", toks, tok.QuotaTokensDaily)
			}
		}
		if tok.QuotaCostMonthlyCents > 0 {
			usages, _ := s.tokenUsageRepo.ListByToken(tok.ID, monthStart, nowUTC)
			var micro int64
			for _, u := range usages {
				micro += u.CostMicroCents
			}
			s.emitQuotaAlert(tok.Name, "monthly cost (cents)", int(MicroCentsToCents(micro)), tok.QuotaCostMonthlyCents)
		}
	}
}

func (s *LLMProxyService) emitQuotaAlert(tokenName, dimension string, used, limit int) {
	if limit <= 0 {
		return
	}
	pct := float64(used) / float64(limit) * 100
	var severity string
	switch {
	case pct >= 100:
		severity = "critical"
	case pct >= 95:
		severity = "error"
	case pct >= 80:
		severity = "warning"
	default:
		return
	}
	s.recordAlert("quota_threshold", severity, tokenName,
		fmt.Sprintf("token %q %s at %.0f%% (%d/%d)", tokenName, dimension, pct, used, limit))
}

// validateBinding checks if a conversation binding is still valid.
func (s *LLMProxyService) validateBinding(binding *model.LLMConversationBinding) error {
	if time.Now().After(binding.ExpiresAt) {
		return fmt.Errorf("binding expired")
	}
	s.mu.RLock()
	ch, ok := s.channels[binding.ChannelName]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("bound channel %s no longer exists", binding.ChannelName)
	}
	if !ch.Health.IsAvailable() {
		return fmt.Errorf("bound channel %s is unavailable", binding.ChannelName)
	}
	return nil
}

// headerToMap converts http.Header to a simple map[string]string (first value only).
func headerToMap(h http.Header) map[string]string {
	m := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			m[k] = v[0]
		}
	}
	return m
}

// convCleanupLoop periodically cleans up expired conversation bindings.
func (s *LLMProxyService) convCleanupLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if s.convMgr != nil {
				s.convMgr.CleanupExpired()
			}
		case <-stopCh:
			return
		}
	}
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
			Bucket: NewTokenBucket(s.effectiveBucketRPM(chCfg), chCfg.RPD, s.cfg.DefaultBucketRPM),
			Client: httpclient.NewClientWithTimeout(time.Duration(s.cfg.DefaultTimeout) * time.Second),
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
			middleware.GetLogger().Error("error initializing model group",
				zap.String("group", gCfg.Name), zap.Error(err))
			continue
		}
		if len(group.Members) == 0 {
			middleware.GetLogger().Warn("model group has no valid members, skipping",
				zap.String("group", gCfg.Name))
			continue
		}
		modelGroups[gCfg.Name] = group

		stop := group.StartCleanup(1 * time.Minute)
		stopChans = append(stopChans, stop)

		middleware.GetLogger().Info("initialized model group",
			zap.String("group", gCfg.Name), zap.Int("members", len(group.Members)),
			zap.String("strategy", gCfg.Strategy), zap.Int("sticky_ttl_seconds", gCfg.StickyTTLSeconds))
	}

	s.channels = channels
	s.modelMap = modelMap
	s.modelGroups = modelGroups
	s.groupStopChans = stopChans

	middleware.GetLogger().Info("loaded llm-proxy config from DB",
		zap.Int("channels", len(channels)), zap.Int("models", len(modelMap)),
		zap.Int("model_groups", len(modelGroups)))
	return nil
}

// Reload reloads all channels and model groups from the database,
// preserving health and rate-limiter state for unchanged channels.
func (s *LLMProxyService) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop only the per-load group-cleanup goroutines. Long-lived background loops
	// (conv cleanup, log archival, quota alerts) live in bgStopChans and must NOT be
	// touched here, or they'd silently die on the first reload (audit #9).
	for _, stop := range s.groupStopChans {
		close(stop)
	}
	s.groupStopChans = nil

	// Preserve existing channel state
	oldChannels := s.channels

	if err := s.loadFromDB(); err != nil {
		return err
	}

	// Restore health + bucket state for channels that still exist. Carry the old
	// bucket (preserves token level + daily counter) but RESIZE it to the freshly
	// learned-safe RPM so adaptive rate-limit learning takes effect on reload.
	for name, newCh := range s.channels {
		if oldCh, ok := oldChannels[name]; ok {
			newCh.Health = oldCh.Health
			oldCh.Bucket.Resize(s.effectiveBucketRPM(newCh.Config), s.cfg.DefaultBucketRPM)
			newCh.Bucket = oldCh.Bucket
		}
	}

	return nil
}

func dbChannelToConfig(ch model.LLMChannel) config.ChannelConfig {
	// Resolve API key: try as env var name first, fallback to direct value
	apiKey := os.Getenv(ch.APIKeyEnv)
	if apiKey == "" {
		apiKey = ch.APIKeyEnv
	}
	providerType := ch.ProviderType
	if providerType == "" {
		providerType = "openai"
	}

	return config.ChannelConfig{
		ID:                  ch.ID,
		Name:                ch.Name,
		BaseURL:             ch.BaseURL,
		APIKey:              apiKey,
		ProviderType:        providerType,
		RPM:                 ch.RPM,
		RPD:                 ch.RPD,
		Priority:            ch.Priority,
		Models:              ch.GetModels(),
		IsEnabled:           ch.IsEnabled,
		IsFree:              ch.IsFree,
		BalanceProviderType: ch.BalanceProviderType,
		BalanceConfigJSON:   ch.BalanceConfigJSON,
		ModelRPMOverrides:   ch.ModelRPMOverrides,
		TaskTypes:           ch.GetTaskTypes(),
		Tier:                ch.Tier,
	}
}

// effectiveBucketRPM seeds per-channel-model rate-limit rows with the configured
// baseline and returns the learned-safe RPM used to size the channel's token bucket
// (min across the channel's models — conservative). Falls back to the configured RPM
// when no learner is present. This is what makes adaptive learning actually feed back
// into client-side throttling (audit #5).
func (s *LLMProxyService) effectiveBucketRPM(cfg config.ChannelConfig) int {
	if s.rateLimitLearner == nil || s.rateLimitLearner.repo == nil || cfg.ID == 0 || len(cfg.Models) == 0 {
		return cfg.RPM
	}
	effective := 0
	for _, m := range cfg.Models {
		rl, err := s.rateLimitLearner.repo.GetOrCreate(cfg.ID, m, cfg.RPM)
		if err != nil || rl == nil {
			continue
		}
		s.rateLimitLearner.CachePut(rl) // so GetSafeRPM/Record429 see the seeded row
		safe := rl.LearnedRPMSafe
		if safe <= 0 {
			safe = int(float64(cfg.RPM) * 0.5) // cold-start fallback
		}
		if effective == 0 || safe < effective {
			effective = safe
		}
	}
	if effective == 0 {
		return cfg.RPM
	}
	return effective
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
	tokenID uint,
) (statusCode int, respBody []byte, respHeaders http.Header, err error) {
	modelName := extractModelFromBody(body)

	// Rerank requests (Tier 7) route only to provider_type=="rerank" channels and
	// bypass virtual groups, sticky binding, and chat conversion — they carry no
	// chat or conversation semantics. Reuses tryChannel (circuit + bucket + logging).
	if path == "/v1/rerank" {
		return s.proxyRerank(modelName, method, path, headers, body, callerID, tokenID)
	}

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

	// --- Task type detection (drives task-aware tiered routing in proxyViaGroup) ---
	headerMap := headerToMap(headers)
	taskType := s.taskRouter.DetectTaskType(headerMap, modelName, body, 0)

	// --- Conversation binding (sticky session) ---
	convID := s.convMgr.ImplicitConversationID(headerMap, body)
	allowSwitch := headers.Get("X-Allow-Channel-Switch") == "true"

	if convID != "" && !allowSwitch {
		binding := s.convMgr.Get(convID)
		if binding != nil {
			if err := s.validateBinding(binding); err == nil {
				// Bound channel is healthy — route directly to protect prompt cache
				s.mu.RLock()
				ch := s.channels[binding.ChannelName]
				s.mu.RUnlock()
				if ch != nil {
					rewrittenBody := rewriteModel(body, binding.Model)
					statusCode, respBody, respHeaders, err := s.tryChannel(ch, method, path, headers, rewrittenBody, callerID, tokenID)
					if err == nil && statusCode < 500 && statusCode != 429 {
						ch.Health.RecordSuccess()
						s.convMgr.Touch(convID, 0, 0)
						return statusCode, respBody, respHeaders, nil
					}
				}
				return 503, []byte(`{"error":"bound channel unavailable for conversation ` + convID + `"}`), nil, nil
			}
		}
	}

	// Check if model matches a virtual model group
	if group, ok := modelGroups[modelName]; ok {
		statusCode, respBody, respHeaders, err = s.proxyViaGroup(group, taskKey, method, path, headers, body, callerID, tokenID, taskType)
		if err == nil && statusCode < 500 && statusCode != 429 {
			// Record conversation binding on success
			if convID != "" {
				// Find which channel was actually used (from sticky binding)
				if taskKey != "" && group.Sticky != nil {
					if stickyCh, stickyModel := group.GetStickyBinding(taskKey); stickyCh != nil {
						s.convMgr.Set(convID, stickyCh.Config.ID, stickyCh.Config.Name, stickyModel, string(taskType))
					}
				}
			}
		}
		return statusCode, respBody, respHeaders, err
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
		statusCode, respBody, respHeaders, err = s.tryChannel(ch, method, path, headers, body, callerID, tokenID)
		if err == nil && statusCode < 500 && statusCode != 429 {
			ch.Health.RecordSuccess()
			if convID != "" {
				s.convMgr.Set(convID, ch.Config.ID, ch.Config.Name, modelName, string(taskType))
			}
			return statusCode, respBody, respHeaders, nil
		}
		result := s.recordChannelFailure(ch, statusCode, respBody, err)
		lastErr = err
		middleware.GetLogger().Warn("channel returned error, trying next",
			zap.String("channel", ch.Config.Name), zap.Int("status", statusCode),
			zap.String("model", modelName), zap.String("class", string(result.Class)))
	}

	return statusCode, respBody, respHeaders, lastErr
}

// recordChannelFailure classifies an upstream failure, applies the semantic breakdown
// to the channel's circuit breaker, and emits the matching aggregated alert (plus a
// circuit-open alert if the breaker tripped). It returns the classification so callers
// can branch on the class / log it. Shared by direct-match routing and rerank routing.
func (s *LLMProxyService) recordChannelFailure(ch *Channel, statusCode int, respBody []byte, err error) llmerrors.Result {
	errBody := ""
	if statusCode != 0 {
		errBody = string(respBody)
	}
	result := llmerrors.Classify(statusCode, errBody, ch.Config.ProviderType)
	duration := llmerrors.BreakdownDuration(result.BreakdownUntil)
	ch.Health.RecordClassifiedFailure(classifyError(statusCode, err), string(result.Class), duration)
	s.alertForClass(ch.Config.Name, result.Class)
	if !ch.Health.IsAvailable() {
		s.recordAlert("circuit_open", "warning", ch.Config.Name,
			fmt.Sprintf("channel %s circuit opened (%s)", ch.Config.Name, result.Class))
	}
	return result
}

// proxyRerank routes a /v1/rerank request to a channel whose provider_type=="rerank"
// (Tier 7). It reuses tryChannel's circuit-breaker + token-bucket + logging path but
// skips chat conversion, virtual groups, and sticky conversation binding — rerank has
// no conversation semantics. Body is forwarded/returned verbatim (Cohere/Jina schema).
func (s *LLMProxyService) proxyRerank(
	modelName, method, path string,
	headers http.Header,
	body []byte,
	callerID string,
	tokenID uint,
) (statusCode int, respBody []byte, respHeaders http.Header, err error) {
	s.mu.RLock()
	modelMap := s.modelMap
	s.mu.RUnlock()

	candidates := findChannelsInMap(modelMap, modelName)
	rerank := make([]*Channel, 0, len(candidates))
	for _, ch := range candidates {
		if ch.Config.ProviderType == "rerank" {
			rerank = append(rerank, ch)
		}
	}
	if len(rerank) == 0 {
		errMsg := fmt.Sprintf("no rerank channel available for model: %s", modelName)
		return 400, []byte(`{"error":"` + errMsg + `"}`), nil, fmt.Errorf("%s", errMsg)
	}
	healthy := s.filterHealthy(rerank)
	if len(healthy) == 0 {
		return 503, []byte(`{"error":"all rerank channels circuit-broken for model: ` + modelName + `"}`), nil, nil
	}

	var lastErr error
	for _, ch := range healthy {
		statusCode, respBody, respHeaders, err = s.tryChannel(ch, method, path, headers, body, callerID, tokenID)
		if err == nil && statusCode < 500 && statusCode != 429 {
			ch.Health.RecordSuccess()
			return statusCode, respBody, respHeaders, nil
		}
		result := s.recordChannelFailure(ch, statusCode, respBody, err)
		lastErr = err
		middleware.GetLogger().Warn("rerank channel returned error, trying next",
			zap.String("channel", ch.Config.Name), zap.Int("status", statusCode),
			zap.String("model", modelName), zap.String("class", string(result.Class)))
	}
	return statusCode, respBody, respHeaders, lastErr
}

// codingPref resolves the coding routing preference for tier ordering. For the
// complexity_aware strategy it derives simple/medium/complex from the prompt;
// otherwise it returns the configured free_first / quality_first directly.
func (s *LLMProxyService) codingPref(body []byte) string {
	if s.taskRouter == nil {
		return "complexity_aware"
	}
	strategy := s.taskRouter.GetCodingStrategy()
	if strategy == "complexity_aware" {
		return string(s.taskRouter.DetectComplexity(body, 0))
	}
	return strategy
}

// balanceSnapshot returns a channel→remaining-USD map from the balance manager,
// used by the balance_aware routing strategy. Channels with fetch errors are omitted.
func (s *LLMProxyService) balanceSnapshot() map[string]float64 {
	if s.balanceMgr == nil {
		return nil
	}
	all := s.balanceMgr.GetAll()
	if len(all) == 0 {
		return nil
	}
	m := make(map[string]float64, len(all))
	for name, info := range all {
		if info != nil && info.Error == "" {
			m[name] = info.Balance
		}
	}
	return m
}

// proxyViaGroup routes a request through a virtual model group with sticky binding.
func (s *LLMProxyService) proxyViaGroup(
	group *ModelGroup,
	taskKey, method, path string,
	headers http.Header,
	body []byte,
	callerID string,
	tokenID uint,
	taskType TaskType,
) (int, []byte, http.Header, error) {
	modelName := extractModelFromBody(body) // original virtual model name for logging
	maxAttempts := len(group.Members)
	tried := map[string]bool{}

	// Resolve coding sub-strategy + balance snapshot once (task-aware tiered routing).
	codingPref := ""
	if taskType == TaskCoding {
		codingPref = s.codingPref(body)
	}
	balances := s.balanceSnapshot()

	for attempt := 0; attempt < maxAttempts; attempt++ {
		ch, realModel := group.SelectChannel(taskKey, taskType, codingPref, balances, tried)
		if ch == nil {
			break
		}
		tried[ch.Config.Name] = true

		// Rewrite the model name in request body to the real model name
		rewrittenBody := rewriteModel(body, realModel)

		statusCode, respBody, respHeaders, err := s.tryChannel(
			ch, method, path, headers, rewrittenBody, callerID, tokenID,
		)

		// Log with original virtual model name for traceability
		s.logGroupRequest(ch.Config.Name, modelName, realModel, path, statusCode, err, callerID, tokenID)

		if err == nil && statusCode < 500 && statusCode != 429 {
			ch.Health.RecordSuccess()
			if taskKey != "" && group.Sticky != nil {
				group.Sticky.Renew(taskKey, group.Config.StickyTTLSeconds)
			}
			return statusCode, respBody, respHeaders, nil
		}

		// Failure: classify error, update health with semantic breakdown, clear sticky binding, try next
		errBody := ""
		if statusCode != 0 {
			errBody = string(respBody)
		}
		result := llmerrors.Classify(statusCode, errBody, ch.Config.ProviderType)
		duration := llmerrors.BreakdownDuration(result.BreakdownUntil)
		ch.Health.RecordClassifiedFailure(classifyError(statusCode, err), string(result.Class), duration)
		s.alertForClass(ch.Config.Name, result.Class)
		if !ch.Health.IsAvailable() {
			s.recordAlert("circuit_open", "warning", ch.Config.Name,
				fmt.Sprintf("channel %s circuit opened (%s)", ch.Config.Name, result.Class))
		}
		if taskKey != "" && group.Sticky != nil {
			group.Sticky.Remove(taskKey)
		}
		middleware.GetLogger().Warn("group channel failed, trying next member",
			zap.String("group", group.Config.Name), zap.String("channel", ch.Config.Name),
			zap.String("model", realModel), zap.Int("status", statusCode),
			zap.String("class", string(result.Class)))
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
	tokenID uint,
) (int, []byte, http.Header, error) {
	maxRetries := s.cfg.MaxRetries
	modelName := extractModelFromBody(body)
	var lastStatusCode int
	var lastBody []byte
	var lastHeaders http.Header

	isAnthropic := ch.Config.ProviderType == "anthropic"

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Client-side rate limiting
		allowed, waitTime := ch.Bucket.TryAcquire()
		if !allowed {
			if attempt < maxRetries {
				maxWait := time.Duration(s.cfg.MaxWaitSeconds) * time.Second
				if waitTime > maxWait {
					waitTime = maxWait
				}
				middleware.GetLogger().Warn("bucket throttle, waiting",
					zap.String("channel", ch.Config.Name), zap.Duration("wait", waitTime),
					zap.Int("attempt", attempt+1), zap.Int("max_retries", maxRetries))
				time.Sleep(waitTime)
				continue
			}
			s.logRequest(ch.Config.Name, modelName, path, 429, true, attempt, 0, 0, 0, 0,
				"client rate limit exhausted", callerID, tokenID)
			return 429, []byte(`{"error":"rate limit: bucket exhausted after retries"}`), nil, nil
		}

		// Prepare request body and path for provider-specific conversion
		forwardBody := body
		forwardPath := path
		isGemini := ch.Config.ProviderType == "gemini"
		realModel := modelName

		if isAnthropic {
			converted, err := ConvertOpenAIToAnthropic(body)
			if err != nil {
				return 400, []byte(`{"error":"anthropic request conversion failed"}`), nil, fmt.Errorf("anthropic conversion: %w", err)
			}
			forwardBody = converted
			if path == "/v1/chat/completions" {
				forwardPath = "/v1/messages"
			}
		} else if isGemini {
			// Parse and strip suffixes, rewrite model
			parsedBody, parsedModel := s.parseModelSuffixes(body)
			realModel = parsedModel
			converted, err := geminiOpenAIToGemini(parsedBody)
			if err != nil {
				return 400, []byte(`{"error":"gemini request conversion failed"}`), nil, fmt.Errorf("gemini conversion: %w", err)
			}
			forwardBody = converted
			if path == "/v1/chat/completions" {
				forwardPath = "/v1beta/models/" + realModel + ":generateContent"
			}
		} else {
			// Strip suffixes for non-gemini providers too (reasoning effort, thinking)
			parsedBody, parsedModel := s.parseModelSuffixes(body)
			realModel = parsedModel
			forwardBody = parsedBody
		}

		// Forward request upstream
		start := time.Now()
		targetURL := strings.TrimRight(ch.Config.BaseURL, "/") + forwardPath
		req, err := http.NewRequest(method, targetURL, bytes.NewReader(forwardBody))
		if err != nil {
			return 0, nil, nil, fmt.Errorf("create request: %w", err)
		}

		// Copy caller headers, override auth
		for k, vv := range headers {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
		if isAnthropic {
			req.Header.Set("x-api-key", ch.Config.APIKey)
			req.Header.Set("anthropic-version", anthropicVersion)
			req.Header.Del("Authorization")
		} else if isGemini {
			req.Header.Set("Authorization", "Bearer "+ch.Config.APIKey)
			req.Header.Set("Content-Type", "application/json")
		} else {
			req.Header.Set("Authorization", "Bearer "+ch.Config.APIKey)
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := ch.Client.Do(req)
		durationMs := int(time.Since(start).Milliseconds())

		if err != nil {
			s.logRequest(ch.Config.Name, realModel, path, 0, false, attempt, durationMs, 0, 0, 0,
				err.Error(), callerID, tokenID)
			return 0, nil, nil, fmt.Errorf("upstream request: %w", err)
		}

		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Extract token usage. For Anthropic, read it from the RAW body BEFORE
		// conversion — the OpenAI conversion drops cache_read_input_tokens, which
		// would make Claude prompt-cache discounts invisible to billing.
		var promptTokens, compTokens, cachedTokens int
		if isAnthropic {
			promptTokens, compTokens, cachedTokens = extractTokenUsage(respBytes)
		}

		// Convert provider response back to OpenAI format
		if isAnthropic && resp.StatusCode < 500 {
			converted, err := ConvertAnthropicToOpenAI(respBytes)
			if err != nil {
				middleware.GetLogger().Warn("anthropic response conversion failed",
					zap.String("channel", ch.Config.Name), zap.Error(err))
			} else {
				respBytes = converted
			}
		} else if isGemini && resp.StatusCode < 500 {
			converted, err := geminiGeminiToOpenAI(respBytes, modelName)
			if err != nil {
				middleware.GetLogger().Warn("gemini response conversion failed",
					zap.String("channel", ch.Config.Name), zap.Error(err))
			} else {
				respBytes = converted
			}
		}

		lastStatusCode = resp.StatusCode
		lastBody = respBytes
		lastHeaders = resp.Header

		isRateLimit := resp.StatusCode == 429
		// Non-Anthropic providers: extract from the (now OpenAI-shaped) body.
		if !isAnthropic {
			promptTokens, compTokens, cachedTokens = extractTokenUsage(respBytes)
		}

		s.logRequest(ch.Config.Name, modelName, path, resp.StatusCode, isRateLimit,
			attempt, durationMs, promptTokens, compTokens, cachedTokens, "", callerID, tokenID)

		// Record latency for routing strategy scoring
		if resp.StatusCode < 500 && resp.StatusCode != 429 {
			ch.RecordLatency(durationMs)
		}

		if resp.StatusCode == 429 {
			// Adaptive rate-limit learning (stage 3): an upstream 429 means our
			// configured RPM is too high. Record it so the learner downgrades the
			// learned-safe RPM to ~85% of the configured rate. Use realModel (the
			// rewritten upstream model), which is what the learner keys on.
			if s.rateLimitLearner != nil && ch.Config.ID != 0 {
				s.rateLimitLearner.Record429(ch.Config.ID, realModel, ch.Config.RPM)
			}
			if attempt < maxRetries {
				backoff := s.calculateBackoff(attempt)
				middleware.GetLogger().Warn("upstream 429, backing off",
					zap.String("channel", ch.Config.Name), zap.Duration("backoff", backoff),
					zap.Int("attempt", attempt+1), zap.Int("max_retries", maxRetries))
				time.Sleep(backoff)
				continue
			}
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
	if err := json.Unmarshal(body, &req); err != nil {
		middleware.GetLogger().Warn("failed to extract model from body", zap.Error(err))
		return ""
	}
	return req.Model
}

func extractTokenUsage(body []byte) (prompt, comp, cached int) {
	// Try OpenAI format first
	var openaiResp struct {
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			CachedTokens        int `json:"cached_tokens,omitempty"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details,omitempty"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &openaiResp); err == nil && openaiResp.Usage.PromptTokens > 0 {
		cached := openaiResp.Usage.CachedTokens
		if cached == 0 {
			cached = openaiResp.Usage.PromptTokensDetails.CachedTokens
		}
		return openaiResp.Usage.PromptTokens, openaiResp.Usage.CompletionTokens, cached
	}

	// Try Anthropic format (raw, pre-conversion). input_tokens EXCLUDES cached reads,
	// so total prompt = input + cache_read + cache_creation; cached = cache_read.
	var anthropicResp struct {
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &anthropicResp); err == nil &&
		(anthropicResp.Usage.InputTokens > 0 || anthropicResp.Usage.CacheReadInputTokens > 0) {
		cached := anthropicResp.Usage.CacheReadInputTokens
		prompt := anthropicResp.Usage.InputTokens +
			anthropicResp.Usage.CacheReadInputTokens +
			anthropicResp.Usage.CacheCreationInputTokens
		return prompt, anthropicResp.Usage.OutputTokens, cached
	}

	return 0, 0, 0
}

func (s *LLMProxyService) logRequest(channelName, modelName, path string, statusCode int,
	isRateLimit bool, retryCount, durationMs, promptTokens, compTokens, cachedTokens int,
	errMsg, callerID string, tokenID uint) {
	if s.repo == nil {
		return
	}
	go func() {
		// Price by the real upstream model. Group requests log "virtual→real" for
		// traceability, but pricing must use the real model name.
		pricingModel := modelName
		if i := strings.Index(modelName, "→"); i >= 0 {
			pricingModel = modelName[i+len("→"):]
		}

		// Calculate cost in micro-cents (precise) and cents (rounded display).
		var costMicroCents int64
		var costCents int
		if s.pricer != nil && promptTokens+compTokens > 0 {
			costMicroCents, _ = s.pricer.CalcMicroCents(channelName, pricingModel, Usage{
				PromptTokens:     promptTokens,
				CompletionTokens: compTokens,
				CachedTokens:     cachedTokens,
			})
			costCents = int(MicroCentsToCents(costMicroCents))
		}

		entry := &model.LLMProxyLog{
			ChannelName:    channelName,
			Model:          modelName,
			RequestPath:    path,
			StatusCode:     statusCode,
			IsRateLimit:    isRateLimit,
			RetryCount:     retryCount,
			DurationMs:     durationMs,
			PromptTokens:   promptTokens,
			CompTokens:     compTokens,
			CachedTokens:   cachedTokens,
			CostCents:      costCents,
			CostMicroCents: costMicroCents,
			ErrorMessage:   errMsg,
			CallerID:       callerID,
			CreatedAt:      time.Now(),
		}
		if err := s.repo.CreateLog(entry); err != nil {
			middleware.GetLogger().Warn("failed to log llm proxy request", zap.Error(err))
			return
		}

		// Aggregate token usage by token (daily). Skip the server/admin key (tokenID 0):
		// it has no billable token row and would pollute aggregates with id 0.
		if s.tokenUsageRepo != nil && tokenID != 0 {
			requests := 1
			errCount := 0
			if statusCode >= 400 {
				errCount = 1
				requests = 0 // Don't count errors as successful requests
			}
			if err := s.tokenUsageRepo.AddUsage(tokenID, time.Now().Truncate(24*time.Hour),
				requests, promptTokens, compTokens, cachedTokens, costCents, costMicroCents, errCount); err != nil {
				middleware.GetLogger().Warn("failed to aggregate token usage", zap.Error(err))
			}
		}
	}()
}

// logGroupRequest logs a model group proxy attempt for traceability.
func (s *LLMProxyService) logGroupRequest(channelName, virtualModel, realModel, path string,
	statusCode int, err error, callerID string, tokenID uint) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	isRateLimit := statusCode == 429
	// Log with virtual model name so operators can trace group requests
	s.logRequest(channelName, virtualModel+"→"+realModel, path, statusCode, isRateLimit,
		0, 0, 0, 0, 0, errMsg, callerID, tokenID)
}

// --- Streaming Proxy ---

// StreamResult holds the result of a streaming proxy request.
type StreamResult struct {
	StatusCode   int
	RespHeaders  http.Header
	BodyReader   io.ReadCloser
	ProviderType string
	ChannelName  string
	ModelName    string
}

// IsStreamRequest checks if the request body has stream:true.
func (s *LLMProxyService) IsStreamRequest(body []byte) bool {
	var req struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	return req.Stream
}

// ProxyStreamRequest forwards an OpenAI-compatible streaming request.
// Returns a StreamResult with an open BodyReader that the caller must close.
func (s *LLMProxyService) ProxyStreamRequest(
	method, path string,
	headers http.Header,
	body []byte,
	callerID string,
	tokenID uint,
) (*StreamResult, error) {
	modelName := extractModelFromBody(body)

	s.mu.RLock()
	modelGroups := s.modelGroups
	modelMap := s.modelMap
	s.mu.RUnlock()

	taskKey := headers.Get("X-Task-Key")
	if taskKey == "" && callerID != "" && callerID != "unknown" {
		taskKey = callerID + ":" + modelName
	}

	// --- Conversation binding (sticky session) ---
	headerMap := headerToMap(headers)
	taskType := s.taskRouter.DetectTaskType(headerMap, modelName, body, 0)
	convID := s.convMgr.ImplicitConversationID(headerMap, body)
	allowSwitch := headers.Get("X-Allow-Channel-Switch") == "true"

	if convID != "" && !allowSwitch {
		binding := s.convMgr.Get(convID)
		if binding != nil {
			if err := s.validateBinding(binding); err == nil {
				s.mu.RLock()
				ch := s.channels[binding.ChannelName]
				s.mu.RUnlock()
				if ch != nil {
					rewrittenBody := rewriteModel(body, binding.Model)
					result, err := s.tryChannelStream(ch, method, path, headers, rewrittenBody, callerID)
					if err == nil {
						result.ModelName = binding.Model
						s.logRequest(ch.Config.Name, binding.Model, path, result.StatusCode, false,
							0, 0, 0, 0, 0, "", callerID, tokenID)
						s.convMgr.Touch(convID, 0, 0)
					}
					return result, err
				}
			}
			return nil, fmt.Errorf("bound channel unavailable for conversation %s", convID)
		}
	}

	// Check virtual model group
	if group, ok := modelGroups[modelName]; ok {
		codingPref := ""
		if taskType == TaskCoding {
			codingPref = s.codingPref(body)
		}
		ch, realModel := group.SelectChannel(taskKey, taskType, codingPref, s.balanceSnapshot(), nil)
		if ch == nil {
			return nil, fmt.Errorf("no available channel in group %s", modelName)
		}
		rewrittenBody := rewriteModel(body, realModel)
		result, err := s.tryChannelStream(ch, method, path, headers, rewrittenBody, callerID)
		if err == nil {
			result.ModelName = modelName + "→" + realModel
			// Log stream start
			s.logRequest(ch.Config.Name, modelName+"→"+realModel, path, result.StatusCode, false,
				0, 0, 0, 0, 0, "", callerID, tokenID)
			if convID != "" {
				if taskKey != "" && group.Sticky != nil {
					if stickyCh, stickyModel := group.GetStickyBinding(taskKey); stickyCh != nil {
						s.convMgr.Set(convID, stickyCh.Config.ID, stickyCh.Config.Name, stickyModel, "")
					}
				}
			}
		}
		return result, err
	}

	// Direct channel matching
	channels := findChannelsInMap(modelMap, modelName)
	if len(channels) == 0 {
		return nil, fmt.Errorf("no channel available for model: %s", modelName)
	}

	healthy := s.filterHealthy(channels)
	if len(healthy) == 0 {
		return nil, fmt.Errorf("all channels circuit-broken for model: %s", modelName)
	}

	// Try first healthy channel (no retry for streaming)
	ch := healthy[0]
	result, err := s.tryChannelStream(ch, method, path, headers, body, callerID)
	if err == nil {
		result.ModelName = modelName
		s.logRequest(ch.Config.Name, modelName, path, result.StatusCode, false,
			0, 0, 0, 0, 0, "", callerID, tokenID)
		if convID != "" {
			s.convMgr.Set(convID, ch.Config.ID, ch.Config.Name, modelName, "")
		}
	}
	return result, err
}

// tryChannelStream sends a streaming request to a single channel.
// No retries — once a stream starts, you can't retry.
func (s *LLMProxyService) tryChannelStream(
	ch *Channel,
	method, path string,
	headers http.Header,
	body []byte,
	callerID string,
) (*StreamResult, error) {
	// Single rate limit check (no retry loop)
	allowed, waitTime := ch.Bucket.TryAcquire()
	if !allowed {
		maxWait := time.Duration(s.cfg.MaxWaitSeconds) * time.Second
		if waitTime > maxWait {
			waitTime = maxWait
		}
		// For streaming, we can wait once but not retry
		if waitTime <= maxWait {
			middleware.GetLogger().Warn("stream bucket throttle, waiting once",
				zap.String("channel", ch.Config.Name), zap.Duration("wait", waitTime))
			time.Sleep(waitTime)
		}
	}

	isAnthropic := ch.Config.ProviderType == "anthropic"

	forwardBody := body
	forwardPath := path
	if isAnthropic {
		converted, err := ConvertOpenAIToAnthropic(body)
		if err != nil {
			return nil, fmt.Errorf("anthropic stream conversion: %w", err)
		}
		forwardBody = converted
		if path == "/v1/chat/completions" {
			forwardPath = "/v1/messages"
		}
	}

	targetURL := strings.TrimRight(ch.Config.BaseURL, "/") + forwardPath
	req, err := http.NewRequest(method, targetURL, bytes.NewReader(forwardBody))
	if err != nil {
		return nil, fmt.Errorf("create stream request: %w", err)
	}

	for k, vv := range headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	if isAnthropic {
		req.Header.Set("x-api-key", ch.Config.APIKey)
		req.Header.Set("anthropic-version", anthropicVersion)
		req.Header.Del("Authorization")
	} else {
		req.Header.Set("Authorization", "Bearer "+ch.Config.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use the underlying http.Client without timeout for streaming
	// (the httpclient.Client has an overall Timeout which kills long streams)
	streamClient := ch.Client.UnderlyingClient()
	// Temporarily remove timeout by creating a request-specific context
	// Actually, the underlying client.Timeout is the issue — we need a client without it
	noTimeoutClient := &http.Client{
		Transport: streamClient.Transport,
		Timeout:   0,
	}

	resp, err := noTimeoutClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream stream request: %w", err)
	}

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Convert Anthropic error response
		if isAnthropic {
			converted, convErr := ConvertAnthropicErrorToOpenAI(respBytes)
			if convErr == nil {
				respBytes = converted
			}
		}

		return &StreamResult{
			StatusCode:   resp.StatusCode,
			BodyReader:   io.NopCloser(bytes.NewReader(respBytes)),
			RespHeaders:  resp.Header,
			ProviderType: ch.Config.ProviderType,
			ChannelName:  ch.Config.Name,
		}, nil
	}

	ch.Health.RecordSuccess()

	return &StreamResult{
		StatusCode:   resp.StatusCode,
		BodyReader:   resp.Body,
		RespHeaders:  resp.Header,
		ProviderType: ch.Config.ProviderType,
		ChannelName:  ch.Config.Name,
	}, nil
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

// --- Conversations ---

// GetConversations returns all active conversation bindings.
func (s *LLMProxyService) GetConversations() []*model.LLMConversationBinding {
	if s.convMgr == nil {
		return nil
	}
	return s.convMgr.List()
}

// DeleteConversation removes a conversation binding.
func (s *LLMProxyService) DeleteConversation(convID string) error {
	if s.convMgr == nil {
		return fmt.Errorf("conversation manager not initialized")
	}
	s.convMgr.Remove(convID)
	if s.convMgr.repo != nil {
		return s.convMgr.repo.Delete(convID)
	}
	return nil
}

// --- Rate Limits ---

// GetRateLimits returns the adaptive rate-limit learning state for all channel×model pairs.
func (s *LLMProxyService) GetRateLimits() ([]model.LLMModelRateLimit, error) {
	if s.rateLimitLearner == nil || s.rateLimitLearner.repo == nil {
		return nil, nil
	}
	return s.rateLimitLearner.repo.ListAll()
}

// ResetRateLimit resets learned rate limit for a channel×model to cold-start defaults.
func (s *LLMProxyService) ResetRateLimit(channelID uint, model string) error {
	if s.rateLimitLearner == nil || s.rateLimitLearner.repo == nil {
		return fmt.Errorf("rate limit learner not initialized")
	}
	rl, err := s.rateLimitLearner.repo.GetOrCreate(channelID, model, 0)
	if err != nil {
		return err
	}
	// Reset to configured * 0.5
	rl.LearnedRPMSafe = int(float64(rl.ConfiguredRPM) * 0.5)
	rl.LearnedRPDSafe = 0
	rl.ConfidenceScore = 0.1
	rl.LastAdjustAt = nil
	rl.Last429At = nil
	rl.Last429ObservedRPM = 0
	rl.AdjustmentLog = ""
	return s.rateLimitLearner.repo.Update(rl)
}

// LockRateLimit toggles the locked state for a rate limit record.
func (s *LLMProxyService) LockRateLimit(id uint, locked bool) error {
	if s.rateLimitLearner == nil || s.rateLimitLearner.repo == nil {
		return fmt.Errorf("rate limit learner not initialized")
	}
	return s.rateLimitLearner.repo.SetLocked(id, locked)
}

// --- Task Router ---

// GetCodingStrategy returns the current coding routing strategy.
func (s *LLMProxyService) GetCodingStrategy() string {
	if s.taskRouter == nil {
		return "complexity_aware"
	}
	return s.taskRouter.GetCodingStrategy()
}

// SetCodingStrategy updates the coding routing strategy.
func (s *LLMProxyService) SetCodingStrategy(strategy string) {
	if s.taskRouter == nil {
		return
	}
	s.taskRouter.SetCodingStrategy(strategy)
}

// --- Usage / Billing ---

// GetUsageAggregates returns token usage aggregated by the requested dimension.
func (s *LLMProxyService) GetUsageAggregates(groupBy string, from, to time.Time) ([]map[string]interface{}, error) {
	// model dimension has no column in llm_token_usage_daily; aggregate from proxy logs.
	if groupBy == "model" {
		if s.repo == nil {
			return nil, fmt.Errorf("proxy repo not initialized")
		}
		return s.repo.AggregateByModel(from, to)
	}
	if s.tokenUsageRepo == nil {
		return nil, fmt.Errorf("token usage repo not initialized")
	}
	return s.tokenUsageRepo.Aggregate(groupBy, from, to)
}

// parseModelSuffixes handles reasoning effort (-high/-medium/-low) and thinking mode (-thinking-N) suffixes.
// Returns modified request body and the real model name to send upstream.
func (s *LLMProxyService) parseModelSuffixes(body []byte) ([]byte, string) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return body, ""
	}
	originalModel := req.Model
	realModel := originalModel

	// Reasoning effort suffixes: o4-mini-high → o4-mini + reasoning.effort=high
	if strings.HasSuffix(originalModel, "-high") {
		realModel = strings.TrimSuffix(originalModel, "-high")
		body = injectField(body, "reasoning", map[string]string{"effort": "high"})
	} else if strings.HasSuffix(originalModel, "-medium") {
		realModel = strings.TrimSuffix(originalModel, "-medium")
		body = injectField(body, "reasoning", map[string]string{"effort": "medium"})
	} else if strings.HasSuffix(originalModel, "-low") {
		realModel = strings.TrimSuffix(originalModel, "-low")
		body = injectField(body, "reasoning", map[string]string{"effort": "low"})
	}

	// Thinking mode suffix: claude-sonnet-thinking-2048 → thinking.budget_tokens=2048
	if idx := strings.LastIndex(realModel, "-thinking-"); idx > 0 {
		budgetStr := realModel[idx+len("-thinking-"):]
		if budget, err := strconv.Atoi(budgetStr); err == nil {
			realModel = realModel[:idx]
			body = injectField(body, "thinking", map[string]int{"budget_tokens": budget})
		}
	}

	// Replace model name in body
	if realModel != originalModel {
		body = replaceModelInBody(body, realModel)
	}
	return body, realModel
}

func injectField(body []byte, key string, value interface{}) []byte {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	m[key] = value
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

func replaceModelInBody(body []byte, model string) []byte {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	m["model"] = model
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

// Alias for converter package functions to avoid import cycle.
func geminiOpenAIToGemini(body []byte) ([]byte, error) {
	return converter.OpenAIToGemini(body)
}

func geminiGeminiToOpenAI(body []byte, model string) ([]byte, error) {
	return converter.GeminiToOpenAI(body, model)
}

// startLogArchival starts a background goroutine that archives old LLM proxy logs daily.
func (s *LLMProxyService) startLogArchival(stopCh <-chan struct{}) {
	retentionDays := s.cfg.LogRetentionDays
	if retentionDays <= 0 {
		retentionDays = 30
	}

	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		// Run once at startup (after a short delay)
		select {
		case <-time.After(5 * time.Minute):
			s.archiveLogs(retentionDays)
		case <-stopCh:
			return
		}

		for {
			select {
			case <-ticker.C:
				s.archiveLogs(retentionDays)
			case <-stopCh:
				return
			}
		}
	}()
}

func (s *LLMProxyService) archiveLogs(retentionDays int) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	month := cutoff.Format("2006-01")

	archiveDir := "/mnt/knowledge/logs-archive/llm"
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		middleware.GetLogger().Warn("failed to create archive dir", zap.String("dir", archiveDir), zap.Error(err))
		return
	}

	archivePath := filepath.Join(archiveDir, month+".jsonl.gz")
	file, err := os.OpenFile(archivePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		middleware.GetLogger().Warn("failed to open archive file", zap.String("path", archivePath), zap.Error(err))
		return
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	// Query old logs
	logs, err := s.repo.GetLogsBefore(cutoff)
	if err != nil {
		middleware.GetLogger().Warn("failed to query old logs", zap.Error(err))
		return
	}
	if len(logs) == 0 {
		return
	}

	encoder := json.NewEncoder(gzWriter)
	for _, log := range logs {
		if err := encoder.Encode(log); err != nil {
			middleware.GetLogger().Warn("failed to encode log", zap.Error(err))
		}
	}

	// Delete archived logs
	if err := s.repo.DeleteLogsBefore(cutoff); err != nil {
		middleware.GetLogger().Warn("failed to delete old logs", zap.Error(err))
		return
	}

	middleware.GetLogger().Info("archived LLM proxy logs",
		zap.Int("count", len(logs)),
		zap.String("archive", archivePath),
		zap.Time("cutoff", cutoff))
}
