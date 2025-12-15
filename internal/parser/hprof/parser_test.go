package hprof

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewParser(t *testing.T) {
	t.Run("with default options", func(t *testing.T) {
		parser := NewParser(nil)
		assert.NotNil(t, parser)
		assert.NotNil(t, parser.opts)
		assert.Equal(t, 0, parser.opts.TopClassesN) // 0 means no limit
	})

	t.Run("with custom options", func(t *testing.T) {
		opts := &ParserOptions{
			TopClassesN: 50,
		}
		parser := NewParser(opts)
		assert.Equal(t, 50, parser.opts.TopClassesN)
	})
}

func TestReader_ReadHeader(t *testing.T) {
	// Create a minimal HPROF header
	var buf bytes.Buffer

	// Format string (null-terminated)
	buf.WriteString("JAVA PROFILE 1.0.2")
	buf.WriteByte(0)

	// ID size (4 bytes, big-endian)
	binary.Write(&buf, binary.BigEndian, uint32(8))

	// Timestamp (8 bytes, big-endian)
	timestamp := time.Now().UnixMilli()
	binary.Write(&buf, binary.BigEndian, uint64(timestamp))

	reader := NewReader(&buf)
	header, err := reader.ReadHeader()

	require.NoError(t, err)
	assert.Equal(t, "JAVA PROFILE 1.0.2", header.Format)
	assert.Equal(t, 8, header.IDSize)
	assert.Equal(t, 8, reader.IDSize())
}

func TestReader_ReadID(t *testing.T) {
	t.Run("4-byte ID", func(t *testing.T) {
		var buf bytes.Buffer
		binary.Write(&buf, binary.BigEndian, uint32(0x12345678))

		reader := NewReader(&buf)
		reader.SetIDSize(4)

		id, err := reader.ReadID()
		require.NoError(t, err)
		assert.Equal(t, uint64(0x12345678), id)
	})

	t.Run("8-byte ID", func(t *testing.T) {
		var buf bytes.Buffer
		binary.Write(&buf, binary.BigEndian, uint64(0x123456789ABCDEF0))

		reader := NewReader(&buf)
		reader.SetIDSize(8)

		id, err := reader.ReadID()
		require.NoError(t, err)
		assert.Equal(t, uint64(0x123456789ABCDEF0), id)
	})
}

func TestBasicTypeSize(t *testing.T) {
	tests := []struct {
		typ      BasicType
		idSize   int
		expected int
	}{
		{TypeBoolean, 8, 1},
		{TypeByte, 8, 1},
		{TypeChar, 8, 2},
		{TypeShort, 8, 2},
		{TypeInt, 8, 4},
		{TypeFloat, 8, 4},
		{TypeLong, 8, 8},
		{TypeDouble, 8, 8},
		{TypeObject, 4, 4},
		{TypeObject, 8, 8},
	}

	for _, tt := range tests {
		size := BasicTypeSize(tt.typ, tt.idSize)
		assert.Equal(t, tt.expected, size)
	}
}

func TestNormalizeClassName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"java/lang/String", "java.lang.String"},
		{"java/util/HashMap", "java.util.HashMap"},
		{"[Ljava/lang/Object;", "java.lang.Object[]"},
		{"[[I", "int[][]"},
		{"[B", "byte[]"},
		{"[C", "char[]"},
		{"[Z", "boolean[]"},
		{"[S", "short[]"},
		{"[J", "long[]"},
		{"[F", "float[]"},
		{"[D", "double[]"},
	}

	for _, tt := range tests {
		result := normalizeClassName(tt.input)
		assert.Equal(t, tt.expected, result, "input: %s", tt.input)
	}
}

func TestPrimitiveArrayTypeName(t *testing.T) {
	tests := []struct {
		typ      BasicType
		expected string
	}{
		{TypeBoolean, "boolean[]"},
		{TypeByte, "byte[]"},
		{TypeChar, "char[]"},
		{TypeShort, "short[]"},
		{TypeInt, "int[]"},
		{TypeLong, "long[]"},
		{TypeFloat, "float[]"},
		{TypeDouble, "double[]"},
	}

	for _, tt := range tests {
		result := primitiveArrayTypeName(tt.typ)
		assert.Equal(t, tt.expected, result)
	}
}

func TestParser_ParseRealFile(t *testing.T) {
	// Skip if test file doesn't exist
	testFile := "../../../test/heapdump2025-12-12-08-5818336174256011702999.hprof"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("Test HPROF file not found, skipping integration test")
	}

	file, err := os.Open(testFile)
	require.NoError(t, err)
	defer file.Close()

	parser := NewParser(nil)
	ctx := context.Background()

	result, err := parser.Parse(ctx, file)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Basic assertions
	assert.NotNil(t, result.Header)
	assert.Equal(t, "JAVA PROFILE 1.0.2", result.Header.Format)
	assert.True(t, result.Header.IDSize == 4 || result.Header.IDSize == 8)
	assert.Greater(t, result.TotalInstances, int64(0))
	assert.Greater(t, result.TotalHeapSize, int64(0))
	assert.Greater(t, result.TotalClasses, 0)
	assert.NotEmpty(t, result.TopClasses)

	// Log some results for debugging
	t.Logf("Header: %+v", result.Header)
	t.Logf("Total instances: %d", result.TotalInstances)
	t.Logf("Total heap size: %d bytes (%.2f MB)", result.TotalHeapSize, float64(result.TotalHeapSize)/(1024*1024))
	t.Logf("Total classes: %d", result.TotalClasses)
	t.Logf("Top 5 classes by size:")
	for i, cls := range result.TopClasses {
		if i >= 5 {
			break
		}
		t.Logf("  %d. %s: %d instances, %.2f MB (%.2f%%)",
			i+1, cls.ClassName, cls.InstanceCount,
			float64(cls.TotalSize)/(1024*1024), cls.Percentage)
	}
}

func TestParser_ContextCancellation(t *testing.T) {
	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Create minimal HPROF data
	var buf bytes.Buffer
	buf.WriteString("JAVA PROFILE 1.0.2")
	buf.WriteByte(0)
	binary.Write(&buf, binary.BigEndian, uint32(8))
	binary.Write(&buf, binary.BigEndian, uint64(time.Now().UnixMilli()))
	// Add a record header that would trigger context check
	buf.WriteByte(byte(TagHeapDump))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(100))

	parser := NewParser(nil)
	_, err := parser.Parse(ctx, &buf)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}
