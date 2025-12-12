package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestGormDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	return db
}

func TestNewRepositories(t *testing.T) {
	db := newTestGormDB(t)

	t.Run("PostgreSQL", func(t *testing.T) {
		repos := NewRepositories(db, "postgres", "1.0.0")
		require.NotNil(t, repos)
		assert.NotNil(t, repos.Task)
		assert.NotNil(t, repos.Result)
		assert.NotNil(t, repos.Suggestion)
		assert.NotNil(t, repos.MasterTask)
	})

	t.Run("PostgreSQL_Alt", func(t *testing.T) {
		repos := NewRepositories(db, "postgresql", "1.0.0")
		require.NotNil(t, repos)
		assert.NotNil(t, repos.Task)
	})

	t.Run("MySQL", func(t *testing.T) {
		repos := NewRepositories(db, "mysql", "1.0.0")
		require.NotNil(t, repos)
		assert.NotNil(t, repos.Task)
		assert.NotNil(t, repos.Result)
		assert.NotNil(t, repos.Suggestion)
		assert.NotNil(t, repos.MasterTask)
	})

	t.Run("Default", func(t *testing.T) {
		repos := NewRepositories(db, "unknown", "1.0.0")
		require.NotNil(t, repos)
		assert.NotNil(t, repos.Task)
	})
}

func TestRepositories_Close(t *testing.T) {
	db := newTestGormDB(t)
	repos := NewRepositories(db, "postgres", "1.0.0")

	err := repos.Close()
	assert.NoError(t, err)
}

func TestRepositories_DB(t *testing.T) {
	db := newTestGormDB(t)
	repos := NewRepositories(db, "postgres", "1.0.0")

	sqlDB := repos.DB()
	assert.NotNil(t, sqlDB)
}

func TestRepositories_GormDB(t *testing.T) {
	db := newTestGormDB(t)
	repos := NewRepositories(db, "postgres", "1.0.0")

	gormDB := repos.GormDB()
	assert.Equal(t, db, gormDB)
}

func TestDBConfig_Validation(t *testing.T) {
	t.Run("ValidPostgresConfig", func(t *testing.T) {
		cfg := &DBConfig{
			Type:     "postgres",
			Host:     "localhost",
			Port:     5432,
			Database: "testdb",
			User:     "testuser",
			Password: "testpass",
			MaxConns: 10,
		}
		assert.Equal(t, "postgres", cfg.Type)
		assert.Equal(t, 5432, cfg.Port)
	})

	t.Run("ValidMySQLConfig", func(t *testing.T) {
		cfg := &DBConfig{
			Type:     "mysql",
			Host:     "localhost",
			Port:     3306,
			Database: "testdb",
			User:     "testuser",
			Password: "testpass",
			MaxConns: 10,
		}
		assert.Equal(t, "mysql", cfg.Type)
		assert.Equal(t, 3306, cfg.Port)
	})
}
