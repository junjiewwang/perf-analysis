// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// BiggestObjectsBuilder builds the biggest objects list with lazy field loading.
type BiggestObjectsBuilder = libhprof.BiggestObjectsBuilder

// ============================================================================
// Function Forwarding
// ============================================================================

// NewBiggestObjectsBuilder creates a new biggest objects builder.
func NewBiggestObjectsBuilder(refGraph *ReferenceGraph, classLayouts map[uint64]*ClassFieldLayout, strings map[uint64]string) *BiggestObjectsBuilder {
	return libhprof.NewBiggestObjectsBuilder(refGraph, classLayouts, strings)
}

// Note: All methods on BiggestObjectsBuilder (GetBiggestObjectsByRetainedSize,
// DebugClassLoaderRetainedSize, etc.) are automatically available through the type alias.
