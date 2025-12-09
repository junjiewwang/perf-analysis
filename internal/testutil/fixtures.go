// Package testutil provides utilities for testing.
package testutil

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// GetTestDataPath returns the absolute path to a file in the testdata directory.
// It searches for testdata in the caller's directory and parent directories.
func GetTestDataPath(t *testing.T, filename string) string {
	t.Helper()

	_, callerFile, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("failed to get caller file path")
	}

	// Search for testdata directory starting from caller's directory
	dir := filepath.Dir(callerFile)
	for i := 0; i < 5; i++ { // Search up to 5 levels
		testdataPath := filepath.Join(dir, "testdata", filename)
		if _, err := os.Stat(testdataPath); err == nil {
			return testdataPath
		}
		dir = filepath.Dir(dir)
	}

	// Fallback to relative path
	return filepath.Join("testdata", filename)
}

// LoadFixture loads a test fixture file and returns its contents.
func LoadFixture(t *testing.T, filename string) []byte {
	t.Helper()
	path := GetTestDataPath(t, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to load fixture %s: %v", filename, err)
	}
	return data
}

// LoadFixtureString loads a test fixture file and returns its contents as string.
func LoadFixtureString(t *testing.T, filename string) string {
	return string(LoadFixture(t, filename))
}

// LoadFixtureReader loads a test fixture file and returns an io.Reader.
func LoadFixtureReader(t *testing.T, filename string) io.Reader {
	return bytes.NewReader(LoadFixture(t, filename))
}

// MustLoadFixture loads a test fixture file, panicking on error.
// Use this only in non-test contexts like benchmarks.
func MustLoadFixture(filename string) []byte {
	_, callerFile, _, ok := runtime.Caller(1)
	if !ok {
		panic("failed to get caller file path")
	}

	dir := filepath.Dir(callerFile)
	for i := 0; i < 5; i++ {
		testdataPath := filepath.Join(dir, "testdata", filename)
		data, err := os.ReadFile(testdataPath)
		if err == nil {
			return data
		}
		dir = filepath.Dir(dir)
	}

	panic("failed to load fixture: " + filename)
}

// TempDir creates a temporary directory for testing and returns its path.
// The directory is automatically cleaned up when the test completes.
func TempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "perf-analysis-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}

// TempFile creates a temporary file with the given content and returns its path.
// The file is automatically cleaned up when the test completes.
func TempFile(t *testing.T, content string) string {
	t.Helper()
	dir := TempDir(t)
	path := filepath.Join(dir, "temp_file")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

// TempFileWithName creates a temporary file with the given name and content.
func TempFileWithName(t *testing.T, name, content string) string {
	t.Helper()
	dir := TempDir(t)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

// WriteFile writes content to a file in the given directory.
func WriteFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	return path
}

// CreateDir creates a directory within the given parent directory.
func CreateDir(t *testing.T, parent, name string) string {
	t.Helper()
	path := filepath.Join(parent, name)
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	return path
}

// ReadFile reads a file and returns its contents.
func ReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}
	return string(data)
}

// FileExists checks if a file exists.
func FileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}
