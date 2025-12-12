package scheduler

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/internal/scheduler/source"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// MockSuggestionRepository is a mock implementation of SuggestionRepository.
type MockSuggestionRepository struct {
	mock.Mock
}

func (m *MockSuggestionRepository) GetAnalysisRules(ctx context.Context) ([]model.SuggestionRule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.SuggestionRule), args.Error(1)
}

func (m *MockSuggestionRepository) SaveSuggestions(ctx context.Context, suggestions []model.Suggestion) error {
	args := m.Called(ctx, suggestions)
	return args.Error(0)
}

func (m *MockSuggestionRepository) GetSuggestionsByTaskUUID(ctx context.Context, taskUUID string) ([]model.Suggestion, error) {
	args := m.Called(ctx, taskUUID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.Suggestion), args.Error(1)
}

// MockTaskProcessor is a mock implementation of TaskProcessor.
type MockTaskProcessor struct {
	mock.Mock
	processedCount int32
}

func (m *MockTaskProcessor) Process(ctx context.Context, task *Task, rules []model.SuggestionRule) error {
	atomic.AddInt32(&m.processedCount, 1)
	args := m.Called(ctx, task, rules)
	return args.Error(0)
}

func (m *MockTaskProcessor) GetProcessedCount() int32 {
	return atomic.LoadInt32(&m.processedCount)
}

func TestScheduler_New(t *testing.T) {
	processor := &MockTaskProcessor{}
	suggestionRepo := &MockSuggestionRepository{}
	logger := utils.NewDefaultLogger(utils.LevelDebug, io.Discard)

	// Create a simple aggregator with no sources for testing
	aggregator := source.NewAggregator(nil, 10, logger)

	t.Run("WithDefaultConfig", func(t *testing.T) {
		s := New(nil, aggregator, processor, suggestionRepo, nil)
		require.NotNil(t, s)
		assert.Equal(t, 5, s.config.WorkerCount)
		assert.Equal(t, 2*time.Second, s.config.PollInterval)
	})

	t.Run("WithCustomConfig", func(t *testing.T) {
		config := &SchedulerConfig{
			PollInterval:  5 * time.Second,
			WorkerCount:   10,
			PrioritySlots: 3,
			TaskBatchSize: 20,
		}
		s := New(config, aggregator, processor, suggestionRepo, nil)
		require.NotNil(t, s)
		assert.Equal(t, 10, s.config.WorkerCount)
		assert.Equal(t, 5*time.Second, s.config.PollInterval)
	})
}

func TestScheduler_Stats(t *testing.T) {
	processor := &MockTaskProcessor{}
	suggestionRepo := &MockSuggestionRepository{}
	logger := utils.NewDefaultLogger(utils.LevelDebug, io.Discard)
	aggregator := source.NewAggregator(nil, 10, logger)

	config := &SchedulerConfig{
		WorkerCount: 5,
	}

	s := New(config, aggregator, processor, suggestionRepo, nil)

	stats := s.Stats()
	// Before Start(), workerPool is empty, so ActiveWorkers = WorkerCount - 0 = WorkerCount
	assert.Equal(t, 5, stats.ActiveWorkers)
	assert.Equal(t, 5, stats.TotalWorkers)
	assert.False(t, stats.Running)
}

func TestScheduler_ShouldAcceptTask(t *testing.T) {
	processor := &MockTaskProcessor{}
	suggestionRepo := &MockSuggestionRepository{}
	logger := utils.NewDefaultLogger(utils.LevelDebug, io.Discard)
	aggregator := source.NewAggregator(nil, 10, logger)

	config := &SchedulerConfig{
		WorkerCount:   5,
		PrioritySlots: 2,
		PollInterval:  100 * time.Millisecond,
		TaskBatchSize: 5,
	}

	s := New(config, aggregator, processor, suggestionRepo, logger)

	// Need to initialize worker pool like Start() does
	for i := 0; i < config.WorkerCount; i++ {
		s.workerPool <- struct{}{}
	}

	t.Run("HighPriorityTask", func(t *testing.T) {
		task := &Task{Priority: 1}
		assert.True(t, s.shouldAcceptTask(task))
	})

	t.Run("NormalPriorityTask", func(t *testing.T) {
		task := &Task{Priority: 0}
		assert.True(t, s.shouldAcceptTask(task))
	})
}

func TestScheduler_StartStop(t *testing.T) {
	processor := &MockTaskProcessor{}
	suggestionRepo := &MockSuggestionRepository{}
	logger := utils.NewDefaultLogger(utils.LevelDebug, io.Discard)
	aggregator := source.NewAggregator(nil, 10, logger)

	config := &SchedulerConfig{
		PollInterval:  100 * time.Millisecond,
		WorkerCount:   2,
		PrioritySlots: 1,
		TaskBatchSize: 5,
	}

	s := New(config, aggregator, processor, suggestionRepo, logger)

	// Setup expectations
	suggestionRepo.On("GetAnalysisRules", mock.Anything).Return([]model.SuggestionRule{}, nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Start scheduler
	err := s.Start(ctx)
	require.NoError(t, err)

	stats := s.Stats()
	assert.True(t, stats.Running)

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// Stop scheduler
	cancel()
	s.Stop()

	stats = s.Stats()
	assert.False(t, stats.Running)
}

func TestDefaultSchedulerConfig(t *testing.T) {
	config := DefaultSchedulerConfig()
	assert.Equal(t, 2*time.Second, config.PollInterval)
	assert.Equal(t, 5, config.WorkerCount)
	assert.Equal(t, 2, config.PrioritySlots)
	assert.Equal(t, 10, config.TaskBatchSize)
}

func TestScheduler_ConvertEventToTask(t *testing.T) {
	processor := &MockTaskProcessor{}
	suggestionRepo := &MockSuggestionRepository{}
	logger := utils.NewDefaultLogger(utils.LevelDebug, io.Discard)
	aggregator := source.NewAggregator(nil, 10, logger)

	s := New(nil, aggregator, processor, suggestionRepo, logger)

	masterTID := "master-123"
	modelTask := &model.Task{
		ID:            1,
		TaskUUID:      "uuid-123",
		Type:          model.TaskTypeJava,
		ProfilerType:  model.ProfilerTypePerf,
		ResultFile:    "result.data",
		UserName:      "testuser",
		MasterTaskTID: &masterTID,
		COSBucket:     "bucket-1",
		RequestParams: model.RequestParams{
			Duration: 60,
		},
	}

	event := source.NewTaskEvent(modelTask, source.SourceTypeDB, "test-source")
	task := s.convertEventToTask(event)

	assert.Equal(t, int64(1), task.ID)
	assert.Equal(t, "uuid-123", task.UUID)
	assert.Equal(t, model.TaskTypeJava, task.Type)
	assert.Equal(t, model.ProfilerTypePerf, task.ProfilerType)
	assert.Equal(t, "result.data", task.ResultFile)
	assert.Equal(t, "testuser", task.UserName)
	assert.NotNil(t, task.MasterTaskTID)
	assert.Equal(t, "master-123", *task.MasterTaskTID)
}

func TestScheduler_ConvertEventToTask_Priority(t *testing.T) {
	processor := &MockTaskProcessor{}
	suggestionRepo := &MockSuggestionRepository{}
	logger := utils.NewDefaultLogger(utils.LevelDebug, io.Discard)
	aggregator := source.NewAggregator(nil, 10, logger)

	s := New(nil, aggregator, processor, suggestionRepo, logger)

	t.Run("HighPriorityFromEvent", func(t *testing.T) {
		modelTask := &model.Task{
			ID:       1,
			TaskUUID: "uuid-123",
			RequestParams: model.RequestParams{
				Duration: 60, // Short duration = high priority
			},
		}
		event := source.NewTaskEvent(modelTask, source.SourceTypeDB, "test-source")
		task := s.convertEventToTask(event)
		assert.Equal(t, 1, task.Priority) // High priority from event
	})

	t.Run("NormalPriorityFromEvent", func(t *testing.T) {
		modelTask := &model.Task{
			ID:       2,
			TaskUUID: "uuid-456",
			RequestParams: model.RequestParams{
				Duration: 300, // Long duration = normal priority
			},
		}
		event := source.NewTaskEvent(modelTask, source.SourceTypeDB, "test-source")
		task := s.convertEventToTask(event)
		assert.Equal(t, 0, task.Priority) // Normal priority
	})
}
