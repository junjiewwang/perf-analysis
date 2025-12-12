package storage

import (
	"testing"

	"github.com/perf-analysis/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestNewCOSStorage_Validation(t *testing.T) {
	t.Run("MissingBucket", func(t *testing.T) {
		cfg := &COSConfig{
			Region:    "ap-guangzhou",
			SecretID:  "test-id",
			SecretKey: "test-key",
		}

		storage, err := NewCOSStorage(cfg)
		assert.Error(t, err)
		assert.Nil(t, storage)
		assert.Contains(t, err.Error(), "bucket and region are required")
	})

	t.Run("MissingRegion", func(t *testing.T) {
		cfg := &COSConfig{
			Bucket:    "test-bucket",
			SecretID:  "test-id",
			SecretKey: "test-key",
		}

		storage, err := NewCOSStorage(cfg)
		assert.Error(t, err)
		assert.Nil(t, storage)
		assert.Contains(t, err.Error(), "bucket and region are required")
	})

	t.Run("MissingCredentials", func(t *testing.T) {
		cfg := &COSConfig{
			Bucket: "test-bucket",
			Region: "ap-guangzhou",
		}

		storage, err := NewCOSStorage(cfg)
		assert.Error(t, err)
		assert.Nil(t, storage)
		assert.Contains(t, err.Error(), "credentials are required")
	})

	t.Run("ValidConfig", func(t *testing.T) {
		cfg := &COSConfig{
			Bucket:    "test-bucket",
			Region:    "ap-guangzhou",
			SecretID:  "test-id",
			SecretKey: "test-key",
		}

		storage, err := NewCOSStorage(cfg)
		assert.NoError(t, err)
		assert.NotNil(t, storage)
	})
}

func TestCOSStorage_GetURL(t *testing.T) {
	cfg := &COSConfig{
		Bucket:    "my-bucket",
		Region:    "ap-guangzhou",
		SecretID:  "test-id",
		SecretKey: "test-key",
	}

	storage, err := NewCOSStorage(cfg)
	assert.NoError(t, err)

	url := storage.GetURL("path/to/file.txt")
	expected := "https://my-bucket.cos.ap-guangzhou.myqcloud.com/path/to/file.txt"
	assert.Equal(t, expected, url)
}

func TestNewStorage_COS(t *testing.T) {
	cfg := &config.StorageConfig{
		Type:      "cos",
		Bucket:    "test-bucket",
		Region:    "ap-guangzhou",
		SecretID:  "test-id",
		SecretKey: "test-key",
	}

	storage, err := NewStorage(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, storage)

	// Verify it's a COSStorage
	_, ok := storage.(*COSStorage)
	assert.True(t, ok)
}

func TestValidateConfig(t *testing.T) {
	t.Run("NilConfig", func(t *testing.T) {
		err := ValidateConfig(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "storage config is nil")
	})

	t.Run("InvalidStorageType", func(t *testing.T) {
		cfg := &config.StorageConfig{
			Type: "s3",
		}
		err := ValidateConfig(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported storage type")
	})

	t.Run("COSMissingBucket", func(t *testing.T) {
		cfg := &config.StorageConfig{
			Type:      "cos",
			Region:    "ap-guangzhou",
			SecretID:  "test-id",
			SecretKey: "test-key",
		}
		err := ValidateConfig(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "COS bucket is required")
	})

	t.Run("COSMissingRegion", func(t *testing.T) {
		cfg := &config.StorageConfig{
			Type:      "cos",
			Bucket:    "test-bucket",
			SecretID:  "test-id",
			SecretKey: "test-key",
		}
		err := ValidateConfig(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "COS region is required")
	})

	t.Run("COSMissingCredentials", func(t *testing.T) {
		cfg := &config.StorageConfig{
			Type:   "cos",
			Bucket: "test-bucket",
			Region: "ap-guangzhou",
		}
		err := ValidateConfig(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "COS credentials are required")
	})

	t.Run("LocalMissingPath", func(t *testing.T) {
		cfg := &config.StorageConfig{
			Type: "local",
		}
		err := ValidateConfig(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "local storage path is required")
	})

	t.Run("ValidCOSConfig", func(t *testing.T) {
		cfg := &config.StorageConfig{
			Type:      "cos",
			Bucket:    "test-bucket",
			Region:    "ap-guangzhou",
			SecretID:  "test-id",
			SecretKey: "test-key",
		}
		err := ValidateConfig(cfg)
		assert.NoError(t, err)
	})

	t.Run("ValidLocalConfig", func(t *testing.T) {
		cfg := &config.StorageConfig{
			Type:      "local",
			LocalPath: "/tmp/storage",
		}
		err := ValidateConfig(cfg)
		assert.NoError(t, err)
	})
}
