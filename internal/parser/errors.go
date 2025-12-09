package parser

import "errors"

var (
	// ErrInvalidFormat is returned when the input format is invalid.
	ErrInvalidFormat = errors.New("invalid input format")

	// ErrEmptyInput is returned when the input is empty.
	ErrEmptyInput = errors.New("empty input")

	// ErrParseFailed is returned when parsing fails.
	ErrParseFailed = errors.New("parse failed")

	// ErrUnsupportedFormat is returned when the format is not supported.
	ErrUnsupportedFormat = errors.New("unsupported format")

	// ErrInvalidStackFrame is returned when a stack frame is invalid.
	ErrInvalidStackFrame = errors.New("invalid stack frame")

	// ErrContextCanceled is returned when the context is canceled during parsing.
	ErrContextCanceled = errors.New("context canceled")
)
