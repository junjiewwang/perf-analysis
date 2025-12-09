package integration

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/internal/callgraph"
	"github.com/perf-analysis/internal/flamegraph"
	"github.com/perf-analysis/internal/parser/collapsed"
	"github.com/perf-analysis/internal/statistics"
)

// getTestDataPath returns the path to test data files.
func getTestDataPath(filename string) string {
	_, currentFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(currentFile)
	return filepath.Join(dir, "..", "parser", "collapsed", "testdata", filename)
}

func TestFullAnalysisPipeline_JavaCPU(t *testing.T) {
	filePath := getTestDataPath("java_cpu.folded")

	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	ctx := context.Background()

	// Step 1: Parse the collapsed data
	parser := collapsed.NewParser(collapsed.DefaultParserOptions())
	parseResult, err := parser.Parse(ctx, file)

	require.NoError(t, err)
	require.NotNil(t, parseResult)
	assert.Greater(t, parseResult.TotalSamples, int64(0))

	// Step 2: Generate flame graph
	fgGen := flamegraph.NewGenerator(flamegraph.DefaultGeneratorOptions())
	fg, err := fgGen.Generate(ctx, parseResult.Samples)

	require.NoError(t, err)
	require.NotNil(t, fg)
	// Note: FlameGraph includes all samples (including swapper),
	// while ParseResult.TotalSamples excludes swapper
	assert.Greater(t, fg.TotalSamples, int64(0))
	assert.NotNil(t, fg.Root)

	// Step 3: Generate call graph
	cgGen := callgraph.NewGenerator(&callgraph.GeneratorOptions{
		MinNodePct: 0,
		MinEdgePct: 0,
	})
	cg, err := cgGen.Generate(ctx, parseResult.Samples)

	require.NoError(t, err)
	require.NotNil(t, cg)
	assert.NotEmpty(t, cg.Nodes)

	// Step 4: Calculate top functions
	topFuncsCalc := statistics.NewTopFuncsCalculator(statistics.WithTopN(10))
	topFuncsResult := topFuncsCalc.Calculate(parseResult.Samples)

	require.NotNil(t, topFuncsResult)
	assert.NotEmpty(t, topFuncsResult.TopFuncs)

	// Step 5: Calculate thread stats
	threadStatsCalc := statistics.NewThreadStatsCalculator()
	threadStatsResult := threadStatsCalc.Calculate(parseResult.Samples)

	require.NotNil(t, threadStatsResult)
	assert.NotEmpty(t, threadStatsResult.Threads)

	t.Logf("Parse Result: %d samples, %d total", len(parseResult.Samples), parseResult.TotalSamples)
	t.Logf("Flame Graph: %d total samples, depth %d", fg.TotalSamples, fg.MaxDepth)
	t.Logf("Call Graph: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
	t.Logf("Top Functions: %d entries", len(topFuncsResult.TopFuncs))
	t.Logf("Thread Stats: %d threads", len(threadStatsResult.Threads))
}

func TestFullAnalysisPipeline_APMFormat(t *testing.T) {
	filePath := getTestDataPath("apm_format.folded")

	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	ctx := context.Background()

	// Parse
	parser := collapsed.NewParser(nil)
	parseResult, err := parser.Parse(ctx, file)

	require.NoError(t, err)
	assert.Greater(t, parseResult.TotalSamples, int64(0))

	// Generate flame graph
	fgGen := flamegraph.NewGenerator(nil)
	fg, err := fgGen.Generate(ctx, parseResult.Samples)

	require.NoError(t, err)
	assert.NotNil(t, fg.Root)

	// Generate call graph
	cgGen := callgraph.NewGenerator(nil)
	cg, err := cgGen.Generate(ctx, parseResult.Samples)

	require.NoError(t, err)
	assert.NotEmpty(t, cg.Nodes)
}

func TestFlameGraphOutput_GzipJSON(t *testing.T) {
	filePath := getTestDataPath("java_cpu.folded")

	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	ctx := context.Background()

	// Parse and generate
	parser := collapsed.NewParser(nil)
	parseResult, err := parser.Parse(ctx, file)
	require.NoError(t, err)

	fgGen := flamegraph.NewGenerator(nil)
	fg, err := fgGen.Generate(ctx, parseResult.Samples)
	require.NoError(t, err)

	// Write to gzip
	var buf bytes.Buffer
	gzWriter := flamegraph.NewGzipWriter()
	err = gzWriter.Write(fg, &buf)
	require.NoError(t, err)

	// Verify it's valid gzip and JSON
	gzReader, err := gzip.NewReader(&buf)
	require.NoError(t, err)
	defer gzReader.Close()

	decompressed, err := io.ReadAll(gzReader)
	require.NoError(t, err)

	var node flamegraph.Node
	err = json.Unmarshal(decompressed, &node)
	require.NoError(t, err)

	assert.Equal(t, "root", node.Func)
	assert.Greater(t, node.Value, int64(0))
}

func TestCallGraphOutput_JSON(t *testing.T) {
	filePath := getTestDataPath("java_cpu.folded")

	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	ctx := context.Background()

	// Parse and generate
	parser := collapsed.NewParser(nil)
	parseResult, err := parser.Parse(ctx, file)
	require.NoError(t, err)

	cgGen := callgraph.NewGenerator(&callgraph.GeneratorOptions{
		MinNodePct: 0,
		MinEdgePct: 0,
	})
	cg, err := cgGen.Generate(ctx, parseResult.Samples)
	require.NoError(t, err)

	// Write to JSON
	var buf bytes.Buffer
	jsonWriter := callgraph.NewJSONWriter()
	err = jsonWriter.Write(cg, &buf)
	require.NoError(t, err)

	// Verify it's valid JSON
	var result callgraph.CallGraph
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Nodes)
	assert.Greater(t, result.TotalSamples, int64(0))
}

func TestCallGraphOutput_XDotJSON(t *testing.T) {
	filePath := getTestDataPath("java_cpu.folded")

	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	ctx := context.Background()

	// Parse and generate
	parser := collapsed.NewParser(nil)
	parseResult, err := parser.Parse(ctx, file)
	require.NoError(t, err)

	cgGen := callgraph.NewGenerator(&callgraph.GeneratorOptions{
		MinNodePct: 0,
		MinEdgePct: 0,
	})
	cg, err := cgGen.Generate(ctx, parseResult.Samples)
	require.NoError(t, err)

	// Write to xdot_json format
	var buf bytes.Buffer
	xdotWriter := callgraph.NewXDotWriter()
	err = xdotWriter.Write(cg, &buf)
	require.NoError(t, err)

	// Verify it's valid JSON with expected structure
	var result callgraph.XDotJSONOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Directed)
	assert.NotEmpty(t, result.Objects)
}

func TestWriteToTempFiles(t *testing.T) {
	filePath := getTestDataPath("java_cpu.folded")

	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	ctx := context.Background()

	// Parse
	parser := collapsed.NewParser(nil)
	parseResult, err := parser.Parse(ctx, file)
	require.NoError(t, err)

	// Generate
	fgGen := flamegraph.NewGenerator(nil)
	fg, err := fgGen.Generate(ctx, parseResult.Samples)
	require.NoError(t, err)

	cgGen := callgraph.NewGenerator(&callgraph.GeneratorOptions{MinNodePct: 0, MinEdgePct: 0})
	cg, err := cgGen.Generate(ctx, parseResult.Samples)
	require.NoError(t, err)

	// Create temp directory
	tempDir := t.TempDir()

	// Write flame graph to gzip file
	fgPath := filepath.Join(tempDir, "flamegraph.json.gz")
	gzWriter := flamegraph.NewGzipWriter()
	stats, err := gzWriter.WriteToFileWithStats(fg, fgPath)
	require.NoError(t, err)
	t.Logf("Flame graph: JSON size %.2fKB, compressed %.2fKB (%.1f%%)",
		float64(stats.JSONSize)/1024, float64(stats.CompressedSize)/1024, stats.CompressionPct)

	// Write call graph to JSON file
	cgPath := filepath.Join(tempDir, "callgraph.json")
	jsonWriter := callgraph.NewJSONWriter()
	err = jsonWriter.WriteToFile(cg, cgPath)
	require.NoError(t, err)

	// Verify files exist
	_, err = os.Stat(fgPath)
	require.NoError(t, err)

	_, err = os.Stat(cgPath)
	require.NoError(t, err)
}
