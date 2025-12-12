package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/internal/parser/hprof"
	"github.com/perf-analysis/pkg/model"
)

func TestNewJavaHeapAnalyzer(t *testing.T) {
	t.Run("with nil config", func(t *testing.T) {
		analyzer := NewJavaHeapAnalyzer(nil)
		assert.NotNil(t, analyzer)
		assert.NotNil(t, analyzer.config)
		assert.NotNil(t, analyzer.hprofOpts)
	})

	t.Run("with custom config", func(t *testing.T) {
		config := &BaseAnalyzerConfig{
			OutputDir: "/tmp/test",
		}
		analyzer := NewJavaHeapAnalyzer(config)
		assert.Equal(t, "/tmp/test", analyzer.config.OutputDir)
	})

	t.Run("with custom hprof options", func(t *testing.T) {
		opts := &hprof.ParserOptions{
			TopClassesN: 50,
		}
		analyzer := NewJavaHeapAnalyzer(nil, WithHprofOptions(opts))
		assert.Equal(t, 50, analyzer.hprofOpts.TopClassesN)
	})
}

func TestJavaHeapAnalyzer_Name(t *testing.T) {
	analyzer := NewJavaHeapAnalyzer(nil)
	assert.Equal(t, "java_heap_analyzer", analyzer.Name())
}

func TestJavaHeapAnalyzer_SupportedTypes(t *testing.T) {
	analyzer := NewJavaHeapAnalyzer(nil)
	types := analyzer.SupportedTypes()

	assert.Len(t, types, 1)
	assert.Contains(t, types, model.TaskTypeJavaHeap)
}

func TestJavaHeapAnalyzer_Analyze_WrongTaskType(t *testing.T) {
	analyzer := NewJavaHeapAnalyzer(nil)
	ctx := context.Background()

	req := &model.AnalysisRequest{
		TaskType: model.TaskTypeJava, // Wrong type
	}

	_, err := analyzer.Analyze(ctx, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "java heap analyzer only supports task type java_heap")
}

func TestJavaHeapAnalyzer_Analyze_FileNotFound(t *testing.T) {
	analyzer := NewJavaHeapAnalyzer(nil)
	ctx := context.Background()

	req := &model.AnalysisRequest{
		TaskType:  model.TaskTypeJavaHeap,
		InputFile: "/nonexistent/file.hprof",
	}

	_, err := analyzer.Analyze(ctx, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open input file")
}

func TestJavaHeapAnalyzer_AnalyzeRealFile(t *testing.T) {
	// Skip if test file doesn't exist
	testFile := "../../test/heapdump2025-12-12-08-5818336174256011702999.hprof"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("Test HPROF file not found, skipping integration test")
	}

	// Create temp output directory
	tempDir, err := os.MkdirTemp("", "heap_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config := &BaseAnalyzerConfig{
		OutputDir: tempDir,
	}
	analyzer := NewJavaHeapAnalyzer(config)
	ctx := context.Background()

	req := &model.AnalysisRequest{
		TaskUUID:  "test-heap-task-123",
		TaskType:  model.TaskTypeJavaHeap,
		InputFile: testFile,
	}

	resp, err := analyzer.Analyze(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify response
	assert.Equal(t, "test-heap-task-123", resp.TaskUUID)
	assert.Greater(t, resp.TotalRecords, 0)

	// Verify Data is HeapAnalysisData
	heapData, ok := resp.Data.(*model.HeapAnalysisData)
	require.True(t, ok, "Data should be HeapAnalysisData")
	assert.NotEmpty(t, heapData.TopClasses)
	assert.Greater(t, heapData.TotalInstances, int64(0))

	// Verify output files exist
	heapReportFile := filepath.Join(tempDir, "test-heap-task-123", "heap_analysis.json")
	histogramFile := filepath.Join(tempDir, "test-heap-task-123", "class_histogram.json")

	assert.FileExists(t, heapReportFile)
	assert.FileExists(t, histogramFile)

	// Log results
	t.Logf("Total records (instances): %d", resp.TotalRecords)
	t.Logf("Heap report file: %s", heapData.HeapReportFile)
	t.Logf("Histogram file: %s", heapData.HistogramFile)
	t.Logf("Suggestions count: %d", len(resp.Suggestions))

	for i, sug := range resp.Suggestions {
		if i >= 5 {
			break
		}
		t.Logf("  Suggestion %d: %s", i+1, sug.Suggestion)
	}
}

func TestJavaHeapAnalyzer_GetOutputFiles(t *testing.T) {
	analyzer := NewJavaHeapAnalyzer(nil)

	files := analyzer.GetOutputFiles("task-123", "/tmp/output")

	assert.Len(t, files, 2)
	assert.Equal(t, "/tmp/output/heap_analysis.json", files[0].LocalPath)
	assert.Equal(t, "task-123/heap_analysis.json", files[0].COSKey)
	assert.Equal(t, "/tmp/output/class_histogram.json", files[1].LocalPath)
	assert.Equal(t, "task-123/class_histogram.json", files[1].COSKey)
}

func TestJavaHeapAnalyzer_isPotentialLeakClass(t *testing.T) {
	analyzer := NewJavaHeapAnalyzer(nil)

	tests := []struct {
		className string
		expected  bool
	}{
		{"java.util.HashMap", true},
		{"java.util.ArrayList", true},
		{"java.util.LinkedList", true},
		{"java.util.HashSet", true},
		{"java.util.concurrent.ConcurrentHashMap", true},
		{"java.util.LinkedHashMap", true},
		{"java.lang.String", false},
		{"java.lang.Object", false},
		{"byte[]", false},
	}

	for _, tt := range tests {
		result := analyzer.isPotentialLeakClass(tt.className)
		assert.Equal(t, tt.expected, result, "className: %s", tt.className)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{100, "100 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1024 * 1024, "1.00 MB"},
		{1024 * 1024 * 1024, "1.00 GB"},
		{1024 * 1024 * 1024 * 2, "2.00 GB"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.bytes)
		assert.Equal(t, tt.expected, result)
	}
}

func TestSortClassesBySize(t *testing.T) {
	classes := []*hprof.ClassStats{
		{ClassName: "A", TotalSize: 100},
		{ClassName: "B", TotalSize: 300},
		{ClassName: "C", TotalSize: 200},
	}

	SortClassesBySize(classes)

	assert.Equal(t, "B", classes[0].ClassName)
	assert.Equal(t, "C", classes[1].ClassName)
	assert.Equal(t, "A", classes[2].ClassName)
}

func TestSortClassesByCount(t *testing.T) {
	classes := []*hprof.ClassStats{
		{ClassName: "A", InstanceCount: 10},
		{ClassName: "B", InstanceCount: 30},
		{ClassName: "C", InstanceCount: 20},
	}

	SortClassesByCount(classes)

	assert.Equal(t, "B", classes[0].ClassName)
	assert.Equal(t, "C", classes[1].ClassName)
	assert.Equal(t, "A", classes[2].ClassName)
}

func TestFactory_CreateJavaHeapAnalyzer(t *testing.T) {
	factory := NewFactory(nil)

	analyzer, err := factory.CreateAnalyzer(model.TaskTypeJavaHeap, model.ProfilerTypePerf)

	require.NoError(t, err)
	require.NotNil(t, analyzer)
	assert.Equal(t, "java_heap_analyzer", analyzer.Name())
}

func TestFactory_CreateManager_IncludesJavaHeapAnalyzer(t *testing.T) {
	factory := NewFactory(nil)
	manager := factory.CreateManager()

	analyzer, ok := manager.GetAnalyzer(model.TaskTypeJavaHeap)

	assert.True(t, ok)
	assert.NotNil(t, analyzer)
	assert.Equal(t, "java_heap_analyzer", analyzer.Name())
}
