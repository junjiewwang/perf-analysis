// Package pprof provides performance profiling collection capabilities.
// It supports two modes: file mode for CLI tools and HTTP mode for long-running services.
package pprof

import (
	"fmt"
	"strings"
	"time"
)

// ModeType defines the pprof collection mode.
type ModeType string

const (
	// ModeFile writes profile data to files at regular intervals.
	ModeFile ModeType = "file"
	// ModeHTTP exposes pprof endpoints via HTTP for on-demand collection.
	ModeHTTP ModeType = "http"
)

// ProfileType defines the type of profile to collect.
type ProfileType string

const (
	ProfileCPU       ProfileType = "cpu"
	ProfileHeap      ProfileType = "heap"
	ProfileGoroutine ProfileType = "goroutine"
	ProfileBlock     ProfileType = "block"
	ProfileMutex     ProfileType = "mutex"
	ProfileAllocs    ProfileType = "allocs"
)

// AllProfileTypes returns all supported profile types.
func AllProfileTypes() []ProfileType {
	return []ProfileType{
		ProfileCPU,
		ProfileHeap,
		ProfileGoroutine,
		ProfileBlock,
		ProfileMutex,
		ProfileAllocs,
	}
}

// DefaultProfileTypes returns the default profile types to collect.
func DefaultProfileTypes() []ProfileType {
	return []ProfileType{ProfileCPU, ProfileHeap, ProfileGoroutine}
}

// ParseProfileTypes parses a comma-separated string into profile types.
func ParseProfileTypes(s string) ([]ProfileType, error) {
	if s == "" {
		return DefaultProfileTypes(), nil
	}

	parts := strings.Split(s, ",")
	types := make([]ProfileType, 0, len(parts))
	valid := make(map[ProfileType]bool)
	for _, pt := range AllProfileTypes() {
		valid[pt] = true
	}

	for _, p := range parts {
		pt := ProfileType(strings.TrimSpace(strings.ToLower(p)))
		if !valid[pt] {
			return nil, fmt.Errorf("unknown profile type: %q", p)
		}
		types = append(types, pt)
	}

	return types, nil
}

// Config holds the pprof configuration.
type Config struct {
	// Enabled indicates whether pprof collection is enabled.
	Enabled bool `mapstructure:"enabled"`

	// Mode specifies the collection mode: file or http.
	Mode ModeType `mapstructure:"mode"`

	// Profiles specifies which profile types to collect.
	Profiles []ProfileType `mapstructure:"profiles"`

	// OutputDir is the directory for profile output files.
	OutputDir string `mapstructure:"output_dir"`

	// FileConfig holds file mode specific configuration.
	FileConfig *FileConfig `mapstructure:"file"`

	// HTTPConfig holds HTTP mode specific configuration.
	HTTPConfig *HTTPConfig `mapstructure:"http"`
}

// FileConfig holds configuration for file mode.
type FileConfig struct {
	// Interval is the time between profile snapshots.
	Interval time.Duration `mapstructure:"interval"`

	// CPUDuration is how long to collect CPU profile data.
	CPUDuration time.Duration `mapstructure:"cpu_duration"`

	// CPURate is the CPU profiling rate in Hz.
	CPURate int `mapstructure:"cpu_rate"`

	// MaxFileSize is the maximum size of a single profile file in bytes.
	MaxFileSize int64 `mapstructure:"max_file_size"`

	// MaxFiles is the maximum number of profile files to keep per type.
	MaxFiles int `mapstructure:"max_files"`

	// AutoRotate enables automatic file rotation.
	AutoRotate bool `mapstructure:"auto_rotate"`
}

// HTTPConfig holds configuration for HTTP mode.
type HTTPConfig struct {
	// Addr is the HTTP server listen address.
	Addr string `mapstructure:"addr"`

	// Path is the URL path prefix for pprof endpoints.
	Path string `mapstructure:"path"`

	// EnableUI enables the pprof web UI.
	EnableUI bool `mapstructure:"enable_ui"`

	// Auth holds authentication configuration.
	Auth *AuthConfig `mapstructure:"auth"`

	// SaveToFile enables saving profiles to files in addition to HTTP responses.
	SaveToFile bool `mapstructure:"save_to_file"`

	// DefaultSeconds is the default duration for CPU profiling requests.
	DefaultSeconds int `mapstructure:"default_seconds"`
}

// AuthConfig holds HTTP authentication configuration.
type AuthConfig struct {
	// Enabled indicates whether authentication is required.
	Enabled bool `mapstructure:"enabled"`

	// Username for basic auth.
	Username string `mapstructure:"username"`

	// Password for basic auth.
	Password string `mapstructure:"password"`

	// Token for token-based auth.
	Token string `mapstructure:"token"`
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		Enabled:   false,
		Mode:      ModeFile,
		Profiles:  DefaultProfileTypes(),
		OutputDir: "./pprof",
		FileConfig: &FileConfig{
			Interval:    30 * time.Second,
			CPUDuration: 10 * time.Second,
			CPURate:     100,
			MaxFileSize: 100 * 1024 * 1024, // 100MB
			MaxFiles:    10,
			AutoRotate:  true,
		},
		HTTPConfig: &HTTPConfig{
			Addr:           ":6060",
			Path:           "/debug/pprof",
			EnableUI:       true,
			SaveToFile:     false,
			DefaultSeconds: 30,
		},
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.Mode != ModeFile && c.Mode != ModeHTTP {
		return fmt.Errorf("invalid pprof mode: %q (valid: file, http)", c.Mode)
	}

	if len(c.Profiles) == 0 {
		return fmt.Errorf("at least one profile type must be specified")
	}

	if c.OutputDir == "" {
		return fmt.Errorf("output directory is required")
	}

	if c.Mode == ModeFile && c.FileConfig != nil {
		if c.FileConfig.Interval < time.Second {
			return fmt.Errorf("interval must be at least 1 second")
		}
		if c.FileConfig.CPUDuration < time.Second {
			return fmt.Errorf("CPU duration must be at least 1 second")
		}
		if c.FileConfig.CPUDuration >= c.FileConfig.Interval {
			return fmt.Errorf("CPU duration must be less than interval")
		}
	}

	if c.Mode == ModeHTTP && c.HTTPConfig != nil {
		if c.HTTPConfig.Addr == "" {
			return fmt.Errorf("HTTP address is required")
		}
	}

	return nil
}

// HasProfile checks if a profile type is enabled.
func (c *Config) HasProfile(pt ProfileType) bool {
	for _, p := range c.Profiles {
		if p == pt {
			return true
		}
	}
	return false
}
