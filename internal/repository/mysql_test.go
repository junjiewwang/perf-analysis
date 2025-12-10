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

func TestMySQLTaskRepository_GetPendingTasks(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewMySQLTaskRepository(db)

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
	})
}

func TestMySQLTaskRepository_UpdateAnalysisStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewMySQLTaskRepository(db)

	t.Run("UpdateStatus_Success", func(t *testing.T) {
		mock.ExpectExec("UPDATE hotmethod_task").
			WithArgs(model.AnalysisStatusCompleted, int64(1)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.UpdateAnalysisStatus(context.Background(), 1, model.AnalysisStatusCompleted)
		require.NoError(t, err)
	})
}

func TestMySQLResultRepository_SaveResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewMySQLResultRepository(db, "1.0.0")

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

func TestMySQLSuggestionRepository_SaveSuggestions(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewMySQLSuggestionRepository(db)

	t.Run("SaveSuggestions_Success", func(t *testing.T) {
		suggestions := []model.Suggestion{
			{TaskUUID: "uuid-1", Suggestion: "Test suggestion"},
		}

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO analysis_suggestions").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.SaveSuggestions(context.Background(), suggestions)
		require.NoError(t, err)
	})
}

func TestPostgresMasterTaskRepository(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewPostgresMasterTaskRepository(db)

	t.Run("GetMasterTask_Success", func(t *testing.T) {
		subTIDs := []string{"sub-1", "sub-2"}
		subTIDsJSON, _ := json.Marshal(subTIDs)
		suggestionsJSON, _ := json.Marshal(model.NewMasterTaskSuggestions())

		rows := sqlmock.NewRows([]string{"tid", "sub_tids", "analysis_suggestions", "analysis_status"}).
			AddRow("master-1", subTIDsJSON, suggestionsJSON, model.AnalysisStatusRunning)

		mock.ExpectQuery("SELECT tid, sub_tids").WithArgs("master-1").WillReturnRows(rows)

		task, err := repo.GetMasterTask(context.Background(), "master-1")
		require.NoError(t, err)
		assert.Equal(t, "master-1", task.TID)
		assert.Equal(t, 2, len(task.SubTIDs))
	})

	t.Run("GetMasterTask_NotFound", func(t *testing.T) {
		mock.ExpectQuery("SELECT tid, sub_tids").WithArgs("nonexistent").WillReturnError(sql.ErrNoRows)

		task, err := repo.GetMasterTask(context.Background(), "nonexistent")
		assert.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "master task not found")
	})

	t.Run("GetIncompleteSubTaskCount_Success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"count"}).AddRow(3)
		mock.ExpectQuery("SELECT COUNT").WithArgs("master-1").WillReturnRows(rows)

		count, err := repo.GetIncompleteSubTaskCount(context.Background(), "master-1")
		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})

	t.Run("UpdateMasterTaskStatus_Running", func(t *testing.T) {
		mock.ExpectExec("UPDATE multiple_task SET analysis_status").
			WithArgs(model.AnalysisStatusRunning, "master-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.UpdateMasterTaskStatus(context.Background(), "master-1", model.AnalysisStatusRunning)
		require.NoError(t, err)
	})
}

func TestMySQLMasterTaskRepository(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewMySQLMasterTaskRepository(db)

	t.Run("GetMasterTask_Success", func(t *testing.T) {
		subTIDs := []string{"sub-1"}
		subTIDsJSON, _ := json.Marshal(subTIDs)

		rows := sqlmock.NewRows([]string{"tid", "sub_tids", "analysis_suggestions", "analysis_status"}).
			AddRow("master-mysql-1", subTIDsJSON, nil, model.AnalysisStatusPending)

		mock.ExpectQuery("SELECT tid, sub_tids").WithArgs("master-mysql-1").WillReturnRows(rows)

		task, err := repo.GetMasterTask(context.Background(), "master-mysql-1")
		require.NoError(t, err)
		assert.Equal(t, "master-mysql-1", task.TID)
	})

	t.Run("GetIncompleteSubTaskCount_Success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"count"}).AddRow(0)
		mock.ExpectQuery("SELECT COUNT").WithArgs("master-mysql-1").WillReturnRows(rows)

		count, err := repo.GetIncompleteSubTaskCount(context.Background(), "master-mysql-1")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("CheckAndCompleteIfReady_AllComplete", func(t *testing.T) {
		// GetIncompleteSubTaskCount returns 0
		rows := sqlmock.NewRows([]string{"count"}).AddRow(0)
		mock.ExpectQuery("SELECT COUNT").WithArgs("master-mysql-1").WillReturnRows(rows)

		// UpdateMasterTaskStatus to completed
		mock.ExpectExec("UPDATE multiple_task SET analysis_status").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.CheckAndCompleteIfReady(context.Background(), "master-mysql-1")
		require.NoError(t, err)
	})
}

func TestPostgresResultRepository_UpdateResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewPostgresResultRepository(db, "1.0.0")

	t.Run("UpdateResult_Success", func(t *testing.T) {
		result := &model.AnalysisResult{
			TaskUUID:       "uuid-1",
			ContainersInfo: map[string]model.ContainerInfo{},
			Result:         map[string]model.NamespaceResult{},
		}

		mock.ExpectExec("UPDATE general_analysis_results").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.UpdateResult(context.Background(), result)
		require.NoError(t, err)
	})

	t.Run("UpdateResult_NotFound", func(t *testing.T) {
		result := &model.AnalysisResult{
			TaskUUID:       "nonexistent",
			ContainersInfo: map[string]model.ContainerInfo{},
			Result:         map[string]model.NamespaceResult{},
		}

		mock.ExpectExec("UPDATE general_analysis_results").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.UpdateResult(context.Background(), result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "result not found")
	})
}

func TestMySQLResultRepository_UpdateResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewMySQLResultRepository(db, "1.0.0")

	t.Run("UpdateResult_Success", func(t *testing.T) {
		result := &model.AnalysisResult{
			TaskUUID:       "uuid-1",
			ContainersInfo: map[string]model.ContainerInfo{},
			Result:         map[string]model.NamespaceResult{},
		}

		mock.ExpectExec("UPDATE general_analysis_results").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.UpdateResult(context.Background(), result)
		require.NoError(t, err)
	})
}
