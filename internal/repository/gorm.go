package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/perf-analysis/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormTaskRepository implements TaskRepository using GORM.
type GormTaskRepository struct {
	db *gorm.DB
}

// NewGormTaskRepository creates a new GormTaskRepository.
func NewGormTaskRepository(db *gorm.DB) *GormTaskRepository {
	return &GormTaskRepository{db: db}
}

// GetPendingTasks retrieves tasks that are pending analysis.
func (r *GormTaskRepository) GetPendingTasks(ctx context.Context, limit int) ([]*model.Task, error) {
	var tasks []HotmethodTask

	err := r.db.WithContext(ctx).
		Where("status = ? AND analysis_status = ?", model.TaskStatusCompleted, model.AnalysisStatusPending).
		Order("id DESC").
		Limit(limit).
		Find(&tasks).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query pending tasks: %w", err)
	}

	result := make([]*model.Task, len(tasks))
	for i, t := range tasks {
		result[i] = t.ToModel()
	}

	return result, nil
}

// GetTaskByID retrieves a task by its ID.
func (r *GormTaskRepository) GetTaskByID(ctx context.Context, id int64) (*model.Task, error) {
	var task HotmethodTask

	err := r.db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("task not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	return task.ToModel(), nil
}

// GetTaskByUUID retrieves a task by its UUID.
func (r *GormTaskRepository) GetTaskByUUID(ctx context.Context, uuid string) (*model.Task, error) {
	var task HotmethodTask

	err := r.db.WithContext(ctx).Where("tid = ?", uuid).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("task not found: %s", uuid)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	return task.ToModel(), nil
}

// UpdateAnalysisStatus updates the analysis status of a task.
func (r *GormTaskRepository) UpdateAnalysisStatus(ctx context.Context, id int64, status model.AnalysisStatus) error {
	result := r.db.WithContext(ctx).
		Model(&HotmethodTask{}).
		Where("id = ?", id).
		Update("analysis_status", status)

	if result.Error != nil {
		return fmt.Errorf("failed to update analysis status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("task not found: %d", id)
	}

	return nil
}

// UpdateAnalysisStatusWithInfo updates the analysis status with additional info.
func (r *GormTaskRepository) UpdateAnalysisStatusWithInfo(ctx context.Context, id int64, status model.AnalysisStatus, info string) error {
	result := r.db.WithContext(ctx).
		Model(&HotmethodTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"analysis_status": status,
			"status_info":     info,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update analysis status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("task not found: %d", id)
	}

	return nil
}

// LockTaskForAnalysis attempts to lock a task for analysis using FOR UPDATE.
func (r *GormTaskRepository) LockTaskForAnalysis(ctx context.Context, id int64) (bool, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task HotmethodTask

		// Try to lock the row with FOR UPDATE
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND analysis_status = ?", id, model.AnalysisStatusPending).
			First(&task).Error

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}

		// Update status to running
		return tx.Model(&HotmethodTask{}).
			Where("id = ?", id).
			Update("analysis_status", model.AnalysisStatusRunning).Error
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to lock task: %w", err)
	}

	return true, nil
}

// GormResultRepository implements ResultRepository using GORM.
type GormResultRepository struct {
	db      *gorm.DB
	version string
}

// NewGormResultRepository creates a new GormResultRepository.
func NewGormResultRepository(db *gorm.DB, version string) *GormResultRepository {
	return &GormResultRepository{db: db, version: version}
}

// SaveResult saves an analysis result to the database.
func (r *GormResultRepository) SaveResult(ctx context.Context, result *model.AnalysisResult) error {
	containersInfoJSON, err := json.Marshal(result.ContainersInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal containers info: %w", err)
	}

	resultJSON, err := json.Marshal(result.Result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	record := &GeneralAnalysisResult{
		TID:            result.TaskUUID,
		ContainersInfo: containersInfoJSON,
		Result:         resultJSON,
		Version:        r.version,
	}

	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("failed to save analysis result: %w", err)
	}

	return nil
}

// GetResultByTaskUUID retrieves the analysis result for a task.
func (r *GormResultRepository) GetResultByTaskUUID(ctx context.Context, taskUUID string) (*model.AnalysisResult, error) {
	var record GeneralAnalysisResult

	err := r.db.WithContext(ctx).Where("tid = ?", taskUUID).First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("result not found for task: %s", taskUUID)
		}
		return nil, fmt.Errorf("failed to get result: %w", err)
	}

	return record.ToModel()
}

// UpdateResult updates an existing analysis result.
func (r *GormResultRepository) UpdateResult(ctx context.Context, result *model.AnalysisResult) error {
	containersInfoJSON, err := json.Marshal(result.ContainersInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal containers info: %w", err)
	}

	resultJSON, err := json.Marshal(result.Result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	res := r.db.WithContext(ctx).
		Model(&GeneralAnalysisResult{}).
		Where("tid = ?", result.TaskUUID).
		Updates(map[string]interface{}{
			"containers_info": containersInfoJSON,
			"result":          resultJSON,
			"version":         r.version,
		})

	if res.Error != nil {
		return fmt.Errorf("failed to update result: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("result not found for task: %s", result.TaskUUID)
	}

	return nil
}

// GormSuggestionRepository implements SuggestionRepository using GORM.
type GormSuggestionRepository struct {
	db *gorm.DB
}

// NewGormSuggestionRepository creates a new GormSuggestionRepository.
func NewGormSuggestionRepository(db *gorm.DB) *GormSuggestionRepository {
	return &GormSuggestionRepository{db: db}
}

// SaveSuggestions saves multiple suggestions to the database.
func (r *GormSuggestionRepository) SaveSuggestions(ctx context.Context, suggestions []model.Suggestion) error {
	if len(suggestions) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		for _, sug := range suggestions {
			if sug.Suggestion == "" {
				continue
			}

			callStack := JSONField("{}")
			if sug.CallStack != nil {
				callStack = JSONField(sug.CallStack)
			}

			record := &AnalysisSuggestion{
				TID:          sug.TaskUUID,
				Namespace:    sug.Namespace,
				Suggestion:   sug.Suggestion,
				Func:         sug.FuncName,
				CallStack:    callStack,
				AISuggestion: sug.AISuggestion,
				CreatedAt:    now,
				UpdatedAt:    now,
			}

			if err := tx.Create(record).Error; err != nil {
				return fmt.Errorf("failed to insert suggestion: %w", err)
			}
		}

		return nil
	})
}

// GetSuggestionsByTaskUUID retrieves suggestions for a task.
func (r *GormSuggestionRepository) GetSuggestionsByTaskUUID(ctx context.Context, taskUUID string) ([]model.Suggestion, error) {
	var records []AnalysisSuggestion

	err := r.db.WithContext(ctx).Where("tid = ?", taskUUID).Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query suggestions: %w", err)
	}

	suggestions := make([]model.Suggestion, len(records))
	for i, rec := range records {
		suggestions[i] = rec.ToModel()
	}

	return suggestions, nil
}

// GetAnalysisRules retrieves all active analysis rules.
func (r *GormSuggestionRepository) GetAnalysisRules(ctx context.Context) ([]model.SuggestionRule, error) {
	var records []AnalysisSuggestionRule

	err := r.db.WithContext(ctx).Where("deleted IS NULL").Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query rules: %w", err)
	}

	rules := make([]model.SuggestionRule, len(records))
	for i, rec := range records {
		rules[i] = rec.ToModel()
	}

	return rules, nil
}

// GormMasterTaskRepository implements MasterTaskRepository using GORM.
type GormMasterTaskRepository struct {
	db *gorm.DB
}

// NewGormMasterTaskRepository creates a new GormMasterTaskRepository.
func NewGormMasterTaskRepository(db *gorm.DB) *GormMasterTaskRepository {
	return &GormMasterTaskRepository{db: db}
}

// GetMasterTask retrieves a master task by its UUID.
func (r *GormMasterTaskRepository) GetMasterTask(ctx context.Context, masterTID string) (*MasterTask, error) {
	var record MultipleTask

	err := r.db.WithContext(ctx).Where("tid = ?", masterTID).First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("master task not found: %s", masterTID)
		}
		return nil, fmt.Errorf("failed to get master task: %w", err)
	}

	return record.ToMasterTask()
}

// UpdateMasterTaskSuggestions updates the suggestions for a master task atomically.
func (r *GormMasterTaskRepository) UpdateMasterTaskSuggestions(ctx context.Context, masterTID string, resourceType string, suggestions *model.SuggestionGroup) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record MultipleTask

		// Lock the row for update
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tid = ?", masterTID).
			First(&record).Error
		if err != nil {
			return fmt.Errorf("failed to lock master task: %w", err)
		}

		// Parse existing suggestions
		existingSuggestions := model.NewMasterTaskSuggestions()
		if record.AnalysisSuggestions != nil {
			if err := json.Unmarshal(record.AnalysisSuggestions, existingSuggestions); err != nil {
				existingSuggestions = model.NewMasterTaskSuggestions()
			}
		}

		// Add new suggestions
		existingSuggestions.AddSuggestionGroup(resourceType, *suggestions)

		// Serialize and update
		newSuggestionsJSON, err := json.Marshal(existingSuggestions)
		if err != nil {
			return fmt.Errorf("failed to marshal suggestions: %w", err)
		}

		return tx.Model(&MultipleTask{}).
			Where("tid = ?", masterTID).
			Update("analysis_suggestions", newSuggestionsJSON).Error
	})
}

// UpdateMasterTaskStatus updates the analysis status of a master task.
func (r *GormMasterTaskRepository) UpdateMasterTaskStatus(ctx context.Context, masterTID string, status model.AnalysisStatus) error {
	updates := map[string]interface{}{
		"analysis_status": status,
	}

	if status == model.AnalysisStatusCompleted {
		updates["end_time"] = time.Now()
	}

	return r.db.WithContext(ctx).
		Model(&MultipleTask{}).
		Where("tid = ?", masterTID).
		Updates(updates).Error
}

// GetIncompleteSubTaskCount returns the count of incomplete sub-tasks.
func (r *GormMasterTaskRepository) GetIncompleteSubTaskCount(ctx context.Context, masterTID string) (int, error) {
	var count int64

	err := r.db.WithContext(ctx).
		Model(&HotmethodTask{}).
		Where("mastertask_tid = ? AND analysis_status <= 1 AND status != 3", masterTID).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count incomplete sub-tasks: %w", err)
	}

	return int(count), nil
}

// CheckAndCompleteIfReady checks if all sub-tasks are done and updates master task status.
func (r *GormMasterTaskRepository) CheckAndCompleteIfReady(ctx context.Context, masterTID string) error {
	count, err := r.GetIncompleteSubTaskCount(ctx, masterTID)
	if err != nil {
		return err
	}

	var newStatus model.AnalysisStatus
	if count == 0 {
		newStatus = model.AnalysisStatusCompleted
	} else {
		newStatus = model.AnalysisStatusRunning
	}

	return r.UpdateMasterTaskStatus(ctx, masterTID, newStatus)
}
