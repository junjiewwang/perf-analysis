package analyzer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/perf-analysis/pkg/model"
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

// SupportedTypes returns the task types supported by this analyzer.
func (a *JavaMemAnalyzer) SupportedTypes() []model.TaskType {
	return []model.TaskType{model.TaskTypeJava}
}

// CanHandle checks if this analyzer can handle the given request.
func (a *JavaMemAnalyzer) CanHandle(req *model.AnalysisRequest) bool {
	return req.TaskType == model.TaskTypeJava && req.ProfilerType == model.ProfilerTypeAsyncAlloc
}

// Analyze performs Java memory profiling analysis using an input file.
func (a *JavaMemAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	if req.ProfilerType != model.ProfilerTypeAsyncAlloc {
		return nil, fmt.Errorf("java mem analyzer only supports profiler type async_alloc, got %v", req.ProfilerType)
	}

	file, err := os.Open(req.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	return a.AnalyzeFromReader(ctx, req, file)
}

// AnalyzeFromReader performs Java memory profiling analysis from a reader.
func (a *JavaMemAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	if req.ProfilerType != model.ProfilerTypeAsyncAlloc {
		return nil, fmt.Errorf("java mem analyzer only supports profiler type async_alloc, got %v", req.ProfilerType)
	}

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
		taskDir, err = a.EnsureOutputDir(req.TaskUUID)
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
			Name:        "Allocation Flame Graph",
			LocalPath:   flameGraphFile,
			COSKey:      req.TaskUUID + "/alloc_data.json.gz",
			ContentType: "application/gzip",
		},
		{
			Name:        "Allocation Call Graph",
			LocalPath:   callGraphFile,
			COSKey:      req.TaskUUID + "/alloc_callgraph_data.json.gz",
			ContentType: "application/gzip",
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
	memSuggestions := a.generateMemorySuggestions(topAllocatorsMap)
	suggestions = append(suggestions, memSuggestions...)

	// Step 10: Build response
	return &model.AnalysisResponse{
		TaskUUID:     req.TaskUUID,
		TaskType:     req.TaskType,
		TotalRecords: int(parseResult.TotalSamples),
		OutputFiles:  outputFiles,
		Data:         allocData,
		Suggestions:  suggestions,
	}, nil
}

// generateMemorySuggestions generates memory-specific suggestions.
func (a *JavaMemAnalyzer) generateMemorySuggestions(topAllocators model.TopFuncsMap) []model.SuggestionItem {
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

// GetOutputFiles returns the list of output files generated by the analyzer.
func (a *JavaMemAnalyzer) GetOutputFiles(taskUUID, taskDir string) []model.OutputFile {
	return []model.OutputFile{
		{
			Name:        "Allocation Flame Graph",
			LocalPath:   filepath.Join(taskDir, "alloc_data.json.gz"),
			COSKey:      taskUUID + "/alloc_data.json.gz",
			ContentType: "application/gzip",
		},
		{
			Name:        "Allocation Call Graph",
			LocalPath:   filepath.Join(taskDir, "alloc_callgraph_data.json.gz"),
			COSKey:      taskUUID + "/alloc_callgraph_data.json.gz",
			ContentType: "application/gzip",
		},
	}
}
