// Package config provides configuration management for the perf-analysis service.
package config

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"

	"github.com/perf-analysis/pkg/pprof"
)

// Config holds all configuration for the application.
type Config struct {
	Analysis  AnalysisConfig  `mapstructure:"analysis"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Sources   []SourceConfig  `mapstructure:"sources"`
	Ingress   IngressConfig   `mapstructure:"ingress"`
	Log       LogConfig       `mapstructure:"log"`
	Pprof     *pprof.Config   `mapstructure:"pprof"`
	ViewURL   ViewURLConfig   `mapstructure:"view_url"`
	WebUI     WebUIConfig     `mapstructure:"webui"`
	Retention RetentionConfig `mapstructure:"retention"`
	Callback  CallbackConfig  `mapstructure:"callback"`
}

// SourceConfig holds configuration for a task source.
type SourceConfig struct {
	Type    string                 `mapstructure:"type"`    // database, kafka
	Name    string                 `mapstructure:"name"`    // unique name for this source
	Enabled bool                   `mapstructure:"enabled"` // whether this source is enabled
	Options map[string]interface{} `mapstructure:"options"` // source-specific options
}

// AnalysisConfig holds analysis-related configuration.
type AnalysisConfig struct {
	Version string `mapstructure:"version"`
	DataDir string `mapstructure:"data_dir"`
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	Type        string `mapstructure:"type"` // postgres or mysql
	Host        string `mapstructure:"host"`
	Port        int    `mapstructure:"port"`
	Database    string `mapstructure:"database"`
	User        string `mapstructure:"user"`
	Password    string `mapstructure:"password"`
	MaxConns    int    `mapstructure:"max_conns"`
	AutoMigrate bool   `mapstructure:"auto_migrate"` // auto-migrate database schema on startup
}

// StorageConfig holds object storage configuration.
// Type selects which sub-config is active; only the matching sub-config is used.
type StorageConfig struct {
	Type  string             `mapstructure:"type"` // cos or local
	COS   COSStorageConfig   `mapstructure:"cos"`
	Local LocalStorageConfig `mapstructure:"local"`
}

// COSStorageConfig holds COS-specific storage configuration.
type COSStorageConfig struct {
	Bucket    string `mapstructure:"bucket"`
	Region    string `mapstructure:"region"`
	SecretID  string `mapstructure:"secret_id"`
	SecretKey string `mapstructure:"secret_key"`
	Domain    string `mapstructure:"domain"` // e.g., "myqcloud.com"
	Scheme    string `mapstructure:"scheme"` // e.g., "https" or "http"
}

// LocalStorageConfig holds local filesystem storage configuration.
type LocalStorageConfig struct {
	Path string `mapstructure:"path"`
}

// SchedulerConfig holds scheduler configuration.
type SchedulerConfig struct {
	Enabled       bool   `mapstructure:"enabled"`        // whether to enable the scheduler
	PollInterval  string `mapstructure:"poll_interval"`  // e.g., "2s"
	WorkerCount   int    `mapstructure:"worker_count"`
	PrioritySlots int    `mapstructure:"priority_slots"`
	TaskBatchSize int    `mapstructure:"task_batch_size"`
}

// IngressConfig holds ingress configuration.
type IngressConfig struct {
	HTTP HTTPIngressConfig `mapstructure:"http"`
}

// HTTPIngressConfig holds HTTP ingress configuration.
type HTTPIngressConfig struct {
	Enabled      bool   `mapstructure:"enabled"`       // whether to enable HTTP ingress
	ListenAddr   string `mapstructure:"listen_addr"`   // e.g., ":8081"
	Path         string `mapstructure:"path"`           // HTTP path for receiving tasks
	ReadTimeout  string `mapstructure:"read_timeout"`   // e.g., "30s"
	WriteTimeout string `mapstructure:"write_timeout"`  // e.g., "30s"
	MaxBodySize  int64  `mapstructure:"max_body_size"`  // max request body in bytes
	CallbackURL  string `mapstructure:"callback_url"`   // ingress-level callback URL (downgrade-save)
}

// GetReadTimeout returns the read timeout duration.
func (c *HTTPIngressConfig) GetReadTimeout() time.Duration {
	if c.ReadTimeout == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(c.ReadTimeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// GetWriteTimeout returns the write timeout duration.
func (c *HTTPIngressConfig) GetWriteTimeout() time.Duration {
	if c.WriteTimeout == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(c.WriteTimeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level      string `mapstructure:"level"`
	OutputPath string `mapstructure:"output_path"`
	Format     string `mapstructure:"format"` // json or text
}

// ViewURLConfig holds configuration for generating signed view URLs.
// This is used by analyzer to produce callback URLs and by WebUI to validate them.
type ViewURLConfig struct {
	BaseURL string         `mapstructure:"base_url"` // e.g., "https://perf.example.com"
	Auth    ViewAuthConfig `mapstructure:"auth"`
}

// ViewAuthConfig holds authentication configuration for view URL signing and validation.
type ViewAuthConfig struct {
	Enabled        bool     `mapstructure:"enabled"`
	Secret         string   `mapstructure:"secret"`          // HMAC signing secret
	AllowedOrigins []string `mapstructure:"allowed_origins"` // allowed origins for iframe embedding
}

// WebUIConfig holds WebUI HTTP server configuration.
type WebUIConfig struct {
	Enabled  bool   `mapstructure:"enabled"`   // whether to start embedded WebUI server
	Port     int    `mapstructure:"port"`      // WebUI listen port
	CacheDir string `mapstructure:"cache_dir"` // local cache directory for remote storage
	CacheMax int64  `mapstructure:"cache_max"` // max cache size in bytes (0 = unlimited)
}

// RetentionConfig holds result retention configuration.
type RetentionConfig struct {
	Default string           `mapstructure:"default"` // default retention duration, e.g., "168h" (7 days)
	Rules   []RetentionRule  `mapstructure:"rules"`   // per-task-type overrides
}

// RetentionRule defines retention duration for a specific task type.
type RetentionRule struct {
	TaskType string `mapstructure:"task_type"` // task type name
	Duration string `mapstructure:"duration"`  // retention duration, e.g., "720h" (30 days)
}

// CallbackConfig holds callback notification configuration.
type CallbackConfig struct {
	DefaultURL string `mapstructure:"default_url"` // global default callback URL
	Timeout    string `mapstructure:"timeout"`     // callback HTTP timeout, e.g., "10s"
	MaxRetries int    `mapstructure:"max_retries"` // max retry attempts
}

// GetTimeout returns the callback timeout duration.
func (c *CallbackConfig) GetTimeout() time.Duration {
	if c.Timeout == "" {
		return 10 * time.Second
	}
	d, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

// GetDefaultRetention returns the default retention duration.
func (r *RetentionConfig) GetDefaultRetention() time.Duration {
	if r.Default == "" {
		return 7 * 24 * time.Hour // default 7 days
	}
	d, err := time.ParseDuration(r.Default)
	if err != nil {
		return 7 * 24 * time.Hour
	}
	return d
}

// GetRetentionForType returns the retention duration for a given task type.
func (r *RetentionConfig) GetRetentionForType(taskType string) time.Duration {
	for _, rule := range r.Rules {
		if rule.TaskType == taskType {
			d, err := time.ParseDuration(rule.Duration)
			if err != nil {
				return r.GetDefaultRetention()
			}
			return d
		}
	}
	return r.GetDefaultRetention()
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

	// Database defaults
	v.SetDefault("database.type", "postgres")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.max_conns", 10)
	v.SetDefault("database.auto_migrate", false)

	// Storage defaults
	v.SetDefault("storage.type", "local")
	v.SetDefault("storage.local.path", "./storage")

	// Scheduler defaults
	v.SetDefault("scheduler.enabled", true)
	v.SetDefault("scheduler.poll_interval", "2s")
	v.SetDefault("scheduler.worker_count", 5)
	v.SetDefault("scheduler.priority_slots", 2)
	v.SetDefault("scheduler.task_batch_size", 10)

	// Ingress defaults
	v.SetDefault("ingress.http.enabled", false)
	v.SetDefault("ingress.http.listen_addr", ":8081")
	v.SetDefault("ingress.http.path", "/tasks")
	v.SetDefault("ingress.http.read_timeout", "30s")
	v.SetDefault("ingress.http.write_timeout", "30s")
	v.SetDefault("ingress.http.max_body_size", 1<<20)
	v.SetDefault("ingress.http.callback_url", "")

	// Log defaults
	v.SetDefault("log.level", "info")
	v.SetDefault("log.output_path", "./logs")
	v.SetDefault("log.format", "text")

	// ViewURL defaults
	v.SetDefault("view_url.base_url", "")
	v.SetDefault("view_url.auth.enabled", false)
	v.SetDefault("view_url.auth.secret", "")
	v.SetDefault("view_url.auth.allowed_origins", []string{})

	// WebUI defaults
	v.SetDefault("webui.enabled", false)
	v.SetDefault("webui.port", 8080)
	v.SetDefault("webui.cache_dir", "./cache")
	v.SetDefault("webui.cache_max", 0)

	// Retention defaults
	v.SetDefault("retention.default", "168h")
	v.SetDefault("retention.rules", []RetentionRule{})

	// Callback defaults
	v.SetDefault("callback.default_url", "")
	v.SetDefault("callback.timeout", "10s")
	v.SetDefault("callback.max_retries", 3)

	// Pprof defaults
	v.SetDefault("pprof.enabled", false)
	v.SetDefault("pprof.mode", "http")
	v.SetDefault("pprof.output_dir", "./pprof")
	v.SetDefault("pprof.profiles", []string{"cpu", "heap", "goroutine"})
	v.SetDefault("pprof.file.interval", "30s")
	v.SetDefault("pprof.file.cpu_duration", "10s")
	v.SetDefault("pprof.file.cpu_rate", 100)
	v.SetDefault("pprof.file.max_file_size", 104857600)
	v.SetDefault("pprof.file.max_files", 10)
	v.SetDefault("pprof.file.auto_rotate", true)
	v.SetDefault("pprof.http.addr", ":6060")
	v.SetDefault("pprof.http.path", "/debug/pprof")
	v.SetDefault("pprof.http.enable_ui", true)
	v.SetDefault("pprof.http.save_to_file", false)
	v.SetDefault("pprof.http.default_seconds", 30)
}

// Validate validates the configuration by delegating to each component's Validate method.
func (c *Config) Validate() error {
	validators := []struct {
		name string
		v    Validatable
	}{
		{"database", &c.Database},
		{"scheduler", &c.Scheduler},
		{"ingress", &c.Ingress},
		{"log", &c.Log},
		{"webui", &c.WebUI},
		{"view_url", &c.ViewURL},
		{"retention", &c.Retention},
		{"callback", &c.Callback},
	}

	for _, item := range validators {
		if err := item.v.Validate(); err != nil {
			return err
		}
	}

	// Validate sources common fields (name uniqueness, required fields)
	if err := validateSources(c.Sources); err != nil {
		return err
	}

	return nil
}
