package router

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/handler"
	"github.com/singll/bellkeeper/internal/middleware"
)

// Setup configures all routes on the Gin engine.
func Setup(r *gin.Engine, handlers *handler.Handlers, mode string, apiKey string) {
	// Health check (no auth required)
	r.GET("/api/health", handlers.Health.Check)
	r.GET("/api/health/detailed", handlers.Health.Detailed)
	r.GET("/api/health/live", handlers.Health.Liveness)
	r.GET("/api/health/ready", handlers.Health.Readiness)

	// Prometheus metrics endpoint (no auth required)
	r.GET("/metrics", handler.MetricsHandler())

	// API routes (with Authelia auth + API Key support)
	api := r.Group("/api")
	api.Use(middleware.AutheliaAuth(mode, apiKey))

	registerTagRoutes(api, handlers.Tag)
	registerRSSRoutes(api, handlers.RSS)
	registerDatasetRoutes(api, handlers.Dataset)
	registerRagFlowRoutes(api, handlers.RagFlow)
	registerSettingRoutes(api, handlers.Setting)
	registerWorkflowRoutes(api, handlers.Workflow)
	registerSystemRoutes(api, handlers.System)
	registerLLMProxyRoutes(api, handlers.LLMProxy)
	registerClassifyRoutes(api, handlers.Classify)
	registerActivityLogRoutes(api, handlers.ActivityLog)
	registerFileIngestionRoutes(api, handlers.FileIngestion)
	registerLogLevelRoutes(api, handlers.LogLevel)
	registerConfigRoutes(api, handlers.Config)
	registerTodoTxtRoutes(api, handlers.TodoTxt)
	registerSearchRoutes(api, handlers.Search)

	// Matrix notification routes
	if handlers.MatrixNotify != nil {
		registerMatrixNotifyRoutes(api, handlers.MatrixNotify)
	}
	if handlers.MatrixAdmin != nil {
		registerMatrixAdminRoutes(api, handlers.MatrixAdmin)
	}
	// Knowledge routes
	if handlers.Knowledge != nil {
		registerKnowledgeRoutes(api, handlers.Knowledge)
	}
	// Knowledge Files routes (file browser)
	if handlers.KnowledgeFiles != nil {
		registerKnowledgeFilesRoutes(api, handlers.KnowledgeFiles)
	}
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

func registerRSSRoutes(api *gin.RouterGroup, h *handler.RSSHandler) {
	api.GET("/rss", h.List)
	api.POST("/rss", h.Create)
	api.GET("/rss/:id", h.Get)
	api.PUT("/rss/:id", h.Update)
	api.DELETE("/rss/:id", h.Delete)
	// RSS Fetcher endpoints
	api.POST("/rss/fetch/:id", h.Fetch)
	api.POST("/rss/fetch-all", h.FetchAll)
	api.GET("/rss/fetch-status", h.FetchStatus)
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
	api.POST("/ragflow/ingest/obsidian", h.IngestObsidianNote)
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
	api.POST("/ragflow/documents/parse/throttled", h.RunParsingThrottled)
	api.POST("/ragflow/documents/parse/smart", h.RunParsingSmart)
	api.GET("/ragflow/documents/parse/smart/:task_id", h.GetParseTaskStatus)
	api.GET("/ragflow/documents/parse/tasks", h.ListParseTasks)
	api.POST("/ragflow/documents/parse/stop", h.StopParsing)
	api.GET("/ragflow/documents/parse/status", h.GetParsingStatus)
	api.POST("/ragflow/upload/batch", h.BatchUpload)
	api.POST("/ragflow/documents/batch-delete", h.BatchDeleteDocuments)
	api.POST("/ragflow/documents/transfer", h.TransferDocument)
	api.PUT("/ragflow/documents/metadata", h.UpdateDocumentMetadata)
	api.PUT("/ragflow/documents/parser-config", h.UpdateDocumentParserConfig)
	api.GET("/ragflow/chunks", h.ListChunks)
	api.DELETE("/ragflow/chunks", h.DeleteChunks)
	api.POST("/ragflow/documents/batch-transfer", h.BatchTransferDocuments)
	api.POST("/ragflow/datasets/sync", h.SyncDatasets)
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

func registerSystemRoutes(api *gin.RouterGroup, h *handler.SystemHandler) {
	api.POST("/system/restart", h.Restart)
}

func registerLLMProxyRoutes(api *gin.RouterGroup, h *handler.LLMProxyHandler) {
	// OpenAI-compatible proxy endpoint
	llm := api.Group("/llm")
	llm.Any("/v1/*path", h.Proxy)

	// Management endpoints (runtime status)
	llm.GET("/channels/status", h.ChannelsStatus)
	llm.GET("/stats", h.Stats)
	llm.GET("/logs", h.Logs)
	llm.GET("/rate-limit-events", h.RateLimitEvents)

	// Health & model group management
	llm.GET("/health", h.HealthStatus)
	llm.GET("/groups/status", h.GroupsStatus)
	llm.DELETE("/groups/:name/sticky", h.ClearGroupSticky)
	llm.POST("/channels/:name/reset", h.ResetChannelCircuit)

	// Config CRUD (DB-backed, under /config/ to avoid wildcard conflicts)
	cfg := llm.Group("/config")
	cfg.GET("/channels", h.ListChannels)
	cfg.POST("/channels", h.CreateChannel)
	cfg.PUT("/channels/:id", h.UpdateChannel)
	cfg.DELETE("/channels/:id", h.DeleteChannel)

	cfg.GET("/groups", h.ListGroups)
	cfg.POST("/groups", h.CreateGroup)
	cfg.PUT("/groups/:id", h.UpdateGroup)
	cfg.DELETE("/groups/:id", h.DeleteGroup)

	llm.POST("/reload", h.ReloadConfig)
}

func registerClassifyRoutes(api *gin.RouterGroup, h *handler.ClassifyHandler) {
	api.POST("/classify/article", h.ClassifyArticle)
}

func registerActivityLogRoutes(api *gin.RouterGroup, h *handler.ActivityLogHandler) {
	api.GET("/logs", h.List)
	api.GET("/logs/modules", h.Modules)
	api.GET("/logs/stats", h.Stats)
}

func registerFileIngestionRoutes(api *gin.RouterGroup, h *handler.FileIngestionHandler) {
	files := api.Group("/files")
	files.POST("/ingest/url", h.IngestURL)
	files.GET("/metadata/:id", h.GetMetadata)
	files.GET("/list", h.List)
}

func registerMatrixNotifyRoutes(api *gin.RouterGroup, h *handler.MatrixNotifyHandler) {
	matrix := api.Group("/matrix")
	matrix.POST("/notify", h.Send)
	matrix.GET("/notify/:id", h.GetStatus)
	matrix.GET("/notify/channels", h.ListChannels)
	matrix.POST("/notify/channels/reload", h.ReloadChannels)
}

func registerMatrixAdminRoutes(api *gin.RouterGroup, h *handler.MatrixAdminHandler) {
	admin := api.Group("/matrix/admin")
	admin.GET("/rooms", h.ListRooms)
	admin.POST("/rooms", h.CreateRoom)
	admin.DELETE("/rooms/:id", h.DeleteRoom)
	admin.GET("/channels", h.ListChannels)
	admin.PUT("/channels/:name", h.UpdateChannel)
	admin.GET("/commands", h.ListCommands)
	admin.GET("/command-logs", h.ListCommandLogs)
	admin.GET("/events", h.GetEventLogs)
	admin.GET("/notifications", h.GetNotificationLogs)
	admin.GET("/stats", h.GetStats)
	// User roles
	admin.GET("/roles", h.ListUserRoles)
	admin.GET("/roles/:user_id", h.GetUserRole)
	admin.POST("/roles", h.SetUserRole)
	admin.DELETE("/roles/:user_id", h.RemoveUserRole)
}

func registerLogLevelRoutes(api *gin.RouterGroup, h *handler.LogLevelHandler) {
	api.GET("/logging/level", h.GetLevel)
	api.PUT("/logging/level", h.SetLevel)
}

func registerConfigRoutes(api *gin.RouterGroup, h *handler.ConfigHandler) {
	api.POST("/config/reload", h.ReloadAll)
	api.POST("/config/reload/llm-proxy", h.ReloadLLMProxy)
}

func registerTodoTxtRoutes(api *gin.RouterGroup, h *handler.TodoTxtHandler) {
	todos := api.Group("/todos")
	todos.GET("/export", h.Export)
	todos.GET("/export/plain", h.ExportPlain)
}

func registerSearchRoutes(api *gin.RouterGroup, h *handler.SearchHandler) {
	api.GET("/search", h.Search)
}

func registerKnowledgeRoutes(api *gin.RouterGroup, h *handler.KnowledgeHandler) {
	files := api.Group("/files")
	files.POST("/search", h.Search)
	files.POST("/ask", h.Ask)
	files.GET("/stats", h.Stats)
	files.POST("/rebuild", h.Rebuild)
	files.GET("/health", h.Health)
}

func registerKnowledgeFilesRoutes(api *gin.RouterGroup, h *handler.KnowledgeFilesHandler) {
	knowledge := api.Group("/knowledge/files")
	knowledge.GET("/tree", h.GetTree)
	knowledge.GET("/list", h.ListFiles)
	knowledge.GET("/read", h.ReadFile)
	knowledge.GET("/stats", h.GetStats)
	knowledge.GET("/search", h.SearchFiles)
}
