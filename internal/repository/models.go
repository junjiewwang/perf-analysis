// Package repository provides database abstraction for the perf-analysis service.
package repository

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/perf-analysis/pkg/model"
)

// HotmethodTask represents the hotmethod_task table.
type HotmethodTask struct {
	ID             int64                `gorm:"column:id;primaryKey;autoIncrement"`
	TID            string               `gorm:"column:tid;type:varchar(64);uniqueIndex"`
	Type           model.TaskType       `gorm:"column:type"`
	ProfilerType   model.ProfilerType   `gorm:"column:profiler_type"`
	Status         model.TaskStatus     `gorm:"column:status"`
	AnalysisStatus model.AnalysisStatus `gorm:"column:analysis_status"`
	StatusInfo     string               `gorm:"column:status_info;type:text"`
	ResultFile     string               `gorm:"column:result_file;type:varchar(512)"`
	UserName       string               `gorm:"column:user_name;type:varchar(128)"`
	MasterTaskTID  *string              `gorm:"column:mastertask_tid;type:varchar(64)"`
	COSBucket      string               `gorm:"column:cos_bucket;type:varchar(128)"`
	RequestParams  JSONField            `gorm:"column:request_params;type:json"`
	CreateTime     time.Time            `gorm:"column:create_time;autoCreateTime"`
	BeginTime      *time.Time           `gorm:"column:begin_time"`
	EndTime        *time.Time           `gorm:"column:end_time"`
}

// TableName returns the table name for HotmethodTask.
func (HotmethodTask) TableName() string {
	return "hotmethod_task"
}

// ToModel converts HotmethodTask to model.Task.
func (t *HotmethodTask) ToModel() *model.Task {
	task := &model.Task{
		ID:             t.ID,
		TaskUUID:       t.TID,
		Type:           t.Type,
		ProfilerType:   t.ProfilerType,
		Status:         t.Status,
		AnalysisStatus: t.AnalysisStatus,
		StatusInfo:     t.StatusInfo,
		ResultFile:     t.ResultFile,
		UserName:       t.UserName,
		MasterTaskTID:  t.MasterTaskTID,
		COSBucket:      t.COSBucket,
		CreateTime:     t.CreateTime,
		BeginTime:      t.BeginTime,
		EndTime:        t.EndTime,
	}

	if t.RequestParams != nil {
		_ = json.Unmarshal(t.RequestParams, &task.RequestParams)
	}

	return task
}

// GeneralAnalysisResult represents the general_analysis_results table.
type GeneralAnalysisResult struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	TID            string    `gorm:"column:tid;type:varchar(64);uniqueIndex"`
	ContainersInfo JSONField `gorm:"column:containers_info;type:json"`
	Result         JSONField `gorm:"column:result;type:json"`
	Version        string    `gorm:"column:version;type:varchar(32)"`
}

// TableName returns the table name for GeneralAnalysisResult.
func (GeneralAnalysisResult) TableName() string {
	return "general_analysis_results"
}

// ToModel converts GeneralAnalysisResult to model.AnalysisResult.
func (r *GeneralAnalysisResult) ToModel() (*model.AnalysisResult, error) {
	result := &model.AnalysisResult{
		TaskUUID: r.TID,
		Version:  r.Version,
	}

	if r.ContainersInfo != nil {
		if err := json.Unmarshal(r.ContainersInfo, &result.ContainersInfo); err != nil {
			return nil, err
		}
	}

	if r.Result != nil {
		if err := json.Unmarshal(r.Result, &result.Result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// AnalysisSuggestion represents the analysis_suggestions table.
type AnalysisSuggestion struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	TID          string    `gorm:"column:tid;type:varchar(64);index"`
	Namespace    string    `gorm:"column:namespace;type:varchar(256)"`
	Suggestion   string    `gorm:"column:suggestion;type:text"`
	Func         string    `gorm:"column:func;type:varchar(512)"`
	CallStack    JSONField `gorm:"column:call_stack;type:json"`
	AISuggestion string    `gorm:"column:ai_suggestion;type:text"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName returns the table name for AnalysisSuggestion.
func (AnalysisSuggestion) TableName() string {
	return "analysis_suggestions"
}

// ToModel converts AnalysisSuggestion to model.Suggestion.
func (s *AnalysisSuggestion) ToModel() model.Suggestion {
	return model.Suggestion{
		ID:           s.ID,
		TaskUUID:     s.TID,
		Namespace:    s.Namespace,
		Suggestion:   s.Suggestion,
		FuncName:     s.Func,
		CallStack:    json.RawMessage(s.CallStack),
		AISuggestion: s.AISuggestion,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
	}
}

// AnalysisSuggestionRule represents the analysis_suggestion_rules table.
type AnalysisSuggestionRule struct {
	ID                int64    `gorm:"column:id;primaryKey;autoIncrement"`
	Type              string   `gorm:"column:type;type:varchar(64)"`
	Operation         string   `gorm:"column:operation;type:varchar(64)"`
	Target            string   `gorm:"column:target;type:varchar(512)"`
	TargetType        string   `gorm:"column:target_type;type:varchar(64)"`
	Threshold         float64  `gorm:"column:threshold"`
	SuggestionContent string   `gorm:"column:suggestion_content;type:text"`
	Deleted           *int64   `gorm:"column:deleted"`
}

// TableName returns the table name for AnalysisSuggestionRule.
func (AnalysisSuggestionRule) TableName() string {
	return "analysis_suggestion_rules"
}

// ToModel converts AnalysisSuggestionRule to model.SuggestionRule.
func (r *AnalysisSuggestionRule) ToModel() model.SuggestionRule {
	return model.SuggestionRule{
		ID:                r.ID,
		Type:              r.Type,
		Operation:         r.Operation,
		Target:            r.Target,
		TargetType:        r.TargetType,
		Threshold:         r.Threshold,
		SuggestionContent: r.SuggestionContent,
	}
}

// MultipleTask represents the multiple_task table for master tasks.
type MultipleTask struct {
	TID                 string               `gorm:"column:tid;type:varchar(64);primaryKey"`
	SubTIDs             JSONField            `gorm:"column:sub_tids;type:json"`
	AnalysisSuggestions JSONField            `gorm:"column:analysis_suggestions;type:json"`
	AnalysisStatus      model.AnalysisStatus `gorm:"column:analysis_status"`
	EndTime             *time.Time           `gorm:"column:end_time"`
}

// TableName returns the table name for MultipleTask.
func (MultipleTask) TableName() string {
	return "multiple_task"
}

// ToMasterTask converts MultipleTask to MasterTask.
func (m *MultipleTask) ToMasterTask() (*MasterTask, error) {
	task := &MasterTask{
		TID:            m.TID,
		AnalysisStatus: m.AnalysisStatus,
	}

	if m.SubTIDs != nil {
		if err := json.Unmarshal(m.SubTIDs, &task.SubTIDs); err != nil {
			return nil, err
		}
	}

	if m.AnalysisSuggestions != nil {
		task.AnalysisSuggestions = model.NewMasterTaskSuggestions()
		if err := json.Unmarshal(m.AnalysisSuggestions, task.AnalysisSuggestions); err != nil {
			task.AnalysisSuggestions = model.NewMasterTaskSuggestions()
		}
	}

	return task, nil
}

// JSONField is a custom type for handling JSON fields in GORM.
type JSONField []byte

// Value implements driver.Valuer interface.
func (j JSONField) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return []byte(j), nil
}

// Scan implements sql.Scanner interface.
func (j *JSONField) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	switch v := value.(type) {
	case []byte:
		*j = append((*j)[0:0], v...)
		return nil
	case string:
		*j = []byte(v)
		return nil
	default:
		return errors.New("unsupported type for JSONField")
	}
}

// MarshalJSON implements json.Marshaler interface.
func (j JSONField) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return j, nil
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (j *JSONField) UnmarshalJSON(data []byte) error {
	if data == nil || string(data) == "null" {
		*j = nil
		return nil
	}
	*j = append((*j)[0:0], data...)
	return nil
}
