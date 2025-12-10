package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStorage implements Storage interface for local filesystem.
type LocalStorage struct {
	basePath string
}

// NewLocalStorage creates a new LocalStorage instance.
func NewLocalStorage(basePath string) (*LocalStorage, error) {
	if basePath == "" {
		basePath = "./storage"
	}

	// Ensure base directory exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &LocalStorage{basePath: basePath}, nil
}

// Upload uploads data from reader to the specified key.
func (s *LocalStorage) Upload(ctx context.Context, key string, reader io.Reader) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	fullPath := s.getFullPath(key)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// UploadFile uploads a local file to the specified key.
func (s *LocalStorage) UploadFile(ctx context.Context, key string, localPath string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	fullPath := s.getFullPath(key)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Open source file
	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	// Create destination file
	dst, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	// Copy contents
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

// Download downloads data from the specified key.
func (s *LocalStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	fullPath := s.getFullPath(key)
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", key)
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return file, nil
}

// DownloadFile downloads data from the specified key to a local file.
func (s *LocalStorage) DownloadFile(ctx context.Context, key string, localPath string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	fullPath := s.getFullPath(key)

	// Ensure parent directory of destination exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Open source file
	src, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", key)
		}
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	// Create destination file
	dst, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	// Copy contents
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

// Delete deletes the object at the specified key.
func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	fullPath := s.getFullPath(key)
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil // File already deleted
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// Exists checks if an object exists at the specified key.
func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	fullPath := s.getFullPath(key)
	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return true, nil
}

// GetURL returns the file path for local storage.
func (s *LocalStorage) GetURL(key string) string {
	return s.getFullPath(key)
}

// getFullPath returns the full filesystem path for the given key.
func (s *LocalStorage) getFullPath(key string) string {
	return filepath.Join(s.basePath, key)
}

// GetBasePath returns the base path for the local storage.
func (s *LocalStorage) GetBasePath() string {
	return s.basePath
}
