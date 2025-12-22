package pprof

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Writer provides concurrent-safe file writing for profile data.
type Writer struct {
	mu        sync.Mutex
	outputDir string
	maxSize   int64
	maxFiles  int
	autoRotate bool
}

// NewWriter creates a new Writer.
func NewWriter(outputDir string, maxSize int64, maxFiles int, autoRotate bool) *Writer {
	return &Writer{
		outputDir:  outputDir,
		maxSize:    maxSize,
		maxFiles:   maxFiles,
		autoRotate: autoRotate,
	}
}

// EnsureDir creates the output directory and profile type subdirectories.
func (w *Writer) EnsureDir(profiles []ProfileType) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(w.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	for _, pt := range profiles {
		dir := filepath.Join(w.outputDir, string(pt))
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create profile directory %s: %w", pt, err)
		}
	}

	return nil
}

// Write writes profile data to a file.
func (w *Writer) Write(pt ProfileType, data []byte) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	dir := filepath.Join(w.outputDir, string(pt))
	filename := fmt.Sprintf("%s_%s.pprof", pt, time.Now().Format("20060102_150405"))
	filePath := filepath.Join(dir, filename)

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write profile file: %w", err)
	}

	// Rotate old files if needed
	if w.autoRotate {
		if err := w.rotateFiles(dir); err != nil {
			// Log but don't fail
			fmt.Printf("Warning: failed to rotate files: %v\n", err)
		}
	}

	return filePath, nil
}

// WriteToFile writes profile data to a specific file path.
func (w *Writer) WriteToFile(filePath string, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return os.WriteFile(filePath, data, 0600)
}

// rotateFiles removes old files if the count exceeds maxFiles.
func (w *Writer) rotateFiles(dir string) error {
	if w.maxFiles <= 0 {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	// Filter and sort pprof files by modification time
	type fileInfo struct {
		name    string
		modTime time.Time
	}

	var files []fileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".pprof" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{
			name:    entry.Name(),
			modTime: info.ModTime(),
		})
	}

	// Sort by modification time (oldest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	// Remove oldest files if count exceeds maxFiles
	for len(files) > w.maxFiles {
		oldest := files[0]
		filePath := filepath.Join(dir, oldest.name)
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to remove old file %s: %w", oldest.name, err)
		}
		files = files[1:]
	}

	return nil
}

// GetOutputDir returns the output directory.
func (w *Writer) GetOutputDir() string {
	return w.outputDir
}

// ListFiles returns all profile files for a given profile type.
func (w *Writer) ListFiles(pt ProfileType) ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	dir := filepath.Join(w.outputDir, string(pt))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".pprof" {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	return files, nil
}

// Clean removes all profile files.
func (w *Writer) Clean() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return os.RemoveAll(w.outputDir)
}
