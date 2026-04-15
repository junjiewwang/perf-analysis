// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// SerializeOptions controls serialization behavior.
type SerializeOptions = libhprof.SerializeOptions

// SerializationStats holds statistics about the serialization process.
type SerializationStats = libhprof.SerializationStats

// ============================================================================
// Constant Aliases
// ============================================================================

const (
	// SerializerVersion is the current serialization format version.
	SerializerVersion = libhprof.SerializerVersion

	// MagicBytes are the magic bytes for file format identification.
	MagicBytes = libhprof.MagicBytes
)

// ============================================================================
// Function Forwarding
// ============================================================================

// DefaultSerializeOptions returns default serialization options.
func DefaultSerializeOptions() SerializeOptions {
	return libhprof.DefaultSerializeOptions()
}

// FastSerializeOptions returns options optimized for speed.
func FastSerializeOptions() SerializeOptions {
	return libhprof.FastSerializeOptions()
}

// LegacySerializeOptions returns options compatible with older versions (gzip).
func LegacySerializeOptions() SerializeOptions {
	return libhprof.LegacySerializeOptions()
}

// DeserializeReferenceGraph deserializes a ReferenceGraph from compressed protobuf bytes.
func DeserializeReferenceGraph(data []byte) (*ReferenceGraph, error) {
	return libhprof.DeserializeReferenceGraph(data)
}

// DeserializeReferenceGraphFromFile deserializes a ReferenceGraph from a file.
func DeserializeReferenceGraphFromFile(filename string) (*ReferenceGraph, error) {
	return libhprof.DeserializeReferenceGraphFromFile(filename)
}

// Note: Methods on ReferenceGraph (Serialize, SerializeToFile) are automatically
// available through the type alias.
