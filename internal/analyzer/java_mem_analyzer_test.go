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

func TestNewJavaMemAnalyzer(t *testing.T) {
	analyzer := NewJavaMemAnalyzer(nil)

	assert.NotNil(t, analyzer)
	assert.NotNil(t, analyzer.BaseAnalyzer)
	assert.Equal(t, "java_mem_analyzer", analyzer.Name())
}

func TestJavaMemAnalyzer_Analyze_Success(t *testing.T) {
	tempDir := t.TempDir()
	config := &BaseAnalyzerConfig{
		OutputDir: tempDir,
		TopFuncsN: 10,
	}

	analyzer := NewJavaMemAnalyzer(config)

	input := `main-thread;java.lang.Object.<init>;com.example.App.createObjects 10000
worker-1;java.util.ArrayList.<init>;com.example.Worker.allocate 5000
main-thread;java.lang.String.valueOf;com.example.App.stringify 3000`

	taskDir := filepath.Join(tempDir, "test-java-mem-uuid")
	os.MkdirAll(taskDir, 0755)

	req := &model.AnalysisRequest{
		TaskID:   1,
		TaskUUID: "test-java-mem-uuid",
		Mode:     string(ModeJavaAlloc),
		OutputDir: taskDir,
	}

	result, err := analyzer.AnalyzeFromReader(context.Background(), req, strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "test-java-mem-uuid", result.TaskUUID)
	assert.Equal(t, 18000, result.TotalRecords)

	// Verify Data is AllocationData
	allocData, ok := result.Data.(*model.AllocationData)
	require.True(t, ok, "Data should be AllocationData")
	assert.Contains(t, allocData.FlameGraphFile, "alloc_data.json.gz")
	assert.Contains(t, allocData.CallGraphFile, "alloc_callgraph_data.json.gz")
}

func TestJavaMemAnalyzer_Analyze_EmptyData(t *testing.T) {
	tempDir := t.TempDir()
	config := &BaseAnalyzerConfig{
		OutputDir: tempDir,
	}

	analyzer := NewJavaMemAnalyzer(config)

	req := &model.AnalysisRequest{
		TaskID:   1,
		TaskUUID: "test-empty-uuid",
		Mode:     string(ModeJavaAlloc),
	}

	_, err := analyzer.AnalyzeFromReader(context.Background(), req, strings.NewReader(""))

	assert.Error(t, err)
	assert.Equal(t, ErrEmptyData, err)
}

func TestJavaMemAnalyzer_GenerateMemorySuggestions(t *testing.T) {
	analyzer := NewJavaMemAnalyzer(nil)

	topAllocators := model.TopFuncsMap{
		"com.example.App.allocate":   model.TopFuncValue{Self: 15.0},
		"com.example.Worker.process": model.TopFuncValue{Self: 5.0},
	}

	suggestions := analyzer.generateMemorySuggestions(topAllocators)

	// Should have suggestion for function > 10%
	require.Len(t, suggestions, 1)
	assert.Contains(t, suggestions[0].Suggestion, "com.example.App.allocate")
}

func TestJavaMemAnalyzer_GetOutputFiles(t *testing.T) {
	analyzer := NewJavaMemAnalyzer(nil)

	files := analyzer.GetOutputFiles("test-uuid", "/tmp/test-uuid")

	assert.Len(t, files, 2)

	// Check flame graph file
	assert.Equal(t, "/tmp/test-uuid/alloc_data.json.gz", files[0].LocalPath)
	assert.Equal(t, "test-uuid/alloc_data.json.gz", files[0].COSKey)

	// Check call graph file (now gzipped)
	assert.Equal(t, "/tmp/test-uuid/alloc_callgraph_data.json.gz", files[1].LocalPath)
	assert.Equal(t, "test-uuid/alloc_callgraph_data.json.gz", files[1].COSKey)
}
