package flamegraph

import (
	"fmt"
	"io"
	"os"

	"github.com/perf-analysis/pkg/writer"
)

// JSONWriter writes flame graph data as JSON.
// This is a type alias for backward compatibility.
type JSONWriter = writer.JSONWriter[*FlameGraph]

// NewJSONWriter creates a new JSON writer.
func NewJSONWriter() *JSONWriter {
	return writer.NewJSONWriter[*FlameGraph]()
}

// NewPrettyJSONWriter creates a JSON writer with pretty printing.
func NewPrettyJSONWriter() *JSONWriter {
	return writer.NewPrettyJSONWriter[*FlameGraph]()
}

// GzipWriter writes flame graph data as gzipped JSON.
// This is a type alias for backward compatibility.
type GzipWriter = writer.GzipWriter[*FlameGraph]

// NewGzipWriter creates a new gzip writer with default compression.
func NewGzipWriter() *GzipWriter {
	return writer.NewGzipWriter[*FlameGraph]()
}

// NewGzipWriterWithLevel creates a gzip writer with specified compression level.
func NewGzipWriterWithLevel(level int) *GzipWriter {
	return writer.NewGzipWriterWithLevel[*FlameGraph](level)
}

// WriteResult is an alias to the common writer.WriteResult.
type WriteResult = writer.WriteResult

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
	if node.Name != "root" && node.Name != "" {
		if currentStack == "" {
			currentStack = node.Name
		} else {
			currentStack = currentStack + ";" + node.Name
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
