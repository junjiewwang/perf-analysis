package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log message.
type LogLevel int

const (
	// LevelDebug is the debug log level.
	LevelDebug LogLevel = iota
	// LevelInfo is the info log level.
	LevelInfo
	// LevelWarn is the warning log level.
	LevelWarn
	// LevelError is the error log level.
	LevelError
)

// String returns the string representation of LogLevel.
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger is the interface for logging.
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	WithField(key string, value interface{}) Logger
	WithFields(fields map[string]interface{}) Logger
}

// DefaultLogger is a simple logger implementation.
type DefaultLogger struct {
	mu     sync.Mutex
	level  LogLevel
	output io.Writer
	fields map[string]interface{}
	prefix string
}

// NewDefaultLogger creates a new DefaultLogger.
func NewDefaultLogger(level LogLevel, output io.Writer) *DefaultLogger {
	return &DefaultLogger{
		level:  level,
		output: output,
		fields: make(map[string]interface{}),
	}
}

// NewFileLogger creates a logger that writes to a file.
func NewFileLogger(level LogLevel, logPath string) (*DefaultLogger, error) {
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return NewDefaultLogger(level, file), nil
}

// SetLevel sets the log level.
func (l *DefaultLogger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// Debug logs a debug message.
func (l *DefaultLogger) Debug(msg string, args ...interface{}) {
	l.log(LevelDebug, msg, args...)
}

// Info logs an info message.
func (l *DefaultLogger) Info(msg string, args ...interface{}) {
	l.log(LevelInfo, msg, args...)
}

// Warn logs a warning message.
func (l *DefaultLogger) Warn(msg string, args ...interface{}) {
	l.log(LevelWarn, msg, args...)
}

// Error logs an error message.
func (l *DefaultLogger) Error(msg string, args ...interface{}) {
	l.log(LevelError, msg, args...)
}

// WithField creates a new logger with the given field.
func (l *DefaultLogger) WithField(key string, value interface{}) Logger {
	newLogger := &DefaultLogger{
		level:  l.level,
		output: l.output,
		fields: make(map[string]interface{}),
		prefix: l.prefix,
	}
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	newLogger.fields[key] = value
	return newLogger
}

// WithFields creates a new logger with the given fields.
func (l *DefaultLogger) WithFields(fields map[string]interface{}) Logger {
	newLogger := &DefaultLogger{
		level:  l.level,
		output: l.output,
		fields: make(map[string]interface{}),
		prefix: l.prefix,
	}
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	return newLogger
}

func (l *DefaultLogger) log(level LogLevel, msg string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	formattedMsg := fmt.Sprintf(msg, args...)

	// Build field string
	fieldStr := ""
	for k, v := range l.fields {
		fieldStr += fmt.Sprintf(" %s=%v", k, v)
	}

	logLine := fmt.Sprintf("[%s] [%s]%s %s\n", timestamp, level.String(), fieldStr, formattedMsg)

	_, _ = l.output.Write([]byte(logLine))
}

// ParseLogLevel parses a string to LogLevel.
func ParseLogLevel(level string) LogLevel {
	switch level {
	case "debug", "DEBUG":
		return LevelDebug
	case "info", "INFO":
		return LevelInfo
	case "warn", "WARN", "warning", "WARNING":
		return LevelWarn
	case "error", "ERROR":
		return LevelError
	default:
		return LevelInfo
	}
}

// Global logger instance
var globalLogger Logger = NewDefaultLogger(LevelInfo, os.Stdout)

// SetGlobalLogger sets the global logger.
func SetGlobalLogger(logger Logger) {
	globalLogger = logger
}

// GetGlobalLogger returns the global logger.
func GetGlobalLogger() Logger {
	return globalLogger
}

// NullLogger is a logger that discards all log messages.
type NullLogger struct{}

// Debug does nothing.
func (l *NullLogger) Debug(msg string, args ...interface{}) {}

// Info does nothing.
func (l *NullLogger) Info(msg string, args ...interface{}) {}

// Warn does nothing.
func (l *NullLogger) Warn(msg string, args ...interface{}) {}

// Error does nothing.
func (l *NullLogger) Error(msg string, args ...interface{}) {}

// WithField returns the same NullLogger.
func (l *NullLogger) WithField(key string, value interface{}) Logger {
	return l
}

// WithFields returns the same NullLogger.
func (l *NullLogger) WithFields(fields map[string]interface{}) Logger {
	return l
}

// StdLogger wraps the standard library logger.
type StdLogger struct {
	logger *log.Logger
	level  LogLevel
	fields map[string]interface{}
}

// NewStdLogger creates a new StdLogger.
func NewStdLogger(level LogLevel, output io.Writer) *StdLogger {
	return &StdLogger{
		logger: log.New(output, "", log.LstdFlags|log.Lmicroseconds),
		level:  level,
		fields: make(map[string]interface{}),
	}
}

// Debug logs a debug message.
func (l *StdLogger) Debug(msg string, args ...interface{}) {
	if l.level <= LevelDebug {
		l.logger.Printf("[DEBUG] "+msg, args...)
	}
}

// Info logs an info message.
func (l *StdLogger) Info(msg string, args ...interface{}) {
	if l.level <= LevelInfo {
		l.logger.Printf("[INFO] "+msg, args...)
	}
}

// Warn logs a warning message.
func (l *StdLogger) Warn(msg string, args ...interface{}) {
	if l.level <= LevelWarn {
		l.logger.Printf("[WARN] "+msg, args...)
	}
}

// Error logs an error message.
func (l *StdLogger) Error(msg string, args ...interface{}) {
	if l.level <= LevelError {
		l.logger.Printf("[ERROR] "+msg, args...)
	}
}

// WithField creates a new logger with the given field.
func (l *StdLogger) WithField(key string, value interface{}) Logger {
	newLogger := NewStdLogger(l.level, l.logger.Writer())
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	newLogger.fields[key] = value
	return newLogger
}

// WithFields creates a new logger with the given fields.
func (l *StdLogger) WithFields(fields map[string]interface{}) Logger {
	newLogger := NewStdLogger(l.level, l.logger.Writer())
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	return newLogger
}
