// Package model defines output data abstractions for different analysis types.
package model

import "encoding/json"

// ThreadInfo represents information about a thread.
type ThreadInfo struct {
	TID        int     `json:"tid"`
	ThreadName string  `json:"thread_name"`
	Samples    int64   `json:"samples"`
	Percentage float64 `json:"percentage"`
}

// TopFuncsMap is a map of function name to its sample count/percentage.
type TopFuncsMap map[string]TopFuncValue

// TopFuncValue holds the value for a top function entry.
type TopFuncValue struct {
	Self  float64 `json:"self"`
	Total float64 `json:"total,omitempty"`
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
	Suggestions        []SuggestionItem          `json:"suggestions,omitempty"`
}

// SuggestionItem represents a single suggestion from analysis.
// This is the library-clean version without DB tags or business fields.
type SuggestionItem struct {
	Type         string          `json:"type,omitempty"`
	Severity     string          `json:"severity,omitempty"`
	Suggestion   string          `json:"suggestion"`
	FuncName     string          `json:"func,omitempty"`
	Namespace    string          `json:"namespace,omitempty"`
	CallStack    json.RawMessage `json:"callstack,omitempty"`
	AISuggestion string          `json:"ai_suggestion,omitempty"`
}
