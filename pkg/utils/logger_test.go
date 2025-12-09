package utils

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"warn", LevelWarn},
		{"WARN", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"unknown", LevelInfo}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, ParseLogLevel(tt.input))
		})
	}
}

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LogLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.level.String())
		})
	}
}

func TestDefaultLogger_LogLevels(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewDefaultLogger(LevelDebug, buf)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()
	assert.Contains(t, output, "[DEBUG]")
	assert.Contains(t, output, "[INFO]")
	assert.Contains(t, output, "[WARN]")
	assert.Contains(t, output, "[ERROR]")
	assert.Contains(t, output, "debug message")
	assert.Contains(t, output, "info message")
	assert.Contains(t, output, "warn message")
	assert.Contains(t, output, "error message")
}

func TestDefaultLogger_FilterByLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewDefaultLogger(LevelWarn, buf)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()
	assert.NotContains(t, output, "debug message")
	assert.NotContains(t, output, "info message")
	assert.Contains(t, output, "warn message")
	assert.Contains(t, output, "error message")
}

func TestDefaultLogger_WithField(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewDefaultLogger(LevelInfo, buf)

	loggerWithField := logger.WithField("task_id", "123")
	loggerWithField.Info("processing task")

	output := buf.String()
	assert.Contains(t, output, "task_id=123")
	assert.Contains(t, output, "processing task")
}

func TestDefaultLogger_WithFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewDefaultLogger(LevelInfo, buf)

	fields := map[string]interface{}{
		"task_id": "123",
		"user":    "admin",
	}
	loggerWithFields := logger.WithFields(fields)
	loggerWithFields.Info("processing")

	output := buf.String()
	assert.Contains(t, output, "task_id=123")
	assert.Contains(t, output, "user=admin")
}

func TestDefaultLogger_Formatting(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewDefaultLogger(LevelInfo, buf)

	logger.Info("count: %d, name: %s", 42, "test")

	output := buf.String()
	assert.Contains(t, output, "count: 42, name: test")
}

func TestDefaultLogger_SetLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewDefaultLogger(LevelInfo, buf)

	// Initially at Info level
	logger.Debug("debug 1")
	assert.NotContains(t, buf.String(), "debug 1")

	// Change to Debug level
	logger.SetLevel(LevelDebug)
	logger.Debug("debug 2")
	assert.Contains(t, buf.String(), "debug 2")
}

func TestNullLogger(t *testing.T) {
	logger := &NullLogger{}

	// These should not panic
	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")

	// WithField should return the same logger
	result := logger.WithField("key", "value")
	assert.Equal(t, logger, result)

	// WithFields should return the same logger
	result = logger.WithFields(map[string]interface{}{"key": "value"})
	assert.Equal(t, logger, result)
}

func TestStdLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewStdLogger(LevelInfo, buf)

	logger.Info("info message")

	output := buf.String()
	assert.Contains(t, output, "[INFO]")
	assert.Contains(t, output, "info message")
}

func TestGlobalLogger(t *testing.T) {
	// Save original
	original := globalLogger

	// Set a new global logger
	buf := &bytes.Buffer{}
	newLogger := NewDefaultLogger(LevelInfo, buf)
	SetGlobalLogger(newLogger)

	// Get and use global logger
	logger := GetGlobalLogger()
	logger.Info("global log")

	assert.Contains(t, buf.String(), "global log")

	// Restore original
	SetGlobalLogger(original)
}

func TestLoggerInterface(t *testing.T) {
	// Verify all implementations satisfy the Logger interface
	var _ Logger = &DefaultLogger{}
	var _ Logger = &NullLogger{}
	var _ Logger = &StdLogger{}
}

func TestDefaultLogger_TimestampFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewDefaultLogger(LevelInfo, buf)

	logger.Info("test message")

	output := buf.String()
	// Check timestamp format: [YYYY-MM-DD HH:MM:SS.mmm]
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Len(t, lines, 1)

	// Should start with timestamp in brackets
	assert.True(t, strings.HasPrefix(lines[0], "["))
}
