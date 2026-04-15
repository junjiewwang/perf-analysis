// Package statistics provides unified profiling statistics utilities.
// This package delegates to perflib/statistics and provides type aliases for backward compatibility.
package statistics

import (
	libstats "github.com/perf-analysis/perflib/statistics"
)

// ---- Type aliases delegating to perflib/statistics ----

// TopFuncsCalculator calculates top function statistics from samples.
type TopFuncsCalculator = libstats.TopFuncsCalculator

// TopFuncsOption configures the TopFuncsCalculator.
type TopFuncsOption = libstats.TopFuncsOption

// TopFuncEntry represents a function with its statistics.
type TopFuncEntry = libstats.TopFuncEntry

// TopFuncsResult holds the calculation result.
type TopFuncsResult = libstats.TopFuncsResult

// ---- Constructor and option functions delegating to perflib ----

// WithTopN sets the number of top functions to return.
func WithTopN(n int) TopFuncsOption {
	return libstats.WithTopN(n)
}

// WithSwapper includes swapper threads in calculations.
func WithSwapper(include bool) TopFuncsOption {
	return libstats.WithSwapper(include)
}

// NewTopFuncsCalculator creates a new TopFuncsCalculator.
func NewTopFuncsCalculator(opts ...TopFuncsOption) *TopFuncsCalculator {
	return libstats.NewTopFuncsCalculator(opts...)
}
