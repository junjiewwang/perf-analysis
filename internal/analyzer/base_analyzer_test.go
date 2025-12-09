package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/internal/statistics"
	"github.com/perf-analysis/pkg/model"
)

func TestNewBaseAnalyzer(t *testing.T) {
	// Test with nil config
	analyzer := NewBaseAnalyzer(nil)
	assert.NotNil(t, analyzer)
	assert.NotNil(t, analyzer.config)
	assert.NotNil(t, analyzer.parser)
	assert.NotNil(t, analyzer.flameGraphGen)
	assert.NotNil(t, analyzer.callGraphGen)
	assert.NotNil(t, analyzer.topFuncsCalc)
	assert.NotNil(t, analyzer.threadStatsCalc)

	// Test with custom config
	config := &BaseAnalyzerConfig{
		OutputDir: "/tmp/test",
		TopFuncsN: 100,
	}
	analyzer = NewBaseAnalyzer(config)
	assert.Equal(t, "/tmp/test", analyzer.config.OutputDir)
	assert.Equal(t, 100, analyzer.config.TopFuncsN)
}

func TestBaseAnalyzer_Parse(t *testing.T) {
	analyzer := NewBaseAnalyzer(nil)

	input := `main-thread;java.lang.Thread.run;com.example.App.main 100
worker-1;java.lang.Thread.run;com.example.Worker.process 50`

	result, err := analyzer.Parse(context.Background(), strings.NewReader(input))

	require.NoError(t, err)
	assert.Equal(t, int64(150), result.TotalSamples)
	assert.Len(t, result.Samples, 2)
}

func TestBaseAnalyzer_GenerateFlameGraph(t *testing.T) {
	analyzer := NewBaseAnalyzer(nil)

	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"func1", "func2"}, Value: 100},
		{ThreadName: "main", TID: 1, CallStack: []string{"func1", "func3"}, Value: 50},
	}

	fg, err := analyzer.GenerateFlameGraph(context.Background(), samples)

	require.NoError(t, err)
	assert.NotNil(t, fg)
	assert.Equal(t, int64(150), fg.TotalSamples)
}

func TestBaseAnalyzer_GenerateCallGraph(t *testing.T) {
	analyzer := NewBaseAnalyzer(nil)

	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"func1", "func2"}, Value: 100},
	}

	cg, err := analyzer.GenerateCallGraph(context.Background(), samples)

	require.NoError(t, err)
	assert.NotNil(t, cg)
	assert.NotEmpty(t, cg.Nodes)
}

func TestBaseAnalyzer_CalculateTopFuncs(t *testing.T) {
	analyzer := NewBaseAnalyzer(nil)

	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"hot_func"}, Value: 100},
		{ThreadName: "main", TID: 1, CallStack: []string{"cold_func"}, Value: 10},
	}

	result := analyzer.CalculateTopFuncs(samples)

	require.NotNil(t, result)
	assert.NotEmpty(t, result.TopFuncs)
	// hot_func should be first
	assert.Equal(t, "hot_func", result.TopFuncs[0].Name)
}

func TestBaseAnalyzer_CalculateThreadStats(t *testing.T) {
	analyzer := NewBaseAnalyzer(nil)

	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"func"}, Value: 100},
		{ThreadName: "worker", TID: 2, CallStack: []string{"func"}, Value: 50},
	}

	result := analyzer.CalculateThreadStats(samples)

	require.NotNil(t, result)
	assert.Len(t, result.Threads, 2)
}

func TestBaseAnalyzer_EnsureOutputDir(t *testing.T) {
	tempDir := t.TempDir()
	config := &BaseAnalyzerConfig{
		OutputDir: tempDir,
	}
	analyzer := NewBaseAnalyzer(config)

	taskDir, err := analyzer.EnsureOutputDir("test-task-uuid")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDir, "test-task-uuid"), taskDir)

	// Verify directory exists
	info, err := os.Stat(taskDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestBaseAnalyzer_CleanupOutputDir(t *testing.T) {
	tempDir := t.TempDir()
	taskDir := filepath.Join(tempDir, "test-task")
	err := os.MkdirAll(taskDir, 0755)
	require.NoError(t, err)

	// Create a file in the directory
	testFile := filepath.Join(taskDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	analyzer := NewBaseAnalyzer(nil)
	err = analyzer.CleanupOutputDir(taskDir)

	require.NoError(t, err)

	// Verify directory is removed
	_, err = os.Stat(taskDir)
	assert.True(t, os.IsNotExist(err))
}

func TestBaseAnalyzer_BuildNamespaceResult(t *testing.T) {
	analyzer := NewBaseAnalyzer(nil)

	parseResult := &model.ParseResult{
		TotalSamples: 150,
		Samples:      []*model.Sample{},
	}

	topFuncsResult := &statistics.TopFuncsResult{
		TopFuncs: []statistics.TopFuncEntry{
			{Name: "func1", SelfSamples: 100, SelfPercent: 66.67},
			{Name: "func2", SelfSamples: 50, SelfPercent: 33.33},
		},
		FuncCallstacks: make(map[string]map[string]int64),
	}

	threadStatsResult := &statistics.ThreadStatsResult{
		Threads: []statistics.ThreadEntry{
			{TID: 1, ThreadName: "main", Samples: 100, Percentage: 66.67},
			{TID: 2, ThreadName: "worker", Samples: 50, Percentage: 33.33},
		},
	}

	suggestions := []model.Suggestion{
		{Suggestion: "test suggestion"},
	}

	result, err := analyzer.BuildNamespaceResult(
		"test-uuid",
		parseResult,
		topFuncsResult,
		threadStatsResult,
		"test-uuid/flamegraph.json.gz",
		"test-uuid/callgraph.json",
		suggestions,
	)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, result.TopFuncs, "func1")
	assert.Contains(t, result.ActiveThreadsJSON, "main")
	assert.Equal(t, "test-uuid/flamegraph.json.gz", result.FlameGraphFile)
	assert.Equal(t, "test-uuid/callgraph.json", result.CallGraphFile)
	assert.Len(t, result.Suggestions, 1)
}

func TestBaseAnalyzer_WriteFlameGraphGzip(t *testing.T) {
	analyzer := NewBaseAnalyzer(nil)

	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"func1"}, Value: 100},
	}

	fg, err := analyzer.GenerateFlameGraph(context.Background(), samples)
	require.NoError(t, err)

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "flamegraph.json.gz")

	err = analyzer.WriteFlameGraphGzip(fg, outputPath)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(outputPath)
	require.NoError(t, err)
}

func TestBaseAnalyzer_WriteCallGraphJSON(t *testing.T) {
	analyzer := NewBaseAnalyzer(nil)

	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"func1", "func2"}, Value: 100},
	}

	cg, err := analyzer.GenerateCallGraph(context.Background(), samples)
	require.NoError(t, err)

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "callgraph.json")

	err = analyzer.WriteCallGraphJSON(cg, outputPath)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(outputPath)
	require.NoError(t, err)
}
