// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// ResultBuilder builds the final HeapAnalysisResult from parsed state.
type ResultBuilder = libhprof.ResultBuilder

// Note: NewResultBuilder takes an unexported *parserState parameter,
// so it cannot be forwarded. After full migration, the call to NewResultBuilder
// happens inside perflib's parser.go.
// All methods on ResultBuilder (Build) are automatically available through the type alias.
