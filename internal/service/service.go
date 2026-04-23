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
	RagFlow       *RagFlowService
	Health        *HealthService
	Workflow      *WorkflowService
	LLMProxy      *LLMProxyService
	Classify      *ClassifyService
	ActivityLog   *ActivityLogService
	LogCenter     *LogCenterService
	FileIngestion *FileIngestionService
	Search        *SearchService
	Report        *ReportService
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

	// Create ragflow service with activity log
	ragFlowSvc := NewRagFlowService(cfg.RagFlow, repos.DatasetMapping, repos.Tag, activityLogSvc)

	// Create dataset service with document verifier
	datasetSvc := NewDatasetService(repos.DatasetMapping, repos.Tag, ragFlowSvc.DocumentExistsInRagFlow)

	// Create classify service with activity log
	classifySvc := NewClassifyService(cfg.Classify, cfg.Server.APIKey, activityLogSvc)

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
		},
		repos.RSS,
		fileIngestionSvc,
	)

	// Wire activity log into RSS fetcher
	rssFetcherSvc.SetActivityLogService(activityLogSvc)

	// Create crawl service (wraps RSS fetcher with management operations)
	crawlSvc := NewCrawlService(rssFetcherSvc, repos.RSS, activityLogSvc)

	return &Services{
		Tag:            NewTagService(repos.Tag),
		RSS:            NewRSSService(repos.RSS, repos.Tag),
		RSSFetcher:     rssFetcherSvc,
		Crawler:        crawlSvc,
		Dataset:        datasetSvc,
		Setting:        NewSettingService(repos.Setting),
		RagFlow:        ragFlowSvc,
		Health:         NewHealthService(cfg, version, repos.Tag, repos.RSS, repos.DatasetMapping),
		Workflow:       NewWorkflowService(cfg.N8N, repos.Setting),
		LLMProxy:       NewLLMProxyService(cfg.LLMProxy, repos.LLMProxy, repos.LLMChannel, repos.LLMModelGroup),
		Classify:       classifySvc,
		ActivityLog:    activityLogSvc,
		LogCenter:      logCenterSvc,
		FileIngestion:  fileIngestionSvc,
		Search:          NewSearchService(repos.Tag, repos.ArticleTag, repos.RSS),
		Report:          NewReportService(cfg.FileIngestion.BasePath),
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
