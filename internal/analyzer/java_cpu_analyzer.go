package analyzer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/perf-analysis/pkg/model"
)

// JavaCPUAnalyzer analyzes Java async-profiler CPU data.
type JavaCPUAnalyzer struct {
	*BaseAnalyzer
}

// NewJavaCPUAnalyzer creates a new Java CPU analyzer.
func NewJavaCPUAnalyzer(config *BaseAnalyzerConfig) *JavaCPUAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}
	// Default to standard profile for CPU analysis
	if config.AnalysisProfile == "" {
		config.AnalysisProfile = ProfileStandard
	}

	return &JavaCPUAnalyzer{
		BaseAnalyzer: NewBaseAnalyzer(config),
	}
}

// Name returns the analyzer name.
func (a *JavaCPUAnalyzer) Name() string {
	return "java_cpu_analyzer"
}

// SupportedTypes returns the task types supported by this analyzer.
func (a *JavaCPUAnalyzer) SupportedTypes() []model.TaskType {
	return []model.TaskType{model.TaskTypeJava}
}

// Analyze performs Java CPU profiling analysis using an input file.
func (a *JavaCPUAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	if req.ProfilerType != model.ProfilerTypePerf {
		return nil, fmt.Errorf("java cpu analyzer only supports profiler type perf, got %v", req.ProfilerType)
	}

	file, err := os.Open(req.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	return a.AnalyzeFromReader(ctx, req, file)
}

// AnalyzeFromReader performs Java CPU profiling analysis from a reader.
func (a *JavaCPUAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	if req.ProfilerType != model.ProfilerTypePerf {
		return nil, fmt.Errorf("java cpu analyzer only supports profiler type perf, got %v", req.ProfilerType)
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

	// Step 3: Generate unified flame graph with thread analysis
	fg, err := a.GenerateFlameGraphWithAnalysis(ctx, parseResult.Samples)
	if err != nil {
		return nil, fmt.Errorf("failed to generate flame graph: %w", err)
	}

	// Step 4: Write flame graph (gzipped JSON)
	flameGraphFile := filepath.Join(taskDir, "collapsed_data.json.gz")
	if err := a.WriteFlameGraphGzip(fg, flameGraphFile); err != nil {
		return nil, fmt.Errorf("failed to write flame graph: %w", err)
	}

	// Step 5: Generate enhanced call graph with full analysis
	cg, err := a.GenerateCallGraphWithAnalysis(ctx, parseResult.Samples)
	if err != nil {
		return nil, fmt.Errorf("failed to generate call graph: %w", err)
	}

	// Step 6: Write call graph (gzipped JSON for consistency)
	callGraphFile := filepath.Join(taskDir, "callgraph_data.json.gz")
	if err := a.WriteCallGraphGzip(cg, callGraphFile); err != nil {
		return nil, fmt.Errorf("failed to write call graph: %w", err)
	}

	// Step 7: Calculate statistics from flame graph
	topFuncsMap := make(model.TopFuncsMap)
	if fg.ThreadAnalysis != nil {
		for _, tf := range fg.ThreadAnalysis.TopFunctions {
			topFuncsMap[tf.Name] = model.TopFuncValue{Self: tf.Percentage}
		}
	}

	// Step 8: Build thread stats from flame graph
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
	}

	// Step 9: Build CPUProfilingData
	cpuData := &model.CPUProfilingData{
		FlameGraphFile: flameGraphFile,
		CallGraphFile:  callGraphFile,
		ThreadStats:    threadStats,
		TopFuncs:       topFuncsMap,
		TotalSamples:   parseResult.TotalSamples,
	}

	// Step 10: Build output files
	outputFiles := []model.OutputFile{
		{
			Name:        "Flame Graph",
			LocalPath:   flameGraphFile,
			COSKey:      req.TaskUUID + "/collapsed_data.json.gz",
			ContentType: "application/gzip",
		},
		{
			Name:        "Call Graph",
			LocalPath:   callGraphFile,
			COSKey:      req.TaskUUID + "/callgraph_data.json.gz",
			ContentType: "application/gzip",
		},
	}

	// Step 11: Convert suggestions
	suggestions := make([]model.SuggestionItem, 0, len(parseResult.Suggestions))
	for _, sug := range parseResult.Suggestions {
		suggestions = append(suggestions, model.SuggestionItem{
			Suggestion: sug.Suggestion,
			FuncName:   sug.FuncName,
			Namespace:  sug.Namespace,
		})
	}

	// Step 12: Build response
	return &model.AnalysisResponse{
		TaskUUID:     req.TaskUUID,
		TaskType:     req.TaskType,
		TotalRecords: int(parseResult.TotalSamples),
		OutputFiles:  outputFiles,
		Data:         cpuData,
		Suggestions:  suggestions,
	}, nil
}

// GetOutputFiles returns the list of output files generated by the analyzer.
func (a *JavaCPUAnalyzer) GetOutputFiles(taskUUID, taskDir string) []model.OutputFile {
	return []model.OutputFile{
		{
			Name:        "Flame Graph",
			LocalPath:   filepath.Join(taskDir, "collapsed_data.json.gz"),
			COSKey:      taskUUID + "/collapsed_data.json.gz",
			ContentType: "application/gzip",
		},
		{
			Name:        "Call Graph",
			LocalPath:   filepath.Join(taskDir, "callgraph_data.json.gz"),
			COSKey:      taskUUID + "/callgraph_data.json.gz",
			ContentType: "application/gzip",
		},
	}
}
