package analyzer

import (
	"context"
	"io"

	libanalyzer "github.com/junjiewwang/perf-analysis/perflib/analyzer"
	"github.com/junjiewwang/perf-analysis/pkg/model"
)

// PProfCPUAnalyzer analyzes Go pprof CPU profile data.
// It delegates to the perflib engine and adapts the business model.
type PProfCPUAnalyzer struct {
	engine *libanalyzer.PProfCPUAnalyzer
	config *BaseAnalyzerConfig
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
		engine: libanalyzer.NewPProfCPUAnalyzer(convertConfig(config)),
		config: config,
	}
}

// Name returns the analyzer name.
func (a *PProfCPUAnalyzer) Name() string {
	return a.engine.Name()
}

// Analyze performs pprof CPU analysis using an input file.
func (a *PProfCPUAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	libResp, err := a.engine.Analyze(ctx, convertRequest(req))
	if err != nil {
		return nil, err
	}
	return convertResponse(libResp, req.TaskUUID), nil
}

// AnalyzeFromReader performs pprof CPU analysis from a reader.
func (a *PProfCPUAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	libResp, err := a.engine.AnalyzeFromReader(ctx, convertRequest(req), dataReader)
	if err != nil {
		return nil, err
	}
	return convertResponse(libResp, req.TaskUUID), nil
}
