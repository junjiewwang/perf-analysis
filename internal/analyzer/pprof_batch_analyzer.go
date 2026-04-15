package analyzer

import (
	"context"
	"fmt"
	"io"
	"time"

	libanalyzer "github.com/perf-analysis/perflib/analyzer"
	pprofparser "github.com/perf-analysis/internal/parser/pprof"
	"github.com/perf-analysis/pkg/model"
)

// PProfBatchAnalyzer analyzes a directory of pprof files.
// It delegates to the perflib engine and adapts the business model.
type PProfBatchAnalyzer struct {
	engine *libanalyzer.PProfBatchAnalyzer
	config *BaseAnalyzerConfig
}

// NewPProfBatchAnalyzer creates a new PProfBatchAnalyzer.
func NewPProfBatchAnalyzer(config *BaseAnalyzerConfig) *PProfBatchAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}

	return &PProfBatchAnalyzer{
		engine: libanalyzer.NewPProfBatchAnalyzer(convertConfig(config)),
		config: config,
	}
}

// Name returns the name of this analyzer.
func (a *PProfBatchAnalyzer) Name() string {
	return a.engine.Name()
}

// Analyze performs batch analysis on a pprof directory.
func (a *PProfBatchAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	libResp, err := a.engine.Analyze(ctx, convertRequest(req))
	if err != nil {
		return nil, err
	}
	return convertResponse(libResp, req.TaskUUID), nil
}

// AnalyzeFromReader is not supported for batch analysis.
func (a *PProfBatchAnalyzer) AnalyzeFromReader(_ context.Context, _ *model.AnalysisRequest, _ io.Reader) (*model.AnalysisResponse, error) {
	return nil, fmt.Errorf("PProfBatchAnalyzer does not support reading from a single reader, use Analyze with a directory path")
}

// ProfileSetResult represents analysis results for a set of profiles.
// Retained for backward compatibility.
type ProfileSetResult struct {
	ProfileType  string                 `json:"profile_type"`
	FileCount    int                    `json:"file_count"`
	Files        []string               `json:"files"`
	LatestFile   string                 `json:"latest_file"`
	TotalSamples int64                  `json:"total_samples"`
	OutputFiles  []string               `json:"output_files"`
	LeakReport   *pprofparser.LeakReport `json:"leak_report,omitempty"`
}

// BatchAnalysisResult represents the complete batch analysis result.
// Retained for backward compatibility.
type BatchAnalysisResult struct {
	BaseDir      string                              `json:"base_dir"`
	OutputDir    string                              `json:"output_dir"`
	AnalyzedAt   time.Time                           `json:"analyzed_at"`
	ProfileSets  map[string]*ProfileSetResult        `json:"profile_sets"`
	LeakReports  map[string]*pprofparser.LeakReport  `json:"leak_reports,omitempty"`
	TotalSamples int64                               `json:"total_samples"`
}
