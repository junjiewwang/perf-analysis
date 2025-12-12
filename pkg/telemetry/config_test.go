package telemetry

import (
	"os"
	"testing"
)

func TestLoadFromEnv(t *testing.T) {
	// Save original env and restore after test
	originalEnv := map[string]string{
		"OTEL_ENABLED":                 os.Getenv("OTEL_ENABLED"),
		"OTEL_SERVICE_NAME":            os.Getenv("OTEL_SERVICE_NAME"),
		"OTEL_SERVICE_VERSION":         os.Getenv("OTEL_SERVICE_VERSION"),
		"OTEL_EXPORTER_OTLP_ENDPOINT":  os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		"OTEL_EXPORTER_OTLP_PROTOCOL":  os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"),
		"OTEL_EXPORTER_OTLP_HEADERS":   os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"),
		"OTEL_EXPORTER_OTLP_INSECURE":  os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"),
		"OTEL_TRACES_SAMPLER":          os.Getenv("OTEL_TRACES_SAMPLER"),
		"OTEL_TRACES_SAMPLER_ARG":      os.Getenv("OTEL_TRACES_SAMPLER_ARG"),
		"OTEL_RESOURCE_ATTRIBUTES":     os.Getenv("OTEL_RESOURCE_ATTRIBUTES"),
	}
	defer func() {
		for k, v := range originalEnv {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// Clear all env vars first
	for k := range originalEnv {
		os.Unsetenv(k)
	}

	t.Run("defaults", func(t *testing.T) {
		cfg := LoadFromEnv()

		if cfg.Enabled {
			t.Error("Expected Enabled to be false by default")
		}
		if cfg.ServiceName != "perf-analyzer" {
			t.Errorf("Expected ServiceName to be 'perf-analyzer', got '%s'", cfg.ServiceName)
		}
		if cfg.ServiceVersion != "unknown" {
			t.Errorf("Expected ServiceVersion to be 'unknown', got '%s'", cfg.ServiceVersion)
		}
		if cfg.Protocol != "grpc" {
			t.Errorf("Expected Protocol to be 'grpc', got '%s'", cfg.Protocol)
		}
	})

	t.Run("enabled", func(t *testing.T) {
		os.Setenv("OTEL_ENABLED", "true")
		defer os.Unsetenv("OTEL_ENABLED")

		cfg := LoadFromEnv()
		if !cfg.Enabled {
			t.Error("Expected Enabled to be true")
		}
	})

	t.Run("enabled_case_insensitive", func(t *testing.T) {
		os.Setenv("OTEL_ENABLED", "TRUE")
		defer os.Unsetenv("OTEL_ENABLED")

		cfg := LoadFromEnv()
		if !cfg.Enabled {
			t.Error("Expected Enabled to be true for 'TRUE'")
		}
	})

	t.Run("custom_values", func(t *testing.T) {
		os.Setenv("OTEL_SERVICE_NAME", "my-service")
		os.Setenv("OTEL_SERVICE_VERSION", "1.0.0")
		os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://collector.example.com:4317")
		os.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
		os.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
		defer func() {
			os.Unsetenv("OTEL_SERVICE_NAME")
			os.Unsetenv("OTEL_SERVICE_VERSION")
			os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
			os.Unsetenv("OTEL_EXPORTER_OTLP_PROTOCOL")
			os.Unsetenv("OTEL_EXPORTER_OTLP_INSECURE")
		}()

		cfg := LoadFromEnv()

		if cfg.ServiceName != "my-service" {
			t.Errorf("Expected ServiceName 'my-service', got '%s'", cfg.ServiceName)
		}
		if cfg.ServiceVersion != "1.0.0" {
			t.Errorf("Expected ServiceVersion '1.0.0', got '%s'", cfg.ServiceVersion)
		}
		if cfg.Endpoint != "https://collector.example.com:4317" {
			t.Errorf("Expected Endpoint 'https://collector.example.com:4317', got '%s'", cfg.Endpoint)
		}
		if cfg.Protocol != "http/protobuf" {
			t.Errorf("Expected Protocol 'http/protobuf', got '%s'", cfg.Protocol)
		}
		if !cfg.Insecure {
			t.Error("Expected Insecure to be true")
		}
	})

	t.Run("headers_parsing", func(t *testing.T) {
		os.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=Bearer token123,X-Custom=value")
		defer os.Unsetenv("OTEL_EXPORTER_OTLP_HEADERS")

		cfg := LoadFromEnv()

		if len(cfg.Headers) != 2 {
			t.Errorf("Expected 2 headers, got %d", len(cfg.Headers))
		}
		if cfg.Headers["Authorization"] != "Bearer token123" {
			t.Errorf("Expected Authorization header 'Bearer token123', got '%s'", cfg.Headers["Authorization"])
		}
		if cfg.Headers["X-Custom"] != "value" {
			t.Errorf("Expected X-Custom header 'value', got '%s'", cfg.Headers["X-Custom"])
		}
	})

	t.Run("resource_attributes", func(t *testing.T) {
		os.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment=production,service.namespace=perf")
		defer os.Unsetenv("OTEL_RESOURCE_ATTRIBUTES")

		cfg := LoadFromEnv()

		if len(cfg.ResourceAttrs) != 2 {
			t.Errorf("Expected 2 resource attributes, got %d", len(cfg.ResourceAttrs))
		}
		if cfg.ResourceAttrs["deployment.environment"] != "production" {
			t.Errorf("Expected deployment.environment 'production', got '%s'", cfg.ResourceAttrs["deployment.environment"])
		}
	})
}

func TestParseKeyValuePairs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:     "empty",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:     "single_pair",
			input:    "key=value",
			expected: map[string]string{"key": "value"},
		},
		{
			name:     "multiple_pairs",
			input:    "key1=value1,key2=value2",
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name:     "with_spaces",
			input:    " key1 = value1 , key2 = value2 ",
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name:     "value_with_equals",
			input:    "Authorization=Bearer token=abc",
			expected: map[string]string{"Authorization": "Bearer token=abc"},
		},
		{
			name:     "empty_value",
			input:    "key=",
			expected: map[string]string{"key": ""},
		},
		{
			name:     "invalid_no_equals",
			input:    "invalid",
			expected: map[string]string{},
		},
		{
			name:     "mixed_valid_invalid",
			input:    "valid=value,invalid,another=test",
			expected: map[string]string{"valid": "value", "another": "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseKeyValuePairs(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d pairs, got %d", len(tt.expected), len(result))
			}

			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("Expected %s='%s', got '%s'", k, v, result[k])
				}
			}
		})
	}
}
