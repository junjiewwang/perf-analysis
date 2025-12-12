// Package storage provides object storage abstraction for the perf-analysis service.
package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/perf-analysis/pkg/config"
)

// Storage defines the interface for object storage operations.
type Storage interface {
	// Upload uploads data from reader to the specified key.
	Upload(ctx context.Context, key string, reader io.Reader) error

	// UploadFile uploads a local file to the specified key.
	UploadFile(ctx context.Context, key string, localPath string) error

	// Download downloads data from the specified key.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// DownloadFile downloads data from the specified key to a local file.
	DownloadFile(ctx context.Context, key string, localPath string) error

	// Delete deletes the object at the specified key.
	Delete(ctx context.Context, key string) error

	// Exists checks if an object exists at the specified key.
	Exists(ctx context.Context, key string) (bool, error)

	// GetURL returns the URL for the specified key (if applicable).
	GetURL(key string) string
}

// StorageType represents the type of storage backend.
type StorageType string

const (
	StorageTypeLocal StorageType = "local"
	StorageTypeCOS   StorageType = "cos"
)

// NewStorage creates a new Storage instance based on the configuration.
func NewStorage(cfg *config.StorageConfig) (Storage, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	switch StorageType(cfg.Type) {
	case StorageTypeLocal:
		return NewLocalStorage(cfg.LocalPath)
	case StorageTypeCOS:
		return NewCOSStorage(&COSConfig{
			Bucket:    cfg.Bucket,
			Region:    cfg.Region,
			SecretID:  cfg.SecretID,
			SecretKey: cfg.SecretKey,
			Domain:    cfg.Domain,
			Scheme:    cfg.Scheme,
		})
	default:
		return NewLocalStorage(cfg.LocalPath)
	}
}

// ValidateConfig validates the storage configuration.
func ValidateConfig(cfg *config.StorageConfig) error {
	if cfg == nil {
		return fmt.Errorf("storage config is nil")
	}

	storageType := StorageType(cfg.Type)

	// Empty type defaults to local
	if storageType == "" {
		storageType = StorageTypeLocal
	}

	if storageType != StorageTypeCOS && storageType != StorageTypeLocal {
		return fmt.Errorf("unsupported storage type: %s", cfg.Type)
	}

	if storageType == StorageTypeCOS {
		if cfg.Bucket == "" {
			return fmt.Errorf("COS bucket is required")
		}
		if cfg.Region == "" {
			return fmt.Errorf("COS region is required")
		}
		if cfg.SecretID == "" || cfg.SecretKey == "" {
			return fmt.Errorf("COS credentials are required")
		}
	}

	if storageType == StorageTypeLocal {
		if cfg.LocalPath == "" {
			return fmt.Errorf("local storage path is required")
		}
	}

	return nil
}
