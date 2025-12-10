package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/perf-analysis/internal/service"
	"github.com/perf-analysis/pkg/config"
	"github.com/perf-analysis/pkg/utils"
)

var (
	configPath = flag.String("c", "", "Path to configuration file")
	logDir     = flag.String("d", ".", "Directory for log files")
	version    = flag.Bool("v", false, "Print version and exit")
)

// Version information (set by build flags)
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	flag.Parse()

	if *version {
		fmt.Printf("perf-analysis version %s (commit: %s, built: %s)\n", Version, GitCommit, BuildTime)
		os.Exit(0)
	}

	// Initialize logger
	logger := utils.NewDefaultLogger(utils.LevelInfo, os.Stdout)
	utils.SetGlobalLogger(logger)

	logger.Info("Starting perf-analysis service...")
	logger.Info("Version: %s, Commit: %s, Built: %s", Version, GitCommit, BuildTime)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("Failed to load configuration: %v", err)
		os.Exit(1)
	}

	logger.Info("Configuration loaded successfully")
	logger.Info("Analysis version: %s", cfg.Analysis.Version)
	logger.Info("Max workers: %d", cfg.Scheduler.WorkerCount)
	logger.Info("Database: %s://%s:%d/%s", cfg.Database.Type, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database)
	logger.Info("Storage: %s", cfg.Storage.Type)

	// Ensure data directory exists
	if err := cfg.EnsureDataDir(); err != nil {
		logger.Error("Failed to create data directory: %v", err)
		os.Exit(1)
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
		logger.Error("Failed to create service: %v", err)
		os.Exit(1)
	}

	if err := svc.Initialize(ctx); err != nil {
		logger.Error("Failed to initialize service: %v", err)
		os.Exit(1)
	}

	// Start service
	if err := svc.Start(ctx); err != nil {
		logger.Error("Failed to start service: %v", err)
		os.Exit(1)
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
}
