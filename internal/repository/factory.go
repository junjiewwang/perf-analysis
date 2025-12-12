package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/perf-analysis/pkg/telemetry"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
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

// NewGormDB creates a new GORM database connection based on configuration.
func NewGormDB(cfg *DBConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch DBType(cfg.Type) {
	case DBTypePostgres, DBType("postgresql"):
		dsn := fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database,
		)
		dialector = postgres.Open(dsn)
	case DBTypeMySQL:
		dsn := fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=Local",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database,
		)
		dialector = mysql.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable OpenTelemetry tracing if OTEL_ENABLED=true
	if telemetry.Enabled() {
		if err := db.Use(tracing.NewPlugin()); err != nil {
			return nil, fmt.Errorf("failed to enable telemetry: %w", err)
		}
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	maxConns := cfg.MaxConns
	if maxConns <= 0 {
		maxConns = 10
	}
	sqlDB.SetMaxOpenConns(maxConns)
	sqlDB.SetMaxIdleConns(maxConns / 2)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(30 * time.Minute)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
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
	gormDB     *gorm.DB
	dbType     string
}

// NewRepositories creates all repositories using GORM.
func NewRepositories(gormDB *gorm.DB, dbType string, version string) *Repositories {
	repos := &Repositories{gormDB: gormDB, dbType: dbType}

	repos.Task = NewGormTaskRepository(gormDB)
	repos.Result = NewGormResultRepository(gormDB, version)
	repos.Suggestion = NewGormSuggestionRepository(gormDB)
	repos.MasterTask = NewGormMasterTaskRepository(gormDB)

	return repos
}

// Close closes the database connection.
func (r *Repositories) Close() error {
	if r.gormDB != nil {
		sqlDB, err := r.gormDB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// HealthCheck verifies the database connection is still alive.
func (r *Repositories) HealthCheck(ctx context.Context) error {
	sqlDB, err := r.gormDB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// DB returns the underlying sql.DB connection.
func (r *Repositories) DB() *sql.DB {
	sqlDB, _ := r.gormDB.DB()
	return sqlDB
}

// GormDB returns the underlying GORM DB instance.
func (r *Repositories) GormDB() *gorm.DB {
	return r.gormDB
}
