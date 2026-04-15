// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"github.com/perf-analysis/perflib/internal/compression"
)

// ============================================================================
// Type Aliases for internal compression usage
// ============================================================================

// CompressionType is an alias to compression.Type.
type CompressionType = compression.Type

// CompressionLevel is an alias to compression.Level.
type CompressionLevel = compression.Level

// Compressor is an alias to compression.Compressor.
type Compressor = compression.Compressor

// GzipCompressor is an alias to compression.GzipCompressor.
type GzipCompressor = compression.GzipCompressor

// ZstdCompressor is an alias to compression.ZstdCompressor.
type ZstdCompressor = compression.ZstdCompressor

// ============================================================================
// Constant Aliases
// ============================================================================

const (
	// CompressionGzip uses gzip compression (legacy, slower but widely compatible)
	CompressionGzip = compression.TypeGzip
	// CompressionZstd uses zstd compression (faster and better compression ratio)
	CompressionZstd = compression.TypeZstd
)

const (
	// CompressionFastest prioritizes speed over compression ratio
	CompressionFastest = compression.LevelFastest
	// CompressionDefault balances speed and compression ratio
	CompressionDefault = compression.LevelDefault
	// CompressionBest prioritizes compression ratio over speed
	CompressionBest = compression.LevelBest
)

// ============================================================================
// Function Wrappers
// ============================================================================

// NewGzipCompressor creates a new gzip compressor.
func NewGzipCompressor(level CompressionLevel) *GzipCompressor {
	return compression.NewGzipCompressor(level)
}

// NewZstdCompressor creates a new zstd compressor.
func NewZstdCompressor(level CompressionLevel) (*ZstdCompressor, error) {
	return compression.NewZstdCompressor(level)
}

// DefaultCompressor returns the default compressor (zstd with default level).
func DefaultCompressor() Compressor {
	return compression.Default()
}

// FastCompressor returns a fast compressor optimized for speed.
func FastCompressor() Compressor {
	return compression.Fast()
}

// detectCompressionType detects the compression type from magic bytes.
func detectCompressionType(data []byte) CompressionType {
	return compression.DetectType(data)
}

// AutoDecompress automatically detects compression type and decompresses data.
func AutoDecompress(data []byte) ([]byte, error) {
	return compression.AutoDecompress(data)
}
