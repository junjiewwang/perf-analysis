// Package model defines the core data structures used throughout the application.
package model

import (
	"encoding/json"
	"time"
)

// TaskType represents the type of analysis task.
type TaskType int

const (
	TaskTypeGeneric        TaskType = 0  // Generic CPU profiling (perf)
	TaskTypeJava           TaskType = 1  // Java async-profiler
	TaskTypeTracing        TaskType = 2  // IO tracing
	TaskTypeTiming         TaskType = 3  // Timing analysis
	TaskTypeMemLeak        TaskType = 4  // Memory leak analysis
	TaskTypePProfMem       TaskType = 5  // Go pprof memory
	TaskTypeJavaHeap       TaskType = 6  // Java heap dump
	TaskTypePhysMem        TaskType = 7  // Physical memory
	TaskTypeJeprof         TaskType = 8  // Jeprof
	TaskTypeBolt           TaskType = 9  // Bolt optimization
	TaskTypePProfCPU       TaskType = 10 // Go pprof CPU
	TaskTypePProfHeap      TaskType = 11 // Go pprof Heap
	TaskTypePProfGoroutine TaskType = 12 // Go pprof Goroutine
	TaskTypePProfBlock     TaskType = 13 // Go pprof Block
	TaskTypePProfMutex     TaskType = 14 // Go pprof Mutex
)

// String returns the string representation of TaskType.
func (t TaskType) String() string {
	switch t {
	case TaskTypeGeneric:
		return "generic"
	case TaskTypeJava:
		return "java"
	case TaskTypeTracing:
		return "tracing"
	case TaskTypeTiming:
		return "timing"
	case TaskTypeMemLeak:
		return "memleak"
	case TaskTypePProfMem:
		return "pprof_mem"
	case TaskTypeJavaHeap:
		return "java_heap"
	case TaskTypePhysMem:
		return "phys_mem"
	case TaskTypeJeprof:
		return "jeprof"
	case TaskTypeBolt:
		return "bolt"
	case TaskTypePProfCPU:
		return "pprof_cpu"
	case TaskTypePProfHeap:
		return "pprof_heap"
	case TaskTypePProfGoroutine:
		return "pprof_goroutine"
	case TaskTypePProfBlock:
		return "pprof_block"
	case TaskTypePProfMutex:
		return "pprof_mutex"
	default:
		return "unknown"
	}
}

// ProfilerType represents the profiler type.
type ProfilerType int

const (
	ProfilerTypePerf       ProfilerType = 0 // perf / async-profiler CPU
	ProfilerTypeAsyncAlloc ProfilerType = 1 // async-profiler allocation
	ProfilerTypePProf      ProfilerType = 2 // Go pprof
)

// String returns the string representation of ProfilerType.
func (p ProfilerType) String() string {
	switch p {
	case ProfilerTypePerf:
		return "perf"
	case ProfilerTypeAsyncAlloc:
		return "async_alloc"
	case ProfilerTypePProf:
		return "pprof"
	default:
		return "unknown"
	}
}

// TaskStatus represents the status of a task.
type TaskStatus int

const (
	TaskStatusPending   TaskStatus = 0 // Pending
	TaskStatusRunning   TaskStatus = 1 // Running (data collection)
	TaskStatusCompleted TaskStatus = 2 // Completed (data collection done)
	TaskStatusFailed    TaskStatus = 3 // Failed
)

// AnalysisStatus represents the analysis status.
type AnalysisStatus int

const (
	AnalysisStatusPending   AnalysisStatus = 0 // Not started
	AnalysisStatusRunning   AnalysisStatus = 1 // Running
	AnalysisStatusCompleted AnalysisStatus = 2 // Completed
	AnalysisStatusFailed    AnalysisStatus = 3 // Failed
	AnalysisStatusEmpty     AnalysisStatus = 5 // Empty data
)

// Task represents a profiling task.
type Task struct {
	ID             int64          `json:"id" db:"id"`
	TaskUUID       string         `json:"tid" db:"tid"`
	Type           TaskType       `json:"type" db:"type"`
	ProfilerType   ProfilerType   `json:"profiler_type" db:"profiler_type"`
	Status         TaskStatus     `json:"status" db:"status"`
	AnalysisStatus AnalysisStatus `json:"analysis_status" db:"analysis_status"`
	StatusInfo     string         `json:"status_info" db:"status_info"`
	ResultFile     string         `json:"result_file" db:"result_file"`
	UserName       string         `json:"user_name" db:"user_name"`
	MasterTaskTID  *string        `json:"mastertask_tid" db:"mastertask_tid"`
	COSBucket      string         `json:"cos_bucket" db:"cos_bucket"`
	RequestParams  RequestParams  `json:"request_params" db:"request_params"`
	CreateTime     time.Time      `json:"create_time" db:"create_time"`
	BeginTime      *time.Time     `json:"begin_time" db:"begin_time"`
	EndTime        *time.Time     `json:"end_time" db:"end_time"`
}

// RequestParams holds task request parameters.
type RequestParams struct {
	Duration       int    `json:"duration,omitempty"`
	PerfDuration   int    `json:"perf_duration,omitempty"`
	ContainerType  int    `json:"container_type,omitempty"`
	ContainerName  string `json:"container_name,omitempty"`
	AnnotateEnable bool   `json:"annotate_enable,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler for RequestParams.
func (rp *RequestParams) UnmarshalJSON(data []byte) error {
	type Alias RequestParams
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(rp),
	}
	return json.Unmarshal(data, aux)
}

// IsHighPriority returns true if the task should be treated as high priority.
func (t *Task) IsHighPriority() bool {
	if t.RequestParams.Duration > 0 && t.RequestParams.Duration <= 120 {
		return true
	}
	if t.RequestParams.PerfDuration > 0 && t.RequestParams.PerfDuration <= 120 {
		return true
	}
	return false
}

// IsMasterTask returns true if the task has a master task.
func (t *Task) IsMasterTask() bool {
	return t.MasterTaskTID != nil && *t.MasterTaskTID != ""
}

// GetResourceType returns the resource type string for the task.
func (t *Task) GetResourceType() string {
	switch t.Type {
	case TaskTypeGeneric, TaskTypeTiming, TaskTypePProfCPU:
		return "CPU"
	case TaskTypeJava:
		return "App"
	case TaskTypeTracing:
		return "Disk"
	case TaskTypeMemLeak, TaskTypePProfMem, TaskTypeJavaHeap, TaskTypePhysMem, TaskTypeJeprof, TaskTypePProfHeap:
		return "Memory"
	case TaskTypePProfGoroutine:
		return "Goroutine"
	case TaskTypePProfBlock, TaskTypePProfMutex:
		return "Concurrency"
	default:
		return "Unknown"
	}
}

// NewTask creates a new Task instance.
func NewTask(id int64, taskUUID string, taskType TaskType, profilerType ProfilerType) *Task {
	return &Task{
		ID:             id,
		TaskUUID:       taskUUID,
		Type:           taskType,
		ProfilerType:   profilerType,
		Status:         TaskStatusPending,
		AnalysisStatus: AnalysisStatusPending,
		CreateTime:     time.Now(),
	}
}
