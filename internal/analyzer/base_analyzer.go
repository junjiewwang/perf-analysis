package analyzer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/junjiewwang/perf-analysis/internal/callgraph"
	"github.com/junjiewwang/perf-analysis/internal/flamegraph"
	libanalyzer "github.com/junjiewwang/perf-analysis/perflib/analyzer"
	"github.com/junjiewwang/perf-analysis/pkg/utils"
)

// AnalysisProfile defines preset analysis configurations for different use cases.
// It is a type alias to the perflib analyzer AnalysisProfile.
type AnalysisProfile = libanalyzer.AnalysisProfile

// Profile constants — aliases to perflib/analyzer.
const (
	// ProfileQuick provides fast analysis with minimal overhead.
	ProfileQuick = libanalyzer.ProfileQuick
	// ProfileStandard provides balanced analysis (default).
	ProfileStandard = libanalyzer.ProfileStandard
	// ProfileDetailed provides comprehensive analysis for deep investigation.
	ProfileDetailed = libanalyzer.ProfileDetailed
)

// BaseAnalyzerConfig holds configuration for the base analyzer.
// It uses utils.Logger (business layer) instead of perflib.Logger.
type BaseAnalyzerConfig struct {
	// OutputDir is the directory for output files.
	OutputDir string

	// FlameGraphOptions configures flame graph generation.
	FlameGraphOptions *flamegraph.GeneratorOptions

	// CallGraphOptions configures call graph generation.
	CallGraphOptions *callgraph.GeneratorOptions

	// TopFuncsN configures top functions calculation.
	TopFuncsN int

	// IncludeSwapper includes swapper thread in statistics.
	IncludeSwapper bool

	// Logger is used for debug logging. If nil, debug logs are suppressed.
	Logger utils.Logger

	// Verbose enables verbose debug output including detailed analysis.
	// This is typically enabled via the -v command line flag.
	Verbose bool

	// AnalysisProfile selects preset analysis configuration.
	AnalysisProfile AnalysisProfile
}

// DefaultBaseAnalyzerConfig returns default configuration.
func DefaultBaseAnalyzerConfig() *BaseAnalyzerConfig {
	return &BaseAnalyzerConfig{
		OutputDir:         "",
		FlameGraphOptions: flamegraph.DefaultGeneratorOptions(),
		CallGraphOptions:  callgraph.DefaultGeneratorOptions(),
		TopFuncsN:         50,
		IncludeSwapper:    false,
		AnalysisProfile:   ProfileStandard,
	}
}

// BaseAnalyzer provides common functionality for all analyzers.
// It embeds the perflib BaseAnalyzer engine and delegates all pure analysis methods.
// Business-specific methods (EnsureOutputDir with taskUUID) are kept locally.
type BaseAnalyzer struct {
	*libanalyzer.BaseAnalyzer
	config *BaseAnalyzerConfig
}

// NewBaseAnalyzer creates a new base analyzer.
func NewBaseAnalyzer(config *BaseAnalyzerConfig) *BaseAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}

	return &BaseAnalyzer{
		BaseAnalyzer: libanalyzer.NewBaseAnalyzer(convertConfig(config)),
		config:       config,
	}
}

// EnsureOutputDir ensures the output directory exists.
// This is a business-specific method that uses taskUUID as the subdirectory name.
// The perflib version uses a descriptive subDir name (e.g., "cpu-analysis").
func (a *BaseAnalyzer) EnsureOutputDir(taskUUID string) (string, error) {
	outputDir := a.config.OutputDir
	if outputDir == "" {
		outputDir = os.TempDir()
	}

	taskDir := filepath.Join(outputDir, taskUUID)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	return taskDir, nil
}
