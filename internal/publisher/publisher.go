// Package publisher provides a unified layer for publishing analysis result files
// to storage (COS or local filesystem). Both the CLI and Scheduler share this
// module so that summary generation, detail-file generation, and output-file
// upload logic are maintained in a single place.
package publisher

import (
	"context"

	"github.com/perf-analysis/internal/storage"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// ResultPublisher defines the interface for publishing analysis results to storage.
type ResultPublisher interface {
	// Publish writes all result files (output files, summary, detail files)
	// to the underlying storage and returns the publish output.
	Publish(ctx context.Context, req *PublishRequest) (*PublishOutput, error)
}

// PublishRequest holds everything needed to publish analysis results.
type PublishRequest struct {
	TaskUUID        string
	Mode            string
	TaskDir         string // local directory where analyzer wrote files
	Response        *model.AnalysisResponse
	Suggestions     []model.SuggestionItem
	AnalysisVersion string
	ModeDescription string // human-readable mode description (e.g. "Java CPU hotspot analysis")
	Profile         string // analysis depth: quick / standard / detailed
	InputFile       string // original input file name (base name only)
	AnalysisTimeMs  int64  // analysis duration in milliseconds; 0 means not recorded
}

// PublishOutput captures what was published.
type PublishOutput struct {
	// UploadedFiles maps output-file name to storage key.
	UploadedFiles map[string]string
	// Summary is the generated summary.json content (nil if generation failed).
	Summary map[string]interface{}
}

// New creates a DefaultResultPublisher.
func New(s storage.Storage, logger utils.Logger) *DefaultResultPublisher {
	return NewDefaultResultPublisher(s, logger)
}
