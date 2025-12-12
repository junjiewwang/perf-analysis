// Package service provides the main application service that integrates all components.
package service

import (
	"context"
	"fmt"

	"github.com/perf-analysis/internal/repository"
	"github.com/perf-analysis/internal/scheduler"
	"github.com/perf-analysis/internal/storage"
	"github.com/perf-analysis/pkg/config"
	"github.com/perf-analysis/pkg/utils"
)

// Service is the main application service.
type Service struct {
	config    *config.Config
	logger    utils.Logger
	db        *repository.Repositories
	storage   storage.Storage
	scheduler *scheduler.Scheduler

	running bool
}

// New creates a new Service instance.
func New(cfg *config.Config, logger utils.Logger) (*Service, error) {
	if logger == nil {
		logger = utils.NewDefaultLogger(utils.LevelInfo, nil)
	}

	return &Service{
		config: cfg,
		logger: logger,
	}, nil
}

// Initialize initializes all service components.
func (s *Service) Initialize(ctx context.Context) error {
	s.logger.Info("Initializing service components...")

	// Initialize database connection
	if err := s.initDatabase(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize storage
	if err := s.initStorage(); err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize scheduler
	if err := s.initScheduler(); err != nil {
		return fmt.Errorf("failed to initialize scheduler: %w", err)
	}

	s.logger.Info("Service components initialized successfully")
	return nil
}

// initDatabase initializes the database connection and repositories.
func (s *Service) initDatabase() error {
	s.logger.Info("Connecting to database (%s)...", s.config.Database.Type)

	dbConfig := &repository.DBConfig{
		Type:     s.config.Database.Type,
		Host:     s.config.Database.Host,
		Port:     s.config.Database.Port,
		Database: s.config.Database.Database,
		User:     s.config.Database.User,
		Password: s.config.Database.Password,
		MaxConns: s.config.Database.MaxConns,
	}

	gormDB, err := repository.NewGormDB(dbConfig)
	if err != nil {
		return err
	}

	s.db = repository.NewRepositories(gormDB, s.config.Database.Type, s.config.Analysis.Version)
	s.logger.Info("Database connection established")

	return nil
}

// initStorage initializes the object storage.
func (s *Service) initStorage() error {
	s.logger.Info("Initializing storage (%s)...", s.config.Storage.Type)

	store, err := storage.NewStorage(&s.config.Storage)
	if err != nil {
		return err
	}

	s.storage = store
	s.logger.Info("Storage initialized")

	return nil
}

// initScheduler initializes the task scheduler.
func (s *Service) initScheduler() error {
	s.logger.Info("Initializing scheduler...")

	// Create task fetcher
	fetcher := scheduler.NewRepositoryTaskFetcher(s.db.Task, s.db.Suggestion)

	// Create task processor
	processorConfig := &scheduler.ProcessorConfig{
		Config:  s.config,
		Storage: s.storage,
		Repos:   s.db,
		Logger:  s.logger,
	}
	processor := scheduler.NewDefaultTaskProcessor(processorConfig)

	// Create scheduler
	schedulerConfig := scheduler.FromConfig(&s.config.Scheduler)
	s.scheduler = scheduler.New(schedulerConfig, fetcher, processor, s.logger)

	s.logger.Info("Scheduler initialized")
	return nil
}

// Start starts the service.
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting service...")

	if err := s.scheduler.Start(ctx); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	s.running = true
	s.logger.Info("Service started successfully")

	return nil
}

// Stop stops the service gracefully.
func (s *Service) Stop() error {
	s.logger.Info("Stopping service...")

	if s.scheduler != nil {
		s.scheduler.Stop()
	}

	if s.db != nil {
		if err := s.db.Close(); err != nil {
			s.logger.Error("Failed to close database connection: %v", err)
		}
	}

	s.running = false
	s.logger.Info("Service stopped")

	return nil
}

// IsRunning returns whether the service is running.
func (s *Service) IsRunning() bool {
	return s.running
}

// Stats returns service statistics.
func (s *Service) Stats() ServiceStats {
	stats := ServiceStats{
		Running: s.running,
	}

	if s.scheduler != nil {
		stats.Scheduler = s.scheduler.Stats()
	}

	return stats
}

// HealthCheck performs a health check on the service.
func (s *Service) HealthCheck(ctx context.Context) error {
	// Check database connection
	if s.db != nil {
		if err := s.db.HealthCheck(ctx); err != nil {
			return fmt.Errorf("database health check failed: %w", err)
		}
	}

	return nil
}

// ServiceStats holds service statistics.
type ServiceStats struct {
	Running   bool                     `json:"running"`
	Scheduler scheduler.SchedulerStats `json:"scheduler"`
}
