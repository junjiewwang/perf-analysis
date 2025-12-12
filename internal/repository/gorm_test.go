package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/perf-analysis/pkg/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Create tables
	err = db.AutoMigrate(
		&HotmethodTask{},
		&GeneralAnalysisResult{},
		&AnalysisSuggestion{},
		&AnalysisSuggestionRule{},
		&MultipleTask{},
	)
	require.NoError(t, err)

	return db
}

func TestGormTaskRepository_GetPendingTasks(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormTaskRepository(db)
	ctx := context.Background()

	t.Run("GetPendingTasks_Empty", func(t *testing.T) {
		tasks, err := repo.GetPendingTasks(ctx, 10)
		require.NoError(t, err)
		assert.Empty(t, tasks)
	})

	t.Run("GetPendingTasks_WithData", func(t *testing.T) {
		// Insert test data
		task := &HotmethodTask{
			TID:            "test-uuid-1",
			Type:           model.TaskTypeJava,
			ProfilerType:   model.ProfilerTypePerf,
			Status:         model.TaskStatusCompleted,
			AnalysisStatus: model.AnalysisStatusPending,
			UserName:       "testuser",
		}
		require.NoError(t, db.Create(task).Error)

		tasks, err := repo.GetPendingTasks(ctx, 10)
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		assert.Equal(t, "test-uuid-1", tasks[0].TaskUUID)
	})
}

func TestGormTaskRepository_GetTaskByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormTaskRepository(db)
	ctx := context.Background()

	t.Run("GetTaskByID_NotFound", func(t *testing.T) {
		task, err := repo.GetTaskByID(ctx, 999)
		assert.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "task not found")
	})

	t.Run("GetTaskByID_Success", func(t *testing.T) {
		// Insert test data
		task := &HotmethodTask{
			TID:            "test-uuid-2",
			Type:           model.TaskTypeGeneric,
			ProfilerType:   model.ProfilerTypePerf,
			Status:         model.TaskStatusCompleted,
			AnalysisStatus: model.AnalysisStatusPending,
		}
		require.NoError(t, db.Create(task).Error)

		result, err := repo.GetTaskByID(ctx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, "test-uuid-2", result.TaskUUID)
	})
}

func TestGormTaskRepository_GetTaskByUUID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormTaskRepository(db)
	ctx := context.Background()

	t.Run("GetTaskByUUID_NotFound", func(t *testing.T) {
		task, err := repo.GetTaskByUUID(ctx, "nonexistent")
		assert.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "task not found")
	})

	t.Run("GetTaskByUUID_Success", func(t *testing.T) {
		task := &HotmethodTask{
			TID:            "test-uuid-3",
			Type:           model.TaskTypeGeneric,
			ProfilerType:   model.ProfilerTypePerf,
			Status:         model.TaskStatusCompleted,
			AnalysisStatus: model.AnalysisStatusPending,
		}
		require.NoError(t, db.Create(task).Error)

		result, err := repo.GetTaskByUUID(ctx, "test-uuid-3")
		require.NoError(t, err)
		assert.Equal(t, task.ID, result.ID)
	})
}

func TestGormTaskRepository_UpdateAnalysisStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormTaskRepository(db)
	ctx := context.Background()

	t.Run("UpdateStatus_NotFound", func(t *testing.T) {
		err := repo.UpdateAnalysisStatus(ctx, 999, model.AnalysisStatusCompleted)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task not found")
	})

	t.Run("UpdateStatus_Success", func(t *testing.T) {
		task := &HotmethodTask{
			TID:            "test-uuid-4",
			Type:           model.TaskTypeGeneric,
			ProfilerType:   model.ProfilerTypePerf,
			Status:         model.TaskStatusCompleted,
			AnalysisStatus: model.AnalysisStatusPending,
		}
		require.NoError(t, db.Create(task).Error)

		err := repo.UpdateAnalysisStatus(ctx, task.ID, model.AnalysisStatusCompleted)
		require.NoError(t, err)

		// Verify update
		var updated HotmethodTask
		require.NoError(t, db.First(&updated, task.ID).Error)
		assert.Equal(t, model.AnalysisStatusCompleted, updated.AnalysisStatus)
	})
}

func TestGormTaskRepository_UpdateAnalysisStatusWithInfo(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormTaskRepository(db)
	ctx := context.Background()

	task := &HotmethodTask{
		TID:            "test-uuid-5",
		Type:           model.TaskTypeGeneric,
		ProfilerType:   model.ProfilerTypePerf,
		Status:         model.TaskStatusCompleted,
		AnalysisStatus: model.AnalysisStatusPending,
	}
	require.NoError(t, db.Create(task).Error)

	err := repo.UpdateAnalysisStatusWithInfo(ctx, task.ID, model.AnalysisStatusFailed, "error message")
	require.NoError(t, err)

	var updated HotmethodTask
	require.NoError(t, db.First(&updated, task.ID).Error)
	assert.Equal(t, model.AnalysisStatusFailed, updated.AnalysisStatus)
	assert.Equal(t, "error message", updated.StatusInfo)
}

func TestGormTaskRepository_LockTaskForAnalysis(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormTaskRepository(db)
	ctx := context.Background()

	t.Run("Lock_NotFound", func(t *testing.T) {
		locked, err := repo.LockTaskForAnalysis(ctx, 999)
		require.NoError(t, err)
		assert.False(t, locked)
	})

	t.Run("Lock_Success", func(t *testing.T) {
		task := &HotmethodTask{
			TID:            "test-uuid-6",
			Type:           model.TaskTypeGeneric,
			ProfilerType:   model.ProfilerTypePerf,
			Status:         model.TaskStatusCompleted,
			AnalysisStatus: model.AnalysisStatusPending,
		}
		require.NoError(t, db.Create(task).Error)

		locked, err := repo.LockTaskForAnalysis(ctx, task.ID)
		require.NoError(t, err)
		assert.True(t, locked)

		// Verify status changed to running
		var updated HotmethodTask
		require.NoError(t, db.First(&updated, task.ID).Error)
		assert.Equal(t, model.AnalysisStatusRunning, updated.AnalysisStatus)
	})
}

func TestGormResultRepository(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormResultRepository(db, "1.0.0")
	ctx := context.Background()

	t.Run("SaveResult_Success", func(t *testing.T) {
		result := &model.AnalysisResult{
			TaskUUID:       "result-uuid-1",
			ContainersInfo: map[string]model.ContainerInfo{},
			Result:         map[string]model.NamespaceResult{},
		}

		err := repo.SaveResult(ctx, result)
		require.NoError(t, err)
	})

	t.Run("GetResultByTaskUUID_Success", func(t *testing.T) {
		result, err := repo.GetResultByTaskUUID(ctx, "result-uuid-1")
		require.NoError(t, err)
		assert.Equal(t, "result-uuid-1", result.TaskUUID)
		assert.Equal(t, "1.0.0", result.Version)
	})

	t.Run("GetResultByTaskUUID_NotFound", func(t *testing.T) {
		result, err := repo.GetResultByTaskUUID(ctx, "nonexistent")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "result not found")
	})

	t.Run("UpdateResult_Success", func(t *testing.T) {
		result := &model.AnalysisResult{
			TaskUUID:       "result-uuid-1",
			ContainersInfo: map[string]model.ContainerInfo{"test": {}},
			Result:         map[string]model.NamespaceResult{},
		}

		err := repo.UpdateResult(ctx, result)
		require.NoError(t, err)
	})

	t.Run("UpdateResult_NotFound", func(t *testing.T) {
		result := &model.AnalysisResult{
			TaskUUID:       "nonexistent",
			ContainersInfo: map[string]model.ContainerInfo{},
			Result:         map[string]model.NamespaceResult{},
		}

		err := repo.UpdateResult(ctx, result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "result not found")
	})
}

func TestGormSuggestionRepository(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormSuggestionRepository(db)
	ctx := context.Background()

	t.Run("SaveSuggestions_Empty", func(t *testing.T) {
		err := repo.SaveSuggestions(ctx, []model.Suggestion{})
		require.NoError(t, err)
	})

	t.Run("SaveSuggestions_Success", func(t *testing.T) {
		suggestions := []model.Suggestion{
			{TaskUUID: "sug-uuid-1", Suggestion: "Test suggestion 1"},
			{TaskUUID: "sug-uuid-1", Suggestion: "Test suggestion 2"},
		}

		err := repo.SaveSuggestions(ctx, suggestions)
		require.NoError(t, err)
	})

	t.Run("SaveSuggestions_SkipEmpty", func(t *testing.T) {
		suggestions := []model.Suggestion{
			{TaskUUID: "sug-uuid-2", Suggestion: ""},
			{TaskUUID: "sug-uuid-2", Suggestion: "Valid suggestion"},
		}

		err := repo.SaveSuggestions(ctx, suggestions)
		require.NoError(t, err)

		// Verify only one was saved
		result, err := repo.GetSuggestionsByTaskUUID(ctx, "sug-uuid-2")
		require.NoError(t, err)
		assert.Len(t, result, 1)
	})

	t.Run("GetSuggestionsByTaskUUID_Success", func(t *testing.T) {
		result, err := repo.GetSuggestionsByTaskUUID(ctx, "sug-uuid-1")
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("GetAnalysisRules_Success", func(t *testing.T) {
		// Insert a rule
		rule := &AnalysisSuggestionRule{
			Type:              "cpu",
			Operation:         "gt",
			Target:            "GC",
			TargetType:        "function",
			Threshold:         10.0,
			SuggestionContent: "High GC overhead",
		}
		require.NoError(t, db.Create(rule).Error)

		rules, err := repo.GetAnalysisRules(ctx)
		require.NoError(t, err)
		assert.Len(t, rules, 1)
		assert.Equal(t, "cpu", rules[0].Type)
	})
}

func TestGormMasterTaskRepository(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormMasterTaskRepository(db)
	ctx := context.Background()

	t.Run("GetMasterTask_NotFound", func(t *testing.T) {
		task, err := repo.GetMasterTask(ctx, "nonexistent")
		assert.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "master task not found")
	})

	t.Run("GetMasterTask_Success", func(t *testing.T) {
		task := &MultipleTask{
			TID:            "master-1",
			SubTIDs:        JSONField(`["sub-1", "sub-2"]`),
			AnalysisStatus: model.AnalysisStatusRunning,
		}
		require.NoError(t, db.Create(task).Error)

		result, err := repo.GetMasterTask(ctx, "master-1")
		require.NoError(t, err)
		assert.Equal(t, "master-1", result.TID)
		assert.Len(t, result.SubTIDs, 2)
	})

	t.Run("UpdateMasterTaskStatus_Success", func(t *testing.T) {
		err := repo.UpdateMasterTaskStatus(ctx, "master-1", model.AnalysisStatusCompleted)
		require.NoError(t, err)

		var updated MultipleTask
		require.NoError(t, db.First(&updated, "tid = ?", "master-1").Error)
		assert.Equal(t, model.AnalysisStatusCompleted, updated.AnalysisStatus)
		assert.NotNil(t, updated.EndTime)
	})

	t.Run("GetIncompleteSubTaskCount_Success", func(t *testing.T) {
		// Create sub-tasks
		subTask := &HotmethodTask{
			TID:            "sub-task-1",
			MasterTaskTID:  strPtr("master-1"),
			Status:         model.TaskStatusCompleted,
			AnalysisStatus: model.AnalysisStatusPending,
		}
		require.NoError(t, db.Create(subTask).Error)

		count, err := repo.GetIncompleteSubTaskCount(ctx, "master-1")
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

func strPtr(s string) *string {
	return &s
}
