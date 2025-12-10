// Package storage provides object storage abstraction for the perf-analysis service.
package storage

import (
	"context"
	"io"
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

// Config holds storage configuration.
type Config struct {
	Type      StorageType `mapstructure:"type"`
	Bucket    string      `mapstructure:"bucket"`
	Region    string      `mapstructure:"region"`
	SecretID  string      `mapstructure:"secret_id"`
	SecretKey string      `mapstructure:"secret_key"`
	LocalPath string      `mapstructure:"local_path"`
	BaseURL   string      `mapstructure:"base_url"`
}

// NewStorage creates a new Storage instance based on the configuration.
func NewStorage(cfg *Config) (Storage, error) {
	switch cfg.Type {
	case StorageTypeLocal:
		return NewLocalStorage(cfg.LocalPath)
	case StorageTypeCOS:
		return NewCOSStorage(&COSConfig{
			Bucket:    cfg.Bucket,
			Region:    cfg.Region,
			SecretID:  cfg.SecretID,
			SecretKey: cfg.SecretKey,
		})
	default:
		return NewLocalStorage(cfg.LocalPath)
	}
}
