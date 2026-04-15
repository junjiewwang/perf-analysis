// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// RetainerInfo describes what retains a class's instances.
type RetainerInfo = libhprof.RetainerInfo

// ClassRetainers holds retainer information for a class.
type ClassRetainers = libhprof.ClassRetainers

// BusinessRetainer represents a business-level retainer with full path information.
type BusinessRetainer = libhprof.BusinessRetainer

// SamplingConfig controls how sampling is performed for large datasets.
type SamplingConfig = libhprof.SamplingConfig

// ReferenceGraphData holds data for reference graph visualization.
type ReferenceGraphData = libhprof.ReferenceGraphData

// ReferenceGraphNode represents a node in the reference graph visualization.
type ReferenceGraphNode = libhprof.ReferenceGraphNode

// ReferenceGraphEdge represents an edge in the reference graph visualization.
type ReferenceGraphEdge = libhprof.ReferenceGraphEdge

// ============================================================================
// Constant Aliases
// ============================================================================

// MaxObjectsForRetainerAnalysis is the maximum number of target objects to analyze.
const MaxObjectsForRetainerAnalysis = libhprof.MaxObjectsForRetainerAnalysis

// ============================================================================
// Function Forwarding
// ============================================================================

// DefaultSamplingConfig returns the default sampling configuration.
func DefaultSamplingConfig() SamplingConfig {
	return libhprof.DefaultSamplingConfig()
}

// Note: All methods on ReferenceGraph (ComputeMultiLevelRetainers, ComputeRetainersForClass,
// ComputeTopRetainers, ComputeBusinessRetainers, GetReferenceGraphForClass) are automatically
// available through the type alias.
