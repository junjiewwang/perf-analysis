package storage

import (
	"testing"

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
	cfg := &Config{
		Type:      StorageTypeCOS,
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
