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
	// Optional handlers
	MatrixNotify *MatrixNotifyHandler
}

// NewHandlers creates all handler instances
func NewHandlers(services *service.Services, shutdownChan chan struct{}) *Handlers {
	h := &Handlers{
		Tag:           NewTagHandler(services.Tag),
		RSS:           NewRSSHandler(services.RSS),
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
	}

	// Set optional handlers
	if services.Notification != nil {
		h.MatrixNotify = NewMatrixNotifyHandler(services.Notification)
	}

	return h
}
