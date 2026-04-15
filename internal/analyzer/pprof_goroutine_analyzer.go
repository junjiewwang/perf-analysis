package analyzer

import (
	"context"
	"io"

	libanalyzer "github.com/junjiewwang/perf-analysis/perflib/analyzer"
	"github.com/junjiewwang/perf-analysis/pkg/model"
)

// PProfGoroutineAnalyzer analyzes Go pprof Goroutine profile data.
// It delegates to the perflib engine and adapts the business model.
type PProfGoroutineAnalyzer struct {
	engine *libanalyzer.PProfGoroutineAnalyzer
	config *BaseAnalyzerConfig
}

// NewPProfGoroutineAnalyzer creates a new pprof Goroutine analyzer.
func NewPProfGoroutineAnalyzer(config *BaseAnalyzerConfig) *PProfGoroutineAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}
	if config.AnalysisProfile == "" {
		config.AnalysisProfile = ProfileStandard
	}

	return &PProfGoroutineAnalyzer{
		engine: libanalyzer.NewPProfGoroutineAnalyzer(convertConfig(config)),
		config: config,
	}
}

// Name returns the analyzer name.
func (a *PProfGoroutineAnalyzer) Name() string {
	return a.engine.Name()
}

// Analyze performs pprof Goroutine analysis using an input file.
func (a *PProfGoroutineAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	libResp, err := a.engine.Analyze(ctx, convertRequest(req))
	if err != nil {
		return nil, err
	}
	return convertResponse(libResp, req.TaskUUID), nil
}

// AnalyzeFromReader performs pprof Goroutine analysis from a reader.
func (a *PProfGoroutineAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	libResp, err := a.engine.AnalyzeFromReader(ctx, convertRequest(req), dataReader)
	if err != nil {
		return nil, err
	}
	return convertResponse(libResp, req.TaskUUID), nil
}
