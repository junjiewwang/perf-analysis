// Package scheduler provides task scheduling and worker pool management.
package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/perf-analysis/internal/repository"
	"github.com/perf-analysis/internal/scheduler/source"
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
	processor TaskProcessor
	logger    utils.Logger

	// Source-based task fetching (Strategy Pattern)
	aggregator     *source.Aggregator
	suggestionRepo repository.SuggestionRepository

	workerPool chan struct{}          // Semaphore for worker count
	taskQueue  chan *Task             // Task queue
	wg         sync.WaitGroup         // Wait group for workers
	mu         sync.Mutex             // Mutex for rules cache
	rules      []model.SuggestionRule // Cached rules

	running bool
	stopCh  chan struct{}
}

// New creates a new Scheduler with source aggregator.
func New(config *SchedulerConfig, aggregator *source.Aggregator, processor TaskProcessor, suggestionRepo repository.SuggestionRepository, logger utils.Logger) *Scheduler {
	if config == nil {
		config = DefaultSchedulerConfig()
	}
	if logger == nil {
		logger = utils.NewDefaultLogger(utils.LevelInfo, nil)
	}

	return &Scheduler{
		config:         config,
		aggregator:     aggregator,
		suggestionRepo: suggestionRepo,
		processor:      processor,
		logger:         logger,
		workerPool:     make(chan struct{}, config.WorkerCount),
		taskQueue:      make(chan *Task, config.TaskBatchSize*2),
		stopCh:         make(chan struct{}),
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

	// Refresh rules initially
	s.refreshRules(ctx)

	// Start the aggregator
	if err := s.aggregator.Start(ctx); err != nil {
		return err
	}

	// Start the source-based event loop
	go s.sourceEventLoop(ctx)

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
		return
	}

	s.logger.Info("Task %d completed successfully in %v", task.ID, duration)
}

// sourceEventLoop receives task events from the aggregator and queues them for processing.
func (s *Scheduler) sourceEventLoop(ctx context.Context) {
	// Periodically refresh rules
	rulesTicker := time.NewTicker(30 * time.Second)
	defer rulesTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-rulesTicker.C:
			s.refreshRules(ctx)
		case event, ok := <-s.aggregator.Tasks():
			if !ok {
				s.logger.Info("Aggregator channel closed")
				return
			}

			// Convert TaskEvent to Task
			task := s.convertEventToTask(event)

			// Check if we should accept this task
			if !s.shouldAcceptTask(task) {
				s.logger.Debug("Skipping task %d due to priority constraints", task.ID)
				continue
			}

			// Queue the task
			select {
			case s.taskQueue <- task:
				s.logger.Info("Queued task %d (UUID: %s) from source %s/%s",
					task.ID, task.UUID, event.SourceType, event.SourceName)
			default:
				// Queue full, nack the event so it can be retried
				s.logger.Warn("Task queue full, nacking task %d", task.ID)
				if err := s.aggregator.Nack(ctx, event, "task queue full"); err != nil {
					s.logger.Error("Failed to nack event: %v", err)
				}
			}
		}
	}
}

// refreshRules fetches and caches analysis rules.
func (s *Scheduler) refreshRules(ctx context.Context) {
	if s.suggestionRepo == nil {
		return
	}

	rules, err := s.suggestionRepo.GetAnalysisRules(ctx)
	if err != nil {
		s.logger.Warn("Failed to refresh analysis rules: %v", err)
		return
	}

	s.mu.Lock()
	s.rules = rules
	s.mu.Unlock()

	s.logger.Debug("Refreshed %d analysis rules", len(rules))
}

// convertEventToTask converts a source.TaskEvent to a scheduler.Task.
func (s *Scheduler) convertEventToTask(event *source.TaskEvent) *Task {
	t := event.Task
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
		Priority:      event.Priority,
	}
	return task
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
