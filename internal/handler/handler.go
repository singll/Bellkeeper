package handler

import (
	"github.com/singll/bellkeeper/internal/pkg/defaults"
	"github.com/singll/bellkeeper/internal/service"
)

// Handlers holds all handler instances
type Handlers struct {
	Tag           *TagHandler
	RSS           *RSSHandler
	Dataset       *DatasetHandler
	Setting       *SettingHandler
	RagFlow       *RagFlowHandler
	Health        *HealthHandler
	Workflow      *WorkflowHandler
	System        *SystemHandler
	LLMProxy      *LLMProxyHandler
	Classify      *ClassifyHandler
	ActivityLog   *ActivityLogHandler
	LogCenter     *LogCenterHandler
	FileIngestion *FileIngestionHandler
	LogLevel      *LogLevelHandler
	Config        *ConfigHandler
	TodoTxt       *TodoTxtHandler
	Search        *SearchHandler
	Report        *ReportHandler
	Crawler       *CrawlerHandler
	// Optional handlers
	MatrixNotify *MatrixNotifyHandler
	MatrixAdmin  *MatrixAdminHandler
	// Knowledge handler (initialized in main.go)
	Knowledge        *KnowledgeHandler
	KnowledgeFiles   *KnowledgeFilesHandler
}

// NewHandlers creates all handler instances
func NewHandlers(services *service.Services, shutdownChan chan struct{}, apiKey, memosBaseURL, memosAPIToken string) *Handlers {
	h := &Handlers{
		Tag:           NewTagHandler(services.Tag),
		RSS:           NewRSSHandler(services.RSS, services.RSSFetcher),
		Dataset:       NewDatasetHandler(services.Dataset),
		Setting:       NewSettingHandler(services.Setting),
		RagFlow:       NewRagFlowHandler(services.RagFlow),
		Health:        NewHealthHandler(services.Health),
		Workflow:      NewWorkflowHandler(services.Workflow),
		System:        NewSystemHandler(shutdownChan),
		LLMProxy:      NewLLMProxyHandler(services.LLMProxy),
		Classify:      NewClassifyHandler(services.Classify),
		ActivityLog:   NewActivityLogHandler(services.ActivityLog),
		LogCenter:     NewLogCenterHandler(services.LogCenter, apiKey),
		FileIngestion: NewFileIngestionHandler(services.FileIngestion),
		LogLevel:      NewLogLevelHandler(),
		Config:        NewConfigHandler(services.LLMProxy),
		TodoTxt:       NewTodoTxtHandler(memosBaseURL, memosAPIToken),
		Search:        NewSearchHandler(services.Search),
		Report:        NewReportHandler(services.Report),
		Crawler:       NewCrawlerHandler(services.Crawler),
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
