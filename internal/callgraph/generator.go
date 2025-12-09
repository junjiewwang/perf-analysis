package callgraph

import (
	"context"
	"strings"

	"github.com/perf-analysis/pkg/model"
)

// GeneratorOptions holds configuration options for the call graph generator.
type GeneratorOptions struct {
	// MinNodePct is the minimum percentage for a node to be included.
	MinNodePct float64

	// MinEdgePct is the minimum percentage for an edge to be included.
	MinEdgePct float64

	// IncludeModule includes module information in the output.
	IncludeModule bool
}

// DefaultGeneratorOptions returns default generator options.
func DefaultGeneratorOptions() *GeneratorOptions {
	return &GeneratorOptions{
		MinNodePct:    0.5, // 0.5% minimum for nodes
		MinEdgePct:    0.1, // 0.1% minimum for edges
		IncludeModule: true,
	}
}

// Generator generates call graph data from parsed samples.
type Generator struct {
	opts *GeneratorOptions
}

// NewGenerator creates a new call graph generator.
func NewGenerator(opts *GeneratorOptions) *Generator {
	if opts == nil {
		opts = DefaultGeneratorOptions()
	}
	return &Generator{opts: opts}
}

// Generate generates a call graph from the given samples.
func (g *Generator) Generate(ctx context.Context, samples []*model.Sample) (*CallGraph, error) {
	cg := NewCallGraph()

	// Track self time for each function (time at top of stack)
	selfTime := make(map[string]int64)

	for _, sample := range samples {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if len(sample.CallStack) == 0 {
			continue
		}

		cg.TotalSamples += sample.Value

		// Process each frame in the call stack
		for i, frame := range sample.CallStack {
			function, module := g.splitFuncAndModule(frame)

			// Add node (accumulate total time)
			cg.AddNode(function, module, 0, sample.Value)

			// Track self time (only for leaf/top function)
			if i == len(sample.CallStack)-1 {
				nodeID := makeNodeID(function, module)
				selfTime[nodeID] += sample.Value
			}

			// Add edge from caller to callee
			if i > 0 {
				callerFrame := sample.CallStack[i-1]
				callerFunc, callerModule := g.splitFuncAndModule(callerFrame)
				cg.AddEdge(callerFunc, callerModule, function, module, sample.Value)
			}
		}
	}

	// Update self times
	for nodeID, time := range selfTime {
		if node := cg.nodeMap[nodeID]; node != nil {
			node.SelfTime = time
		}
	}

	// Calculate percentages and cleanup
	cg.CalculatePercentages()
	cg.Cleanup(g.opts.MinNodePct, g.opts.MinEdgePct)

	return cg, nil
}

// splitFuncAndModule splits a frame into function name and module.
func (g *Generator) splitFuncAndModule(frame string) (function, module string) {
	lastParen := strings.LastIndex(frame, "(")
	if lastParen == -1 || !strings.HasSuffix(frame, ")") {
		return frame, ""
	}

	function = frame[:lastParen]
	if g.opts.IncludeModule {
		module = frame[lastParen+1 : len(frame)-1]
	}
	return function, module
}

// GenerateFromParseResult generates a call graph from a parse result.
func (g *Generator) GenerateFromParseResult(ctx context.Context, result *model.ParseResult) (*CallGraph, error) {
	return g.Generate(ctx, result.Samples)
}
