package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/handler"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/matrix/policy"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/meili"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/singll/bellkeeper/internal/router"
	"github.com/singll/bellkeeper/internal/service"
	"github.com/singll/bellkeeper/internal/matrix/gateway"
	"github.com/singll/bellkeeper/internal/matrix/infra"
	"github.com/singll/bellkeeper/internal/matrix/worker"

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

	// Run auto-migration and seed defaults (including LLM proxy config from YAML)
	if err := model.AutoMigrateWithLLMSeed(db, cfg); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize logger
	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	// Shutdown channel for restart functionality
	shutdownChan := make(chan struct{}, 1)

	// Initialize layers: Repository → Service → Handler
	repos := repository.NewRepositories(db)
	services := service.NewServices(repos, cfg, version)
	handlers := handler.NewHandlers(services, shutdownChan, cfg.Memos.BaseURL, cfg.Memos.APIToken)

	// Initialize Knowledge services (Meilisearch-based search and QA)
	var knowledgeIndexSvc *service.KnowledgeIndexService
	var knowledgeSearchSvc *service.FileSearchService
	var askSvc *service.AskService
	var knowledgeSearchAdapter *service.SearchServiceAdapter
	var knowledgeAskAdapter *service.AskServiceAdapter
	if cfg.Knowledge.Enabled {
		log.Println("[Knowledge] initializing knowledge services...")

		// Initialize Meilisearch client
		meiliClient, err := meili.NewClient(
			cfg.Meilisearch.URL,
			cfg.Meilisearch.APIKey,
			cfg.Meilisearch.Index,
		)
		if err != nil {
			log.Printf("[Knowledge] failed to initialize Meilisearch client: %v", err)
		} else {
			// Initialize services
			knowledgeIndexSvc = service.NewKnowledgeIndexService(cfg.Knowledge, meiliClient)
			knowledgeSearchSvc = service.NewFileSearchService(meiliClient)
			askSvc = service.NewAskService(knowledgeSearchSvc, fmt.Sprintf("http://localhost:%d/api/llm/v1", cfg.Server.Port), cfg.Server.APIKey)

			// Create adapters for Matrix commands
			knowledgeSearchAdapter = service.NewSearchServiceAdapter(knowledgeSearchSvc)
			knowledgeAskAdapter = service.NewAskServiceAdapter(askSvc)

			// Initialize HTTP handlers
			handlers.Knowledge = handler.NewKnowledgeHandler(knowledgeSearchSvc, knowledgeIndexSvc, askSvc)

			// Start indexing in background
			knowledgeIndexSvc.StartFullScan(context.Background())
			knowledgeIndexSvc.StartIncrementalScan(context.Background())

			log.Println("[Knowledge] knowledge services initialized")
		}
	}

	// Initialize Matrix infrastructure (Redis, NATS)
	var (
		redisClient           *infra.RedisClient
		natsClient            *infra.NATSClient
		matrixClient          *gateway.Client
		matrixSyncLoop        *gateway.SyncLoop
		notifyWorker          *worker.NotificationWorker
	)

	if cfg.Matrix.BotAccessToken != "" || cfg.Redis.Host != "" {
		log.Println("[Matrix] initializing Matrix infrastructure...")

		// Initialize Redis (used by both Gateway and Notification)
		redisClient, err = infra.NewRedisClient(cfg.Redis)
		if err != nil {
			log.Fatalf("Failed to initialize Redis: %v", err)
		}
		defer redisClient.Close()

		// Initialize NATS (used by Notification Gateway)
		natsClient, err = infra.NewNATSClient(cfg.NATS)
		if err != nil {
			log.Fatalf("Failed to initialize NATS: %v", err)
		}
		defer natsClient.Close()

		// Initialize Notification Service
		notifySvc := service.NewNotificationService(cfg.NATS, redisClient, natsClient, repos)
		services.SetNotificationService(notifySvc)
		// Inject notification handler via field assignment (instead of recreating all handlers)
		handlers.MatrixNotify = handler.NewMatrixNotifyHandler(notifySvc)

		// Initialize Notification Worker
		notifySender := service.NewNotificationSender(nil, repos) // sender needs matrix client
		notifyWorker = worker.NewNotificationWorker(cfg.NATS, natsClient, notifySender, cfg.Matrix.MaxRetry)
		if err := notifyWorker.Start(context.Background()); err != nil {
			log.Fatalf("Failed to start notification worker: %v", err)
		}

		// Initialize Admin Service for Matrix platform management
		// Create a policy checker for permission validation
		adminPolicy := policy.NewChecker(repos, []string{})
		services.MatrixAdmin = service.NewAdminService(repos, adminPolicy)
		// Inject admin handler via field assignment (instead of recreating all handlers)
		matrixDomain := "matrix.singll.net"
		handlers.MatrixAdmin = handler.NewMatrixAdminHandler(services.MatrixAdmin, matrixDomain)
	}

	// Initialize Matrix Gateway (if bot token configured)
	if cfg.Matrix.BotAccessToken != "" {
		log.Println("[Matrix] initializing Matrix Gateway...")

		// Initialize Matrix client
		matrixClient, err = gateway.NewClient(cfg.Matrix, redisClient, repos)
		if err != nil {
			log.Fatalf("Failed to initialize Matrix client: %v", err)
		}

		// Create and start sync loop
		matrixSyncLoop = gateway.NewSyncLoop(matrixClient)

		// Initialize Command Service and attach to sync loop
		commandSvc := service.NewCommandService(cfg.Matrix, cfg.N8N, cfg.Memos, repos, matrixClient)

		// Wire up AdminService for admin commands
		if services.MatrixAdmin != nil {
			commandSvc.SetAdminService(services.MatrixAdmin)
		}

		// Wire up knowledge command handlers if available
		if knowledgeSearchAdapter != nil && knowledgeAskAdapter != nil {
			commandSvc.SetKnowledgeHandlers(knowledgeAskAdapter, knowledgeSearchAdapter)
		}

		matrixSyncLoop.SetCommandService(commandSvc)
		log.Printf("[Matrix] Command Router initialized with %d commands", len(commandSvc.ListCommands()))

		if err := matrixSyncLoop.Start(context.Background()); err != nil {
			log.Fatalf("Failed to start Matrix sync loop: %v", err)
		}

		// Update notification sender with matrix client
		if notifyWorker != nil {
			notifyWorker.UpdateMatrixClient(matrixClient)
		}

		log.Println("[Matrix] Matrix Gateway started successfully")
	} else {
		log.Println("[Matrix] Matrix Gateway disabled (no bot token configured)")
	}

	// Auto-sync dataset mappings with RAGFlow in background
	go func() {
		if result, err := services.RagFlow.SyncDatasetMappings(); err != nil {
			log.Printf("warn: dataset sync failed (will retry on next restart): %v", err)
		} else {
			log.Printf("info: dataset sync done — matched:%d created:%d skipped:%d failed:%d",
				result.Matched, result.Created, result.Skipped, result.Failed)
		}
	}()

	// Setup Gin
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	// Global middleware
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())

	// Register all routes
	router.Setup(r, handlers, cfg.Server.Mode, cfg.Server.APIKey)

	// Serve frontend static files
	if _, err := os.Stat("web/dist"); err == nil {
		r.Static("/assets", "web/dist/assets")
		r.StaticFile("/favicon.ico", "web/dist/favicon.ico")

		// SPA fallback: serve index.html for all non-API routes
		r.NoRoute(func(c *gin.Context) {
			if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/api" {
				c.JSON(404, gin.H{"error": "Not found"})
				return
			}
			c.File("web/dist/index.html")
		})
	}

	// Create http.Server for graceful shutdown
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Bellkeeper server starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for shutdown signal (OS signal or restart request)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("Received signal %v, shutting down...", sig)
	case <-shutdownChan:
		log.Println("Restart requested, shutting down gracefully...")
	}

	// Graceful shutdown with configured timeout
	shutdownTimeout := cfg.Server.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(shutdownTimeout)*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced shutdown: %v", err)
	}

	// Shutdown Matrix infrastructure in reverse order of initialization
	log.Println("Shutting down Matrix infrastructure...")

	// Stop Matrix sync loop
	if matrixSyncLoop != nil {
		matrixSyncLoop.Stop()
	}

	// Stop notification worker
	if notifyWorker != nil {
		notifyWorker.Stop()
	}

	// Close NATS connection
	if natsClient != nil {
		natsClient.Close()
	}

	// Close Redis connection
	if redisClient != nil {
		redisClient.Close()
	}

	// Close database connection
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.Close()
	}

	log.Println("Server exited")
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
