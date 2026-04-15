// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// GraphEdge represents an edge in the reference graph.
type GraphEdge = libhprof.GraphEdge

// ClassStatsAccumulator accumulates class statistics.
type ClassStatsAccumulator = libhprof.ClassStatsAccumulator

// ParallelBFSResult holds the result of parallel BFS traversal.
type ParallelBFSResult = libhprof.ParallelBFSResult

// ConcurrentMap is a thread-safe map wrapper.
type ConcurrentMap[K comparable, V any] = libhprof.ConcurrentMap[K, V]

// ============================================================================
// Function Forwarding
// ============================================================================

// BuildIncomingRefsParallel builds incoming references map in parallel.
func BuildIncomingRefsParallel(refs []GraphEdge) map[uint64][]ObjectReference {
	return libhprof.BuildIncomingRefsParallel(refs)
}

// ProcessObjectsParallel processes objects in parallel with a custom function.
func ProcessObjectsParallel[R any](
	g *ReferenceGraph,
	processor func(objID uint64, classID uint64, size int64) R,
	reducer func(results []R) R,
) R {
	return libhprof.ProcessObjectsParallel(g, processor, reducer)
}

// ComputeClassStatsParallel computes class statistics in parallel.
func ComputeClassStatsParallel(g *ReferenceGraph, includeRetained bool) map[uint64]ClassStatsAccumulator {
	return libhprof.ComputeClassStatsParallel(g, includeRetained)
}

// ParallelBFSFromRoots performs BFS from multiple roots in parallel.
func ParallelBFSFromRoots(
	g *ReferenceGraph,
	roots []uint64,
	maxDepth int,
	getNeighbors func(objID uint64) []uint64,
) ParallelBFSResult {
	return libhprof.ParallelBFSFromRoots(g, roots, maxDepth, getNeighbors)
}

// NewConcurrentMap creates a new concurrent map.
func NewConcurrentMap[K comparable, V any]() *ConcurrentMap[K, V] {
	return libhprof.NewConcurrentMap[K, V]()
}

// NewConcurrentMapWithCapacity creates a new concurrent map with initial capacity.
func NewConcurrentMapWithCapacity[K comparable, V any](capacity int) *ConcurrentMap[K, V] {
	return libhprof.NewConcurrentMapWithCapacity[K, V](capacity)
}

// Note: All methods on ConcurrentMap (Get, Set, Delete, Len, Range, ToMap, Update)
// are automatically available through the type alias.
