package analyzer

import (
	"context"
	"io"

	libanalyzer "github.com/perf-analysis/perflib/analyzer"
	"github.com/perf-analysis/pkg/model"
)

// PProfHeapAnalyzer analyzes Go pprof Heap profile data.
// It delegates to the perflib engine and adapts the business model.
type PProfHeapAnalyzer struct {
	engine *libanalyzer.PProfHeapAnalyzer
	config *BaseAnalyzerConfig
}

// NewPProfHeapAnalyzer creates a new pprof Heap analyzer.
func NewPProfHeapAnalyzer(config *BaseAnalyzerConfig) *PProfHeapAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}
	if config.AnalysisProfile == "" {
		config.AnalysisProfile = ProfileStandard
	}

	return &PProfHeapAnalyzer{
		engine: libanalyzer.NewPProfHeapAnalyzer(convertConfig(config)),
		config: config,
	}
}

// Name returns the analyzer name.
func (a *PProfHeapAnalyzer) Name() string {
	return a.engine.Name()
}

// Analyze performs pprof Heap analysis using an input file.
func (a *PProfHeapAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	libResp, err := a.engine.Analyze(ctx, convertRequest(req))
	if err != nil {
		return nil, err
	}
	return convertResponse(libResp, req.TaskUUID), nil
}

// AnalyzeFromReader performs pprof Heap analysis from a reader.
func (a *PProfHeapAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	libResp, err := a.engine.AnalyzeFromReader(ctx, convertRequest(req), dataReader)
	if err != nil {
		return nil, err
	}
	return convertResponse(libResp, req.TaskUUID), nil
}
