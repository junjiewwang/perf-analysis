package mock

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/perf-analysis/pkg/model"
)

// MockTaskRepository is a mock implementation of the TaskRepository interface.
type MockTaskRepository struct {
	mock.Mock
}

// GetPendingTasks mocks the GetPendingTasks method.
func (m *MockTaskRepository) GetPendingTasks(ctx context.Context, limit int) ([]*model.Task, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Task), args.Error(1)
}

// GetTaskByID mocks the GetTaskByID method.
func (m *MockTaskRepository) GetTaskByID(ctx context.Context, id int64) (*model.Task, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Task), args.Error(1)
}

// GetTaskByUUID mocks the GetTaskByUUID method.
func (m *MockTaskRepository) GetTaskByUUID(ctx context.Context, uuid string) (*model.Task, error) {
	args := m.Called(ctx, uuid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Task), args.Error(1)
}

// UpdateAnalysisStatus mocks the UpdateAnalysisStatus method.
func (m *MockTaskRepository) UpdateAnalysisStatus(ctx context.Context, id int64, status model.AnalysisStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

// UpdateAnalysisStatusWithInfo mocks the UpdateAnalysisStatusWithInfo method.
func (m *MockTaskRepository) UpdateAnalysisStatusWithInfo(ctx context.Context, id int64, status model.AnalysisStatus, info string) error {
	args := m.Called(ctx, id, status, info)
	return args.Error(0)
}

// LockTaskForAnalysis mocks the LockTaskForAnalysis method.
func (m *MockTaskRepository) LockTaskForAnalysis(ctx context.Context, id int64) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

// ExpectGetPendingTasks sets up an expectation for GetPendingTasks.
func (m *MockTaskRepository) ExpectGetPendingTasks(limit int, tasks []*model.Task, err error) *mock.Call {
	return m.On("GetPendingTasks", mock.Anything, limit).Return(tasks, err)
}

// ExpectUpdateAnalysisStatus sets up an expectation for UpdateAnalysisStatus.
func (m *MockTaskRepository) ExpectUpdateAnalysisStatus(id int64, status model.AnalysisStatus, err error) *mock.Call {
	return m.On("UpdateAnalysisStatus", mock.Anything, id, status).Return(err)
}

// ExpectLockTaskForAnalysis sets up an expectation for LockTaskForAnalysis.
func (m *MockTaskRepository) ExpectLockTaskForAnalysis(id int64, success bool, err error) *mock.Call {
	return m.On("LockTaskForAnalysis", mock.Anything, id).Return(success, err)
}

// MockResultRepository is a mock implementation of the ResultRepository interface.
type MockResultRepository struct {
	mock.Mock
}

// SaveResult mocks the SaveResult method.
func (m *MockResultRepository) SaveResult(ctx context.Context, result *model.AnalysisResult) error {
	args := m.Called(ctx, result)
	return args.Error(0)
}

// GetResultByTaskUUID mocks the GetResultByTaskUUID method.
func (m *MockResultRepository) GetResultByTaskUUID(ctx context.Context, taskUUID string) (*model.AnalysisResult, error) {
	args := m.Called(ctx, taskUUID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.AnalysisResult), args.Error(1)
}

// ExpectSaveResult sets up an expectation for SaveResult.
func (m *MockResultRepository) ExpectSaveResult(err error) *mock.Call {
	return m.On("SaveResult", mock.Anything, mock.Anything).Return(err)
}

// MockSuggestionRepository is a mock implementation of the SuggestionRepository interface.
type MockSuggestionRepository struct {
	mock.Mock
}

// SaveSuggestions mocks the SaveSuggestions method.
func (m *MockSuggestionRepository) SaveSuggestions(ctx context.Context, suggestions []model.Suggestion) error {
	args := m.Called(ctx, suggestions)
	return args.Error(0)
}

// GetSuggestionsByTaskUUID mocks the GetSuggestionsByTaskUUID method.
func (m *MockSuggestionRepository) GetSuggestionsByTaskUUID(ctx context.Context, taskUUID string) ([]model.Suggestion, error) {
	args := m.Called(ctx, taskUUID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.Suggestion), args.Error(1)
}

// ExpectSaveSuggestions sets up an expectation for SaveSuggestions.
func (m *MockSuggestionRepository) ExpectSaveSuggestions(err error) *mock.Call {
	return m.On("SaveSuggestions", mock.Anything, mock.Anything).Return(err)
}
