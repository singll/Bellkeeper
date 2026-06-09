package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

// Health tracking constants
const (
	HealthScoreMax          = 100
	HealthScoreThreshold    = 20 // auto-unpause when health_score recovers above this
	HealthScoreDecrement    = 15 // 每次连续失败扣减（5次失败后 health=25，仍可恢复）
	HealthScoreIncrement    = 10 // increase per successful fetch (capped at 100)
	HealthScoreRecoveryStep = 5  // gradual recovery per successful fetch after failure

	// Retry backoff: 1min -> 5min -> 30min -> 2h
	RetryDelayMin    = 1 * time.Minute
	RetryDelayMax    = 2 * time.Hour
	MaxRetryAttempts = 4

	// 连续失败暂停阈值：5次连续失败才自动暂停（独立于重试调度）
	PauseThreshold = 5
)

// retryBackoff returns the delay for a given retry attempt using exponential backoff.
// Attempt 0: 1min, Attempt 1: 5min, Attempt 2: 30min, Attempt 3: 2h
func retryBackoff(attempt int) time.Duration {
	switch attempt {
	case 0:
		return 1 * time.Minute
	case 1:
		return 5 * time.Minute
	case 2:
		return 30 * time.Minute
	default:
		return 2 * time.Hour
	}
}

// RSSFetcherConfig holds configuration for the RSS fetcher
type RSSFetcherConfig struct {
	Enabled       bool
	CheckInterval int    // seconds between feed checks
	MaxPerBatch   int    // max feeds to process per check
	Timeout       int    // seconds for each feed fetch
	RSSHubBaseURL string // RSSHub 实例地址，用于拼接以 / 开头的相对路径
}

// retryItem represents a feed scheduled for retry
type retryItem struct {
	feedID    uint
	attempt   int       // current retry attempt (0 = first retry)
	scheduled time.Time // when this retry should execute
}

// RSSFetcherService handles periodic RSS feed fetching with health tracking
type RSSFetcherService struct {
	cfg        RSSFetcherConfig
	rssRepo    *repository.RSSRepository
	ingestion  *FileIngestionService
	activity   *ActivityLogService
	fetcher    *gofeed.Parser
	httpClient *http.Client
	stopCh     chan struct{}
	wg         sync.WaitGroup
	running    bool
	mu         sync.RWMutex

	// Crawl queue (optional, set via SetCrawlQueueService)
	crawlQueue *CrawlQueueService

	// Retry queue
	retryQueue []retryItem
	retryMu    sync.Mutex

	// Per-source concurrency semaphores
	semaphores map[uint]*semaphore
	semMu      sync.Mutex
}

// semaphore is a simple concurrency limiter using a buffered channel
type semaphore struct {
	ch chan struct{}
}

func newSemaphore(max int) *semaphore {
	if max <= 0 {
		max = 3
	}
	return &semaphore{ch: make(chan struct{}, max)}
}

func (s *semaphore) acquire() { s.ch <- struct{}{} }
func (s *semaphore) release() { <-s.ch }

// NewRSSFetcherService creates a new RSS fetcher service
func NewRSSFetcherService(
	cfg RSSFetcherConfig,
	rssRepo *repository.RSSRepository,
	ingestion *FileIngestionService,
) *RSSFetcherService {
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 60
	}
	if cfg.MaxPerBatch <= 0 {
		cfg.MaxPerBatch = 5
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30
	}

	return &RSSFetcherService{
		cfg:        cfg,
		rssRepo:    rssRepo,
		ingestion:  ingestion,
		fetcher:    gofeed.NewParser(),
		httpClient: &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
		stopCh:     make(chan struct{}),
		semaphores: make(map[uint]*semaphore),
	}
}

// SetActivityLogService sets the activity log service for observability
func (s *RSSFetcherService) SetActivityLogService(activity *ActivityLogService) {
	s.activity = activity
}

// SetCrawlQueueService sets the crawl queue service for async processing
func (s *RSSFetcherService) SetCrawlQueueService(cq *CrawlQueueService) {
	s.crawlQueue = cq
}

// Start starts the RSS fetcher background loop
func (s *RSSFetcherService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.runLoop(ctx)
	log.Println("[RSSFetcher] started")
}

// Stop stops the RSS fetcher background loop
func (s *RSSFetcherService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	s.wg.Wait()
	log.Println("[RSSFetcher] stopped")
}

// runLoop runs the main fetch loop with retry processing
func (s *RSSFetcherService) runLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(s.cfg.CheckInterval) * time.Second)
	retryTicker := time.NewTicker(30 * time.Second) // check retry queue every 30s
	defer ticker.Stop()
	defer retryTicker.Stop()

	for {
		select {
		case <-ticker.C:
			s.fetchAllActive(ctx)
		case <-retryTicker.C:
			s.processRetryQueue(ctx)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// fetchAllActive fetches all active (non-paused) RSS feeds
func (s *RSSFetcherService) fetchAllActive(ctx context.Context) {
	feeds, err := s.rssRepo.GetActive()
	if err != nil {
		log.Printf("[RSSFetcher] failed to get active feeds: %v", err)
		return
	}

	if len(feeds) == 0 {
		return
	}

	now := time.Now()
	dueFeeds := make([]model.RSSFeed, 0, len(feeds))
	for _, feed := range feeds {
		if due, next := isRSSFeedDue(feed, now); due {
			dueFeeds = append(dueFeeds, feed)
		} else {
			log.Printf("[RSSFetcher] skipping feed %d (%s): next fetch at %s", feed.ID, feed.Name, next.Format(time.RFC3339))
		}
	}
	if len(dueFeeds) == 0 {
		log.Printf("[RSSFetcher] no feeds due (active=%d)", len(feeds))
		return
	}

	log.Printf("[RSSFetcher] fetching %d due feeds (active=%d)", len(dueFeeds), len(feeds))

	// Process feeds in batches with per-source concurrency
	batchSize := s.cfg.MaxPerBatch
	for i := 0; i < len(dueFeeds); i += batchSize {
		end := i + batchSize
		if end > len(dueFeeds) {
			end = len(dueFeeds)
		}

		batch := dueFeeds[i:end]
		var wg sync.WaitGroup
		for _, feed := range batch {
			wg.Add(1)
			go func(f model.RSSFeed) {
				defer wg.Done()
				sem := s.getSemaphore(f.ID, f.MaxConcurrency)
				sem.acquire()
				defer sem.release()
				res := s.fetchFeed(ctx, &f)
				if res.Error != nil {
					log.Printf("[RSSFetcher] failed to fetch feed %d (%s): %v", f.ID, f.Name, res.Error)
				}
			}(feed)
		}
		wg.Wait()
	}
}

func isRSSFeedDue(feed model.RSSFeed, now time.Time) (bool, time.Time) {
	if feed.LastFetchedAt == nil {
		return true, time.Time{}
	}
	intervalMinutes := feed.FetchIntervalMinutes
	if intervalMinutes <= 0 {
		intervalMinutes = 60
	}
	next := feed.LastFetchedAt.Add(time.Duration(intervalMinutes) * time.Minute)
	return !now.Before(next), next
}

// getSemaphore returns (or creates) a concurrency semaphore for a source
func (s *RSSFetcherService) getSemaphore(feedID uint, maxConcurrency int) *semaphore {
	s.semMu.Lock()
	defer s.semMu.Unlock()

	if sem, ok := s.semaphores[feedID]; ok {
		return sem
	}
	sem := newSemaphore(maxConcurrency)
	s.semaphores[feedID] = sem
	return sem
}

// FetchFeed fetches a single RSS feed by ID
func (s *RSSFetcherService) FetchFeed(ctx context.Context, feedID uint) error {
	feed, err := s.rssRepo.GetByID(feedID)
	if err != nil {
		return fmt.Errorf("failed to get feed: %w", err)
	}

	result := s.fetchFeed(ctx, feed)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// FetchFeedResult holds the result of a fetch operation
type FetchFeedResult struct {
	FeedID        uint   `json:"feed_id"`
	FeedName      string `json:"feed_name"`
	ItemsFound    int    `json:"items_found"`
	ItemsNew      int    `json:"items_new"`
	ItemsDup      int    `json:"items_dup"`
	Skipped       bool   `json:"skipped,omitempty"`
	SkippedReason string `json:"skipped_reason,omitempty"`
	Error         string `json:"error,omitempty"`
}

// fetchFeedInternalResult holds the internal result of fetching a single feed
type fetchFeedInternalResult struct {
	ItemsFound int
	ItemsNew   int
	ItemsDup   int
	Error      error
}

// FetchFeedInternal fetches a single RSS feed by ID and returns the detailed internal result
// with item counts. This is used by CrawlService.FetchSource to populate FetchFeedResult fields.
func (s *RSSFetcherService) FetchFeedInternal(ctx context.Context, feedID uint) *fetchFeedInternalResult {
	feed, err := s.rssRepo.GetByID(feedID)
	if err != nil {
		return &fetchFeedInternalResult{
			Error: fmt.Errorf("failed to get feed: %w", err),
		}
	}

	return s.fetchFeed(ctx, feed)
}

// fetchFeed fetches and processes a single RSS feed with health tracking
func (s *RSSFetcherService) fetchFeed(ctx context.Context, feed *model.RSSFeed) *fetchFeedInternalResult {
	result := &fetchFeedInternalResult{}
	startTime := time.Now()

	// Set timeout for this fetch
	fetchCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.Timeout)*time.Second)
	defer cancel()

	log.Printf("[RSSFetcher] fetching feed: %s (%s)", feed.Name, feed.URL)

	// 拼接 RSSHub 相对路径：以 / 开头的 URL 视为 RSSHub 路径，需加上 base URL
	feedURL := feed.URL
	if strings.HasPrefix(feedURL, "/") && s.cfg.RSSHubBaseURL != "" {
		feedURL = s.cfg.RSSHubBaseURL + feedURL
		log.Printf("[RSSFetcher] resolved RSSHub URL: %s -> %s", feed.URL, feedURL)
	}

	// Parse RSS feed with improved error handling
	rssFeed, err := s.parseFeedWithRetry(fetchCtx, feedURL)
	if err != nil {
		// Record failure in health tracking
		s.recordFailure(feed, err.Error())

		// Update last_fetched_at even on error
		now := time.Now()
		feed.LastFetchedAt = &now
		s.rssRepo.Update(feed)

		// Log activity
		durationMs := int(time.Since(startTime).Milliseconds())
		s.logActivity("rss_fetch", "fetch", "failure",
			fmt.Sprintf("Feed %s fetch failed: %s", feed.Name, err.Error()),
			feed.ID, durationMs)

		result.Error = fmt.Errorf("failed to parse RSS: %w", err)
		return result
	}

	// Record success in health tracking
	s.recordSuccess(feed)

	// Update last_fetched_at
	now := time.Now()
	feed.LastFetchedAt = &now
	if err := s.rssRepo.Update(feed); err != nil {
		log.Printf("[RSSFetcher] failed to update feed timestamp: %v", err)
	}

	if len(rssFeed.Items) == 0 {
		log.Printf("[RSSFetcher] no items in feed %s", feed.Name)
		s.logActivity("rss_fetch", "fetch", "success",
			fmt.Sprintf("Feed %s fetched, 0 items found", feed.Name),
			feed.ID, int(time.Since(startTime).Milliseconds()))
		return result
	}

	log.Printf("[RSSFetcher] found %d items in feed %s", len(rssFeed.Items), feed.Name)
	result.ItemsFound = len(rssFeed.Items)

	// Process each item
	for _, item := range rssFeed.Items {
		if item.Link == "" {
			continue
		}

		// Use crawl queue if available (async, fast enqueue)
		if s.crawlQueue != nil {
			_, err := s.crawlQueue.Enqueue(feed.ID, item.Link, item.Title, "auto", nil)
			if err != nil {
				log.Printf("[RSSFetcher] failed to enqueue %s: %v", item.Link, err)
				s.logActivity("rss_fetch", "enqueue", "failure",
					fmt.Sprintf("Enqueue failed for %s: %v", item.Link, err),
					feed.ID, 0)
			} else {
				result.ItemsNew++
				log.Printf("[RSSFetcher] enqueued: %s", item.Link)
				s.logActivity("rss_fetch", "enqueue", "success",
					fmt.Sprintf("Enqueued: %s", item.Link),
					feed.ID, 0)
			}
			continue
		}

		// Fallback: direct ingestion (backward compatible)
		if s.ingestion == nil {
			continue
		}

		ingestResult, err := s.ingestion.IngestURL(&IngestURLRequest{
			URL:   item.Link,
			Title: item.Title,
		})
		if err != nil {
			log.Printf("[RSSFetcher] failed to ingest %s: %v", item.Link, err)
			s.logActivity("rss_fetch", "ingest", "failure",
				fmt.Sprintf("Ingest failed for %s: %v", item.Link, err),
				feed.ID, 0)
			continue
		}

		if ingestResult.Status == "duplicate" {
			result.ItemsDup++
			log.Printf("[RSSFetcher] skipped duplicate: %s", item.Link)
			s.logActivity("rss_fetch", "ingest", "duplicate",
				fmt.Sprintf("Duplicate skipped: %s", item.Link),
				feed.ID, 0)
		} else if ingestResult.Status == "success" {
			result.ItemsNew++
			log.Printf("[RSSFetcher] ingested: %s -> %s", item.Link, ingestResult.FilePath)
			s.logActivity("rss_fetch", "ingest", "success",
				fmt.Sprintf("New article ingested: %s", item.Link),
				feed.ID, 0)
		}
	}

	durationMs := int(time.Since(startTime).Milliseconds())
	s.logActivity("rss_fetch", "fetch", "success",
		fmt.Sprintf("Feed %s fetched: %d items, %d new, %d dup", feed.Name, result.ItemsFound, result.ItemsNew, result.ItemsDup),
		feed.ID, durationMs)

	return result
}

// parseFeedWithRetry attempts to parse an RSS feed, with fallback for malformed XML
func (s *RSSFetcherService) parseFeedWithRetry(ctx context.Context, url string) (*gofeed.Feed, error) {
	// First attempt: standard gofeed parsing
	feed, err := s.fetcher.ParseURLWithContext(url, ctx)
	if err == nil {
		return feed, nil
	}

	// Second attempt: fetch raw content, strip invalid XML chars, retry parse
	log.Printf("[RSSFetcher] standard parse failed for %s: %v, trying with XML cleanup", url, err)

	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP fetch returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Strip invalid XML characters (control chars except \t, \n, \r)
	cleaned := stripInvalidXMLChars(string(bodyBytes))

	// Try parsing the cleaned content
	feed, err = s.fetcher.Parse(strings.NewReader(cleaned))
	if err != nil {
		return nil, fmt.Errorf("RSS parse failed even after XML cleanup: %w", err)
	}

	return feed, nil
}

// stripInvalidXMLChars removes characters that are invalid in XML 1.0
// Valid chars: #x9 | #xA | #xD | [#x20-#xD7FF] | [#xE000-#xFFFD] | [#x10000-#x10FFFF]
func stripInvalidXMLChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == 0x9 || r == 0xA || r == 0xD ||
			(r >= 0x20 && r <= 0xD7FF) ||
			(r >= 0xE000 && r <= 0xFFFD) ||
			(r >= 0x10000 && r <= 0x10FFFF) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// recordSuccess updates health tracking after a successful fetch
func (s *RSSFetcherService) recordSuccess(feed *model.RSSFeed) {
	if feed.ConsecutiveFailures > 0 {
		// Recovery: bump health by recovery step
		feed.HealthScore += HealthScoreRecoveryStep
		if feed.HealthScore > HealthScoreMax {
			feed.HealthScore = HealthScoreMax
		}
	} else {
		// Steady success: small bump
		feed.HealthScore += HealthScoreIncrement
		if feed.HealthScore > HealthScoreMax {
			feed.HealthScore = HealthScoreMax
		}
	}
	feed.ConsecutiveFailures = 0
	feed.LastFailureReason = ""
	feed.TotalFetched++

	// Auto-unpause if health recovers above threshold
	if feed.IsPaused && feed.HealthScore >= HealthScoreThreshold+10 {
		feed.IsPaused = false
		feed.PausedAt = nil
		log.Printf("[RSSFetcher] auto-unpaused feed %d (%s) — health_score recovered to %d", feed.ID, feed.Name, feed.HealthScore)
		s.logActivity("rss_fetch", "health", "auto_unpause",
			fmt.Sprintf("Feed %s auto-unpaused, health_score=%d", feed.Name, feed.HealthScore),
			feed.ID, 0)
	}

	s.rssRepo.Update(feed)
}

// recordFailure updates health tracking after a failed fetch
func (s *RSSFetcherService) recordFailure(feed *model.RSSFeed, reason string) {
	feed.ConsecutiveFailures++
	feed.HealthScore -= HealthScoreDecrement
	if feed.HealthScore < 0 {
		feed.HealthScore = 0
	}
	feed.LastFailureReason = reason
	if len(reason) > 500 {
		feed.LastFailureReason = reason[:500]
	}
	feed.TotalFailed++

	// Auto-pause when consecutive failures reach PauseThreshold (默认5次)
	// 重试调度使用 MaxRetryAttempts，暂停阈值独立控制
	if !feed.IsPaused && feed.ConsecutiveFailures >= PauseThreshold {
		feed.IsPaused = true
		now := time.Now()
		feed.PausedAt = &now
		log.Printf("[RSSFetcher] auto-paused feed %d (%s) — health_score=%d, consecutive_failures=%d", feed.ID, feed.Name, feed.HealthScore, feed.ConsecutiveFailures)
		s.logActivity("rss_fetch", "health", "auto_pause",
			fmt.Sprintf("Feed %s auto-paused, health_score=%d, failures=%d, reason=%s", feed.Name, feed.HealthScore, feed.ConsecutiveFailures, reason),
			feed.ID, 0)
	}

	s.rssRepo.Update(feed)

	// Schedule retry if not already at max and not paused
	if feed.ConsecutiveFailures <= MaxRetryAttempts && !feed.IsPaused {
		s.scheduleRetry(feed.ID, feed.ConsecutiveFailures-1)
	}
}

// scheduleRetry adds a feed to the retry queue
func (s *RSSFetcherService) scheduleRetry(feedID uint, attempt int) {
	s.retryMu.Lock()
	defer s.retryMu.Unlock()

	delay := retryBackoff(attempt)
	scheduled := time.Now().Add(delay)

	s.retryQueue = append(s.retryQueue, retryItem{
		feedID:    feedID,
		attempt:   attempt,
		scheduled: scheduled,
	})

	log.Printf("[RSSFetcher] scheduled retry for feed %d, attempt %d, delay %v", feedID, attempt, delay)
}

// processRetryQueue checks for feeds that are ready for retry
func (s *RSSFetcherService) processRetryQueue(ctx context.Context) {
	s.retryMu.Lock()
	var ready []retryItem
	var remaining []retryItem

	now := time.Now()
	for _, item := range s.retryQueue {
		if now.After(item.scheduled) || now.Equal(item.scheduled) {
			ready = append(ready, item)
		} else {
			remaining = append(remaining, item)
		}
	}
	s.retryQueue = remaining
	s.retryMu.Unlock()

	for _, item := range ready {
		log.Printf("[RSSFetcher] processing retry for feed %d, attempt %d", item.feedID, item.attempt)
		if err := s.FetchFeed(ctx, item.feedID); err != nil {
			log.Printf("[RSSFetcher] retry attempt %d for feed %d failed: %v", item.attempt, item.feedID, err)
		}
	}
}

// ResumeSource manually resumes a paused source, resetting its health
func (s *RSSFetcherService) ResumeSource(feedID uint) error {
	feed, err := s.rssRepo.GetByID(feedID)
	if err != nil {
		return fmt.Errorf("failed to get feed: %w", err)
	}

	if !feed.IsPaused {
		return fmt.Errorf("feed %d is not paused", feedID)
	}

	feed.IsPaused = false
	feed.PausedAt = nil
	feed.HealthScore = HealthScoreThreshold + 10 // start above threshold
	feed.ConsecutiveFailures = 0
	feed.LastFailureReason = ""

	if err := s.rssRepo.Update(feed); err != nil {
		return fmt.Errorf("failed to update feed: %w", err)
	}

	log.Printf("[RSSFetcher] manually resumed feed %d (%s)", feedID, feed.Name)
	s.logActivity("rss_fetch", "health", "manual_resume",
		fmt.Sprintf("Feed %s manually resumed, health_score set to %d", feed.Name, feed.HealthScore),
		feedID, 0)

	return nil
}

// PauseSource manually pauses a source
func (s *RSSFetcherService) PauseSource(feedID uint) error {
	feed, err := s.rssRepo.GetByID(feedID)
	if err != nil {
		return fmt.Errorf("failed to get feed: %w", err)
	}

	if feed.IsPaused {
		return fmt.Errorf("feed %d is already paused", feedID)
	}

	feed.IsPaused = true
	now := time.Now()
	feed.PausedAt = &now

	if err := s.rssRepo.Update(feed); err != nil {
		return fmt.Errorf("failed to update feed: %w", err)
	}

	log.Printf("[RSSFetcher] manually paused feed %d (%s)", feedID, feed.Name)
	s.logActivity("rss_fetch", "health", "manual_pause",
		fmt.Sprintf("Feed %s manually paused", feed.Name),
		feedID, 0)

	return nil
}

// GetSourceHealthStatus returns health status for all feeds
func (s *RSSFetcherService) GetSourceHealthStatus() ([]SourceHealthStatus, error) {
	feeds, _, err := s.rssRepo.List(1, 1000, "", "", nil)
	if err != nil {
		return nil, err
	}

	statuses := make([]SourceHealthStatus, 0, len(feeds))
	for _, feed := range feeds {
		statuses = append(statuses, SourceHealthStatus{
			FeedID:              feed.ID,
			FeedName:            feed.Name,
			URL:                 feed.URL,
			Category:            feed.Category,
			IsActive:            feed.IsActive,
			IsPaused:            feed.IsPaused,
			HealthScore:         feed.HealthScore,
			ConsecutiveFailures: feed.ConsecutiveFailures,
			LastFailureReason:   feed.LastFailureReason,
			TotalFetched:        feed.TotalFetched,
			TotalFailed:         feed.TotalFailed,
			LastFetchedAt:       feed.LastFetchedAt,
			PausedAt:            feed.PausedAt,
		})
	}
	return statuses, nil
}

// SourceHealthStatus represents health info for a single feed
type SourceHealthStatus struct {
	FeedID              uint       `json:"feed_id"`
	FeedName            string     `json:"feed_name"`
	URL                 string     `json:"url"`
	Category            string     `json:"category"`
	IsActive            bool       `json:"is_active"`
	IsPaused            bool       `json:"is_paused"`
	HealthScore         int        `json:"health_score"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastFailureReason   string     `json:"last_failure_reason,omitempty"`
	TotalFetched        int        `json:"total_fetched"`
	TotalFailed         int        `json:"total_failed"`
	LastFetchedAt       *time.Time `json:"last_fetched_at,omitempty"`
	PausedAt            *time.Time `json:"paused_at,omitempty"`
}

// logActivity logs an activity event if the activity service is available
func (s *RSSFetcherService) logActivity(module, action, status, summary string, feedID uint, durationMs int) {
	if s.activity == nil {
		return
	}
	s.activity.LogActivity(LogActivityParams{
		Module:     module,
		Action:     action,
		Status:     status,
		Summary:    summary,
		RefID:      fmt.Sprintf("rss:%d", feedID),
		DurationMs: durationMs,
	})
}

// FetchAllResult holds the result of fetching all feeds
type FetchAllResult struct {
	TotalFeeds   int               `json:"total_feeds"`
	FetchedFeeds int               `json:"fetched_feeds"`
	SkippedFeeds int               `json:"skipped_feeds"`
	Results      []FetchFeedResult `json:"results"`
}

// FetchAll fetches all active feeds and returns results
func (s *RSSFetcherService) FetchAll(ctx context.Context, force bool) (*FetchAllResult, error) {
	feeds, err := s.rssRepo.GetActive()
	if err != nil {
		return nil, fmt.Errorf("failed to get feeds: %w", err)
	}

	result := &FetchAllResult{
		TotalFeeds: len(feeds),
		Results:    make([]FetchFeedResult, 0, len(feeds)),
	}

	now := time.Now()
	for _, feed := range feeds {
		fetchResult := FetchFeedResult{
			FeedID:   feed.ID,
			FeedName: feed.Name,
		}

		if !force {
			if due, next := isRSSFeedDue(feed, now); !due {
				fetchResult.Skipped = true
				fetchResult.SkippedReason = fmt.Sprintf("next fetch at %s", next.Format(time.RFC3339))
				result.SkippedFeeds++
				result.Results = append(result.Results, fetchResult)
				continue
			}
		}

		internalResult := s.fetchFeed(ctx, &feed)
		fetchResult.ItemsFound = internalResult.ItemsFound
		fetchResult.ItemsNew = internalResult.ItemsNew
		fetchResult.ItemsDup = internalResult.ItemsDup
		if internalResult.Error != nil {
			fetchResult.Error = internalResult.Error.Error()
		}
		result.FetchedFeeds++

		result.Results = append(result.Results, fetchResult)
	}

	return result, nil
}

// GetStatus returns the current status of the RSS fetcher
func (s *RSSFetcherService) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.retryMu.Lock()
	retryCount := len(s.retryQueue)
	s.retryMu.Unlock()

	return map[string]interface{}{
		"running":     s.running,
		"config":      s.cfg,
		"retry_queue": retryCount,
	}
}
