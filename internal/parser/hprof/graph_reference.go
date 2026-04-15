// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// ReferenceGraph holds the object reference graph with GC root tracking.
type ReferenceGraph = libhprof.ReferenceGraph

// ObjectReference represents a reference from one object to another.
type ObjectReference = libhprof.ObjectReference

// IndexedOutRef represents an outgoing/incoming reference using compact index.
type IndexedOutRef = libhprof.IndexedOutRef

// IndexedReference represents a reference using compact indices instead of object IDs.
type IndexedReference = libhprof.IndexedReference

// ============================================================================
// Function Forwarding
// ============================================================================

// NewReferenceGraph creates a new reference graph.
func NewReferenceGraph() *ReferenceGraph {
	return libhprof.NewReferenceGraph()
}

// NewReferenceGraphWithCapacity creates a new reference graph with pre-allocated capacity.
func NewReferenceGraphWithCapacity(estimatedObjects int) *ReferenceGraph {
	return libhprof.NewReferenceGraphWithCapacity(estimatedObjects)
}

// Note: All methods on ReferenceGraph (AddReference, AddGCRoot, ComputeDominatorTree,
// GetStats, GetRetainedSize, FindPathsToGCRoot, Serialize, SerializeToFile, etc.)
// are automatically available through the type alias.
