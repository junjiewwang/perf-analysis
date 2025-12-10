package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

// DBConfig holds database configuration.
type DBConfig struct {
	Type     string `mapstructure:"type"` // postgres or mysql
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	MaxConns int    `mapstructure:"max_conns"`
}

// DBType represents the database type.
type DBType string

const (
	DBTypePostgres DBType = "postgres"
	DBTypeMySQL    DBType = "mysql"
)

// NewDB creates a new database connection based on configuration.
func NewDB(cfg *DBConfig) (*sql.DB, error) {
	var dsn string
	var driverName string

	switch DBType(cfg.Type) {
	case DBTypePostgres, DBType("postgresql"):
		driverName = "postgres"
		dsn = fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database,
		)
	case DBTypeMySQL:
		driverName = "mysql"
		dsn = fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=Local",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database,
		)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	maxConns := cfg.MaxConns
	if maxConns <= 0 {
		maxConns = 10
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns / 2)
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(30 * time.Minute)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// Repositories holds all repository instances.
type Repositories struct {
	Task       TaskRepository
	Result     ResultRepository
	Suggestion SuggestionRepository
	MasterTask MasterTaskRepository
	db         *sql.DB
	dbType     string
}

// NewRepositories creates all repositories based on database type.
func NewRepositories(db *sql.DB, dbType string, version string) *Repositories {
	repos := &Repositories{db: db, dbType: dbType}

	switch DBType(dbType) {
	case DBTypePostgres, DBType("postgresql"):
		repos.Task = NewPostgresTaskRepository(db)
		repos.Result = NewPostgresResultRepository(db, version)
		repos.Suggestion = NewPostgresSuggestionRepository(db)
		repos.MasterTask = NewPostgresMasterTaskRepository(db)
	case DBTypeMySQL:
		repos.Task = NewMySQLTaskRepository(db)
		repos.Result = NewMySQLResultRepository(db, version)
		repos.Suggestion = NewMySQLSuggestionRepository(db)
		repos.MasterTask = NewMySQLMasterTaskRepository(db)
	default:
		// Default to PostgreSQL
		repos.Task = NewPostgresTaskRepository(db)
		repos.Result = NewPostgresResultRepository(db, version)
		repos.Suggestion = NewPostgresSuggestionRepository(db)
		repos.MasterTask = NewPostgresMasterTaskRepository(db)
	}

	return repos
}

// Close closes the database connection.
func (r *Repositories) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

// HealthCheck verifies the database connection is still alive.
func (r *Repositories) HealthCheck(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

// DB returns the underlying database connection.
func (r *Repositories) DB() *sql.DB {
	return r.db
}
