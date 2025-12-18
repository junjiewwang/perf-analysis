package compression

import (
	"bytes"
	"testing"
)

func TestGzipCompressor(t *testing.T) {
	c := NewGzipCompressor(LevelDefault)

	original := []byte("Hello, World! This is a test string for compression.")

	compressed, err := c.Compress(original)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	decompressed, err := c.Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	if !bytes.Equal(original, decompressed) {
		t.Error("Decompressed data doesn't match original")
	}

	if c.Type() != TypeGzip {
		t.Errorf("Expected TypeGzip, got %v", c.Type())
	}

	if c.Name() != "gzip" {
		t.Errorf("Expected 'gzip', got %s", c.Name())
	}
}

func TestZstdCompressor(t *testing.T) {
	c, err := NewZstdCompressor(LevelDefault)
	if err != nil {
		t.Fatalf("Failed to create zstd compressor: %v", err)
	}
	defer c.Close()

	original := []byte("Hello, World! This is a test string for compression.")

	compressed, err := c.Compress(original)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	decompressed, err := c.Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	if !bytes.Equal(original, decompressed) {
		t.Error("Decompressed data doesn't match original")
	}

	if c.Type() != TypeZstd {
		t.Errorf("Expected TypeZstd, got %v", c.Type())
	}

	if c.Name() != "zstd" {
		t.Errorf("Expected 'zstd', got %s", c.Name())
	}
}

func TestNoOpCompressor(t *testing.T) {
	c := NewNoOpCompressor()

	original := []byte("Hello, World!")

	compressed, err := c.Compress(original)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	if !bytes.Equal(original, compressed) {
		t.Error("NoOp compressor should return data unchanged")
	}

	decompressed, err := c.Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	if !bytes.Equal(original, decompressed) {
		t.Error("NoOp decompressor should return data unchanged")
	}

	if c.Type() != TypeNone {
		t.Errorf("Expected TypeNone, got %v", c.Type())
	}
}

func TestDetectType(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected Type
	}{
		{
			name:     "gzip magic",
			data:     []byte{0x1f, 0x8b, 0x08, 0x00},
			expected: TypeGzip,
		},
		{
			name:     "zstd magic",
			data:     []byte{0x28, 0xb5, 0x2f, 0xfd},
			expected: TypeZstd,
		},
		{
			name:     "unknown (defaults to gzip)",
			data:     []byte{0x00, 0x00, 0x00, 0x00},
			expected: TypeGzip,
		},
		{
			name:     "too short",
			data:     []byte{0x1f},
			expected: TypeGzip,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectType(tt.data)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestAutoDecompress(t *testing.T) {
	original := []byte("Hello, World! This is a test string for auto decompression.")

	// Test with gzip
	gzipComp := NewGzipCompressor(LevelDefault)
	gzipCompressed, _ := gzipComp.Compress(original)

	gzipDecompressed, err := AutoDecompress(gzipCompressed)
	if err != nil {
		t.Fatalf("AutoDecompress gzip failed: %v", err)
	}
	if !bytes.Equal(original, gzipDecompressed) {
		t.Error("AutoDecompress gzip: data mismatch")
	}

	// Test with zstd
	zstdComp, _ := NewZstdCompressor(LevelDefault)
	defer zstdComp.Close()
	zstdCompressed, _ := zstdComp.Compress(original)

	zstdDecompressed, err := AutoDecompress(zstdCompressed)
	if err != nil {
		t.Fatalf("AutoDecompress zstd failed: %v", err)
	}
	if !bytes.Equal(original, zstdDecompressed) {
		t.Error("AutoDecompress zstd: data mismatch")
	}
}

func TestFactoryFunctions(t *testing.T) {
	// Test Default
	def := Default()
	if def == nil {
		t.Error("Default() returned nil")
	}
	Close(def)

	// Test Fast
	fast := Fast()
	if fast == nil {
		t.Error("Fast() returned nil")
	}
	Close(fast)

	// Test Best
	best := Best()
	if best == nil {
		t.Error("Best() returned nil")
	}
	Close(best)
}

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		compType  Type
		level     Level
		expectErr bool
	}{
		{"gzip default", TypeGzip, LevelDefault, false},
		{"zstd default", TypeZstd, LevelDefault, false},
		{"none", TypeNone, LevelDefault, false},
		{"unknown", Type(100), LevelDefault, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.compType, tt.level)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if c == nil {
					t.Error("Expected compressor, got nil")
				}
				Close(c)
			}
		})
	}
}

func TestCompressionLevels(t *testing.T) {
	original := make([]byte, 10000)
	for i := range original {
		original[i] = byte(i % 256)
	}

	levels := []Level{LevelFastest, LevelDefault, LevelBest}

	for _, level := range levels {
		t.Run("gzip", func(t *testing.T) {
			c := NewGzipCompressor(level)
			compressed, err := c.Compress(original)
			if err != nil {
				t.Fatalf("Compress failed: %v", err)
			}
			decompressed, err := c.Decompress(compressed)
			if err != nil {
				t.Fatalf("Decompress failed: %v", err)
			}
			if !bytes.Equal(original, decompressed) {
				t.Error("Data mismatch")
			}
		})

		t.Run("zstd", func(t *testing.T) {
			c, err := NewZstdCompressor(level)
			if err != nil {
				t.Fatalf("Failed to create compressor: %v", err)
			}
			defer c.Close()

			compressed, err := c.Compress(original)
			if err != nil {
				t.Fatalf("Compress failed: %v", err)
			}
			decompressed, err := c.Decompress(compressed)
			if err != nil {
				t.Fatalf("Decompress failed: %v", err)
			}
			if !bytes.Equal(original, decompressed) {
				t.Error("Data mismatch")
			}
		})
	}
}

func BenchmarkGzipCompress(b *testing.B) {
	c := NewGzipCompressor(LevelDefault)
	data := make([]byte, 10000)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Compress(data)
	}
}

func BenchmarkZstdCompress(b *testing.B) {
	c, _ := NewZstdCompressor(LevelDefault)
	defer c.Close()
	data := make([]byte, 10000)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Compress(data)
	}
}

func BenchmarkGzipDecompress(b *testing.B) {
	c := NewGzipCompressor(LevelDefault)
	data := make([]byte, 10000)
	for i := range data {
		data[i] = byte(i % 256)
	}
	compressed, _ := c.Compress(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Decompress(compressed)
	}
}

func BenchmarkZstdDecompress(b *testing.B) {
	c, _ := NewZstdCompressor(LevelDefault)
	defer c.Close()
	data := make([]byte, 10000)
	for i := range data {
		data[i] = byte(i % 256)
	}
	compressed, _ := c.Compress(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Decompress(compressed)
	}
}
