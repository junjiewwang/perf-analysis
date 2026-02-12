// Package model defines the core data structures used throughout the application.
package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// Profiler represents the profiling tool used to collect data.
type Profiler string

const (
	// ProfilerPerf is the Linux perf profiler.
	ProfilerPerf Profiler = "perf"

	// ProfilerAsyncProfiler is the async-profiler for Java.
	ProfilerAsyncProfiler Profiler = "async-profiler"

	// ProfilerPProf is the Go pprof profiler.
	ProfilerPProf Profiler = "pprof"

	// ProfilerHeapDump is the Java heap dump tool.
	ProfilerHeapDump Profiler = "heapdump"

	// ProfilerJeprof is the jemalloc profiler.
	ProfilerJeprof Profiler = "jeprof"
)

// knownProfilers is the set of valid profiler values.
var knownProfilers = map[Profiler]bool{
	ProfilerPerf:          true,
	ProfilerAsyncProfiler: true,
	ProfilerPProf:         true,
	ProfilerHeapDump:      true,
	ProfilerJeprof:        true,
}

// IsValid returns true if the profiler is a known value.
func (p Profiler) IsValid() bool {
	return knownProfilers[p]
}

// String returns the string representation of the profiler.
func (p Profiler) String() string {
	return string(p)
}

// EventType represents the type of profiling event.
type EventType string

const (
	// EventCPU is CPU sampling event.
	EventCPU EventType = "cpu"

	// EventAlloc is memory allocation event.
	EventAlloc EventType = "alloc"

	// EventHeap is heap snapshot event.
	EventHeap EventType = "heap"

	// EventWall is wall-clock time event.
	EventWall EventType = "wall"

	// EventLock is lock contention event.
	EventLock EventType = "lock"

	// EventGoroutine is Go goroutine profiling event.
	EventGoroutine EventType = "goroutine"

	// EventBlock is Go block profiling event.
	EventBlock EventType = "block"

	// EventMutex is Go mutex profiling event.
	EventMutex EventType = "mutex"

	// EventIO is IO tracing event.
	EventIO EventType = "io"
)

// knownEventTypes is the set of valid event type values.
var knownEventTypes = map[EventType]bool{
	EventCPU:       true,
	EventAlloc:     true,
	EventHeap:      true,
	EventWall:      true,
	EventLock:      true,
	EventGoroutine: true,
	EventBlock:     true,
	EventMutex:     true,
	EventIO:        true,
}

// IsValid returns true if the event type is a known value.
func (e EventType) IsValid() bool {
	return knownEventTypes[e]
}

// String returns the string representation of the event type.
func (e EventType) String() string {
	return string(e)
}

// ResourceType represents the resource category that an analysis mode targets.
type ResourceType string

const (
	// ResourceCPU is CPU-related profiling.
	ResourceCPU ResourceType = "CPU"

	// ResourceMemory is memory-related profiling.
	ResourceMemory ResourceType = "Memory"

	// ResourceIO is disk/IO-related profiling.
	ResourceIO ResourceType = "Disk"

	// ResourceApp is application-level profiling (e.g. Java wall/lock).
	ResourceApp ResourceType = "App"

	// ResourceGoroutine is Go goroutine profiling.
	ResourceGoroutine ResourceType = "Goroutine"

	// ResourceConcurrency is concurrency-related profiling (block, mutex).
	ResourceConcurrency ResourceType = "Concurrency"
)

// AnalysisMode returns the composite mode string "{profiler}-{event}".
func AnalysisMode(profiler Profiler, event EventType) string {
	return fmt.Sprintf("%s-%s", profiler, event)
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
	ID             int64             `json:"id" db:"id"`
	TaskUUID       string            `json:"tid" db:"tid"`
	Profiler       Profiler          `json:"profiler" db:"profiler"`
	Event          EventType         `json:"event" db:"event"`
	Mode           string            `json:"mode" db:"mode"`
	Status         TaskStatus        `json:"status" db:"status"`
	AnalysisStatus AnalysisStatus    `json:"analysis_status" db:"analysis_status"`
	StatusInfo     string            `json:"status_info" db:"status_info"`
	ResultFile     string            `json:"result_file" db:"result_file"`
	UserName       string            `json:"user_name" db:"user_name"`
	MasterTaskTID  *string           `json:"mastertask_tid" db:"mastertask_tid"`
	COSBucket      string            `json:"cos_bucket" db:"cos_bucket"`
	RequestParams  RequestParams     `json:"request_params" db:"request_params"`
	CallbackURL    string            `json:"callback_url,omitempty" db:"callback_url"`
	Metadata       map[string]string `json:"metadata,omitempty" db:"metadata"`
	CreateTime     time.Time         `json:"create_time" db:"create_time"`
	BeginTime      *time.Time        `json:"begin_time" db:"begin_time"`
	EndTime        *time.Time        `json:"end_time" db:"end_time"`
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

// NewTask creates a new Task instance.
func NewTask(id int64, taskUUID string, profiler Profiler, event EventType) *Task {
	return &Task{
		ID:             id,
		TaskUUID:       taskUUID,
		Profiler:       profiler,
		Event:          event,
		Mode:           AnalysisMode(profiler, event),
		Status:         TaskStatusPending,
		AnalysisStatus: AnalysisStatusPending,
		CreateTime:     time.Now(),
	}
}
