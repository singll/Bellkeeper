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
	"github.com/singll/bellkeeper/internal/router"
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

	// Initialize layers: Repository → Service → Handler
	repos := repository.NewRepositories(db)
	services := service.NewServices(repos, cfg, version)
	handlers := handler.NewHandlers(services)

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
