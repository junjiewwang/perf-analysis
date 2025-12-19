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

func TestNewJavaCPUAnalyzer(t *testing.T) {
	analyzer := NewJavaCPUAnalyzer(nil)

	assert.NotNil(t, analyzer)
	assert.NotNil(t, analyzer.BaseAnalyzer)
	assert.Equal(t, "java_cpu_analyzer", analyzer.Name())
}

func TestJavaCPUAnalyzer_SupportedTypes(t *testing.T) {
	analyzer := NewJavaCPUAnalyzer(nil)

	types := analyzer.SupportedTypes()

	assert.Len(t, types, 1)
	assert.Contains(t, types, model.TaskTypeJava)
}

func TestJavaCPUAnalyzer_Analyze_Success(t *testing.T) {
	tempDir := t.TempDir()
	config := &BaseAnalyzerConfig{
		OutputDir: tempDir,
		TopFuncsN: 10,
	}

	analyzer := NewJavaCPUAnalyzer(config)

	input := `main-thread;java.lang.Thread.run;com.example.App.main 100
worker-1;java.lang.Thread.run;com.example.Worker.process 50
main-thread;java.lang.Thread.run;com.example.App.init 30`

	req := &model.AnalysisRequest{
		TaskID:       1,
		TaskUUID:     "test-java-cpu-uuid",
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypePerf,
		OutputDir:    filepath.Join(tempDir, "test-java-cpu-uuid"),
	}

	// Ensure output directory exists
	os.MkdirAll(req.OutputDir, 0755)

	result, err := analyzer.AnalyzeFromReader(context.Background(), req, strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "test-java-cpu-uuid", result.TaskUUID)
	assert.Equal(t, 180, result.TotalRecords)

	// Verify Data is CPUProfilingData
	cpuData, ok := result.Data.(*model.CPUProfilingData)
	require.True(t, ok, "Data should be CPUProfilingData")
	assert.NotEmpty(t, cpuData.TopFuncs)
	assert.Contains(t, cpuData.FlameGraphFile, "collapsed_data.json.gz")
	assert.Contains(t, cpuData.CallGraphFile, "callgraph_data.json.gz")
}

func TestJavaCPUAnalyzer_Analyze_EmptyData(t *testing.T) {
	tempDir := t.TempDir()
	config := &BaseAnalyzerConfig{
		OutputDir: tempDir,
	}

	analyzer := NewJavaCPUAnalyzer(config)

	req := &model.AnalysisRequest{
		TaskID:       1,
		TaskUUID:     "test-empty-uuid",
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypePerf,
	}

	_, err := analyzer.AnalyzeFromReader(context.Background(), req, strings.NewReader(""))

	assert.Error(t, err)
	assert.Equal(t, ErrEmptyData, err)
}

func TestJavaCPUAnalyzer_Analyze_WrongProfilerType(t *testing.T) {
	analyzer := NewJavaCPUAnalyzer(nil)

	req := &model.AnalysisRequest{
		TaskID:       1,
		TaskUUID:     "test-uuid",
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypeAsyncAlloc, // Wrong type
	}

	_, err := analyzer.AnalyzeFromReader(context.Background(), req, strings.NewReader("data"))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only supports profiler type perf")
}

func TestJavaCPUAnalyzer_Analyze_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	config := &BaseAnalyzerConfig{
		OutputDir: tempDir,
	}

	analyzer := NewJavaCPUAnalyzer(config)

	// Large input to ensure context cancellation can happen
	var sb strings.Builder
	for i := 0; i < 10000; i++ {
		sb.WriteString("thread;func1;func2;func3 100\n")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := &model.AnalysisRequest{
		TaskID:       1,
		TaskUUID:     "test-cancel-uuid",
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypePerf,
	}

	_, err := analyzer.AnalyzeFromReader(ctx, req, strings.NewReader(sb.String()))

	assert.Error(t, err)
}

func TestJavaCPUAnalyzer_GetOutputFiles(t *testing.T) {
	analyzer := NewJavaCPUAnalyzer(nil)

	files := analyzer.GetOutputFiles("test-uuid", "/tmp/test-uuid")

	assert.Len(t, files, 2)

	// Check flame graph file
	assert.Equal(t, "/tmp/test-uuid/collapsed_data.json.gz", files[0].LocalPath)
	assert.Equal(t, "test-uuid/collapsed_data.json.gz", files[0].COSKey)

	// Check call graph file
	assert.Equal(t, "/tmp/test-uuid/callgraph_data.json.gz", files[1].LocalPath)
	assert.Equal(t, "test-uuid/callgraph_data.json.gz", files[1].COSKey)
}

func TestJavaCPUAnalyzer_OutputFilesCreated(t *testing.T) {
	tempDir := t.TempDir()
	config := &BaseAnalyzerConfig{
		OutputDir: tempDir,
	}

	analyzer := NewJavaCPUAnalyzer(config)

	input := `main-thread;java.lang.Thread.run;com.example.App.main 100`

	taskDir := filepath.Join(tempDir, "test-output-uuid")
	os.MkdirAll(taskDir, 0755)

	req := &model.AnalysisRequest{
		TaskID:       1,
		TaskUUID:     "test-output-uuid",
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypePerf,
		OutputDir:    taskDir,
	}

	_, err := analyzer.AnalyzeFromReader(context.Background(), req, strings.NewReader(input))
	require.NoError(t, err)

	// Check output files exist
	fgPath := filepath.Join(taskDir, "collapsed_data.json.gz")
	_, err = os.Stat(fgPath)
	assert.NoError(t, err)

	cgPath := filepath.Join(taskDir, "callgraph_data.json.gz")
	_, err = os.Stat(cgPath)
	assert.NoError(t, err)
}

// Benchmark test
func BenchmarkJavaCPUAnalyzer_Analyze(b *testing.B) {
	tempDir := b.TempDir()
	config := &BaseAnalyzerConfig{
		OutputDir: tempDir,
		TopFuncsN: 50,
	}

	analyzer := NewJavaCPUAnalyzer(config)

	// Generate test data
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("thread;func1;func2;func3;func4;func5 100\n")
	}
	input := sb.String()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		taskDir := filepath.Join(tempDir, "bench-uuid-"+string(rune('0'+i%10)))
		os.MkdirAll(taskDir, 0755)
		req := &model.AnalysisRequest{
			TaskID:       int64(i),
			TaskUUID:     "bench-uuid-" + string(rune('0'+i%10)),
			TaskType:     model.TaskTypeJava,
			ProfilerType: model.ProfilerTypePerf,
			OutputDir:    taskDir,
		}

		_, _ = analyzer.AnalyzeFromReader(context.Background(), req, strings.NewReader(input))
	}
}
