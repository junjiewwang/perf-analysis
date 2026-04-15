// Package model defines output data abstractions for different analysis types.
package model

import "sort"

// CPUProfilingData holds CPU profiling analysis data.
type CPUProfilingData struct {
	FlameGraphFile string       `json:"flamegraph_file"`
	CallGraphFile  string       `json:"callgraph_file"`
	ThreadStats    []ThreadInfo `json:"thread_stats"`
	TopFuncs       TopFuncsMap  `json:"top_funcs"`
	TotalSamples   int64        `json:"total_samples"`
}

// Type returns the analysis data type.
func (d *CPUProfilingData) Type() AnalysisDataType {
	return DataTypeCPUProfiling
}

// Summary returns a summary of the CPU profiling analysis.
func (d *CPUProfilingData) Summary() map[string]interface{} {
	return map[string]interface{}{
		"total_samples":   d.TotalSamples,
		"thread_count":    len(d.ThreadStats),
		"flamegraph_file": d.FlameGraphFile,
		"callgraph_file":  d.CallGraphFile,
	}
}

// TopItems returns the top functions from CPU profiling.
func (d *CPUProfilingData) TopItems() []TopItem {
	items := make([]TopItem, 0, len(d.TopFuncs))
	for name, val := range d.TopFuncs {
		items = append(items, TopItem{
			Name:       name,
			Percentage: val.Self,
		})
	}
	sortTopItemsByPercentage(items)
	return items
}

// AllocationData holds memory allocation analysis data.
type AllocationData struct {
	FlameGraphFile   string       `json:"flamegraph_file"`
	CallGraphFile    string       `json:"callgraph_file"`
	ThreadStats      []ThreadInfo `json:"thread_stats"`
	TopAllocators    TopFuncsMap  `json:"top_allocators"`
	TotalAllocations int64        `json:"total_allocations"`
	TotalBytes       int64        `json:"total_bytes,omitempty"`
}

// Type returns the analysis data type.
func (d *AllocationData) Type() AnalysisDataType {
	return DataTypeAllocation
}

// Summary returns a summary of the allocation analysis.
func (d *AllocationData) Summary() map[string]interface{} {
	return map[string]interface{}{
		"total_allocations": d.TotalAllocations,
		"total_bytes":       d.TotalBytes,
		"thread_count":      len(d.ThreadStats),
		"flamegraph_file":   d.FlameGraphFile,
		"callgraph_file":    d.CallGraphFile,
	}
}

// TopItems returns the top allocators from allocation analysis.
func (d *AllocationData) TopItems() []TopItem {
	items := make([]TopItem, 0, len(d.TopAllocators))
	for name, val := range d.TopAllocators {
		items = append(items, TopItem{
			Name:       name,
			Percentage: val.Self,
		})
	}
	sortTopItemsByPercentage(items)
	return items
}

// MemoryLeakData holds memory leak analysis data.
type MemoryLeakData struct {
	LeakReportFile string       `json:"leak_report_file"`
	AllocationFile string       `json:"allocation_file"`
	LeakSuspects   []LeakInfo   `json:"leak_suspects"`
	TopAllocators  TopFuncsMap  `json:"top_allocators"`
	TotalLeakBytes int64        `json:"total_leak_bytes"`
	TotalLeakCount int64        `json:"total_leak_count"`
	ThreadStats    []ThreadInfo `json:"thread_stats,omitempty"`
}

// LeakInfo holds information about a potential memory leak.
type LeakInfo struct {
	Location    string  `json:"location"`
	LeakBytes   int64   `json:"leak_bytes"`
	LeakCount   int64   `json:"leak_count"`
	Percentage  float64 `json:"percentage"`
	Description string  `json:"description,omitempty"`
}

// Type returns the analysis data type.
func (d *MemoryLeakData) Type() AnalysisDataType {
	return DataTypeMemoryLeak
}

// Summary returns a summary of the memory leak analysis.
func (d *MemoryLeakData) Summary() map[string]interface{} {
	return map[string]interface{}{
		"total_leak_bytes": d.TotalLeakBytes,
		"total_leak_count": d.TotalLeakCount,
		"suspect_count":    len(d.LeakSuspects),
		"leak_report_file": d.LeakReportFile,
		"allocation_file":  d.AllocationFile,
	}
}

// TopItems returns the top leak suspects.
func (d *MemoryLeakData) TopItems() []TopItem {
	items := make([]TopItem, 0, len(d.LeakSuspects))
	for _, leak := range d.LeakSuspects {
		items = append(items, TopItem{
			Name:       leak.Location,
			Value:      leak.LeakBytes,
			Percentage: leak.Percentage,
			Extra: map[string]interface{}{
				"leak_count":  leak.LeakCount,
				"description": leak.Description,
			},
		})
	}
	return items
}

// TracingData holds tracing analysis data.
type TracingData struct {
	FlameGraphFile string       `json:"flamegraph_file"`
	CallGraphFile  string       `json:"callgraph_file"`
	ThreadStats    []ThreadInfo `json:"thread_stats"`
	TopFuncs       TopFuncsMap  `json:"top_funcs"`
	TotalSamples   int64        `json:"total_samples"`
}

// Type returns the analysis data type.
func (d *TracingData) Type() AnalysisDataType {
	return DataTypeTracing
}

// Summary returns a summary of the tracing analysis.
func (d *TracingData) Summary() map[string]interface{} {
	return map[string]interface{}{
		"total_samples":   d.TotalSamples,
		"thread_count":    len(d.ThreadStats),
		"flamegraph_file": d.FlameGraphFile,
		"callgraph_file":  d.CallGraphFile,
	}
}

// TopItems returns the top functions from tracing.
func (d *TracingData) TopItems() []TopItem {
	items := make([]TopItem, 0, len(d.TopFuncs))
	for name, val := range d.TopFuncs {
		items = append(items, TopItem{
			Name:       name,
			Percentage: val.Self,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Percentage > items[j].Percentage
	})
	return items
}
