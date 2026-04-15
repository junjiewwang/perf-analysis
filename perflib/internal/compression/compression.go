// Package compression provides unified compression/decompression utilities.
package compression

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

// Type represents the compression algorithm used.
type Type uint8

const (
	// TypeGzip uses gzip compression (legacy, slower but widely compatible)
	TypeGzip Type = 0
	// TypeZstd uses zstd compression (faster and better compression ratio)
	TypeZstd Type = 1
	// TypeNone represents no compression
	TypeNone Type = 255
)

// Level represents the compression level.
type Level int

const (
	// LevelFastest prioritizes speed over compression ratio
	LevelFastest Level = 1
	// LevelDefault balances speed and compression ratio
	LevelDefault Level = 3
	// LevelBest prioritizes compression ratio over speed
	LevelBest Level = 9
)

// Compressor provides a unified interface for compression operations.
type Compressor interface {
	// Compress compresses the input data
	Compress(data []byte) ([]byte, error)
	// Decompress decompresses the input data
	Decompress(data []byte) ([]byte, error)
	// Type returns the compression type
	Type() Type
	// Name returns the human-readable name of the compressor
	Name() string
}

// ============================================================================
// Gzip Compressor
// ============================================================================

// GzipCompressor implements Compressor using gzip.
type GzipCompressor struct {
	level int
}

// NewGzipCompressor creates a new gzip compressor.
func NewGzipCompressor(level Level) *GzipCompressor {
	gzipLevel := gzip.DefaultCompression
	switch level {
	case LevelFastest:
		gzipLevel = gzip.BestSpeed
	case LevelBest:
		gzipLevel = gzip.BestCompression
	default:
		gzipLevel = gzip.DefaultCompression
	}
	return &GzipCompressor{level: gzipLevel}
}

// Compress compresses data using gzip.
func (c *GzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, c.level)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip writer: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return nil, fmt.Errorf("failed to write gzip data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}
	return buf.Bytes(), nil
}

// Decompress decompresses gzip data.
func (c *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// Type returns TypeGzip.
func (c *GzipCompressor) Type() Type {
	return TypeGzip
}

// Name returns "gzip".
func (c *GzipCompressor) Name() string {
	return "gzip"
}

// ============================================================================
// Zstd Compressor
// ============================================================================

// ZstdCompressor implements Compressor using zstd.
type ZstdCompressor struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
	level   zstd.EncoderLevel
}

// NewZstdCompressor creates a new zstd compressor.
// The compressor is reusable and thread-safe for encoding.
func NewZstdCompressor(level Level) (*ZstdCompressor, error) {
	zstdLevel := zstd.SpeedDefault
	switch level {
	case LevelFastest:
		zstdLevel = zstd.SpeedFastest
	case LevelBest:
		zstdLevel = zstd.SpeedBestCompression
	default:
		zstdLevel = zstd.SpeedDefault
	}

	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstdLevel))
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		encoder.Close()
		return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
	}

	return &ZstdCompressor{
		encoder: encoder,
		decoder: decoder,
		level:   zstdLevel,
	}, nil
}

// Compress compresses data using zstd.
func (c *ZstdCompressor) Compress(data []byte) ([]byte, error) {
	return c.encoder.EncodeAll(data, make([]byte, 0, len(data)/2)), nil
}

// Decompress decompresses zstd data.
func (c *ZstdCompressor) Decompress(data []byte) ([]byte, error) {
	return c.decoder.DecodeAll(data, nil)
}

// Type returns TypeZstd.
func (c *ZstdCompressor) Type() Type {
	return TypeZstd
}

// Name returns "zstd".
func (c *ZstdCompressor) Name() string {
	return "zstd"
}

// Close releases resources used by the compressor.
func (c *ZstdCompressor) Close() {
	if c.encoder != nil {
		c.encoder.Close()
	}
	if c.decoder != nil {
		c.decoder.Close()
	}
}

// ============================================================================
// No-Op Compressor
// ============================================================================

// NoOpCompressor is a pass-through compressor that does not compress data.
type NoOpCompressor struct{}

// NewNoOpCompressor creates a new no-op compressor.
func NewNoOpCompressor() *NoOpCompressor {
	return &NoOpCompressor{}
}

// Compress returns the data unchanged.
func (c *NoOpCompressor) Compress(data []byte) ([]byte, error) {
	return data, nil
}

// Decompress returns the data unchanged.
func (c *NoOpCompressor) Decompress(data []byte) ([]byte, error) {
	return data, nil
}

// Type returns TypeNone.
func (c *NoOpCompressor) Type() Type {
	return TypeNone
}

// Name returns "none".
func (c *NoOpCompressor) Name() string {
	return "none"
}

// ============================================================================
// Factory Functions
// ============================================================================

// Default returns the default compressor (zstd with default level).
// Falls back to gzip if zstd initialization fails.
func Default() Compressor {
	comp, err := NewZstdCompressor(LevelDefault)
	if err != nil {
		return NewGzipCompressor(LevelDefault)
	}
	return comp
}

// Fast returns a fast compressor optimized for speed.
func Fast() Compressor {
	comp, err := NewZstdCompressor(LevelFastest)
	if err != nil {
		return NewGzipCompressor(LevelFastest)
	}
	return comp
}

// Best returns a compressor optimized for compression ratio.
func Best() Compressor {
	comp, err := NewZstdCompressor(LevelBest)
	if err != nil {
		return NewGzipCompressor(LevelBest)
	}
	return comp
}

// New creates a compressor by type and level.
func New(t Type, level Level) (Compressor, error) {
	switch t {
	case TypeZstd:
		return NewZstdCompressor(level)
	case TypeGzip:
		return NewGzipCompressor(level), nil
	case TypeNone:
		return NewNoOpCompressor(), nil
	default:
		return nil, fmt.Errorf("unknown compression type: %d", t)
	}
}

// ============================================================================
// Auto-Detection
// ============================================================================

// DetectType detects the compression type from magic bytes.
// Returns TypeGzip for gzip (0x1f 0x8b), TypeZstd for zstd (0x28 0xb5 0x2f 0xfd).
func DetectType(data []byte) Type {
	if len(data) < 4 {
		return TypeGzip // Default to gzip for backward compatibility
	}
	// zstd magic: 0x28 0xb5 0x2f 0xfd
	if data[0] == 0x28 && data[1] == 0xb5 && data[2] == 0x2f && data[3] == 0xfd {
		return TypeZstd
	}
	// gzip magic: 0x1f 0x8b
	if data[0] == 0x1f && data[1] == 0x8b {
		return TypeGzip
	}
	// Default to gzip for backward compatibility
	return TypeGzip
}

// AutoDecompress automatically detects compression type and decompresses data.
func AutoDecompress(data []byte) ([]byte, error) {
	compType := DetectType(data)
	switch compType {
	case TypeZstd:
		comp, err := NewZstdCompressor(LevelDefault)
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd decompressor: %w", err)
		}
		defer comp.Close()
		return comp.Decompress(data)
	default:
		comp := NewGzipCompressor(LevelDefault)
		return comp.Decompress(data)
	}
}

// ============================================================================
// Closeable Interface
// ============================================================================

// Closeable is an optional interface for compressors that hold resources.
type Closeable interface {
	Close()
}

// Close closes a compressor if it implements Closeable.
func Close(c Compressor) {
	if closer, ok := c.(Closeable); ok {
		closer.Close()
	}
}
