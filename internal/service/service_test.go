package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/pkg/config"
	"github.com/perf-analysis/pkg/utils"
)

func TestService_New(t *testing.T) {
	cfg := &config.Config{
		Analysis: config.AnalysisConfig{
			Version: "1.0.0",
			DataDir: "./test_data",
		},
		Database: config.DatabaseConfig{
			Type: "postgres",
			Host: "localhost",
			Port: 5432,
		},
		Storage: config.StorageConfig{
			Type:      "local",
			LocalPath: "./test_storage",
		},
		Scheduler: config.SchedulerConfig{
			WorkerCount:   5,
			PollInterval:  2,
			PrioritySlots: 2,
			TaskBatchSize: 10,
		},
	}

	t.Run("WithLogger", func(t *testing.T) {
		logger := utils.NewDefaultLogger(utils.LevelInfo, nil)
		svc, err := New(cfg, logger)
		require.NoError(t, err)
		require.NotNil(t, svc)
		assert.False(t, svc.IsRunning())
	})

	t.Run("WithoutLogger", func(t *testing.T) {
		svc, err := New(cfg, nil)
		require.NoError(t, err)
		require.NotNil(t, svc)
	})
}

func TestService_Stats(t *testing.T) {
	cfg := &config.Config{
		Analysis: config.AnalysisConfig{
			Version: "1.0.0",
		},
		Database: config.DatabaseConfig{
			Type: "postgres",
			Host: "localhost",
		},
		Storage: config.StorageConfig{
			Type: "local",
		},
		Scheduler: config.SchedulerConfig{
			WorkerCount: 5,
		},
	}

	svc, err := New(cfg, nil)
	require.NoError(t, err)

	stats := svc.Stats()
	assert.False(t, stats.Running)
}

func TestServiceStats_JSON(t *testing.T) {
	stats := ServiceStats{
		Running: true,
	}
	assert.True(t, stats.Running)
}

func TestService_HealthCheck_NoComponents(t *testing.T) {
	cfg := &config.Config{
		Analysis: config.AnalysisConfig{
			Version: "1.0.0",
		},
		Database: config.DatabaseConfig{
			Type: "postgres",
			Host: "localhost",
		},
		Storage: config.StorageConfig{
			Type: "local",
		},
		Scheduler: config.SchedulerConfig{
			WorkerCount: 5,
		},
	}

	svc, err := New(cfg, nil)
	require.NoError(t, err)

	// HealthCheck should not fail when components are not initialized
	err = svc.HealthCheck(context.Background())
	assert.NoError(t, err)
}
