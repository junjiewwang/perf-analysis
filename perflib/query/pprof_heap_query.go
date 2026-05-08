// Package query provides reusable query utilities for analysis data.
// This file implements the Go pprof heap allocation histogram query.
package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/junjiewwang/perf-analysis/perflib/model"
	"github.com/junjiewwang/perf-analysis/perflib/output"
)

// PProfHeapQueryHelper provides query operations on Go pprof heap analysis data.
// It loads precomputed PProfHeapData and provides filtering, sorting, and
// aggregation to serve the unified /api/heap/histogram endpoint.
type PProfHeapQueryHelper struct {
	data *model.PProfHeapData
}

// NewPProfHeapQueryHelper creates a new helper from PProfHeapData.
func NewPProfHeapQueryHelper(data *model.PProfHeapData) *PProfHeapQueryHelper {
	return &PProfHeapQueryHelper{data: data}
}

// LoadPProfHeapQueryHelper loads pprof heap analysis data from a JSON file.
func LoadPProfHeapQueryHelper(filePath string) (*PProfHeapQueryHelper, error) {
	var data model.PProfHeapData
	if err := output.ReadJSON(filePath, &data); err != nil {
		return nil, fmt.Errorf("failed to load pprof heap analysis: %w", err)
	}
	return NewPProfHeapQueryHelper(&data), nil
}

// AllocHistogramResponse is the unified response for heap histogram queries.
// It covers both Java hprof (class-based) and Go pprof (function-based) scenarios.
type AllocHistogramResponse struct {
	Source     string                `json:"source"`      // "java_hprof" | "go_pprof"
	Metric     string                `json:"metric"`      // e.g. "inuse_space", "retained_size"
	Total      int64                 `json:"total"`
	Unit       string                `json:"unit"`        // "bytes" | "objects"
	EntryCount int                   `json:"entry_count"` // Total entries before limiting
	Entries    []AllocHistogramEntry `json:"entries"`
}

// AllocHistogramEntry is a single entry in the allocation histogram.
type AllocHistogramEntry struct {
	Name    string  `json:"name"`              // Java: className, Go: funcName
	Flat    int64   `json:"flat"`              // Java: shallowSize, Go: flat
	FlatPct float64 `json:"flat_pct"`
	Cum     int64   `json:"cum"`              // Java: retainedSize, Go: cum
	CumPct  float64 `json:"cum_pct"`
	Count   int64   `json:"count,omitempty"`  // Java: objectCount, Go: not applicable
	Module  string  `json:"module,omitempty"` // Go: package path
}

// QueryAllocHistogram returns the allocation histogram for a given metric.
// Parameters:
//   - metric: "inuse_space" (default), "inuse_objects", "alloc_space", "alloc_objects"
//   - sortBy: "flat" (default), "cum", "flat_pct"
//   - topN: maximum entries (0 = default 100)
//   - filter: optional function name substring filter (case-insensitive)
func (h *PProfHeapQueryHelper) QueryAllocHistogram(metric, sortBy string, topN int, filter string) *AllocHistogramResponse {
	if h.data == nil {
		return &AllocHistogramResponse{Source: "go_pprof", Entries: []AllocHistogramEntry{}}
	}

	if topN <= 0 {
		topN = 100
	}

	// Select metric
	var stats *model.PProfMemoryStats
	switch strings.ToLower(metric) {
	case "inuse_objects":
		stats = h.data.InuseObjects
	case "alloc_space":
		stats = h.data.AllocSpace
	case "alloc_objects":
		stats = h.data.AllocObjects
	default: // "inuse_space" or empty
		metric = "inuse_space"
		stats = h.data.InuseSpace
	}

	if stats == nil {
		return &AllocHistogramResponse{
			Source:  "go_pprof",
			Metric:  metric,
			Entries: []AllocHistogramEntry{},
		}
	}

	// Build entries from TopFuncs
	entries := make([]AllocHistogramEntry, 0, len(stats.TopFuncs))
	for _, tf := range stats.TopFuncs {
		// Apply filter
		if filter != "" && !strings.Contains(strings.ToLower(tf.Name), strings.ToLower(filter)) {
			continue
		}
		entries = append(entries, AllocHistogramEntry{
			Name:    tf.Name,
			Flat:    tf.Flat,
			FlatPct: tf.FlatPct,
			Cum:     tf.Cum,
			CumPct:  tf.CumPct,
			Module:  tf.Module,
		})
	}

	totalEntries := len(entries)

	// Sort
	switch strings.ToLower(sortBy) {
	case "cum":
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Cum > entries[j].Cum
		})
	case "flat_pct":
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].FlatPct > entries[j].FlatPct
		})
	default: // "flat"
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Flat > entries[j].Flat
		})
	}

	// Limit
	if topN > 0 && topN < len(entries) {
		entries = entries[:topN]
	}

	return &AllocHistogramResponse{
		Source:     "go_pprof",
		Metric:     metric,
		Total:      stats.Total,
		Unit:       stats.Unit,
		EntryCount: totalEntries,
		Entries:    entries,
	}
}

// QueryHeapStats returns high-level heap statistics from pprof data.
func (h *PProfHeapQueryHelper) QueryHeapStats() *HeapStatsResult {
	if h.data == nil {
		return &HeapStatsResult{}
	}

	result := &HeapStatsResult{}
	if h.data.HeapSummary != nil {
		result.TotalHeapSize = h.data.HeapSummary.TotalInuseBytes
		result.TotalObjects = h.data.HeapSummary.TotalInuseObjects
	}

	// Find top function by flat allocation
	if h.data.InuseSpace != nil && len(h.data.InuseSpace.TopFuncs) > 0 {
		result.TopClassName = h.data.InuseSpace.TopFuncs[0].Name // "TopClassName" reused for top func
		result.TotalClasses = h.data.InuseSpace.TopNCount
	}

	return result
}
