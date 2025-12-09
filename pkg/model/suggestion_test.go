package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSuggestionBuilder(t *testing.T) {
	suggestion := NewSuggestionBuilder().
		WithTaskUUID("task-123").
		WithNamespace("container-1").
		WithSuggestion("Consider optimizing this function").
		WithFunc("com.example.App.process").
		WithCallStack([]string{"frame1", "frame2"}).
		WithAISuggestion("AI analysis: ...").
		Build()

	assert.Equal(t, "task-123", suggestion.TaskUUID)
	assert.Equal(t, "container-1", suggestion.Namespace)
	assert.Equal(t, "Consider optimizing this function", suggestion.Suggestion)
	assert.Equal(t, "com.example.App.process", suggestion.FuncName)
	assert.Equal(t, "AI analysis: ...", suggestion.AISuggestion)
	assert.NotNil(t, suggestion.CallStack)
	assert.False(t, suggestion.CreatedAt.IsZero())
	assert.False(t, suggestion.UpdatedAt.IsZero())
}

func TestSuggestion_IsEmpty(t *testing.T) {
	tests := []struct {
		name       string
		suggestion Suggestion
		expected   bool
	}{
		{
			name:       "empty suggestion",
			suggestion: Suggestion{Suggestion: ""},
			expected:   true,
		},
		{
			name:       "non-empty suggestion",
			suggestion: Suggestion{Suggestion: "some text"},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.suggestion.IsEmpty())
		})
	}
}

func TestMasterTaskSuggestions(t *testing.T) {
	mts := NewMasterTaskSuggestions()

	cpuGroup := SuggestionGroup{
		Suggestion: []Suggestion{
			{Suggestion: "CPU suggestion 1"},
		},
	}
	memGroup := SuggestionGroup{
		Suggestion: []Suggestion{
			{Suggestion: "Memory suggestion 1"},
		},
	}

	mts.AddSuggestionGroup("CPU", cpuGroup)
	mts.AddSuggestionGroup("Memory", memGroup)

	assert.Len(t, mts.CPU, 1)
	assert.Len(t, mts.Memory, 1)
	assert.Len(t, mts.Disk, 0)
	assert.Len(t, mts.App, 0)
}

func TestSuggestion_JSONMarshal(t *testing.T) {
	suggestion := Suggestion{
		TaskUUID:   "task-123",
		Namespace:  "ns-1",
		Suggestion: "optimize this",
		FuncName:   "foo",
	}

	data, err := json.Marshal(suggestion)
	require.NoError(t, err)

	var decoded Suggestion
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, suggestion.TaskUUID, decoded.TaskUUID)
	assert.Equal(t, suggestion.Namespace, decoded.Namespace)
	assert.Equal(t, suggestion.Suggestion, decoded.Suggestion)
	assert.Equal(t, suggestion.FuncName, decoded.FuncName)
}

func TestSuggestionBuilder_WithCallStack_Nil(t *testing.T) {
	suggestion := NewSuggestionBuilder().
		WithCallStack(nil).
		Build()

	assert.Nil(t, suggestion.CallStack)
}

func TestSuggestionBuilder_WithCallStack_Map(t *testing.T) {
	callStack := map[string]interface{}{
		"depth":  3,
		"frames": []string{"a", "b", "c"},
	}

	suggestion := NewSuggestionBuilder().
		WithCallStack(callStack).
		Build()

	assert.NotNil(t, suggestion.CallStack)

	var decoded map[string]interface{}
	err := json.Unmarshal(suggestion.CallStack, &decoded)
	require.NoError(t, err)
	assert.Equal(t, float64(3), decoded["depth"])
}
