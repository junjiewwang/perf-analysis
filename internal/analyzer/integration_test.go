package analyzer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/internal/analyzer"
	"github.com/perf-analysis/pkg/model"
)

const testDataFile = "../../test/origin.data"

// TestJavaCPUAnalyzer_FullPipeline tests the complete analysis pipeline with real data.
func TestJavaCPUAnalyzer_FullPipeline(t *testing.T) {
	// Skip if test data file doesn't exist
	if _, err := os.Stat(testDataFile); os.IsNotExist(err) {
		t.Skip("Test data file not found:", testDataFile)
	}

	// Create temporary output directory
	tempDir := t.TempDir()
	taskDir := filepath.Join(tempDir, "test-integration-uuid")
	os.MkdirAll(taskDir, 0755)

	// Configure analyzer
	config := &analyzer.BaseAnalyzerConfig{
		OutputDir: tempDir,
		TopFuncsN: 20,
	}

	// Create analyzer
	javaCPUAnalyzer := analyzer.NewJavaCPUAnalyzer(config)

	// Open test data file
	file, err := os.Open(testDataFile)
	require.NoError(t, err)
	defer file.Close()

	// Create analysis request
	req := &model.AnalysisRequest{
		TaskID:       1,
		TaskUUID:     "test-integration-uuid",
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypePerf,
		OutputDir:    taskDir,
	}

	// Run analysis
	result, err := javaCPUAnalyzer.AnalyzeFromReader(context.Background(), req, file)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify result structure
	t.Run("VerifyResultStructure", func(t *testing.T) {
		assert.Equal(t, "test-integration-uuid", result.TaskUUID)
		assert.Greater(t, result.TotalRecords, 0)
		t.Logf("Total records: %d", result.TotalRecords)
	})

	// Verify response fields
	t.Run("VerifyResponseFields", func(t *testing.T) {
		// Verify Data is CPUProfilingData
		cpuData, ok := result.Data.(*model.CPUProfilingData)
		require.True(t, ok, "Data should be CPUProfilingData")

		// Verify top funcs
		assert.NotEmpty(t, cpuData.TopFuncs)
		t.Logf("Number of top funcs: %d", len(cpuData.TopFuncs))

		// Verify thread stats
		assert.NotEmpty(t, cpuData.ThreadStats)
		t.Logf("Number of threads: %d", len(cpuData.ThreadStats))

		// Verify file paths
		assert.NotEmpty(t, cpuData.FlameGraphFile)
		assert.NotEmpty(t, cpuData.CallGraphFile)
		t.Logf("Flame graph file: %s", cpuData.FlameGraphFile)
		t.Logf("Call graph file: %s", cpuData.CallGraphFile)
	})

	// Verify output files exist
	t.Run("VerifyOutputFiles", func(t *testing.T) {
		// Check flame graph file
		fgPath := filepath.Join(taskDir, "collapsed_data.json.gz")
		info, err := os.Stat(fgPath)
		require.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0))
		t.Logf("Flame graph file size: %d bytes", info.Size())

		// Check call graph file
		cgPath := filepath.Join(taskDir, "collapsed_data.json")
		info, err = os.Stat(cgPath)
		require.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0))
		t.Logf("Call graph file size: %d bytes", info.Size())
	})
}

// TestAnalyzerFactory_CreateJavaCPUAnalyzer tests the factory pattern.
func TestAnalyzerFactory_CreateJavaCPUAnalyzer(t *testing.T) {
	factory := analyzer.NewFactory(nil)

	a, err := factory.CreateAnalyzer(model.TaskTypeJava, model.ProfilerTypePerf)
	require.NoError(t, err)
	require.NotNil(t, a)

	assert.Equal(t, "java_cpu_analyzer", a.Name())
}

// TestAnalyzerFactory_CreateJavaMemAnalyzer tests creating memory analyzer.
func TestAnalyzerFactory_CreateJavaMemAnalyzer(t *testing.T) {
	factory := analyzer.NewFactory(nil)

	a, err := factory.CreateAnalyzer(model.TaskTypeJava, model.ProfilerTypeAsyncAlloc)
	require.NoError(t, err)
	require.NotNil(t, a)

	assert.Equal(t, "java_mem_analyzer", a.Name())
}

// TestParseAndStatistics tests parsing and statistics calculation.
func TestParseAndStatistics(t *testing.T) {
	// Skip if test data file doesn't exist
	if _, err := os.Stat(testDataFile); os.IsNotExist(err) {
		t.Skip("Test data file not found:", testDataFile)
	}

	tempDir := t.TempDir()
	config := &analyzer.BaseAnalyzerConfig{
		OutputDir: tempDir,
		TopFuncsN: 10,
	}

	baseAnalyzer := analyzer.NewBaseAnalyzer(config)

	// Open test data file
	file, err := os.Open(testDataFile)
	require.NoError(t, err)
	defer file.Close()

	// Parse data
	parseResult, err := baseAnalyzer.Parse(context.Background(), file)
	require.NoError(t, err)
	require.NotNil(t, parseResult)

	t.Logf("Total samples: %d", parseResult.TotalSamples)
	t.Logf("Number of sample entries: %d", len(parseResult.Samples))

	// Calculate top funcs
	topFuncsResult := baseAnalyzer.CalculateTopFuncs(parseResult.Samples)
	require.NotNil(t, topFuncsResult)
	require.NotEmpty(t, topFuncsResult.TopFuncs)

	t.Log("Top 10 functions:")
	for i, tf := range topFuncsResult.TopFuncs {
		if i >= 10 {
			break
		}
		t.Logf("  %d. %s: %.2f%% (self), %d samples",
			i+1, tf.Name, tf.SelfPercent, tf.SelfSamples)
	}

	// Calculate thread stats
	threadStatsResult := baseAnalyzer.CalculateThreadStats(parseResult.Samples)
	require.NotNil(t, threadStatsResult)

	t.Log("Thread statistics:")
	for i, ts := range threadStatsResult.Threads {
		if i >= 5 {
			break
		}
		t.Logf("  TID %d (%s): %d samples (%.2f%%)",
			ts.TID, ts.ThreadName, ts.Samples, ts.Percentage)
	}
}

// TestFlameGraphGeneration tests flame graph generation with real data.
func TestFlameGraphGeneration(t *testing.T) {
	// Skip if test data file doesn't exist
	if _, err := os.Stat(testDataFile); os.IsNotExist(err) {
		t.Skip("Test data file not found:", testDataFile)
	}

	tempDir := t.TempDir()
	baseAnalyzer := analyzer.NewBaseAnalyzer(&analyzer.BaseAnalyzerConfig{
		OutputDir: tempDir,
	})

	// Parse data
	file, err := os.Open(testDataFile)
	require.NoError(t, err)
	defer file.Close()

	parseResult, err := baseAnalyzer.Parse(context.Background(), file)
	require.NoError(t, err)

	// Generate flame graph
	fg, err := baseAnalyzer.GenerateFlameGraph(context.Background(), parseResult.Samples)
	require.NoError(t, err)
	require.NotNil(t, fg)

	t.Logf("Flame graph total samples: %d", fg.TotalSamples)
	t.Logf("Root node has %d children", len(fg.Root.Children))

	// Verify flame graph structure
	assert.Greater(t, fg.TotalSamples, int64(0))
	assert.NotNil(t, fg.Root)
}

// TestCallGraphGeneration tests call graph generation with real data.
func TestCallGraphGeneration(t *testing.T) {
	// Skip if test data file doesn't exist
	if _, err := os.Stat(testDataFile); os.IsNotExist(err) {
		t.Skip("Test data file not found:", testDataFile)
	}

	tempDir := t.TempDir()
	baseAnalyzer := analyzer.NewBaseAnalyzer(&analyzer.BaseAnalyzerConfig{
		OutputDir: tempDir,
	})

	// Parse data
	file, err := os.Open(testDataFile)
	require.NoError(t, err)
	defer file.Close()

	parseResult, err := baseAnalyzer.Parse(context.Background(), file)
	require.NoError(t, err)

	// Generate call graph
	cg, err := baseAnalyzer.GenerateCallGraph(context.Background(), parseResult.Samples)
	require.NoError(t, err)
	require.NotNil(t, cg)

	t.Logf("Call graph has %d nodes and %d edges", len(cg.Nodes), len(cg.Edges))

	// Verify call graph structure
	assert.NotEmpty(t, cg.Nodes)
	assert.NotEmpty(t, cg.Edges)
}

// BenchmarkFullPipeline benchmarks the complete analysis pipeline.
func BenchmarkFullPipeline(b *testing.B) {
	// Skip if test data file doesn't exist
	if _, err := os.Stat(testDataFile); os.IsNotExist(err) {
		b.Skip("Test data file not found:", testDataFile)
	}

	tempDir := b.TempDir()
	config := &analyzer.BaseAnalyzerConfig{
		OutputDir: tempDir,
		TopFuncsN: 50,
	}

	javaCPUAnalyzer := analyzer.NewJavaCPUAnalyzer(config)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		file, _ := os.Open(testDataFile)
		taskDir := filepath.Join(tempDir, "bench-uuid")
		os.MkdirAll(taskDir, 0755)

		req := &model.AnalysisRequest{
			TaskID:       int64(i),
			TaskUUID:     "bench-uuid",
			TaskType:     model.TaskTypeJava,
			ProfilerType: model.ProfilerTypePerf,
			OutputDir:    taskDir,
		}

		_, _ = javaCPUAnalyzer.AnalyzeFromReader(context.Background(), req, file)
		file.Close()
	}
}
