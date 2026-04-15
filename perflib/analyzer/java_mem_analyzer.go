package analyzer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/junjiewwang/perf-analysis/perflib/model"
)

// JavaMemAnalyzer analyzes Java async-profiler allocation/memory data.
type JavaMemAnalyzer struct {
	*BaseAnalyzer
}

// NewJavaMemAnalyzer creates a new Java memory analyzer.
func NewJavaMemAnalyzer(config *BaseAnalyzerConfig) *JavaMemAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}
	// Default to standard profile for memory analysis
	if config.AnalysisProfile == "" {
		config.AnalysisProfile = ProfileStandard
	}

	return &JavaMemAnalyzer{
		BaseAnalyzer: NewBaseAnalyzer(config),
	}
}

// Name returns the analyzer name.
func (a *JavaMemAnalyzer) Name() string {
	return "java_mem_analyzer"
}

// Analyze performs Java memory profiling analysis using an input file.
func (a *JavaMemAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	file, err := os.Open(req.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	return a.AnalyzeFromReader(ctx, req, file)
}

// AnalyzeFromReader performs Java memory profiling analysis from a reader.
func (a *JavaMemAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	// Step 1: Parse the collapsed data
	parseResult, err := a.Parse(ctx, dataReader)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseError, err)
	}

	if parseResult.TotalSamples == 0 {
		return nil, ErrEmptyData
	}

	// Step 2: Determine output directory
	taskDir := req.OutputDir
	if taskDir == "" {
		taskDir, err = a.EnsureOutputDir("mem-analysis")
		if err != nil {
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Step 3: Generate flame graph with thread analysis (allocation flame graph)
	fg, err := a.GenerateFlameGraphWithAnalysis(ctx, parseResult.Samples)
	if err != nil {
		return nil, fmt.Errorf("failed to generate flame graph: %w", err)
	}

	flameGraphFile := filepath.Join(taskDir, "alloc_data.json.gz")
	if err := a.WriteFlameGraphGzip(fg, flameGraphFile); err != nil {
		return nil, fmt.Errorf("failed to write flame graph: %w", err)
	}

	// Step 4: Generate call graph with thread analysis (allocation call graph)
	cg, err := a.GenerateCallGraphWithAnalysis(ctx, parseResult.Samples)
	if err != nil {
		return nil, fmt.Errorf("failed to generate call graph: %w", err)
	}

	// Use gzip format for consistency with CPU analyzer
	callGraphFile := filepath.Join(taskDir, "alloc_callgraph_data.json.gz")
	if err := a.WriteCallGraphGzip(cg, callGraphFile); err != nil {
		return nil, fmt.Errorf("failed to write call graph: %w", err)
	}

	// Step 5: Build top allocators from flame graph thread analysis
	topAllocatorsMap := make(model.TopFuncsMap)
	if fg.ThreadAnalysis != nil {
		for _, tf := range fg.ThreadAnalysis.TopFunctions {
			topAllocatorsMap[tf.Name] = model.TopFuncValue{Self: tf.Percentage}
		}
	} else {
		// Fallback to statistics calculation if thread analysis not available
		topFuncsResult := a.CalculateTopFuncs(parseResult.Samples)
		for _, tf := range topFuncsResult.TopFuncs {
			topAllocatorsMap[tf.Name] = model.TopFuncValue{Self: tf.SelfPercent}
		}
	}

	// Step 6: Build thread stats from flame graph
	threadStats := make([]model.ThreadInfo, 0)
	if fg.ThreadAnalysis != nil {
		for _, t := range fg.ThreadAnalysis.Threads {
			threadStats = append(threadStats, model.ThreadInfo{
				TID:        t.TID,
				ThreadName: t.Name,
				Samples:    t.Samples,
				Percentage: t.Percentage,
			})
		}
	} else {
		// Fallback to statistics calculation
		threadStatsResult := a.CalculateThreadStats(parseResult.Samples)
		for _, t := range threadStatsResult.Threads {
			threadStats = append(threadStats, model.ThreadInfo{
				TID:        t.TID,
				ThreadName: t.ThreadName,
				Samples:    t.Samples,
				Percentage: t.Percentage,
			})
		}
	}

	// Step 7: Build AllocationData
	allocData := &model.AllocationData{
		FlameGraphFile:   flameGraphFile,
		CallGraphFile:    callGraphFile,
		ThreadStats:      threadStats,
		TopAllocators:    topAllocatorsMap,
		TotalAllocations: parseResult.TotalSamples,
	}

	// Step 8: Build output files
	outputFiles := []model.OutputFile{
		{
			Name:         "Allocation Flame Graph",
			LocalPath:    flameGraphFile,
			RelativePath: "alloc_data.json.gz",
			ContentType:  "application/gzip",
		},
		{
			Name:         "Allocation Call Graph",
			LocalPath:    callGraphFile,
			RelativePath: "alloc_callgraph_data.json.gz",
			ContentType:  "application/gzip",
		},
	}

	// Step 9: Convert suggestions and add memory-specific ones
	suggestions := make([]model.SuggestionItem, 0, len(parseResult.Suggestions))
	for _, sug := range parseResult.Suggestions {
		suggestions = append(suggestions, model.SuggestionItem{
			Suggestion: sug.Suggestion,
			FuncName:   sug.FuncName,
			Namespace:  sug.Namespace,
		})
	}

	// Add memory-specific suggestions based on top allocators
	memSuggestions := generateMemorySuggestions(topAllocatorsMap)
	suggestions = append(suggestions, memSuggestions...)

	// Step 10: Build response
	return &model.AnalysisResponse{
		Mode:         req.Mode,
		TotalRecords: int(parseResult.TotalSamples),
		OutputFiles:  outputFiles,
		Data:         allocData,
		Suggestions:  suggestions,
	}, nil
}

// generateMemorySuggestions generates memory-specific suggestions.
func generateMemorySuggestions(topAllocators model.TopFuncsMap) []model.SuggestionItem {
	suggestions := make([]model.SuggestionItem, 0)

	for name, value := range topAllocators {
		if value.Self > 10.0 {
			suggestions = append(suggestions, model.SuggestionItem{
				Suggestion: fmt.Sprintf("函数 %s 分配内存占比 %.2f%%，建议检查是否存在频繁内存分配", name, value.Self),
				FuncName:   name,
			})
		}
	}

	return suggestions
}
