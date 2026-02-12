package publisher

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/internal/storage"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

func testLogger() utils.Logger {
	return utils.NewDefaultLogger(utils.LevelError, io.Discard)
}

func TestDefaultResultPublisher_Publish_CPUProfiling(t *testing.T) {
	t.Parallel()

	taskDir := t.TempDir()
	taskUUID := "test-cpu-uuid"

	// Create a fake flame graph file that OutputFiles references
	fgPath := filepath.Join(taskDir, "collapsed_data.json.gz")
	require.NoError(t, os.WriteFile(fgPath, []byte("fake-fg"), 0644))

	localStorage, err := storage.NewLocalStorage(filepath.Dir(taskDir))
	require.NoError(t, err)

	pub := NewDefaultResultPublisher(localStorage, testLogger())

	resp := &model.AnalysisResponse{
		TaskUUID:     taskUUID,
		Mode:         "async-profiler-cpu",
		TotalRecords: 1000,
		OutputFiles: []model.OutputFile{
			{Name: "Flame Graph", LocalPath: fgPath, COSKey: taskUUID + "/collapsed_data.json.gz"},
		},
		Data: &model.CPUProfilingData{
			TotalSamples: 1000,
			TopFuncs:     model.TopFuncsMap{"main": {Self: 50.0}},
			ThreadStats:  []model.ThreadInfo{{ThreadName: "main", Samples: 500, Percentage: 50.0}},
		},
	}

	output, err := pub.Publish(context.Background(), &PublishRequest{
		TaskUUID:        taskUUID,
		Mode:            "async-profiler-cpu",
		TaskDir:         taskDir,
		Response:        resp,
		AnalysisVersion: "1.0.0",
	})

	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify output files were uploaded
	assert.Contains(t, output.UploadedFiles, "Flame Graph")

	// Verify summary.json was created
	summaryPath := filepath.Join(taskDir, "summary.json")
	assert.FileExists(t, summaryPath)

	// Verify summary content
	require.NotNil(t, output.Summary)
	assert.Equal(t, taskUUID, output.Summary["task_uuid"])
	assert.Equal(t, 1000, output.Summary["total_records"])

	// Verify metadata
	metadata, ok := output.Summary["metadata"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "async-profiler-cpu", metadata["mode"])
	assert.Equal(t, "1.0.0", metadata["analysis_version"])
	assert.Contains(t, metadata, "created_at")
}

func TestDefaultResultPublisher_Publish_HeapDump_WithRetainers(t *testing.T) {
	t.Parallel()

	taskDir := t.TempDir()
	taskUUID := "test-heap-uuid"

	// Create fake heap report file
	heapPath := filepath.Join(taskDir, "heap_analysis.json")
	require.NoError(t, os.WriteFile(heapPath, []byte("{}"), 0644))

	localStorage, err := storage.NewLocalStorage(filepath.Dir(taskDir))
	require.NoError(t, err)

	pub := NewDefaultResultPublisher(localStorage, testLogger())

	resp := &model.AnalysisResponse{
		TaskUUID:     taskUUID,
		Mode:         "heapdump-heap",
		TotalRecords: 500,
		OutputFiles: []model.OutputFile{
			{Name: "Heap Report", LocalPath: heapPath, COSKey: taskUUID + "/heap_analysis.json"},
		},
		Data: &model.HeapAnalysisData{
			TotalClasses:   100,
			TotalInstances: 5000,
			TotalHeapSize:  1024 * 1024,
			HeapSizeHuman:  "1 MB",
			TopClasses: []model.HeapClassStats{
				{ClassName: "java.lang.String", InstanceCount: 1000, TotalSize: 512000, Percentage: 50.0},
			},
		},
	}

	output, err := pub.Publish(context.Background(), &PublishRequest{
		TaskUUID: taskUUID,
		Mode:     "heapdump-heap",
		TaskDir:  taskDir,
		Response: resp,
	})

	require.NoError(t, err)

	// Verify retainer_analysis.json was created
	retainerPath := filepath.Join(taskDir, "retainer_analysis.json")
	assert.FileExists(t, retainerPath)
	assert.Contains(t, output.UploadedFiles, "Retainer Analysis")

	// Verify summary.json was also created
	summaryPath := filepath.Join(taskDir, "summary.json")
	assert.FileExists(t, summaryPath)
}

func TestDefaultResultPublisher_Publish_WithExtraMetadata(t *testing.T) {
	t.Parallel()

	taskDir := t.TempDir()
	taskUUID := "test-meta-uuid"

	localStorage, err := storage.NewLocalStorage(filepath.Dir(taskDir))
	require.NoError(t, err)

	pub := NewDefaultResultPublisher(localStorage, testLogger())

	resp := &model.AnalysisResponse{
		TaskUUID:     taskUUID,
		Mode:         "java-cpu",
		TotalRecords: 100,
		Data: &model.CPUProfilingData{
			TotalSamples: 100,
		},
	}

	output, err := pub.Publish(context.Background(), &PublishRequest{
		TaskUUID:        taskUUID,
		Mode:            "java-cpu",
		TaskDir:         taskDir,
		Response:        resp,
		ModeDescription: "Java CPU hotspot analysis",
		Profile:         "standard",
		InputFile:       "test.data",
		AnalysisTimeMs:  1234,
	})

	require.NoError(t, err)
	require.NotNil(t, output.Summary)

	metadata := output.Summary["metadata"].(map[string]interface{})
	assert.Equal(t, "standard", metadata["profile"])
	assert.Equal(t, "test.data", metadata["input_file"])
	assert.Equal(t, int64(1234), metadata["analysis_time_ms"])
	assert.Equal(t, "Java CPU hotspot analysis", metadata["mode_description"])
}

func TestDefaultResultPublisher_Publish_NilData(t *testing.T) {
	t.Parallel()

	taskDir := t.TempDir()
	taskUUID := "test-nil-uuid"

	localStorage, err := storage.NewLocalStorage(filepath.Dir(taskDir))
	require.NoError(t, err)

	pub := NewDefaultResultPublisher(localStorage, testLogger())

	resp := &model.AnalysisResponse{
		TaskUUID:     taskUUID,
		Mode:         "unknown-mode",
		TotalRecords: 0,
		Data:         nil,
	}

	output, err := pub.Publish(context.Background(), &PublishRequest{
		TaskUUID: taskUUID,
		Mode:     "unknown-mode",
		TaskDir:  taskDir,
		Response: resp,
	})

	require.NoError(t, err)
	// Summary should still be generated (fallback formatter handles nil data)
	assert.NotNil(t, output.Summary)
	// No retainer_analysis.json since data is nil
	retainerPath := filepath.Join(taskDir, "retainer_analysis.json")
	assert.NoFileExists(t, retainerPath)
}

func TestDefaultResultPublisher_Publish_NonexistentOutputFile(t *testing.T) {
	t.Parallel()

	taskDir := t.TempDir()
	taskUUID := "test-nofile-uuid"

	localStorage, err := storage.NewLocalStorage(filepath.Dir(taskDir))
	require.NoError(t, err)

	pub := NewDefaultResultPublisher(localStorage, testLogger())

	resp := &model.AnalysisResponse{
		TaskUUID: taskUUID,
		Mode:     "java-cpu",
		OutputFiles: []model.OutputFile{
			{Name: "Flame Graph", LocalPath: "/nonexistent/path.json.gz"},
			{Name: "Empty Path", LocalPath: ""},
		},
		Data: &model.CPUProfilingData{TotalSamples: 10},
	}

	output, err := pub.Publish(context.Background(), &PublishRequest{
		TaskUUID: taskUUID,
		Mode:     "java-cpu",
		TaskDir:  taskDir,
		Response: resp,
	})

	require.NoError(t, err)
	assert.Empty(t, output.UploadedFiles["Flame Graph"])
}

func TestExtractDBFields_CPUProfiling(t *testing.T) {
	t.Parallel()

	data := &model.CPUProfilingData{
		TopFuncs:    model.TopFuncsMap{"main": {Self: 50.0}},
		ThreadStats: []model.ThreadInfo{{ThreadName: "main", Samples: 500}},
	}
	uploaded := map[string]string{
		"Flame Graph": "uuid/fg.json.gz",
		"Call Graph":  "uuid/cg.json.gz",
	}

	fields := ExtractDBFields(data, uploaded)

	assert.Equal(t, "uuid/fg.json.gz", fields.FlameGraphKey)
	assert.Equal(t, "uuid/cg.json.gz", fields.CallGraphKey)
	assert.NotEmpty(t, fields.TopFuncs)
	assert.NotEmpty(t, fields.ActiveThreadsJSON)

	// Verify JSON is valid
	var topFuncs map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(fields.TopFuncs), &topFuncs))
	assert.Contains(t, topFuncs, "main")
}

func TestExtractDBFields_Allocation(t *testing.T) {
	t.Parallel()

	data := &model.AllocationData{
		TopAllocators: model.TopFuncsMap{"alloc": {Self: 30.0}},
		ThreadStats:   []model.ThreadInfo{{ThreadName: "worker", Samples: 200}},
	}
	uploaded := map[string]string{
		"Allocation Flame Graph": "uuid/alloc_fg.json.gz",
		"Allocation Call Graph":  "uuid/alloc_cg.json.gz",
	}

	fields := ExtractDBFields(data, uploaded)

	assert.Equal(t, "uuid/alloc_fg.json.gz", fields.FlameGraphKey)
	assert.Equal(t, "uuid/alloc_cg.json.gz", fields.CallGraphKey)
	assert.NotEmpty(t, fields.TopFuncs)
}

func TestExtractDBFields_Heap(t *testing.T) {
	t.Parallel()

	data := &model.HeapAnalysisData{
		TotalClasses:   10,
		TotalInstances: 100,
		TotalHeapSize:  1024,
		HeapSizeHuman:  "1 KB",
		TopClasses: []model.HeapClassStats{
			{ClassName: "String", InstanceCount: 50, TotalSize: 512},
		},
	}
	uploaded := map[string]string{
		"Heap Report":    "uuid/heap.json",
		"Class Histogram": "uuid/hist.json",
	}

	fields := ExtractDBFields(data, uploaded)

	assert.Equal(t, "uuid/heap.json", fields.FlameGraphKey)
	assert.Equal(t, "uuid/hist.json", fields.CallGraphKey)
	assert.NotEmpty(t, fields.TopFuncs)
	assert.NotEmpty(t, fields.ActiveThreadsJSON)
}

func TestExtractDBFields_Tracing(t *testing.T) {
	t.Parallel()

	data := &model.TracingData{
		TopFuncs:    model.TopFuncsMap{"trace_fn": {Self: 10.0}},
		ThreadStats: []model.ThreadInfo{{ThreadName: "pool-1", Samples: 100}},
	}
	uploaded := map[string]string{
		"Flame Graph": "uuid/trace_fg.json.gz",
		"Call Graph":  "uuid/trace_cg.json.gz",
	}

	fields := ExtractDBFields(data, uploaded)

	assert.Equal(t, "uuid/trace_fg.json.gz", fields.FlameGraphKey)
	assert.Equal(t, "uuid/trace_cg.json.gz", fields.CallGraphKey)
}

func TestExtractDBFields_NilData(t *testing.T) {
	t.Parallel()

	fields := ExtractDBFields(nil, map[string]string{})

	assert.Empty(t, fields.TopFuncs)
	assert.Empty(t, fields.ActiveThreadsJSON)
	assert.Empty(t, fields.FlameGraphKey)
	assert.Empty(t, fields.CallGraphKey)
}
