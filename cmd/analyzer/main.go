package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/junjiewwang/perf-analysis/internal/service"
	"github.com/junjiewwang/perf-analysis/pkg/config"
	"github.com/junjiewwang/perf-analysis/pkg/telemetry"
	"github.com/junjiewwang/perf-analysis/pkg/utils"
	"github.com/junjiewwang/perf-analysis/pkg/viewurl"
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
	// Initialize bootstrap logger (stdout only, before config is loaded)
	logLevel := utils.LevelInfo
	if verbose {
		logLevel = utils.LevelDebug
	}
	logger := utils.NewDefaultLogger(logLevel, os.Stdout)
	utils.SetGlobalLogger(logger)

	logger.Info("Starting perf-analyzer service...")
	bootstrapLogger := logger

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize OpenTelemetry (sets global TracerProvider)
	shutdown, err := telemetry.Init(ctx)
	if err != nil {
		logger.Warn("Failed to initialize telemetry: %v", err)
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			logger.Warn("Failed to shutdown telemetry: %v", err)
		}
	}()

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Re-initialize logger with config settings
	logger, err = initLoggerFromConfig(cfg, verbose)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	utils.SetGlobalLogger(logger)

	// Print startup banner to stdout (always visible on console)
	printStartupBanner(bootstrapLogger, cfg)
	// If logger writes to file, also record the banner there
	if logger != bootstrapLogger {
		printStartupBanner(logger, cfg)
	}

	// Ensure data directory exists
	if cfg.Analysis.DataDir != "" {
		if err := os.MkdirAll(cfg.Analysis.DataDir, 0755); err != nil {
			return fmt.Errorf("failed to create data directory: %w", err)
		}
	}

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

// printStartupBanner prints a structured startup banner with version, infrastructure,
// and component status information.
func printStartupBanner(logger *utils.DefaultLogger, cfg *config.Config) {
	separator := strings.Repeat("─", 60)

	logger.Info("%s", separator)
	logger.Info("  perf-analyzer service")
	logger.Info("%s", separator)

	// Section 1: Version & Environment
	logger.Info("  Version:      %s", Version)
	logger.Info("  Git Commit:   %s", GitCommit)
	logger.Info("  Build Time:   %s", BuildTime)
	logger.Info("  Go Version:   %s", runtime.Version())
	logger.Info("  OS/Arch:      %s/%s", runtime.GOOS, runtime.GOARCH)
	logger.Info("  PID:          %d", os.Getpid())

	// Section 2: Infrastructure
	logger.Info("%s", separator)
	logger.Info("  Infrastructure")
	logger.Info("%s", separator)

	// Database
	logger.Info("  Database:     %s://%s:%d/%s (max_conns=%d)",
		cfg.Database.Type, cfg.Database.Host, cfg.Database.Port,
		cfg.Database.Database, cfg.Database.MaxConns)

	// Storage
	switch cfg.Storage.Type {
	case "cos":
		logger.Info("  Storage:      COS (bucket=%s, region=%s)", cfg.Storage.COS.Bucket, cfg.Storage.COS.Region)
	case "local":
		logger.Info("  Storage:      Local (path=%s)", cfg.Storage.Local.Path)
	default:
		logger.Info("  Storage:      %s", cfg.Storage.Type)
	}

	// Analysis
	logger.Info("  Analysis:     version=%s, data_dir=%s", cfg.Analysis.Version, cfg.Analysis.DataDir)

	// Section 3: Components
	logger.Info("%s", separator)
	logger.Info("  Components")
	logger.Info("%s", separator)

	// Scheduler
	if cfg.Scheduler.Enabled {
		logger.Info("  Scheduler:    ENABLED (workers=%d, poll=%s, batch=%d)",
			cfg.Scheduler.WorkerCount, cfg.Scheduler.PollInterval, cfg.Scheduler.TaskBatchSize)

		// Sources
		if len(cfg.Sources) > 0 {
			var enabledSources []string
			for _, src := range cfg.Sources {
				if src.Enabled {
					enabledSources = append(enabledSources, src.Type)
				}
			}
			if len(enabledSources) > 0 {
				logger.Info("  Sources:      %s", strings.Join(enabledSources, ", "))
			} else {
				logger.Info("  Sources:      (none enabled)")
			}
		} else {
			logger.Info("  Sources:      (none configured)")
		}
	} else {
		logger.Info("  Scheduler:    DISABLED")
	}

	// Ingress
	if cfg.Ingress.HTTP.Enabled {
		logger.Info("  Ingress:      ENABLED (addr=%s, path=%s)",
			cfg.Ingress.HTTP.ListenAddr, cfg.Ingress.HTTP.Path)
	} else {
		logger.Info("  Ingress:      DISABLED")
	}

	// WebUI
	if cfg.WebUI.Enabled {
		logger.Info("  WebUI:        ENABLED (port=%d, cache_dir=%s)",
			cfg.WebUI.Port, cfg.WebUI.CacheDir)
	} else {
		logger.Info("  WebUI:        DISABLED")
	}

	// ViewURL
	viewBuilder := viewurl.NewBuilder(&cfg.ViewURL, &cfg.WebUI, &cfg.Retention)
	viewBaseURL := viewBuilder.GetBaseURL()
	if viewBaseURL != "" {
		authStatus := "off"
		if cfg.ViewURL.Auth.Enabled {
			authStatus = "on"
		}
		logger.Info("  ViewURL:      %s (auth=%s)", viewBaseURL, authStatus)
	} else {
		logger.Info("  ViewURL:      (not configured)")
	}

	// Telemetry
	if telemetry.Enabled() {
		teleCfg := telemetry.GetConfig()
		logger.Info("  Telemetry:    ENABLED (endpoint=%s)", teleCfg.Endpoint)
	} else {
		logger.Info("  Telemetry:    DISABLED")
	}

	// Callback
	if cfg.Callback.DefaultURL != "" {
		logger.Info("  Callback:     url=%s, timeout=%s, retries=%d",
			cfg.Callback.DefaultURL, cfg.Callback.Timeout, cfg.Callback.MaxRetries)
	} else {
		logger.Info("  Callback:     (no default URL)")
	}

	// Retention
	logger.Info("  Retention:    default=%s", cfg.Retention.Default)

	logger.Info("%s", separator)
}

// initLoggerFromConfig creates a logger based on configuration settings.
func initLoggerFromConfig(cfg *config.Config, verbose bool) (*utils.DefaultLogger, error) {
	// Determine log level (verbose flag overrides config)
	logLevel := utils.ParseLogLevel(cfg.Log.Level)
	if verbose {
		logLevel = utils.LevelDebug
	}

	// If output_path is configured, write to file
	if cfg.Log.OutputPath != "" && cfg.Log.OutputPath != "stdout" {
		// Create log directory
		if err := os.MkdirAll(cfg.Log.OutputPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory %s: %w", cfg.Log.OutputPath, err)
		}

		// Create log file with date suffix
		logFile := filepath.Join(cfg.Log.OutputPath, "perf-analyzer.log")
		return utils.NewFileLogger(logLevel, logFile)
	}

	// Default to stdout
	return utils.NewDefaultLogger(logLevel, os.Stdout), nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
