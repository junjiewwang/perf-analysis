// Package repository provides database abstraction for the perf-analysis service.
package repository

import (
	"context"

	"github.com/perf-analysis/pkg/model"
)

// TaskRepository defines the interface for task-related database operations.
type TaskRepository interface {
	// GetPendingTasks retrieves tasks that are pending analysis.
	GetPendingTasks(ctx context.Context, limit int) ([]*model.Task, error)

	// GetTaskByID retrieves a task by its ID.
	GetTaskByID(ctx context.Context, id int64) (*model.Task, error)

	// GetTaskByUUID retrieves a task by its UUID.
	GetTaskByUUID(ctx context.Context, uuid string) (*model.Task, error)

	// UpdateAnalysisStatus updates the analysis status of a task.
	UpdateAnalysisStatus(ctx context.Context, id int64, status model.AnalysisStatus) error

	// UpdateAnalysisStatusWithInfo updates the analysis status with additional info.
	UpdateAnalysisStatusWithInfo(ctx context.Context, id int64, status model.AnalysisStatus, info string) error

	// LockTaskForAnalysis attempts to lock a task for analysis (prevents concurrent processing).
	LockTaskForAnalysis(ctx context.Context, id int64) (bool, error)
}

// ResultRepository defines the interface for analysis result operations.
type ResultRepository interface {
	// SaveResult saves an analysis result to the database.
	SaveResult(ctx context.Context, result *model.AnalysisResult) error

	// GetResultByTaskUUID retrieves the analysis result for a task.
	GetResultByTaskUUID(ctx context.Context, taskUUID string) (*model.AnalysisResult, error)

	// UpdateResult updates an existing analysis result.
	UpdateResult(ctx context.Context, result *model.AnalysisResult) error
}

// SuggestionRepository defines the interface for suggestion operations.
type SuggestionRepository interface {
	// SaveSuggestions saves multiple suggestions to the database.
	SaveSuggestions(ctx context.Context, suggestions []model.Suggestion) error

	// GetSuggestionsByTaskUUID retrieves suggestions for a task.
	GetSuggestionsByTaskUUID(ctx context.Context, taskUUID string) ([]model.Suggestion, error)

	// GetAnalysisRules retrieves all active analysis rules.
	GetAnalysisRules(ctx context.Context) ([]model.SuggestionRule, error)
}

// MasterTaskRepository defines the interface for master task operations.
type MasterTaskRepository interface {
	// GetMasterTask retrieves a master task by its UUID.
	GetMasterTask(ctx context.Context, masterTID string) (*MasterTask, error)

	// UpdateMasterTaskSuggestions updates the suggestions for a master task.
	UpdateMasterTaskSuggestions(ctx context.Context, masterTID string, resourceType string, suggestions *model.SuggestionGroup) error

	// UpdateMasterTaskStatus updates the analysis status of a master task.
	UpdateMasterTaskStatus(ctx context.Context, masterTID string, status model.AnalysisStatus) error

	// GetIncompleteSubTaskCount returns the count of incomplete sub-tasks.
	GetIncompleteSubTaskCount(ctx context.Context, masterTID string) (int, error)

	// CheckAndCompleteIfReady checks if all sub-tasks are done and updates status.
	CheckAndCompleteIfReady(ctx context.Context, masterTID string) error
}

// MasterTask represents a master task that may have sub-tasks.
type MasterTask struct {
	TID                 string                       `json:"tid" db:"tid"`
	SubTIDs             []string                     `json:"sub_tids" db:"sub_tids"`
	AnalysisSuggestions *model.MasterTaskSuggestions `json:"analysis_suggestions" db:"analysis_suggestions"`
	AnalysisStatus      model.AnalysisStatus         `json:"analysis_status" db:"analysis_status"`
}
