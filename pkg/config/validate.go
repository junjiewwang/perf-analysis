// Package config provides configuration management for the perf-analysis service.
package config

import (
	"fmt"
	"strings"
	"time"
)

// Validatable defines the interface for configuration components that support validation.
type Validatable interface {
	Validate() error
}

// --- Shared validation utilities ---

// validateRequired checks that a string field is not empty.
func validateRequired(field, name string) error {
	if strings.TrimSpace(field) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

// validateDuration checks that a string is a valid Go duration.
// Empty string is accepted when allowEmpty is true.
func validateDuration(field, name string, allowEmpty bool) error {
	if field == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("%s is required", name)
	}
	if _, err := time.ParseDuration(field); err != nil {
		return fmt.Errorf("%s is not a valid duration: %w", name, err)
	}
	return nil
}

// validatePort checks that a port number is within the valid range.
func validatePort(port int, name string) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535, got %d", name, port)
	}
	return nil
}

// validateOneOf checks that a string value is one of the allowed values.
func validateOneOf(field, name string, allowed []string) error {
	for _, a := range allowed {
		if field == a {
			return nil
		}
	}
	return fmt.Errorf("%s must be one of %v, got %q", name, allowed, field)
}

// --- Component-level Validate implementations ---

// Validate validates the database configuration.
func (c *DatabaseConfig) Validate() error {
	if err := validateRequired(c.Host, "database.host"); err != nil {
		return err
	}
	if err := validateOneOf(c.Type, "database.type", []string{"postgres", "mysql"}); err != nil {
		return err
	}
	if err := validatePort(c.Port, "database.port"); err != nil {
		return err
	}
	if c.MaxConns < 1 {
		return fmt.Errorf("database.max_conns must be at least 1, got %d", c.MaxConns)
	}
	return nil
}

// Validate validates the scheduler configuration (only meaningful when Enabled is true).
func (c *SchedulerConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.WorkerCount < 1 {
		return fmt.Errorf("scheduler.worker_count must be at least 1, got %d", c.WorkerCount)
	}
	if err := validateDuration(c.PollInterval, "scheduler.poll_interval", false); err != nil {
		return err
	}
	if c.PrioritySlots < 0 || c.PrioritySlots >= c.WorkerCount {
		return fmt.Errorf("scheduler.priority_slots must be in [0, worker_count), got %d", c.PrioritySlots)
	}
	if c.TaskBatchSize < 1 {
		return fmt.Errorf("scheduler.task_batch_size must be at least 1, got %d", c.TaskBatchSize)
	}
	return nil
}

// Validate validates the HTTP ingress configuration (only meaningful when Enabled is true).
func (c *HTTPIngressConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if err := validateRequired(c.ListenAddr, "ingress.http.listen_addr"); err != nil {
		return err
	}
	if err := validateRequired(c.Path, "ingress.http.path"); err != nil {
		return err
	}
	if err := validateDuration(c.ReadTimeout, "ingress.http.read_timeout", true); err != nil {
		return err
	}
	if err := validateDuration(c.WriteTimeout, "ingress.http.write_timeout", true); err != nil {
		return err
	}
	if c.MaxBodySize < 0 {
		return fmt.Errorf("ingress.http.max_body_size must be non-negative, got %d", c.MaxBodySize)
	}
	return nil
}

// Validate validates the ingress configuration.
func (c *IngressConfig) Validate() error {
	return c.HTTP.Validate()
}

// Validate validates the log configuration.
func (c *LogConfig) Validate() error {
	if err := validateOneOf(c.Level, "log.level", []string{"debug", "info", "warn", "error"}); err != nil {
		return err
	}
	if err := validateOneOf(c.Format, "log.format", []string{"text", "json"}); err != nil {
		return err
	}
	return nil
}

// Validate validates the WebUI configuration (only meaningful when Enabled is true).
func (c *WebUIConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if err := validatePort(c.Port, "webui.port"); err != nil {
		return err
	}
	return nil
}

// Validate validates the view URL authentication configuration.
func (c *ViewAuthConfig) Validate() error {
	if c.Enabled && strings.TrimSpace(c.Secret) == "" {
		return fmt.Errorf("view_url.auth.secret is required when auth is enabled")
	}
	return nil
}

// Validate validates the view URL configuration.
func (c *ViewURLConfig) Validate() error {
	return c.Auth.Validate()
}

// Validate validates the retention configuration.
func (c *RetentionConfig) Validate() error {
	if err := validateDuration(c.Default, "retention.default", true); err != nil {
		return err
	}
	for i, rule := range c.Rules {
		if err := validateRequired(rule.TaskType, fmt.Sprintf("retention.rules[%d].task_type", i)); err != nil {
			return err
		}
		if err := validateDuration(rule.Duration, fmt.Sprintf("retention.rules[%d].duration", i), false); err != nil {
			return err
		}
	}
	return nil
}

// Validate validates the callback configuration.
func (c *CallbackConfig) Validate() error {
	if err := validateDuration(c.Timeout, "callback.timeout", true); err != nil {
		return err
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("callback.max_retries must be non-negative, got %d", c.MaxRetries)
	}
	return nil
}

// validateSources validates the sources configuration for common fields.
func validateSources(sources []SourceConfig) error {
	names := make(map[string]bool)
	for i, src := range sources {
		if !src.Enabled {
			continue
		}
		if err := validateRequired(src.Name, fmt.Sprintf("sources[%d].name", i)); err != nil {
			return err
		}
		if err := validateRequired(src.Type, fmt.Sprintf("sources[%d].type", i)); err != nil {
			return err
		}
		if names[src.Name] {
			return fmt.Errorf("sources[%d].name %q is duplicated", i, src.Name)
		}
		names[src.Name] = true
	}
	return nil
}
