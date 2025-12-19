// Package writer provides common JSON and Gzip writers for profiling data.
package writer

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// JSONWriter writes data as JSON.
type JSONWriter[T any] struct {
	// Indent specifies the indentation for pretty printing.
	// Empty string means compact output.
	Indent string
}

// NewJSONWriter creates a new JSON writer with compact output.
func NewJSONWriter[T any]() *JSONWriter[T] {
	return &JSONWriter[T]{Indent: ""}
}

// NewPrettyJSONWriter creates a JSON writer with pretty printing.
func NewPrettyJSONWriter[T any]() *JSONWriter[T] {
	return &JSONWriter[T]{Indent: "  "}
}

// Write writes the data as JSON to the writer.
func (w *JSONWriter[T]) Write(data T, writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	if w.Indent != "" {
		encoder.SetIndent("", w.Indent)
	}
	return encoder.Encode(data)
}

// WriteToFile writes the data as JSON to a file.
func (w *JSONWriter[T]) WriteToFile(data T, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	return w.Write(data, file)
}

// GzipWriter writes data as gzipped JSON.
type GzipWriter[T any] struct {
	// CompressionLevel is the gzip compression level (1-9).
	CompressionLevel int
}

// NewGzipWriter creates a new gzip writer with default compression.
func NewGzipWriter[T any]() *GzipWriter[T] {
	return &GzipWriter[T]{CompressionLevel: gzip.DefaultCompression}
}

// NewGzipWriterWithLevel creates a gzip writer with specified compression level.
func NewGzipWriterWithLevel[T any](level int) *GzipWriter[T] {
	return &GzipWriter[T]{CompressionLevel: level}
}

// Write writes the data as gzipped JSON to the writer.
func (w *GzipWriter[T]) Write(data T, writer io.Writer) error {
	gzWriter, err := gzip.NewWriterLevel(writer, w.CompressionLevel)
	if err != nil {
		return fmt.Errorf("failed to create gzip writer: %w", err)
	}
	defer gzWriter.Close()

	encoder := json.NewEncoder(gzWriter)
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode data: %w", err)
	}

	return gzWriter.Close()
}

// WriteToFile writes the data as gzipped JSON to a file.
func (w *GzipWriter[T]) WriteToFile(data T, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	return w.Write(data, file)
}

// WriteResult contains statistics about the written file.
type WriteResult struct {
	JSONSize       int64
	CompressedSize int64
	CompressionPct float64
}

// WriteToFileWithStats writes and returns statistics about the output.
func (w *GzipWriter[T]) WriteToFileWithStats(data T, filepath string) (*WriteResult, error) {
	// First, get the JSON size
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}
	jsonSize := int64(len(jsonData))

	// Write the gzipped file
	file, err := os.Create(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	gzWriter, err := gzip.NewWriterLevel(file, w.CompressionLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip writer: %w", err)
	}

	_, err = gzWriter.Write(jsonData)
	if err != nil {
		gzWriter.Close()
		return nil, fmt.Errorf("failed to write gzip data: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Get compressed size
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	compressedSize := fileInfo.Size()

	compressionPct := 0.0
	if jsonSize > 0 {
		compressionPct = float64(compressedSize) / float64(jsonSize) * 100
	}

	return &WriteResult{
		JSONSize:       jsonSize,
		CompressedSize: compressedSize,
		CompressionPct: compressionPct,
	}, nil
}
