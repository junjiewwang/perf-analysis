// Package query provides reusable query utilities for analysis data.
// This file implements the ClassHistogram and HeapStats queries for heap analysis.
package query

import (
	"container/heap"
	"sort"

	"github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// ClassHistogramEntry represents a single entry in the class histogram.
type ClassHistogramEntry struct {
	ClassName    string  `json:"class_name"`
	ObjectCount  int64   `json:"object_count"`
	ShallowSize  int64   `json:"shallow_size"`
	RetainedSize int64   `json:"retained_size"`
	Percentage   float64 `json:"percentage"` // Percentage of total retained size
}

// ClassHistogramResult represents the complete class histogram response.
type ClassHistogramResult struct {
	TotalClasses int                   `json:"total_classes"`
	TotalObjects int64                 `json:"total_objects"`
	TotalSize    int64                 `json:"total_size"` // Total retained size
	Classes      []ClassHistogramEntry `json:"classes"`
}

// HeapStatsResult represents high-level heap statistics.
type HeapStatsResult struct {
	TotalHeapSize   int64  `json:"total_heap_size"`
	TotalObjects    int64  `json:"total_objects"`
	TotalClasses    int    `json:"total_classes"`
	TotalGCRoots    int    `json:"total_gc_roots"`
	MaxObjectSize   int64  `json:"max_object_size"`
	MaxRetainedSize int64  `json:"max_retained_size"`
	TopClassName    string `json:"top_class_name"`
}

// HeapQueryHelper provides reusable heap query operations on a HeapGraph.
// This is a perflib-level utility that can be used by any consumer.
type HeapQueryHelper struct {
	graph hprof.HeapGraph
}

// NewHeapQueryHelper creates a new HeapQueryHelper from a HeapGraph.
func NewHeapQueryHelper(graph hprof.HeapGraph) *HeapQueryHelper {
	return &HeapQueryHelper{graph: graph}
}

// QueryClassHistogram returns class-level aggregated statistics.
// Parameters:
//   - sortBy: "retained" (default), "shallow", "count"
//   - topN: maximum number of classes to return (0 = all)
//   - filter: optional class name substring filter (case-insensitive)
func (h *HeapQueryHelper) QueryClassHistogram(sortBy string, topN int, filter string) *ClassHistogramResult {
	if topN <= 0 {
		topN = 100
	}

	// Aggregate by class
	type classStats struct {
		classID      uint64
		className    string
		objectCount  int64
		shallowSize  int64
		retainedSize int64
	}

	classMap := make(map[uint64]*classStats)
	var totalObjects int64
	var totalRetained int64

	objectCount := h.graph.ObjectCount()
	for i := int32(0); i < objectCount; i++ {
		if !h.graph.IsReachable(i) {
			continue
		}

		classID := h.graph.GetClassID(i)
		shallow := h.graph.GetShallowSize(i)
		retained := h.graph.GetRetainedSize(i)

		totalObjects++
		totalRetained += retained

		if cs, ok := classMap[classID]; ok {
			cs.objectCount++
			cs.shallowSize += shallow
			cs.retainedSize += retained
		} else {
			className := h.graph.GetClassName(classID)
			classMap[classID] = &classStats{
				classID:      classID,
				className:    className,
				objectCount:  1,
				shallowSize:  shallow,
				retainedSize: retained,
			}
		}
	}

	// Filter if needed
	var filtered []*classStats
	if filter != "" {
		filterLower := toLower(filter)
		for _, cs := range classMap {
			if containsLower(cs.className, filterLower) {
				filtered = append(filtered, cs)
			}
		}
	} else {
		filtered = make([]*classStats, 0, len(classMap))
		for _, cs := range classMap {
			filtered = append(filtered, cs)
		}
	}

	// Sort
	switch sortBy {
	case "shallow":
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].shallowSize > filtered[j].shallowSize
		})
	case "count":
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].objectCount > filtered[j].objectCount
		})
	default: // "retained"
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].retainedSize > filtered[j].retainedSize
		})
	}

	// Limit
	if topN > 0 && topN < len(filtered) {
		filtered = filtered[:topN]
	}

	// Build result
	entries := make([]ClassHistogramEntry, len(filtered))
	for i, cs := range filtered {
		var pct float64
		if totalRetained > 0 {
			pct = float64(cs.retainedSize) * 100.0 / float64(totalRetained)
		}
		entries[i] = ClassHistogramEntry{
			ClassName:    cs.className,
			ObjectCount:  cs.objectCount,
			ShallowSize:  cs.shallowSize,
			RetainedSize: cs.retainedSize,
			Percentage:   pct,
		}
	}

	return &ClassHistogramResult{
		TotalClasses: len(classMap),
		TotalObjects: totalObjects,
		TotalSize:    totalRetained,
		Classes:      entries,
	}
}

// QueryHeapStats returns high-level heap statistics.
func (h *HeapQueryHelper) QueryHeapStats() *HeapStatsResult {
	result := &HeapStatsResult{}

	classSet := make(map[uint64]struct{})
	var topClassID uint64
	var topClassRetained int64

	objectCount := h.graph.ObjectCount()
	for i := int32(0); i < objectCount; i++ {
		if !h.graph.IsReachable(i) {
			continue
		}

		result.TotalObjects++

		shallow := h.graph.GetShallowSize(i)
		retained := h.graph.GetRetainedSize(i)
		result.TotalHeapSize += shallow

		if retained > result.MaxRetainedSize {
			result.MaxRetainedSize = retained
			result.MaxObjectSize = shallow
		}

		classID := h.graph.GetClassID(i)
		classSet[classID] = struct{}{}

		if h.graph.IsGCRoot(i) {
			result.TotalGCRoots++
		}

		// Track top class by retained
		if retained > topClassRetained {
			topClassRetained = retained
			topClassID = classID
		}
	}

	result.TotalClasses = len(classSet)
	if topClassID != 0 {
		result.TopClassName = h.graph.GetClassName(topClassID)
	}

	return result
}

// containsLower checks if s contains substr (both already lowered).
func containsLower(s, substrLower string) bool {
	return len(s) >= len(substrLower) && containsIgnoreCase(s, substrLower)
}

// containsIgnoreCase checks if s contains substr ignoring case.
func containsIgnoreCase(s, substr string) bool {
	sLower := toLower(s)
	return len(sLower) >= len(substr) && indexOf(sLower, substr) >= 0
}

// toLower converts a string to lowercase without importing strings.
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// indexOf returns the index of substr in s, or -1 if not found.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// histogramEntry is used by the min-heap for top-N selection.
type histogramEntry struct {
	classID   uint64
	sortValue int64
}

// histogramHeap implements heap.Interface for top-N selection (min-heap).
type histogramHeap []histogramEntry

func (h histogramHeap) Len() int            { return len(h) }
func (h histogramHeap) Less(i, j int) bool  { return h[i].sortValue < h[j].sortValue }
func (h histogramHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *histogramHeap) Push(x interface{}) { *h = append(*h, x.(histogramEntry)) }
func (h *histogramHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// Ensure heap interface is satisfied at compile time.
var _ heap.Interface = (*histogramHeap)(nil)
