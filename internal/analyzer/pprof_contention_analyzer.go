package analyzer

import (
	"context"
	"io"

	libanalyzer "github.com/perf-analysis/perflib/analyzer"
	"github.com/perf-analysis/pkg/model"
)

// PProfContentionAnalyzer analyzes Go pprof Block/Mutex profile data.
// It handles both block and mutex profiles as they have similar structure.
// It delegates to the perflib engine and adapts the business model.
type PProfContentionAnalyzer struct {
	engine *libanalyzer.PProfContentionAnalyzer
	config *BaseAnalyzerConfig
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
		engine: libanalyzer.NewPProfBlockAnalyzer(convertConfig(config)),
		config: config,
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
		engine: libanalyzer.NewPProfMutexAnalyzer(convertConfig(config)),
		config: config,
	}
}

// Name returns the analyzer name.
func (a *PProfContentionAnalyzer) Name() string {
	return a.engine.Name()
}

// Analyze performs pprof Block/Mutex analysis using an input file.
func (a *PProfContentionAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	libResp, err := a.engine.Analyze(ctx, convertRequest(req))
	if err != nil {
		return nil, err
	}
	return convertResponse(libResp, req.TaskUUID), nil
}

// AnalyzeFromReader performs pprof Block/Mutex analysis from a reader.
func (a *PProfContentionAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	libResp, err := a.engine.AnalyzeFromReader(ctx, convertRequest(req), dataReader)
	if err != nil {
		return nil, err
	}
	return convertResponse(libResp, req.TaskUUID), nil
}
