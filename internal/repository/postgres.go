package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/perf-analysis/pkg/model"
)

// PostgresTaskRepository implements TaskRepository for PostgreSQL.
type PostgresTaskRepository struct {
	db *sql.DB
}

// NewPostgresTaskRepository creates a new PostgresTaskRepository.
func NewPostgresTaskRepository(db *sql.DB) *PostgresTaskRepository {
	return &PostgresTaskRepository{db: db}
}

// GetPendingTasks retrieves tasks that are pending analysis.
func (r *PostgresTaskRepository) GetPendingTasks(ctx context.Context, limit int) ([]*model.Task, error) {
	query := `
		SELECT id, tid, type, profiler_type, status, analysis_status, 
			   COALESCE(status_info, ''), COALESCE(result_file, ''), 
			   COALESCE(user_name, ''), mastertask_tid, COALESCE(cos_bucket, ''),
			   request_params, create_time, begin_time, end_time
		FROM hotmethod_task 
		WHERE status = $1 AND analysis_status = $2 
		ORDER BY id DESC
		LIMIT $3
	`

	rows, err := r.db.QueryContext(ctx, query, model.TaskStatusCompleted, model.AnalysisStatusPending, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending tasks: %w", err)
	}
	defer rows.Close()

	return r.scanTasks(rows)
}

// GetTaskByID retrieves a task by its ID.
func (r *PostgresTaskRepository) GetTaskByID(ctx context.Context, id int64) (*model.Task, error) {
	query := `
		SELECT id, tid, type, profiler_type, status, analysis_status, 
			   COALESCE(status_info, ''), COALESCE(result_file, ''), 
			   COALESCE(user_name, ''), mastertask_tid, COALESCE(cos_bucket, ''),
			   request_params, create_time, begin_time, end_time
		FROM hotmethod_task 
		WHERE id = $1
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
func (r *PostgresTaskRepository) GetTaskByUUID(ctx context.Context, uuid string) (*model.Task, error) {
	query := `
		SELECT id, tid, type, profiler_type, status, analysis_status, 
			   COALESCE(status_info, ''), COALESCE(result_file, ''), 
			   COALESCE(user_name, ''), mastertask_tid, COALESCE(cos_bucket, ''),
			   request_params, create_time, begin_time, end_time
		FROM hotmethod_task 
		WHERE tid = $1
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
func (r *PostgresTaskRepository) UpdateAnalysisStatus(ctx context.Context, id int64, status model.AnalysisStatus) error {
	query := `UPDATE hotmethod_task SET analysis_status = $1 WHERE id = $2`
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
func (r *PostgresTaskRepository) UpdateAnalysisStatusWithInfo(ctx context.Context, id int64, status model.AnalysisStatus, info string) error {
	query := `UPDATE hotmethod_task SET analysis_status = $1, status_info = $2 WHERE id = $3`
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

// LockTaskForAnalysis attempts to lock a task for analysis using FOR UPDATE NOWAIT.
func (r *PostgresTaskRepository) LockTaskForAnalysis(ctx context.Context, id int64) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Try to lock the row with FOR UPDATE NOWAIT
	var analysisStatus model.AnalysisStatus
	query := `SELECT analysis_status FROM hotmethod_task WHERE id = $1 AND analysis_status = $2 FOR UPDATE NOWAIT`
	err = tx.QueryRowContext(ctx, query, id, model.AnalysisStatusPending).Scan(&analysisStatus)
	if err != nil {
		// Could not lock - either not found or already locked
		return false, nil
	}

	// Update status to running
	updateQuery := `UPDATE hotmethod_task SET analysis_status = $1 WHERE id = $2`
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
func (r *PostgresTaskRepository) scanTasks(rows *sql.Rows) ([]*model.Task, error) {
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

// PostgresResultRepository implements ResultRepository for PostgreSQL.
type PostgresResultRepository struct {
	db      *sql.DB
	version string
}

// NewPostgresResultRepository creates a new PostgresResultRepository.
func NewPostgresResultRepository(db *sql.DB, version string) *PostgresResultRepository {
	return &PostgresResultRepository{db: db, version: version}
}

// SaveResult saves an analysis result to the database.
func (r *PostgresResultRepository) SaveResult(ctx context.Context, result *model.AnalysisResult) error {
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
		VALUES ($1, $2, $3, $4)
	`

	_, err = r.db.ExecContext(ctx, query, result.TaskUUID, containersInfoJSON, resultJSON, r.version)
	if err != nil {
		return fmt.Errorf("failed to save analysis result: %w", err)
	}

	return nil
}

// GetResultByTaskUUID retrieves the analysis result for a task.
func (r *PostgresResultRepository) GetResultByTaskUUID(ctx context.Context, taskUUID string) (*model.AnalysisResult, error) {
	query := `
		SELECT tid, containers_info, result, version
		FROM general_analysis_results
		WHERE tid = $1
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
func (r *PostgresResultRepository) UpdateResult(ctx context.Context, result *model.AnalysisResult) error {
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
		SET containers_info = $1, result = $2, version = $3
		WHERE tid = $4
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

// PostgresSuggestionRepository implements SuggestionRepository for PostgreSQL.
type PostgresSuggestionRepository struct {
	db *sql.DB
}

// NewPostgresSuggestionRepository creates a new PostgresSuggestionRepository.
func NewPostgresSuggestionRepository(db *sql.DB) *PostgresSuggestionRepository {
	return &PostgresSuggestionRepository{db: db}
}

// SaveSuggestions saves multiple suggestions to the database.
func (r *PostgresSuggestionRepository) SaveSuggestions(ctx context.Context, suggestions []model.Suggestion) error {
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
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
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
func (r *PostgresSuggestionRepository) GetSuggestionsByTaskUUID(ctx context.Context, taskUUID string) ([]model.Suggestion, error) {
	query := `
		SELECT id, tid, COALESCE(namespace, ''), suggestion, COALESCE(func, ''), 
			   call_stack, COALESCE(ai_suggestion, ''), created_at, updated_at
		FROM analysis_suggestions
		WHERE tid = $1
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
func (r *PostgresSuggestionRepository) GetAnalysisRules(ctx context.Context) ([]model.SuggestionRule, error) {
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
