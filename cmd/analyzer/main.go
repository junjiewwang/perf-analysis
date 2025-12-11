package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/perf-analysis/internal/service"
	"github.com/perf-analysis/pkg/config"
	"github.com/perf-analysis/pkg/utils"
)

// Version information (injected by build flags)
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// Command line flags
var (
	configPath string
	logDir     string
	verbose    bool
)

// binName returns the base name of the current executable
func binName() string {
	return filepath.Base(os.Args[0])
}

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "perf-analyzer",
	Short: "A performance profiling analysis service",
	Long: `perf-analyzer is a background service for analyzing performance profiling data.

It reads tasks from a message queue, processes profiling data, and stores
the analysis results. The service supports multiple profiler types including
perf, async-profiler, and pprof.`,
	RunE: runService,
}

// versionCmd shows version information
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s version %s\n", binName(), Version)
		fmt.Printf("  Git Commit: %s\n", GitCommit)
		fmt.Printf("  Build Time: %s\n", BuildTime)
		fmt.Printf("  Go Version: %s\n", runtime.Version())
		fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	// Set dynamic example
	bin := binName()
	rootCmd.Example = `  # Start service with config file
  ` + bin + ` -c /etc/perf-analyzer/config.yaml

  # Start with custom log directory
  ` + bin + ` -c ./config.yaml -d /var/log/perf-analyzer

  # Start with verbose output
  ` + bin + ` -c ./config.yaml -v`

	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	// Root command flags
	rootCmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file (required)")
	rootCmd.Flags().StringVarP(&logDir, "log-dir", "d", ".", "Directory for log files")

	// Mark required flags
	rootCmd.MarkFlagRequired("config")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
}

func runService(cmd *cobra.Command, args []string) error {
	// Initialize logger
	logLevel := utils.LevelInfo
	if verbose {
		logLevel = utils.LevelDebug
	}
	logger := utils.NewDefaultLogger(logLevel, os.Stdout)
	utils.SetGlobalLogger(logger)

	logger.Info("Starting perf-analyzer service...")
	logger.Info("Version: %s, Commit: %s, Built: %s", Version, GitCommit, BuildTime)

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger.Info("Configuration loaded successfully")
	logger.Info("Analysis version: %s", cfg.Analysis.Version)
	logger.Info("Max workers: %d", cfg.Scheduler.WorkerCount)
	logger.Info("Database: %s://%s:%d/%s", cfg.Database.Type, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database)
	logger.Info("Storage: %s", cfg.Storage.Type)

	// Ensure data directory exists
	if err := cfg.EnsureDataDir(); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create and initialize service
	svc, err := service.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	if err := svc.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize service: %w", err)
	}

	// Start service
	if err := svc.Start(ctx); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	logger.Info("Service started, waiting for tasks...")

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		logger.Info("Received signal %v, initiating graceful shutdown...", sig)
		cancel()
	case <-ctx.Done():
		logger.Info("Context cancelled, shutting down...")
	}

	// Stop service
	if err := svc.Stop(); err != nil {
		logger.Error("Error during shutdown: %v", err)
	}

	logger.Info("Service stopped")
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
