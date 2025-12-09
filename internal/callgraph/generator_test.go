package callgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/pkg/model"
)

func TestGenerator_Generate_Basic(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"func1", "func2", "func3"}, Value: 100},
		{ThreadName: "main", TID: 1, CallStack: []string{"func1", "func2", "func4"}, Value: 50},
		{ThreadName: "worker", TID: 2, CallStack: []string{"func1", "func5"}, Value: 30},
	}

	gen := NewGenerator(&GeneratorOptions{MinNodePct: 0, MinEdgePct: 0})
	cg, err := gen.Generate(context.Background(), samples)

	require.NoError(t, err)
	require.NotNil(t, cg)

	assert.Equal(t, int64(180), cg.TotalSamples)
	assert.NotEmpty(t, cg.Nodes)
	assert.NotEmpty(t, cg.Edges)
}

func TestGenerator_Generate_EmptySamples(t *testing.T) {
	gen := NewGenerator(nil)
	cg, err := gen.Generate(context.Background(), []*model.Sample{})

	require.NoError(t, err)
	require.NotNil(t, cg)

	assert.Equal(t, int64(0), cg.TotalSamples)
	assert.Empty(t, cg.Nodes)
	assert.Empty(t, cg.Edges)
}

func TestGenerator_Generate_SelfTime(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"caller", "callee"}, Value: 100},
	}

	gen := NewGenerator(&GeneratorOptions{MinNodePct: 0, MinEdgePct: 0})
	cg, err := gen.Generate(context.Background(), samples)

	require.NoError(t, err)

	// callee should have self time (it's the leaf)
	var callee *Node
	for _, node := range cg.Nodes {
		if node.Name == "callee" {
			callee = node
			break
		}
	}
	require.NotNil(t, callee)
	assert.Equal(t, int64(100), callee.SelfTime)

	// caller should not have self time
	var caller *Node
	for _, node := range cg.Nodes {
		if node.Name == "caller" {
			caller = node
			break
		}
	}
	require.NotNil(t, caller)
	assert.Equal(t, int64(0), caller.SelfTime)
}

func TestGenerator_Generate_ContextCancellation(t *testing.T) {
	samples := make([]*model.Sample, 1000)
	for i := range samples {
		samples[i] = &model.Sample{
			ThreadName: "thread",
			TID:        1,
			CallStack:  []string{"func"},
			Value:      1,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	gen := NewGenerator(nil)
	_, err := gen.Generate(ctx, samples)

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestGenerator_Generate_WithModules(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"func1(mod1)", "func2(mod2)"}, Value: 100},
	}

	gen := NewGenerator(&GeneratorOptions{MinNodePct: 0, MinEdgePct: 0, IncludeModule: true})
	cg, err := gen.Generate(context.Background(), samples)

	require.NoError(t, err)

	// Check that module was extracted
	var func1 *Node
	for _, node := range cg.Nodes {
		if node.Name == "func1" {
			func1 = node
			break
		}
	}
	require.NotNil(t, func1)
	assert.Equal(t, "mod1", func1.Module)
}

func TestGenerator_Generate_Filtering(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"hot"}, Value: 900},
		{ThreadName: "main", TID: 1, CallStack: []string{"cold"}, Value: 100},
	}

	// Use high threshold to filter cold functions
	gen := NewGenerator(&GeneratorOptions{MinNodePct: 50, MinEdgePct: 0})
	cg, err := gen.Generate(context.Background(), samples)

	require.NoError(t, err)

	// Only hot should remain (90% > 50%)
	assert.Len(t, cg.Nodes, 1)
	assert.Equal(t, "hot", cg.Nodes[0].Name)
}

func TestJSONWriter_Write(t *testing.T) {
	cg := NewCallGraph()
	cg.TotalSamples = 100
	cg.AddNode("func1", "", 50, 100)
	cg.AddNode("func2", "", 30, 50)
	cg.AddEdge("func1", "", "func2", "", 50)
	cg.CalculatePercentages()
	cg.Cleanup(0, 0)

	var buf bytes.Buffer
	writer := NewJSONWriter()
	err := writer.Write(cg, &buf)

	require.NoError(t, err)

	// Parse the output
	var result CallGraph
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, int64(100), result.TotalSamples)
	assert.Len(t, result.Nodes, 2)
	assert.Len(t, result.Edges, 1)
}

func TestJSONWriter_WriteToFile(t *testing.T) {
	cg := NewCallGraph()
	cg.TotalSamples = 100
	cg.AddNode("func1", "", 50, 100)
	cg.Cleanup(0, 0)

	tempDir := t.TempDir()
	filepath := filepath.Join(tempDir, "test.json")

	writer := NewJSONWriter()
	err := writer.WriteToFile(cg, filepath)

	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(filepath)
	require.NoError(t, err)

	// Read and verify content
	data, err := os.ReadFile(filepath)
	require.NoError(t, err)

	var result CallGraph
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)
	assert.Len(t, result.Nodes, 1)
}

func TestXDotWriter_Write(t *testing.T) {
	cg := NewCallGraph()
	cg.TotalSamples = 100
	cg.AddNode("func1", "", 50, 100)
	cg.AddNode("func2", "", 30, 50)
	cg.AddEdge("func1", "", "func2", "", 50)
	cg.CalculatePercentages()
	cg.Cleanup(0, 0)

	var buf bytes.Buffer
	writer := NewXDotWriter()
	err := writer.Write(cg, &buf)

	require.NoError(t, err)

	// Parse the output
	var result XDotJSONOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Directed)
	assert.Len(t, result.Objects, 2)
	assert.Len(t, result.Edges, 1)
}

func TestDOTWriter_Write(t *testing.T) {
	cg := NewCallGraph()
	cg.TotalSamples = 100
	cg.AddNode("func1", "", 50, 100)
	cg.AddNode("func2", "", 30, 50)
	cg.AddEdge("func1", "", "func2", "", 50)
	cg.CalculatePercentages()
	cg.Cleanup(0, 0)

	var buf bytes.Buffer
	writer := NewDOTWriter()
	err := writer.Write(cg, &buf)

	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "digraph callgraph")
	assert.Contains(t, output, "func1")
	assert.Contains(t, output, "func2")
	assert.Contains(t, output, "->")
}

func TestDOTWriter_WriteToFile(t *testing.T) {
	cg := NewCallGraph()
	cg.TotalSamples = 100
	cg.AddNode("func1", "", 50, 100)
	cg.Cleanup(0, 0)

	tempDir := t.TempDir()
	fp := filepath.Join(tempDir, "test.dot")

	writer := NewDOTWriter()
	err := writer.WriteToFile(cg, fp)

	require.NoError(t, err)

	// Verify file content
	data, err := os.ReadFile(fp)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "digraph"))
}

// Benchmark tests
func BenchmarkGenerator_Generate(b *testing.B) {
	samples := make([]*model.Sample, 10000)
	for i := range samples {
		samples[i] = &model.Sample{
			ThreadName: "thread",
			TID:        i % 10,
			CallStack:  []string{"func1", "func2", "func3", "func4", "func5"},
			Value:      100,
		}
	}

	gen := NewGenerator(&GeneratorOptions{MinNodePct: 0.5, MinEdgePct: 0.1})
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = gen.Generate(context.Background(), samples)
	}
}

func BenchmarkJSONWriter_Write(b *testing.B) {
	cg := NewCallGraph()
	cg.TotalSamples = 10000
	for i := 0; i < 100; i++ {
		cg.AddNode("func"+string(rune('a'+i%26)), "", int64(i), int64(i*2))
	}
	cg.CalculatePercentages()
	cg.Cleanup(0, 0)

	writer := NewJSONWriter()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = writer.Write(cg, &buf)
	}
}
