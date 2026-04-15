// Package model defines output data abstractions for different analysis types.
package model

// AnalysisRequest represents a request to analyze profiling data.
// This is the library-clean version without business-specific fields
// (no TaskID, COSBucket, MasterTaskTID, RequestParams).
type AnalysisRequest struct {
	// Mode is the analysis mode (e.g., "async-profiler-cpu", "pprof-heap").
	Mode string

	// InputFile is the path to the input profiling data file or directory.
	InputFile string

	// OutputDir is the directory where analysis output files will be written.
	OutputDir string

	// Options holds optional analysis parameters.
	Options map[string]interface{}
}

// AnalysisResponse represents the response from an analysis.
type AnalysisResponse struct {
	Mode         string         `json:"mode"`
	TotalRecords int            `json:"total_records"`
	OutputFiles  []OutputFile   `json:"output_files"`
	Data         AnalysisData   `json:"data"`
	Suggestions  []SuggestionItem `json:"suggestions"`
	Error        string         `json:"error,omitempty"`
}
