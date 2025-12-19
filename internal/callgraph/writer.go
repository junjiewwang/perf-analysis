package callgraph

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/perf-analysis/pkg/writer"
)

// JSONWriter writes call graph data as JSON.
// This is a type alias for backward compatibility.
type JSONWriter = writer.JSONWriter[*CallGraph]

// NewJSONWriter creates a new JSON writer.
func NewJSONWriter() *JSONWriter {
	return writer.NewJSONWriter[*CallGraph]()
}

// NewPrettyJSONWriter creates a JSON writer with pretty printing.
func NewPrettyJSONWriter() *JSONWriter {
	return writer.NewPrettyJSONWriter[*CallGraph]()
}

// GzipWriter writes call graph data as gzipped JSON.
// This is a type alias for backward compatibility.
type GzipWriter = writer.GzipWriter[*CallGraph]

// NewGzipWriter creates a new gzip writer with default compression.
func NewGzipWriter() *GzipWriter {
	return writer.NewGzipWriter[*CallGraph]()
}

// XDotJSONOutput represents the xdot_json format compatible with graphviz.
// This format is used by the Python version's gprof2dot output.
type XDotJSONOutput struct {
	Name     string       `json:"name"`
	Directed bool         `json:"directed"`
	Strict   bool         `json:"strict"`
	Objects  []XDotObject `json:"objects"`
	Edges    []XDotEdge   `json:"edges"`
}

// XDotObject represents a node in xdot_json format.
type XDotObject struct {
	ID        int         `json:"_gvid"`
	Name      string      `json:"name"`
	Label     string      `json:"label,omitempty"`
	Style     string      `json:"style,omitempty"`
	Fillcolor string      `json:"fillcolor,omitempty"`
	Shape     string      `json:"shape,omitempty"`
	Draw      interface{} `json:"_draw_,omitempty"`
	Ldraw     interface{} `json:"_ldraw_,omitempty"`
}

// XDotEdge represents an edge in xdot_json format.
type XDotEdge struct {
	ID    int         `json:"_gvid"`
	Head  int         `json:"head"`
	Tail  int         `json:"tail"`
	Label string      `json:"label,omitempty"`
	Style string      `json:"style,omitempty"`
	Draw  interface{} `json:"_draw_,omitempty"`
	Ldraw interface{} `json:"_ldraw_,omitempty"`
	Hdraw interface{} `json:"_hdraw_,omitempty"`
}

// XDotWriter writes call graph data in xdot_json format.
type XDotWriter struct{}

// NewXDotWriter creates a new xdot writer.
func NewXDotWriter() *XDotWriter {
	return &XDotWriter{}
}

// Write writes the call graph in xdot_json format.
func (w *XDotWriter) Write(cg *CallGraph, writer io.Writer) error {
	output := w.convertToXDot(cg)
	encoder := json.NewEncoder(writer)
	return encoder.Encode(output)
}

// WriteToFile writes the call graph in xdot_json format to a file.
func (w *XDotWriter) WriteToFile(cg *CallGraph, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	return w.Write(cg, file)
}

// convertToXDot converts the call graph to xdot_json format.
func (w *XDotWriter) convertToXDot(cg *CallGraph) *XDotJSONOutput {
	output := &XDotJSONOutput{
		Name:     "callgraph",
		Directed: true,
		Strict:   false,
		Objects:  make([]XDotObject, 0, len(cg.Nodes)),
		Edges:    make([]XDotEdge, 0, len(cg.Edges)),
	}

	// Build node ID map
	nodeIDToIndex := make(map[string]int)
	for i, node := range cg.Nodes {
		nodeIDToIndex[node.ID] = i

		label := fmt.Sprintf("%s\\n%.2f%%\\n(%.2f%%)", node.Name, node.TotalPct, node.SelfPct)
		output.Objects = append(output.Objects, XDotObject{
			ID:    i,
			Name:  node.ID,
			Label: label,
			Shape: "box",
		})
	}

	// Convert edges
	for i, edge := range cg.Edges {
		sourceIdx, sourceOK := nodeIDToIndex[edge.Source]
		targetIdx, targetOK := nodeIDToIndex[edge.Target]

		if !sourceOK || !targetOK {
			continue
		}

		label := fmt.Sprintf("%.2f%%", edge.Weight)
		output.Edges = append(output.Edges, XDotEdge{
			ID:    i,
			Tail:  sourceIdx,
			Head:  targetIdx,
			Label: label,
		})
	}

	return output
}

// DOTWriter writes call graph data in DOT format.
type DOTWriter struct{}

// NewDOTWriter creates a new DOT format writer.
func NewDOTWriter() *DOTWriter {
	return &DOTWriter{}
}

// Write writes the call graph in DOT format.
func (w *DOTWriter) Write(cg *CallGraph, writer io.Writer) error {
	// Write header
	if _, err := fmt.Fprintln(writer, "digraph callgraph {"); err != nil {
		return err
	}

	// Write graph attributes
	if _, err := fmt.Fprintln(writer, "  node [shape=box];"); err != nil {
		return err
	}

	// Write nodes
	for _, node := range cg.Nodes {
		label := fmt.Sprintf("%s\\n%.2f%%\\n(%.2f%%)", node.Name, node.TotalPct, node.SelfPct)
		if _, err := fmt.Fprintf(writer, "  \"%s\" [label=\"%s\"];\n", node.ID, label); err != nil {
			return err
		}
	}

	// Write edges
	for _, edge := range cg.Edges {
		label := fmt.Sprintf("%.2f%%", edge.Weight)
		if _, err := fmt.Fprintf(writer, "  \"%s\" -> \"%s\" [label=\"%s\"];\n",
			edge.Source, edge.Target, label); err != nil {
			return err
		}
	}

	// Write footer
	if _, err := fmt.Fprintln(writer, "}"); err != nil {
		return err
	}

	return nil
}

// WriteToFile writes the call graph in DOT format to a file.
func (w *DOTWriter) WriteToFile(cg *CallGraph, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	return w.Write(cg, file)
}
