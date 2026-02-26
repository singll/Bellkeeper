package main

import (
	"fmt"
	"log"
	"os"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/handler"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/singll/bellkeeper/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	version = "1.0.0"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "bellkeeper",
		Short: "Bellkeeper - Knowledge Management System",
		Long:  `Bellkeeper is a knowledge management system for collecting, organizing, and retrieving information.`,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config/bellkeeper.yaml)")

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Bellkeeper server",
		Run:   runServer,
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Bellkeeper version %s\n", version)
		},
	}

	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		Run:   runMigrate,
	}

	rootCmd.AddCommand(serveCmd, versionCmd, migrateCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	db, err := model.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Run auto-migration and seed defaults
	if err := model.AutoMigrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize repositories
	repos := repository.NewRepositories(db)

	// Initialize services
	services := service.NewServices(repos, cfg)

	// Initialize handlers
	handlers := handler.NewHandlers(services)

	// Setup Gin
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	// Middleware
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())

	// Health check (no auth required)
	r.GET("/api/health", handlers.Health.Check)
	r.GET("/api/health/detailed", handlers.Health.Detailed)

	// API routes (with Authelia auth)
	api := r.Group("/api")
	api.Use(middleware.AutheliaAuth(cfg.Server.Mode))
	{
		// Tags
		api.GET("/tags", handlers.Tag.List)
		api.POST("/tags", handlers.Tag.Create)
		api.GET("/tags/:id", handlers.Tag.Get)
		api.PUT("/tags/:id", handlers.Tag.Update)
		api.DELETE("/tags/:id", handlers.Tag.Delete)
		// Tags - 高级端点 (Batch A)
		api.GET("/tags/all", handlers.Tag.GetAll)
		api.POST("/tags/batch", handlers.Tag.BatchGetOrCreate)
		api.POST("/tags/match", handlers.Tag.Match)
		api.POST("/tags/by-names", handlers.Tag.GetByNames)

		// Data Sources
		api.GET("/datasources", handlers.DataSource.List)
		api.POST("/datasources", handlers.DataSource.Create)
		api.GET("/datasources/:id", handlers.DataSource.Get)
		api.PUT("/datasources/:id", handlers.DataSource.Update)
		api.DELETE("/datasources/:id", handlers.DataSource.Delete)

		// RSS Feeds
		api.GET("/rss", handlers.RSS.List)
		api.POST("/rss", handlers.RSS.Create)
		api.GET("/rss/:id", handlers.RSS.Get)
		api.PUT("/rss/:id", handlers.RSS.Update)
		api.DELETE("/rss/:id", handlers.RSS.Delete)

		// Webhooks
		api.GET("/webhooks", handlers.Webhook.List)
		api.POST("/webhooks", handlers.Webhook.Create)
		api.GET("/webhooks/:id", handlers.Webhook.Get)
		api.PUT("/webhooks/:id", handlers.Webhook.Update)
		api.DELETE("/webhooks/:id", handlers.Webhook.Delete)
		api.POST("/webhooks/:id/trigger", handlers.Webhook.Trigger)
		api.GET("/webhooks/:id/history", handlers.Webhook.History)

		// Dataset Mappings
		api.GET("/datasets", handlers.Dataset.List)
		api.POST("/datasets", handlers.Dataset.Create)
		api.GET("/datasets/:id", handlers.Dataset.Get)
		api.PUT("/datasets/:id", handlers.Dataset.Update)
		api.DELETE("/datasets/:id", handlers.Dataset.Delete)
		// Dataset Mappings - 高级端点 (Batch C)
		api.GET("/datasets/all", handlers.Dataset.GetAll)
		api.GET("/datasets/by-name/:name", handlers.Dataset.GetByName)
		api.POST("/datasets/by-tag", handlers.Dataset.RecommendByTag)
		api.POST("/datasets/article-tags", handlers.Dataset.AddArticleTags)
		api.GET("/datasets/article-tags/:document_id", handlers.Dataset.GetArticleTags)
		api.GET("/datasets/articles-by-tag/:tag_id", handlers.Dataset.GetArticlesByTag)

		// RagFlow
		api.POST("/ragflow/upload", handlers.RagFlow.Upload)
		api.POST("/ragflow/upload/with-routing", handlers.RagFlow.UploadWithRouting)
		api.GET("/ragflow/check-url", handlers.RagFlow.CheckURL)
		api.GET("/ragflow/documents", handlers.RagFlow.ListDocuments)
		api.DELETE("/ragflow/documents/:id", handlers.RagFlow.DeleteDocument)
		// RagFlow - 高级操作 (Batch B)
		api.GET("/ragflow/datasets", handlers.RagFlow.ListDatasets)
		api.GET("/ragflow/datasets/:dataset_id", handlers.RagFlow.GetDataset)
		api.POST("/ragflow/datasets", handlers.RagFlow.CreateDataset)
		api.PUT("/ragflow/datasets/:dataset_id", handlers.RagFlow.UpdateDataset)
		api.DELETE("/ragflow/datasets/:dataset_id", handlers.RagFlow.DeleteDataset)
		api.POST("/ragflow/documents/parse", handlers.RagFlow.RunParsing)
		api.POST("/ragflow/documents/parse/stop", handlers.RagFlow.StopParsing)
		api.GET("/ragflow/documents/parse/status", handlers.RagFlow.GetParsingStatus)
		api.POST("/ragflow/upload/batch", handlers.RagFlow.BatchUpload)
		api.POST("/ragflow/documents/batch-delete", handlers.RagFlow.BatchDeleteDocuments)
		api.POST("/ragflow/documents/transfer", handlers.RagFlow.TransferDocument)
		api.PUT("/ragflow/documents/metadata", handlers.RagFlow.UpdateDocumentMetadata)
		api.GET("/ragflow/chunks", handlers.RagFlow.ListChunks)
		api.DELETE("/ragflow/chunks", handlers.RagFlow.DeleteChunks)

		// Settings
		api.GET("/settings", handlers.Setting.List)
		api.GET("/settings/:key", handlers.Setting.Get)
		api.PUT("/settings/:key", handlers.Setting.Update)

		// Workflows (n8n integration)
		api.GET("/workflows/status", handlers.Workflow.Status)
		api.GET("/workflows/:id", handlers.Workflow.Get)
		api.POST("/workflows/:id/activate", handlers.Workflow.Activate)
		api.POST("/workflows/:id/deactivate", handlers.Workflow.Deactivate)
		api.GET("/workflows/executions", handlers.Workflow.Executions)
		api.POST("/workflows/trigger/:name", handlers.Workflow.Trigger)
	}

	// Serve frontend static files
	// Check if web/dist exists (for production builds)
	if _, err := os.Stat("web/dist"); err == nil {
		r.Static("/assets", "web/dist/assets")
		r.StaticFile("/favicon.ico", "web/dist/favicon.ico")

		// SPA fallback: serve index.html for all non-API routes
		r.NoRoute(func(c *gin.Context) {
			// Don't serve index.html for API routes
			if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/api" {
				c.JSON(404, gin.H{"error": "Not found"})
				return
			}
			c.File("web/dist/index.html")
		})
	}

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Bellkeeper server starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func runMigrate(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db, err := model.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := model.AutoMigrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	log.Println("Database migrations completed successfully")
}
