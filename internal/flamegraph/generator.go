package flamegraph

import (
	"context"
	"io"
	"strings"

	"github.com/perf-analysis/pkg/model"
)

// GeneratorOptions holds configuration options for the flame graph generator.
type GeneratorOptions struct {
	// MinPercent is the minimum percentage for a node to be included.
	MinPercent float64

	// IncludeModule includes module information in the output.
	IncludeModule bool
}

// DefaultGeneratorOptions returns default generator options.
func DefaultGeneratorOptions() *GeneratorOptions {
	return &GeneratorOptions{
		MinPercent:    0.01, // 0.01% minimum
		IncludeModule: true,
	}
}

// Generator generates flame graph data from parsed samples.
type Generator struct {
	opts *GeneratorOptions
}

// NewGenerator creates a new flame graph generator.
func NewGenerator(opts *GeneratorOptions) *Generator {
	if opts == nil {
		opts = DefaultGeneratorOptions()
	}
	return &Generator{opts: opts}
}

// Generate generates a flame graph from the given samples.
func (g *Generator) Generate(ctx context.Context, samples []*model.Sample) (*FlameGraph, error) {
	fg := NewFlameGraph()

	for _, sample := range samples {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		g.appendStack(fg, sample)
	}

	fg.TotalSamples = fg.Root.Value
	fg.Cleanup(g.opts.MinPercent)
	fg.CalculateMaxDepth()

	return fg, nil
}

// appendStack appends a sample's call stack to the flame graph.
func (g *Generator) appendStack(fg *FlameGraph, sample *model.Sample) {
	if len(sample.CallStack) == 0 {
		return
	}

	node := fg.Root
	node.Value += sample.Value

	// Process name is the first element, rest is call stack
	process := sample.ThreadName
	tid := sample.TID

	for _, frame := range sample.CallStack {
		function, module := splitFuncAndModule(frame)

		child := node.GetChild(process, tid, function, module)
		if child == nil {
			child = NewNode(process, tid, function, module, 0)
			node.AddChild(child)
		}

		child.Value += sample.Value
		node = child
	}
}

// splitFuncAndModule splits a frame into function name and module.
// e.g., "func(module)" => ("func", "module")
func splitFuncAndModule(frame string) (function, module string) {
	lastParen := strings.LastIndex(frame, "(")
	if lastParen == -1 || !strings.HasSuffix(frame, ")") {
		return frame, ""
	}

	function = frame[:lastParen]
	module = frame[lastParen+1 : len(frame)-1]
	return function, module
}

// GenerateFromParseResult generates a flame graph from a parse result.
func (g *Generator) GenerateFromParseResult(ctx context.Context, result *model.ParseResult) (*FlameGraph, error) {
	return g.Generate(ctx, result.Samples)
}

// Writer defines the interface for writing flame graph output.
type Writer interface {
	Write(fg *FlameGraph, w io.Writer) error
}
