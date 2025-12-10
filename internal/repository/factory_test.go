package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNewRepositories(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect ping for health check
	mock.ExpectPing()

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
		// Should default to PostgreSQL
		assert.NotNil(t, repos.Task)
	})
}

func TestRepositories_Close(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repos := NewRepositories(db, "postgres", "1.0.0")

	mock.ExpectClose()

	err = repos.Close()
	assert.NoError(t, err)
}

func TestRepositories_DB(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repos := NewRepositories(db, "postgres", "1.0.0")
	assert.Equal(t, db, repos.DB())
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
