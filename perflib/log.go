// Package perflib provides reusable performance analysis engine components.
package perflib

// Logger defines the logging interface for perflib.
// Consumers inject their own Logger implementation (e.g., slog adapter, zap, logrus).
//
// This is a minimal interface covering only the methods used by perflib internals.
// Any logger with these four methods automatically satisfies this interface.
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}
