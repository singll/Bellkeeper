package router

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/auth"
	"github.com/singll/bellkeeper/internal/handler"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/repository"
)

// Setup configures all routes on the Gin engine.
func Setup(r *gin.Engine, handlers *handler.Handlers, mode string, apiKey string, tokenRepo *repository.LLMTokenRepository) {
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
	registerSettingRoutes(api, handlers.Setting)
	registerWorkflowRoutes(api, handlers.Workflow)
	registerSystemRoutes(api, handlers.System)
	registerLLMProxyRoutes(r, api, handlers.LLMProxy, tokenRepo, apiKey)
	registerClassifyRoutes(api, handlers.Classify)
	registerActivityLogRoutes(api, handlers.ActivityLog)
	registerLogCenterRoutes(api, handlers.LogCenter)
	registerFileIngestionRoutes(api, handlers.FileIngestion)
	registerLogLevelRoutes(api, handlers.LogLevel)
	registerConfigRoutes(api, handlers.Config)
	registerTodoTxtRoutes(api, handlers.TodoTxt)
	registerSearchRoutes(api, handlers.Search)
	registerCrawlerRoutes(api, handlers.Crawler)

	// Crawl queue routes
	if handlers.CrawlQueue != nil {
		registerCrawlQueueRoutes(api, handlers.CrawlQueue)
	}

	// Crawl failure routes
	if handlers.CrawlFailure != nil {
		registerCrawlFailureRoutes(api, handlers.CrawlFailure)
	}

	// Extraction rule routes
	if handlers.ExtractionRule != nil {
		registerExtractionRuleRoutes(api, handlers.ExtractionRule)
	}

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
	// Report routes
	if handlers.Report != nil {
		registerReportRoutes(api, handlers.Report)
	}
	if handlers.PKBReport != nil {
		registerPKBReportRoutes(api, handlers.PKBReport)
	}
	// Dashboard aggregated stats
	if handlers.Dashboard != nil {
		registerDashboardRoutes(api, handlers.Dashboard)
	}
	// Daily report (backend-driven)
	if handlers.DailyReport != nil {
		registerDailyReportRoutes(api, handlers.DailyReport)
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
	api.POST("/rss/validate", h.ValidateFeed)
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

func registerSettingRoutes(api *gin.RouterGroup, h *handler.SettingHandler) {
	api.GET("/settings", h.List)
	api.GET("/settings/:key", h.Get)
	api.PUT("/settings/:key", h.Update)
}

func registerWorkflowRoutes(api *gin.RouterGroup, h *handler.WorkflowHandler) {
	api.GET("/workflows/definitions", h.Definitions)
	api.POST("/workflows/definitions/push-all", h.PushAllDefinitions)
	api.GET("/workflows/definitions/:key", h.Definition)
	api.PUT("/workflows/definitions/:key", h.SaveDefinition)
	api.DELETE("/workflows/definitions/:key", h.DeleteDefinition)
	api.POST("/workflows/definitions/:key/push", h.PushDefinition)
	api.GET("/workflows/status", h.Status)
	api.GET("/workflows/executions", h.Executions)
	api.POST("/workflows/trigger/:name", h.Trigger)
	api.GET("/workflows/:id", h.Get)
	api.POST("/workflows/:id/activate", h.Activate)
	api.POST("/workflows/:id/deactivate", h.Deactivate)
}

func registerSystemRoutes(api *gin.RouterGroup, h *handler.SystemHandler) {
	api.POST("/system/restart", h.Restart)
	api.GET("/system/disk", h.DiskUsage)
	api.GET("/system/containers", h.ContainerList)
	api.POST("/system/containers/:name/restart", h.ContainerRestart)
	api.POST("/system/backup", h.BackupRun)
}

func registerLLMProxyRoutes(r *gin.Engine, api *gin.RouterGroup, h *handler.LLMProxyHandler, tokenRepo *repository.LLMTokenRepository, serverAPIKey string) {
	// Proxy endpoint: registered on the engine (NOT the api group) so it bypasses the
	// global Authelia middleware. External Bearer tokens (sk-bk-*) authenticate via
	// LLMTokenAuth only; the server.api_key bypass is handled inside that middleware.
	proxy := r.Group("/api/llm")
	proxy.Use(auth.LLMTokenAuth(tokenRepo, serverAPIKey))
	proxy.Any("/v1/*path", h.Proxy)

	// Management endpoints: inherit Authelia auth from the parent api group (web UI +
	// internal callers). These must NOT be reachable with a plain LLM token.
	llm := api.Group("/llm")

	// Management endpoints (runtime status)
	llm.GET("/channels/status", h.ChannelsStatus)
	llm.GET("/stats", h.Stats)
	llm.GET("/logs", h.Logs)
	llm.GET("/rate-limit-events", h.RateLimitEvents)
	llm.GET("/alerts", h.ListAlertEvents)

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

	// Channel credentials (encrypted at rest, Tier 6)
	cfg.GET("/channels/:id/credentials", h.ListChannelCredentials)
	cfg.POST("/channels/:id/credentials", h.CreateChannelCredential)
	cfg.PUT("/credentials/:id", h.UpdateChannelCredential)
	cfg.DELETE("/credentials/:id", h.DeleteChannelCredential)

	cfg.GET("/groups", h.ListGroups)
	cfg.POST("/groups", h.CreateGroup)
	cfg.PUT("/groups/:id", h.UpdateGroup)
	cfg.DELETE("/groups/:id", h.DeleteGroup)

	llm.POST("/reload", h.ReloadConfig)

	// Balance
	llm.GET("/channels/:name/balance", h.ChannelBalance)
	llm.GET("/channels/:name/balance/history", h.ChannelBalanceHistory)
	llm.GET("/balances", h.AllBalances)
	llm.POST("/balances/refresh", h.RefreshBalances)

	// Token CRUD
	llm.GET("/tokens", h.ListTokens)
	llm.POST("/tokens", h.CreateToken)
	llm.PUT("/tokens/:id", h.UpdateToken)
	llm.DELETE("/tokens/:id", h.DeleteToken)
	llm.POST("/tokens/:id/regenerate", h.RegenerateTokenKey)
	llm.GET("/tokens/:id/usage", h.GetTokenUsage)

	// Pricing CRUD
	llm.GET("/pricing", h.ListPricing)
	llm.POST("/pricing", h.CreatePricing)
	llm.PUT("/pricing/:id", h.UpdatePricing)
	llm.DELETE("/pricing/:id", h.DeletePricing)
	llm.POST("/pricing/test-calc", h.TestPricingCalc)

	// Conversations (sticky session management)
	llm.GET("/conversations", h.ListConversations)
	llm.DELETE("/conversations/:id", h.DeleteConversation)

	// Usage / Billing
	llm.GET("/usage", h.GetUsage)

	// Rate Limits (adaptive learning)
	llm.GET("/rate-limits", h.ListRateLimits)
	// Both routes share the same first param name (:id) so gin's radix tree accepts
	// them as siblings (a differing name here panics at registration). reset addresses
	// a channel+model pair; lock addresses the same channel id.
	llm.POST("/rate-limits/:id/:model/reset", h.ResetRateLimit)
	llm.POST("/rate-limits/:id/lock", h.LockRateLimit)

	// Coding strategy
	llm.GET("/coding-strategy", h.GetCodingStrategy)
	llm.POST("/coding-strategy", h.SetCodingStrategy)
}

func registerClassifyRoutes(api *gin.RouterGroup, h *handler.ClassifyHandler) {
	api.POST("/classify/article", h.ClassifyArticle)
}

func registerActivityLogRoutes(api *gin.RouterGroup, h *handler.ActivityLogHandler) {
	api.GET("/logs", h.List)
	api.GET("/logs/modules", h.Modules)
	api.GET("/logs/stats", h.Stats)
	api.POST("/logs", h.Create)
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
	admin.PUT("/rooms/:id", h.UpdateRoom)
	admin.DELETE("/rooms/:id", h.DeleteRoom)
	admin.GET("/channels", h.ListChannels)
	admin.POST("/channels", h.CreateChannel)
	admin.PUT("/channels/:name", h.UpdateChannel)
	admin.DELETE("/channels/:name", h.DeleteChannel)
	admin.GET("/commands", h.ListCommands)
	admin.PUT("/commands/:name", h.UpdateCommand)
	admin.GET("/command-logs", h.ListCommandLogs)
	admin.GET("/events", h.GetEventLogs)
	admin.GET("/notifications", h.GetNotificationLogs)
	admin.POST("/notifications/:id/retry", h.RetryNotification)
	admin.GET("/stats", h.GetStats)
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

func registerReportRoutes(api *gin.RouterGroup, h *handler.ReportHandler) {
	reports := api.Group("/reports")
	reports.POST("/write", h.Write)
}

func registerPKBReportRoutes(api *gin.RouterGroup, h *handler.PKBReportHandler) {
	pkb := api.Group("/pkb")
	pkb.GET("/daily", h.Daily)
	pkb.GET("/vault-cards", h.VaultCards)
	pkb.GET("/digests/latest", h.LatestDigests)
}

func registerDashboardRoutes(api *gin.RouterGroup, h *handler.DashboardHandler) {
	api.GET("/dashboard/stats", h.Stats)
}

func registerDailyReportRoutes(api *gin.RouterGroup, h *handler.DailyReportHandler) {
	reports := api.Group("/reports")
	reports.GET("/daily-data", h.DailyData)
	reports.POST("/daily/generate", h.Generate)
	reports.GET("/brief-data", h.BriefData)
	reports.POST("/brief/generate", h.GenerateBrief)
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

func registerCrawlerRoutes(api *gin.RouterGroup, h *handler.CrawlerHandler) {
	crawl := api.Group("/crawl")
	crawl.GET("/sources/health", h.SourceHealth)
	// 批量操作（固定路径必须在 :id 参数路由之前注册，避免路由冲突）
	crawl.POST("/sources/batch/pause", h.BatchPauseSources)
	crawl.POST("/sources/batch/resume", h.BatchResumeSources)
	crawl.POST("/sources/all/pause", h.PauseAllSources)
	crawl.POST("/sources/all/resume", h.ResumeAllSources)
	crawl.POST("/sources/:id/resume", h.ResumeSource)
	crawl.POST("/sources/:id/pause", h.PauseSource)
	crawl.GET("/jobs", h.CrawlJobs)
	crawl.POST("/fetch/:sourceId", h.FetchSource)
}

func registerCrawlQueueRoutes(api *gin.RouterGroup, h *handler.CrawlQueueHandler) {
	queue := api.Group("/crawl/queue")
	queue.GET("/stats", h.Stats)
	queue.GET("/audit", h.Audit)
	queue.GET("/domains", h.Domains)
	queue.GET("/jobs", h.ListJobs)
	queue.POST("/jobs/:id/retry", h.RetryJob)
	queue.GET("/workers", h.Workers)
	queue.GET("/blocked", h.Blocked)
	queue.POST("/blocked/:id/unblock", h.Unblock)
	queue.POST("/enqueue", h.Enqueue)
	queue.POST("/cleanup", h.Cleanup)
}

func registerLogCenterRoutes(api *gin.RouterGroup, h *handler.LogCenterHandler) {
	logs := api.Group("/logs")
	// Entries
	logs.POST("/entries", h.CreateEntry)
	logs.GET("/entries", h.ListEntries)
	logs.GET("/entries/:id", h.GetEntry)
	// Sources
	logs.GET("/sources", h.ListSources)
	logs.POST("/sources", h.RegisterSource)
	logs.PUT("/sources/:id", h.UpdateSource)
	logs.DELETE("/sources/:id", h.DeleteSource)
	// Dashboard
	logs.GET("/dashboard", h.GetDashboard)
	logs.GET("/dashboard/:period", h.GetDashboardByPeriod)
	// Alert rules
	logs.GET("/alerts", h.ListAlertRules)
	logs.POST("/alerts", h.CreateAlertRule)
	logs.PUT("/alerts/:id", h.UpdateAlertRule)
	logs.DELETE("/alerts/:id", h.DeleteAlertRule)
}

func registerExtractionRuleRoutes(api *gin.RouterGroup, h *handler.ExtractionRuleHandler) {
	rules := api.Group("/crawl/rules")
	rules.GET("", h.ListRules)
	rules.POST("", h.CreateRule)
	rules.GET("/domain/:domain", h.GetRule)
	rules.PUT("/:id/status", h.UpdateRuleStatus)
	rules.GET("/:id/trials", h.ListTrials)
}

func registerCrawlFailureRoutes(api *gin.RouterGroup, h *handler.CrawlFailureHandler) {
	failures := api.Group("/crawl/failures")
	failures.GET("", h.List)
	failures.GET("/:id", h.Get)
	failures.POST("/:id/retry", h.Retry)
	failures.POST("/:id/abandon", h.Abandon)
}
