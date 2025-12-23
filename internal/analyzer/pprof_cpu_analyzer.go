package analyzer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	pprofparser "github.com/perf-analysis/internal/parser/pprof"
	"github.com/perf-analysis/pkg/model"
)

// PProfCPUAnalyzer analyzes Go pprof CPU profile data.
type PProfCPUAnalyzer struct {
	*BaseAnalyzer
}

// NewPProfCPUAnalyzer creates a new pprof CPU analyzer.
func NewPProfCPUAnalyzer(config *BaseAnalyzerConfig) *PProfCPUAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}
	if config.AnalysisProfile == "" {
		config.AnalysisProfile = ProfileStandard
	}

	return &PProfCPUAnalyzer{
		BaseAnalyzer: NewBaseAnalyzer(config),
	}
}

// Name returns the analyzer name.
func (a *PProfCPUAnalyzer) Name() string {
	return "pprof_cpu_analyzer"
}

// SupportedTypes returns the task types supported by this analyzer.
func (a *PProfCPUAnalyzer) SupportedTypes() []model.TaskType {
	return []model.TaskType{model.TaskTypePProfCPU}
}

// Analyze performs pprof CPU analysis using an input file.
func (a *PProfCPUAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	file, err := os.Open(req.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	return a.AnalyzeFromReader(ctx, req, file)
}

// AnalyzeFromReader performs pprof CPU analysis from a reader.
func (a *PProfCPUAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	// Step 1: Parse the pprof data
	parser := pprofparser.NewParser()
	if err := parser.Parse(dataReader); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseError, err)
	}

	// Step 2: Convert to samples for flame graph generation
	samples, err := parser.ToSamples(pprofparser.SampleTypeCPU)
	if err != nil {
		// Try alternative sample type
		samples, err = parser.ToSamples(pprofparser.SampleTypeSamples)
		if err != nil {
			return nil, fmt.Errorf("failed to convert pprof to samples: %w", err)
		}
	}

	if len(samples) == 0 {
		return nil, ErrEmptyData
	}

	// Step 3: Determine output directory
	taskDir := req.OutputDir
	if taskDir == "" {
		var err error
		taskDir, err = a.EnsureOutputDir(req.TaskUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Step 4: Generate flame graph with analysis
	fg, err := a.GenerateFlameGraphWithAnalysis(ctx, samples)
	if err != nil {
		return nil, fmt.Errorf("failed to generate flame graph: %w", err)
	}

	// Step 5: Write flame graph (gzipped JSON)
	flameGraphFile := filepath.Join(taskDir, "collapsed_data.json.gz")
	if err := a.WriteFlameGraphGzip(fg, flameGraphFile); err != nil {
		return nil, fmt.Errorf("failed to write flame graph: %w", err)
	}

	// Step 6: Generate call graph
	cg, err := a.GenerateCallGraphWithAnalysis(ctx, samples)
	if err != nil {
		return nil, fmt.Errorf("failed to generate call graph: %w", err)
	}

	// Step 7: Write call graph
	callGraphFile := filepath.Join(taskDir, "callgraph_data.json.gz")
	if err := a.WriteCallGraphGzip(cg, callGraphFile); err != nil {
		return nil, fmt.Errorf("failed to write call graph: %w", err)
	}

	// Step 8: Get top functions from pprof parser (more accurate)
	topFuncsN := a.config.TopFuncsN
	if topFuncsN <= 0 {
		topFuncsN = 50
	}
	topFuncsByFlat := parser.GetTopFunctions(topFuncsN, pprofparser.SampleTypeCPU, false)
	topFuncsByCum := parser.GetTopFunctions(topFuncsN, pprofparser.SampleTypeCPU, true)

	// Convert to model types
	pprofTopFuncs := make([]model.PProfTopFunc, 0, len(topFuncsByFlat))
	for _, tf := range topFuncsByFlat {
		pprofTopFuncs = append(pprofTopFuncs, model.PProfTopFunc{
			Name:       tf.Name,
			Flat:       tf.Flat,
			FlatPct:    tf.FlatPct,
			Cum:        tf.Cum,
			CumPct:     tf.CumPct,
			Module:     tf.Module,
			SourceFile: tf.SourceFile,
			SourceLine: tf.SourceLine,
		})
	}

	topFuncsByFlatModel := make([]model.PProfTopFunc, 0, len(topFuncsByFlat))
	for _, tf := range topFuncsByFlat {
		topFuncsByFlatModel = append(topFuncsByFlatModel, model.PProfTopFunc{
			Name:       tf.Name,
			Flat:       tf.Flat,
			FlatPct:    tf.FlatPct,
			Cum:        tf.Cum,
			CumPct:     tf.CumPct,
			Module:     tf.Module,
			SourceFile: tf.SourceFile,
			SourceLine: tf.SourceLine,
		})
	}

	topFuncsByCumModel := make([]model.PProfTopFunc, 0, len(topFuncsByCum))
	for _, tf := range topFuncsByCum {
		topFuncsByCumModel = append(topFuncsByCumModel, model.PProfTopFunc{
			Name:       tf.Name,
			Flat:       tf.Flat,
			FlatPct:    tf.FlatPct,
			Cum:        tf.Cum,
			CumPct:     tf.CumPct,
			Module:     tf.Module,
			SourceFile: tf.SourceFile,
			SourceLine: tf.SourceLine,
		})
	}

	// Step 9: Build PProfCPUData
	totalSamples := parser.GetTotalSamples(pprofparser.SampleTypeCPU)
	if totalSamples == 0 {
		totalSamples = parser.GetTotalSamples(pprofparser.SampleTypeSamples)
	}

	cpuData := &model.PProfCPUData{
		FlameGraphFile: flameGraphFile,
		CallGraphFile:  callGraphFile,
		Duration:       parser.GetDuration(),
		TotalSamples:   totalSamples,
		SampleUnit:     parser.GetUnit(pprofparser.SampleTypeCPU),
		TopFuncs:       pprofTopFuncs,
		TopFuncsByFlat: topFuncsByFlatModel,
		TopFuncsByCum:  topFuncsByCumModel,
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

	// Step 11: Build response
	return &model.AnalysisResponse{
		TaskUUID:     req.TaskUUID,
		TaskType:     req.TaskType,
		TotalRecords: int(totalSamples),
		OutputFiles:  outputFiles,
		Data:         cpuData,
	}, nil
}
