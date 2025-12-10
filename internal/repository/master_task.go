package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/perf-analysis/pkg/model"
)

// PostgresMasterTaskRepository implements MasterTaskRepository for PostgreSQL.
type PostgresMasterTaskRepository struct {
	db *sql.DB
}

// NewPostgresMasterTaskRepository creates a new PostgresMasterTaskRepository.
func NewPostgresMasterTaskRepository(db *sql.DB) *PostgresMasterTaskRepository {
	return &PostgresMasterTaskRepository{db: db}
}

// GetMasterTask retrieves a master task by its UUID.
func (r *PostgresMasterTaskRepository) GetMasterTask(ctx context.Context, masterTID string) (*MasterTask, error) {
	query := `
		SELECT tid, sub_tids, analysis_suggestions, analysis_status
		FROM multiple_task
		WHERE tid = $1
	`

	var subTIDsJSON, suggestionsJSON []byte
	task := &MasterTask{}

	err := r.db.QueryRowContext(ctx, query, masterTID).Scan(
		&task.TID, &subTIDsJSON, &suggestionsJSON, &task.AnalysisStatus,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("master task not found: %s", masterTID)
		}
		return nil, fmt.Errorf("failed to get master task: %w", err)
	}

	if subTIDsJSON != nil {
		if err := json.Unmarshal(subTIDsJSON, &task.SubTIDs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal sub_tids: %w", err)
		}
	}

	if suggestionsJSON != nil {
		task.AnalysisSuggestions = model.NewMasterTaskSuggestions()
		if err := json.Unmarshal(suggestionsJSON, task.AnalysisSuggestions); err != nil {
			return nil, fmt.Errorf("failed to unmarshal analysis_suggestions: %w", err)
		}
	}

	return task, nil
}

// UpdateMasterTaskSuggestions updates the suggestions for a master task atomically.
func (r *PostgresMasterTaskRepository) UpdateMasterTaskSuggestions(ctx context.Context, masterTID string, resourceType string, suggestions *model.SuggestionGroup) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock the row for update
	var existingSuggestionsJSON []byte
	query := `SELECT analysis_suggestions FROM multiple_task WHERE tid = $1 FOR UPDATE`
	err = tx.QueryRowContext(ctx, query, masterTID).Scan(&existingSuggestionsJSON)
	if err != nil {
		return fmt.Errorf("failed to lock master task: %w", err)
	}

	// Parse existing suggestions
	var existingSuggestions *model.MasterTaskSuggestions
	if existingSuggestionsJSON != nil {
		existingSuggestions = model.NewMasterTaskSuggestions()
		if err := json.Unmarshal(existingSuggestionsJSON, existingSuggestions); err != nil {
			existingSuggestions = model.NewMasterTaskSuggestions()
		}
	} else {
		existingSuggestions = model.NewMasterTaskSuggestions()
	}

	// Add new suggestions
	existingSuggestions.AddSuggestionGroup(resourceType, *suggestions)

	// Serialize and update
	newSuggestionsJSON, err := json.Marshal(existingSuggestions)
	if err != nil {
		return fmt.Errorf("failed to marshal suggestions: %w", err)
	}

	updateQuery := `UPDATE multiple_task SET analysis_suggestions = $1 WHERE tid = $2`
	_, err = tx.ExecContext(ctx, updateQuery, newSuggestionsJSON, masterTID)
	if err != nil {
		return fmt.Errorf("failed to update suggestions: %w", err)
	}

	return tx.Commit()
}

// UpdateMasterTaskStatus updates the analysis status of a master task.
func (r *PostgresMasterTaskRepository) UpdateMasterTaskStatus(ctx context.Context, masterTID string, status model.AnalysisStatus) error {
	query := `UPDATE multiple_task SET analysis_status = $1 WHERE tid = $2`
	if status == model.AnalysisStatusCompleted {
		query = `UPDATE multiple_task SET analysis_status = $1, end_time = $2 WHERE tid = $3`
		_, err := r.db.ExecContext(ctx, query, status, time.Now(), masterTID)
		return err
	}

	_, err := r.db.ExecContext(ctx, query, status, masterTID)
	return err
}

// GetIncompleteSubTaskCount returns the count of incomplete sub-tasks.
func (r *PostgresMasterTaskRepository) GetIncompleteSubTaskCount(ctx context.Context, masterTID string) (int, error) {
	query := `
		SELECT COUNT(*) 
		FROM hotmethod_task 
		WHERE mastertask_tid = $1 AND analysis_status <= 1 AND status != 3
	`

	var count int
	err := r.db.QueryRowContext(ctx, query, masterTID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count incomplete sub-tasks: %w", err)
	}

	return count, nil
}

// CheckAndCompleteIfReady checks if all sub-tasks are done and updates master task status.
func (r *PostgresMasterTaskRepository) CheckAndCompleteIfReady(ctx context.Context, masterTID string) error {
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

// MySQLMasterTaskRepository implements MasterTaskRepository for MySQL.
type MySQLMasterTaskRepository struct {
	db *sql.DB
}

// NewMySQLMasterTaskRepository creates a new MySQLMasterTaskRepository.
func NewMySQLMasterTaskRepository(db *sql.DB) *MySQLMasterTaskRepository {
	return &MySQLMasterTaskRepository{db: db}
}

// GetMasterTask retrieves a master task by its UUID.
func (r *MySQLMasterTaskRepository) GetMasterTask(ctx context.Context, masterTID string) (*MasterTask, error) {
	query := `
		SELECT tid, sub_tids, analysis_suggestions, analysis_status
		FROM multiple_task
		WHERE tid = ?
	`

	var subTIDsJSON, suggestionsJSON []byte
	task := &MasterTask{}

	err := r.db.QueryRowContext(ctx, query, masterTID).Scan(
		&task.TID, &subTIDsJSON, &suggestionsJSON, &task.AnalysisStatus,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("master task not found: %s", masterTID)
		}
		return nil, fmt.Errorf("failed to get master task: %w", err)
	}

	if subTIDsJSON != nil {
		if err := json.Unmarshal(subTIDsJSON, &task.SubTIDs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal sub_tids: %w", err)
		}
	}

	if suggestionsJSON != nil {
		task.AnalysisSuggestions = model.NewMasterTaskSuggestions()
		if err := json.Unmarshal(suggestionsJSON, task.AnalysisSuggestions); err != nil {
			return nil, fmt.Errorf("failed to unmarshal analysis_suggestions: %w", err)
		}
	}

	return task, nil
}

// UpdateMasterTaskSuggestions updates the suggestions for a master task atomically.
func (r *MySQLMasterTaskRepository) UpdateMasterTaskSuggestions(ctx context.Context, masterTID string, resourceType string, suggestions *model.SuggestionGroup) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock the row for update
	var existingSuggestionsJSON []byte
	query := `SELECT analysis_suggestions FROM multiple_task WHERE tid = ? FOR UPDATE`
	err = tx.QueryRowContext(ctx, query, masterTID).Scan(&existingSuggestionsJSON)
	if err != nil {
		return fmt.Errorf("failed to lock master task: %w", err)
	}

	// Parse existing suggestions
	var existingSuggestions *model.MasterTaskSuggestions
	if existingSuggestionsJSON != nil {
		existingSuggestions = model.NewMasterTaskSuggestions()
		if err := json.Unmarshal(existingSuggestionsJSON, existingSuggestions); err != nil {
			existingSuggestions = model.NewMasterTaskSuggestions()
		}
	} else {
		existingSuggestions = model.NewMasterTaskSuggestions()
	}

	// Add new suggestions
	existingSuggestions.AddSuggestionGroup(resourceType, *suggestions)

	// Serialize and update
	newSuggestionsJSON, err := json.Marshal(existingSuggestions)
	if err != nil {
		return fmt.Errorf("failed to marshal suggestions: %w", err)
	}

	updateQuery := `UPDATE multiple_task SET analysis_suggestions = ? WHERE tid = ?`
	_, err = tx.ExecContext(ctx, updateQuery, newSuggestionsJSON, masterTID)
	if err != nil {
		return fmt.Errorf("failed to update suggestions: %w", err)
	}

	return tx.Commit()
}

// UpdateMasterTaskStatus updates the analysis status of a master task.
func (r *MySQLMasterTaskRepository) UpdateMasterTaskStatus(ctx context.Context, masterTID string, status model.AnalysisStatus) error {
	query := `UPDATE multiple_task SET analysis_status = ? WHERE tid = ?`
	if status == model.AnalysisStatusCompleted {
		query = `UPDATE multiple_task SET analysis_status = ?, end_time = ? WHERE tid = ?`
		_, err := r.db.ExecContext(ctx, query, status, time.Now(), masterTID)
		return err
	}

	_, err := r.db.ExecContext(ctx, query, status, masterTID)
	return err
}

// GetIncompleteSubTaskCount returns the count of incomplete sub-tasks.
func (r *MySQLMasterTaskRepository) GetIncompleteSubTaskCount(ctx context.Context, masterTID string) (int, error) {
	query := `
		SELECT COUNT(*) 
		FROM hotmethod_task 
		WHERE mastertask_tid = ? AND analysis_status <= 1 AND status != 3
	`

	var count int
	err := r.db.QueryRowContext(ctx, query, masterTID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count incomplete sub-tasks: %w", err)
	}

	return count, nil
}

// CheckAndCompleteIfReady checks if all sub-tasks are done and updates master task status.
func (r *MySQLMasterTaskRepository) CheckAndCompleteIfReady(ctx context.Context, masterTID string) error {
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
