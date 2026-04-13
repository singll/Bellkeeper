package handler

import (
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
	FileIngestion *FileIngestionHandler
	LogLevel      *LogLevelHandler
	Config        *ConfigHandler
	TodoTxt       *TodoTxtHandler
	Search        *SearchHandler
	// Optional handlers
	MatrixNotify *MatrixNotifyHandler
	MatrixAdmin  *MatrixAdminHandler
	// Knowledge handler (initialized in main.go)
	Knowledge *KnowledgeHandler
}

// NewHandlers creates all handler instances
func NewHandlers(services *service.Services, shutdownChan chan struct{}, memosBaseURL, memosAPIToken string) *Handlers {
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
		FileIngestion: NewFileIngestionHandler(services.FileIngestion),
		LogLevel:      NewLogLevelHandler(),
		Config:        NewConfigHandler(services.LLMProxy),
		TodoTxt:       NewTodoTxtHandler(memosBaseURL, memosAPIToken),
		Search:        NewSearchHandler(services.Search),
	}

	// Set optional handlers
	if services.Notification != nil {
		h.MatrixNotify = NewMatrixNotifyHandler(services.Notification)
	}
	if services.MatrixAdmin != nil {
		// Matrix domain from config - default to matrix.singll.net
		matrixDomain := "matrix.singll.net"
		h.MatrixAdmin = NewMatrixAdminHandler(services.MatrixAdmin, matrixDomain)
	}

	return h
}
