package service

import (
	"fmt"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/repository"
)

// Services holds all service instances
type Services struct {
	Tag           *TagService
	RSS           *RSSService
	RSSFetcher    *RSSFetcherService
	Crawler       *CrawlService
	Dataset       *DatasetService
	Setting       *SettingService
	Health        *HealthService
	Workflow      *WorkflowService
	LLMProxy      *LLMProxyService
	LLMJobQueue   *LLMJobQueueService
	Classify      *ClassifyService
	ActivityLog   *ActivityLogService
	LogCenter     *LogCenterService
	FileIngestion *FileIngestionService
	Search        *SearchService
	Report        *ReportService
	PKBReport     *PKBReportService
	Dashboard     *DashboardService
	CrawlQueue    *CrawlQueueService
	RuleOptimizer *RuleOptimizerService
	DailyReport   *DailyReportService
	// Optional services (initialized in main.go with infra dependencies)
	Notification *NotificationService
	MatrixAdmin  *AdminService
	// Knowledge services adapters (for Matrix command handlers)
	KnowledgeSearchAdapter *SearchServiceAdapter
	KnowledgeAskAdapter    *AskServiceAdapter
}

// NewServices creates all service instances
func NewServices(repos *repository.Repositories, cfg *config.Config, version string) *Services {
	// Create activity log first (used by multiple services)
	activityLogSvc := NewActivityLogService(repos.ActivityLog)

	// Create log center service
	logCenterSvc := NewLogCenterService(repos.LogEntry, repos.LogSource, repos.LogAlertRule)

	// Wire LogCenter into ActivityLog for delegation
	activityLogSvc.SetLogCenter(logCenterSvc)

	// Create dataset service
	datasetSvc := NewDatasetService(repos.DatasetMapping, repos.Tag)

	llmJobQueueSvc := NewLLMJobQueueService(cfg.LLMJobQueue, repos.LLMJob, cfg.Classify.LLMProxyURL, cfg.Server.APIKey)

	// Create classify service with activity log
	classifySvc := NewClassifyService(cfg.Classify, cfg.Server.APIKey, activityLogSvc)
	if cfg.LLMJobQueue.Enabled {
		classifySvc.SetLLMJobQueue(llmJobQueueSvc)
	}

	// Create extractor service with activity log
	extractorSvc := NewExtractorService(cfg.FileIngestion, activityLogSvc)

	// Create file ingestion service with all dependencies
	fileIngestionSvc := NewFileIngestionService(
		cfg.FileIngestion,
		extractorSvc,
		datasetSvc,
		classifySvc,
		repos.Tag,
		repos.ArticleTag,
		activityLogSvc,
	)

	// Create RSS fetcher service (depends on file ingestion)
	rssFetcherSvc := NewRSSFetcherService(
		RSSFetcherConfig{
			Enabled:              cfg.RSSFetcher.Enabled,
			CheckInterval:        cfg.RSSFetcher.CheckInterval,
			MaxPerBatch:          cfg.RSSFetcher.MaxPerBatch,
			Timeout:              cfg.RSSFetcher.Timeout,
			RSSHubBaseURL:        cfg.RSSFetcher.RSSHubBaseURL,
			ProbeIntervalMinutes: cfg.RSSFetcher.ProbeIntervalMinutes,
		},
		repos.RSS,
		fileIngestionSvc,
	)

	// Wire activity log into RSS fetcher
	rssFetcherSvc.SetActivityLogService(activityLogSvc)

	// Create crawl service (wraps RSS fetcher with management operations)
	crawlSvc := NewCrawlService(rssFetcherSvc, repos.RSS, activityLogSvc)

	// Create crawl queue service (if enabled)
	var crawlQueueSvc *CrawlQueueService
	var ruleOptimizerSvc *RuleOptimizerService
	if cfg.CrawlQueue.Enabled {
		crawlQueueSvc = NewCrawlQueueService(cfg.CrawlQueue, repos.CrawlJob, repos.CrawlDomainProfile, extractorSvc, fileIngestionSvc, activityLogSvc)
		rssFetcherSvc.SetCrawlQueueService(crawlQueueSvc)
		if cfg.CrawlQueue.RuleOptimizerEnabled {
			ruleOptimizerSvc = NewRuleOptimizerService(cfg.CrawlQueue, repos.CrawlExtractionRule, repos.CrawlJob, cfg.Classify.LLMProxyURL, cfg.Server.APIKey, extractorSvc, activityLogSvc)
		}
	}

	// Create pricing service
	pricer := NewPricer(repos.LLMModelPricing)
	// Seed default pricing on first run
	_ = pricer.SeedDefaultPricing()

	reportSvc := NewReportService(cfg.FileIngestion.BasePath)
	reportSvc.SetDailyReportPath(cfg.DailyReport.Path)

	pkbReportSvc := NewPKBReportService(cfg.Knowledge, cfg.DailyReport, activityLogSvc)
	dashboardSvc := NewDashboardService(repos.CrawlJob, repos.RSS, repos.LLMProxy, pkbReportSvc, cfg.DailyReport)
	healthSvc := NewHealthService(cfg, version, repos.Tag, repos.RSS, repos.DatasetMapping)

	// DEVIATION: 计划要求「直接调用进程内 LLMProxyService(不走 HTTP 回环)」，
	// 实际走 localhost HTTP 回环。原因：复用 llmclient 的重试/超时/日志逻辑，
	// 避免在 DailyReportService 中重复实现 LLM 调用适配层。
	// 如需改为进程内调用，需将 LLMProxyService 的 chatCompletion 方法抽为接口注入。
	llmProxyURL := fmt.Sprintf("http://localhost:%d/api/llm/v1", cfg.Server.Port)
	dailyReportSvc := NewDailyReportService(
		healthSvc,
		dashboardSvc,
		pkbReportSvc,
		repos.ActivityLog,
		repos.CrawlJob,
		nil,
		reportSvc,
		cfg.DailyReport,
		llmProxyURL,
		cfg.Server.APIKey,
	)

	return &Services{
		Tag:           NewTagService(repos.Tag),
		RSS:           NewRSSService(repos.RSS, repos.Tag),
		RSSFetcher:    rssFetcherSvc,
		Crawler:       crawlSvc,
		Dataset:       datasetSvc,
		Setting:       NewSettingService(repos.Setting),
		Health:        healthSvc,
		Workflow:      NewWorkflowService(cfg.N8N, repos.Setting),
		LLMProxy:      NewLLMProxyService(cfg.LLMProxy, repos.LLMProxy, repos.LLMChannel, repos.LLMModelGroup, pricer, repos.LLMTokenUsage, repos.LLMRateLimit, repos.LLMConversationBinding, repos.LLMToken, repos.LLMChannelCredential, repos.LLMChannelBalanceSnapshot),
		LLMJobQueue:   llmJobQueueSvc,
		Classify:      classifySvc,
		ActivityLog:   activityLogSvc,
		LogCenter:     logCenterSvc,
		FileIngestion: fileIngestionSvc,
		Search:        NewSearchService(repos.Tag, repos.ArticleTag, repos.RSS),
		Report:        reportSvc,
		PKBReport:     pkbReportSvc,
		Dashboard:     dashboardSvc,
		CrawlQueue:    crawlQueueSvc,
		RuleOptimizer: ruleOptimizerSvc,
		DailyReport:   dailyReportSvc,
	}
}

// SetNotificationService sets the optional notification service
func (s *Services) SetNotificationService(svc *NotificationService) {
	s.Notification = svc
	if s.DailyReport != nil {
		s.DailyReport.notify = svc
	}
}

// SetKnowledgeServices sets the knowledge service adapters for Matrix command handlers
func (s *Services) SetKnowledgeServices(searchAdapter *SearchServiceAdapter, askAdapter *AskServiceAdapter) {
	s.KnowledgeSearchAdapter = searchAdapter
	s.KnowledgeAskAdapter = askAdapter
}
