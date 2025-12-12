// Package telemetry provides OpenTelemetry integration for distributed tracing.
package telemetry

import (
	"os"
	"strings"
)

// Config holds OpenTelemetry configuration loaded from environment variables.
type Config struct {
	// Enabled indicates whether OpenTelemetry tracing is enabled.
	// Loaded from OTEL_ENABLED environment variable.
	Enabled bool

	// ServiceName is the name of the service.
	// Loaded from OTEL_SERVICE_NAME, defaults to "perf-analyzer".
	ServiceName string

	// ServiceVersion is the version of the service.
	// Loaded from OTEL_SERVICE_VERSION, defaults to "unknown".
	ServiceVersion string

	// Endpoint is the OTLP collector endpoint.
	// Loaded from OTEL_EXPORTER_OTLP_ENDPOINT.
	Endpoint string

	// Protocol is the OTLP protocol (grpc or http/protobuf).
	// Loaded from OTEL_EXPORTER_OTLP_PROTOCOL, defaults to "grpc".
	Protocol string

	// Headers contains custom headers for OTLP exporter (e.g., Authorization).
	// Loaded from OTEL_EXPORTER_OTLP_HEADERS.
	// Format: "key1=value1,key2=value2"
	Headers map[string]string

	// Insecure indicates whether to use insecure connection.
	// Loaded from OTEL_EXPORTER_OTLP_INSECURE.
	Insecure bool

	// Sampler is the sampler type.
	// Loaded from OTEL_TRACES_SAMPLER.
	// Supported values: always_on, always_off, traceidratio,
	// parentbased_always_on, parentbased_always_off, parentbased_traceidratio.
	// Defaults to always_on (full sampling).
	Sampler string

	// SamplerArg is the sampler argument (e.g., ratio for traceidratio).
	// Loaded from OTEL_TRACES_SAMPLER_ARG.
	SamplerArg string

	// ResourceAttrs contains additional resource attributes.
	// Loaded from OTEL_RESOURCE_ATTRIBUTES.
	// Format: "key1=value1,key2=value2"
	ResourceAttrs map[string]string
}

// LoadFromEnv loads configuration from environment variables.
func LoadFromEnv() *Config {
	return &Config{
		Enabled:        strings.ToLower(os.Getenv("OTEL_ENABLED")) == "true",
		ServiceName:    getEnvOrDefault("OTEL_SERVICE_NAME", "perf-analyzer"),
		ServiceVersion: getEnvOrDefault("OTEL_SERVICE_VERSION", "unknown"),
		Endpoint:       os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Protocol:       getEnvOrDefault("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc"),
		Headers:        parseKeyValuePairs(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")),
		Insecure:       strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE")) == "true",
		Sampler:        os.Getenv("OTEL_TRACES_SAMPLER"),
		SamplerArg:     os.Getenv("OTEL_TRACES_SAMPLER_ARG"),
		ResourceAttrs:  parseKeyValuePairs(os.Getenv("OTEL_RESOURCE_ATTRIBUTES")),
	}
}

// getEnvOrDefault returns the environment variable value or a default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseKeyValuePairs parses a comma-separated list of key=value pairs.
// Example: "key1=value1,key2=value2" -> map[string]string{"key1": "value1", "key2": "value2"}
func parseKeyValuePairs(s string) map[string]string {
	result := make(map[string]string)
	if s == "" {
		return result
	}

	pairs := strings.Split(s, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		// Split on first '=' only to allow '=' in values
		idx := strings.Index(pair, "=")
		if idx <= 0 {
			continue
		}

		key := strings.TrimSpace(pair[:idx])
		value := strings.TrimSpace(pair[idx+1:])
		if key != "" {
			result[key] = value
		}
	}

	return result
}
