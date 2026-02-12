package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProfiler_String(t *testing.T) {
	tests := []struct {
		profiler Profiler
		expected string
	}{
		{ProfilerPerf, "perf"},
		{ProfilerAsyncProfiler, "async-profiler"},
		{ProfilerPProf, "pprof"},
		{ProfilerHeapDump, "heapdump"},
		{ProfilerJeprof, "jeprof"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.profiler.String())
		})
	}
}

func TestProfiler_IsValid(t *testing.T) {
	tests := []struct {
		profiler Profiler
		valid    bool
	}{
		{ProfilerPerf, true},
		{ProfilerAsyncProfiler, true},
		{ProfilerPProf, true},
		{ProfilerHeapDump, true},
		{ProfilerJeprof, true},
		{Profiler("unknown"), false},
		{Profiler(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.profiler), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.profiler.IsValid())
		})
	}
}

func TestEventType_String(t *testing.T) {
	tests := []struct {
		event    EventType
		expected string
	}{
		{EventCPU, "cpu"},
		{EventAlloc, "alloc"},
		{EventHeap, "heap"},
		{EventWall, "wall"},
		{EventLock, "lock"},
		{EventGoroutine, "goroutine"},
		{EventBlock, "block"},
		{EventMutex, "mutex"},
		{EventIO, "io"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.event.String())
		})
	}
}

func TestEventType_IsValid(t *testing.T) {
	tests := []struct {
		event EventType
		valid bool
	}{
		{EventCPU, true},
		{EventAlloc, true},
		{EventHeap, true},
		{EventWall, true},
		{EventLock, true},
		{EventGoroutine, true},
		{EventBlock, true},
		{EventMutex, true},
		{EventIO, true},
		{EventType("unknown"), false},
		{EventType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.event.IsValid())
		})
	}
}

func TestAnalysisMode(t *testing.T) {
	tests := []struct {
		profiler Profiler
		event    EventType
		expected string
	}{
		{ProfilerPerf, EventCPU, "perf-cpu"},
		{ProfilerAsyncProfiler, EventCPU, "async-profiler-cpu"},
		{ProfilerAsyncProfiler, EventAlloc, "async-profiler-alloc"},
		{ProfilerAsyncProfiler, EventWall, "async-profiler-wall"},
		{ProfilerAsyncProfiler, EventLock, "async-profiler-lock"},
		{ProfilerPProf, EventCPU, "pprof-cpu"},
		{ProfilerPProf, EventHeap, "pprof-heap"},
		{ProfilerPProf, EventGoroutine, "pprof-goroutine"},
		{ProfilerPProf, EventBlock, "pprof-block"},
		{ProfilerPProf, EventMutex, "pprof-mutex"},
		{ProfilerHeapDump, EventHeap, "heapdump-heap"},
		{ProfilerJeprof, EventHeap, "jeprof-heap"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, AnalysisMode(tt.profiler, tt.event))
		})
	}
}

func TestTask_IsHighPriority(t *testing.T) {
	tests := []struct {
		name     string
		task     *Task
		expected bool
	}{
		{
			name: "short duration task",
			task: &Task{
				RequestParams: RequestParams{
					Duration: 60,
				},
			},
			expected: true,
		},
		{
			name: "long duration task",
			task: &Task{
				RequestParams: RequestParams{
					Duration: 300,
				},
			},
			expected: false,
		},
		{
			name: "short perf duration task",
			task: &Task{
				RequestParams: RequestParams{
					PerfDuration: 120,
				},
			},
			expected: true,
		},
		{
			name: "no duration specified",
			task: &Task{
				RequestParams: RequestParams{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.task.IsHighPriority())
		})
	}
}

func TestTask_IsMasterTask(t *testing.T) {
	tests := []struct {
		name     string
		task     *Task
		expected bool
	}{
		{
			name:     "without master task",
			task:     &Task{MasterTaskTID: nil},
			expected: false,
		},
		{
			name:     "with empty master task",
			task:     &Task{MasterTaskTID: stringPtr("")},
			expected: false,
		},
		{
			name:     "with master task",
			task:     &Task{MasterTaskTID: stringPtr("master-123")},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.task.IsMasterTask())
		})
	}
}

func TestNewTask(t *testing.T) {
	task := NewTask(123, "uuid-456", ProfilerAsyncProfiler, EventCPU)

	assert.Equal(t, int64(123), task.ID)
	assert.Equal(t, "uuid-456", task.TaskUUID)
	assert.Equal(t, ProfilerAsyncProfiler, task.Profiler)
	assert.Equal(t, EventCPU, task.Event)
	assert.Equal(t, "async-profiler-cpu", task.Mode)
	assert.Equal(t, TaskStatusPending, task.Status)
	assert.Equal(t, AnalysisStatusPending, task.AnalysisStatus)
	assert.False(t, task.CreateTime.IsZero())
}

func TestRequestParams_UnmarshalJSON(t *testing.T) {
	jsonStr := `{"duration": 60, "container_type": 1, "container_name": "nginx"}`

	var params RequestParams
	err := json.Unmarshal([]byte(jsonStr), &params)

	assert.NoError(t, err)
	assert.Equal(t, 60, params.Duration)
	assert.Equal(t, 1, params.ContainerType)
	assert.Equal(t, "nginx", params.ContainerName)
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
