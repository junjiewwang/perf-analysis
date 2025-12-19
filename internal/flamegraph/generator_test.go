package flamegraph

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/profiling"
)

func TestGenerator_Generate_Basic(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"func1", "func2", "func3"}, Value: 100},
		{ThreadName: "main", TID: 1, CallStack: []string{"func1", "func2", "func4"}, Value: 50},
		{ThreadName: "worker", TID: 2, CallStack: []string{"func1", "func5"}, Value: 30},
	}

	opts := DefaultGeneratorOptions()
	opts.EnableThreadAnalysis = false // Simple mode for this test
	gen := NewGenerator(opts)
	fg, err := gen.Generate(context.Background(), samples)

	require.NoError(t, err)
	require.NotNil(t, fg)

	assert.Equal(t, int64(180), fg.TotalSamples)
	assert.Equal(t, int64(180), fg.Root.Value)
	assert.NotEmpty(t, fg.Root.Children)
}

func TestGenerator_Generate_WithThreadAnalysis(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"func1", "func2"}, Value: 100},
		{ThreadName: "worker", TID: 2, CallStack: []string{"func1", "func3"}, Value: 50},
	}

	opts := DefaultGeneratorOptions()
	opts.EnableThreadAnalysis = true
	gen := NewGenerator(opts)
	fg, err := gen.Generate(context.Background(), samples)

	require.NoError(t, err)
	require.NotNil(t, fg)
	require.NotNil(t, fg.ThreadAnalysis)

	assert.Equal(t, 2, fg.ThreadAnalysis.TotalThreads)
	assert.Len(t, fg.ThreadAnalysis.Threads, 2)
	assert.NotEmpty(t, fg.ThreadAnalysis.TopFunctions)
}

func TestGenerator_Generate_EmptySamples(t *testing.T) {
	gen := NewGenerator(nil)
	fg, err := gen.Generate(context.Background(), []*model.Sample{})

	require.NoError(t, err)
	require.NotNil(t, fg)

	assert.Equal(t, int64(0), fg.TotalSamples)
	assert.Equal(t, int64(0), fg.Root.Value)
}

func TestGenerator_Generate_WithModules(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"func1(mod1)", "func2(mod2)"}, Value: 100},
	}

	opts := DefaultGeneratorOptions()
	opts.EnableThreadAnalysis = false
	gen := NewGenerator(opts)
	fg, err := gen.Generate(context.Background(), samples)

	require.NoError(t, err)
	require.NotNil(t, fg)

	// Check that thread name is the first level (when IncludeThreadInStack is true)
	assert.NotEmpty(t, fg.Root.Children)
	threadNode := fg.Root.Children[0]
	assert.Equal(t, "main", threadNode.Name) // Thread name is first level

	// Check that module was extracted in the function node
	require.NotEmpty(t, threadNode.Children)
	funcNode := threadNode.Children[0]
	assert.Equal(t, "func1", funcNode.Name)
	assert.Equal(t, "mod1", funcNode.Module)
}

func TestGenerator_Generate_WithModules_NoThreadInStack(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"func1(mod1)", "func2(mod2)"}, Value: 100},
	}

	opts := DefaultGeneratorOptions()
	opts.EnableThreadAnalysis = false
	opts.IncludeThreadInStack = false // Disable thread in stack
	gen := NewGenerator(opts)
	fg, err := gen.Generate(context.Background(), samples)

	require.NoError(t, err)
	require.NotNil(t, fg)

	// Check that function is the first level (when IncludeThreadInStack is false)
	assert.NotEmpty(t, fg.Root.Children)
	child := fg.Root.Children[0]
	assert.Equal(t, "func1", child.Name)
	assert.Equal(t, "mod1", child.Module)
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

func TestGenerator_Generate_Aggregation(t *testing.T) {
	// Same call stack should be aggregated
	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"a", "b"}, Value: 50},
		{ThreadName: "main", TID: 1, CallStack: []string{"a", "b"}, Value: 30},
		{ThreadName: "main", TID: 1, CallStack: []string{"a", "b"}, Value: 20},
	}

	opts := DefaultGeneratorOptions()
	opts.EnableThreadAnalysis = false
	gen := NewGenerator(opts)
	fg, err := gen.Generate(context.Background(), samples)

	require.NoError(t, err)

	// Should be aggregated into single path
	assert.Len(t, fg.Root.Children, 1)
	assert.Equal(t, int64(100), fg.Root.Children[0].Value)
}

func TestGenerator_ThreadGroups(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "pool-1-thread-1", TID: 1, CallStack: []string{"func"}, Value: 50},
		{ThreadName: "pool-1-thread-2", TID: 2, CallStack: []string{"func"}, Value: 30},
		{ThreadName: "pool-2-thread-1", TID: 3, CallStack: []string{"func"}, Value: 20},
	}

	gen := NewGenerator(nil)
	fg, err := gen.Generate(context.Background(), samples)

	require.NoError(t, err)
	require.NotNil(t, fg.ThreadAnalysis)

	// Should have 2 thread groups
	assert.Len(t, fg.ThreadAnalysis.ThreadGroups, 2)
}

func TestSplitFuncAndModule(t *testing.T) {
	tests := []struct {
		input      string
		wantFunc   string
		wantModule string
	}{
		{"func(module)", "func", "module"},
		{"func", "func", ""},
		{"java.lang.Thread.run(Thread.java)", "java.lang.Thread.run", "Thread.java"},
		{"func(", "func(", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotFunc, gotModule := profiling.SplitFuncAndModule(tt.input)
			assert.Equal(t, tt.wantFunc, gotFunc)
			assert.Equal(t, tt.wantModule, gotModule)
		})
	}
}

func TestJSONWriter_Write(t *testing.T) {
	fg := NewFlameGraph()
	fg.Root.Value = 100
	child := NewNode("func1", 100)
	fg.Root.AddChild(child)
	fg.Cleanup(0)

	var buf bytes.Buffer
	writer := NewJSONWriter()
	err := writer.Write(fg, &buf)

	require.NoError(t, err)

	// Parse the output as FlameGraph
	var result FlameGraph
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "root", result.Root.Name)
	assert.Equal(t, int64(100), result.Root.Value)
}

func TestPrettyJSONWriter_Write(t *testing.T) {
	fg := NewFlameGraph()
	fg.Root.Value = 100

	var buf bytes.Buffer
	writer := NewPrettyJSONWriter()
	err := writer.Write(fg, &buf)

	require.NoError(t, err)
	// Should contain indentation
	assert.Contains(t, buf.String(), "  ")
}

func TestGzipWriter_Write(t *testing.T) {
	fg := NewFlameGraph()
	fg.Root.Value = 100
	child := NewNode("func1", 100)
	fg.Root.AddChild(child)
	fg.Cleanup(0)

	var buf bytes.Buffer
	writer := NewGzipWriter()
	err := writer.Write(fg, &buf)

	require.NoError(t, err)

	// Decompress and verify
	gzReader, err := gzip.NewReader(&buf)
	require.NoError(t, err)
	defer gzReader.Close()

	decompressed, err := io.ReadAll(gzReader)
	require.NoError(t, err)

	var result FlameGraph
	err = json.Unmarshal(decompressed, &result)
	require.NoError(t, err)

	assert.Equal(t, "root", result.Root.Name)
}

func TestGzipWriter_WriteToFile(t *testing.T) {
	fg := NewFlameGraph()
	fg.Root.Value = 100

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.json.gz")

	writer := NewGzipWriter()
	err := writer.WriteToFile(fg, filePath)

	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(filePath)
	require.NoError(t, err)
}

func TestGzipWriter_WriteToFileWithStats(t *testing.T) {
	fg := NewFlameGraph()
	fg.Root.Value = 10000
	// Add enough children to make compression effective
	for i := 0; i < 100; i++ {
		child := NewNode("function_with_longer_name_for_compression", 100)
		for j := 0; j < 5; j++ {
			grandchild := NewNode("nested_function_name", 20)
			child.AddChild(grandchild)
		}
		fg.Root.AddChild(child)
	}
	fg.Cleanup(0)

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.json.gz")

	writer := NewGzipWriter()
	stats, err := writer.WriteToFileWithStats(fg, filePath)

	require.NoError(t, err)
	require.NotNil(t, stats)

	assert.Greater(t, stats.JSONSize, int64(0))
	assert.Greater(t, stats.CompressedSize, int64(0))
	// CompressionPct can be > 100 for small data, just verify it's calculated
	assert.NotEqual(t, 0.0, stats.CompressionPct)
}

func TestFoldedWriter_Write(t *testing.T) {
	fg := NewFlameGraph()
	fg.Root.Value = 100

	// Build: root -> a -> b
	a := NewNode("a", 100)
	b := NewNode("b", 100)
	a.Children = []*Node{b}
	fg.Root.Children = []*Node{a}

	var buf bytes.Buffer
	writer := NewFoldedWriter()
	err := writer.Write(fg, &buf)

	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "a;b")
	assert.Contains(t, output, "100")
}

func TestIsSwapperThread(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"swapper", true},
		{"swapper/0", true},
		{"[swapper]", true},
		{"[swapper/1]", true},
		{"main", false},
		{"worker-1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, profiling.IsSwapperThread(tt.name))
		})
	}
}

func TestExtractThreadGroup(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"pool-1-thread-1", "pool-1-thread"},
		{"worker-5", "worker"},
		{"main", "main"},
		{"http-nio-8080-exec-10", "http-nio-8080-exec"},
		{"123", "123"}, // All numbers, keep as-is
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, profiling.ExtractThreadGroup(tt.input))
		})
	}
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

	opts := DefaultGeneratorOptions()
	opts.EnableThreadAnalysis = false
	gen := NewGenerator(opts)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = gen.Generate(context.Background(), samples)
	}
}

func BenchmarkGenerator_GenerateWithAnalysis(b *testing.B) {
	samples := make([]*model.Sample, 10000)
	for i := range samples {
		samples[i] = &model.Sample{
			ThreadName: "thread",
			TID:        i % 10,
			CallStack:  []string{"func1", "func2", "func3", "func4", "func5"},
			Value:      100,
		}
	}

	gen := NewGenerator(nil)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = gen.Generate(context.Background(), samples)
	}
}

func BenchmarkGzipWriter_Write(b *testing.B) {
	fg := NewFlameGraph()
	fg.Root.Value = 10000
	for i := 0; i < 100; i++ {
		child := NewNode("func", 100)
		fg.Root.AddChild(child)
	}
	fg.Cleanup(0)

	writer := NewGzipWriter()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = writer.Write(fg, &buf)
	}
}
