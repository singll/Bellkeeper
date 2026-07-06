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
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

// CrawlQueueService manages the persistent crawl queue with worker pools.
type CrawlQueueService struct {
	cfg           config.CrawlQueueConfig
	repo          *repository.CrawlJobRepository
	domainRepo    *repository.CrawlDomainProfileRepository
	failureRepo   *repository.CrawlFailureRepository
	extractor     *ExtractorService
	ingestion     *FileIngestionService
	activityLog   *ActivityLogService
	notification  *NotificationService
	eventPublisher *KBEventPublisher

	// Worker management
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Per-channel circuit breakers
	breakers map[string]*circuitBreaker
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
	failureRepo *repository.CrawlFailureRepository,
	extractor *ExtractorService,
	ingestion *FileIngestionService,
	activityLog *ActivityLogService,
) *CrawlQueueService {
	svc := &CrawlQueueService{
		cfg:         cfg,
		repo:        repo,
		domainRepo:  domainRepo,
		failureRepo: failureRepo,
		extractor:   extractor,
		ingestion:   ingestion,
		activityLog: activityLog,
		breakers:    make(map[string]*circuitBreaker),
	}

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

// SetEventPublisher 注入知识库事件发布器（1.0 事件化）。
// 未注入时 crawl worker 走 fallback 同步入库（行为等同重构前）。
func (s *CrawlQueueService) SetEventPublisher(pub *KBEventPublisher) {
	s.eventPublisher = pub
}

// Domain cooling backoff bounds (exponential: base * 2^(failures-1), capped at max).
// Cooling is persisted on crawl_domain_profiles.next_allowed_at so it survives restarts
// and is enforced at dequeue time by DequeueFair's LEFT JOIN — no in-memory state.
const (
	crawlCoolingBase = 1 * time.Minute
	crawlCoolingMax  = 1 * time.Hour
)

// enterCooling pushes a domain's next_allowed_at out via exponential backoff.
func (s *CrawlQueueService) enterCooling(domain, errType, errMsg string) {
	if domain == "" || s.domainRepo == nil {
		return
	}
	if err := s.domainRepo.EnterCooling(domain, crawlCoolingBase, crawlCoolingMax); err != nil {
		log.Printf("[CrawlQueue] enter cooling for %s failed: %v", domain, err)
		return
	}
	log.Printf("[CrawlQueue] domain %s cooling: errType=%s err=%s", domain, errType, errMsg)
}

// clearCooling resets a domain's cooling state after a successful crawl.
func (s *CrawlQueueService) clearCooling(domain string) {
	if domain == "" || s.domainRepo == nil {
		return
	}
	if err := s.domainRepo.ClearCooling(domain); err != nil {
		log.Printf("[CrawlQueue] clear cooling for %s failed: %v", domain, err)
	}
}

// getRequestOverrides loads domain-level extraction overrides (set by the rule
// optimizer). Read-only — never creates a profile row on the hot path.
func (s *CrawlQueueService) getRequestOverrides(domain string) *RequestOverrides {
	if domain == "" || s.domainRepo == nil {
		return nil
	}
	profile, err := s.domainRepo.FindByDomain(domain)
	if err != nil {
		log.Printf("[CrawlQueue] load overrides for %s failed: %v", domain, err)
		return nil
	}
	if profile == nil || len(profile.RequestOverrides) == 0 {
		return nil
	}
	var overrides RequestOverrides
	if err := json.Unmarshal(profile.RequestOverrides, &overrides); err != nil {
		log.Printf("[CrawlQueue] unmarshal overrides for %s failed: %v", domain, err)
		return nil
	}
	return &overrides
}

// Enqueue adds a URL to the crawl queue.
func (s *CrawlQueueService) Enqueue(sourceID uint, rawURL, title, channelType string, metadata map[string]interface{}) (uint, error) {
	domain := crawlExtractDomain(rawURL)

	status := model.CrawlJobPending
	if channelType == "" {
		channelType = "auto"
	}

	// Per-domain back-pressure: cap how many in-queue jobs a single domain may
	// accumulate. This is the safety valve against discovery outrunning consumption
	// (e.g. one site flooding the queue with millions of pending URLs). Explicit
	// rejection (⏭️), never a silent drop.
	if s.cfg.DomainPendingCap > 0 && domain != "" {
		pending, err := s.repo.CountPendingByDomain(domain)
		if err != nil {
			log.Printf("[CrawlQueue] pending count for %s failed: %v", domain, err)
		} else if pending >= int64(s.cfg.DomainPendingCap) {
			s.logActivity("enqueue_rejected", "skipped",
				fmt.Sprintf("⏭️ Enqueue rejected: domain %s at pending cap %d — %s", domain, s.cfg.DomainPendingCap, rawURL), sourceID)
			return 0, fmt.Errorf("domain %s reached pending cap %d", domain, s.cfg.DomainPendingCap)
		}
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
		b, err := json.Marshal(metadata)
		if err != nil {
			middleware.GetLogger().Warn("crawl queue: marshal metadata failed",
				zap.String("url", rawURL), zap.Error(err))
		} else {
			job.Metadata = b
		}
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

	if err := s.repo.RecoverOrphanedJobs(); err != nil {
		log.Printf("[CrawlQueue] failed to recover orphaned jobs: %v", err)
	}

	s.wg.Add(1)
	go s.staleJobRecoveryLoop(ctx)

	s.wg.Add(1)
	go s.heartbeatLoop(ctx)

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
			// 1.0 事件化：回收卡在 crawled（待 extract-worker 入库）超时的 job。
			crawledRecovered, err := s.repo.RecoverStaleCrawledJobs(staleTimeout)
			if err != nil {
				log.Printf("[CrawlQueue] stale crawled recovery error: %v", err)
				continue
			}
			if crawledRecovered > 0 {
				log.Printf("[CrawlQueue] recovered %d stale crawled jobs (stale>%s) — extract-worker stuck or event lost", crawledRecovered, staleTimeout)
			}
		}
	}
}

// heartbeatLoop writes a heartbeat activity_log every 5 minutes.
func (s *CrawlQueueService) heartbeatLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.logActivity("crawl_queue", "success", "CrawlQueue alive", 0)
		}
	}
}

// tierRefreshLoop removed: domain prioritization is now handled at dequeue time by
// DequeueFair (per-domain fair rotation + next_allowed_at cooling filter), so there
// is no in-memory tier map to refresh.

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

		cb := s.breakers[channelType]
		if cb != nil && !cb.allow() {
			continue
		}

		job, err := s.repo.DequeueFair(channelType)
		if err != nil {
			log.Printf("[CrawlQueue:%s] dequeue error: %v", channelType, err)
			continue
		}
		if job == nil {
			continue
		}

		s.processJob(ctx, job, channelType, cb)
	}
}

// dequeueByTier removed: replaced by repo.DequeueFair (single SQL, per-domain fair
// rotation with cooling filtered via next_allowed_at).
//
// Per-domain throttle gating (reserveDomainSlot/decideDomainThrottle) was also
// removed: next_allowed_at — set by recordDomainOutcome on success and by
// enterCooling on failure — is now enforced directly in DequeueFair, so a job is
// never claimed for a domain that should wait.

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
//
// 1.0 重构（§2.1.2）：crawl worker 只负责"抓取+提取正文"（extractor.Extract），
// 成功后发布 knowledge.crawl.done 事件并将 job 状态置 crawled（待 extract-worker
// 入库）；入库（IngestURL）与 Meili 索引由独立 extract-worker / index-worker 经
// eventbus 消费完成。事件发布失败或 payload 超限时走 fallback 同步入库（行为
// 等同重构前），保证不丢内容。
func (s *CrawlQueueService) processJob(ctx context.Context, job *model.CrawlJob, workerChannel string, cb *circuitBreaker) {
	startTime := time.Now()

	// Determine extractor based on channel type
	extractorName := workerChannel
	if workerChannel == "auto" {
		extractorName = "" // let ExtractorService decide (trafilatura first, firecrawl fallback)
	}

	// Extract content (抓取+提取正文一步完成)
	extractReq := &ExtractionRequest{URL: job.URL}
	if overrides := s.getRequestOverrides(job.SourceDomain); overrides != nil {
		extractReq.Overrides = overrides
	}
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

	content := result.Content
	extractorUsed := result.Extractor

	// Layer 2: empty content check → possible paywall
	if len(content) < 100 {
		s.handleEmptyContent(job, extractorUsed, cb, durationMs)
		return
	}

	// Layer 3: paywall keyword detection
	if s.detectPaywallKeywords(content) {
		s.handleExtractionFailure(job, fmt.Errorf("paywall keywords detected"), extractorUsed, cb, durationMs)
		return
	}

	// 抓取+提取成功 → 尝试发布 knowledge.crawl.done 事件，交 extract-worker 入库。
	if s.eventPublisher != nil {
		payload := KBCrawlDonePayload{
			JobID:         job.ID,
			URL:           job.URL,
			Title:         coalesce(job.Title, result.Title),
			SourceID:      job.SourceID,
			SourceDomain:  job.SourceDomain,
			Content:       content,
			Extractor:     extractorUsed,
			ContentLength: len(content),
		}
		published, perr := s.eventPublisher.PublishCrawlDone(ctx, payload)
		if perr != nil {
			log.Printf("[CrawlQueue] publish crawl.done failed for job %d: %v (fallback to sync ingest)", job.ID, perr)
		}
		if published {
			// 事件已投递：状态置 crawled（待 extract-worker 入库），退出冷却。
			updates := map[string]interface{}{
				"content_length": len(content),
				"extractor_used": extractorUsed,
			}
			s.clearCooling(job.SourceDomain)
			if err := s.repo.UpdateStatus(job.ID, model.CrawlJobCrawled, updates); err != nil {
				log.Printf("[CrawlQueue] failed to mark job %d crawled: %v", job.ID, err)
			}
			s.logActivity("job_crawled", "crawled",
				fmt.Sprintf("Crawled (pending extract): %s extractor=%s len=%d", job.URL, extractorUsed, len(content)),
				job.SourceID)
			s.recordDomainOutcome(job, string(model.CrawlJobCrawled), "", "success", nil)
			if cb != nil {
				cb.recordSuccess()
			}
			return
		}
		// published=false：payload 超限或 bus 未配置 → fallback 同步入库。
	}

	// Fallback：同步入库（无 eventPublisher 或 payload 超限），行为等同重构前。
	s.ingestAndFinalize(ctx, job, content, extractorUsed, cb)
}

// ingestAndFinalize 同步执行入库 + 状态终结（fallback 路径，或 extract-worker 内部复用）。
func (s *CrawlQueueService) ingestAndFinalize(ctx context.Context, job *model.CrawlJob, content, extractorUsed string, cb *circuitBreaker) {
	ingestResult, err := s.ingestion.IngestURL(&IngestURLRequest{
		URL:      job.URL,
		Title:    coalesce(job.Title, ""),
		Content:  content,
		Category: "",
		Layer:    "",
	})

	if err != nil {
		s.handleExtractionFailure(job, err, extractorUsed, cb, 0)
		return
	}

	// Mark success
	updates := map[string]interface{}{
		"content_length": len(content),
		"extractor_used": extractorUsed,
	}
	s.clearCooling(job.SourceDomain)
	if s.failureRepo != nil {
		if err := s.failureRepo.ResolveByURL(job.URL); err != nil {
			log.Printf("[CrawlQueue] resolve failure record for %s failed: %v", job.URL, err)
		}
	}
	if ingestResult.Status == "duplicate" || ingestResult.Status == "duplicate_content" {
		if err := s.repo.UpdateStatus(job.ID, model.CrawlJobSkipped, updates); err != nil {
			log.Printf("[CrawlQueue] failed to update job %d status to skipped: %v", job.ID, err)
		}
		s.logActivity("job_skipped", "skipped",
			fmt.Sprintf("Skipped (duplicate): %s", job.URL), job.SourceID)
		s.recordDomainOutcome(job, string(model.CrawlJobSkipped), "", "duplicate", nil)
	} else {
		if err := s.repo.UpdateStatus(job.ID, model.CrawlJobSuccess, updates); err != nil {
			log.Printf("[CrawlQueue] failed to update job %d status to success: %v", job.ID, err)
		}
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

	s.enterCooling(job.SourceDomain, errType, errMsg)

	if job.RetryCount >= job.MaxRetries {
		if s.failureRepo != nil {
			if fErr := s.failureRepo.UpsertFromJob(job, errType, errMsg); fErr != nil {
				log.Printf("[CrawlQueue] failed to upsert crawl failure for job %d: %v", job.ID, fErr)
			}
		}
		if err := s.repo.UpdateStatus(job.ID, model.CrawlJobSkipped, map[string]interface{}{
			"error_type":    errType,
			"error_message": errMsg,
		}); err != nil {
			log.Printf("[CrawlQueue] failed to mark job %d skipped (max retries): %v", job.ID, err)
		}
		s.logActivity("job_archived", "skipped",
			fmt.Sprintf("Archived to failures (max retries): %s retries=%d err=%s", job.URL, job.RetryCount, errMsg), job.SourceID)
		s.recordDomainOutcome(job, string(model.CrawlJobSkipped), errType, errMsg, nil)
		s.publishCrawlFailed(job, errType, errMsg, true)
		return
	}

	nextRetry := s.calculateBackoff(job.RetryCount, errType)
	if errType == "rate_limited" {
		if retryAfter, ok := retryAfterFromError(err, time.Now()); ok && retryAfter.After(time.Now()) {
			nextRetry = retryAfter
		}
	}

	if err := s.repo.MarkRetry(job.ID, nextRetry, errType, errMsg); err != nil {
		log.Printf("[CrawlQueue] failed to mark job %d retry: %v", job.ID, err)
	}
	s.logActivity("job_retry", "retrying",
		fmt.Sprintf("Retry: %s attempt=%d next=%s type=%s err=%s", job.URL, job.RetryCount+1, nextRetry.Format(time.Kitchen), errType, errMsg),
		job.SourceID)
	s.recordDomainOutcome(job, string(model.CrawlJobRetrying), errType, errMsg, &nextRetry)
	// 1.0 §2.1.1：发布 crawl.failed 事件（非终结，将重试）供域名健康度 worker 评估。
	s.publishCrawlFailed(job, errType, errMsg, false)
}

// publishCrawlFailed 发布 knowledge.crawl.failed 事件（eventPublisher 未配置时静默跳过）。
func (s *CrawlQueueService) publishCrawlFailed(job *model.CrawlJob, errType, errMsg string, terminal bool) {
	if s.eventPublisher == nil {
		return
	}
	if perr := s.eventPublisher.PublishCrawlFailed(context.Background(), KBCrawlFailedPayload{
		JobID:        job.ID,
		URL:          job.URL,
		SourceDomain: job.SourceDomain,
		ErrorType:    errType,
		ErrorMessage: errMsg,
		RetryCount:   job.RetryCount,
		Terminal:     terminal,
	}); perr != nil {
		log.Printf("[CrawlQueue] publish crawl.failed for job %d failed: %v", job.ID, perr)
	}
}

// handleEmptyContent handles extraction that returned too-short content.
func (s *CrawlQueueService) handleEmptyContent(job *model.CrawlJob, extractor string, cb *circuitBreaker, durationMs int) {
	s.enterCooling(job.SourceDomain, "empty_content", "content too short")

	if cb != nil {
		cb.recordFailure()
	}

	if job.RetryCount >= job.MaxRetries {
		if s.failureRepo != nil {
			if fErr := s.failureRepo.UpsertFromJob(job, "empty_content", "content too short"); fErr != nil {
				log.Printf("[CrawlQueue] failed to upsert crawl failure for empty content job %d: %v", job.ID, fErr)
			}
		}
		if err := s.repo.UpdateStatus(job.ID, model.CrawlJobSkipped, map[string]interface{}{
			"error_type":    "empty_content",
			"error_message": "content too short",
		}); err != nil {
			log.Printf("[CrawlQueue] failed to mark job %d skipped (empty content): %v", job.ID, err)
		}
		s.logActivity("job_archived", "skipped",
			fmt.Sprintf("Archived (repeated empty): %s domain=%s", job.URL, job.SourceDomain), job.SourceID)
		s.recordDomainOutcome(job, string(model.CrawlJobSkipped), "empty_content", "content too short", nil)
		s.publishCrawlFailed(job, "empty_content", "content too short", true)
		return
	}

	nextRetry := time.Now().Add(30 * time.Second)
	if err := s.repo.MarkRetry(job.ID, nextRetry, "empty_content", "content too short"); err != nil {
		log.Printf("[CrawlQueue] failed to mark job %d for retry: %v", job.ID, err)
	}
	s.logActivity("job_retry", "retrying",
		fmt.Sprintf("Retry (empty content): %s", job.URL), job.SourceID)
	s.recordDomainOutcome(job, string(model.CrawlJobRetrying), "empty_content", "content too short", &nextRetry)
	s.publishCrawlFailed(job, "empty_content", "content too short", false)
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

// CleanupStalePending marks stale pending jobs as skipped.
func (s *CrawlQueueService) CleanupStalePending(olderThanDays int, domain string) (int64, error) {
	olderThan := time.Now().AddDate(0, 0, -olderThanDays)
	skipped, err := s.repo.MarkSkippedStalePending(olderThan, domain)
	if err != nil {
		return 0, fmt.Errorf("cleanup stale pending: %w", err)
	}
	if skipped > 0 {
		s.logActivity("cleanup", "skipped",
			fmt.Sprintf("Skipped %d stale pending jobs (older than %d days, domain=%s)", skipped, olderThanDays, domain), 0)
	}
	return skipped, nil
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
