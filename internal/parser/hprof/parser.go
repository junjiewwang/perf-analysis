// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// SizeCalculationMode defines how shallow sizes are calculated.
type SizeCalculationMode = libhprof.SizeCalculationMode

// ParserOptions configures the HPROF parser.
type ParserOptions = libhprof.ParserOptions

// Parser parses HPROF heap dump files.
type Parser = libhprof.Parser

// FieldDescriptor describes a field in a class.
type FieldDescriptor = libhprof.FieldDescriptor

// ============================================================================
// Constant Aliases
// ============================================================================

const (
	// SizeModeCompressedOops uses compressed oops (12-byte header, 4-byte refs).
	SizeModeCompressedOops = libhprof.SizeModeCompressedOops

	// SizeModeNonCompressed uses non-compressed oops (16-byte header, 8-byte refs).
	SizeModeNonCompressed = libhprof.SizeModeNonCompressed

	// SizeModeAuto automatically detects based on heap size.
	SizeModeAuto = libhprof.SizeModeAuto
)

// ============================================================================
// Function Forwarding
// ============================================================================

// DefaultParserOptions returns default parser options.
func DefaultParserOptions() *ParserOptions {
	return libhprof.DefaultParserOptions()
}

// NewParser creates a new HPROF parser.
func NewParser(opts *ParserOptions) *Parser {
	return libhprof.NewParser(opts)
}

// Note: All methods on Parser (Parse) are automatically available through the type alias.
