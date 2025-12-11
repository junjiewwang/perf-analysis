package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/perf-analysis/pkg/utils"
)

var (
	// Global flags
	verbose bool
	logger  utils.Logger
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "perf-analysis",
	Short: "A performance profiling analysis tool",
	Long: `perf-analysis is a CLI tool for analyzing performance profiling data.

It supports multiple profiler types including perf, async-profiler (alloc mode),
and pprof. The tool generates flame graphs, call graphs, and provides
performance optimization suggestions.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Setup logger based on verbose flag
		logLevel := utils.LevelInfo
		if verbose {
			logLevel = utils.LevelDebug
		}
		logger = utils.NewDefaultLogger(logLevel, os.Stdout)
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

	// Set dynamic example using actual binary name
	binName := BinName()
	rootCmd.Example = `  # Analyze Java CPU profiling data
  ` + binName + ` analyze -i ./test/origin.data -t java -p perf

  # Analyze memory allocation data
  ` + binName + ` analyze -i ./alloc.data -t java -p async_alloc

  # Start web server to view results
  ` + binName + ` serve -d ./output -p 8080

  # Analyze and immediately view results
  ` + binName + ` analyze -i ./test/origin.data --serve`
}

// GetLogger returns the configured logger
func GetLogger() utils.Logger {
	return logger
}

// BinName returns the base name of the current executable
func BinName() string {
	return filepath.Base(os.Args[0])
}
