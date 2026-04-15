// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// ParallelConfig configures parallel analysis behavior.
type ParallelConfig = libhprof.ParallelConfig

// AnalysisTask represents a unit of analysis work.
type AnalysisTask = libhprof.AnalysisTask

// RetainerResult holds the result of retainer analysis.
type RetainerResult = libhprof.RetainerResult

// GraphResult holds the result of reference graph analysis.
type GraphResult = libhprof.GraphResult

// BusinessRetainerResult holds the result of business retainer analysis.
type BusinessRetainerResult = libhprof.BusinessRetainerResult

// ParallelAnalyzer runs analysis tasks in parallel.
type ParallelAnalyzer = libhprof.ParallelAnalyzer

// FullAnalysisResult holds the complete result of all parallel analysis.
type FullAnalysisResult = libhprof.FullAnalysisResult

// AnalysisStats holds statistics about the analysis.
type AnalysisStats = libhprof.AnalysisStats

// AnalysisOptions configures analysis behavior.
type AnalysisOptions = libhprof.AnalysisOptions

// ============================================================================
// Function Forwarding
// ============================================================================

// DefaultParallelConfig returns default parallel configuration.
func DefaultParallelConfig() ParallelConfig {
	return libhprof.DefaultParallelConfig()
}

// NewParallelAnalyzer creates a new parallel analyzer.
func NewParallelAnalyzer(refGraph *ReferenceGraph, config ParallelConfig) *ParallelAnalyzer {
	return libhprof.NewParallelAnalyzer(refGraph, config)
}

// DefaultAnalysisOptions returns default analysis options.
func DefaultAnalysisOptions() AnalysisOptions {
	return libhprof.DefaultAnalysisOptions()
}

// Note: All methods on ParallelAnalyzer (RunFullAnalysis, etc.) and
// ParallelConfig are automatically available through type aliases.
