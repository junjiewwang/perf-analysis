package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/perf-analysis/internal/statistics"
	"github.com/perf-analysis/pkg/model"
)

// JavaMemAnalyzer analyzes Java async-profiler allocation/memory data.
type JavaMemAnalyzer struct {
	*BaseAnalyzer
}

// NewJavaMemAnalyzer creates a new Java memory analyzer.
func NewJavaMemAnalyzer(config *BaseAnalyzerConfig) *JavaMemAnalyzer {
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

	// Step 3: Generate flame graph (allocation flame graph)
	fg, err := a.GenerateFlameGraph(ctx, parseResult.Samples)
	if err != nil {
		return nil, fmt.Errorf("failed to generate flame graph: %w", err)
	}

	flameGraphFile := filepath.Join(taskDir, "alloc_data.json.gz")
	if err := a.WriteFlameGraphGzip(fg, flameGraphFile); err != nil {
		return nil, fmt.Errorf("failed to write flame graph: %w", err)
	}

	// Step 4: Generate call graph (allocation call graph)
	cg, err := a.GenerateCallGraph(ctx, parseResult.Samples)
	if err != nil {
		return nil, fmt.Errorf("failed to generate call graph: %w", err)
	}

	callGraphFile := filepath.Join(taskDir, "alloc_data.json")
	if err := a.WriteCallGraphJSON(cg, callGraphFile); err != nil {
		return nil, fmt.Errorf("failed to write call graph: %w", err)
	}

	// Step 5: Calculate statistics
	topFuncsResult := a.CalculateTopFuncs(parseResult.Samples)
	threadStatsResult := a.CalculateThreadStats(parseResult.Samples)

	// Step 6: Build top funcs JSON
	topFuncsMap := make(model.TopFuncsMap)
	for _, tf := range topFuncsResult.TopFuncs {
		topFuncsMap[tf.Name] = model.TopFuncValue{Self: tf.SelfPercent}
	}
	topFuncsJSON, _ := json.Marshal(topFuncsMap)

	// Step 7: Build active threads JSON
	activeThreads := make([]model.ThreadInfo, 0, len(threadStatsResult.Threads))
	for _, t := range threadStatsResult.Threads {
		activeThreads = append(activeThreads, model.ThreadInfo{
			TID:        t.TID,
			ThreadName: t.ThreadName,
			Samples:    t.Samples,
			Percentage: t.Percentage,
		})
	}
	activeThreadsJSON, _ := json.Marshal(activeThreads)

	// Step 8: Convert suggestions and add memory-specific ones
	suggestions := make([]model.SuggestionItem, 0, len(parseResult.Suggestions))
	for _, sug := range parseResult.Suggestions {
		suggestions = append(suggestions, model.SuggestionItem{
			Suggestion: sug.Suggestion,
			FuncName:   sug.FuncName,
			Namespace:  sug.Namespace,
		})
	}

	// Add memory-specific suggestions
	memSuggestions := a.generateMemorySuggestions(topFuncsResult)
	suggestions = append(suggestions, memSuggestions...)

	// Step 9: Build response
	return &model.AnalysisResponse{
		TaskUUID:          req.TaskUUID,
		TopFuncs:          string(topFuncsJSON),
		TotalRecords:      int(parseResult.TotalSamples),
		FlameGraphFile:    flameGraphFile,
		CallGraphFile:     callGraphFile,
		ActiveThreadsJSON: string(activeThreadsJSON),
		Suggestions:       suggestions,
	}, nil
}

// generateMemorySuggestions generates memory-specific suggestions.
func (a *JavaMemAnalyzer) generateMemorySuggestions(topFuncsResult *statistics.TopFuncsResult) []model.SuggestionItem {
	suggestions := make([]model.SuggestionItem, 0)

	for _, tf := range topFuncsResult.TopFuncs {
		if tf.SelfPercent > 10.0 {
			suggestions = append(suggestions, model.SuggestionItem{
				Suggestion: fmt.Sprintf("函数 %s 分配内存占比 %.2f%%，建议检查是否存在频繁内存分配", tf.Name, tf.SelfPercent),
				FuncName:   tf.Name,
			})
		}
	}

	return suggestions
}

// GetOutputFiles returns the list of output files generated by the analyzer.
func (a *JavaMemAnalyzer) GetOutputFiles(taskUUID, taskDir string) []OutputFile {
	return []OutputFile{
		{LocalPath: filepath.Join(taskDir, "alloc_data.json.gz"), COSKey: taskUUID + "/alloc_data.json.gz"},
		{LocalPath: filepath.Join(taskDir, "alloc_data.json"), COSKey: taskUUID + "/alloc_data.json"},
	}
}
