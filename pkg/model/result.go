package model

import (
	"encoding/json"
	"time"
)

// AnalysisResult represents the result of an analysis task.
type AnalysisResult struct {
	TaskUUID                string                     `json:"tid"`
	ContainersInfo          map[string]ContainerInfo   `json:"containers_info"`
	Result                  map[string]NamespaceResult `json:"result"`
	Version                 string                     `json:"version"`
	TotalRecords            int64                      `json:"total_records"`
	TotalRecordsWithSwapper int64                      `json:"total_records_with_swapper"`
	AnalyzedAt              time.Time                  `json:"analyzed_at"`
}

// ContainerInfo holds container metadata.
type ContainerInfo struct {
	ContainerID   string `json:"container_id,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	PodName       string `json:"pod_name,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
}

// NamespaceResult holds the analysis result for a specific namespace/container.
type NamespaceResult struct {
	TopFuncs               string          `json:"top_funcs"`
	TopFuncsCallstacks     json.RawMessage `json:"top_funcs_callstacks,omitempty"`
	ActiveThreadsJSON      string          `json:"active_threads_json"`
	FlameGraphFile         string          `json:"flamegraph_file"`
	ExtendedFlameGraphFile string          `json:"extended_flamegraph_file"`
	CallGraphFile          string          `json:"callgraph_file"`
	Suggestions            []Suggestion    `json:"suggestions"`
	TotalRecords           int64           `json:"total_records"`
}

// AnalysisRequest represents a request to analyze profiling data.
// This is the business version with task-specific fields.
// For the library-clean version, use perflib/model.AnalysisRequest.
type AnalysisRequest struct {
	TaskID        int64
	TaskUUID      string
	Mode          string
	InputFile     string
	OutputDir     string
	ResultFile    string
	UserName      string
	MasterTaskTID *string
	COSBucket     string
	RequestParams RequestParams
}

// AnalysisResponse represents the response from an analysis.
type AnalysisResponse struct {
	TaskUUID     string           `json:"task_uuid"`
	Mode         string           `json:"mode"`
	TotalRecords int              `json:"total_records"`
	OutputFiles  []OutputFile     `json:"output_files"`
	Data         AnalysisData     `json:"data"`
	Suggestions  []SuggestionItem `json:"suggestions"`
	Error        string           `json:"error,omitempty"`
}

// AnalysisContext holds the context during analysis.
type AnalysisContext struct {
	ActiveThreadJSON        string         `json:"active_thread_json"`
	CallGraphFile           string         `json:"callgraph_file"`
	ExtendedFlameGraphFile  string         `json:"extended_flamegraph_file"`
	FlameGraphFile          string         `json:"flamegraph_file"`
	Suggestions             []Suggestion   `json:"suggestions"`
	TopFuncs                string         `json:"top_funcs"`
	TopFuncsWithSwapper     string         `json:"top_funcs_with_swapper"`
	TotalRecords            int64          `json:"total_records"`
	TotalRecordsWithSwapper int64          `json:"total_records_with_swapper"`
	TID                     string         `json:"tid"`
	Mode                    string         `json:"mode"`
	Status                  TaskStatus     `json:"status"`
	StatusInfo              string         `json:"status_info"`
	CreateTime              int64          `json:"create_time"`
	BeginTime               int64          `json:"begin_time"`
	EndTime                 int64          `json:"end_time"`
	AnalysisStatus          AnalysisStatus `json:"analysis_status"`
}

// NewAnalysisContext creates a new AnalysisContext with default values.
func NewAnalysisContext() *AnalysisContext {
	return &AnalysisContext{
		Suggestions:    make([]Suggestion, 0),
		AnalysisStatus: AnalysisStatusPending,
	}
}

// SetFromNamespaceResult updates context from namespace result.
func (ctx *AnalysisContext) SetFromNamespaceResult(ns *NamespaceResult) {
	ctx.ActiveThreadJSON = ns.ActiveThreadsJSON
	ctx.CallGraphFile = ns.CallGraphFile
	ctx.ExtendedFlameGraphFile = ns.ExtendedFlameGraphFile
	ctx.FlameGraphFile = ns.FlameGraphFile
	ctx.TopFuncs = ns.TopFuncs
	ctx.TopFuncsWithSwapper = ns.TopFuncs
	ctx.TotalRecords = ns.TotalRecords
	ctx.TotalRecordsWithSwapper = ns.TotalRecords
	ctx.Suggestions = ns.Suggestions
}
