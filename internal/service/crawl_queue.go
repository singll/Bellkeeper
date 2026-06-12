package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

// CrawlQueueService manages the persistent crawl queue with worker pools.
type CrawlQueueService struct {
	cfg          config.CrawlQueueConfig
	repo         *repository.CrawlJobRepository
	domainRepo   *repository.CrawlDomainProfileRepository
	extractor    *ExtractorService
	ingestion    *FileIngestionService
	activityLog  *ActivityLogService
	notification *NotificationService

	// Worker management
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Per-channel circuit breakers
	breakers map[string]*circuitBreaker

	// In-memory blocked domain set (auto-learned + config)
	blockedDomains map[string]bool
	domainMu       sync.RWMutex
}

// circuitBreaker implements per-channel back-pressure.
type circuitBreaker struct {
	mu              sync.Mutex
	consecutiveFail int
	threshold       int
	cooldown        time.Duration
	halfOpenMax     int
	halfOpenCount   int
	openedAt        time.Time
	state           string // "closed", "open", "half-open"
}

func newCircuitBreaker(threshold int, cooldown time.Duration) *circuitBreaker {
	return &circuitBreaker{
		threshold: threshold,
		cooldown:  cooldown,
		state:     "closed",
	}
}

func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case "closed":
		return true
	case "open":
		if time.Since(cb.openedAt) > cb.cooldown {
			cb.state = "half-open"
			cb.halfOpenCount = 0
			return true
		}
		return false
	case "half-open":
		return cb.halfOpenCount < cb.halfOpenMax
	default:
		return true
	}
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFail = 0
	cb.state = "closed"
	cb.halfOpenCount = 0
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFail++
	cb.halfOpenCount++

	if cb.state == "half-open" {
		cb.trip()
		return
	}
	if cb.consecutiveFail >= cb.threshold {
		cb.trip()
	}
}

func (cb *circuitBreaker) trip() {
	cb.state = "open"
	cb.openedAt = time.Now()
	log.Printf("[CrawlQueue] circuit breaker tripped, cooldown=%s", cb.cooldown)
}

func (cb *circuitBreaker) status() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// NewCrawlQueueService creates a new crawl queue service.
func NewCrawlQueueService(
	cfg config.CrawlQueueConfig,
	repo *repository.CrawlJobRepository,
	domainRepo *repository.CrawlDomainProfileRepository,
	extractor *ExtractorService,
	ingestion *FileIngestionService,
	activityLog *ActivityLogService,
) *CrawlQueueService {
	svc := &CrawlQueueService{
		cfg:            cfg,
		repo:           repo,
		domainRepo:     domainRepo,
		extractor:      extractor,
		ingestion:      ingestion,
		activityLog:    activityLog,
		blockedDomains: make(map[string]bool),
		breakers:       make(map[string]*circuitBreaker),
	}

	// Seed blocked domains from config
	for _, d := range cfg.BlockedDomains {
		svc.blockedDomains[d] = true
	}

	// Create circuit breakers per channel
	cbCooldown := 2 * time.Minute
	svc.breakers["firecrawl"] = newCircuitBreaker(5, cbCooldown)
	svc.breakers["trafilatura"] = newCircuitBreaker(5, cbCooldown)
	svc.breakers["auto"] = newCircuitBreaker(5, cbCooldown)

	return svc
}

// SetNotificationService injects the notification service (set later by app wiring).
func (s *CrawlQueueService) SetNotificationService(svc *NotificationService) {
	s.notification = svc
}

// Enqueue adds a URL to the crawl queue.
func (s *CrawlQueueService) Enqueue(sourceID uint, rawURL, title, channelType string, metadata map[string]interface{}) (uint, error) {
	domain := crawlExtractDomain(rawURL)

	// Layer 1: domain blacklist check
	if s.isBlockedDomain(domain) {
		job := &model.CrawlJob{
			SourceID:     sourceID,
			URL:          rawURL,
			Title:        title,
			Status:       model.CrawlJobBlocked,
			ChannelType:  channelType,
			SourceDomain: domain,
			BlockReason:  "domain_blacklist",
		}
		if metadata != nil {
			b, _ := json.Marshal(metadata)
			job.Metadata = b
		}
		if err := s.repo.Enqueue(job); err != nil {
			return 0, err
		}
		s.logActivity("job_blocked", "blocked", fmt.Sprintf("URL blocked (domain blacklist): %s domain=%s", rawURL, domain), sourceID)
		s.notifyBlocked(rawURL, domain, "domain_blacklist")
		return job.ID, nil
	}

	status := model.CrawlJobPending
	if channelType == "" {
		channelType = "auto"
	}

	job := &model.CrawlJob{
		SourceID:     sourceID,
		URL:          rawURL,
		Title:        title,
		Status:       status,
		ChannelType:  channelType,
		SourceDomain: domain,
		MaxRetries:   s.cfg.MaxRetries,
	}
	if metadata != nil {
		b, _ := json.Marshal(metadata)
		job.Metadata = b
	}

	if err := s.repo.Enqueue(job); err != nil {
		return 0, err
	}

	s.logActivity("job_enqueued", "pending", fmt.Sprintf("Enqueued: %s channel=%s", rawURL, channelType), sourceID)
	return job.ID, nil
}

// Start launches the worker pool.
func (s *CrawlQueueService) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	// Recover orphaned jobs from previous crash
	if err := s.repo.RecoverOrphanedJobs(); err != nil {
		log.Printf("[CrawlQueue] failed to recover orphaned jobs: %v", err)
	}

	// Rebuild blocked domain list from DB
	s.rebuildBlockedDomains()

	// Launch periodic stale job recovery
	s.wg.Add(1)
	go s.staleJobRecoveryLoop(ctx)

	// Launch workers
	s.startWorkers(ctx, "firecrawl", s.cfg.FirecrawlWorkers)
	s.startWorkers(ctx, "trafilatura", s.cfg.TrafilaturaWorkers)
	s.startWorkers(ctx, "auto", s.cfg.AutoWorkers)

	log.Printf("[CrawlQueue] started: firecrawl=%d trafilatura=%d auto=%d poll=%ds",
		s.cfg.FirecrawlWorkers, s.cfg.TrafilaturaWorkers, s.cfg.AutoWorkers, s.cfg.PollInterval)
}

// Stop gracefully stops all workers.
func (s *CrawlQueueService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	log.Printf("[CrawlQueue] all workers stopped")
}

// staleJobRecoveryLoop periodically recovers jobs stuck in "running" state.
func (s *CrawlQueueService) staleJobRecoveryLoop(ctx context.Context) {
	defer s.wg.Done()

	interval := time.Duration(s.cfg.RecoveryIntervalMinutes) * time.Minute
	staleTimeout := time.Duration(s.cfg.StaleTimeoutMinutes) * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recovered, err := s.repo.RecoverStaleRunningJobs(staleTimeout)
			if err != nil {
				log.Printf("[CrawlQueue] stale job recovery error: %v", err)
				continue
			}
			if recovered > 0 {
				log.Printf("[CrawlQueue] recovered %d stale running jobs (stale>%s)", recovered, staleTimeout)
			}
		}
	}
}

// startWorkers launches n workers for a given channel type.
func (s *CrawlQueueService) startWorkers(ctx context.Context, channelType string, n int) {
	for i := 0; i < n; i++ {
		s.wg.Add(1)
		go func(id int) {
			defer s.wg.Done()
			s.workerLoop(ctx, channelType, id)
		}(i)
	}
}

// workerLoop is the main loop for each worker goroutine.
func (s *CrawlQueueService) workerLoop(ctx context.Context, channelType string, id int) {
	ticker := time.NewTicker(time.Duration(s.cfg.PollInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		// Check circuit breaker
		cb := s.breakers[channelType]
		if cb != nil && !cb.allow() {
			continue
		}

		job, err := s.repo.Dequeue(channelType)
		if err != nil {
			log.Printf("[CrawlQueue:%s:%d] dequeue error: %v", channelType, id, err)
			continue
		}
		if job == nil {
			continue
		}

		if retryAt, reason, ok := s.reserveDomainSlot(job); !ok {
			if err := s.repo.DelayJob(job.ID, retryAt, "domain_throttled", reason); err != nil {
				log.Printf("[CrawlQueue:%s:%d] failed to requeue domain-throttled job %d: %v", channelType, id, job.ID, err)
			}
			s.logActivity("job_throttled", "retrying",
				fmt.Sprintf("Domain throttled: %s domain=%s next=%s reason=%s", job.URL, job.SourceDomain, retryAt.Format(time.RFC3339), reason),
				job.SourceID)
			continue
		}

		s.processJob(ctx, job, channelType, cb)
	}
}

type domainThrottleDecision struct {
	Allowed bool
	RetryAt time.Time
	Reason  string
}

func decideDomainThrottle(profile *model.CrawlDomainProfile, runningRank int64, now time.Time) domainThrottleDecision {
	if profile == nil || profile.Domain == "" {
		return domainThrottleDecision{Allowed: true}
	}
	delay := time.Duration(profile.DefaultDelaySeconds) * time.Second
	if delay <= 0 {
		delay = 60 * time.Second
	}
	maxConcurrency := profile.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}

	if profile.NextAllowedAt != nil && profile.NextAllowedAt.After(now) {
		return domainThrottleDecision{
			Allowed: false,
			RetryAt: *profile.NextAllowedAt,
			Reason:  "next_allowed_at",
		}
	}
	if runningRank > int64(maxConcurrency) {
		return domainThrottleDecision{
			Allowed: false,
			RetryAt: now.Add(delay),
			Reason:  "max_concurrency",
		}
	}
	return domainThrottleDecision{Allowed: true}
}

func (s *CrawlQueueService) reserveDomainSlot(job *model.CrawlJob) (time.Time, string, bool) {
	if job == nil || job.SourceDomain == "" || !s.cfg.DomainThrottleEnabled || s.domainRepo == nil {
		return time.Time{}, "", true
	}
	now := time.Now()
	profile, err := s.domainRepo.FindOrCreate(job.SourceDomain, s.domainDefaultDelaySeconds(), s.domainDefaultMaxConcurrency())
	if err != nil {
		log.Printf("[CrawlQueue] domain profile lookup failed for %s: %v", job.SourceDomain, err)
		return time.Time{}, "", true
	}
	runningRank, err := s.repo.CountRunningDomainRank(job.SourceDomain, job.ID, job.StartedAt)
	if err != nil {
		log.Printf("[CrawlQueue] domain running rank failed for %s: %v", job.SourceDomain, err)
		return time.Time{}, "", true
	}

	decision := decideDomainThrottle(profile, runningRank, now)
	if !decision.Allowed {
		return decision.RetryAt, decision.Reason, false
	}

	nextAllowed := now.Add(time.Duration(normalizePositive(profile.DefaultDelaySeconds, s.domainDefaultDelaySeconds())) * time.Second)
	if err := s.domainRepo.RecordStart(job.SourceDomain, nextAllowed); err != nil {
		log.Printf("[CrawlQueue] domain profile start update failed for %s: %v", job.SourceDomain, err)
	}
	return time.Time{}, "", true
}

func (s *CrawlQueueService) domainDefaultDelaySeconds() int {
	return normalizePositive(s.cfg.DomainDefaultDelay, 60)
}

func (s *CrawlQueueService) domainDefaultMaxConcurrency() int {
	return normalizePositive(s.cfg.DomainDefaultConcurrency, 1)
}

func normalizePositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

// processJob executes a single crawl job.
func (s *CrawlQueueService) processJob(ctx context.Context, job *model.CrawlJob, workerChannel string, cb *circuitBreaker) {
	startTime := time.Now()

	// Determine extractor based on channel type
	extractorName := workerChannel
	if workerChannel == "auto" {
		extractorName = "" // let ExtractorService decide (trafilatura first, firecrawl fallback)
	}

	// Extract content
	extractReq := &ExtractionRequest{URL: job.URL}
	result, err := s.extractor.Extract(extractReq)
	durationMs := int(time.Since(startTime).Milliseconds())

	if err != nil {
		s.handleExtractionFailure(job, err, extractorName, cb, durationMs)
		return
	}

	if !result.Success {
		s.handleExtractionFailure(job, fmt.Errorf("%s", result.Error), result.Extractor, cb, durationMs)
		return
	}

	// Success path
	content := result.Content
	extractorUsed := result.Extractor

	// Layer 2: empty content check → possible paywall
	if len(content) < 100 {
		s.handleEmptyContent(job, extractorUsed, cb, durationMs)
		return
	}

	// Layer 3: paywall keyword detection
	if s.detectPaywallKeywords(content) {
		s.repo.MarkBlocked(job.ID, "paywall_keywords")
		s.logActivity("job_blocked", "blocked",
			fmt.Sprintf("Paywall keywords detected: %s extractor=%s", job.URL, extractorUsed), job.SourceID)
		s.notifyBlocked(job.URL, job.SourceDomain, "paywall_keywords")
		s.recordDomainOutcome(job, string(model.CrawlJobBlocked), "paywall_keywords", "paywall keywords detected", nil)
		if cb != nil {
			cb.recordSuccess()
		}
		return
	}

	// Ingest the extracted content
	ingestResult, err := s.ingestion.IngestURL(&IngestURLRequest{
		URL:      job.URL,
		Title:    coalesce(job.Title, result.Title),
		Content:  content,
		Category: "",
		Layer:    "",
	})

	if err != nil {
		s.handleExtractionFailure(job, err, extractorUsed, cb, durationMs)
		return
	}

	// Mark success
	updates := map[string]interface{}{
		"content_length": len(content),
		"extractor_used": extractorUsed,
	}
	if ingestResult.Status == "duplicate" || ingestResult.Status == "duplicate_content" {
		s.repo.UpdateStatus(job.ID, model.CrawlJobSkipped, updates)
		s.logActivity("job_skipped", "skipped",
			fmt.Sprintf("Skipped (duplicate): %s", job.URL), job.SourceID)
		s.recordDomainOutcome(job, string(model.CrawlJobSkipped), "", "duplicate", nil)
	} else {
		s.repo.UpdateStatus(job.ID, model.CrawlJobSuccess, updates)
		s.logActivity("job_success", "success",
			fmt.Sprintf("Success: %s extractor=%s len=%d", job.URL, extractorUsed, len(content)),
			job.SourceID)
		s.recordDomainOutcome(job, string(model.CrawlJobSuccess), "", "success", nil)
	}

	if cb != nil {
		cb.recordSuccess()
	}
}

// handleExtractionFailure classifies the error and applies retry or terminal logic.
func (s *CrawlQueueService) handleExtractionFailure(job *model.CrawlJob, err error, extractor string, cb *circuitBreaker, durationMs int) {
	errType, errMsg := classifyCrawlError(err)
	log.Printf("[CrawlQueue] job %d failed: type=%s err=%s", job.ID, errType, errMsg)

	if cb != nil {
		cb.recordFailure()
	}

	switch errType {
	case "not_found", "client_error":
		s.repo.MarkDead(job.ID, errType, errMsg)
		s.logActivity("job_dead", "dead",
			fmt.Sprintf("Dead (%s): %s err=%s", errType, job.URL, errMsg), job.SourceID)
		s.recordDomainOutcome(job, string(model.CrawlJobDead), errType, errMsg, nil)
	case "forbidden":
		if job.RetryCount >= 1 {
			s.repo.MarkBlocked(job.ID, "forbidden")
			s.logActivity("job_blocked", "blocked",
				fmt.Sprintf("Blocked (forbidden after retry): %s err=%s", job.URL, errMsg), job.SourceID)
			s.notifyBlocked(job.URL, job.SourceDomain, "forbidden")
			s.recordDomainOutcome(job, string(model.CrawlJobBlocked), errType, errMsg, nil)
		} else {
			nextRetry := s.calculateBackoff(job.RetryCount, errType)
			s.repo.MarkRetry(job.ID, nextRetry, errType, errMsg)
			s.logActivity("job_retry", "retrying",
				fmt.Sprintf("Retry (forbidden): %s attempt=%d next=%s err=%s", job.URL, job.RetryCount+1, nextRetry.Format(time.RFC3339), errMsg),
				job.SourceID)
			s.recordDomainOutcome(job, string(model.CrawlJobRetrying), errType, errMsg, &nextRetry)
		}
	case "rate_limited":
		if job.RetryCount >= job.MaxRetries {
			s.repo.MarkDead(job.ID, errType, errMsg)
			s.logActivity("job_dead", "dead",
				fmt.Sprintf("Dead (rate_limited max retries): %s retries=%d err=%s", job.URL, job.RetryCount, errMsg), job.SourceID)
			s.recordDomainOutcome(job, string(model.CrawlJobDead), errType, errMsg, nil)
		} else {
			nextRetry := s.calculateBackoff(job.RetryCount, errType)
			if retryAfter, ok := retryAfterFromError(err, time.Now()); ok && retryAfter.After(time.Now()) {
				nextRetry = retryAfter
			}
			s.repo.MarkRetry(job.ID, nextRetry, errType, errMsg)
			s.logActivity("job_retry", "retrying",
				fmt.Sprintf("Retry (rate_limited): %s attempt=%d next=%s err=%s", job.URL, job.RetryCount+1, nextRetry.Format(time.RFC3339), errMsg),
				job.SourceID)
			s.recordDomainOutcome(job, string(model.CrawlJobRetrying), errType, errMsg, &nextRetry)
		}
	case "paywall":
		s.repo.MarkBlocked(job.ID, "paywall_detected")
		s.logActivity("job_blocked", "blocked",
			fmt.Sprintf("Blocked (paywall): %s", job.URL), job.SourceID)
		s.notifyBlocked(job.URL, job.SourceDomain, "paywall_detected")
		s.autoLearnDomain(job.SourceDomain)
		s.recordDomainOutcome(job, string(model.CrawlJobBlocked), errType, errMsg, nil)
	default:
		// Retryable: timeout, server_error, network, empty_content, unknown
		if job.RetryCount >= job.MaxRetries {
			s.repo.MarkDead(job.ID, errType, errMsg)
			s.logActivity("job_dead", "dead",
				fmt.Sprintf("Dead (max retries): %s retries=%d err=%s", job.URL, job.RetryCount, errMsg), job.SourceID)
			s.recordDomainOutcome(job, string(model.CrawlJobDead), errType, errMsg, nil)
		} else {
			nextRetry := s.calculateBackoff(job.RetryCount, errType)
			s.repo.MarkRetry(job.ID, nextRetry, errType, errMsg)
			s.logActivity("job_retry", "retrying",
				fmt.Sprintf("Retry: %s attempt=%d next=%s err=%s", job.URL, job.RetryCount+1, nextRetry.Format(time.Kitchen), errMsg),
				job.SourceID)
			s.recordDomainOutcome(job, string(model.CrawlJobRetrying), errType, errMsg, &nextRetry)
		}
	}
}

// handleEmptyContent handles extraction that returned too-short content.
func (s *CrawlQueueService) handleEmptyContent(job *model.CrawlJob, extractor string, cb *circuitBreaker, durationMs int) {
	// Check how many times this domain had empty content recently
	count, _ := s.repo.CountByDomainAndStatus(job.SourceDomain, model.CrawlJobBlocked, time.Now().Add(-24*time.Hour))
	if int(count)+1 >= s.cfg.PaywallThreshold {
		s.repo.MarkBlocked(job.ID, "empty_content_repeated")
		s.logActivity("job_blocked", "blocked",
			fmt.Sprintf("Blocked (repeated empty): %s domain=%s count=%d", job.URL, job.SourceDomain, count+1),
			job.SourceID)
		s.notifyBlocked(job.URL, job.SourceDomain, "empty_content_repeated")
		s.autoLearnDomain(job.SourceDomain)
		s.recordDomainOutcome(job, string(model.CrawlJobBlocked), "empty_content_repeated", "repeated empty content", nil)
	} else {
		// Retry once to confirm
		if job.RetryCount >= 1 {
			s.repo.MarkBlocked(job.ID, "empty_content")
			s.logActivity("job_blocked", "blocked",
				fmt.Sprintf("Blocked (empty content): %s", job.URL), job.SourceID)
			s.recordDomainOutcome(job, string(model.CrawlJobBlocked), "empty_content", "content too short", nil)
		} else {
			nextRetry := time.Now().Add(30 * time.Second)
			s.repo.MarkRetry(job.ID, nextRetry, "empty_content", "content too short")
			s.logActivity("job_retry", "retrying",
				fmt.Sprintf("Retry (empty content): %s", job.URL), job.SourceID)
			s.recordDomainOutcome(job, string(model.CrawlJobRetrying), "empty_content", "content too short", &nextRetry)
		}
	}
}

// detectPaywallKeywords checks for paywall/anti-crawl keywords in extracted content.
func (s *CrawlQueueService) detectPaywallKeywords(content string) bool {
	if len(content) > 500 {
		content = content[:500]
	}
	keywords := []string{
		"subscribe to continue", "sign in to read", "premium content",
		"subscribe to read", "subscription required", "members only",
		"paid content", "paid article",
		"订阅后查看", "登录后阅读", "付费内容", "会员专享",
	}
	lower := strings.ToLower(content)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// calculateBackoff computes the next retry time with exponential backoff + jitter.
func (s *CrawlQueueService) calculateBackoff(attempt int, errType string) time.Time {
	base := time.Duration(s.cfg.RetryBackoffBase) * time.Second
	cap := time.Duration(s.cfg.RetryBackoffMax) * time.Second

	switch errType {
	case "server_error":
		base = 30 * time.Second
		cap = 5 * time.Minute
	case "rate_limited", "forbidden":
		minBase := 5 * time.Minute
		if base < minBase {
			base = minBase
		}
		if cap < base {
			cap = base
		}
	}

	// Exponential: base * 2^attempt
	delay := base * time.Duration(math.Pow(2, float64(attempt)))
	if delay > cap {
		delay = cap
	}

	// ±25% jitter
	jitter := time.Duration(float64(delay) * 0.25 * (2*rand.Float64() - 1))
	delay += jitter
	if delay < 0 {
		delay = base
	}

	return time.Now().Add(delay)
}

func (s *CrawlQueueService) recordDomainOutcome(job *model.CrawlJob, status, errType, notes string, retryAt *time.Time) {
	if job == nil || job.SourceDomain == "" || !s.cfg.DomainThrottleEnabled || s.domainRepo == nil {
		return
	}
	profile, err := s.domainRepo.FindOrCreate(job.SourceDomain, s.domainDefaultDelaySeconds(), s.domainDefaultMaxConcurrency())
	if err != nil {
		log.Printf("[CrawlQueue] domain profile outcome lookup failed for %s: %v", job.SourceDomain, err)
		return
	}
	nextAllowedAt := nextAllowedForDomainOutcome(profile, status, errType, time.Now(), retryAt)
	if len(notes) > 1000 {
		notes = notes[:1000]
	}
	if err := s.domainRepo.RecordOutcome(job.SourceDomain, status, notes, nextAllowedAt); err != nil {
		log.Printf("[CrawlQueue] domain profile outcome update failed for %s: %v", job.SourceDomain, err)
	}
	if err := s.domainRepo.RefreshRates(job.SourceDomain, time.Now().Add(-24*time.Hour)); err != nil {
		log.Printf("[CrawlQueue] domain profile rate refresh failed for %s: %v", job.SourceDomain, err)
	}
}

func nextAllowedForDomainOutcome(profile *model.CrawlDomainProfile, status, errType string, now time.Time, retryAt *time.Time) *time.Time {
	if retryAt != nil && retryAt.After(now) {
		return retryAt
	}
	delay := 60 * time.Second
	if profile != nil && profile.DefaultDelaySeconds > 0 {
		delay = time.Duration(profile.DefaultDelaySeconds) * time.Second
	}

	switch errType {
	case "rate_limited", "forbidden":
		next := now.Add(maxDuration(5*time.Minute, 2*delay))
		return &next
	case "timeout", "network", "server_error", "empty_content", "empty_content_repeated":
		next := now.Add(maxDuration(time.Minute, 2*delay))
		return &next
	}

	switch status {
	case string(model.CrawlJobSuccess), string(model.CrawlJobSkipped):
		next := now.Add(delay)
		return &next
	case string(model.CrawlJobBlocked):
		next := now.Add(maxDuration(10*time.Minute, 5*delay))
		return &next
	}
	return nil
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// isBlockedDomain checks if a domain is in the blocked set.
func (s *CrawlQueueService) isBlockedDomain(domain string) bool {
	s.domainMu.RLock()
	defer s.domainMu.RUnlock()
	return s.blockedDomains[domain]
}

// autoLearnDomain adds a domain to the in-memory blocked set.
func (s *CrawlQueueService) autoLearnDomain(domain string) {
	if domain == "" {
		return
	}
	s.domainMu.Lock()
	defer s.domainMu.Unlock()
	if !s.blockedDomains[domain] {
		s.blockedDomains[domain] = true
		log.Printf("[CrawlQueue] auto-learned blocked domain: %s", domain)
	}
}

// rebuildBlockedDomains rebuilds the blocked domain set from DB history.
func (s *CrawlQueueService) rebuildBlockedDomains() {
	since := time.Now().Add(-7 * 24 * time.Hour)
	domains, err := s.repo.GetRecentlyBlockedDomains(since)
	if err != nil {
		log.Printf("[CrawlQueue] failed to rebuild blocked domains: %v", err)
		return
	}
	s.domainMu.Lock()
	defer s.domainMu.Unlock()
	for _, d := range domains {
		if !s.blockedDomains[d] {
			s.blockedDomains[d] = true
			log.Printf("[CrawlQueue] restored blocked domain from DB: %s", d)
		}
	}
}

// notifyBlocked sends a Matrix notification for blocked URLs.
func (s *CrawlQueueService) notifyBlocked(rawURL, domain, reason string) {
	if s.notification == nil {
		return
	}
	msg := fmt.Sprintf("[CrawlQueue] URL blocked: %s | Domain: %s | Reason: %s", rawURL, domain, reason)
	s.notification.Send(context.Background(), &NotificationRequest{
		Channel:     "alerts",
		Message:     msg,
		MessageType: "text",
		DedupKey:    "crawl_blocked:" + domain,
		Severity:    "info",
	})
}

// logActivity logs a crawl queue activity event.
func (s *CrawlQueueService) logActivity(action, status, summary string, sourceID uint) {
	if s.activityLog == nil {
		return
	}
	s.activityLog.LogActivity(LogActivityParams{
		Module:  "crawl_queue",
		Action:  action,
		Status:  status,
		Summary: summary,
		RefID:   fmt.Sprintf("source:%d", sourceID),
	})
}

// Stats returns queue statistics.
func (s *CrawlQueueService) Stats() (*repository.CrawlQueueStats, error) {
	return s.repo.Stats()
}

// Audit returns recent crawl failure and extractor aggregates.
func (s *CrawlQueueService) Audit(since time.Time, limit int) (*repository.CrawlAuditStats, error) {
	return s.repo.Audit(since, limit)
}

// ListDomainProfiles returns per-domain throttle and health profiles.
func (s *CrawlQueueService) ListDomainProfiles(page, limit int) ([]model.CrawlDomainProfile, int64, error) {
	if s.domainRepo == nil {
		return nil, 0, fmt.Errorf("domain profile repository is not configured")
	}
	return s.domainRepo.List(page, limit)
}

// ListJobs returns filtered crawl jobs.
func (s *CrawlQueueService) ListJobs(opts repository.ListCrawlJobOpts) ([]model.CrawlJob, int64, error) {
	return s.repo.List(opts)
}

// RetryJob manually retries a dead or blocked job.
func (s *CrawlQueueService) RetryJob(id uint) error {
	return s.repo.UpdateStatus(id, model.CrawlJobPending, map[string]interface{}{
		"retry_count":   0,
		"next_retry_at": nil,
		"started_at":    nil,
		"error_type":    "",
		"error_message": "",
		"block_reason":  "",
	})
}

// UnblockJob unblocks a blocked job and retries it.
func (s *CrawlQueueService) UnblockJob(id uint) error {
	return s.repo.UpdateStatus(id, model.CrawlJobPending, map[string]interface{}{
		"block_reason":  "",
		"retry_count":   0,
		"next_retry_at": nil,
		"started_at":    nil,
		"error_type":    "",
		"error_message": "",
	})
}

// WorkerStatus holds per-channel worker health info.
type WorkerStatus struct {
	Channel         string `json:"channel"`
	WorkerCount     int    `json:"worker_count"`
	BreakerState    string `json:"breaker_state"`
	ConsecutiveFail int    `json:"consecutive_fail"`
}

// WorkerStatuses returns the health status of all worker channels.
func (s *CrawlQueueService) WorkerStatuses() []WorkerStatus {
	channels := map[string]int{
		"firecrawl":   s.cfg.FirecrawlWorkers,
		"trafilatura": s.cfg.TrafilaturaWorkers,
		"auto":        s.cfg.AutoWorkers,
	}
	var statuses []WorkerStatus
	for ch, count := range channels {
		cb := s.breakers[ch]
		state := "closed"
		failCount := 0
		if cb != nil {
			state = cb.status()
			cb.mu.Lock()
			failCount = cb.consecutiveFail
			cb.mu.Unlock()
		}
		statuses = append(statuses, WorkerStatus{
			Channel:         ch,
			WorkerCount:     count,
			BreakerState:    state,
			ConsecutiveFail: failCount,
		})
	}
	return statuses
}

// GetBlockedJobs returns all blocked jobs.
func (s *CrawlQueueService) GetBlockedJobs() ([]model.CrawlJob, error) {
	return s.repo.GetBlockedSince(time.Time{}) // all time
}

// classifyCrawlError maps an extraction error to an error type.
func classifyCrawlError(err error) (string, string) {
	if err == nil {
		return "unknown", ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)

	switch {
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "timeout", msg
	case strings.Contains(lower, "429"):
		return "rate_limited", msg
	case strings.Contains(lower, "404") || strings.Contains(lower, "410") || strings.Contains(lower, "not found"):
		return "not_found", msg
	case strings.Contains(lower, "403") || strings.Contains(lower, "forbidden"):
		return "forbidden", msg
	case strings.Contains(lower, "400") || strings.Contains(lower, "401") || strings.Contains(lower, "422"):
		return "client_error", msg
	case strings.Contains(lower, "500") || strings.Contains(lower, "502") || strings.Contains(lower, "503") || strings.Contains(lower, "504"):
		return "server_error", msg
	case strings.Contains(lower, "paywall") || strings.Contains(lower, "subscribe"):
		return "paywall", msg
	case strings.Contains(lower, "empty content") || strings.Contains(lower, "too short"):
		return "empty_content", msg
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "dns") || strings.Contains(lower, "network"):
		return "network", msg
	default:
		return "unknown", msg
	}
}

func retryAfterFromError(err error, now time.Time) (time.Time, bool) {
	if err == nil {
		return time.Time{}, false
	}
	value := extractRetryAfterValue(err.Error())
	if value == "" {
		return time.Time{}, false
	}
	if seconds, parseErr := strconv.Atoi(value); parseErr == nil {
		if seconds <= 0 {
			return time.Time{}, false
		}
		if seconds > 86400 {
			seconds = 86400
		}
		return now.Add(time.Duration(seconds) * time.Second), true
	}
	retryAt, parseErr := http.ParseTime(value)
	if parseErr != nil || !retryAt.After(now) {
		return time.Time{}, false
	}
	return retryAt, true
}

func extractRetryAfterValue(msg string) string {
	for _, marker := range []string{"retry_after=", "retry-after=", "retry-after:", "retry after "} {
		lower := strings.ToLower(msg)
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(msg[idx+len(marker):])
		if rest == "" {
			continue
		}
		if strings.HasPrefix(rest, "\"") {
			if end := strings.Index(rest[1:], "\""); end >= 0 {
				return rest[1 : 1+end]
			}
		}
		line := strings.TrimSpace(strings.SplitN(rest, "\n", 2)[0])
		if _, err := http.ParseTime(line); err == nil {
			return line
		}
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			continue
		}
		return strings.Trim(parts[0], " ,;\"'")
	}
	return ""
}

// crawlExtractDomain extracts the domain from a URL.
func crawlExtractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// coalesce returns the first non-empty string.
func coalesce(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
