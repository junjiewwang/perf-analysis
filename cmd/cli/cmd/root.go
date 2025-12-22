package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/perf-analysis/pkg/pprof"
	"github.com/perf-analysis/pkg/utils"
)

var (
	// Global flags
	verbose bool
	logger  utils.Logger

	// Pprof flags
	pprofEnabled     bool
	pprofMode        string
	pprofDir         string
	pprofProfiles    string
	pprofInterval    string
	pprofCPUDuration string
	pprofCPURate     int
	pprofAddr        string

	// Pprof collector
	pprofCollector *pprof.Collector
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "perf-analysis",
	Short: "A performance profiling analysis tool",
	Long: `perf-analysis is a CLI tool for analyzing performance profiling data.

It supports multiple profiler types including perf, async-profiler (alloc mode),
and pprof. The tool generates flame graphs, call graphs, and provides
performance optimization suggestions.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Setup logger based on verbose flag
		logLevel := utils.LevelInfo
		if verbose {
			logLevel = utils.LevelDebug
		}
		logger = utils.NewDefaultLogger(logLevel, os.Stdout)

		// Initialize pprof if enabled
		if pprofEnabled {
			cfg, err := buildPprofConfig()
			if err != nil {
				return err
			}

			collector, err := pprof.NewCollector(cfg)
			if err != nil {
				return err
			}

			if err := collector.Start(); err != nil {
				return err
			}

			pprofCollector = collector
			logger.Info("pprof collection started (mode: %s, dir: %s)", cfg.Mode, cfg.OutputDir)
		}

		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		// Stop pprof collector
		if pprofCollector != nil {
			logger.Info("Stopping pprof collection...")
			if err := pprofCollector.Stop(); err != nil {
				logger.Warn("Failed to stop pprof collector: %v", err)
			}
			logger.Info("pprof data saved to: %s", pprofCollector.Writer().GetOutputDir())
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	// Pprof flags
	rootCmd.PersistentFlags().BoolVar(&pprofEnabled, "pprof", false, "Enable pprof performance profiling")
	rootCmd.PersistentFlags().StringVar(&pprofMode, "pprof-mode", "file", "Pprof mode: file (periodic snapshots) or http (on-demand)")
	rootCmd.PersistentFlags().StringVar(&pprofDir, "pprof-dir", "./pprof", "Output directory for pprof data")
	rootCmd.PersistentFlags().StringVar(&pprofProfiles, "pprof-profiles", "cpu,heap,goroutine", "Comma-separated profile types: cpu,heap,goroutine,block,mutex,allocs")
	rootCmd.PersistentFlags().StringVar(&pprofInterval, "pprof-interval", "30s", "Snapshot interval for file mode")
	rootCmd.PersistentFlags().StringVar(&pprofCPUDuration, "pprof-cpu-duration", "10s", "CPU profile duration per snapshot")
	rootCmd.PersistentFlags().IntVar(&pprofCPURate, "pprof-cpu-rate", 100, "CPU profiling rate in Hz")
	rootCmd.PersistentFlags().StringVar(&pprofAddr, "pprof-addr", ":6060", "HTTP listen address for http mode")

	// Set dynamic example using actual binary name
	binName := BinName()
	rootCmd.Example = `  # Analyze Java CPU profiling data
  ` + binName + ` analyze -i ./test/origin.data -t java -p perf

  # Analyze memory allocation data
  ` + binName + ` analyze -i ./alloc.data -t java -p async_alloc

  # Start web server to view results
  ` + binName + ` serve -d ./output -p 8080

  # Analyze and immediately view results
  ` + binName + ` analyze -i ./test/origin.data --serve

  # Enable pprof profiling during analysis
  ` + binName + ` analyze -i ./test/origin.data --pprof --pprof-profiles cpu,heap

  # Use HTTP mode for pprof (useful for long-running operations)
  ` + binName + ` analyze -i ./test/origin.data --pprof --pprof-mode http --pprof-addr :6060`
}

// GetLogger returns the configured logger
func GetLogger() utils.Logger {
	return logger
}

// BinName returns the base name of the current executable
func BinName() string {
	return filepath.Base(os.Args[0])
}

// buildPprofConfig builds pprof configuration from command line flags.
func buildPprofConfig() (*pprof.Config, error) {
	cfg := pprof.DefaultConfig()
	cfg.Enabled = true
	cfg.OutputDir = pprofDir

	// Parse mode
	switch pprofMode {
	case "file":
		cfg.Mode = pprof.ModeFile
	case "http":
		cfg.Mode = pprof.ModeHTTP
	default:
		return nil, fmt.Errorf("invalid pprof mode: %q (valid: file, http)", pprofMode)
	}

	// Parse profile types
	profiles, err := pprof.ParseProfileTypes(pprofProfiles)
	if err != nil {
		return nil, err
	}
	cfg.Profiles = profiles

	// Parse interval
	interval, err := time.ParseDuration(pprofInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid pprof interval: %w", err)
	}
	cfg.FileConfig.Interval = interval

	// Parse CPU duration
	cpuDuration, err := time.ParseDuration(pprofCPUDuration)
	if err != nil {
		return nil, fmt.Errorf("invalid pprof CPU duration: %w", err)
	}
	cfg.FileConfig.CPUDuration = cpuDuration
	cfg.FileConfig.CPURate = pprofCPURate

	// HTTP config
	cfg.HTTPConfig.Addr = pprofAddr

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
