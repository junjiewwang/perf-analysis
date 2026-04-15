// Package analyzer defines the core analyzer interfaces.
package analyzer

import (
	"context"
	"io"

	"github.com/junjiewwang/perf-analysis/pkg/model"
)

// Analyzer is the interface for all profiling data analyzers.
type Analyzer interface {
	// Analyze performs the analysis on the given request.
	Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error)

	// AnalyzeFromReader performs the analysis using a reader.
	AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error)

	// Name returns the name of this analyzer.
	Name() string
}
