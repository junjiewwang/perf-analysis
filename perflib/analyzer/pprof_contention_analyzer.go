package analyzer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/junjiewwang/perf-analysis/perflib/model"
	"github.com/junjiewwang/perf-analysis/perflib/output"
	pprofparser "github.com/junjiewwang/perf-analysis/perflib/parser/pprof"
)

// PProfContentionAnalyzer analyzes Go pprof Block/Mutex profile data.
// It handles both block and mutex profiles as they have similar structure.
type PProfContentionAnalyzer struct {
	*BaseAnalyzer
	analyzerType string // "block" or "mutex"
}

// NewPProfBlockAnalyzer creates a new pprof Block analyzer.
func NewPProfBlockAnalyzer(config *BaseAnalyzerConfig) *PProfContentionAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}
	if config.AnalysisProfile == "" {
		config.AnalysisProfile = ProfileStandard
	}

	return &PProfContentionAnalyzer{
		BaseAnalyzer: NewBaseAnalyzer(config),
		analyzerType: "block",
	}
}

// NewPProfMutexAnalyzer creates a new pprof Mutex analyzer.
func NewPProfMutexAnalyzer(config *BaseAnalyzerConfig) *PProfContentionAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}
	if config.AnalysisProfile == "" {
		config.AnalysisProfile = ProfileStandard
	}

	return &PProfContentionAnalyzer{
		BaseAnalyzer: NewBaseAnalyzer(config),
		analyzerType: "mutex",
	}
}

// Name returns the analyzer name.
func (a *PProfContentionAnalyzer) Name() string {
	return fmt.Sprintf("pprof_%s_analyzer", a.analyzerType)
}

// Analyze performs pprof Block/Mutex analysis using an input file.
func (a *PProfContentionAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	file, err := os.Open(req.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	return a.AnalyzeFromReader(ctx, req, file)
}

// AnalyzeFromReader performs pprof Block/Mutex analysis from a reader.
func (a *PProfContentionAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	// Step 1: Parse the pprof data
	parser := pprofparser.NewParser()
	if err := parser.Parse(dataReader); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseError, err)
	}

	// Step 2: Determine output directory
	taskDir := req.OutputDir
	if taskDir == "" {
		var err error
		taskDir, err = a.EnsureOutputDir(fmt.Sprintf("pprof-%s-analysis", a.analyzerType))
		if err != nil {
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Step 3: Get top functions (try delay first, then contentions)
	topFuncsN := a.config.TopFuncsN
	if topFuncsN <= 0 {
		topFuncsN = 50
	}

	var topFuncs []pprofparser.TopFunction
	var sampleType pprofparser.SampleType
	var totalDelay, totalCount int64

	// Try delay (nanoseconds) first
	topFuncs = parser.GetTopFunctions(topFuncsN, pprofparser.SampleTypeDelay, false)
	if len(topFuncs) > 0 {
		sampleType = pprofparser.SampleTypeDelay
		totalDelay = parser.GetTotalSamples(pprofparser.SampleTypeDelay)
	}

	// Also get contentions count
	totalCount = parser.GetTotalSamples(pprofparser.SampleTypeContentions)

	// If no delay data, try contentions
	if len(topFuncs) == 0 {
		topFuncs = parser.GetTopFunctions(topFuncsN, pprofparser.SampleTypeContentions, false)
		sampleType = pprofparser.SampleTypeContentions
	}

	if len(topFuncs) == 0 {
		return nil, ErrEmptyData
	}

	// Convert to model types
	pprofTopFuncs := convertTopFunctions(topFuncs)

	// Step 4: Generate flame graph
	var flameGraphFile string
	var outputFiles []model.OutputFile

	samples, err := parser.ToSamples(sampleType)
	if err == nil && len(samples) > 0 {
		fg, err := a.GenerateFlameGraphWithAnalysis(ctx, samples)
		if err == nil {
			flameGraphFile = filepath.Join(taskDir, output.FlameGraphFileName(a.analyzerType))
			if err := a.WriteFlameGraphGzip(fg, flameGraphFile); err == nil {
				outputFiles = append(outputFiles, model.OutputFile{
					Name:         fmt.Sprintf("%s Flame Graph", a.analyzerType),
					LocalPath:    flameGraphFile,
					RelativePath: output.FlameGraphFileName(a.analyzerType),
					ContentType:  "application/gzip",
				})
			}
		}
	}

	// Step 5: Build PProfBlockData
	unit := parser.GetUnit(sampleType)
	if unit == "" {
		unit = "nanoseconds"
	}

	blockData := &model.PProfBlockData{
		TotalDelay:     totalDelay,
		TotalCount:     totalCount,
		Unit:           unit,
		TopFuncs:       pprofTopFuncs,
		FlameGraphFile: flameGraphFile,
	}

	// Step 6: Build response
	return &model.AnalysisResponse{
		Mode:         req.Mode,
		TotalRecords: int(totalCount),
		OutputFiles:  outputFiles,
		Data:         blockData,
	}, nil
}
