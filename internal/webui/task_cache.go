package webui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/junjiewwang/perf-analysis/internal/storage"
	"github.com/junjiewwang/perf-analysis/pkg/utils"
)

// taskIDPattern matches a standard UUID v4 format used as task identifiers.
// This is used to filter out non-task directories/prefixes in storage.
var taskIDPattern = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
)

// TaskCache provides a local filesystem cache layer for remote storage.
// WebUI reads local files directly; TaskCache ensures the task data is
// downloaded from remote storage to a local cache directory on first access.
type TaskCache struct {
	cacheDir string
	storage  storage.Storage
	maxSize  int64 // max cache size in bytes, 0 = unlimited
	logger   utils.Logger
	mu       sync.Mutex
}

// NewTaskCache creates a new TaskCache instance.
func NewTaskCache(cacheDir string, store storage.Storage, maxSize int64, logger utils.Logger) (*TaskCache, error) {
	if cacheDir == "" {
		return nil, fmt.Errorf("cache directory is required")
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &TaskCache{
		cacheDir: cacheDir,
		storage:  store,
		maxSize:  maxSize,
		logger:   logger,
	}, nil
}

// EnsureTask ensures the task data is available in the local cache directory.
// If the task directory already exists locally, it returns immediately.
// Otherwise, it downloads all task files from remote storage.
func (tc *TaskCache) EnsureTask(ctx context.Context, taskID string) (string, error) {
	if !taskIDPattern.MatchString(taskID) {
		return "", fmt.Errorf("invalid task ID format: %s", taskID)
	}

	taskDir := filepath.Join(tc.cacheDir, taskID)

	// Fast path: check if already cached
	if info, err := os.Stat(taskDir); err == nil && info.IsDir() {
		entries, readErr := os.ReadDir(taskDir)
		if readErr == nil && len(entries) > 0 {
			return taskDir, nil
		}
	}

	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Double-check after acquiring lock
	if info, err := os.Stat(taskDir); err == nil && info.IsDir() {
		entries, readErr := os.ReadDir(taskDir)
		if readErr == nil && len(entries) > 0 {
			return taskDir, nil
		}
	}

	// List all objects for this task from remote storage
	prefix := taskID + "/"
	objects, err := tc.storage.ListByPrefix(ctx, prefix)
	if err != nil {
		return "", fmt.Errorf("failed to list task objects from storage: %w", err)
	}

	if len(objects) == 0 {
		return "", fmt.Errorf("task %s not found in remote storage", taskID)
	}

	// Evict old cache entries if necessary
	if tc.maxSize > 0 {
		var totalSize int64
		for _, obj := range objects {
			totalSize += obj.Size
		}
		if err := tc.evictIfNeeded(totalSize); err != nil {
			tc.logger.Warn("failed to evict cache entries: %v", err)
		}
	}

	// Download all files
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create task cache directory: %w", err)
	}

	for _, obj := range objects {
		// Convert storage key to local path
		relPath := strings.TrimPrefix(obj.Key, prefix)
		if relPath == "" {
			continue
		}
		localPath := filepath.Join(taskDir, relPath)

		if err := tc.storage.DownloadFile(ctx, obj.Key, localPath); err != nil {
			// Clean up partially downloaded task
			os.RemoveAll(taskDir)
			return "", fmt.Errorf("failed to download %s: %w", obj.Key, err)
		}
	}

	tc.logger.Info("cached task %s (%d files)", taskID, len(objects))
	return taskDir, nil
}

// GetTaskDir returns the local task directory path without downloading.
// Used for tasks that are already known to be local (e.g., local storage mode).
func (tc *TaskCache) GetTaskDir(taskID string) string {
	return filepath.Join(tc.cacheDir, taskID)
}

// ListTasks returns a list of task IDs available either locally or in remote storage.
func (tc *TaskCache) ListTasks(ctx context.Context) ([]string, error) {
	taskSet := make(map[string]struct{})

	// List local cached tasks
	entries, err := os.ReadDir(tc.cacheDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() && taskIDPattern.MatchString(entry.Name()) {
				taskSet[entry.Name()] = struct{}{}
			}
		}
	}

	// List remote tasks (by enumerating top-level prefixes)
	objects, err := tc.storage.ListByPrefix(ctx, "")
	if err != nil {
		tc.logger.Warn("failed to list remote tasks: %v", err)
	} else {
		for _, obj := range objects {
			parts := strings.SplitN(obj.Key, "/", 2)
			if len(parts) > 0 && taskIDPattern.MatchString(parts[0]) {
				taskSet[parts[0]] = struct{}{}
			}
		}
	}

	tasks := make([]string, 0, len(taskSet))
	for taskID := range taskSet {
		tasks = append(tasks, taskID)
	}
	sort.Strings(tasks)

	return tasks, nil
}

// cacheEntry represents a cached task directory with its metadata.
type cacheEntry struct {
	taskID   string
	size     int64
	modTime  time.Time
}

// evictIfNeeded evicts old cache entries to make room for new data.
func (tc *TaskCache) evictIfNeeded(newDataSize int64) error {
	entries, err := tc.getCacheEntries()
	if err != nil {
		return err
	}

	var totalSize int64
	for _, entry := range entries {
		totalSize += entry.size
	}

	// Check if eviction is needed
	if totalSize+newDataSize <= tc.maxSize {
		return nil
	}

	// Sort by modification time (oldest first) for LRU eviction
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].modTime.Before(entries[j].modTime)
	})

	// Evict oldest entries until enough space is available
	for _, entry := range entries {
		if totalSize+newDataSize <= tc.maxSize {
			break
		}
		taskDir := filepath.Join(tc.cacheDir, entry.taskID)
		if err := os.RemoveAll(taskDir); err != nil {
			tc.logger.Warn("failed to evict cache entry %s: %v", entry.taskID, err)
			continue
		}
		totalSize -= entry.size
		tc.logger.Info("evicted cached task %s (%.2f MB)", entry.taskID, float64(entry.size)/(1024*1024))
	}

	return nil
}

// getCacheEntries lists all cached task directories with their total sizes.
func (tc *TaskCache) getCacheEntries() ([]cacheEntry, error) {
	dirEntries, err := os.ReadDir(tc.cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	var entries []cacheEntry
	for _, de := range dirEntries {
		if !de.IsDir() || strings.HasPrefix(de.Name(), ".") {
			continue
		}

		taskDir := filepath.Join(tc.cacheDir, de.Name())
		size, modTime := tc.dirSizeAndModTime(taskDir)
		entries = append(entries, cacheEntry{
			taskID:  de.Name(),
			size:    size,
			modTime: modTime,
		})
	}

	return entries, nil
}

// dirSizeAndModTime calculates the total size and latest modification time of a directory.
func (tc *TaskCache) dirSizeAndModTime(dir string) (int64, time.Time) {
	var totalSize int64
	var latestMod time.Time

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			totalSize += info.Size()
			if info.ModTime().After(latestMod) {
				latestMod = info.ModTime()
			}
		}
		return nil
	})

	return totalSize, latestMod
}
