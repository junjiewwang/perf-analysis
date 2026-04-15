package flamegraph

import (
	libfg "github.com/junjiewwang/perf-analysis/perflib/flamegraph"
	"github.com/junjiewwang/perf-analysis/perflib/writer"
)

// ---- Writer type aliases delegating to perflib ----

// JSONWriter writes flame graph data as JSON.
type JSONWriter = writer.JSONWriter[*FlameGraph]

// NewJSONWriter creates a new JSON writer.
func NewJSONWriter() *JSONWriter {
	return libfg.NewJSONWriter()
}

// NewPrettyJSONWriter creates a JSON writer with pretty printing.
func NewPrettyJSONWriter() *JSONWriter {
	return libfg.NewPrettyJSONWriter()
}

// GzipWriter writes flame graph data as gzipped JSON.
type GzipWriter = writer.GzipWriter[*FlameGraph]

// NewGzipWriter creates a new gzip writer with default compression.
func NewGzipWriter() *GzipWriter {
	return libfg.NewGzipWriter()
}

// NewGzipWriterWithLevel creates a gzip writer with specified compression level.
func NewGzipWriterWithLevel(level int) *GzipWriter {
	return libfg.NewGzipWriterWithLevel(level)
}

// WriteResult is an alias to the common writer.WriteResult.
type WriteResult = writer.WriteResult

// FoldedWriter writes flame graph data in collapsed/folded format.
type FoldedWriter = libfg.FoldedWriter

// NewFoldedWriter creates a new folded format writer.
func NewFoldedWriter() *FoldedWriter {
	return libfg.NewFoldedWriter()
}
