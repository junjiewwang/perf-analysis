// Package scheduler provides task scheduling and worker pool management.
package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/perf-analysis/pkg/config"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// Task represents a task to be processed by the worker pool.
type Task struct {
	ID            int64
	UUID          string
	Type          model.TaskType
	ProfilerType  model.ProfilerType
	ResultFile    string
	UserName      string
	MasterTaskTID *string
	COSBucket     string
	RequestParams model.RequestParams
	Priority      int // Higher value = higher priority
}

// TaskFetcher defines the interface for fetching tasks from the database.
type TaskFetcher interface {
	// FetchPendingTasks returns pending tasks to be processed.
	FetchPendingTasks(ctx context.Context, limit int) ([]*Task, error)

	// LockTask attempts to lock a task for processing.
	LockTask(ctx context.Context, taskID int64) (bool, error)

	// UpdateTaskStatus updates the task status.
	UpdateTaskStatus(ctx context.Context, taskID int64, status model.AnalysisStatus, info string) error

	// FetchAnalysisRules returns the analysis rules from the database.
	FetchAnalysisRules(ctx context.Context) ([]model.SuggestionRule, error)
}

// TaskProcessor defines the interface for processing tasks.
type TaskProcessor interface {
	// Process processes a single task.
	Process(ctx context.Context, task *Task, rules []model.SuggestionRule) error
}

// SchedulerConfig holds scheduler configuration.
type SchedulerConfig struct {
	PollInterval  time.Duration // How often to poll for new tasks
	WorkerCount   int           // Number of concurrent workers
	PrioritySlots int           // Reserved slots for high priority tasks
	TaskBatchSize int           // Max tasks to fetch per poll
}

// DefaultSchedulerConfig returns default scheduler configuration.
func DefaultSchedulerConfig() *SchedulerConfig {
	return &SchedulerConfig{
		PollInterval:  2 * time.Second,
		WorkerCount:   5,
		PrioritySlots: 2,
		TaskBatchSize: 10,
	}
}

// FromConfig creates scheduler config from application config.
func FromConfig(cfg *config.SchedulerConfig) *SchedulerConfig {
	return &SchedulerConfig{
		PollInterval:  time.Duration(cfg.PollInterval) * time.Second,
		WorkerCount:   cfg.WorkerCount,
		PrioritySlots: cfg.PrioritySlots,
		TaskBatchSize: cfg.TaskBatchSize,
	}
}

// Scheduler manages task scheduling and worker pool.
type Scheduler struct {
	config    *SchedulerConfig
	fetcher   TaskFetcher
	processor TaskProcessor
	logger    utils.Logger

	workerPool chan struct{}          // Semaphore for worker count
	taskQueue  chan *Task             // Task queue
	wg         sync.WaitGroup         // Wait group for workers
	mu         sync.Mutex             // Mutex for rules cache
	rules      []model.SuggestionRule // Cached rules

	running bool
	stopCh  chan struct{}
}

// New creates a new Scheduler.
func New(config *SchedulerConfig, fetcher TaskFetcher, processor TaskProcessor, logger utils.Logger) *Scheduler {
	if config == nil {
		config = DefaultSchedulerConfig()
	}
	if logger == nil {
		logger = utils.NewDefaultLogger(utils.LevelInfo, nil)
	}

	return &Scheduler{
		config:     config,
		fetcher:    fetcher,
		processor:  processor,
		logger:     logger,
		workerPool: make(chan struct{}, config.WorkerCount),
		taskQueue:  make(chan *Task, config.TaskBatchSize*2),
		stopCh:     make(chan struct{}),
	}
}

// Start starts the scheduler.
func (s *Scheduler) Start(ctx context.Context) error {
	s.logger.Info("Starting scheduler with %d workers", s.config.WorkerCount)

	s.running = true

	// Start worker goroutines
	for i := 0; i < s.config.WorkerCount; i++ {
		s.workerPool <- struct{}{}
	}

	// Start the task polling loop
	go s.pollLoop(ctx)

	// Start the task processing loop
	go s.processLoop(ctx)

	return nil
}

// Stop stops the scheduler gracefully.
func (s *Scheduler) Stop() {
	s.logger.Info("Stopping scheduler...")
	s.running = false
	close(s.stopCh)

	// Wait for all workers to complete
	s.wg.Wait()
	s.logger.Info("Scheduler stopped")
}

// pollLoop continuously polls for new tasks.
func (s *Scheduler) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()

	// Initial poll
	s.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

// poll fetches and queues pending tasks.
func (s *Scheduler) poll(ctx context.Context) {
	// Check available capacity
	activeWorkers := s.config.WorkerCount - len(s.workerPool)
	if activeWorkers >= s.config.WorkerCount {
		return // All workers busy
	}

	// Fetch pending tasks
	tasks, err := s.fetcher.FetchPendingTasks(ctx, s.config.TaskBatchSize)
	if err != nil {
		s.logger.Error("Failed to fetch pending tasks: %v", err)
		return
	}

	if len(tasks) == 0 {
		return
	}

	// Refresh analysis rules
	rules, err := s.fetcher.FetchAnalysisRules(ctx)
	if err != nil {
		s.logger.Warn("Failed to fetch analysis rules: %v", err)
	} else {
		s.mu.Lock()
		s.rules = rules
		s.mu.Unlock()
	}

	// Queue tasks with priority consideration
	for _, task := range tasks {
		// Check if we should accept this task
		if !s.shouldAcceptTask(task) {
			continue
		}

		// Try to lock the task
		locked, err := s.fetcher.LockTask(ctx, task.ID)
		if err != nil {
			s.logger.Error("Failed to lock task %d: %v", task.ID, err)
			continue
		}
		if !locked {
			continue // Task already locked by another instance
		}

		// Queue the task
		select {
		case s.taskQueue <- task:
			s.logger.Info("Queued task %d (UUID: %s)", task.ID, task.UUID)
		default:
			// Queue full, task will be picked up in next poll
			s.logger.Warn("Task queue full, task %d will be retried", task.ID)
		}
	}
}

// shouldAcceptTask determines if a task should be accepted based on priority.
func (s *Scheduler) shouldAcceptTask(task *Task) bool {
	activeWorkers := s.config.WorkerCount - len(s.workerPool)
	reservedSlots := s.config.WorkerCount - s.config.PrioritySlots

	// High priority tasks can always be accepted if there's capacity
	if task.Priority > 0 {
		return activeWorkers < s.config.WorkerCount
	}

	// Normal priority tasks can only use non-reserved slots
	return activeWorkers < reservedSlots
}

// processLoop processes queued tasks.
func (s *Scheduler) processLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case task := <-s.taskQueue:
			// Acquire a worker slot
			select {
			case <-s.workerPool:
				s.wg.Add(1)
				go s.processTask(ctx, task)
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			}
		}
	}
}

// processTask processes a single task.
func (s *Scheduler) processTask(ctx context.Context, task *Task) {
	defer func() {
		s.workerPool <- struct{}{} // Release worker slot
		s.wg.Done()
	}()

	s.logger.Info("Processing task %d (UUID: %s, Type: %d, Profiler: %d)",
		task.ID, task.UUID, task.Type, task.ProfilerType)

	// Get cached rules
	s.mu.Lock()
	rules := s.rules
	s.mu.Unlock()

	// Process the task
	startTime := time.Now()
	err := s.processor.Process(ctx, task, rules)
	duration := time.Since(startTime)

	if err != nil {
		s.logger.Error("Task %d failed after %v: %v", task.ID, duration, err)
		if updateErr := s.fetcher.UpdateTaskStatus(ctx, task.ID, model.AnalysisStatusFailed, err.Error()); updateErr != nil {
			s.logger.Error("Failed to update task status: %v", updateErr)
		}
		return
	}

	s.logger.Info("Task %d completed successfully in %v", task.ID, duration)
}

// Stats returns current scheduler statistics.
func (s *Scheduler) Stats() SchedulerStats {
	return SchedulerStats{
		ActiveWorkers: s.config.WorkerCount - len(s.workerPool),
		TotalWorkers:  s.config.WorkerCount,
		QueuedTasks:   len(s.taskQueue),
		Running:       s.running,
	}
}

// SchedulerStats holds scheduler statistics.
type SchedulerStats struct {
	ActiveWorkers int  `json:"active_workers"`
	TotalWorkers  int  `json:"total_workers"`
	QueuedTasks   int  `json:"queued_tasks"`
	Running       bool `json:"running"`
}
