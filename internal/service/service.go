package service

import (
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/repository"
)

// Services holds all service instances
type Services struct {
	Tag        *TagService
	DataSource *DataSourceService
	RSS        *RSSService
	Webhook    *WebhookService
	Dataset    *DatasetService
	Setting    *SettingService
	RagFlow    *RagFlowService
	Health     *HealthService
	Workflow   *WorkflowService
	LLMProxy   *LLMProxyService
}

// NewServices creates all service instances
func NewServices(repos *repository.Repositories, cfg *config.Config, version string) *Services {
	datasetSvc := NewDatasetService(repos.DatasetMapping, repos.Tag)
	ragFlowSvc := NewRagFlowService(cfg.RagFlow, repos.DatasetMapping, repos.Tag)

	// Wire up document verifier: DatasetService can verify documents via RagFlowService
	datasetSvc.SetDocumentVerifier(ragFlowSvc.DocumentExistsInRagFlow)

	return &Services{
		Tag:        NewTagService(repos.Tag),
		DataSource: NewDataSourceService(repos.DataSource, repos.Tag),
		RSS:        NewRSSService(repos.RSS, repos.Tag),
		Webhook:    NewWebhookService(repos.Webhook),
		Dataset:    datasetSvc,
		Setting:    NewSettingService(repos.Setting),
		RagFlow:    ragFlowSvc,
		Health:     NewHealthService(cfg, version, repos.Tag, repos.DataSource, repos.RSS, repos.DatasetMapping),
		Workflow:   NewWorkflowService(cfg.N8N, repos.Setting),
		LLMProxy:   NewLLMProxyService(cfg.LLMProxy, repos.LLMProxy),
	}
}
