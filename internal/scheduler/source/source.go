// Package source provides task source abstractions for the scheduler.
// It implements the Strategy Pattern where each source type (database, kafka, http)
// is a concrete strategy implementing the TaskSource interface.
package source

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SourceType defines the type of task source.
// Each strategy implementation defines its own constant.
type SourceType string

// TaskSource defines the strategy interface for task sources.
// Each concrete implementation (database, kafka, http) implements this interface.
type TaskSource interface {
	// Type returns the source type constant defined by the strategy.
	Type() SourceType

	// Name returns the instance name (for distinguishing multiple instances of the same type).
	Name() string

	// Start starts the task source.
	Start(ctx context.Context) error

	// Stop stops the task source gracefully.
	Stop() error

	// Tasks returns a channel that emits task events.
	Tasks() <-chan *TaskEvent

	// Ack acknowledges that a task has been successfully processed.
	Ack(ctx context.Context, event *TaskEvent) error

	// Nack indicates that a task processing failed and may need retry.
	Nack(ctx context.Context, event *TaskEvent, reason string) error

	// HealthCheck performs a health check on the source.
	HealthCheck(ctx context.Context) error
}

// SourceConfig holds the configuration for a task source.
type SourceConfig struct {
	// Type is the source type (database, kafka, http).
	Type SourceType `yaml:"type" mapstructure:"type"`

	// Name is the unique name for this source instance.
	Name string `yaml:"name" mapstructure:"name"`

	// Enabled indicates whether this source is enabled.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// Options holds source-specific configuration options.
	Options map[string]interface{} `yaml:"options" mapstructure:"options"`
}

// GetString retrieves a string option with a default value.
func (c *SourceConfig) GetString(key, defaultValue string) string {
	if c.Options == nil {
		return defaultValue
	}
	if v, ok := c.Options[key].(string); ok {
		return v
	}
	return defaultValue
}

// GetInt retrieves an int option with a default value.
func (c *SourceConfig) GetInt(key string, defaultValue int) int {
	if c.Options == nil {
		return defaultValue
	}
	switch v := c.Options[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return defaultValue
}

// GetDuration retrieves a duration option with a default value.
// Accepts string (e.g., "2s") or int (seconds).
func (c *SourceConfig) GetDuration(key string, defaultValue time.Duration) time.Duration {
	if c.Options == nil {
		return defaultValue
	}
	switch v := c.Options[key].(type) {
	case string:
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	case int:
		return time.Duration(v) * time.Second
	case int64:
		return time.Duration(v) * time.Second
	case float64:
		return time.Duration(v) * time.Second
	}
	return defaultValue
}

// GetBool retrieves a bool option with a default value.
func (c *SourceConfig) GetBool(key string, defaultValue bool) bool {
	if c.Options == nil {
		return defaultValue
	}
	if v, ok := c.Options[key].(bool); ok {
		return v
	}
	return defaultValue
}

// GetStringSlice retrieves a string slice option with a default value.
func (c *SourceConfig) GetStringSlice(key string, defaultValue []string) []string {
	if c.Options == nil {
		return defaultValue
	}
	switch v := c.Options[key].(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return defaultValue
}

// SourceCreator is a function that creates a TaskSource from configuration.
type SourceCreator func(cfg *SourceConfig) (TaskSource, error)

// registry holds all registered source creators.
var (
	registry   = make(map[SourceType]SourceCreator)
	registryMu sync.RWMutex
)

// Register registers a source creator for a given source type.
// This is typically called in the init() function of each strategy implementation.
func Register(sourceType SourceType, creator SourceCreator) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[sourceType] = creator
}

// IsRegistered checks if a source type is registered.
func IsRegistered(sourceType SourceType) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, exists := registry[sourceType]
	return exists
}

// RegisteredTypes returns all registered source types.
func RegisteredTypes() []SourceType {
	registryMu.RLock()
	defer registryMu.RUnlock()
	types := make([]SourceType, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	return types
}

// CreateSource creates a TaskSource from the given configuration.
func CreateSource(cfg *SourceConfig) (TaskSource, error) {
	registryMu.RLock()
	creator, exists := registry[cfg.Type]
	registryMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown source type: %s (registered types: %v)", cfg.Type, RegisteredTypes())
	}

	return creator(cfg)
}

// CreateSources creates multiple TaskSources from configurations.
// Only enabled sources are created.
func CreateSources(configs []*SourceConfig) ([]TaskSource, error) {
	var sources []TaskSource

	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}

		src, err := CreateSource(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create source %q: %w", cfg.Name, err)
		}

		sources = append(sources, src)
	}

	return sources, nil
}
