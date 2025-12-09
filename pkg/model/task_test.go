package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskType_String(t *testing.T) {
	tests := []struct {
		taskType TaskType
		expected string
	}{
		{TaskTypeGeneric, "generic"},
		{TaskTypeJava, "java"},
		{TaskTypeTracing, "tracing"},
		{TaskTypeTiming, "timing"},
		{TaskTypeMemLeak, "memleak"},
		{TaskTypePProfMem, "pprof_mem"},
		{TaskTypeJavaHeap, "java_heap"},
		{TaskTypePhysMem, "phys_mem"},
		{TaskTypeJeprof, "jeprof"},
		{TaskTypeBolt, "bolt"},
		{TaskType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.taskType.String())
		})
	}
}

func TestProfilerType_String(t *testing.T) {
	tests := []struct {
		profilerType ProfilerType
		expected     string
	}{
		{ProfilerTypePerf, "perf"},
		{ProfilerTypeAsyncAlloc, "async_alloc"},
		{ProfilerTypePProf, "pprof"},
		{ProfilerType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.profilerType.String())
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

func TestTask_GetResourceType(t *testing.T) {
	tests := []struct {
		taskType TaskType
		expected string
	}{
		{TaskTypeGeneric, "CPU"},
		{TaskTypeTiming, "CPU"},
		{TaskTypeJava, "App"},
		{TaskTypeTracing, "Disk"},
		{TaskTypeMemLeak, "Memory"},
		{TaskTypePProfMem, "Memory"},
		{TaskTypeJavaHeap, "Memory"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			task := &Task{Type: tt.taskType}
			assert.Equal(t, tt.expected, task.GetResourceType())
		})
	}
}

func TestNewTask(t *testing.T) {
	task := NewTask(123, "uuid-456", TaskTypeJava, ProfilerTypePerf)

	assert.Equal(t, int64(123), task.ID)
	assert.Equal(t, "uuid-456", task.TaskUUID)
	assert.Equal(t, TaskTypeJava, task.Type)
	assert.Equal(t, ProfilerTypePerf, task.ProfilerType)
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
