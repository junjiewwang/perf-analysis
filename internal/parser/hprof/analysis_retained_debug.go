// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// RetainedSizeAnalyzer analyzes retained size discrepancies between different calculation methods.
type RetainedSizeAnalyzer = libhprof.RetainedSizeAnalyzer

// AnalyzerConfig holds configuration for the analyzer.
type AnalyzerConfig = libhprof.AnalyzerConfig

// AnalysisStrategy defines the interface for pluggable analysis strategies.
type AnalysisStrategy = libhprof.AnalysisStrategy

// StrategyResult holds the result of a strategy analysis.
type StrategyResult = libhprof.StrategyResult

// Finding represents a single finding from the analysis.
type Finding = libhprof.Finding

// FindingLevel indicates the severity of a finding.
type FindingLevel = libhprof.FindingLevel

// InstanceAnalysisContext holds all the analysis data for a single instance.
type InstanceAnalysisContext = libhprof.InstanceAnalysisContext

// FieldAnalysisResult holds analysis result for a single field.
type FieldAnalysisResult = libhprof.FieldAnalysisResult

// ObjectArrayRefInfo holds information about Object[] references.
type ObjectArrayRefInfo = libhprof.ObjectArrayRefInfo

// HolderTypeStats holds statistics grouped by holder type.
type HolderTypeStats = libhprof.HolderTypeStats

// RetainedSizeAnalysisResult holds the complete analysis result.
type RetainedSizeAnalysisResult = libhprof.RetainedSizeAnalysisResult

// InstanceAnalysisResult holds analysis result for a single instance.
type InstanceAnalysisResult = libhprof.InstanceAnalysisResult

// IDEAStyleRetainedResult holds the result of IDEA-style retained size calculation.
type IDEAStyleRetainedResult = libhprof.IDEAStyleRetainedResult

// ObjectArrayAnalysisStrategy analyzes Object[] references.
type ObjectArrayAnalysisStrategy = libhprof.ObjectArrayAnalysisStrategy

// HolderTypeAnalysisStrategy analyzes holder types.
type HolderTypeAnalysisStrategy = libhprof.HolderTypeAnalysisStrategy

// ScenarioComparisonStrategy compares different retained size calculation scenarios.
type ScenarioComparisonStrategy = libhprof.ScenarioComparisonStrategy

// ============================================================================
// Constant Aliases
// ============================================================================

const (
	// FindingInfo indicates an informational finding.
	FindingInfo = libhprof.FindingInfo

	// FindingWarning indicates a warning finding.
	FindingWarning = libhprof.FindingWarning

	// FindingCritical indicates a critical finding.
	FindingCritical = libhprof.FindingCritical
)

// ============================================================================
// Function Forwarding
// ============================================================================

// DefaultAnalyzerConfig returns the default configuration.
func DefaultAnalyzerConfig() *AnalyzerConfig {
	return libhprof.DefaultAnalyzerConfig()
}

// NewRetainedSizeAnalyzer creates a new analyzer with default configuration.
func NewRetainedSizeAnalyzer(refGraph *ReferenceGraph) *RetainedSizeAnalyzer {
	return libhprof.NewRetainedSizeAnalyzer(refGraph)
}

// NewRetainedSizeAnalyzerWithConfig creates a new analyzer with custom configuration.
func NewRetainedSizeAnalyzerWithConfig(refGraph *ReferenceGraph, config *AnalyzerConfig) *RetainedSizeAnalyzer {
	return libhprof.NewRetainedSizeAnalyzerWithConfig(refGraph, config)
}

// FormatBytes formats bytes to human-readable string.
func FormatBytes(bytes int64) string {
	return libhprof.FormatBytes(bytes)
}

// Note: All methods on RetainedSizeAnalyzer (AnalyzeClass, AnalyzeClassWithDebug,
// CalculateIDEAStyleRetainedSize, CalculateIDEAStyleForClass, PrintIDEAStyleComparison)
// and analysis strategy implementations are automatically available through type aliases.
