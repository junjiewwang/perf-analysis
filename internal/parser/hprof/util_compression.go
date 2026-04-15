// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// CompressionType is an alias to perflib hprof CompressionType.
type CompressionType = libhprof.CompressionType

// CompressionLevel is an alias to perflib hprof CompressionLevel.
type CompressionLevel = libhprof.CompressionLevel

// Compressor is an alias to perflib hprof Compressor.
type Compressor = libhprof.Compressor

// GzipCompressor is an alias to perflib hprof GzipCompressor.
type GzipCompressor = libhprof.GzipCompressor

// ZstdCompressor is an alias to perflib hprof ZstdCompressor.
type ZstdCompressor = libhprof.ZstdCompressor

// ============================================================================
// Constant Aliases
// ============================================================================

const (
	// CompressionGzip uses gzip compression (legacy, slower but widely compatible)
	CompressionGzip = libhprof.CompressionGzip
	// CompressionZstd uses zstd compression (faster and better compression ratio)
	CompressionZstd = libhprof.CompressionZstd
)

const (
	// CompressionFastest prioritizes speed over compression ratio
	CompressionFastest = libhprof.CompressionFastest
	// CompressionDefault balances speed and compression ratio
	CompressionDefault = libhprof.CompressionDefault
	// CompressionBest prioritizes compression ratio over speed
	CompressionBest = libhprof.CompressionBest
)

// ============================================================================
// Function Forwarding
// ============================================================================

// NewGzipCompressor creates a new gzip compressor.
func NewGzipCompressor(level CompressionLevel) *GzipCompressor {
	return libhprof.NewGzipCompressor(level)
}

// NewZstdCompressor creates a new zstd compressor.
func NewZstdCompressor(level CompressionLevel) (*ZstdCompressor, error) {
	return libhprof.NewZstdCompressor(level)
}

// DefaultCompressor returns the default compressor (zstd with default level).
func DefaultCompressor() Compressor {
	return libhprof.DefaultCompressor()
}

// FastCompressor returns a fast compressor optimized for speed.
func FastCompressor() Compressor {
	return libhprof.FastCompressor()
}

// AutoDecompress automatically detects compression type and decompresses data.
func AutoDecompress(data []byte) ([]byte, error) {
	return libhprof.AutoDecompress(data)
}
