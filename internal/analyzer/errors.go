package analyzer

import "errors"

var (
	// ErrUnsupportedTaskType is returned when no analyzer is registered for a task type.
	ErrUnsupportedTaskType = errors.New("unsupported task type")

	// ErrParseError is returned when parsing profiling data fails.
	ErrParseError = errors.New("failed to parse profiling data")

	// ErrEmptyData is returned when profiling data is empty.
	ErrEmptyData = errors.New("profiling data is empty")

	// ErrAnalysisFailed is returned when analysis fails.
	ErrAnalysisFailed = errors.New("analysis failed")

	// ErrContextCanceled is returned when the context is canceled.
	ErrContextCanceled = errors.New("context canceled")
)
