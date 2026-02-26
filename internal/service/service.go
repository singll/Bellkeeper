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
}

// NewServices creates all service instances
func NewServices(repos *repository.Repositories, cfg *config.Config, version string) *Services {
	return &Services{
		Tag:        NewTagService(repos.Tag),
		DataSource: NewDataSourceService(repos.DataSource, repos.Tag),
		RSS:        NewRSSService(repos.RSS, repos.Tag),
		Webhook:    NewWebhookService(repos.Webhook),
		Dataset:    NewDatasetService(repos.DatasetMapping, repos.Tag),
		Setting:    NewSettingService(repos.Setting),
		RagFlow:    NewRagFlowService(cfg.RagFlow, repos.DatasetMapping, repos.Tag),
		Health:     NewHealthService(cfg, version, repos.Tag, repos.DataSource, repos.RSS, repos.DatasetMapping),
		Workflow:   NewWorkflowService(cfg.N8N),
	}
}
