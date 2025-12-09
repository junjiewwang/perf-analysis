// Package errors defines common error types for the application.
package errors

import (
	"errors"
	"fmt"
)

// Error codes for the application.
const (
	CodeUnknown       = "UNKNOWN_ERROR"
	CodeDatabaseError = "DATABASE_ERROR"
	CodeUploadError   = "UPLOAD_ERROR"
	CodeDownloadError = "DOWNLOAD_ERROR"
	CodeAnalysisError = "ANALYSIS_ERROR"
	CodeEmptyFile     = "EMPTY_FILE"
	CodeParseError    = "PARSE_ERROR"
	CodeInvalidInput  = "INVALID_INPUT"
	CodeTimeout       = "TIMEOUT_ERROR"
	CodeNotFound      = "NOT_FOUND"
	CodeConfigError   = "CONFIG_ERROR"
)

// AppError represents an application error with a code and message.
type AppError struct {
	Code    string
	Message string
	Err     error
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *AppError) Unwrap() error {
	return e.Err
}

// Is checks if the error matches the target.
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// New creates a new AppError.
func New(code string, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// Wrap wraps an existing error with an AppError.
func Wrap(code string, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Common error instances.
var (
	ErrDatabaseError = New(CodeDatabaseError, "database error")
	ErrUploadError   = New(CodeUploadError, "upload error")
	ErrDownloadError = New(CodeDownloadError, "download error")
	ErrAnalysisError = New(CodeAnalysisError, "analysis error")
	ErrEmptyFile     = New(CodeEmptyFile, "empty file")
	ErrParseError    = New(CodeParseError, "parse error")
	ErrInvalidInput  = New(CodeInvalidInput, "invalid input")
	ErrTimeout       = New(CodeTimeout, "operation timeout")
	ErrNotFound      = New(CodeNotFound, "resource not found")
	ErrConfigError   = New(CodeConfigError, "configuration error")
)

// IsDatabaseError checks if the error is a database error.
func IsDatabaseError(err error) bool {
	return errors.Is(err, ErrDatabaseError)
}

// IsUploadError checks if the error is an upload error.
func IsUploadError(err error) bool {
	return errors.Is(err, ErrUploadError)
}

// IsDownloadError checks if the error is a download error.
func IsDownloadError(err error) bool {
	return errors.Is(err, ErrDownloadError)
}

// IsAnalysisError checks if the error is an analysis error.
func IsAnalysisError(err error) bool {
	return errors.Is(err, ErrAnalysisError)
}

// IsEmptyFileError checks if the error is an empty file error.
func IsEmptyFileError(err error) bool {
	return errors.Is(err, ErrEmptyFile)
}

// GetErrorCode extracts the error code from an error.
func GetErrorCode(err error) string {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return CodeUnknown
}

// GetErrorMessage extracts the error message from an error.
func GetErrorMessage(err error) string {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// ErrorInfo provides error information mapping (compatible with Python version).
var ErrorInfo = map[string]string{
	"DatabaseError": CodeDatabaseError,
	"UploadError":   CodeUploadError,
	"DownloadError": CodeDownloadError,
	"AnalysisError": CodeAnalysisError,
	"EmptyFile":     CodeEmptyFile,
}
