// Package config provides configuration management for the perf-analysis service.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application.
type Config struct {
	Analysis  AnalysisConfig  `mapstructure:"analysis"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Storage   StorageConfig   `mapstructure:"storage"`
	APM       APMConfig       `mapstructure:"apm"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Log       LogConfig       `mapstructure:"log"`
}

// AnalysisConfig holds analysis-related configuration.
type AnalysisConfig struct {
	Version   string `mapstructure:"version"`
	DataDir   string `mapstructure:"data_dir"`
	MaxWorker int    `mapstructure:"max_worker"`
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	Type     string `mapstructure:"type"` // postgres or mysql
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	MaxConns int    `mapstructure:"max_conns"`
}

// StorageConfig holds object storage configuration.
type StorageConfig struct {
	Type      string `mapstructure:"type"` // cos or local
	Bucket    string `mapstructure:"bucket"`
	Region    string `mapstructure:"region"`
	SecretID  string `mapstructure:"secret_id"`
	SecretKey string `mapstructure:"secret_key"`
	Domain    string `mapstructure:"domain"`     // e.g., "myqcloud.com"
	Scheme    string `mapstructure:"scheme"`     // e.g., "https" or "http"
	LocalPath string `mapstructure:"local_path"` // for local storage
}

// APMConfig holds APM callback configuration.
type APMConfig struct {
	URL           string `mapstructure:"url"`
	RequestYunAPI bool   `mapstructure:"request_yunapi"`
	Enabled       bool   `mapstructure:"enabled"`
}

// SchedulerConfig holds scheduler configuration.
type SchedulerConfig struct {
	PollInterval  int `mapstructure:"poll_interval"` // in seconds
	WorkerCount   int `mapstructure:"worker_count"`
	PrioritySlots int `mapstructure:"priority_slots"`
	TaskBatchSize int `mapstructure:"task_batch_size"`
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level      string `mapstructure:"level"`
	OutputPath string `mapstructure:"output_path"`
	Format     string `mapstructure:"format"` // json or text
}

// Load reads configuration from the specified file path.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set default values
	setDefaults(v)

	// Determine config file path
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Look for config in standard locations
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
		v.AddConfigPath("/etc/perf-analysis")
	}

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		// Check if it's a "file not found" error (either viper's type or os error)
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, use defaults
			fmt.Println("Config file not found, using defaults")
		} else if os.IsNotExist(err) {
			// File specified but doesn't exist, use defaults
			fmt.Printf("Config file %s not found, using defaults\n", configPath)
		} else {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Allow environment variables to override config
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// LoadFromReader loads configuration from an io.Reader (useful for testing).
func LoadFromReader(configType string, content []byte) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetConfigType(configType)
	if err := v.ReadConfig(bytes.NewReader(content)); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values.
func setDefaults(v *viper.Viper) {
	// Analysis defaults
	v.SetDefault("analysis.version", "1.0.0")
	v.SetDefault("analysis.data_dir", "./data")
	v.SetDefault("analysis.max_worker", 5)

	// Database defaults
	v.SetDefault("database.type", "postgres")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.max_conns", 10)

	// Storage defaults
	v.SetDefault("storage.type", "local")
	v.SetDefault("storage.local_path", "./storage")

	// Scheduler defaults
	v.SetDefault("scheduler.poll_interval", 2)
	v.SetDefault("scheduler.worker_count", 5)
	v.SetDefault("scheduler.priority_slots", 2)
	v.SetDefault("scheduler.task_batch_size", 10)

	// Log defaults
	v.SetDefault("log.level", "info")
	v.SetDefault("log.output_path", "./logs")
	v.SetDefault("log.format", "text")
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate database config
	if c.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if c.Database.Type != "postgres" && c.Database.Type != "mysql" {
		return fmt.Errorf("unsupported database type: %s", c.Database.Type)
	}

	// Storage config validation is delegated to storage package

	// Validate scheduler config
	if c.Scheduler.WorkerCount < 1 {
		return fmt.Errorf("worker count must be at least 1")
	}

	return nil
}

// EnsureDataDir creates the data directory if it doesn't exist.
func (c *Config) EnsureDataDir() error {
	if c.Analysis.DataDir == "" {
		return nil
	}
	return os.MkdirAll(c.Analysis.DataDir, 0755)
}

// GetTaskDir returns the task-specific directory path.
func (c *Config) GetTaskDir(taskUUID string) string {
	return filepath.Join(c.Analysis.DataDir, taskUUID)
}
