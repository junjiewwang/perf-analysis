// Package pprof provides parsing functionality for Go pprof profile data.
// This is a thin wrapper around github.com/perf-analysis/perflib/parser/pprof.
package pprof

import (
	libpprof "github.com/perf-analysis/perflib/parser/pprof"
)

// Type aliases - delegate all types to perflib/parser/pprof.
type (
	// SampleType represents the type of sample in a pprof profile.
	SampleType = libpprof.SampleType

	// Parser parses Go pprof profile data.
	Parser = libpprof.Parser

	// TopFunction represents a top function in the profile.
	TopFunction = libpprof.TopFunction
)

// SampleType constants - re-export from perflib.
const (
	// CPU sample types
	SampleTypeCPU     = libpprof.SampleTypeCPU
	SampleTypeSamples = libpprof.SampleTypeSamples

	// Heap sample types
	SampleTypeInuseSpace   = libpprof.SampleTypeInuseSpace
	SampleTypeInuseObjects = libpprof.SampleTypeInuseObjects
	SampleTypeAllocSpace   = libpprof.SampleTypeAllocSpace
	SampleTypeAllocObjects = libpprof.SampleTypeAllocObjects

	// Goroutine sample types
	SampleTypeGoroutine = libpprof.SampleTypeGoroutine

	// Block/Mutex sample types
	SampleTypeContentions = libpprof.SampleTypeContentions
	SampleTypeDelay       = libpprof.SampleTypeDelay
)

// NewParser creates a new pprof parser.
var NewParser = libpprof.NewParser
