// Package model defines the core data structures used throughout the application.
package model

import (
	"encoding/json"
	"time"
)

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
