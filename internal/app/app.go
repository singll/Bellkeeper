package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/handler"
	"github.com/singll/bellkeeper/internal/matrix/agent"
	"github.com/singll/bellkeeper/internal/matrix/command"
	"github.com/singll/bellkeeper/internal/matrix/gateway"
	"github.com/singll/bellkeeper/internal/matrix/infra"
	"github.com/singll/bellkeeper/internal/matrix/policy"
	"github.com/singll/bellkeeper/internal/matrix/worker"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkb"
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
	knowledgeIndexSvc   *service.KnowledgeIndexService
	pkbScheduler        *pkb.Scheduler
	dailyReportWatchdog *service.DailyReportWatchdog

	// Knowledge adapters (wired to Matrix command service later)
	knowledgeAskAdapter    *service.AskServiceAdapter
	knowledgeSearchAdapter *service.SearchServiceAdapter

	// Knowledge service instances (for Agent tool wiring)
	knowledgeSearchSvc *service.FileSearchService
	knowledgeAskSvc    *service.AskService

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

	if cfg.Database.AutoMigrate {
		if err := model.AutoMigrateWithLLMSeed(db, cfg); err != nil {
			if sqlDB, e := db.DB(); e == nil {
				_ = sqlDB.Close()
			}
			return nil, fmt.Errorf("migrate: %w", err)
		}
	} else {
		logger.Info("database AutoMigrate disabled; expecting schema migrations to be applied")
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
	askLayers := make([]string, 0, len(a.cfg.Knowledge.ScanDirs))
	for _, dir := range a.cfg.Knowledge.ScanDirs {
		askLayers = append(askLayers, dir.Layer)
	}
	askSvc.SetAllowedLayers(askLayers)
	if a.cfg.Knowledge.MaxContextRunes > 0 {
		askSvc.SetMaxContextRunes(a.cfg.Knowledge.MaxContextRunes)
	}

	knowledgeSearchAdapter := service.NewSearchServiceAdapter(knowledgeSearchSvc)
	knowledgeAskAdapter := service.NewAskServiceAdapter(askSvc)

	a.handlers.Knowledge = handler.NewKnowledgeHandler(knowledgeSearchSvc, knowledgeIndexSvc, askSvc)
	a.handlers.KnowledgeFiles = handler.NewKnowledgeFilesHandler(
		service.NewKnowledgeFilesService(a.cfg.Knowledge))

	// PKB 调方向掌舵面（Phase I）：待批骨架提议审批 + 设领域大方向(scope)走 REST。
	// basePath=vault 根（同 Matrix !pkb 闭包）；domains.yaml 同 scheduler 的 config/pkb 约定。
	if basePath := a.cfg.Knowledge.BasePath; basePath != "" {
		a.handlers.PKBSteer = handler.NewPKBSteerHandler(basePath, "config/pkb/domains.yaml")
	}

	// Store adapters for wiring in setupMatrixGateway
	a.knowledgeAskAdapter = knowledgeAskAdapter
	a.knowledgeSearchAdapter = knowledgeSearchAdapter
	a.knowledgeSearchSvc = knowledgeSearchSvc
	a.knowledgeAskSvc = askSvc

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
	notifySvc.Start()
	a.handlers.MatrixNotify = handler.NewMatrixNotifyHandler(notifySvc)

	// Wire notification into crawl queue
	if a.services.CrawlQueue != nil {
		a.services.CrawlQueue.SetNotificationService(notifySvc)
	}

	// Wire alert delivery into the LLM proxy's alert aggregator (until now it has
	// only buffered + persisted events with a nil notifier).
	if a.services.LLMProxy != nil {
		a.services.LLMProxy.SetAlertNotifier(service.NewMatrixAlertNotifier(notifySvc, "alerts"))
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

	if a.services.Health != nil {
		commandSvc.SetHealthChecker(a.services.Health)
	}

	// 注册 !pkb 骨架提议审批 handler（ADR-0004 过渡审批闸）。
	// 用闭包注入 pkb 包级函数，避开 command → pkb → service → command 的 import 环。
	if basePath := a.cfg.Knowledge.BasePath; basePath != "" {
		commandSvc.GetRouter().RegisterHandler(command.NewPKBProposalHandler(command.PKBProposalActions{
			List: func() (string, error) {
				props, err := pkb.ListPendingProposals(basePath)
				if err != nil {
					return "", err
				}
				if len(props) == 0 {
					return "无待批骨架提议", nil
				}
				var b strings.Builder
				b.WriteString("待批骨架提议：\n")
				for _, p := range props {
					b.WriteString(fmt.Sprintf("- %s [%s 影响半径=%d] %s — %s\n", p.ID, p.Action, p.ImpactRadius, p.Domain, p.Summary))
				}
				return b.String(), nil
			},
			Approve: func(id string) (string, error) { return pkb.ApplySkeletonProposal(basePath, id) },
			Reject:  func(id string) (string, error) { return pkb.RejectSkeletonProposal(basePath, id) },
		}))
		a.logger.Info("[Matrix] registered !pkb skeleton-proposal handler")
	}

	a.matrixSyncLoop.SetCommandService(commandSvc)
	a.logger.Info("[Matrix] Command Router initialized with %d commands",
		zap.Int("count", len(commandSvc.ListCommands())))

	// Wire knowledge handlers if available
	if a.knowledgeSearchAdapter != nil && a.knowledgeAskAdapter != nil {
		commandSvc.SetKnowledgeHandlers(a.knowledgeAskAdapter, a.knowledgeSearchAdapter)
	}

	// Wire Agent if enabled
	if a.cfg.Matrix.Agent.Enabled && a.redisClient != nil {
		toolRegistry := agent.NewToolRegistry()
		toolDeps := agent.BuildToolDependencies(
			a.services.Health,
			a.services.Dashboard,
			a.knowledgeSearchSvc,
			a.knowledgeAskSvc,
			a.repos.LLMProxy,
			a.services.CrawlQueue,
		)
		agent.RegisterReadonlyTools(toolRegistry, toolDeps)

		writeDeps := agent.BuildWriteToolDependencies(
			a.cfg.Memos.BaseURL,
			a.cfg.Memos.APIToken,
			a.services.Workflow,
		)
		agent.RegisterWriteTools(toolRegistry, writeDeps)

		adminUsers := a.cfg.Matrix.AdminUsers
		if len(adminUsers) == 0 {
			adminUsers = []string{"@singll:" + defaults.DefaultMatrixDomain}
		}
		agentPolicy := policy.NewChecker(a.repos, adminUsers)

		llmProxyURL := fmt.Sprintf("http://localhost:%d/api/llm/v1", a.cfg.Server.Port)
		agentSvc := agent.NewAgentService(
			a.cfg.Matrix.Agent,
			llmProxyURL,
			a.cfg.Server.APIKey,
			a.redisClient.GetClient(),
			a.matrixClient,
			a.repos,
			agentPolicy,
			toolRegistry,
		)
		if agentSvc != nil {
			commandSvc.SetAgent(&agentServiceAdapter{svc: agentSvc})
			a.logger.Info("[Matrix] Agent service initialized")
		}
	} else if a.cfg.Matrix.Agent.Enabled {
		a.logger.Warn("[Matrix] Agent enabled but Redis unavailable, skipping")
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
	// LLM job queue must start before producers such as RSS/crawl/classify.
	if a.services.LLMJobQueue != nil {
		a.services.LLMJobQueue.Start(context.Background())
		a.logger.Info("[LLMJobQueue] LLM job queue started")
	}

	if a.services.LLMProxy != nil {
		if err := a.services.LLMProxy.Start(context.Background()); err != nil {
			a.logger.Error("[LLMProxy] failed to start background tasks", zap.Error(err))
		} else {
			a.logger.Info("[LLMProxy] background tasks started")
		}
	}

	if a.cfg.Knowledge.Enabled {
		var pkbQueue *service.LLMJobQueueService
		if a.cfg.LLMJobQueue.Enabled {
			pkbQueue = a.services.LLMJobQueue
		}
		a.pkbScheduler = pkb.NewScheduler(
			a.cfg,
			"config/pkb",
			a.repos.Setting,
			a.repos.ArticleTag,
			pkbQueue,
			a.services.ActivityLog,
			a.repos.CrawlDomainProfile,
		)
		a.pkbScheduler.Start(context.Background())
		a.logger.Info("[PKBScheduler] PKB scheduler started")
	}

	if a.cfg.DailyReport.Enabled && a.cfg.DailyReport.WatchdogEnabled {
		watchdog, err := service.NewDailyReportWatchdog(a.cfg.DailyReport, a.cfg.FileIngestion.BasePath, a.services.Notification)
		if err != nil {
			a.logger.Error("[DailyReportWatchdog] failed to initialize", zap.Error(err))
		} else {
			a.dailyReportWatchdog = watchdog
			a.dailyReportWatchdog.Start(context.Background())
			a.logger.Info("[DailyReportWatchdog] daily report watchdog started")
		}
	}

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

	// Rule optimizer
	if a.services.RuleOptimizer != nil {
		a.services.RuleOptimizer.Start(context.Background())
		a.logger.Info("[RuleOptimizer] rule optimizer started")
	}

}

// SetupHTTP configures the Gin router and HTTP server.
func (a *App) SetupHTTP() {
	// Only "debug" keeps Gin's verbose debug mode; "release" and "noauth" both run
	// Gin in release mode (auth is controlled by AutheliaAuth, independent of Gin mode).
	if a.cfg.Server.Mode != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())

	router.Setup(r, a.handlers, a.cfg.Server.Mode, a.cfg.Server.APIKey, a.repos.LLMToken)

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
	if a.services.Notification != nil {
		a.services.Notification.Stop()
	}
	if a.services.CrawlQueue != nil {
		a.services.CrawlQueue.Stop()
	}
	if a.services.RuleOptimizer != nil {
		a.services.RuleOptimizer.Stop()
	}
	if a.pkbScheduler != nil {
		a.pkbScheduler.Stop()
	}
	if a.dailyReportWatchdog != nil {
		a.dailyReportWatchdog.Stop()
	}
	if a.services.LLMJobQueue != nil {
		a.services.LLMJobQueue.Stop()
	}
	if a.services.LLMProxy != nil {
		a.services.LLMProxy.Stop()
	}
	if a.knowledgeIndexSvc != nil {
		a.knowledgeIndexSvc.Stop()
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
		_ = a.redisClient.Close()
	}
	if a.db != nil {
		if sqlDB, err := a.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	a.logger.Info("application exited")
	return nil
}

const version = "1.0.0"

type agentServiceAdapter struct {
	svc *agent.AgentService
}

func (a *agentServiceAdapter) HandleMessage(ctx context.Context, roomID, sender, content string) (*service.AgentTurnResult, error) {
	result, err := a.svc.HandleMessage(ctx, roomID, sender, content)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &service.AgentTurnResult{
		Reply:     result.Reply,
		UsedTools: result.UsedTools,
	}, nil
}

func (a *agentServiceAdapter) ResetSession(ctx context.Context, roomID string) error {
	return a.svc.ResetSession(ctx, roomID)
}
