// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"context"

	libhprof "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// HierarchicalDominatorConfig configures the hierarchical dominator algorithm.
type HierarchicalDominatorConfig = libhprof.HierarchicalDominatorConfig

// LevelDominatorState holds state for level-based parallel dominator computation.
type LevelDominatorState = libhprof.LevelDominatorState

// DominatorMetrics tracks performance metrics for dominator computation.
type DominatorMetrics = libhprof.DominatorMetrics

// ParallelRetainedSizeComputer computes retained sizes in parallel.
type ParallelRetainedSizeComputer = libhprof.ParallelRetainedSizeComputer

// DominatorAlgorithm represents the algorithm to use for dominator computation.
type DominatorAlgorithm = libhprof.DominatorAlgorithm

// ============================================================================
// Constant Aliases
// ============================================================================

const (
	// DominatorAlgorithmAuto automatically selects the best algorithm.
	DominatorAlgorithmAuto = libhprof.DominatorAlgorithmAuto

	// DominatorAlgorithmLengauerTarjan uses the classic Lengauer-Tarjan algorithm.
	DominatorAlgorithmLengauerTarjan = libhprof.DominatorAlgorithmLengauerTarjan

	// DominatorAlgorithmHierarchical uses the hierarchical parallel algorithm.
	DominatorAlgorithmHierarchical = libhprof.DominatorAlgorithmHierarchical
)

// ============================================================================
// Function Forwarding
// ============================================================================

// DefaultHierarchicalDominatorConfig returns default configuration.
func DefaultHierarchicalDominatorConfig() HierarchicalDominatorConfig {
	return libhprof.DefaultHierarchicalDominatorConfig()
}

// NewLevelDominatorState creates a new level-based dominator state.
func NewLevelDominatorState(nodeCount int, config HierarchicalDominatorConfig) *LevelDominatorState {
	return libhprof.NewLevelDominatorState(nodeCount, config)
}

// NewParallelRetainedSizeComputer creates a new parallel retained size computer.
func NewParallelRetainedSizeComputer(state *LevelDominatorState, config HierarchicalDominatorConfig) *ParallelRetainedSizeComputer {
	return libhprof.NewParallelRetainedSizeComputer(state, config)
}

// ComputeHierarchicalDominators computes dominators using the hierarchical parallel algorithm.
func ComputeHierarchicalDominators(ctx context.Context, g *ReferenceGraph, config HierarchicalDominatorConfig) {
	libhprof.ComputeHierarchicalDominators(ctx, g, config)
}

// SelectDominatorAlgorithm selects the best algorithm based on graph characteristics.
func SelectDominatorAlgorithm(objectCount int, edgeCount int) DominatorAlgorithm {
	return libhprof.SelectDominatorAlgorithm(objectCount, edgeCount)
}

// ComputeDominatorsAdaptive computes dominators using the best algorithm.
func ComputeDominatorsAdaptive(ctx context.Context, g *ReferenceGraph) {
	libhprof.ComputeDominatorsAdaptive(ctx, g)
}

// ComputeClassRetainedSizesHierarchical computes class retained sizes in parallel.
func ComputeClassRetainedSizesHierarchical(ctx context.Context, g *ReferenceGraph) (map[uint64]int64, map[uint64]int64) {
	return libhprof.ComputeClassRetainedSizesHierarchical(ctx, g)
}

// Note: All methods on LevelDominatorState (BuildFromReferenceGraph, ComputeLevels,
// ComputeDominators, GetDominator, ExportToReferenceGraph) and ParallelRetainedSizeComputer
// (Compute) are automatically available through type aliases.
