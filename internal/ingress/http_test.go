package ingress

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/pkg/config"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// fakeTaskRepo is a simple in-memory TaskRepository for testing.
type fakeTaskRepo struct {
	tasks   []*model.Task
	failErr error
}

func (r *fakeTaskRepo) CreateTask(_ context.Context, task *model.Task) error {
	if r.failErr != nil {
		return r.failErr
	}
	task.ID = int64(len(r.tasks) + 1)
	r.tasks = append(r.tasks, task)
	return nil
}

func (r *fakeTaskRepo) GetPendingTasks(_ context.Context, _ int) ([]*model.Task, error) {
	return nil, nil
}

func (r *fakeTaskRepo) GetTaskByID(_ context.Context, _ int64) (*model.Task, error) {
	return nil, nil
}

func (r *fakeTaskRepo) GetTaskByUUID(_ context.Context, _ string) (*model.Task, error) {
	return nil, nil
}

func (r *fakeTaskRepo) UpdateAnalysisStatus(_ context.Context, _ int64, _ model.AnalysisStatus) error {
	return nil
}

func (r *fakeTaskRepo) UpdateAnalysisStatusWithInfo(_ context.Context, _ int64, _ model.AnalysisStatus, _ string) error {
	return nil
}

func (r *fakeTaskRepo) LockTaskForAnalysis(_ context.Context, _ int64) (bool, error) {
	return true, nil
}

func (r *fakeTaskRepo) GetCompletedTasks(_ context.Context) ([]model.Task, error) {
	return nil, nil
}

func newTestHTTPIngress(repo *fakeTaskRepo, callbackURL string) *HTTPIngress {
	cfg := &config.HTTPIngressConfig{
		Enabled:     true,
		ListenAddr:  ":0",
		Path:        "/tasks",
		MaxBodySize: 1 << 20,
		CallbackURL: callbackURL,
	}
	logger := utils.NewDefaultLogger(utils.LevelError, io.Discard)
	return NewHTTPIngress(cfg, repo, logger)
}

func TestHandleTask_Success(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{}
	h := newTestHTTPIngress(repo, "")

	submission := TaskSubmission{
		TaskUUID:   "test-uuid-001",
		Profiler:   "async-profiler",
		Event:      "cpu",
		ResultFile: "path/to/result.jfr",
		UserName:   "testuser",
	}
	body, err := json.Marshal(submission)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleTask(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp HTTPTaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "test-uuid-001", resp.TaskID)

	// Verify persisted task
	require.Len(t, repo.tasks, 1)
	task := repo.tasks[0]
	assert.Equal(t, "test-uuid-001", task.TaskUUID)
	assert.Equal(t, model.Profiler("async-profiler"), task.Profiler)
	assert.Equal(t, model.EventType("cpu"), task.Event)
	assert.Equal(t, "async-profiler-cpu", task.Mode)
	assert.Equal(t, model.TaskStatusCompleted, task.Status)
	assert.Equal(t, model.AnalysisStatusPending, task.AnalysisStatus)
}

func TestHandleTask_WithMetadata(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{}
	h := newTestHTTPIngress(repo, "")

	submission := TaskSubmission{
		TaskUUID: "test-uuid-002",
		Profiler: "pprof",
		Event:    "heap",
		Metadata: map[string]string{
			"env":     "production",
			"cluster": "us-east-1",
		},
	}
	body, err := json.Marshal(submission)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleTask(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	require.Len(t, repo.tasks, 1)
	assert.Equal(t, "production", repo.tasks[0].Metadata["env"])
	assert.Equal(t, "us-east-1", repo.tasks[0].Metadata["cluster"])
}

func TestHandleTask_CallbackURLDowngrade(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{}
	h := newTestHTTPIngress(repo, "https://default-callback.example.com")

	submission := TaskSubmission{
		TaskUUID: "test-uuid-003",
		Profiler: "async-profiler",
		Event:    "alloc",
	}
	body, err := json.Marshal(submission)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleTask(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	require.Len(t, repo.tasks, 1)
	assert.Equal(t, "https://default-callback.example.com", repo.tasks[0].CallbackURL)
}

func TestHandleTask_TaskCallbackURLPreserved(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{}
	h := newTestHTTPIngress(repo, "https://default-callback.example.com")

	submission := TaskSubmission{
		TaskUUID:    "test-uuid-004",
		Profiler:    "pprof",
		Event:       "cpu",
		CallbackURL: "https://task-specific.example.com/callback",
	}
	body, err := json.Marshal(submission)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleTask(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	require.Len(t, repo.tasks, 1)
	assert.Equal(t, "https://task-specific.example.com/callback", repo.tasks[0].CallbackURL)
}

func TestHandleTask_InvalidMethod(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{}
	h := newTestHTTPIngress(repo, "")

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	w := httptest.NewRecorder()
	h.handleTask(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandleTask_InvalidJSON(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{}
	h := newTestHTTPIngress(repo, "")

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	h.handleTask(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleTask_MissingTID(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{}
	h := newTestHTTPIngress(repo, "")

	submission := TaskSubmission{
		Profiler: "async-profiler",
		Event:    "cpu",
	}
	body, _ := json.Marshal(submission)

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleTask(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp HTTPTaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Message, "tid is required")
}

func TestHandleTask_MissingProfiler(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{}
	h := newTestHTTPIngress(repo, "")

	submission := TaskSubmission{
		TaskUUID: "test-uuid",
		Event:    "cpu",
	}
	body, _ := json.Marshal(submission)

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleTask(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp HTTPTaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Message, "profiler is required")
}

func TestHandleTask_InvalidProfiler(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{}
	h := newTestHTTPIngress(repo, "")

	submission := TaskSubmission{
		TaskUUID: "test-uuid",
		Profiler: "unknown-profiler",
		Event:    "cpu",
	}
	body, _ := json.Marshal(submission)

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleTask(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp HTTPTaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Message, "invalid profiler")
}

func TestHandleTask_InvalidEvent(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{}
	h := newTestHTTPIngress(repo, "")

	submission := TaskSubmission{
		TaskUUID: "test-uuid",
		Profiler: "pprof",
		Event:    "invalid-event",
	}
	body, _ := json.Marshal(submission)

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleTask(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp HTTPTaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Message, "invalid event")
}

func TestHandleTask_DBError(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{failErr: assert.AnError}
	h := newTestHTTPIngress(repo, "")

	submission := TaskSubmission{
		TaskUUID: "test-uuid",
		Profiler: "async-profiler",
		Event:    "cpu",
	}
	body, _ := json.Marshal(submission)

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleTask(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleTask_WithRequestParams(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{}
	h := newTestHTTPIngress(repo, "")

	submission := TaskSubmission{
		TaskUUID: "test-uuid-params",
		Profiler: "perf",
		Event:    "cpu",
		RequestParams: &model.RequestParams{
			Duration:      60,
			PerfDuration:  30,
			ContainerName: "my-container",
		},
	}
	body, _ := json.Marshal(submission)

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.handleTask(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	require.Len(t, repo.tasks, 1)
	assert.Equal(t, 60, repo.tasks[0].RequestParams.Duration)
	assert.Equal(t, 30, repo.tasks[0].RequestParams.PerfDuration)
	assert.Equal(t, "my-container", repo.tasks[0].RequestParams.ContainerName)
}

func TestTaskSubmission_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sub     TaskSubmission
		wantErr string
	}{
		{
			name:    "empty tid",
			sub:     TaskSubmission{Profiler: "pprof", Event: "cpu"},
			wantErr: "tid is required",
		},
		{
			name:    "empty profiler",
			sub:     TaskSubmission{TaskUUID: "id", Event: "cpu"},
			wantErr: "profiler is required",
		},
		{
			name:    "empty event",
			sub:     TaskSubmission{TaskUUID: "id", Profiler: "pprof"},
			wantErr: "event is required",
		},
		{
			name:    "invalid profiler",
			sub:     TaskSubmission{TaskUUID: "id", Profiler: "bad", Event: "cpu"},
			wantErr: "invalid profiler",
		},
		{
			name:    "invalid event",
			sub:     TaskSubmission{TaskUUID: "id", Profiler: "pprof", Event: "bad"},
			wantErr: "invalid event",
		},
		{
			name: "valid",
			sub:  TaskSubmission{TaskUUID: "id", Profiler: "pprof", Event: "cpu"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.sub.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTaskSubmission_ToTask(t *testing.T) {
	t.Parallel()
	masterTID := "master-001"
	sub := TaskSubmission{
		TaskUUID:      "uuid-001",
		Profiler:      "async-profiler",
		Event:         "alloc",
		ResultFile:    "path/to/file",
		UserName:      "alice",
		MasterTaskTID: &masterTID,
		COSBucket:     "my-bucket",
		CallbackURL:   "https://cb.example.com",
		RequestParams: &model.RequestParams{Duration: 30},
		Metadata:      map[string]string{"k": "v"},
	}

	task := sub.ToTask()

	assert.Equal(t, "uuid-001", task.TaskUUID)
	assert.Equal(t, model.Profiler("async-profiler"), task.Profiler)
	assert.Equal(t, model.EventType("alloc"), task.Event)
	assert.Equal(t, "async-profiler-alloc", task.Mode)
	assert.Equal(t, "path/to/file", task.ResultFile)
	assert.Equal(t, "alice", task.UserName)
	require.NotNil(t, task.MasterTaskTID)
	assert.Equal(t, "master-001", *task.MasterTaskTID)
	assert.Equal(t, "my-bucket", task.COSBucket)
	assert.Equal(t, "https://cb.example.com", task.CallbackURL)
	assert.Equal(t, 30, task.RequestParams.Duration)
	assert.Equal(t, "v", task.Metadata["k"])
}

func TestHandleHealth(t *testing.T) {
	t.Parallel()
	repo := &fakeTaskRepo{}
	h := newTestHTTPIngress(repo, "")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.handleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "healthy", resp["status"])
}
