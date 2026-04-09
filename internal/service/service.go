package service

import (
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/repository"
)

// Services holds all service instances
type Services struct {
	Tag           *TagService
	RSS           *RSSService
	Dataset       *DatasetService
	Setting       *SettingService
	RagFlow       *RagFlowService
	Health        *HealthService
	Workflow      *WorkflowService
	LLMProxy      *LLMProxyService
	Classify      *ClassifyService
	ActivityLog   *ActivityLogService
	FileIngestion *FileIngestionService
}

// NewServices creates all service instances
func NewServices(repos *repository.Repositories, cfg *config.Config, version string) *Services {
	datasetSvc := NewDatasetService(repos.DatasetMapping, repos.Tag)
	ragFlowSvc := NewRagFlowService(cfg.RagFlow, repos.DatasetMapping, repos.Tag)
	activityLogSvc := NewActivityLogService(repos.ActivityLog)

	// Wire up document verifier: DatasetService can verify documents via RagFlowService
	datasetSvc.SetDocumentVerifier(ragFlowSvc.DocumentExistsInRagFlow)

	// Wire up activity log for instrumentation
	ragFlowSvc.SetActivityLog(activityLogSvc)

	classifySvc := NewClassifyService(cfg.Classify, cfg.Server.APIKey)
	classifySvc.SetActivityLog(activityLogSvc)

	// File ingestion services
	extractorSvc := NewExtractorService(cfg.FileIngestion)
	extractorSvc.SetActivityLog(activityLogSvc)

	fileIngestionSvc := NewFileIngestionService(
		cfg.FileIngestion,
		extractorSvc,
		datasetSvc,
		classifySvc,
		repos.Tag,
		repos.ArticleTag,
	)
	fileIngestionSvc.SetActivityLog(activityLogSvc)

	return &Services{
		Tag:           NewTagService(repos.Tag),
		RSS:           NewRSSService(repos.RSS, repos.Tag),
		Dataset:       datasetSvc,
		Setting:       NewSettingService(repos.Setting),
		RagFlow:       ragFlowSvc,
		Health:        NewHealthService(cfg, version, repos.Tag, repos.RSS, repos.DatasetMapping),
		Workflow:      NewWorkflowService(cfg.N8N, repos.Setting),
		LLMProxy:      NewLLMProxyService(cfg.LLMProxy, repos.LLMProxy, repos.LLMChannel, repos.LLMModelGroup),
		Classify:      classifySvc,
		ActivityLog:   activityLogSvc,
		FileIngestion: fileIngestionSvc,
	}
}
