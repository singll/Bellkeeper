package router

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/handler"
	"github.com/singll/bellkeeper/internal/middleware"
)

// Setup configures all routes on the Gin engine.
func Setup(r *gin.Engine, handlers *handler.Handlers, mode string) {
	// Health check (no auth required)
	r.GET("/api/health", handlers.Health.Check)
	r.GET("/api/health/detailed", handlers.Health.Detailed)

	// API routes (with Authelia auth)
	api := r.Group("/api")
	api.Use(middleware.AutheliaAuth(mode))

	registerTagRoutes(api, handlers.Tag)
	registerDataSourceRoutes(api, handlers.DataSource)
	registerRSSRoutes(api, handlers.RSS)
	registerWebhookRoutes(api, handlers.Webhook)
	registerDatasetRoutes(api, handlers.Dataset)
	registerRagFlowRoutes(api, handlers.RagFlow)
	registerSettingRoutes(api, handlers.Setting)
	registerWorkflowRoutes(api, handlers.Workflow)
}

func registerTagRoutes(api *gin.RouterGroup, h *handler.TagHandler) {
	api.GET("/tags", h.List)
	api.POST("/tags", h.Create)
	api.GET("/tags/:id", h.Get)
	api.PUT("/tags/:id", h.Update)
	api.DELETE("/tags/:id", h.Delete)
	// 高级端点
	api.GET("/tags/all", h.GetAll)
	api.POST("/tags/batch", h.BatchGetOrCreate)
	api.POST("/tags/match", h.Match)
	api.POST("/tags/by-names", h.GetByNames)
}

func registerDataSourceRoutes(api *gin.RouterGroup, h *handler.DataSourceHandler) {
	api.GET("/datasources", h.List)
	api.POST("/datasources", h.Create)
	api.GET("/datasources/:id", h.Get)
	api.PUT("/datasources/:id", h.Update)
	api.DELETE("/datasources/:id", h.Delete)
}

func registerRSSRoutes(api *gin.RouterGroup, h *handler.RSSHandler) {
	api.GET("/rss", h.List)
	api.POST("/rss", h.Create)
	api.GET("/rss/:id", h.Get)
	api.PUT("/rss/:id", h.Update)
	api.DELETE("/rss/:id", h.Delete)
}

func registerWebhookRoutes(api *gin.RouterGroup, h *handler.WebhookHandler) {
	api.GET("/webhooks", h.List)
	api.POST("/webhooks", h.Create)
	api.GET("/webhooks/:id", h.Get)
	api.PUT("/webhooks/:id", h.Update)
	api.DELETE("/webhooks/:id", h.Delete)
	api.POST("/webhooks/:id/trigger", h.Trigger)
	api.GET("/webhooks/:id/history", h.History)
}

func registerDatasetRoutes(api *gin.RouterGroup, h *handler.DatasetHandler) {
	api.GET("/datasets", h.List)
	api.POST("/datasets", h.Create)
	api.GET("/datasets/:id", h.Get)
	api.PUT("/datasets/:id", h.Update)
	api.DELETE("/datasets/:id", h.Delete)
	// 高级端点
	api.GET("/datasets/all", h.GetAll)
	api.GET("/datasets/by-name/:name", h.GetByName)
	api.POST("/datasets/by-tag", h.RecommendByTag)
	api.POST("/datasets/article-tags", h.AddArticleTags)
	api.GET("/datasets/article-tags/:document_id", h.GetArticleTags)
	api.GET("/datasets/articles-by-tag/:tag_id", h.GetArticlesByTag)
	api.POST("/datasets/check-url", h.CheckURL)
}

func registerRagFlowRoutes(api *gin.RouterGroup, h *handler.RagFlowHandler) {
	api.POST("/ragflow/upload", h.Upload)
	api.POST("/ragflow/upload/with-routing", h.UploadWithRouting)
	api.GET("/ragflow/check-url", h.CheckURL)
	api.GET("/ragflow/documents", h.ListDocuments)
	api.DELETE("/ragflow/documents/:id", h.DeleteDocument)
	// 高级操作
	api.GET("/ragflow/datasets", h.ListDatasets)
	api.GET("/ragflow/datasets/:dataset_id", h.GetDataset)
	api.POST("/ragflow/datasets", h.CreateDataset)
	api.PUT("/ragflow/datasets/:dataset_id", h.UpdateDataset)
	api.DELETE("/ragflow/datasets/:dataset_id", h.DeleteDataset)
	api.POST("/ragflow/documents/parse", h.RunParsing)
	api.POST("/ragflow/documents/parse/stop", h.StopParsing)
	api.GET("/ragflow/documents/parse/status", h.GetParsingStatus)
	api.POST("/ragflow/upload/batch", h.BatchUpload)
	api.POST("/ragflow/documents/batch-delete", h.BatchDeleteDocuments)
	api.POST("/ragflow/documents/transfer", h.TransferDocument)
	api.PUT("/ragflow/documents/metadata", h.UpdateDocumentMetadata)
	api.GET("/ragflow/chunks", h.ListChunks)
	api.DELETE("/ragflow/chunks", h.DeleteChunks)
	api.POST("/ragflow/documents/batch-transfer", h.BatchTransferDocuments)
}

func registerSettingRoutes(api *gin.RouterGroup, h *handler.SettingHandler) {
	api.GET("/settings", h.List)
	api.GET("/settings/:key", h.Get)
	api.PUT("/settings/:key", h.Update)
}

func registerWorkflowRoutes(api *gin.RouterGroup, h *handler.WorkflowHandler) {
	api.GET("/workflows/status", h.Status)
	api.GET("/workflows/:id", h.Get)
	api.POST("/workflows/:id/activate", h.Activate)
	api.POST("/workflows/:id/deactivate", h.Deactivate)
	api.GET("/workflows/executions", h.Executions)
	api.POST("/workflows/trigger/:name", h.Trigger)
}
