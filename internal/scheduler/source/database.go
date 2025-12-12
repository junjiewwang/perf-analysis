package source

import (
	"context"
	"sync"
	"time"

	"github.com/perf-analysis/internal/repository"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// SourceTypeDB is the source type constant for database source.
const SourceTypeDB SourceType = "database"

func init() {
	// Register the database source strategy
	Register(SourceTypeDB, NewDatabaseSource)
}

// DatabaseOptions holds database source specific configuration.
type DatabaseOptions struct {
	// PollInterval is how often to poll for new tasks.
	PollInterval time.Duration

	// BatchSize is the maximum number of tasks to fetch per poll.
	BatchSize int
}

// DefaultDatabaseOptions returns the default options.
func DefaultDatabaseOptions() *DatabaseOptions {
	return &DatabaseOptions{
		PollInterval: 2 * time.Second,
		BatchSize:    10,
	}
}

// DatabaseSource implements TaskSource for database-based task fetching.
type DatabaseSource struct {
	name    string
	options *DatabaseOptions
	logger  utils.Logger

	taskRepo       repository.TaskRepository
	suggestionRepo repository.SuggestionRepository

	taskChan chan *TaskEvent
	stopCh   chan struct{}

	mu      sync.RWMutex
	running bool
}

// NewDatabaseSource creates a new database source from configuration.
func NewDatabaseSource(cfg *SourceConfig) (TaskSource, error) {
	opts := &DatabaseOptions{
		PollInterval: cfg.GetDuration("poll_interval", 2*time.Second),
		BatchSize:    cfg.GetInt("batch_size", 10),
	}

	return &DatabaseSource{
		name:     cfg.Name,
		options:  opts,
		taskChan: make(chan *TaskEvent, opts.BatchSize*2),
		stopCh:   make(chan struct{}),
	}, nil
}

// NewDatabaseSourceWithDeps creates a new database source with explicit dependencies.
// This is useful for production use where repositories are already initialized.
func NewDatabaseSourceWithDeps(name string, opts *DatabaseOptions, taskRepo repository.TaskRepository, suggestionRepo repository.SuggestionRepository, logger utils.Logger) *DatabaseSource {
	if opts == nil {
		opts = DefaultDatabaseOptions()
	}
	if logger == nil {
		logger = utils.NewDefaultLogger(utils.LevelInfo, nil)
	}

	return &DatabaseSource{
		name:           name,
		options:        opts,
		logger:         logger,
		taskRepo:       taskRepo,
		suggestionRepo: suggestionRepo,
		taskChan:       make(chan *TaskEvent, opts.BatchSize*2),
		stopCh:         make(chan struct{}),
	}
}

// SetRepositories sets the task and suggestion repositories.
// This must be called before Start if using the factory-created source.
func (s *DatabaseSource) SetRepositories(taskRepo repository.TaskRepository, suggestionRepo repository.SuggestionRepository) {
	s.taskRepo = taskRepo
	s.suggestionRepo = suggestionRepo
}

// SetLogger sets the logger.
func (s *DatabaseSource) SetLogger(logger utils.Logger) {
	s.logger = logger
}

// Type returns the source type.
func (s *DatabaseSource) Type() SourceType {
	return SourceTypeDB
}

// Name returns the source instance name.
func (s *DatabaseSource) Name() string {
	return s.name
}

// Start starts the database polling loop.
func (s *DatabaseSource) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}

	if s.taskRepo == nil {
		s.mu.Unlock()
		return nil // No repository configured, skip
	}

	s.running = true
	s.mu.Unlock()

	if s.logger != nil {
		s.logger.Info("Database source %s starting with poll_interval=%v, batch_size=%d",
			s.name, s.options.PollInterval, s.options.BatchSize)
	}

	go s.pollLoop(ctx)
	return nil
}

// Stop stops the database source.
func (s *DatabaseSource) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	return nil
}

// Tasks returns the task event channel.
func (s *DatabaseSource) Tasks() <-chan *TaskEvent {
	return s.taskChan
}

// Ack acknowledges a task has been processed successfully.
// For database source, this updates the task status to completed.
func (s *DatabaseSource) Ack(ctx context.Context, event *TaskEvent) error {
	if s.taskRepo == nil || event.Task == nil {
		return nil
	}
	return s.taskRepo.UpdateAnalysisStatus(ctx, event.Task.ID, model.AnalysisStatusCompleted)
}

// Nack indicates a task processing failed.
// For database source, this updates the task status to failed.
func (s *DatabaseSource) Nack(ctx context.Context, event *TaskEvent, reason string) error {
	if s.taskRepo == nil || event.Task == nil {
		return nil
	}
	return s.taskRepo.UpdateAnalysisStatusWithInfo(ctx, event.Task.ID, model.AnalysisStatusFailed, reason)
}

// HealthCheck checks the database connection.
func (s *DatabaseSource) HealthCheck(ctx context.Context) error {
	if s.taskRepo == nil {
		return nil
	}
	// Try to fetch a single task as health check
	_, err := s.taskRepo.GetPendingTasks(ctx, 1)
	return err
}

// pollLoop continuously polls the database for pending tasks.
func (s *DatabaseSource) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(s.options.PollInterval)
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

// poll fetches pending tasks and emits them to the task channel.
func (s *DatabaseSource) poll(ctx context.Context) {
	if s.taskRepo == nil {
		return
	}

	tasks, err := s.taskRepo.GetPendingTasks(ctx, s.options.BatchSize)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("Database source %s failed to fetch tasks: %v", s.name, err)
		}
		return
	}

	for _, task := range tasks {
		// Try to lock the task
		locked, err := s.taskRepo.LockTaskForAnalysis(ctx, task.ID)
		if err != nil {
			if s.logger != nil {
				s.logger.Error("Database source %s failed to lock task %d: %v", s.name, task.ID, err)
			}
			continue
		}
		if !locked {
			continue // Task already locked by another instance
		}

		// Create and emit task event
		event := NewTaskEvent(task, SourceTypeDB, s.name).
			WithMetadata("locked_at", time.Now().Format(time.RFC3339))

		select {
		case s.taskChan <- event:
			if s.logger != nil {
				s.logger.Debug("Database source %s emitted task %s", s.name, task.TaskUUID)
			}
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
			// Channel full, task will be picked up in next poll
			if s.logger != nil {
				s.logger.Warn("Database source %s task channel full, task %d will retry", s.name, task.ID)
			}
		}
	}
}

// GetAnalysisRules fetches analysis rules from the suggestion repository.
func (s *DatabaseSource) GetAnalysisRules(ctx context.Context) ([]model.SuggestionRule, error) {
	if s.suggestionRepo == nil {
		return nil, nil
	}
	return s.suggestionRepo.GetAnalysisRules(ctx)
}
