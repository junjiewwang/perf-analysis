package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/pkg/model"
)

func TestPostgresTaskRepository_GetPendingTasks(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewPostgresTaskRepository(db)

	t.Run("GetPendingTasks_Success", func(t *testing.T) {
		requestParams := model.RequestParams{Duration: 60}
		paramsJSON, _ := json.Marshal(requestParams)

		rows := sqlmock.NewRows([]string{
			"id", "tid", "type", "profiler_type", "status", "analysis_status",
			"status_info", "result_file", "user_name", "mastertask_tid", "cos_bucket",
			"request_params", "create_time", "begin_time", "end_time",
		}).AddRow(
			int64(1), "uuid-1", model.TaskTypeJava, model.ProfilerTypePerf,
			model.TaskStatusCompleted, model.AnalysisStatusPending,
			"", "result.data", "testuser", nil, "bucket-1",
			paramsJSON, time.Now(), nil, nil,
		)

		mock.ExpectQuery("SELECT id, tid, type").WillReturnRows(rows)

		tasks, err := repo.GetPendingTasks(context.Background(), 10)
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		assert.Equal(t, int64(1), tasks[0].ID)
		assert.Equal(t, "uuid-1", tasks[0].TaskUUID)
		assert.Equal(t, model.TaskTypeJava, tasks[0].Type)
	})

	t.Run("GetPendingTasks_Empty", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "tid", "type", "profiler_type", "status", "analysis_status",
			"status_info", "result_file", "user_name", "mastertask_tid", "cos_bucket",
			"request_params", "create_time", "begin_time", "end_time",
		})

		mock.ExpectQuery("SELECT id, tid, type").WillReturnRows(rows)

		tasks, err := repo.GetPendingTasks(context.Background(), 10)
		require.NoError(t, err)
		assert.Empty(t, tasks)
	})
}

func TestPostgresTaskRepository_GetTaskByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewPostgresTaskRepository(db)

	t.Run("GetTaskByID_Success", func(t *testing.T) {
		requestParams := model.RequestParams{Duration: 60}
		paramsJSON, _ := json.Marshal(requestParams)

		rows := sqlmock.NewRows([]string{
			"id", "tid", "type", "profiler_type", "status", "analysis_status",
			"status_info", "result_file", "user_name", "mastertask_tid", "cos_bucket",
			"request_params", "create_time", "begin_time", "end_time",
		}).AddRow(
			int64(1), "uuid-1", model.TaskTypeJava, model.ProfilerTypePerf,
			model.TaskStatusCompleted, model.AnalysisStatusPending,
			"", "result.data", "testuser", nil, "bucket-1",
			paramsJSON, time.Now(), nil, nil,
		)

		mock.ExpectQuery("SELECT id, tid, type").WithArgs(int64(1)).WillReturnRows(rows)

		task, err := repo.GetTaskByID(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, int64(1), task.ID)
		assert.Equal(t, "uuid-1", task.TaskUUID)
	})

	t.Run("GetTaskByID_NotFound", func(t *testing.T) {
		mock.ExpectQuery("SELECT id, tid, type").WithArgs(int64(999)).WillReturnError(sql.ErrNoRows)

		task, err := repo.GetTaskByID(context.Background(), 999)
		assert.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "task not found")
	})
}

func TestPostgresTaskRepository_UpdateAnalysisStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewPostgresTaskRepository(db)

	t.Run("UpdateStatus_Success", func(t *testing.T) {
		mock.ExpectExec("UPDATE hotmethod_task").
			WithArgs(model.AnalysisStatusCompleted, int64(1)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.UpdateAnalysisStatus(context.Background(), 1, model.AnalysisStatusCompleted)
		require.NoError(t, err)
	})

	t.Run("UpdateStatus_NotFound", func(t *testing.T) {
		mock.ExpectExec("UPDATE hotmethod_task").
			WithArgs(model.AnalysisStatusCompleted, int64(999)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.UpdateAnalysisStatus(context.Background(), 999, model.AnalysisStatusCompleted)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task not found")
	})
}

func TestPostgresTaskRepository_LockTaskForAnalysis(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewPostgresTaskRepository(db)

	t.Run("Lock_Success", func(t *testing.T) {
		mock.ExpectBegin()

		rows := sqlmock.NewRows([]string{"analysis_status"}).AddRow(model.AnalysisStatusPending)
		mock.ExpectQuery("SELECT analysis_status").
			WithArgs(int64(1), model.AnalysisStatusPending).
			WillReturnRows(rows)

		mock.ExpectExec("UPDATE hotmethod_task").
			WithArgs(model.AnalysisStatusRunning, int64(1)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		mock.ExpectCommit()

		locked, err := repo.LockTaskForAnalysis(context.Background(), 1)
		require.NoError(t, err)
		assert.True(t, locked)
	})

	t.Run("Lock_AlreadyLocked", func(t *testing.T) {
		mock.ExpectBegin()

		mock.ExpectQuery("SELECT analysis_status").
			WithArgs(int64(1), model.AnalysisStatusPending).
			WillReturnError(sql.ErrNoRows)

		mock.ExpectRollback()

		locked, err := repo.LockTaskForAnalysis(context.Background(), 1)
		require.NoError(t, err)
		assert.False(t, locked)
	})
}

func TestPostgresResultRepository_SaveResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewPostgresResultRepository(db, "1.0.0")

	t.Run("SaveResult_Success", func(t *testing.T) {
		result := &model.AnalysisResult{
			TaskUUID:       "uuid-1",
			ContainersInfo: map[string]model.ContainerInfo{},
			Result:         map[string]model.NamespaceResult{},
		}

		mock.ExpectExec("INSERT INTO general_analysis_results").
			WithArgs(result.TaskUUID, sqlmock.AnyArg(), sqlmock.AnyArg(), "1.0.0").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.SaveResult(context.Background(), result)
		require.NoError(t, err)
	})
}

func TestPostgresResultRepository_GetResultByTaskUUID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewPostgresResultRepository(db, "1.0.0")

	t.Run("GetResult_Success", func(t *testing.T) {
		containersInfo, _ := json.Marshal(map[string]model.ContainerInfo{})
		result, _ := json.Marshal(map[string]model.NamespaceResult{})

		rows := sqlmock.NewRows([]string{"tid", "containers_info", "result", "version"}).
			AddRow("uuid-1", containersInfo, result, "1.0.0")

		mock.ExpectQuery("SELECT tid, containers_info").
			WithArgs("uuid-1").
			WillReturnRows(rows)

		res, err := repo.GetResultByTaskUUID(context.Background(), "uuid-1")
		require.NoError(t, err)
		assert.Equal(t, "uuid-1", res.TaskUUID)
	})

	t.Run("GetResult_NotFound", func(t *testing.T) {
		mock.ExpectQuery("SELECT tid, containers_info").
			WithArgs("uuid-999").
			WillReturnError(sql.ErrNoRows)

		res, err := repo.GetResultByTaskUUID(context.Background(), "uuid-999")
		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Contains(t, err.Error(), "result not found")
	})
}

func TestPostgresSuggestionRepository_SaveSuggestions(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewPostgresSuggestionRepository(db)

	t.Run("SaveSuggestions_Success", func(t *testing.T) {
		suggestions := []model.Suggestion{
			{TaskUUID: "uuid-1", Suggestion: "Test suggestion 1"},
			{TaskUUID: "uuid-1", Suggestion: "Test suggestion 2"},
		}

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO analysis_suggestions").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("INSERT INTO analysis_suggestions").WillReturnResult(sqlmock.NewResult(2, 1))
		mock.ExpectCommit()

		err := repo.SaveSuggestions(context.Background(), suggestions)
		require.NoError(t, err)
	})

	t.Run("SaveSuggestions_Empty", func(t *testing.T) {
		err := repo.SaveSuggestions(context.Background(), []model.Suggestion{})
		require.NoError(t, err)
	})

	t.Run("SaveSuggestions_SkipEmpty", func(t *testing.T) {
		suggestions := []model.Suggestion{
			{TaskUUID: "uuid-1", Suggestion: ""}, // Should be skipped
			{TaskUUID: "uuid-1", Suggestion: "Valid suggestion"},
		}

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO analysis_suggestions").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.SaveSuggestions(context.Background(), suggestions)
		require.NoError(t, err)
	})
}

func TestPostgresSuggestionRepository_GetAnalysisRules(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewPostgresSuggestionRepository(db)

	t.Run("GetRules_Success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "type", "operation", "target", "target_type", "threshold", "suggestion_content",
		}).
			AddRow(int64(1), "cpu", "gt", "GC", "function", 10.0, "High GC overhead").
			AddRow(int64(2), "cpu", "gt", "Lock", "function", 5.0, "Lock contention detected")

		mock.ExpectQuery("SELECT id, type, operation").WillReturnRows(rows)

		rules, err := repo.GetAnalysisRules(context.Background())
		require.NoError(t, err)
		require.Len(t, rules, 2)
		assert.Equal(t, "cpu", rules[0].Type)
		assert.Equal(t, "High GC overhead", rules[0].SuggestionContent)
	})
}
