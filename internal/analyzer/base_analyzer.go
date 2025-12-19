package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/perf-analysis/internal/callgraph"
	"github.com/perf-analysis/internal/flamegraph"
	"github.com/perf-analysis/internal/parser/collapsed"
	"github.com/perf-analysis/internal/statistics"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// AnalysisProfile defines preset analysis configurations for different use cases.
type AnalysisProfile string

const (
	// ProfileQuick provides fast analysis with minimal overhead.
	ProfileQuick AnalysisProfile = "quick"
	// ProfileStandard provides balanced analysis (default).
	ProfileStandard AnalysisProfile = "standard"
	// ProfileDetailed provides comprehensive analysis for deep investigation.
	ProfileDetailed AnalysisProfile = "detailed"
)

// BaseAnalyzerConfig holds configuration for the base analyzer.
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
type BaseAnalyzer struct {
	config          *BaseAnalyzerConfig
	parser          *collapsed.Parser
	flameGraphGen   *flamegraph.Generator
	callGraphGen    *callgraph.Generator
	topFuncsCalc    *statistics.TopFuncsCalculator
	threadStatsCalc *statistics.ThreadStatsCalculator
}

// NewBaseAnalyzer creates a new base analyzer.
func NewBaseAnalyzer(config *BaseAnalyzerConfig) *BaseAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}

	return &BaseAnalyzer{
		config:          config,
		parser:          collapsed.NewParser(collapsed.DefaultParserOptions()),
		flameGraphGen:   flamegraph.NewGenerator(config.FlameGraphOptions),
		callGraphGen:    callgraph.NewGenerator(config.CallGraphOptions),
		topFuncsCalc:    statistics.NewTopFuncsCalculator(statistics.WithTopN(config.TopFuncsN)),
		threadStatsCalc: statistics.NewThreadStatsCalculator(),
	}
}

// Parse parses the input data.
func (a *BaseAnalyzer) Parse(ctx context.Context, reader io.Reader) (*model.ParseResult, error) {
	return a.parser.Parse(ctx, reader)
}

// GenerateFlameGraph generates flame graph from samples.
func (a *BaseAnalyzer) GenerateFlameGraph(ctx context.Context, samples []*model.Sample) (*flamegraph.FlameGraph, error) {
	return a.flameGraphGen.Generate(ctx, samples)
}

// GenerateCallGraph generates call graph from samples.
func (a *BaseAnalyzer) GenerateCallGraph(ctx context.Context, samples []*model.Sample) (*callgraph.CallGraph, error) {
	return a.callGraphGen.Generate(ctx, samples)
}

// GenerateFlameGraphWithAnalysis generates flame graph with thread analysis enabled.
// The analysis depth is controlled by the AnalysisProfile in config.
func (a *BaseAnalyzer) GenerateFlameGraphWithAnalysis(ctx context.Context, samples []*model.Sample) (*flamegraph.FlameGraph, error) {
	opts := a.getFlameGraphOptionsForProfile()
	gen := flamegraph.NewGenerator(opts)
	return gen.Generate(ctx, samples)
}

// GenerateCallGraphWithAnalysis generates call graph with full analysis enabled.
// The analysis depth is controlled by the AnalysisProfile in config.
func (a *BaseAnalyzer) GenerateCallGraphWithAnalysis(ctx context.Context, samples []*model.Sample) (*callgraph.CallGraph, error) {
	opts := a.getCallGraphOptionsForProfile()
	gen := callgraph.NewGenerator(opts)
	return gen.Generate(ctx, samples)
}

// getFlameGraphOptionsForProfile returns flame graph options based on analysis profile.
func (a *BaseAnalyzer) getFlameGraphOptionsForProfile() *flamegraph.GeneratorOptions {
	opts := flamegraph.DefaultGeneratorOptions()

	switch a.config.AnalysisProfile {
	case ProfileQuick:
		// Quick: Minimal analysis for fast results
		opts.EnableThreadAnalysis = false
		opts.BuildPerThreadFlameGraphs = false
		opts.MinPercent = 0.5
		opts.TopNPerThread = 5
		opts.TopNGlobal = 20
		opts.MaxCallStacksPerThread = 50
		opts.MaxCallStacksPerFunc = 3
		opts.IncludeSwapper = false
		opts.IncludeModule = false

	case ProfileDetailed:
		// Detailed: Comprehensive analysis for deep investigation
		opts.EnableThreadAnalysis = true
		opts.BuildPerThreadFlameGraphs = true
		opts.MinPercent = 0.05
		opts.TopNPerThread = 30
		opts.TopNGlobal = 100
		opts.MaxCallStacksPerThread = 500
		opts.MaxCallStacksPerFunc = 20
		opts.IncludeSwapper = a.config.IncludeSwapper
		opts.IncludeModule = true

	default: // ProfileStandard
		// Standard: Balanced analysis (default)
		opts.EnableThreadAnalysis = true
		opts.BuildPerThreadFlameGraphs = true
		opts.MinPercent = 0.1
		opts.TopNPerThread = 15
		opts.TopNGlobal = 50
		opts.MaxCallStacksPerThread = 200
		opts.MaxCallStacksPerFunc = 10
		opts.IncludeSwapper = false
		opts.IncludeModule = true
	}

	return opts
}

// getCallGraphOptionsForProfile returns call graph options based on analysis profile.
func (a *BaseAnalyzer) getCallGraphOptionsForProfile() *callgraph.GeneratorOptions {
	opts := callgraph.DefaultGeneratorOptions()

	switch a.config.AnalysisProfile {
	case ProfileQuick:
		// Quick: Minimal analysis
		opts.EnableThreadAnalysis = false
		opts.EnableHotPathAnalysis = false
		opts.EnableModuleAnalysis = false
		opts.MinNodePct = 1.0
		opts.MinEdgePct = 0.5
		opts.TopNFunctions = 20
		opts.TopNHotPaths = 5
		opts.MaxThreadCallGraphs = 10
		opts.IncludeSwapper = false
		opts.IncludeModule = false

	case ProfileDetailed:
		// Detailed: Comprehensive analysis
		opts.EnableThreadAnalysis = true
		opts.EnableHotPathAnalysis = true
		opts.EnableModuleAnalysis = true
		opts.MinNodePct = 0.1
		opts.MinEdgePct = 0.05
		opts.TopNFunctions = 100
		opts.TopNHotPaths = 50
		opts.MaxThreadCallGraphs = 100
		opts.IncludeSwapper = a.config.IncludeSwapper
		opts.IncludeModule = true

	default: // ProfileStandard
		// Standard: Balanced analysis
		opts.EnableThreadAnalysis = true
		opts.EnableHotPathAnalysis = true
		opts.EnableModuleAnalysis = true
		opts.MinNodePct = 0.5
		opts.MinEdgePct = 0.1
		opts.TopNFunctions = 50
		opts.TopNHotPaths = 20
		opts.MaxThreadCallGraphs = 50
		opts.IncludeSwapper = false
		opts.IncludeModule = true
	}

	return opts
}

// CalculateTopFuncs calculates top hot functions.
func (a *BaseAnalyzer) CalculateTopFuncs(samples []*model.Sample) *statistics.TopFuncsResult {
	return a.topFuncsCalc.Calculate(samples)
}

// CalculateThreadStats calculates thread statistics.
func (a *BaseAnalyzer) CalculateThreadStats(samples []*model.Sample) *statistics.ThreadStatsResult {
	return a.threadStatsCalc.Calculate(samples)
}

// WriteFlameGraphGzip writes flame graph to gzip JSON file.
func (a *BaseAnalyzer) WriteFlameGraphGzip(fg *flamegraph.FlameGraph, outputPath string) error {
	writer := flamegraph.NewGzipWriter()
	return writer.WriteToFile(fg, outputPath)
}

// WriteCallGraphJSON writes call graph to JSON file.
func (a *BaseAnalyzer) WriteCallGraphJSON(cg *callgraph.CallGraph, outputPath string) error {
	writer := callgraph.NewJSONWriter()
	return writer.WriteToFile(cg, outputPath)
}

// WriteCallGraphGzip writes call graph to gzip JSON file.
func (a *BaseAnalyzer) WriteCallGraphGzip(cg *callgraph.CallGraph, outputPath string) error {
	writer := callgraph.NewGzipWriter()
	return writer.WriteToFile(cg, outputPath)
}

// BuildNamespaceResult builds the namespace result from analysis outputs.
func (a *BaseAnalyzer) BuildNamespaceResult(
	taskUUID string,
	parseResult *model.ParseResult,
	topFuncsResult *statistics.TopFuncsResult,
	threadStatsResult *statistics.ThreadStatsResult,
	flameGraphFile string,
	callGraphFile string,
	suggestions []model.Suggestion,
) (*model.NamespaceResult, error) {
	// Convert top funcs to JSON string
	topFuncsMap := make(model.TopFuncsMap)
	for _, tf := range topFuncsResult.TopFuncs {
		topFuncsMap[tf.Name] = model.TopFuncValue{
			Self: tf.SelfPercent,
		}
	}
	topFuncsJSON, err := json.Marshal(topFuncsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal top funcs: %w", err)
	}

	// Convert active threads to JSON string
	activeThreads := make([]model.ThreadInfo, 0, len(threadStatsResult.Threads))
	for _, t := range threadStatsResult.Threads {
		activeThreads = append(activeThreads, model.ThreadInfo{
			TID:        t.TID,
			ThreadName: t.ThreadName,
			Samples:    t.Samples,
			Percentage: t.Percentage,
		})
	}
	activeThreadsJSON, err := json.Marshal(activeThreads)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal active threads: %w", err)
	}

	// Convert top funcs callstacks
	topFuncsCallstacks, err := json.Marshal(topFuncsResult.GetTopFuncsCallstacks(5))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal callstacks: %w", err)
	}

	result := &model.NamespaceResult{
		TopFuncs:               string(topFuncsJSON),
		TopFuncsCallstacks:     topFuncsCallstacks,
		ActiveThreadsJSON:      string(activeThreadsJSON),
		FlameGraphFile:         flameGraphFile,
		ExtendedFlameGraphFile: flameGraphFile, // For JSON.gz format
		CallGraphFile:          callGraphFile,
		Suggestions:            suggestions,
		TotalRecords:           parseResult.TotalSamples,
	}

	return result, nil
}

// EnsureOutputDir ensures the output directory exists.
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

// CleanupOutputDir removes the output directory.
func (a *BaseAnalyzer) CleanupOutputDir(taskDir string) error {
	return os.RemoveAll(taskDir)
}
