package handler

import (
	"github.com/singll/bellkeeper/internal/service"
)

// Handlers holds all handler instances
type Handlers struct {
	Tag        *TagHandler
	DataSource *DataSourceHandler
	RSS        *RSSHandler
	Webhook    *WebhookHandler
	Dataset    *DatasetHandler
	Setting    *SettingHandler
	RagFlow    *RagFlowHandler
	Health     *HealthHandler
	Workflow   *WorkflowHandler
}

// NewHandlers creates all handler instances
func NewHandlers(services *service.Services) *Handlers {
	return &Handlers{
		Tag:        NewTagHandler(services.Tag),
		DataSource: NewDataSourceHandler(services.DataSource),
		RSS:        NewRSSHandler(services.RSS),
		Webhook:    NewWebhookHandler(services.Webhook),
		Dataset:    NewDatasetHandler(services.Dataset),
		Setting:    NewSettingHandler(services.Setting),
		RagFlow:    NewRagFlowHandler(services.RagFlow),
		Health:     NewHealthHandler(services.Health),
		Workflow:   NewWorkflowHandler(services.Workflow),
	}
}
