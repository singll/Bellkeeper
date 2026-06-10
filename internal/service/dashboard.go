package service

import (
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/repository"
)

// DashboardService aggregates key metrics from crawl, PKB and LLM modules
// for the home dashboard.
type DashboardService struct {
	crawlJobRepo *repository.CrawlJobRepository
	rssRepo      *repository.RSSRepository
	llmProxyRepo *repository.LLMProxyRepository
	pkbReport    *PKBReportService
	loc          *time.Location
}

// NewDashboardService creates a dashboard service.
func NewDashboardService(
	crawlJobRepo *repository.CrawlJobRepository,
	rssRepo *repository.RSSRepository,
	llmProxyRepo *repository.LLMProxyRepository,
	pkbReport *PKBReportService,
	dailyCfg config.DailyReportConfig,
) *DashboardService {
	loc := time.Local
	if dailyCfg.Timezone != "" {
		if loaded, err := time.LoadLocation(dailyCfg.Timezone); err == nil {
			loc = loaded
		}
	}
	return &DashboardService{
		crawlJobRepo: crawlJobRepo,
		rssRepo:      rssRepo,
		llmProxyRepo: llmProxyRepo,
		pkbReport:    pkbReport,
		loc:          loc,
	}
}

// CrawlDashboardStats summarizes crawl pipeline state for the dashboard.
type CrawlDashboardStats struct {
	TotalURLs    int64      `json:"total_urls"`
	Success      int64      `json:"success"`
	Failed       int64      `json:"failed"` // failed + blocked + dead
	Pending      int64      `json:"pending"`
	Running      int64      `json:"running"`
	SuccessRate  float64    `json:"success_rate"` // success / terminal jobs
	TodayNew     int64      `json:"today_new"`
	TodaySuccess int64      `json:"today_success"`
	TodayFailed  int64      `json:"today_failed"`
	TodayRate    float64    `json:"today_rate"`
	FeedsTotal   int64      `json:"feeds_total"`
	FeedsActive  int64      `json:"feeds_active"`
	FeedsPaused  int64      `json:"feeds_paused"`
	LastCrawlAt  *time.Time `json:"last_crawl_at,omitempty"`
}

// LLMDashboardStats summarizes LLM proxy usage for the dashboard.
type LLMDashboardStats struct {
	Requests24h   int64   `json:"requests_24h"`
	Errors24h     int64   `json:"errors_24h"`
	RateLimits24h int64   `json:"rate_limits_24h"`
	SuccessRate   float64 `json:"success_rate"`
	Tokens24h     int64   `json:"tokens_24h"`
	CostCents24h  int64   `json:"cost_cents_24h"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

// DashboardStats is the aggregated payload for GET /api/dashboard/stats.
type DashboardStats struct {
	Crawl CrawlDashboardStats `json:"crawl"`
	PKB   PKBVaultStats       `json:"pkb"`
	LLM   LLMDashboardStats   `json:"llm"`
}

// Stats collects metrics from all modules. Crawl queue stats may be absent
// when the crawl queue is disabled; in that case only feed counts are filled.
func (s *DashboardService) Stats() (*DashboardStats, error) {
	out := &DashboardStats{}

	if err := s.fillCrawl(&out.Crawl); err != nil {
		return nil, fmt.Errorf("collect crawl stats: %w", err)
	}

	vault, err := s.pkbReport.VaultStats()
	if err != nil {
		return nil, fmt.Errorf("collect pkb stats: %w", err)
	}
	out.PKB = *vault

	if err := s.fillLLM(&out.LLM); err != nil {
		return nil, fmt.Errorf("collect llm stats: %w", err)
	}
	return out, nil
}

func (s *DashboardService) fillCrawl(out *CrawlDashboardStats) error {
	queue, err := s.crawlJobRepo.Stats()
	if err != nil {
		return fmt.Errorf("queue stats: %w", err)
	}
	out.TotalURLs = queue.Total
	out.Success = queue.Success
	out.Failed = queue.Failed + queue.Blocked + queue.Dead
	out.Pending = queue.Pending + queue.Retrying
	out.Running = queue.Running
	if terminal := queue.Success + queue.Failed + queue.Blocked + queue.Dead + queue.Skipped; terminal > 0 {
		out.SuccessRate = float64(queue.Success) / float64(terminal)
	}

	now := time.Now().In(s.loc)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
	today, err := s.crawlJobRepo.ActivitySince(dayStart)
	if err != nil {
		return fmt.Errorf("today activity: %w", err)
	}
	out.TodayNew = today.Created
	out.TodaySuccess = today.Success
	out.TodayFailed = today.Failed
	if done := today.Success + today.Failed; done > 0 {
		out.TodayRate = float64(today.Success) / float64(done)
	}

	lastAt, err := s.crawlJobRepo.LastSuccessAt()
	if err != nil {
		return fmt.Errorf("last success: %w", err)
	}
	out.LastCrawlAt = lastAt

	feeds, err := s.rssRepo.Counts()
	if err != nil {
		return fmt.Errorf("feed counts: %w", err)
	}
	out.FeedsTotal = feeds.Total
	out.FeedsActive = feeds.Active
	out.FeedsPaused = feeds.Paused
	return nil
}

func (s *DashboardService) fillLLM(out *LLMDashboardStats) error {
	since := time.Now().Add(-24 * time.Hour)
	summary, err := s.llmProxyRepo.SummarySince(since)
	if err != nil {
		return fmt.Errorf("usage summary: %w", err)
	}
	out.Requests24h = summary.TotalRequests
	out.Errors24h = summary.ErrorCount
	out.RateLimits24h = summary.RateLimits
	out.Tokens24h = summary.PromptTokens + summary.CompTokens
	out.CostCents24h = summary.CostCents
	out.AvgDurationMs = summary.AvgDurationMs
	if summary.TotalRequests > 0 {
		out.SuccessRate = float64(summary.TotalRequests-summary.ErrorCount) / float64(summary.TotalRequests)
	}
	return nil
}
