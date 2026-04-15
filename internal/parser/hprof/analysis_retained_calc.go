// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// RetainedSizeStrategy defines the strategy for retained size calculation.
type RetainedSizeStrategy = libhprof.RetainedSizeStrategy

// RetainedSizeCalculator defines the interface for retained size calculation strategies.
type RetainedSizeCalculator = libhprof.RetainedSizeCalculator

// RetainedSizeContext provides read-only access to graph data needed for retained size calculation.
type RetainedSizeContext = libhprof.RetainedSizeContext

// RetainedSizeCalculatorRegistry manages available retained size calculators.
type RetainedSizeCalculatorRegistry = libhprof.RetainedSizeCalculatorRegistry

// StandardRetainedSizeCalculator implements strict dominator-tree based calculation.
type StandardRetainedSizeCalculator = libhprof.StandardRetainedSizeCalculator

// IDEAStyleRetainedSizeCalculator implements IDEA-style retained size calculation.
type IDEAStyleRetainedSizeCalculator = libhprof.IDEAStyleRetainedSizeCalculator

// ============================================================================
// Constant Aliases
// ============================================================================

const (
	// RetainedSizeStrategyStandard uses strict dominator-tree based calculation (Eclipse MAT style).
	RetainedSizeStrategyStandard = libhprof.RetainedSizeStrategyStandard

	// RetainedSizeStrategyIDEA uses IDEA-style calculation that includes logically owned objects.
	RetainedSizeStrategyIDEA = libhprof.RetainedSizeStrategyIDEA
)

// ============================================================================
// Variable Aliases
// ============================================================================

// CollectionClasses defines Java collection classes that use Object[] internally.
var CollectionClasses = libhprof.CollectionClasses

// ============================================================================
// Function Forwarding
// ============================================================================

// IsCollectionClass checks if a class is a Java collection class.
func IsCollectionClass(className string) bool {
	return libhprof.IsCollectionClass(className)
}

// FormatBytesSize formats bytes to human-readable string.
func FormatBytesSize(bytes int64) string {
	return libhprof.FormatBytesSize(bytes)
}

// NewRetainedSizeCalculatorRegistry creates a new registry with default calculators.
func NewRetainedSizeCalculatorRegistry() *RetainedSizeCalculatorRegistry {
	return libhprof.NewRetainedSizeCalculatorRegistry()
}

// Note: All methods on RetainedSizeCalculatorRegistry (Register, Get, GetDefault,
// SetDefault, ListStrategies) and RetainedSizeCalculator implementations are
// automatically available through type aliases.
