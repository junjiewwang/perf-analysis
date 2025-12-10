package scheduler

import (
	"context"

	"github.com/perf-analysis/internal/repository"
	"github.com/perf-analysis/pkg/model"
)

// RepositoryTaskFetcher implements TaskFetcher using repository interfaces.
type RepositoryTaskFetcher struct {
	taskRepo       repository.TaskRepository
	suggestionRepo repository.SuggestionRepository
}

// NewRepositoryTaskFetcher creates a new RepositoryTaskFetcher.
func NewRepositoryTaskFetcher(taskRepo repository.TaskRepository, suggestionRepo repository.SuggestionRepository) *RepositoryTaskFetcher {
	return &RepositoryTaskFetcher{
		taskRepo:       taskRepo,
		suggestionRepo: suggestionRepo,
	}
}

// FetchPendingTasks returns pending tasks to be processed.
func (f *RepositoryTaskFetcher) FetchPendingTasks(ctx context.Context, limit int) ([]*Task, error) {
	tasks, err := f.taskRepo.GetPendingTasks(ctx, limit)
	if err != nil {
		return nil, err
	}

	result := make([]*Task, len(tasks))
	for i, t := range tasks {
		result[i] = convertModelTask(t)
	}

	return result, nil
}

// LockTask attempts to lock a task for processing.
func (f *RepositoryTaskFetcher) LockTask(ctx context.Context, taskID int64) (bool, error) {
	return f.taskRepo.LockTaskForAnalysis(ctx, taskID)
}

// UpdateTaskStatus updates the task status.
func (f *RepositoryTaskFetcher) UpdateTaskStatus(ctx context.Context, taskID int64, status model.AnalysisStatus, info string) error {
	if info != "" {
		return f.taskRepo.UpdateAnalysisStatusWithInfo(ctx, taskID, status, info)
	}
	return f.taskRepo.UpdateAnalysisStatus(ctx, taskID, status)
}

// FetchAnalysisRules returns the analysis rules from the database.
func (f *RepositoryTaskFetcher) FetchAnalysisRules(ctx context.Context) ([]model.SuggestionRule, error) {
	return f.suggestionRepo.GetAnalysisRules(ctx)
}

// convertModelTask converts a model.Task to a scheduler.Task.
func convertModelTask(t *model.Task) *Task {
	task := &Task{
		ID:            t.ID,
		UUID:          t.TaskUUID,
		Type:          t.Type,
		ProfilerType:  t.ProfilerType,
		ResultFile:    t.ResultFile,
		UserName:      t.UserName,
		MasterTaskTID: t.MasterTaskTID,
		COSBucket:     t.COSBucket,
		RequestParams: t.RequestParams,
		Priority:      0, // Default priority
	}

	// Set priority based on duration (short tasks get higher priority)
	if t.RequestParams.Duration > 0 && t.RequestParams.Duration <= 120 {
		task.Priority = 1 // High priority for short tasks
	}
	if t.RequestParams.PerfDuration > 0 && t.RequestParams.PerfDuration <= 120 {
		task.Priority = 1
	}

	return task
}
