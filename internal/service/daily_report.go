package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type DailyReportService struct {
	health       *HealthService
	dashboard    *DashboardService
	pkbReport    *PKBReportService
	activityRepo *repository.ActivityLogRepository
	crawlJobRepo *repository.CrawlJobRepository
	notify       *NotificationService
	report       *ReportService
	llmClient    *llmclient.Client
	llmJobs      *LLMJobQueueService
	cfg          config.DailyReportConfig
	loc          *time.Location
}

func NewDailyReportService(
	health *HealthService,
	dashboard *DashboardService,
	pkbReport *PKBReportService,
	activityRepo *repository.ActivityLogRepository,
	crawlJobRepo *repository.CrawlJobRepository,
	notify *NotificationService,
	report *ReportService,
	cfg config.DailyReportConfig,
	llmProxyURL string,
	apiKey string,
	llmJobs *LLMJobQueueService,
) *DailyReportService {
	loc := time.Local
	if cfg.Timezone != "" {
		if loaded, err := time.LoadLocation(cfg.Timezone); err == nil {
			loc = loaded
		}
	}
	var llmClient *llmclient.Client
	if llmProxyURL != "" {
		llmClient = llmclient.New(llmclient.Options{
			BaseURL: llmProxyURL,
			APIKey:  apiKey,
			Timeout: 120 * time.Second,
		})
	}
	return &DailyReportService{
		health:       health,
		dashboard:    dashboard,
		pkbReport:    pkbReport,
		activityRepo: activityRepo,
		crawlJobRepo: crawlJobRepo,
		notify:       notify,
		report:       report,
		llmClient:    llmClient,
		llmJobs:      llmJobs,
		cfg:          cfg,
		loc:          loc,
	}
}

type CollectError struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

type ActionStatEntry struct {
	Action string `json:"action"`
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

type RSSIngestStats struct {
	Success   int64 `json:"success"`
	Duplicate int64 `json:"duplicate"`
	Failure   int64 `json:"failure"`
}

type FileIngestStats struct {
	Success int64 `json:"success"`
	Failure int64 `json:"failure"`
}

type ClassifyStats struct {
	Success int64 `json:"success"`
	Failure int64 `json:"failure"`
}

type FailureDetail struct {
	Summary string `json:"summary"`
	RefID   string `json:"ref_id,omitempty"`
}

type DailyReportData struct {
	Date        string                     `json:"date"`
	Health      *DetailedHealth            `json:"health,omitempty"`
	Crawl       *CrawlDashboardStats       `json:"crawl,omitempty"`
	RSSIngest   *RSSIngestStats            `json:"rss_ingest,omitempty"`
	FileIngest  *FileIngestStats           `json:"file_ingest,omitempty"`
	Classify    *ClassifyStats             `json:"classify,omitempty"`
	PKB         *PKBVaultStats             `json:"pkb,omitempty"`
	PKBCards    []PKBCardSummary           `json:"pkb_cards,omitempty"`
	LLM         *LLMDashboardStats         `json:"llm,omitempty"`
	Failures    []FailureDetail            `json:"failures,omitempty"`
	AISummary   string                     `json:"ai_summary,omitempty"`
	Errors      []CollectError             `json:"errors,omitempty"`
}

type collectorResult struct {
	name string
	data interface{}
	err  error
}

func (s *DailyReportService) Collect(ctx context.Context, date string) (*DailyReportData, error) {
	var dayStart time.Time
	if date != "" {
		parsed, err := time.ParseInLocation("2006-01-02", date, s.loc)
		if err != nil {
			return nil, fmt.Errorf("invalid date format (expected YYYY-MM-DD): %w", err)
		}
		dayStart = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, s.loc)
	} else {
		now := time.Now().In(s.loc)
		dayStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
	}

	dateStr := dayStart.Format("2006-01-02")
	result := &DailyReportData{Date: dateStr}

	type collector struct {
		name string
		fn   func(ctx context.Context) (interface{}, error)
	}

	collectors := []collector{
		{name: "health", fn: func(ctx context.Context) (interface{}, error) {
			return s.health.Detailed(), nil
		}},
		{name: "crawl", fn: func(ctx context.Context) (interface{}, error) {
			stats := &CrawlDashboardStats{}
			if err := s.dashboard.fillCrawl(stats); err != nil {
				return nil, err
			}
			return stats, nil
		}},
		{name: "rss_ingest", fn: func(ctx context.Context) (interface{}, error) {
			return s.collectActionStats("rss_fetch", dayStart)
		}},
		{name: "file_ingest", fn: func(ctx context.Context) (interface{}, error) {
			return s.collectFileIngestStats(dayStart)
		}},
		{name: "classify", fn: func(ctx context.Context) (interface{}, error) {
			return s.collectClassifyStats(dayStart)
		}},
		{name: "pkb", fn: func(ctx context.Context) (interface{}, error) {
			return s.pkbReport.VaultStats()
		}},
		{name: "pkb_cards", fn: func(ctx context.Context) (interface{}, error) {
			cards, err := s.pkbReport.VaultCardsByDate(dateStr, 10)
			if err != nil {
				return nil, err
			}
			return cards, nil
		}},
		{name: "llm", fn: func(ctx context.Context) (interface{}, error) {
			stats := &LLMDashboardStats{}
			if err := s.dashboard.fillLLM(stats); err != nil {
				return nil, err
			}
			return stats, nil
		}},
		{name: "failures", fn: func(ctx context.Context) (interface{}, error) {
			return s.collectFailureDetails(dayStart)
		}},
	}

	ch := make(chan collectorResult, len(collectors))
	var wg sync.WaitGroup

	sem := make(chan struct{}, 4)
	for _, c := range collectors {
		wg.Add(1)
		go func(col collector) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				ch <- collectorResult{name: col.name, data: nil, err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			data, err := col.fn(ctx)
			ch <- collectorResult{name: col.name, data: data, err: err}
		}(c)
	}
	wg.Wait()
	close(ch)

	for cr := range ch {
		if cr.err != nil {
			result.Errors = append(result.Errors, CollectError{
				Source: cr.name,
				Error:  cr.err.Error(),
			})
			continue
		}
		switch cr.name {
		case "health":
			result.Health = cr.data.(*DetailedHealth)
		case "crawl":
			result.Crawl = cr.data.(*CrawlDashboardStats)
		case "rss_ingest":
			rssStats := cr.data.([]ActionStatEntry)
			ingest := &RSSIngestStats{}
			for _, s := range rssStats {
				if s.Action != "ingest" {
					continue
				}
				switch s.Status {
				case "success":
					ingest.Success = s.Count
				case "duplicate":
					ingest.Duplicate = s.Count
				case "failure":
					ingest.Failure = s.Count
				}
			}
			result.RSSIngest = ingest
		case "file_ingest":
			fiStats := cr.data.([]ActionStatEntry)
			fi := &FileIngestStats{}
			for _, s := range fiStats {
				switch s.Status {
				case "success":
					fi.Success = s.Count
				case "failure":
					fi.Failure = s.Count
				}
			}
			result.FileIngest = fi
		case "classify":
			clStats := cr.data.([]ActionStatEntry)
			cl := &ClassifyStats{}
			for _, s := range clStats {
				switch s.Status {
				case "success":
					cl.Success = s.Count
				case "failure":
					cl.Failure = s.Count
				}
			}
			result.Classify = cl
		case "pkb":
			result.PKB = cr.data.(*PKBVaultStats)
		case "pkb_cards":
			result.PKBCards = cr.data.([]PKBCardSummary)
		case "llm":
			result.LLM = cr.data.(*LLMDashboardStats)
		case "failures":
			result.Failures = cr.data.([]FailureDetail)
		}
	}

	return result, nil
}

func (s *DailyReportService) collectActionStats(module string, since time.Time) ([]ActionStatEntry, error) {
	raw, err := s.activityRepo.GetActionStats(module, since)
	if err != nil {
		return nil, fmt.Errorf("get action stats for %s: %w", module, err)
	}
	result := make([]ActionStatEntry, len(raw))
	for i, r := range raw {
		result[i] = ActionStatEntry{Action: r.Action, Status: r.Status, Count: r.Count}
	}
	return result, nil
}

func (s *DailyReportService) collectFileIngestStats(since time.Time) ([]ActionStatEntry, error) {
	return s.collectActionStats("file_ingestion", since)
}

func (s *DailyReportService) collectClassifyStats(since time.Time) ([]ActionStatEntry, error) {
	return s.collectActionStats("classify", since)
}

func (s *DailyReportService) collectFailureDetails(since time.Time) ([]FailureDetail, error) {
	var details []FailureDetail

	for _, module := range []string{"rss_fetch", "crawl"} {
		failures, err := s.activityRepo.GetRecentFailures(module, since, 10)
		if err != nil {
			return nil, fmt.Errorf("get %s failures: %w", module, err)
		}
		for _, f := range failures {
			details = append(details, FailureDetail{Summary: f.Summary, RefID: f.RefID})
		}
	}

	return details, nil
}

type GenerateOptions struct {
	Date       string `json:"date,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
	SkipNotify bool   `json:"skip_notify,omitempty"`
}

type GenerateResult struct {
	Data       *DailyReportData `json:"data"`
	Markdown   string           `json:"markdown"`
	FilePath   string           `json:"file_path,omitempty"`
	Notified   bool             `json:"notified"`
	DryRun     bool             `json:"dry_run"`
}

func (s *DailyReportService) Generate(ctx context.Context, opts GenerateOptions) (*GenerateResult, error) {
	data, err := s.Collect(ctx, opts.Date)
	if err != nil {
		return nil, fmt.Errorf("collect daily report data: %w", err)
	}

	aiSummary, aiErr := s.generateAISummary(ctx, data)
	if aiErr != nil {
		log.Printf("[DailyReport] AI summary failed: %v", aiErr)
		data.Errors = append(data.Errors, CollectError{
			Source: "ai_summary",
			Error:  aiErr.Error(),
		})
		data.AISummary = "(AI总结暂不可用)"
	} else {
		data.AISummary = aiSummary
	}

	markdown := RenderDailyReport(data)

	result := &GenerateResult{
		Data:     data,
		Markdown: markdown,
		DryRun:   opts.DryRun,
	}

	if opts.DryRun {
		return result, nil
	}

	writeResult, err := s.report.WriteMessage(&WriteRequest{
		Channel: "daily",
		Content: markdown,
		Date:    data.Date,
		Source:  "daily_report_service",
	})
	if err != nil {
		return nil, fmt.Errorf("write report file: %w", err)
	}
	result.FilePath = writeResult.FilePath

	if !opts.SkipNotify && s.notify != nil {
		notifyResp, notifyErr := s.notify.Send(ctx, &NotificationRequest{
			Channel:     s.cfg.ReportChannel,
			Message:     markdown,
			MessageType: "markdown",
			Metadata: map[string]string{
				"source": "daily_report_service",
				"date":   data.Date,
			},
		})
		if notifyErr != nil {
			log.Printf("[DailyReport] notify failed: %v", notifyErr)
		} else if notifyResp != nil && notifyResp.Success {
			result.Notified = true
		}
	}

	return result, nil
}

func (s *DailyReportService) generateAISummary(ctx context.Context, data *DailyReportData) (string, error) {
	var topics []string
	for _, card := range data.PKBCards {
		if card.Title != "" {
			topics = append(topics, card.Title)
		}
	}
	if data.Crawl != nil && data.Crawl.TodaySuccess > 0 {
		topics = append(topics, fmt.Sprintf("今日爬取成功 %d 篇文章", data.Crawl.TodaySuccess))
	}

	if len(topics) == 0 {
		return "今日无显著事件。", nil
	}

	prompt := fmt.Sprintf(
		"基于以下今日要点，生成一段简短的中文日报亮点总结（3-5句话，不使用列表）：\n\n%s",
		formatTopicsForPrompt(topics),
	)

	messages := []llmclient.ChatMessage{
		{Role: "user", Content: prompt},
	}

	if s.llmJobs != nil {
		job, err := s.llmJobs.EnqueueChat(EnqueueLLMChatOptions{
			TaskType:       "summary",
			CallerID:       "daily-report-service",
			Model:          "pool-summary",
			Messages:       messages,
			Temperature:    0.3,
			Priority:       30,
			IdempotencyKey: llmJobIdempotencyKey("daily-summary", data.Date),
		})
		if err != nil {
			return "", fmt.Errorf("enqueue llm job: %w", err)
		}
		summarizeCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
		done, err := s.llmJobs.Wait(summarizeCtx, job.ID, time.Second)
		if err != nil {
			return "", fmt.Errorf("wait llm job: %w", err)
		}
		if done.Status != model.LLMJobSuccess {
			return "", LLMJobTerminalError(done)
		}
		return done.ResponseText, nil
	}

	log.Printf("[DailyReport] llm_jobs not configured, falling back to direct LLM call")
	if s.llmClient == nil {
		return "", fmt.Errorf("llm client not available")
	}
	summarizeCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	resp, err := s.llmClient.ChatCompletion(summarizeCtx, llmclient.ChatRequest{
		Model:       "pool-summary",
		Messages:    messages,
		Temperature: 0.3,
	}, llmclient.ChatOptions{
		CallerID: "daily-report-service",
		TaskType: "summary",
	})
	if err != nil {
		return "", fmt.Errorf("llm summarize: %w", err)
	}
	return resp, nil
}

func formatTopicsForPrompt(topics []string) string {
	var result string
	for i, t := range topics {
		if i >= 30 {
			result += "...(更多省略)\n"
			break
		}
		result += fmt.Sprintf("- %s\n", t)
	}
	return result
}

func (s *DailyReportService) CollectBrief(date string) ([]ActionStatEntry, error) {
	var dayStart time.Time
	if date != "" {
		parsed, err := time.ParseInLocation("2006-01-02", date, s.loc)
		if err != nil {
			return nil, fmt.Errorf("invalid date format: %w", err)
		}
		dayStart = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, s.loc)
	} else {
		now := time.Now().In(s.loc)
		dayStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
	}

	return s.collectActionStats("rss_fetch", dayStart)
}

type BriefGenerateOptions struct {
	Date string `json:"date,omitempty"`
}

type BriefGenerateResult struct {
	Date     string           `json:"date"`
	Markdown string           `json:"markdown"`
	Data     *DailyReportData `json:"data"`
}

func (s *DailyReportService) GenerateBrief(ctx context.Context, opts BriefGenerateOptions) (*BriefGenerateResult, error) {
	data, err := s.Collect(ctx, opts.Date)
	if err != nil {
		return nil, fmt.Errorf("collect brief data: %w", err)
	}

	aiSummary, aiErr := s.generateAISummary(ctx, data)
	if aiErr != nil {
		log.Printf("[DailyReport] AI summary failed: %v", aiErr)
		data.Errors = append(data.Errors, CollectError{
			Source: "ai_summary",
			Error:  aiErr.Error(),
		})
		data.AISummary = "(AI总结暂不可用)"
	} else {
		data.AISummary = aiSummary
	}

	markdown := RenderBriefReport(data)

	return &BriefGenerateResult{
		Date:     data.Date,
		Markdown: markdown,
		Data:     data,
	}, nil
}

type BriefReportData struct {
	Date      string              `json:"date"`
	Crawl     *CrawlDashboardStats `json:"crawl,omitempty"`
	RSSIngest *RSSIngestStats      `json:"rss_ingest,omitempty"`
	PKB       *PKBVaultStats       `json:"pkb,omitempty"`
	PKBCards  []PKBCardSummary     `json:"pkb_cards,omitempty"`
}


