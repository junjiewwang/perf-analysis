package statistics

import (
	libstats "github.com/junjiewwang/perf-analysis/perflib/statistics"
)

// ---- Type aliases delegating to perflib/statistics ----

// ThreadStatsCalculator calculates thread statistics from samples.
type ThreadStatsCalculator = libstats.ThreadStatsCalculator

// ThreadStatsOption configures the ThreadStatsCalculator.
type ThreadStatsOption = libstats.ThreadStatsOption

// ThreadEntry represents a thread with its statistics.
type ThreadEntry = libstats.ThreadEntry

// ThreadStatsResult holds the calculation result.
type ThreadStatsResult = libstats.ThreadStatsResult

// ActiveThreadInfo represents active thread information for JSON output.
type ActiveThreadInfo = libstats.ActiveThreadInfo

// ---- Constructor and option functions delegating to perflib ----

// WithMaxThreads sets the maximum number of threads to return.
func WithMaxThreads(n int) ThreadStatsOption {
	return libstats.WithMaxThreads(n)
}

// NewThreadStatsCalculator creates a new ThreadStatsCalculator.
func NewThreadStatsCalculator(opts ...ThreadStatsOption) *ThreadStatsCalculator {
	return libstats.NewThreadStatsCalculator(opts...)
}
