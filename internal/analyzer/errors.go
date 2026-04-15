package analyzer

import (
	libanalyzer "github.com/perf-analysis/perflib/analyzer"
)

// Error variables are aliases to perflib/analyzer errors,
// ensuring error identity (errors.Is) works correctly across layers.
var (
	// ErrUnsupportedMode is returned when an unknown analysis mode is specified.
	ErrUnsupportedMode = libanalyzer.ErrUnsupportedMode

	// ErrParseError is returned when parsing profiling data fails.
	ErrParseError = libanalyzer.ErrParseError

	// ErrEmptyData is returned when profiling data is empty.
	ErrEmptyData = libanalyzer.ErrEmptyData

	// ErrAnalysisFailed is returned when analysis fails.
	ErrAnalysisFailed = libanalyzer.ErrAnalysisFailed

	// ErrContextCanceled is returned when the context is canceled.
	ErrContextCanceled = libanalyzer.ErrContextCanceled
)
