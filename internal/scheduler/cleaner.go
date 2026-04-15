package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/junjiewwang/perf-analysis/internal/repository"
	"github.com/junjiewwang/perf-analysis/internal/storage"
	"github.com/junjiewwang/perf-analysis/pkg/config"
	"github.com/junjiewwang/perf-analysis/pkg/model"
	"github.com/junjiewwang/perf-analysis/pkg/utils"
)

// ResultCleaner periodically cleans up expired analysis results
// from both storage and database.
type ResultCleaner struct {
	storage   storage.Storage
	repos     *repository.Repositories
	retention *config.RetentionConfig
	interval  time.Duration
	logger    utils.Logger
	stopCh    chan struct{}
}

// NewResultCleaner creates a new ResultCleaner.
func NewResultCleaner(
	store storage.Storage,
	repos *repository.Repositories,
	retention *config.RetentionConfig,
	interval time.Duration,
	logger utils.Logger,
) *ResultCleaner {
	if interval == 0 {
		interval = 1 * time.Hour
	}

	return &ResultCleaner{
		storage:   store,
		repos:     repos,
		retention: retention,
		interval:  interval,
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

// Start begins the periodic cleanup loop.
func (rc *ResultCleaner) Start(ctx context.Context) {
	go rc.run(ctx)
}

// Stop signals the cleaner to stop.
func (rc *ResultCleaner) Stop() {
	close(rc.stopCh)
}

// run is the main loop that periodically checks for and removes expired results.
func (rc *ResultCleaner) run(ctx context.Context) {
	ticker := time.NewTicker(rc.interval)
	defer ticker.Stop()

	// Run once on startup
	rc.clean(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-rc.stopCh:
			return
		case <-ticker.C:
			rc.clean(ctx)
		}
	}
}

// clean performs a single cleanup pass.
func (rc *ResultCleaner) clean(ctx context.Context) {
	rc.logger.Info("starting result cleanup pass")

	tasks, err := rc.repos.Task.GetCompletedTasks(ctx)
	if err != nil {
		rc.logger.Error("failed to get completed tasks for cleanup: %v", err)
		return
	}

	cleaned := 0
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}

		retention := rc.retention.GetRetentionForType(task.Mode)
		expireTime := task.CreateTime.Add(retention)

		if time.Now().Before(expireTime) {
			continue
		}

		if err := rc.cleanTask(ctx, &task); err != nil {
			rc.logger.Warn("failed to clean task %s: %v", task.TaskUUID, err)
			continue
		}
		cleaned++
	}

	if cleaned > 0 {
		rc.logger.Info("cleaned %d expired tasks", cleaned)
	}
}

// cleanTask removes a single task's analysis results from storage.
func (rc *ResultCleaner) cleanTask(ctx context.Context, task *model.Task) error {
	// Delete analysis results from storage (keyed by task UUID)
	if err := rc.storage.DeleteByPrefix(ctx, task.TaskUUID+"/"); err != nil {
		return fmt.Errorf("failed to delete storage objects: %w", err)
	}

	// Update task analysis status to indicate results have been cleaned
	if err := rc.repos.Task.UpdateAnalysisStatus(ctx, task.ID, model.AnalysisStatusEmpty); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	rc.logger.Info("cleaned expired task %s (created: %s)", task.TaskUUID, task.CreateTime.Format(time.RFC3339))
	return nil
}
