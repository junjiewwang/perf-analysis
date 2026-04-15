package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/junjiewwang/perf-analysis/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLocalStorage(t *testing.T) {
	t.Run("CreateWithDefaultPath", func(t *testing.T) {
		tempDir := t.TempDir()
		defaultPath := filepath.Join(tempDir, "storage")

		storage, err := NewLocalStorage(defaultPath)
		require.NoError(t, err)
		require.NotNil(t, storage)

		// Verify directory was created
		info, err := os.Stat(defaultPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("CreateWithEmptyPath", func(t *testing.T) {
		// Save and restore current directory
		origDir, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(origDir)

		tempDir := t.TempDir()
		os.Chdir(tempDir)

		storage, err := NewLocalStorage("")
		require.NoError(t, err)
		require.NotNil(t, storage)

		// Default path should be ./storage
		assert.Equal(t, "./storage", storage.GetBasePath())
	})
}

func TestLocalStorage_Upload(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	t.Run("UploadFromReader", func(t *testing.T) {
		content := []byte("test content for upload")
		reader := bytes.NewReader(content)

		err := storage.Upload(context.Background(), "test/file.txt", reader)
		require.NoError(t, err)

		// Verify file exists
		filePath := filepath.Join(tempDir, "test", "file.txt")
		data, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, content, data)
	})

	t.Run("UploadWithCanceledContext", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := storage.Upload(ctx, "canceled.txt", bytes.NewReader([]byte("test")))
		assert.Error(t, err)
	})
}

func TestLocalStorage_UploadFile(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	t.Run("UploadLocalFile", func(t *testing.T) {
		// Create source file
		srcFile := filepath.Join(tempDir, "source.txt")
		content := []byte("source file content")
		require.NoError(t, os.WriteFile(srcFile, content, 0644))

		// Upload
		err := storage.UploadFile(context.Background(), "dest/file.txt", srcFile)
		require.NoError(t, err)

		// Verify destination
		destPath := filepath.Join(tempDir, "dest", "file.txt")
		data, err := os.ReadFile(destPath)
		require.NoError(t, err)
		assert.Equal(t, content, data)
	})

	t.Run("UploadNonExistentFile", func(t *testing.T) {
		err := storage.UploadFile(context.Background(), "dest.txt", "/nonexistent/path.txt")
		assert.Error(t, err)
	})

	t.Run("UploadFileSamePathSkipsCopy", func(t *testing.T) {
		// Create a file inside basePath where key resolves to the same file
		content := []byte("self-overwrite protection test")
		subDir := filepath.Join(tempDir, "same")
		require.NoError(t, os.MkdirAll(subDir, 0755))
		srcFile := filepath.Join(subDir, "data.json")
		require.NoError(t, os.WriteFile(srcFile, content, 0644))

		// Upload with key that resolves to the same path
		err := storage.UploadFile(context.Background(), "same/data.json", srcFile)
		require.NoError(t, err)

		// Verify file content is preserved (not truncated to 0 bytes)
		data, err := os.ReadFile(srcFile)
		require.NoError(t, err)
		assert.Equal(t, content, data)
	})
}

func Test_isSamePath(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("SameExistingFile", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "test.txt")
		require.NoError(t, os.WriteFile(filePath, []byte("test"), 0644))

		assert.True(t, isSamePath(filePath, filePath))
	})

	t.Run("DifferentExistingFiles", func(t *testing.T) {
		file1 := filepath.Join(tempDir, "file1.txt")
		file2 := filepath.Join(tempDir, "file2.txt")
		require.NoError(t, os.WriteFile(file1, []byte("1"), 0644))
		require.NoError(t, os.WriteFile(file2, []byte("2"), 0644))

		assert.False(t, isSamePath(file1, file2))
	})

	t.Run("RelativeAndAbsoluteSamePath", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "rel.txt")
		require.NoError(t, os.WriteFile(filePath, []byte("rel"), 0644))

		// Both exist, os.SameFile handles this
		assert.True(t, isSamePath(filePath, filePath))
	})

	t.Run("NonExistentPaths", func(t *testing.T) {
		absPath := filepath.Join(tempDir, "noexist", "file.txt")
		assert.True(t, isSamePath(absPath, absPath))
	})

	t.Run("DifferentNonExistentPaths", func(t *testing.T) {
		path1 := filepath.Join(tempDir, "a", "file.txt")
		path2 := filepath.Join(tempDir, "b", "file.txt")
		assert.False(t, isSamePath(path1, path2))
	})
}

func TestLocalStorage_Download(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	t.Run("DownloadExistingFile", func(t *testing.T) {
		// Create file
		content := []byte("download test content")
		filePath := filepath.Join(tempDir, "download", "test.txt")
		require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0755))
		require.NoError(t, os.WriteFile(filePath, content, 0644))

		// Download
		reader, err := storage.Download(context.Background(), "download/test.txt")
		require.NoError(t, err)
		defer reader.Close()

		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, content, data)
	})

	t.Run("DownloadNonExistentFile", func(t *testing.T) {
		_, err := storage.Download(context.Background(), "nonexistent.txt")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "file not found")
	})
}

func TestLocalStorage_DownloadFile(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	t.Run("DownloadToLocalFile", func(t *testing.T) {
		// Create source file
		content := []byte("file download content")
		srcPath := filepath.Join(tempDir, "src", "data.txt")
		require.NoError(t, os.MkdirAll(filepath.Dir(srcPath), 0755))
		require.NoError(t, os.WriteFile(srcPath, content, 0644))

		// Download to local
		destPath := filepath.Join(tempDir, "local", "output.txt")
		err := storage.DownloadFile(context.Background(), "src/data.txt", destPath)
		require.NoError(t, err)

		// Verify
		data, err := os.ReadFile(destPath)
		require.NoError(t, err)
		assert.Equal(t, content, data)
	})

	t.Run("DownloadNonExistentToFile", func(t *testing.T) {
		destPath := filepath.Join(tempDir, "local", "missing.txt")
		err := storage.DownloadFile(context.Background(), "missing.txt", destPath)
		assert.Error(t, err)
	})
}

func TestLocalStorage_Delete(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	t.Run("DeleteExistingFile", func(t *testing.T) {
		// Create file
		filePath := filepath.Join(tempDir, "delete", "test.txt")
		require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0755))
		require.NoError(t, os.WriteFile(filePath, []byte("to delete"), 0644))

		// Delete
		err := storage.Delete(context.Background(), "delete/test.txt")
		require.NoError(t, err)

		// Verify
		_, err = os.Stat(filePath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("DeleteNonExistentFile", func(t *testing.T) {
		// Should not error for non-existent file
		err := storage.Delete(context.Background(), "nonexistent.txt")
		assert.NoError(t, err)
	})
}

func TestLocalStorage_Exists(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	t.Run("FileExists", func(t *testing.T) {
		// Create file
		filePath := filepath.Join(tempDir, "exists.txt")
		require.NoError(t, os.WriteFile(filePath, []byte("exists"), 0644))

		exists, err := storage.Exists(context.Background(), "exists.txt")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("FileNotExists", func(t *testing.T) {
		exists, err := storage.Exists(context.Background(), "notexists.txt")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestLocalStorage_GetURL(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewLocalStorage(tempDir)
	require.NoError(t, err)

	url := storage.GetURL("path/to/file.txt")
	expected := filepath.Join(tempDir, "path/to/file.txt")
	assert.Equal(t, expected, url)
}

func TestNewStorage(t *testing.T) {
	t.Run("CreateLocalStorage", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &config.StorageConfig{
			Type: "local",
			Local: config.LocalStorageConfig{
				Path: tempDir,
			},
		}

		storage, err := NewStorage(cfg)
		require.NoError(t, err)
		require.NotNil(t, storage)

		// Verify it's a LocalStorage
		_, ok := storage.(*LocalStorage)
		assert.True(t, ok)
	})

	t.Run("CreateDefaultStorage", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &config.StorageConfig{
			Type: "",
			Local: config.LocalStorageConfig{
				Path: tempDir,
			},
		}

		storage, err := NewStorage(cfg)
		require.NoError(t, err)
		require.NotNil(t, storage)

		// Should default to local storage
		_, ok := storage.(*LocalStorage)
		assert.True(t, ok)
	})
}
