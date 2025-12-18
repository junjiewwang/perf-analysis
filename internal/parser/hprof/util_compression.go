// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"github.com/perf-analysis/pkg/compression"
)

// ============================================================================
// Type Aliases for backward compatibility
// ============================================================================

// CompressionType is an alias to compression.Type for backward compatibility.
// Deprecated: Use compression.Type directly.
type CompressionType = compression.Type

// CompressionLevel is an alias to compression.Level for backward compatibility.
// Deprecated: Use compression.Level directly.
type CompressionLevel = compression.Level

// Compressor is an alias to compression.Compressor for backward compatibility.
// Deprecated: Use compression.Compressor directly.
type Compressor = compression.Compressor

// GzipCompressor is an alias to compression.GzipCompressor for backward compatibility.
// Deprecated: Use compression.GzipCompressor directly.
type GzipCompressor = compression.GzipCompressor

// ZstdCompressor is an alias to compression.ZstdCompressor for backward compatibility.
// Deprecated: Use compression.ZstdCompressor directly.
type ZstdCompressor = compression.ZstdCompressor

// ============================================================================
// Constant Aliases for backward compatibility
// ============================================================================

const (
	// CompressionGzip uses gzip compression (legacy, slower but widely compatible)
	// Deprecated: Use compression.TypeGzip directly.
	CompressionGzip = compression.TypeGzip
	// CompressionZstd uses zstd compression (faster and better compression ratio)
	// Deprecated: Use compression.TypeZstd directly.
	CompressionZstd = compression.TypeZstd
)

const (
	// CompressionFastest prioritizes speed over compression ratio
	// Deprecated: Use compression.LevelFastest directly.
	CompressionFastest = compression.LevelFastest
	// CompressionDefault balances speed and compression ratio
	// Deprecated: Use compression.LevelDefault directly.
	CompressionDefault = compression.LevelDefault
	// CompressionBest prioritizes compression ratio over speed
	// Deprecated: Use compression.LevelBest directly.
	CompressionBest = compression.LevelBest
)

// ============================================================================
// Function Aliases for backward compatibility
// ============================================================================

// NewGzipCompressor creates a new gzip compressor.
// Deprecated: Use compression.NewGzipCompressor directly.
func NewGzipCompressor(level CompressionLevel) *GzipCompressor {
	return compression.NewGzipCompressor(level)
}

// NewZstdCompressor creates a new zstd compressor.
// Deprecated: Use compression.NewZstdCompressor directly.
func NewZstdCompressor(level CompressionLevel) (*ZstdCompressor, error) {
	return compression.NewZstdCompressor(level)
}

// DefaultCompressor returns the default compressor (zstd with default level).
// Deprecated: Use compression.Default directly.
func DefaultCompressor() Compressor {
	return compression.Default()
}

// FastCompressor returns a fast compressor optimized for speed.
// Deprecated: Use compression.Fast directly.
func FastCompressor() Compressor {
	return compression.Fast()
}

// detectCompressionType detects the compression type from magic bytes.
// Deprecated: Use compression.DetectType directly.
func detectCompressionType(data []byte) CompressionType {
	return compression.DetectType(data)
}

// AutoDecompress automatically detects compression type and decompresses data.
// Deprecated: Use compression.AutoDecompress directly.
func AutoDecompress(data []byte) ([]byte, error) {
	return compression.AutoDecompress(data)
}
