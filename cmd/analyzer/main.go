package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("Failed to load configuration: %v", err)
		os.Exit(1)
	}

	logger.Info("Configuration loaded successfully")
	logger.Info("Analysis version: %s", cfg.Analysis.Version)
	logger.Info("Max workers: %d", cfg.Analysis.MaxWorker)

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

	go func() {
		sig := <-sigChan
		logger.Info("Received signal %v, initiating graceful shutdown...", sig)
		cancel()
	}()

	// TODO: Initialize and start scheduler
	// scheduler := scheduler.New(cfg, ...)
	// if err := scheduler.Start(ctx); err != nil {
	//     logger.Error("Scheduler error: %v", err)
	// }

	logger.Info("Service started, waiting for tasks...")

	// Wait for context cancellation
	<-ctx.Done()

	logger.Info("Shutting down...")
	logger.Info("Service stopped")
}
