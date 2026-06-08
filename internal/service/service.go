package service

import (
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
	CrawlQueue    *CrawlQueueService
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
			Enabled:       cfg.RSSFetcher.Enabled,
			CheckInterval: cfg.RSSFetcher.CheckInterval,
			MaxPerBatch:   cfg.RSSFetcher.MaxPerBatch,
			Timeout:       cfg.RSSFetcher.Timeout,
			RSSHubBaseURL: cfg.RSSFetcher.RSSHubBaseURL,
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
	if cfg.CrawlQueue.Enabled {
		crawlQueueSvc = NewCrawlQueueService(cfg.CrawlQueue, repos.CrawlJob, extractorSvc, fileIngestionSvc, activityLogSvc)
		rssFetcherSvc.SetCrawlQueueService(crawlQueueSvc)
	}

	// Create pricing service
	pricer := NewPricer(repos.LLMModelPricing)
	// Seed default pricing on first run
	_ = pricer.SeedDefaultPricing()

	return &Services{
		Tag:           NewTagService(repos.Tag),
		RSS:           NewRSSService(repos.RSS, repos.Tag),
		RSSFetcher:    rssFetcherSvc,
		Crawler:       crawlSvc,
		Dataset:       datasetSvc,
		Setting:       NewSettingService(repos.Setting),
		Health:        NewHealthService(cfg, version, repos.Tag, repos.RSS, repos.DatasetMapping),
		Workflow:      NewWorkflowService(cfg.N8N, repos.Setting),
		LLMProxy:      NewLLMProxyService(cfg.LLMProxy, repos.LLMProxy, repos.LLMChannel, repos.LLMModelGroup, pricer, repos.LLMTokenUsage, repos.LLMRateLimit, repos.LLMConversationBinding, repos.LLMToken, repos.LLMChannelCredential, repos.LLMChannelBalanceSnapshot),
		LLMJobQueue:   llmJobQueueSvc,
		Classify:      classifySvc,
		ActivityLog:   activityLogSvc,
		LogCenter:     logCenterSvc,
		FileIngestion: fileIngestionSvc,
		Search:        NewSearchService(repos.Tag, repos.ArticleTag, repos.RSS),
		Report:        NewReportService(cfg.FileIngestion.BasePath),
		CrawlQueue:    crawlQueueSvc,
	}
}

// SetNotificationService sets the optional notification service
func (s *Services) SetNotificationService(svc *NotificationService) {
	s.Notification = svc
}

// SetKnowledgeServices sets the knowledge service adapters for Matrix command handlers
func (s *Services) SetKnowledgeServices(searchAdapter *SearchServiceAdapter, askAdapter *AskServiceAdapter) {
	s.KnowledgeSearchAdapter = searchAdapter
	s.KnowledgeAskAdapter = askAdapter
}
