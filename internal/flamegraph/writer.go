package flamegraph

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// JSONWriter writes flame graph data as JSON.
type JSONWriter struct {
	// Indent specifies the indentation for pretty printing.
	// Empty string means compact output.
	Indent string
}

// NewJSONWriter creates a new JSON writer.
func NewJSONWriter() *JSONWriter {
	return &JSONWriter{Indent: ""}
}

// NewPrettyJSONWriter creates a JSON writer with pretty printing.
func NewPrettyJSONWriter() *JSONWriter {
	return &JSONWriter{Indent: "  "}
}

// Write writes the flame graph as JSON to the writer.
func (w *JSONWriter) Write(fg *FlameGraph, writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	if w.Indent != "" {
		encoder.SetIndent("", w.Indent)
	}
	// Write just the root node to match Python output format
	return encoder.Encode(fg.Root)
}

// WriteToFile writes the flame graph as JSON to a file.
func (w *JSONWriter) WriteToFile(fg *FlameGraph, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	return w.Write(fg, file)
}

// GzipWriter writes flame graph data as gzipped JSON.
type GzipWriter struct {
	// CompressionLevel is the gzip compression level (1-9).
	CompressionLevel int
}

// NewGzipWriter creates a new gzip writer with default compression.
func NewGzipWriter() *GzipWriter {
	return &GzipWriter{CompressionLevel: gzip.DefaultCompression}
}

// NewGzipWriterWithLevel creates a gzip writer with specified compression level.
func NewGzipWriterWithLevel(level int) *GzipWriter {
	return &GzipWriter{CompressionLevel: level}
}

// Write writes the flame graph as gzipped JSON to the writer.
func (w *GzipWriter) Write(fg *FlameGraph, writer io.Writer) error {
	gzWriter, err := gzip.NewWriterLevel(writer, w.CompressionLevel)
	if err != nil {
		return fmt.Errorf("failed to create gzip writer: %w", err)
	}
	defer gzWriter.Close()

	encoder := json.NewEncoder(gzWriter)
	// Write just the root node to match Python output format
	if err := encoder.Encode(fg.Root); err != nil {
		return fmt.Errorf("failed to encode flame graph: %w", err)
	}

	return gzWriter.Close()
}

// WriteToFile writes the flame graph as gzipped JSON to a file.
func (w *GzipWriter) WriteToFile(fg *FlameGraph, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	return w.Write(fg, file)
}

// WriteResult contains statistics about the written file.
type WriteResult struct {
	JSONSize       int64
	CompressedSize int64
	CompressionPct float64
}

// WriteToFileWithStats writes and returns statistics about the output.
func (w *GzipWriter) WriteToFileWithStats(fg *FlameGraph, filepath string) (*WriteResult, error) {
	// First, get the JSON size
	jsonData, err := json.Marshal(fg.Root)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal flame graph: %w", err)
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

// FoldedWriter writes flame graph data in collapsed/folded format.
// This format is compatible with flamegraph.pl script.
type FoldedWriter struct{}

// NewFoldedWriter creates a new folded format writer.
func NewFoldedWriter() *FoldedWriter {
	return &FoldedWriter{}
}

// Write writes the flame graph in folded format.
// Format: stack1;stack2;stack3 count
func (w *FoldedWriter) Write(fg *FlameGraph, writer io.Writer) error {
	return w.writeNode(fg.Root, "", writer)
}

func (w *FoldedWriter) writeNode(node *Node, prefix string, writer io.Writer) error {
	// Build current stack
	currentStack := prefix
	if node.Func != "root" {
		if currentStack == "" {
			currentStack = node.Func
		} else {
			currentStack = currentStack + ";" + node.Func
		}
	}

	// If this is a leaf node, write the stack
	if len(node.Children) == 0 && currentStack != "" {
		_, err := fmt.Fprintf(writer, "%s %d\n", currentStack, node.Value)
		return err
	}

	// Recurse to children
	for _, child := range node.Children {
		if err := w.writeNode(child, currentStack, writer); err != nil {
			return err
		}
	}

	return nil
}

// WriteToFile writes the flame graph in folded format to a file.
func (w *FoldedWriter) WriteToFile(fg *FlameGraph, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	return w.Write(fg, file)
}
