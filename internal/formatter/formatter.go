// Package formatter provides result formatting for different analysis types.
package formatter

import (
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// ResultFormatter is the interface for formatting analysis results.
type ResultFormatter interface {
	// Format outputs the analysis result to the logger.
	Format(resp *model.AnalysisResponse, log utils.Logger)

	// FormatSummary returns a summary map for serialization.
	FormatSummary(resp *model.AnalysisResponse) map[string]interface{}

	// SupportedTypes returns the data types this formatter supports.
	SupportedTypes() []model.AnalysisDataType
}

// Registry manages formatter instances.
type Registry struct {
	formatters map[model.AnalysisDataType]ResultFormatter
	fallback   ResultFormatter
}

// NewRegistry creates a new formatter registry with default formatters.
func NewRegistry() *Registry {
	r := &Registry{
		formatters: make(map[model.AnalysisDataType]ResultFormatter),
		fallback:   &DefaultFormatter{},
	}

	// Register default formatters
	r.Register(&CPUFormatter{})
	r.Register(&AllocationFormatter{})
	r.Register(&HeapFormatter{})
	r.Register(&MemLeakFormatter{})
	r.Register(&TracingFormatter{})
	r.Register(&PProfBatchFormatter{})

	return r
}

// Register registers a formatter.
func (r *Registry) Register(f ResultFormatter) {
	for _, t := range f.SupportedTypes() {
		r.formatters[t] = f
	}
}

// Get returns the formatter for a data type.
func (r *Registry) Get(dataType model.AnalysisDataType) ResultFormatter {
	if f, ok := r.formatters[dataType]; ok {
		return f
	}
	return r.fallback
}

// Format formats the analysis response using the appropriate formatter.
func (r *Registry) Format(resp *model.AnalysisResponse, log utils.Logger) {
	if resp == nil {
		return
	}

	if resp.Data == nil {
		r.fallback.Format(resp, log)
		return
	}

	f := r.Get(resp.Data.Type())
	f.Format(resp, log)
}

// FormatSummary returns a summary map using the appropriate formatter.
func (r *Registry) FormatSummary(resp *model.AnalysisResponse) map[string]interface{} {
	if resp == nil {
		return nil
	}

	if resp.Data == nil {
		return r.fallback.FormatSummary(resp)
	}

	f := r.Get(resp.Data.Type())
	return f.FormatSummary(resp)
}
