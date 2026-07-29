package service

import (
	"github.com/singll/bellkeeper/internal/llmgateway"
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
	LLMProxy      *llmgateway.LLMProxyService
	LLMJobQueue   *llmgateway.LLMJobQueueService
	LLMAdmin      *llmgateway.LLMAdminService
	Classify      *ClassifyService
	ActivityLog   *ActivityLogService
	LogCenter     *LogCenterService
	FileIngestion *FileIngestionService
	Search        *SearchService
	Report        *ReportService
	PKBReport     *PKBReportService
	Dashboard     *DashboardService
	CrawlQueue    *CrawlQueueService
	CrawlFailure  *CrawlFailureService
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

	// LLM Gateway: 构造在所有 LLM 调用方之前（classify/daily_report/rule_optimizer/
	// llm_job_queue 经进程内直调 Gateway 接口，替代原 localhost HTTP 自回环）。
	pricer := llmgateway.NewPricer(repos.LLMModelPricing)
	_ = pricer.SeedDefaultPricing()
	llmGateway := llmgateway.NewLLMProxyService(cfg.LLMProxy, repos.LLMProxy, repos.LLMChannel, repos.LLMModelGroup, pricer, repos.LLMTokenUsage, repos.LLMRateLimit, repos.LLMConversationBinding, repos.LLMToken, repos.LLMChannelCredential, repos.LLMChannelBalanceSnapshot)
	// LLMAdminService 收拢 token/pricing 管理 CRUD + 计费试算（消化分层例外②），
	// 复用 pricer；handler 经此 service 访问，不再直接持有 repository。
	llmAdminSvc := llmgateway.NewLLMAdminService(repos.LLMToken, repos.LLMTokenUsage, repos.LLMModelPricing, pricer)

	llmJobQueueSvc := llmgateway.NewLLMJobQueueService(cfg.LLMJobQueue, repos.LLMJob, llmGateway, nil)

	// Create classify service with activity log
	classifySvc := NewClassifyService(cfg.Classify, llmGateway, activityLogSvc)
	// 1.0 §2.1.3：注入知识库提示词加载器（config/prompts/ + registry.yaml）。
	// 加载失败时 classify 回退到内置默认提示词，不影响启动。
	kbPromptLoader := NewKBPromptLoader("config")
	classifySvc.SetPromptLoader(kbPromptLoader)
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
	)

	// Wire activity log into RSS fetcher
	rssFetcherSvc.SetActivityLogService(activityLogSvc)

	// Create crawl service (wraps RSS fetcher with management operations)
	crawlSvc := NewCrawlService(rssFetcherSvc, repos.RSS, activityLogSvc)

	// Create crawl queue service (if enabled)
	var crawlQueueSvc *CrawlQueueService
	var ruleOptimizerSvc *RuleOptimizerService
	var crawlFailureSvc *CrawlFailureService
	if cfg.CrawlQueue.Enabled {
		crawlQueueSvc = NewCrawlQueueService(cfg.CrawlQueue, repos.CrawlJob, repos.CrawlDomainProfile, repos.CrawlFailure, extractorSvc, fileIngestionSvc, activityLogSvc)
		rssFetcherSvc.SetCrawlQueueService(crawlQueueSvc)
		rssFetcherSvc.SetDomainRepo(repos.CrawlDomainProfile)
		crawlFailureSvc = NewCrawlFailureService(repos.CrawlFailure, repos.CrawlJob)
		if cfg.CrawlQueue.RuleOptimizerEnabled {
			ruleOptimizerSvc = NewRuleOptimizerService(cfg.CrawlQueue, repos.CrawlExtractionRule, repos.CrawlDomainProfile, repos.CrawlFailure, llmGateway, extractorSvc, activityLogSvc)
			ruleOptimizerSvc.SetPromptLoader(kbPromptLoader)
			// The optimizer discovers cooling domains via its own periodic loop
			// (RuleOptimizer.Start); no per-cooling synchronous trigger is wired.
		}
	}

	// Create pricing service — pricing 与 LLM Gateway 已在前文构造（llmGateway 复用），
// 此处仅保留 reportSvc/pkbReportSvc/dashboardSvc/healthSvc 的构造顺序。

	reportSvc := NewReportService(cfg.FileIngestion.BasePath)
	reportSvc.SetDailyReportPath(cfg.DailyReport.Path)

	pkbReportSvc := NewPKBReportService(cfg.Knowledge, cfg.DailyReport, activityLogSvc)
	dashboardSvc := NewDashboardService(repos.CrawlJob, repos.RSS, repos.LLMProxy, pkbReportSvc, cfg.DailyReport)
	healthSvc := NewHealthService(cfg, version, repos.Tag, repos.RSS, repos.DatasetMapping)

	// LLM call rule: batch processing → llm_jobs queue, interactive → Gateway direct call.
	// 进程内直调 Gateway 接口（llmGateway），替代原 localhost HTTP 自回环。
	var dailyReportLLMJobs *llmgateway.LLMJobQueueService
	if cfg.LLMJobQueue.Enabled {
		dailyReportLLMJobs = llmJobQueueSvc
	}
	dailyReportSvc := NewDailyReportService(
		healthSvc,
		dashboardSvc,
		pkbReportSvc,
		repos.ActivityLog,
		repos.CrawlJob,
		repos.ArticleTag,
		repos.RSS,
		nil,
		reportSvc,
		cfg.DailyReport,
		llmGateway,
		dailyReportLLMJobs,
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
		LLMProxy:      llmGateway,
		LLMJobQueue:   llmJobQueueSvc,
		LLMAdmin:      llmAdminSvc,
		Classify:      classifySvc,
		ActivityLog:   activityLogSvc,
		LogCenter:     logCenterSvc,
		FileIngestion: fileIngestionSvc,
		Search:        NewSearchService(repos.Tag, repos.ArticleTag, repos.RSS),
		Report:        reportSvc,
		PKBReport:     pkbReportSvc,
		Dashboard:     dashboardSvc,
		CrawlQueue:    crawlQueueSvc,
		CrawlFailure:  crawlFailureSvc,
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
