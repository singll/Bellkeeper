package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/llmgateway"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type DailyReportService struct {
	health       *HealthService
	dashboard    *DashboardService
	pkbReport    *PKBReportService
	activityRepo *repository.ActivityLogRepository
	crawlJobRepo *repository.CrawlJobRepository
	articleRepo  *repository.ArticleTagRepository
	rssRepo      *repository.RSSRepository
	notify       *NotificationService
	report       *ReportService
	llm          llmgateway.Gateway
	llmJobs      *llmgateway.LLMJobQueueService
	cfg          config.DailyReportConfig
	loc          *time.Location
	// knowledgeBasePath 是 PKB 知识库根（cfg.Knowledge.BasePath），早报同源落资讯库
	// vault/资讯/<date>.md 时锚定此根（与 PKBReportService 读取同一路径）。
	knowledgeBasePath string
}

func NewDailyReportService(
	health *HealthService,
	dashboard *DashboardService,
	pkbReport *PKBReportService,
	activityRepo *repository.ActivityLogRepository,
	crawlJobRepo *repository.CrawlJobRepository,
	articleRepo *repository.ArticleTagRepository,
	rssRepo *repository.RSSRepository,
	notify *NotificationService,
	report *ReportService,
	cfg config.DailyReportConfig,
	llm llmgateway.Gateway,
	llmJobs *llmgateway.LLMJobQueueService,
	knowledgeBasePath string,
) *DailyReportService {
	loc := time.Local
	if cfg.Timezone != "" {
		if loaded, err := time.LoadLocation(cfg.Timezone); err == nil {
			loc = loaded
		}
	}
	return &DailyReportService{
		health:       health,
		dashboard:    dashboard,
		pkbReport:    pkbReport,
		activityRepo: activityRepo,
		crawlJobRepo: crawlJobRepo,
		articleRepo:  articleRepo,
		rssRepo:      rssRepo,
		notify:       notify,
		report:       report,
		llm:          llm,
		llmJobs:      llmJobs,
		cfg:          cfg,
		loc:          loc,

		knowledgeBasePath: knowledgeBasePath,
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
	Date         string               `json:"date"`
	Health       *DetailedHealth      `json:"health,omitempty"`
	Crawl        *CrawlDashboardStats `json:"crawl,omitempty"`
	RSSIngest    *RSSIngestStats      `json:"rss_ingest,omitempty"`
	FileIngest   *FileIngestStats     `json:"file_ingest,omitempty"`
	Classify     *ClassifyStats       `json:"classify,omitempty"`
	PKB          *PKBVaultStats       `json:"pkb,omitempty"`
	PKBCards     []PKBCardSummary     `json:"pkb_cards,omitempty"`
	LLM          *LLMDashboardStats   `json:"llm,omitempty"`
	Failures     []FailureDetail      `json:"failures,omitempty"`
	FeedArchives []PKBFeedArchive     `json:"feed_archives,omitempty"`
	NewsTop      []NewsItem           `json:"news_top,omitempty"`
	AISummary    string               `json:"ai_summary,omitempty"`
	Errors       []CollectError       `json:"errors,omitempty"`
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
		{name: "feed_archives", fn: func(ctx context.Context) (interface{}, error) {
			return s.pkbReport.FeedArchivesByDate(dateStr)
		}},
		{name: "news_top", fn: func(ctx context.Context) (interface{}, error) {
			// 晚间日报只内嵌当日（自 dayStart 起）最新数条资讯链接指回早报，不复述早报全文。
			nd, err := s.collectNewsBetween(dayStart, time.Now().In(s.loc))
			if err != nil {
				return nil, err
			}
			return topNews(nd, 6), nil
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
		case "feed_archives":
			result.FeedArchives = cr.data.([]PKBFeedArchive)
		case "news_top":
			result.NewsTop = cr.data.([]NewsItem)
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
	Data     *DailyReportData `json:"data"`
	Markdown string           `json:"markdown"`
	FilePath string           `json:"file_path,omitempty"`
	Notified bool             `json:"notified"`
	DryRun   bool             `json:"dry_run"`
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
		job, err := s.llmJobs.EnqueueChat(llmgateway.EnqueueLLMChatOptions{
			TaskType:       "summary",
			CallerID:       "daily-report-service",
			Model:          "pool-summary",
			Messages:       messages,
			Temperature:    0.3,
			Priority:       30,
			IdempotencyKey: llmgateway.LLMJobIdempotencyKey("daily-summary", data.Date),
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
			return "", llmgateway.LLMJobTerminalError(done)
		}
		return done.ResponseText, nil
	}

	log.Printf("[DailyReport] llm_jobs not configured, falling back to direct LLM call")
	if s.llm == nil {
		return "", fmt.Errorf("llm client not available")
	}
	summarizeCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	resp, err := s.llm.Chat(summarizeCtx, llmclient.ChatRequest{
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
	return resp.Content, nil
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

type BriefGenerateOptions struct {
	// Date 为空=按滚动窗（默认，用于每日 08:00 推送）；指定 YYYY-MM-DD=以该天 08:00 为窗口右界（回补/测试）。
	Date        string `json:"date,omitempty"`
	WindowHours int    `json:"window_hours,omitempty"` // 取材窗口小时数，默认 24
}

type BriefGenerateResult struct {
	Date      string         `json:"date"`
	Markdown  string         `json:"markdown"`
	News      *NewsBriefData `json:"news"`
	VaultPath string         `json:"vault_path,omitempty"` // 同源落地的资讯库存档路径（空=未落盘/空窗）
}

// briefWindowBoundaryHour 是「6-6 周期」取材窗的固定右界小时（本地时区 06:00）。
const briefWindowBoundaryHour = 6

// computeBriefWindow 计算早报取材时间窗 [start, end)，抽为纯函数便于单测。
// 6-6 周期：右界固定卡在 06:00（不晚于 now 的最近一个 06:00）——每日 08:00 推送即取
// 「昨 06:00 → 今 06:00」的闭合窗（推送时已闭合 2h，结果确定、不随触发分钟漂移）。
// opts.Date 指定 YYYY-MM-DD→右界=该日 06:00（回补/测试历史）。WindowHours 默认 24。
func computeBriefWindow(opts BriefGenerateOptions, now time.Time, loc *time.Location) (start, end time.Time, err error) {
	window := 24 * time.Hour
	if opts.WindowHours > 0 {
		window = time.Duration(opts.WindowHours) * time.Hour
	}
	n := now.In(loc)
	if opts.Date != "" {
		parsed, perr := time.ParseInLocation("2006-01-02", opts.Date, loc)
		if perr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid date format (expected YYYY-MM-DD): %w", perr)
		}
		end = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), briefWindowBoundaryHour, 0, 0, 0, loc)
	} else {
		end = time.Date(n.Year(), n.Month(), n.Day(), briefWindowBoundaryHour, 0, 0, 0, loc)
		if n.Before(end) { // now 早于今日 06:00 → 右界退到昨日 06:00
			end = end.AddDate(0, 0, -1)
		}
	}
	start = end.Add(-window)
	return start, end, nil
}

// GenerateBrief 产出「资讯早报」：滚动时间窗内真实入库的资讯，分领域列出 + LLM 全局总结/看点。
// 与运维日报（Generate）彻底分家——早报讲「今天世界/技术圈发生了什么」，日报讲「本系统今天怎么样」。
func (s *DailyReportService) GenerateBrief(ctx context.Context, opts BriefGenerateOptions) (*BriefGenerateResult, error) {
	start, end, err := computeBriefWindow(opts, time.Now(), s.loc)
	if err != nil {
		return nil, err
	}

	data, err := s.collectNewsBetween(start, end)
	if err != nil {
		return nil, fmt.Errorf("collect news brief: %w", err)
	}
	data.WindowHours = int(end.Sub(start) / time.Hour)

	if data.Total > 0 {
		summary, aiErr := s.generateNewsSummary(ctx, data)
		if aiErr != nil {
			log.Printf("[NewsBrief] AI summary failed: %v", aiErr)
			data.Errors = append(data.Errors, CollectError{Source: "ai_summary", Error: aiErr.Error()})
			data.AISummary = "(AI 总结暂不可用)"
		} else {
			data.AISummary = summary
		}
	}

	markdown := RenderNewsBrief(data)

	result := &BriefGenerateResult{
		Date:     data.Date,
		Markdown: markdown,
		News:     data,
	}

	// 资讯 = 早报：整份早报同源落一份到资讯库 vault/资讯/<date>.md（内容与 Matrix 推送逐字相同）。
	// 空窗不写空存档；写失败非致命——推送在 n8n 下游、独立于本地存档，故存档失败显式记 🔶 日志
	// + 计入 result.Errors（禁止静默降级），但绝不阻断早报生成/推送。
	if data.Total > 0 {
		if p, werr := s.writeBriefArchive(data.Date, markdown, data.Total); werr != nil {
			log.Printf("[NewsBrief] 🔶 资讯库存档写入失败（早报生成/推送不受影响）: %v", werr)
			data.Errors = append(data.Errors, CollectError{Source: "feed_archive", Error: werr.Error()})
		} else {
			result.VaultPath = p
			log.Printf("[NewsBrief] 资讯库存档已写入: %s", p)
		}
	}

	return result, nil
}

// writeBriefArchive 把整份早报 markdown 同源落一份到资讯库 vault/资讯/<date>.md（ADR-0005：资讯库=
// 每日存档；早报即资讯库、同时同内容）。外包 type: pkb_feed frontmatter 供资讯库识别与时间线浏览
// （pkb_report.go FeedArchivesByDate/FeedArchiveHTML；其只按文件存在 + 剥 frontmatter 渲染，不强依赖
// 该 frontmatter）。原子写（tmp+rename），单天重跑整份覆盖（幂等）。锚定 knowledge.base_path，与
// PKBReportService 读取同一根。
func (s *DailyReportService) writeBriefArchive(date, markdown string, itemCount int) (string, error) {
	if strings.TrimSpace(s.knowledgeBasePath) == "" {
		return "", fmt.Errorf("knowledge base path not configured")
	}
	dir := filepath.Join(s.knowledgeBasePath, "vault", "资讯")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir feed dir: %w", err)
	}
	dst := filepath.Join(dir, date+".md")

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("title: 资讯 %s\n", date))
	b.WriteString("type: pkb_feed\n")
	b.WriteString(fmt.Sprintf("date: %s\n", date))
	b.WriteString(fmt.Sprintf("generated_at: %s\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("item_count: %d\n", itemCount))
	b.WriteString("source: news_brief\n")
	b.WriteString("tags: [pkb-feed]\n")
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimRight(markdown, "\n") + "\n")

	tmp := dst + ".tmp.md"
	if err := os.WriteFile(tmp, []byte(b.String()), 0644); err != nil {
		return "", fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("rename: %w", err)
	}
	return dst, nil
}

type BriefReportData struct {
	Date      string               `json:"date"`
	Crawl     *CrawlDashboardStats `json:"crawl,omitempty"`
	RSSIngest *RSSIngestStats      `json:"rss_ingest,omitempty"`
	PKB       *PKBVaultStats       `json:"pkb,omitempty"`
	PKBCards  []PKBCardSummary     `json:"pkb_cards,omitempty"`
}
