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

	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// MockTaskFetcher is a mock implementation of TaskFetcher.
type MockTaskFetcher struct {
	mock.Mock
}

func (m *MockTaskFetcher) FetchPendingTasks(ctx context.Context, limit int) ([]*Task, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Task), args.Error(1)
}

func (m *MockTaskFetcher) LockTask(ctx context.Context, taskID int64) (bool, error) {
	args := m.Called(ctx, taskID)
	return args.Bool(0), args.Error(1)
}

func (m *MockTaskFetcher) UpdateTaskStatus(ctx context.Context, taskID int64, status model.AnalysisStatus, info string) error {
	args := m.Called(ctx, taskID, status, info)
	return args.Error(0)
}

func (m *MockTaskFetcher) FetchAnalysisRules(ctx context.Context) ([]model.SuggestionRule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.SuggestionRule), args.Error(1)
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
	fetcher := &MockTaskFetcher{}
	processor := &MockTaskProcessor{}

	t.Run("WithDefaultConfig", func(t *testing.T) {
		s := New(nil, fetcher, processor, nil)
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
		s := New(config, fetcher, processor, nil)
		require.NotNil(t, s)
		assert.Equal(t, 10, s.config.WorkerCount)
		assert.Equal(t, 5*time.Second, s.config.PollInterval)
	})
}

func TestScheduler_Stats(t *testing.T) {
	fetcher := &MockTaskFetcher{}
	processor := &MockTaskProcessor{}
	config := &SchedulerConfig{
		WorkerCount: 5,
	}

	s := New(config, fetcher, processor, nil)

	stats := s.Stats()
	// Before Start(), workerPool is empty, so ActiveWorkers = WorkerCount - 0 = WorkerCount
	assert.Equal(t, 5, stats.ActiveWorkers)
	assert.Equal(t, 5, stats.TotalWorkers)
	assert.False(t, stats.Running)
}

func TestScheduler_ShouldAcceptTask(t *testing.T) {
	fetcher := &MockTaskFetcher{}
	processor := &MockTaskProcessor{}
	logger := utils.NewDefaultLogger(utils.LevelDebug, io.Discard)
	config := &SchedulerConfig{
		WorkerCount:   5,
		PrioritySlots: 2,
		PollInterval:  100 * time.Millisecond,
		TaskBatchSize: 5,
	}

	s := New(config, fetcher, processor, logger)

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
	fetcher := &MockTaskFetcher{}
	processor := &MockTaskProcessor{}
	logger := utils.NewDefaultLogger(utils.LevelDebug, io.Discard)

	config := &SchedulerConfig{
		PollInterval:  100 * time.Millisecond,
		WorkerCount:   2,
		PrioritySlots: 1,
		TaskBatchSize: 5,
	}

	s := New(config, fetcher, processor, logger)

	// Setup expectations
	fetcher.On("FetchPendingTasks", mock.Anything, 5).Return([]*Task{}, nil)
	fetcher.On("FetchAnalysisRules", mock.Anything).Return([]model.SuggestionRule{}, nil)

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

func TestScheduler_ProcessTask(t *testing.T) {
	fetcher := &MockTaskFetcher{}
	processor := &MockTaskProcessor{}
	logger := utils.NewDefaultLogger(utils.LevelDebug, io.Discard)

	config := &SchedulerConfig{
		PollInterval:  100 * time.Millisecond,
		WorkerCount:   2,
		PrioritySlots: 1,
		TaskBatchSize: 5,
	}

	s := New(config, fetcher, processor, logger)

	task := &Task{
		ID:   1,
		UUID: "test-uuid",
		Type: model.TaskTypeJava,
	}

	// Setup expectations
	tasks := []*Task{task}
	fetcher.On("FetchPendingTasks", mock.Anything, 5).Return(tasks, nil).Once()
	fetcher.On("FetchPendingTasks", mock.Anything, 5).Return([]*Task{}, nil)
	fetcher.On("FetchAnalysisRules", mock.Anything).Return([]model.SuggestionRule{}, nil)
	fetcher.On("LockTask", mock.Anything, int64(1)).Return(true, nil)
	processor.On("Process", mock.Anything, task, mock.Anything).Return(nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Start scheduler
	err := s.Start(ctx)
	require.NoError(t, err)

	// Wait for task to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify task was processed
	assert.Equal(t, int32(1), processor.GetProcessedCount())

	// Stop scheduler
	cancel()
	s.Stop()
}

func TestDefaultSchedulerConfig(t *testing.T) {
	config := DefaultSchedulerConfig()
	assert.Equal(t, 2*time.Second, config.PollInterval)
	assert.Equal(t, 5, config.WorkerCount)
	assert.Equal(t, 2, config.PrioritySlots)
	assert.Equal(t, 10, config.TaskBatchSize)
}

func TestConvertModelTask(t *testing.T) {
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

	task := convertModelTask(modelTask)

	assert.Equal(t, int64(1), task.ID)
	assert.Equal(t, "uuid-123", task.UUID)
	assert.Equal(t, model.TaskTypeJava, task.Type)
	assert.Equal(t, model.ProfilerTypePerf, task.ProfilerType)
	assert.Equal(t, "result.data", task.ResultFile)
	assert.Equal(t, "testuser", task.UserName)
	assert.NotNil(t, task.MasterTaskTID)
	assert.Equal(t, "master-123", *task.MasterTaskTID)
	assert.Equal(t, 1, task.Priority) // Short duration = high priority
}

func TestConvertModelTask_LongDuration(t *testing.T) {
	modelTask := &model.Task{
		ID:       2,
		TaskUUID: "uuid-456",
		RequestParams: model.RequestParams{
			Duration: 300, // Long duration
		},
	}

	task := convertModelTask(modelTask)
	assert.Equal(t, 0, task.Priority) // Long duration = normal priority
}
