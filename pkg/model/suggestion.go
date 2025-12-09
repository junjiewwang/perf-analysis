package model

import (
	"encoding/json"
	"time"
)

// Suggestion represents an analysis suggestion.
type Suggestion struct {
	ID           int64           `json:"id,omitempty" db:"id"`
	TaskUUID     string          `json:"tid" db:"tid"`
	Namespace    string          `json:"namespace,omitempty" db:"namespace"`
	Type         string          `json:"type,omitempty"`
	Severity     string          `json:"severity,omitempty"`
	Suggestion   string          `json:"suggestion" db:"suggestion"`
	FuncName     string          `json:"func,omitempty" db:"func"`
	CallStack    json.RawMessage `json:"callstack,omitempty" db:"call_stack"`
	AISuggestion string          `json:"ai_suggestion,omitempty" db:"ai_suggestion"`
	CreatedAt    time.Time       `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at,omitempty" db:"updated_at"`
}

// SuggestionRule represents an analysis rule for generating suggestions.
type SuggestionRule struct {
	ID                int64   `json:"id" db:"id"`
	Type              string  `json:"type" db:"type"`
	Operation         string  `json:"operation" db:"operation"`
	Target            string  `json:"target" db:"target"`
	TargetType        string  `json:"target_type" db:"target_type"`
	Threshold         float64 `json:"threshold" db:"threshold"`
	SuggestionContent string  `json:"suggestion_content" db:"suggestion_content"`
}

// SuggestionBuilder helps build suggestions with a fluent interface.
type SuggestionBuilder struct {
	suggestion Suggestion
}

// NewSuggestionBuilder creates a new SuggestionBuilder.
func NewSuggestionBuilder() *SuggestionBuilder {
	return &SuggestionBuilder{
		suggestion: Suggestion{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
}

// WithTaskUUID sets the task UUID.
func (b *SuggestionBuilder) WithTaskUUID(taskUUID string) *SuggestionBuilder {
	b.suggestion.TaskUUID = taskUUID
	return b
}

// WithNamespace sets the namespace.
func (b *SuggestionBuilder) WithNamespace(namespace string) *SuggestionBuilder {
	b.suggestion.Namespace = namespace
	return b
}

// WithSuggestion sets the suggestion text.
func (b *SuggestionBuilder) WithSuggestion(text string) *SuggestionBuilder {
	b.suggestion.Suggestion = text
	return b
}

// WithFunc sets the function name.
func (b *SuggestionBuilder) WithFunc(funcName string) *SuggestionBuilder {
	b.suggestion.FuncName = funcName
	return b
}

// WithCallStack sets the call stack.
func (b *SuggestionBuilder) WithCallStack(callStack interface{}) *SuggestionBuilder {
	if callStack != nil {
		data, err := json.Marshal(callStack)
		if err == nil {
			b.suggestion.CallStack = data
		}
	}
	return b
}

// WithAISuggestion sets the AI suggestion.
func (b *SuggestionBuilder) WithAISuggestion(aiSuggestion string) *SuggestionBuilder {
	b.suggestion.AISuggestion = aiSuggestion
	return b
}

// Build returns the built Suggestion.
func (b *SuggestionBuilder) Build() Suggestion {
	return b.suggestion
}

// IsEmpty returns true if the suggestion text is empty.
func (s *Suggestion) IsEmpty() bool {
	return s.Suggestion == ""
}

// MasterTaskSuggestions holds suggestions grouped by resource type for master tasks.
type MasterTaskSuggestions struct {
	CPU    []SuggestionGroup `json:"CPU"`
	Memory []SuggestionGroup `json:"Memory"`
	Disk   []SuggestionGroup `json:"Disk"`
	App    []SuggestionGroup `json:"App"`
}

// SuggestionGroup represents a group of suggestions from a sub-task.
type SuggestionGroup struct {
	Suggestion []Suggestion `json:"suggestion"`
}

// NewMasterTaskSuggestions creates a new MasterTaskSuggestions instance.
func NewMasterTaskSuggestions() *MasterTaskSuggestions {
	return &MasterTaskSuggestions{
		CPU:    make([]SuggestionGroup, 0),
		Memory: make([]SuggestionGroup, 0),
		Disk:   make([]SuggestionGroup, 0),
		App:    make([]SuggestionGroup, 0),
	}
}

// AddSuggestionGroup adds a suggestion group to the appropriate resource type.
func (m *MasterTaskSuggestions) AddSuggestionGroup(resourceType string, group SuggestionGroup) {
	switch resourceType {
	case "CPU":
		m.CPU = append(m.CPU, group)
	case "Memory":
		m.Memory = append(m.Memory, group)
	case "Disk":
		m.Disk = append(m.Disk, group)
	case "App":
		m.App = append(m.App, group)
	}
}
