package main

import (
	"fmt"
	"os"

	"github.com/singll/bellkeeper/internal/app"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
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
	a, err := app.NewApp(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize app: %v\n", err)
		os.Exit(1)
	}

	if err := a.Setup(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup application: %v\n", err)
		os.Exit(1)
	}

	if err := a.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func runMigrate(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	db, err := model.InitDB(cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	if err := model.AutoMigrate(db); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run migrations: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Database migrations completed successfully")
}
