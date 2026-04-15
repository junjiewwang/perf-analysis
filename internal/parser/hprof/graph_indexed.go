// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// IndexedObjectStore provides compact indexed storage for objects.
type IndexedObjectStore = libhprof.IndexedObjectStore

// CompactEdgeList stores edges in CSR format.
type CompactEdgeList = libhprof.CompactEdgeList

// CompactEdgeListBuilder builds a CompactEdgeList.
type CompactEdgeListBuilder = libhprof.CompactEdgeListBuilder

// IndexedReferenceGraph provides a high-performance indexed reference graph.
type IndexedReferenceGraph = libhprof.IndexedReferenceGraph

// ============================================================================
// Function Forwarding
// ============================================================================

// NewIndexedObjectStore creates a new indexed object store.
func NewIndexedObjectStore(estimatedObjects int) *IndexedObjectStore {
	return libhprof.NewIndexedObjectStore(estimatedObjects)
}

// NewCompactEdgeList creates a new compact edge list.
func NewCompactEdgeList(nodeCount int, estimatedEdges int) *CompactEdgeList {
	return libhprof.NewCompactEdgeList(nodeCount, estimatedEdges)
}

// NewCompactEdgeListBuilder creates a new compact edge list builder.
func NewCompactEdgeListBuilder(nodeCount int, estimatedEdges int) *CompactEdgeListBuilder {
	return libhprof.NewCompactEdgeListBuilder(nodeCount, estimatedEdges)
}

// NewIndexedReferenceGraph creates a new indexed reference graph.
func NewIndexedReferenceGraph(estimatedObjects int) *IndexedReferenceGraph {
	return libhprof.NewIndexedReferenceGraph(estimatedObjects)
}

// Note: All methods on IndexedObjectStore, CompactEdgeList, CompactEdgeListBuilder,
// and IndexedReferenceGraph are automatically available through type aliases.
