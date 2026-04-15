package parser

import (
	libparser "github.com/junjiewwang/perf-analysis/perflib/parser"
)

// Error variables - delegate to perflib/parser.
var (
	// ErrInvalidFormat is returned when the input format is invalid.
	ErrInvalidFormat = libparser.ErrInvalidFormat

	// ErrEmptyInput is returned when the input is empty.
	ErrEmptyInput = libparser.ErrEmptyInput

	// ErrParseFailed is returned when parsing fails.
	ErrParseFailed = libparser.ErrParseFailed

	// ErrUnsupportedFormat is returned when the format is not supported.
	ErrUnsupportedFormat = libparser.ErrUnsupportedFormat

	// ErrInvalidStackFrame is returned when a stack frame is invalid.
	ErrInvalidStackFrame = libparser.ErrInvalidStackFrame

	// ErrContextCanceled is returned when the context is canceled during parsing.
	ErrContextCanceled = libparser.ErrContextCanceled
)
