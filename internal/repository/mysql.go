package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/perf-analysis/pkg/model"
)

// MySQLTaskRepository implements TaskRepository for MySQL.
type MySQLTaskRepository struct {
	db *sql.DB
}

// NewMySQLTaskRepository creates a new MySQLTaskRepository.
func NewMySQLTaskRepository(db *sql.DB) *MySQLTaskRepository {
	return &MySQLTaskRepository{db: db}
}

// GetPendingTasks retrieves tasks that are pending analysis.
func (r *MySQLTaskRepository) GetPendingTasks(ctx context.Context, limit int) ([]*model.Task, error) {
	query := `
		SELECT id, tid, type, profiler_type, status, analysis_status, 
			   COALESCE(status_info, ''), COALESCE(result_file, ''), 
			   COALESCE(user_name, ''), mastertask_tid, COALESCE(cos_bucket, ''),
			   request_params, create_time, begin_time, end_time
		FROM hotmethod_task 
		WHERE status = ? AND analysis_status = ? 
		ORDER BY id DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, model.TaskStatusCompleted, model.AnalysisStatusPending, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending tasks: %w", err)
	}
	defer rows.Close()

	return r.scanTasks(rows)
}

// GetTaskByID retrieves a task by its ID.
func (r *MySQLTaskRepository) GetTaskByID(ctx context.Context, id int64) (*model.Task, error) {
	query := `
		SELECT id, tid, type, profiler_type, status, analysis_status, 
			   COALESCE(status_info, ''), COALESCE(result_file, ''), 
			   COALESCE(user_name, ''), mastertask_tid, COALESCE(cos_bucket, ''),
			   request_params, create_time, begin_time, end_time
		FROM hotmethod_task 
		WHERE id = ?
	`

	task := &model.Task{}
	var requestParamsJSON []byte
	var masterTaskTID sql.NullString
	var beginTime, endTime sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&task.ID, &task.TaskUUID, &task.Type, &task.ProfilerType,
		&task.Status, &task.AnalysisStatus, &task.StatusInfo, &task.ResultFile,
		&task.UserName, &masterTaskTID, &task.COSBucket,
		&requestParamsJSON, &task.CreateTime, &beginTime, &endTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	if masterTaskTID.Valid {
		task.MasterTaskTID = &masterTaskTID.String
	}
	if beginTime.Valid {
		task.BeginTime = &beginTime.Time
	}
	if endTime.Valid {
		task.EndTime = &endTime.Time
	}

	if requestParamsJSON != nil {
		if err := json.Unmarshal(requestParamsJSON, &task.RequestParams); err != nil {
			return nil, fmt.Errorf("failed to parse request params: %w", err)
		}
	}

	return task, nil
}

// GetTaskByUUID retrieves a task by its UUID.
func (r *MySQLTaskRepository) GetTaskByUUID(ctx context.Context, uuid string) (*model.Task, error) {
	query := `
		SELECT id, tid, type, profiler_type, status, analysis_status, 
			   COALESCE(status_info, ''), COALESCE(result_file, ''), 
			   COALESCE(user_name, ''), mastertask_tid, COALESCE(cos_bucket, ''),
			   request_params, create_time, begin_time, end_time
		FROM hotmethod_task 
		WHERE tid = ?
	`

	task := &model.Task{}
	var requestParamsJSON []byte
	var masterTaskTID sql.NullString
	var beginTime, endTime sql.NullTime

	err := r.db.QueryRowContext(ctx, query, uuid).Scan(
		&task.ID, &task.TaskUUID, &task.Type, &task.ProfilerType,
		&task.Status, &task.AnalysisStatus, &task.StatusInfo, &task.ResultFile,
		&task.UserName, &masterTaskTID, &task.COSBucket,
		&requestParamsJSON, &task.CreateTime, &beginTime, &endTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found: %s", uuid)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	if masterTaskTID.Valid {
		task.MasterTaskTID = &masterTaskTID.String
	}
	if beginTime.Valid {
		task.BeginTime = &beginTime.Time
	}
	if endTime.Valid {
		task.EndTime = &endTime.Time
	}

	if requestParamsJSON != nil {
		if err := json.Unmarshal(requestParamsJSON, &task.RequestParams); err != nil {
			return nil, fmt.Errorf("failed to parse request params: %w", err)
		}
	}

	return task, nil
}

// UpdateAnalysisStatus updates the analysis status of a task.
func (r *MySQLTaskRepository) UpdateAnalysisStatus(ctx context.Context, id int64, status model.AnalysisStatus) error {
	query := `UPDATE hotmethod_task SET analysis_status = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update analysis status: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("task not found: %d", id)
	}

	return nil
}

// UpdateAnalysisStatusWithInfo updates the analysis status with additional info.
func (r *MySQLTaskRepository) UpdateAnalysisStatusWithInfo(ctx context.Context, id int64, status model.AnalysisStatus, info string) error {
	query := `UPDATE hotmethod_task SET analysis_status = ?, status_info = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, status, info, id)
	if err != nil {
		return fmt.Errorf("failed to update analysis status: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("task not found: %d", id)
	}

	return nil
}

// LockTaskForAnalysis attempts to lock a task for analysis using FOR UPDATE.
func (r *MySQLTaskRepository) LockTaskForAnalysis(ctx context.Context, id int64) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Try to lock the row with FOR UPDATE NOWAIT (MySQL 8.0+)
	// For older MySQL versions, use regular FOR UPDATE with a timeout
	var analysisStatus model.AnalysisStatus
	query := `SELECT analysis_status FROM hotmethod_task WHERE id = ? AND analysis_status = ? FOR UPDATE`
	err = tx.QueryRowContext(ctx, query, id, model.AnalysisStatusPending).Scan(&analysisStatus)
	if err != nil {
		if err == sql.ErrNoRows || strings.Contains(err.Error(), "lock wait timeout") {
			return false, nil
		}
		return false, fmt.Errorf("failed to lock task: %w", err)
	}

	// Update status to running
	updateQuery := `UPDATE hotmethod_task SET analysis_status = ? WHERE id = ?`
	_, err = tx.ExecContext(ctx, updateQuery, model.AnalysisStatusRunning, id)
	if err != nil {
		return false, fmt.Errorf("failed to update status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return true, nil
}

// scanTasks scans multiple tasks from rows.
func (r *MySQLTaskRepository) scanTasks(rows *sql.Rows) ([]*model.Task, error) {
	var tasks []*model.Task

	for rows.Next() {
		task := &model.Task{}
		var requestParamsJSON []byte
		var masterTaskTID sql.NullString
		var beginTime, endTime sql.NullTime

		err := rows.Scan(
			&task.ID, &task.TaskUUID, &task.Type, &task.ProfilerType,
			&task.Status, &task.AnalysisStatus, &task.StatusInfo, &task.ResultFile,
			&task.UserName, &masterTaskTID, &task.COSBucket,
			&requestParamsJSON, &task.CreateTime, &beginTime, &endTime,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task row: %w", err)
		}

		if masterTaskTID.Valid {
			task.MasterTaskTID = &masterTaskTID.String
		}
		if beginTime.Valid {
			task.BeginTime = &beginTime.Time
		}
		if endTime.Valid {
			task.EndTime = &endTime.Time
		}

		if requestParamsJSON != nil {
			if err := json.Unmarshal(requestParamsJSON, &task.RequestParams); err != nil {
				return nil, fmt.Errorf("failed to parse request params: %w", err)
			}
		}

		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return tasks, nil
}

// MySQLResultRepository implements ResultRepository for MySQL.
type MySQLResultRepository struct {
	db      *sql.DB
	version string
}

// NewMySQLResultRepository creates a new MySQLResultRepository.
func NewMySQLResultRepository(db *sql.DB, version string) *MySQLResultRepository {
	return &MySQLResultRepository{db: db, version: version}
}

// SaveResult saves an analysis result to the database.
func (r *MySQLResultRepository) SaveResult(ctx context.Context, result *model.AnalysisResult) error {
	containersInfoJSON, err := json.Marshal(result.ContainersInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal containers info: %w", err)
	}

	resultJSON, err := json.Marshal(result.Result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	query := `
		INSERT INTO general_analysis_results (tid, containers_info, result, version)
		VALUES (?, ?, ?, ?)
	`

	_, err = r.db.ExecContext(ctx, query, result.TaskUUID, containersInfoJSON, resultJSON, r.version)
	if err != nil {
		return fmt.Errorf("failed to save analysis result: %w", err)
	}

	return nil
}

// GetResultByTaskUUID retrieves the analysis result for a task.
func (r *MySQLResultRepository) GetResultByTaskUUID(ctx context.Context, taskUUID string) (*model.AnalysisResult, error) {
	query := `
		SELECT tid, containers_info, result, version
		FROM general_analysis_results
		WHERE tid = ?
	`

	var containersInfoJSON, resultJSON []byte
	result := &model.AnalysisResult{}

	err := r.db.QueryRowContext(ctx, query, taskUUID).Scan(
		&result.TaskUUID, &containersInfoJSON, &resultJSON, &result.Version,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("result not found for task: %s", taskUUID)
		}
		return nil, fmt.Errorf("failed to get result: %w", err)
	}

	if containersInfoJSON != nil {
		if err := json.Unmarshal(containersInfoJSON, &result.ContainersInfo); err != nil {
			return nil, fmt.Errorf("failed to unmarshal containers info: %w", err)
		}
	}

	if resultJSON != nil {
		if err := json.Unmarshal(resultJSON, &result.Result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}

	return result, nil
}

// UpdateResult updates an existing analysis result.
func (r *MySQLResultRepository) UpdateResult(ctx context.Context, result *model.AnalysisResult) error {
	containersInfoJSON, err := json.Marshal(result.ContainersInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal containers info: %w", err)
	}

	resultJSON, err := json.Marshal(result.Result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	query := `
		UPDATE general_analysis_results 
		SET containers_info = ?, result = ?, version = ?
		WHERE tid = ?
	`

	res, err := r.db.ExecContext(ctx, query, containersInfoJSON, resultJSON, r.version, result.TaskUUID)
	if err != nil {
		return fmt.Errorf("failed to update result: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("result not found for task: %s", result.TaskUUID)
	}

	return nil
}

// MySQLSuggestionRepository implements SuggestionRepository for MySQL.
type MySQLSuggestionRepository struct {
	db *sql.DB
}

// NewMySQLSuggestionRepository creates a new MySQLSuggestionRepository.
func NewMySQLSuggestionRepository(db *sql.DB) *MySQLSuggestionRepository {
	return &MySQLSuggestionRepository{db: db}
}

// SaveSuggestions saves multiple suggestions to the database.
func (r *MySQLSuggestionRepository) SaveSuggestions(ctx context.Context, suggestions []model.Suggestion) error {
	if len(suggestions) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO analysis_suggestions (tid, namespace, suggestion, func, call_stack, created_at, updated_at, ai_suggestion)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	for _, sug := range suggestions {
		if sug.Suggestion == "" {
			continue
		}

		callStackJSON := "{}"
		if sug.CallStack != nil {
			callStackJSON = string(sug.CallStack)
		}

		_, err := tx.ExecContext(ctx, query,
			sug.TaskUUID, sug.Namespace, sug.Suggestion, sug.FuncName,
			callStackJSON, now, now, sug.AISuggestion,
		)
		if err != nil {
			return fmt.Errorf("failed to insert suggestion: %w", err)
		}
	}

	return tx.Commit()
}

// GetSuggestionsByTaskUUID retrieves suggestions for a task.
func (r *MySQLSuggestionRepository) GetSuggestionsByTaskUUID(ctx context.Context, taskUUID string) ([]model.Suggestion, error) {
	query := `
		SELECT id, tid, COALESCE(namespace, ''), suggestion, COALESCE(func, ''), 
			   call_stack, COALESCE(ai_suggestion, ''), created_at, updated_at
		FROM analysis_suggestions
		WHERE tid = ?
	`

	rows, err := r.db.QueryContext(ctx, query, taskUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to query suggestions: %w", err)
	}
	defer rows.Close()

	var suggestions []model.Suggestion
	for rows.Next() {
		var sug model.Suggestion
		var callStackJSON []byte

		err := rows.Scan(
			&sug.ID, &sug.TaskUUID, &sug.Namespace, &sug.Suggestion, &sug.FuncName,
			&callStackJSON, &sug.AISuggestion, &sug.CreatedAt, &sug.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan suggestion: %w", err)
		}

		sug.CallStack = callStackJSON
		suggestions = append(suggestions, sug)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return suggestions, nil
}

// GetAnalysisRules retrieves all active analysis rules.
func (r *MySQLSuggestionRepository) GetAnalysisRules(ctx context.Context) ([]model.SuggestionRule, error) {
	query := `
		SELECT id, type, operation, target, target_type, threshold, suggestion_content
		FROM analysis_suggestion_rules
		WHERE deleted IS NULL
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query rules: %w", err)
	}
	defer rows.Close()

	var rules []model.SuggestionRule
	for rows.Next() {
		var rule model.SuggestionRule
		err := rows.Scan(
			&rule.ID, &rule.Type, &rule.Operation, &rule.Target,
			&rule.TargetType, &rule.Threshold, &rule.SuggestionContent,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan rule: %w", err)
		}
		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return rules, nil
}
