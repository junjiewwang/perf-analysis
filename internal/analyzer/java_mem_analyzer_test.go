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

func TestNewJavaMemAnalyzer(t *testing.T) {
	analyzer := NewJavaMemAnalyzer(nil)

	assert.NotNil(t, analyzer)
	assert.NotNil(t, analyzer.BaseAnalyzer)
	assert.Equal(t, "java_mem_analyzer", analyzer.Name())
}

func TestJavaMemAnalyzer_SupportedTypes(t *testing.T) {
	analyzer := NewJavaMemAnalyzer(nil)

	types := analyzer.SupportedTypes()

	assert.Len(t, types, 1)
	assert.Contains(t, types, model.TaskTypeJava)
}

func TestJavaMemAnalyzer_CanHandle(t *testing.T) {
	analyzer := NewJavaMemAnalyzer(nil)

	// Should handle allocation profiling
	req := &model.AnalysisRequest{
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypeAsyncAlloc,
	}
	assert.True(t, analyzer.CanHandle(req))

	// Should not handle CPU profiling
	req = &model.AnalysisRequest{
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypePerf,
	}
	assert.False(t, analyzer.CanHandle(req))

	// Should not handle other task types
	req = &model.AnalysisRequest{
		TaskType:     model.TaskTypeGeneric,
		ProfilerType: model.ProfilerTypeAsyncAlloc,
	}
	assert.False(t, analyzer.CanHandle(req))
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
		TaskID:       1,
		TaskUUID:     "test-java-mem-uuid",
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypeAsyncAlloc,
		OutputDir:    taskDir,
	}

	result, err := analyzer.AnalyzeFromReader(context.Background(), req, strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "test-java-mem-uuid", result.TaskUUID)
	assert.Equal(t, 18000, result.TotalRecords)
	assert.Contains(t, result.FlameGraphFile, "alloc_data.json.gz")
	assert.Contains(t, result.CallGraphFile, "alloc_data.json")
}

func TestJavaMemAnalyzer_Analyze_EmptyData(t *testing.T) {
	tempDir := t.TempDir()
	config := &BaseAnalyzerConfig{
		OutputDir: tempDir,
	}

	analyzer := NewJavaMemAnalyzer(config)

	req := &model.AnalysisRequest{
		TaskID:       1,
		TaskUUID:     "test-empty-uuid",
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypeAsyncAlloc,
	}

	_, err := analyzer.AnalyzeFromReader(context.Background(), req, strings.NewReader(""))

	assert.Error(t, err)
	assert.Equal(t, ErrEmptyData, err)
}

func TestJavaMemAnalyzer_Analyze_WrongProfilerType(t *testing.T) {
	analyzer := NewJavaMemAnalyzer(nil)

	req := &model.AnalysisRequest{
		TaskID:       1,
		TaskUUID:     "test-uuid",
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypePerf, // Wrong type
	}

	_, err := analyzer.AnalyzeFromReader(context.Background(), req, strings.NewReader("data"))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only supports profiler type async_alloc")
}

func TestJavaMemAnalyzer_GenerateMemorySuggestions(t *testing.T) {
	analyzer := NewJavaMemAnalyzer(nil)

	topFuncsResult := &statistics.TopFuncsResult{
		TopFuncs: []statistics.TopFuncEntry{
			{Name: "com.example.App.allocate", SelfSamples: 1500, SelfPercent: 15.0},
			{Name: "com.example.Worker.process", SelfSamples: 500, SelfPercent: 5.0},
		},
	}

	suggestions := analyzer.generateMemorySuggestions(topFuncsResult)

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

	// Check call graph file
	assert.Equal(t, "/tmp/test-uuid/alloc_data.json", files[1].LocalPath)
	assert.Equal(t, "test-uuid/alloc_data.json", files[1].COSKey)
}
