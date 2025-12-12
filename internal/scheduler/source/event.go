package source

import (
	"github.com/perf-analysis/pkg/model"
)

// TaskEvent represents a unified task event from any source.
type TaskEvent struct {
	// ID is the unique identifier for this event.
	ID string

	// Task is the actual task data.
	Task *model.Task

	// SourceType indicates which type of source this event came from.
	SourceType SourceType

	// SourceName is the name of the source instance.
	SourceName string

	// Priority indicates the task priority (higher value = higher priority).
	Priority int

	// Metadata holds source-specific metadata.
	Metadata map[string]string

	// AckToken is used for acknowledgment (e.g., Kafka offset, HTTP request context).
	AckToken interface{}
}

// NewTaskEvent creates a new TaskEvent from a model.Task.
func NewTaskEvent(task *model.Task, sourceType SourceType, sourceName string) *TaskEvent {
	priority := 0
	if task.IsHighPriority() {
		priority = 1
	}

	return &TaskEvent{
		ID:         task.TaskUUID,
		Task:       task,
		SourceType: sourceType,
		SourceName: sourceName,
		Priority:   priority,
		Metadata:   make(map[string]string),
	}
}

// WithMetadata adds metadata to the event and returns the event for chaining.
func (e *TaskEvent) WithMetadata(key, value string) *TaskEvent {
	if e.Metadata == nil {
		e.Metadata = make(map[string]string)
	}
	e.Metadata[key] = value
	return e
}

// WithAckToken sets the ack token and returns the event for chaining.
func (e *TaskEvent) WithAckToken(token interface{}) *TaskEvent {
	e.AckToken = token
	return e
}

// GetMetadata retrieves a metadata value by key.
func (e *TaskEvent) GetMetadata(key string) string {
	if e.Metadata == nil {
		return ""
	}
	return e.Metadata[key]
}
