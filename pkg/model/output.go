// Package model defines output data abstractions for different analysis types.
// Core analysis types have been migrated to github.com/perf-analysis/perflib/model.
// This file retains only the AnalysisMode function that uses fmt (not migrated to avoid
// breaking the original function signature which differs from perflib's AnalysisModeString).
package model

import "fmt"

// AnalysisMode returns the composite mode string "{profiler}-{event}".
// Deprecated: Use AnalysisModeString instead.
func AnalysisMode(profiler Profiler, event EventType) string {
	return fmt.Sprintf("%s-%s", profiler, event)
}
