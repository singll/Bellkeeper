package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/handler"
	"github.com/singll/bellkeeper/internal/matrix/gateway"
	"github.com/singll/bellkeeper/internal/matrix/infra"
	"github.com/singll/bellkeeper/internal/matrix/policy"
	"github.com/singll/bellkeeper/internal/matrix/worker"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/defaults"
	"github.com/singll/bellkeeper/internal/pkg/meili"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/singll/bellkeeper/internal/router"
	"github.com/singll/bellkeeper/internal/service"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// App holds all initialized services and infrastructure for the Bellkeeper server.
type App struct {
	cfg *config.Config

	// Database
	db *gorm.DB

	// Repositories
	repos *repository.Repositories

	// Core services
	services *service.Services

	// HTTP handlers
	handlers *handler.Handlers

	// Infrastructure
	redisClient    *infra.RedisClient
	natsClient     *infra.NATSClient
	matrixClient   *gateway.Client
	matrixSyncLoop *gateway.SyncLoop
	notifyWorker   *worker.NotificationWorker

	// Knowledge services
	knowledgeIndexSvc *service.KnowledgeIndexService

	// Knowledge adapters (wired to Matrix command service later)
	knowledgeAskAdapter    *service.AskServiceAdapter
	knowledgeSearchAdapter *service.SearchServiceAdapter

	// HTTP server
	httpSrv *http.Server

	// Shutdown channel for restart functionality
	shutdownChan chan struct{}

	logger *zap.Logger
}

// NewApp loads config, initializes database, logger, and creates an App instance.
func NewApp(cfgFile string) (*App, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		return nil, fmt.Errorf("init logger: %w", err)
	}

	logger := middleware.GetLogger()

	db, err := model.InitDB(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}

	if err := model.AutoMigrateWithLLMSeed(db, cfg); err != nil {
		if sqlDB, e := db.DB(); e == nil {
			sqlDB.Close()
		}
		return nil, fmt.Errorf("migrate: %w", err)
	}

	shutdownChan := make(chan struct{}, 1)

	return &App{
		cfg:          cfg,
		db:           db,
		logger:       logger,
		shutdownChan: shutdownChan,
	}, nil
}

// Setup initializes all services, handlers, and infrastructure.
func (a *App) Setup() error {
	a.logger.Info("setting up application")

	// Initialize layers: Repository -> Service -> Handler
	a.repos = repository.NewRepositories(a.db)
	a.services = service.NewServices(a.repos, a.cfg, version)
	a.handlers = handler.NewHandlers(a.services, a.repos, a.shutdownChan, a.cfg.Server.APIKey, a.cfg.Memos.BaseURL, a.cfg.Memos.APIToken)

	if err := a.setupKnowledge(); err != nil {
		return fmt.Errorf("knowledge setup: %w", err)
	}

	if err := a.setupMatrixInfra(); err != nil {
		return fmt.Errorf("matrix infra setup: %w", err)
	}

	if err := a.setupMatrixGateway(); err != nil {
		return fmt.Errorf("matrix gateway setup: %w", err)
	}

	a.startBackgroundTasks()

	return nil
}

// setupKnowledge initializes Meilisearch-based knowledge services.
func (a *App) setupKnowledge() error {
	if !a.cfg.Knowledge.Enabled {
		return nil
	}

	a.logger.Info("[Knowledge] initializing knowledge services")

	meiliClient, err := meili.NewClient(
		a.cfg.Meilisearch.URL,
		a.cfg.Meilisearch.APIKey,
		a.cfg.Meilisearch.Index,
	)
	if err != nil {
		a.logger.Warn("[Knowledge] failed to initialize Meilisearch client", zap.Error(err))
		return nil // non-fatal: continue without knowledge
	}

	knowledgeIndexSvc := service.NewKnowledgeIndexService(a.cfg.Knowledge, meiliClient)
	knowledgeSearchSvc := service.NewFileSearchService(meiliClient)
	askSvc := service.NewAskService(knowledgeSearchSvc,
		fmt.Sprintf("http://localhost:%d/api/llm/v1", a.cfg.Server.Port),
		a.cfg.Server.APIKey)

	knowledgeSearchAdapter := service.NewSearchServiceAdapter(knowledgeSearchSvc)
	knowledgeAskAdapter := service.NewAskServiceAdapter(askSvc)

	a.handlers.Knowledge = handler.NewKnowledgeHandler(knowledgeSearchSvc, knowledgeIndexSvc, askSvc)
	a.handlers.KnowledgeFiles = handler.NewKnowledgeFilesHandler(
		service.NewKnowledgeFilesService(a.cfg.Knowledge))

	// Store adapters for wiring in setupMatrixGateway
	a.knowledgeAskAdapter = knowledgeAskAdapter
	a.knowledgeSearchAdapter = knowledgeSearchAdapter

	a.knowledgeIndexSvc = knowledgeIndexSvc
	a.logger.Info("[Knowledge] knowledge services initialized")
	return nil
}

// setupMatrixInfra initializes Redis, NATS, and notification services.
func (a *App) setupMatrixInfra() error {
	if a.cfg.Matrix.BotAccessToken == "" && a.cfg.Redis.Host == "" {
		return nil
	}

	a.logger.Info("[Matrix] initializing Matrix infrastructure")

	var err error
	a.redisClient, err = infra.NewRedisClient(a.cfg.Redis)
	if err != nil {
		return fmt.Errorf("init Redis: %w", err)
	}

	a.natsClient, err = infra.NewNATSClient(a.cfg.NATS)
	if err != nil {
		return fmt.Errorf("init NATS: %w", err)
	}

	// Notification service
	notifySvc := service.NewNotificationService(a.cfg.NATS, a.redisClient, a.natsClient, a.repos)
	a.services.SetNotificationService(notifySvc)
	a.handlers.MatrixNotify = handler.NewMatrixNotifyHandler(notifySvc)

	// Wire notification into crawl queue
	if a.services.CrawlQueue != nil {
		a.services.CrawlQueue.SetNotificationService(notifySvc)
	}

	// Notification worker
	notifySender := service.NewNotificationSender(nil, a.repos)
	a.notifyWorker = worker.NewNotificationWorker(a.cfg.NATS, a.natsClient, notifySender, a.cfg.Matrix.MaxRetry)
	if err := a.notifyWorker.Start(context.Background()); err != nil {
		return fmt.Errorf("start notification worker: %w", err)
	}

	// Admin service
	adminPolicy := policy.NewChecker(a.repos, []string{})
	a.services.MatrixAdmin = service.NewAdminService(a.repos, adminPolicy)
	a.handlers.MatrixAdmin = handler.NewMatrixAdminHandler(a.services.MatrixAdmin, defaults.DefaultMatrixDomain)

	return nil
}

// setupMatrixGateway initializes Matrix bot client and sync loop.
func (a *App) setupMatrixGateway() error {
	if a.cfg.Matrix.BotAccessToken == "" {
		a.logger.Info("[Matrix] Matrix Gateway disabled (no bot token configured)")
		return nil
	}

	a.logger.Info("[Matrix] initializing Matrix Gateway")

	var err error
	a.matrixClient, err = gateway.NewClient(a.cfg.Matrix, a.redisClient, a.repos)
	if err != nil {
		return fmt.Errorf("init Matrix client: %w", err)
	}

	a.matrixSyncLoop = gateway.NewSyncLoop(a.matrixClient)

	commandSvc := service.NewCommandService(a.cfg.Matrix, a.cfg.N8N, a.cfg.Memos, a.repos,
		a.matrixClient, a.cfg.Matrix.AdminUsers)

	if a.services.MatrixAdmin != nil {
		commandSvc.SetAdminService(a.services.MatrixAdmin)
	}

	a.matrixSyncLoop.SetCommandService(commandSvc)
	a.logger.Info("[Matrix] Command Router initialized with %d commands",
		zap.Int("count", len(commandSvc.ListCommands())))

	// Wire knowledge handlers if available
	if a.knowledgeSearchAdapter != nil && a.knowledgeAskAdapter != nil {
		commandSvc.SetKnowledgeHandlers(a.knowledgeAskAdapter, a.knowledgeSearchAdapter)
	}

	if err := a.matrixSyncLoop.Start(context.Background()); err != nil {
		return fmt.Errorf("start Matrix sync loop: %w", err)
	}

	// Update notification sender with matrix client
	if a.notifyWorker != nil {
		a.notifyWorker.UpdateMatrixClient(a.matrixClient)
	}

	a.logger.Info("[Matrix] Matrix Gateway started successfully")
	return nil
}

// startBackgroundTasks starts all background goroutines.
func (a *App) startBackgroundTasks() {
	// Knowledge indexing
	if a.knowledgeIndexSvc != nil {
		a.knowledgeIndexSvc.StartFullScan(context.Background())
		a.knowledgeIndexSvc.StartIncrementalScan(context.Background())
	}

	// RSS fetcher
	if a.cfg.RSSFetcher.Enabled {
		a.services.RSSFetcher.Start(context.Background())
		a.logger.Info("[RSSFetcher] RSS fetcher started")
	} else {
		a.logger.Info("[RSSFetcher] RSS fetcher disabled")
	}

	// Crawl queue
	if a.services.CrawlQueue != nil {
		a.services.CrawlQueue.Start(context.Background())
		a.logger.Info("[CrawlQueue] crawl queue started")
	}
}

// SetupHTTP configures the Gin router and HTTP server.
func (a *App) SetupHTTP() {
	if a.cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())

	router.Setup(r, a.handlers, a.cfg.Server.Mode, a.cfg.Server.APIKey)

	// Serve frontend static files
	if _, err := os.Stat("web/dist"); err == nil {
		r.Static("/assets", "web/dist/assets")
		r.StaticFile("/favicon.ico", "web/dist/favicon.ico")
		r.NoRoute(func(c *gin.Context) {
			if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/api" {
				c.JSON(404, gin.H{"error": "Not found"})
				return
			}
			c.File("web/dist/index.html")
		})
	}

	addr := fmt.Sprintf("%s:%d", a.cfg.Server.Host, a.cfg.Server.Port)
	a.httpSrv = &http.Server{
		Addr:    addr,
		Handler: r,
	}
}

// Run starts the HTTP server and blocks until shutdown.
func (a *App) Run() error {
	a.SetupHTTP()

	// Start server
	go func() {
		a.logger.Info("Bellkeeper server starting", zap.String("addr", a.httpSrv.Addr))
		if err := a.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Fatal("server failed", zap.Error(err))
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		a.logger.Info("received signal, shutting down", zap.String("signal", sig.String()))
	case <-a.shutdownChan:
		a.logger.Info("restart requested, shutting down gracefully")
	}

	return a.Shutdown()
}

// Shutdown gracefully shuts down all services.
func (a *App) Shutdown() error {
	a.logger.Info("shutting down application")

	// HTTP server
	shutdownTimeout := a.cfg.Server.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(shutdownTimeout)*time.Second)
	defer cancel()

	if a.httpSrv != nil {
		if err := a.httpSrv.Shutdown(ctx); err != nil {
			a.logger.Warn("HTTP server forced shutdown", zap.Error(err))
		}
	}

	// Core services
	if a.services.CrawlQueue != nil {
		a.services.CrawlQueue.Stop()
	}
	if a.services.LLMProxy != nil {
		a.services.LLMProxy.Stop()
	}

	// Matrix infrastructure (reverse order of initialization)
	if a.matrixSyncLoop != nil {
		a.matrixSyncLoop.Stop()
	}
	if a.notifyWorker != nil {
		a.notifyWorker.Stop()
	}
	if a.natsClient != nil {
		a.natsClient.Close()
	}
	if a.redisClient != nil {
		a.redisClient.Close()
	}
	if a.db != nil {
		if sqlDB, err := a.db.DB(); err == nil {
			sqlDB.Close()
		}
	}

	a.logger.Info("application exited")
	return nil
}

const version = "1.0.0"
