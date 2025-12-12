package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/tencentyun/cos-go-sdk-v5"
)

// COSConfig holds COS-specific configuration.
type COSConfig struct {
	Bucket    string
	Region    string
	SecretID  string
	SecretKey string
	Domain    string // e.g., "myqcloud.com"
	Scheme    string // e.g., "https" or "http"
}

// COSStorage implements Storage interface for Tencent Cloud COS.
type COSStorage struct {
	client *cos.Client
	bucket string
	region string
	domain string
	scheme string
}

// NewCOSStorage creates a new COSStorage instance.
func NewCOSStorage(cfg *COSConfig) (*COSStorage, error) {
	if cfg.Bucket == "" || cfg.Region == "" {
		return nil, fmt.Errorf("bucket and region are required for COS storage")
	}
	if cfg.SecretID == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("credentials are required for COS storage")
	}

	// Set defaults for domain and scheme
	domain := cfg.Domain
	if domain == "" {
		domain = "myqcloud.com"
	}
	scheme := cfg.Scheme
	if scheme == "" {
		scheme = "https"
	}

	bucketURL, err := url.Parse(fmt.Sprintf("%s://%s.cos.%s.%s", scheme, cfg.Bucket, cfg.Region, domain))
	if err != nil {
		return nil, fmt.Errorf("failed to parse bucket URL: %w", err)
	}

	serviceURL, err := url.Parse(fmt.Sprintf("%s://cos.%s.%s", scheme, cfg.Region, domain))
	if err != nil {
		return nil, fmt.Errorf("failed to parse service URL: %w", err)
	}

	client := cos.NewClient(&cos.BaseURL{
		BucketURL:  bucketURL,
		ServiceURL: serviceURL,
	}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.SecretID,
			SecretKey: cfg.SecretKey,
		},
	})

	return &COSStorage{
		client: client,
		bucket: cfg.Bucket,
		region: cfg.Region,
		domain: domain,
		scheme: scheme,
	}, nil
}

// Upload uploads data from reader to the specified key.
func (s *COSStorage) Upload(ctx context.Context, key string, reader io.Reader) error {
	_, err := s.client.Object.Put(ctx, key, reader, nil)
	if err != nil {
		return fmt.Errorf("failed to upload to COS: %w", err)
	}
	return nil
}

// UploadFile uploads a local file to the specified key.
func (s *COSStorage) UploadFile(ctx context.Context, key string, localPath string) error {
	_, err := s.client.Object.PutFromFile(ctx, key, localPath, nil)
	if err != nil {
		return fmt.Errorf("failed to upload file to COS: %w", err)
	}
	return nil
}

// Download downloads data from the specified key.
func (s *COSStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := s.client.Object.Get(ctx, key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to download from COS: %w", err)
	}
	return resp.Body, nil
}

// DownloadFile downloads data from the specified key to a local file.
func (s *COSStorage) DownloadFile(ctx context.Context, key string, localPath string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	_, err := s.client.Object.GetToFile(ctx, key, localPath, nil)
	if err != nil {
		return fmt.Errorf("failed to download file from COS: %w", err)
	}
	return nil
}

// Delete deletes the object at the specified key.
func (s *COSStorage) Delete(ctx context.Context, key string) error {
	_, err := s.client.Object.Delete(ctx, key, nil)
	if err != nil {
		return fmt.Errorf("failed to delete from COS: %w", err)
	}
	return nil
}

// Exists checks if an object exists at the specified key.
func (s *COSStorage) Exists(ctx context.Context, key string) (bool, error) {
	ok, err := s.client.Object.IsExist(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to check existence in COS: %w", err)
	}
	return ok, nil
}

// GetURL returns the public URL for the specified key.
func (s *COSStorage) GetURL(key string) string {
	return fmt.Sprintf("%s://%s.cos.%s.%s/%s", s.scheme, s.bucket, s.region, s.domain, key)
}
