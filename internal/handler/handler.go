package handler

import (
	"github.com/singll/bellkeeper/internal/pkg/defaults"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/singll/bellkeeper/internal/service"
)

// Handlers holds all handler instances
type Handlers struct {
	Tag            *TagHandler
	RSS            *RSSHandler
	Dataset        *DatasetHandler
	Setting        *SettingHandler
	Health         *HealthHandler
	Workflow       *WorkflowHandler
	System         *SystemHandler
	LLMProxy       *LLMProxyHandler
	Classify       *ClassifyHandler
	ActivityLog    *ActivityLogHandler
	LogCenter      *LogCenterHandler
	FileIngestion  *FileIngestionHandler
	LogLevel       *LogLevelHandler
	Config         *ConfigHandler
	TodoTxt        *TodoTxtHandler
	Search         *SearchHandler
	Report         *ReportHandler
	PKBReport      *PKBReportHandler
	Dashboard      *DashboardHandler
	DailyReport    *DailyReportHandler
	Crawler        *CrawlerHandler
	CrawlQueue     *CrawlQueueHandler
	CrawlFailure   *CrawlFailureHandler
	ExtractionRule *ExtractionRuleHandler
	// Optional handlers
	MatrixNotify *MatrixNotifyHandler
	MatrixAdmin  *MatrixAdminHandler
	// Knowledge handler (initialized in main.go)
	Knowledge      *KnowledgeHandler
	KnowledgeFiles *KnowledgeFilesHandler
	// PKB 调方向掌舵面（initialized in app.go，需 vault basePath）
	PKBSteer *PKBSteerHandler
}

// NewHandlers creates all handler instances
func NewHandlers(services *service.Services, repos *repository.Repositories, shutdownChan chan struct{}, apiKey, memosBaseURL, memosAPIToken string) *Handlers {
	h := &Handlers{
		Tag:           NewTagHandler(services.Tag),
		RSS:           NewRSSHandler(services.RSS, services.RSSFetcher),
		Dataset:       NewDatasetHandler(services.Dataset),
		Setting:       NewSettingHandler(services.Setting),
		Health:        NewHealthHandler(services.Health),
		Workflow:      NewWorkflowHandler(services.Workflow),
		System:        NewSystemHandler(shutdownChan),
		LLMProxy:      NewLLMProxyHandler(services.LLMProxy, service.NewPricer(repos.LLMModelPricing), repos.LLMToken, repos.LLMTokenUsage, repos.LLMModelPricing),
		Classify:      NewClassifyHandler(services.Classify),
		ActivityLog:   NewActivityLogHandler(services.ActivityLog),
		LogCenter:     NewLogCenterHandler(services.LogCenter, apiKey),
		FileIngestion: NewFileIngestionHandler(services.FileIngestion),
		LogLevel:      NewLogLevelHandler(),
		Config:        NewConfigHandler(services.LLMProxy),
		TodoTxt:       NewTodoTxtHandler(memosBaseURL, memosAPIToken),
		Search:        NewSearchHandler(services.Search),
		Report:        NewReportHandler(services.Report),
		PKBReport:     NewPKBReportHandler(services.PKBReport),
		Dashboard:     NewDashboardHandler(services.Dashboard),
		DailyReport:   NewDailyReportHandler(services.DailyReport),
		Crawler:       NewCrawlerHandler(services.Crawler),
	}

	// Set crawl queue handler (if available)
	if services.CrawlQueue != nil {
		h.CrawlQueue = NewCrawlQueueHandler(services.CrawlQueue)
	}

	// Set crawl failure handler (if available)
	if services.CrawlFailure != nil {
		h.CrawlFailure = NewCrawlFailureHandler(services.CrawlFailure)
	}

	// Set extraction rule handler (if available)
	if services.RuleOptimizer != nil {
		h.ExtractionRule = NewExtractionRuleHandler(services.RuleOptimizer, repos.CrawlExtractionRule)
	} else if repos.CrawlExtractionRule != nil {
		h.ExtractionRule = NewExtractionRuleHandler(nil, repos.CrawlExtractionRule)
	}

	// Set optional handlers
	if services.Notification != nil {
		h.MatrixNotify = NewMatrixNotifyHandler(services.Notification)
	}
	if services.MatrixAdmin != nil {
		h.MatrixAdmin = NewMatrixAdminHandler(services.MatrixAdmin, defaults.DefaultMatrixDomain)
	}

	return h
}
