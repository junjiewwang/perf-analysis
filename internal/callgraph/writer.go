package callgraph

import (
	libcg "github.com/junjiewwang/perf-analysis/perflib/callgraph"
	"github.com/junjiewwang/perf-analysis/perflib/writer"
)

// ---- Writer type aliases delegating to perflib ----

// JSONWriter writes call graph data as JSON.
type JSONWriter = writer.JSONWriter[*CallGraph]

// NewJSONWriter creates a new JSON writer.
func NewJSONWriter() *JSONWriter {
	return libcg.NewJSONWriter()
}

// NewPrettyJSONWriter creates a JSON writer with pretty printing.
func NewPrettyJSONWriter() *JSONWriter {
	return libcg.NewPrettyJSONWriter()
}

// GzipWriter writes call graph data as gzipped JSON.
type GzipWriter = writer.GzipWriter[*CallGraph]

// NewGzipWriter creates a new gzip writer with default compression.
func NewGzipWriter() *GzipWriter {
	return libcg.NewGzipWriter()
}

// WriteResult is an alias to the common writer.WriteResult.
type WriteResult = writer.WriteResult

// XDotJSONOutput represents the xdot_json format compatible with graphviz.
type XDotJSONOutput = libcg.XDotJSONOutput

// XDotObject represents a node in xdot_json format.
type XDotObject = libcg.XDotObject

// XDotEdge represents an edge in xdot_json format.
type XDotEdge = libcg.XDotEdge

// XDotWriter writes call graph data in xdot_json format.
type XDotWriter = libcg.XDotWriter

// NewXDotWriter creates a new xdot writer.
func NewXDotWriter() *XDotWriter {
	return libcg.NewXDotWriter()
}

// DOTWriter writes call graph data in DOT format.
type DOTWriter = libcg.DOTWriter

// NewDOTWriter creates a new DOT format writer.
func NewDOTWriter() *DOTWriter {
	return libcg.NewDOTWriter()
}
