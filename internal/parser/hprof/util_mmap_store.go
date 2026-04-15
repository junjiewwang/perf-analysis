// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// MmapConfig configures memory-mapped storage behavior.
type MmapConfig = libhprof.MmapConfig

// MmapArray provides a memory-mapped array backed by a file.
type MmapArray[T any] = libhprof.MmapArray[T]

// ObjectRecord represents an object in mmap storage.
type ObjectRecord = libhprof.ObjectRecord

// MmapObjectStore provides memory-mapped storage for object metadata.
type MmapObjectStore = libhprof.MmapObjectStore

// MmapEdgeStore provides memory-mapped storage for graph edges.
type MmapEdgeStore = libhprof.MmapEdgeStore

// ============================================================================
// Function Forwarding
// ============================================================================

// DefaultMmapConfig returns default mmap configuration.
func DefaultMmapConfig() MmapConfig {
	return libhprof.DefaultMmapConfig()
}

// NewMmapArray creates a new memory-mapped array.
func NewMmapArray[T any](filename string, initialCapacity int64) (*MmapArray[T], error) {
	return libhprof.NewMmapArray[T](filename, initialCapacity)
}

// NewMmapObjectStore creates a new mmap object store.
func NewMmapObjectStore(estimatedObjects int, config MmapConfig) (*MmapObjectStore, error) {
	return libhprof.NewMmapObjectStore(estimatedObjects, config)
}

// NewMmapEdgeStore creates a new mmap edge store.
func NewMmapEdgeStore(nodeCount int, estimatedEdges int) (*MmapEdgeStore, error) {
	return libhprof.NewMmapEdgeStore(nodeCount, estimatedEdges)
}

// ShouldUseMmap determines if mmap should be used based on object count and config.
func ShouldUseMmap(objectCount int, config MmapConfig) bool {
	return libhprof.ShouldUseMmap(objectCount, config)
}

// Note: All methods on MmapArray (Get, Set, Len, Cap, Close, Sync),
// MmapObjectStore, and MmapEdgeStore are automatically available through type aliases.
