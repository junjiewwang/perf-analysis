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

// ThreadInfo represents information about a thread.
type ThreadInfo struct {
	TID        int     `json:"tid"`
	ThreadName string  `json:"thread_name"`
	Samples    int64   `json:"samples"`
	Percentage float64 `json:"percentage"`
}

// TopFunction represents a hot function with its statistics.
type TopFunction struct {
	Name         string  `json:"name"`
	Module       string  `json:"module,omitempty"`
	SelfSamples  int64   `json:"self"`
	SelfPercent  float64 `json:"self_pct"`
	TotalSamples int64   `json:"total,omitempty"`
	TotalPercent float64 `json:"total_pct,omitempty"`
}

// TopFuncsMap is a map of function name to its sample count/percentage.
type TopFuncsMap map[string]TopFuncValue

// TopFuncValue holds the value for a top function entry.
type TopFuncValue struct {
	Self  float64 `json:"self"`
	Total float64 `json:"total,omitempty"`
}

// CallStackInfo holds call stack information for a top function.
type CallStackInfo struct {
	FunctionName string   `json:"func"`
	CallStacks   []string `json:"callstacks"`
	Count        int      `json:"count"`
}

// Sample represents a single profiling sample.
type Sample struct {
	ThreadName string   `json:"thread_name"`
	TID        int      `json:"tid,omitempty"`
	CallStack  []string `json:"callstack"`
	Value      int64    `json:"value"`
}

// ParseResult holds the result of parsing profiling data.
type ParseResult struct {
	Samples            []*Sample                 `json:"samples"`
	TotalSamples       int64                     `json:"total_samples"`
	ThreadStats        map[string]*ThreadInfo    `json:"thread_stats"`
	TopFuncs           TopFuncsMap               `json:"top_funcs"`
	TopFuncsCallstacks map[string]*CallStackInfo `json:"top_funcs_callstacks,omitempty"`
	Suggestions        []Suggestion              `json:"suggestions,omitempty"`
}

// AnalysisRequest represents a request to analyze profiling data.
type AnalysisRequest struct {
	TaskID        int64
	TaskUUID      string
	TaskType      TaskType
	ProfilerType  ProfilerType
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
	TaskType     TaskType         `json:"task_type"`
	TotalRecords int              `json:"total_records"`
	OutputFiles  []OutputFile     `json:"output_files"`
	Data         AnalysisData     `json:"data"`
	Suggestions  []SuggestionItem `json:"suggestions"`
	Error        string           `json:"error,omitempty"`
}

// SuggestionItem represents a single suggestion from analysis.
type SuggestionItem struct {
	Suggestion   string `json:"suggestion"`
	FuncName     string `json:"func,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
	CallStack    string `json:"callstack,omitempty"`
	AISuggestion string `json:"ai_suggestion,omitempty"`
}

// AnalysisContext holds the context during analysis.
type AnalysisContext struct {
	ActiveThreadJSON        string                 `json:"active_thread_json"`
	CallGraphFile           string                 `json:"callgraph_file"`
	ExtendedFlameGraphFile  string                 `json:"extended_flamegraph_file"`
	FlameGraphFile          string                 `json:"flamegraph_file"`
	Suggestions             []Suggestion           `json:"suggestions"`
	TopFuncs                string                 `json:"top_funcs"`
	TopFuncsWithSwapper     string                 `json:"top_funcs_with_swapper"`
	TotalRecords            int64                  `json:"total_records"`
	TotalRecordsWithSwapper int64                  `json:"total_records_with_swapper"`
	TID                     string                 `json:"tid"`
	Type                    TaskType               `json:"type"`
	ProfilerType            ProfilerType           `json:"profiler_type"`
	Status                  TaskStatus             `json:"status"`
	StatusInfo              string                 `json:"status_info"`
	CreateTime              int64                  `json:"create_time"`
	BeginTime               int64                  `json:"begin_time"`
	EndTime                 int64                  `json:"end_time"`
	AnalysisStatus          AnalysisStatus         `json:"analysis_status"`
	APMConfig               map[string]interface{} `json:"apm_config"`
}

// NewAnalysisContext creates a new AnalysisContext with default values.
func NewAnalysisContext() *AnalysisContext {
	return &AnalysisContext{
		Suggestions:    make([]Suggestion, 0),
		APMConfig:      make(map[string]interface{}),
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
