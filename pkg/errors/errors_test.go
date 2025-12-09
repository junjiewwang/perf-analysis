package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *AppError
		expected string
	}{
		{
			name:     "without underlying error",
			err:      New(CodeDatabaseError, "connection failed"),
			expected: "[DATABASE_ERROR] connection failed",
		},
		{
			name:     "with underlying error",
			err:      Wrap(CodeUploadError, "upload failed", errors.New("network timeout")),
			expected: "[UPLOAD_ERROR] upload failed: network timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	err := Wrap(CodeAnalysisError, "analysis failed", underlying)

	unwrapped := err.Unwrap()
	assert.Equal(t, underlying, unwrapped)
}

func TestAppError_Is(t *testing.T) {
	err1 := New(CodeDatabaseError, "error 1")
	err2 := New(CodeDatabaseError, "error 2")
	err3 := New(CodeUploadError, "error 3")

	assert.True(t, errors.Is(err1, err2))
	assert.False(t, errors.Is(err1, err3))
}

func TestIsDatabaseError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "database error",
			err:      ErrDatabaseError,
			expected: true,
		},
		{
			name:     "wrapped database error",
			err:      Wrap(CodeDatabaseError, "db error", errors.New("connection refused")),
			expected: true,
		},
		{
			name:     "other error",
			err:      ErrUploadError,
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsDatabaseError(tt.err))
		})
	}
}

func TestIsUploadError(t *testing.T) {
	assert.True(t, IsUploadError(ErrUploadError))
	assert.False(t, IsUploadError(ErrDatabaseError))
}

func TestIsDownloadError(t *testing.T) {
	assert.True(t, IsDownloadError(ErrDownloadError))
	assert.False(t, IsDownloadError(ErrDatabaseError))
}

func TestIsAnalysisError(t *testing.T) {
	assert.True(t, IsAnalysisError(ErrAnalysisError))
	assert.False(t, IsAnalysisError(ErrDatabaseError))
}

func TestIsEmptyFileError(t *testing.T) {
	assert.True(t, IsEmptyFileError(ErrEmptyFile))
	assert.False(t, IsEmptyFileError(ErrDatabaseError))
}

func TestGetErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "app error",
			err:      New(CodeDatabaseError, "db error"),
			expected: CodeDatabaseError,
		},
		{
			name:     "wrapped app error",
			err:      Wrap(CodeUploadError, "upload", errors.New("inner")),
			expected: CodeUploadError,
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: CodeUnknown,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: CodeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetErrorCode(tt.err))
		})
	}
}

func TestGetErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "app error",
			err:      New(CodeDatabaseError, "db connection failed"),
			expected: "db connection failed",
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: "standard error",
		},
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetErrorMessage(tt.err))
		})
	}
}

func TestErrorInfo(t *testing.T) {
	assert.Equal(t, CodeDatabaseError, ErrorInfo["DatabaseError"])
	assert.Equal(t, CodeUploadError, ErrorInfo["UploadError"])
	assert.Equal(t, CodeDownloadError, ErrorInfo["DownloadError"])
	assert.Equal(t, CodeAnalysisError, ErrorInfo["AnalysisError"])
	assert.Equal(t, CodeEmptyFile, ErrorInfo["EmptyFile"])
}
