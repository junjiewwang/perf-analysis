package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/pkg/model"
)

func TestNewBaseAnalyzer(t *testing.T) {
	// Test with nil config
	analyzer := NewBaseAnalyzer(nil)
	assert.NotNil(t, analyzer)
	assert.NotNil(t, analyzer.config)
	assert.NotNil(t, analyzer.BaseAnalyzer) // embedded perflib engine

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

// Note: BuildNamespaceResult was removed as dead code in the perflib migration.
// The method was never called by scheduler or CLI code.

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
