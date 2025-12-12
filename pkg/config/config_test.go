package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultValues(t *testing.T) {
	// Create a minimal config file
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	content := `
database:
  host: localhost
  type: postgres
storage:
  type: local
`
	err := os.WriteFile(configFile, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(configFile)
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	// Check default values
	assert.Equal(t, "1.0.0", cfg.Analysis.Version)
	assert.Equal(t, "./data", cfg.Analysis.DataDir)
	assert.Equal(t, 5, cfg.Analysis.MaxWorker)
	assert.Equal(t, 2, cfg.Scheduler.PollInterval)
	assert.Equal(t, 5, cfg.Scheduler.WorkerCount)
}

func TestLoad_CustomValues(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	content := `
analysis:
  version: "2.0.0"
  data_dir: "/tmp/data"
  max_worker: 10
database:
  type: postgres
  host: db.example.com
  port: 5432
  database: perf_analysis
  user: admin
  password: secret
storage:
  type: local
  local_path: /tmp/storage
scheduler:
  poll_interval: 5
  worker_count: 8
`
	err := os.WriteFile(configFile, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(configFile)
	require.NoError(t, err)

	assert.Equal(t, "2.0.0", cfg.Analysis.Version)
	assert.Equal(t, "/tmp/data", cfg.Analysis.DataDir)
	assert.Equal(t, 10, cfg.Analysis.MaxWorker)
	assert.Equal(t, "db.example.com", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, "perf_analysis", cfg.Database.Database)
	assert.Equal(t, 8, cfg.Scheduler.WorkerCount)
}

func TestLoad_InvalidDatabaseType(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	content := `
database:
  type: sqlite
  host: localhost
storage:
  type: local
`
	err := os.WriteFile(configFile, []byte(content), 0644)
	require.NoError(t, err)

	_, err = Load(configFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported database type")
}

// Note: Storage validation tests moved to internal/storage package

func TestLoad_COSWithCredentials(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	content := `
database:
  type: postgres
  host: localhost
storage:
  type: cos
  bucket: test-bucket
  region: ap-guangzhou
  secret_id: test-id
  secret_key: test-key
`
	err := os.WriteFile(configFile, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(configFile)
	require.NoError(t, err)
	assert.Equal(t, "cos", cfg.Storage.Type)
	assert.Equal(t, "test-bucket", cfg.Storage.Bucket)
}

func TestValidate_EmptyHost(t *testing.T) {
	cfg := &Config{
		Database: DatabaseConfig{
			Type: "postgres",
			Host: "",
		},
		Storage: StorageConfig{
			Type: "local",
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database host is required")
}

func TestValidate_InvalidWorkerCount(t *testing.T) {
	cfg := &Config{
		Database: DatabaseConfig{
			Type: "postgres",
			Host: "localhost",
		},
		Storage: StorageConfig{
			Type: "local",
		},
		Scheduler: SchedulerConfig{
			WorkerCount: 0,
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker count must be at least 1")
}

func TestGetTaskDir(t *testing.T) {
	cfg := &Config{
		Analysis: AnalysisConfig{
			DataDir: "/tmp/data",
		},
	}

	taskDir := cfg.GetTaskDir("task-uuid-123")
	assert.Equal(t, "/tmp/data/task-uuid-123", taskDir)
}

func TestEnsureDataDir(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "analysis", "data")

	cfg := &Config{
		Analysis: AnalysisConfig{
			DataDir: dataDir,
		},
	}

	err := cfg.EnsureDataDir()
	require.NoError(t, err)

	_, err = os.Stat(dataDir)
	assert.NoError(t, err)
}

func TestLoad_FileNotFound(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	// Should not return error, use defaults
	require.NoError(t, err)
	assert.NotNil(t, cfg)
}

func TestLoadFromReader(t *testing.T) {
	content := []byte(`
database:
  type: mysql
  host: mysql.local
storage:
  type: local
`)
	cfg, err := LoadFromReader("yaml", content)
	require.NoError(t, err)
	assert.Equal(t, "mysql", cfg.Database.Type)
	assert.Equal(t, "mysql.local", cfg.Database.Host)
}
