package service

import (
	"context"
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/repository"
)

// CrawlService provides crawl management operations that wrap the enhanced RSSFetcherService.
// It serves as the unified entry point for all crawl-related operations, both RSS and future
// source types (URL seeds, API sources, etc.).
type CrawlService struct {
	rssFetcher *RSSFetcherService
	rssRepo    *repository.RSSRepository
	activity   *ActivityLogService
}

// NewCrawlService creates a new crawl service
func NewCrawlService(
	rssFetcher *RSSFetcherService,
	rssRepo *repository.RSSRepository,
	activity *ActivityLogService,
) *CrawlService {
	return &CrawlService{
		rssFetcher: rssFetcher,
		rssRepo:    rssRepo,
		activity:   activity,
	}
}

// FetchSource fetches a single RSS source with health tracking
func (s *CrawlService) FetchSource(ctx context.Context, sourceID uint) (*FetchFeedResult, error) {
	feed, err := s.rssRepo.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get source: %w", err)
	}

	if feed.IsPaused {
		return &FetchFeedResult{
			FeedID:   feed.ID,
			FeedName: feed.Name,
			Error:    "source is paused — resume it first",
		}, nil
	}

	internalResult := s.rssFetcher.FetchFeedInternal(ctx, sourceID)

	// Re-read to get updated stats
	feed, err = s.rssRepo.GetByID(sourceID)
	if err != nil {
		return &FetchFeedResult{
			FeedID:   sourceID,
			FeedName: "",
		}, nil
	}

	result := &FetchFeedResult{
		FeedID:     feed.ID,
		FeedName:   feed.Name,
		ItemsFound: internalResult.ItemsFound,
		ItemsNew:   internalResult.ItemsNew,
		ItemsDup:   internalResult.ItemsDup,
	}
	if internalResult.Error != nil {
		result.Error = internalResult.Error.Error()
	}

	return result, nil
}

// FetchAllActiveSources batch fetches all active (non-paused) RSS sources
func (s *CrawlService) FetchAllActiveSources(ctx context.Context) (*FetchAllResult, error) {
	return s.rssFetcher.FetchAll(ctx, false)
}

// ProcessArticle processes a single RSS item: extract -> dedup -> classify -> ingest.
// This is called internally by RSSFetcherService.fetchFeed, but can also be invoked
// manually for ad-hoc article processing.
func (s *CrawlService) ProcessArticle(ctx context.Context, sourceID uint, url, title string) (*IngestURLResponse, error) {
	if s.rssFetcher.crawlQueue != nil {
		_, err := s.rssFetcher.crawlQueue.Enqueue(sourceID, url, title, "auto", nil)
		if err != nil {
			return nil, fmt.Errorf("failed to enqueue: %w", err)
		}
		return &IngestURLResponse{Status: "queued"}, nil
	}

	return nil, fmt.Errorf("crawl queue not available")
}

// CheckSourceHealth evaluates source health status and returns details
func (s *CrawlService) CheckSourceHealth(sourceID uint) (*SourceHealthStatus, error) {
	feed, err := s.rssRepo.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get source: %w", err)
	}

	return &SourceHealthStatus{
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
	}, nil
}

// ResumeSource manually resumes a paused source
func (s *CrawlService) ResumeSource(sourceID uint) error {
	return s.rssFetcher.ResumeSource(sourceID)
}

// PauseSource manually pauses an active source
func (s *CrawlService) PauseSource(sourceID uint) error {
	return s.rssFetcher.PauseSource(sourceID)
}

// GetSourceHealthStatus lists health scores for all sources
func (s *CrawlService) GetSourceHealthStatus() ([]SourceHealthStatus, error) {
	return s.rssFetcher.GetSourceHealthStatus()
}

// BatchPauseSources 批量暂停源
func (s *CrawlService) BatchPauseSources(ids []uint) (int64, error) {
	affected, err := s.rssRepo.BatchUpdatePaused(ids, true)
	if err != nil {
		return 0, fmt.Errorf("batch pause failed: %w", err)
	}
	s.logActivity("rss_fetch", "batch", "pause",
		fmt.Sprintf("Batch paused %d sources", affected), 0, 0)
	return affected, nil
}

// BatchResumeSources 批量恢复源
func (s *CrawlService) BatchResumeSources(ids []uint) (int64, error) {
	affected, err := s.rssRepo.BatchUpdatePaused(ids, false)
	if err != nil {
		return 0, fmt.Errorf("batch resume failed: %w", err)
	}
	s.logActivity("rss_fetch", "batch", "resume",
		fmt.Sprintf("Batch resumed %d sources", affected), 0, 0)
	return affected, nil
}

// PauseAllSources 暂停所有活跃源
func (s *CrawlService) PauseAllSources() (int64, error) {
	affected, err := s.rssRepo.UpdateAllPaused(true)
	if err != nil {
		return 0, fmt.Errorf("pause all failed: %w", err)
	}
	s.logActivity("rss_fetch", "batch", "pause_all",
		fmt.Sprintf("Paused all %d active sources", affected), 0, 0)
	return affected, nil
}

// ResumeAllSources 恢复所有暂停源
func (s *CrawlService) ResumeAllSources() (int64, error) {
	affected, err := s.rssRepo.UpdateAllPaused(false)
	if err != nil {
		return 0, fmt.Errorf("resume all failed: %w", err)
	}
	s.logActivity("rss_fetch", "batch", "resume_all",
		fmt.Sprintf("Resumed all %d paused sources", affected), 0, 0)
	return affected, nil
}

// GetRecentCrawlJobs returns recent crawl job records from activity logs.
// Since CrawlJob model is for future expansion, we currently derive this
// from activity_logs with module="rss_fetcher" or "crawl".
func (s *CrawlService) GetRecentCrawlJobs(page, limit int) (*ActivityLogsPage, error) {
	if s.activity == nil {
		return nil, fmt.Errorf("activity log service not available")
	}

	return s.activity.List(ListActivityLogsQuery{
		Module: "rss_fetch",
		Page:   page,
		Limit:  limit,
		Since:  time.Now().Add(-24 * time.Hour),
	})
}

// logActivity logs an activity event
func (s *CrawlService) logActivity(module, action, status, summary string, sourceID uint, durationMs int) {
	if s.activity == nil {
		return
	}
	s.activity.LogActivity(LogActivityParams{
		Module:     module,
		Action:     action,
		Status:     status,
		Summary:    summary,
		RefID:      fmt.Sprintf("source:%d", sourceID),
		DurationMs: durationMs,
	})
}
