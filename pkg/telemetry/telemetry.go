// Package telemetry provides OpenTelemetry integration for distributed tracing.
//
// This package initializes OpenTelemetry with configuration loaded from standard
// environment variables. It sets up a global TracerProvider that can be used
// throughout the application via otel.Tracer().
//
// Environment Variables:
//
//	OTEL_ENABLED                    - Enable/disable tracing (default: false)
//	OTEL_SERVICE_NAME               - Service name (default: perf-analyzer)
//	OTEL_SERVICE_VERSION            - Service version (default: unknown)
//	OTEL_EXPORTER_OTLP_ENDPOINT     - OTLP collector endpoint
//	OTEL_EXPORTER_OTLP_PROTOCOL     - Protocol: grpc or http/protobuf (default: grpc)
//	OTEL_EXPORTER_OTLP_HEADERS      - Headers for authentication (e.g., Authorization=Bearer xxx)
//	OTEL_EXPORTER_OTLP_INSECURE     - Use insecure connection (default: false)
//	OTEL_TRACES_SAMPLER             - Sampler type (default: always_on)
//	OTEL_TRACES_SAMPLER_ARG         - Sampler argument (e.g., ratio)
//	OTEL_RESOURCE_ATTRIBUTES        - Additional resource attributes
//
// Usage:
//
//	func main() {
//	    ctx := context.Background()
//
//	    // Initialize OpenTelemetry
//	    shutdown, err := telemetry.Init(ctx)
//	    if err != nil {
//	        log.Printf("Failed to initialize telemetry: %v", err)
//	    }
//	    defer shutdown(ctx)
//
//	    // Use global tracer in your code
//	    ctx, span := otel.Tracer("my-service").Start(ctx, "operation")
//	    defer span.End()
//	}
package telemetry

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
)

var (
	// globalConfig holds the loaded configuration
	globalConfig *Config
	configOnce   sync.Once
)

// ShutdownFunc is a function that shuts down the TracerProvider.
type ShutdownFunc func(ctx context.Context) error

// noopShutdown is a no-op shutdown function.
func noopShutdown(_ context.Context) error {
	return nil
}

// Init initializes OpenTelemetry and sets up the global TracerProvider.
// If OTEL_ENABLED is not "true", it returns a no-op shutdown function
// and the global TracerProvider remains as the default no-op provider.
//
// The function is safe to call multiple times, but only the first call
// will initialize the TracerProvider.
func Init(ctx context.Context) (ShutdownFunc, error) {
	cfg := loadConfig()

	if !cfg.Enabled {
		return noopShutdown, nil
	}

	// Build resource with host.name = IP
	res, err := buildResource(ctx, cfg)
	if err != nil {
		return noopShutdown, err
	}

	// Create OTLP exporter
	exporter, err := createExporter(ctx, cfg)
	if err != nil {
		return noopShutdown, err
	}

	// Create sampler (defaults to AlwaysSample)
	sampler := createSampler(cfg)

	// Create TracerProvider
	tp := trace.NewTracerProvider(
		trace.WithResource(res),
		trace.WithBatcher(exporter),
		trace.WithSampler(sampler),
	)

	// Set global TracerProvider
	otel.SetTracerProvider(tp)

	// Set global propagator for context propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Return shutdown function
	return func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	}, nil
}

// Enabled returns whether OpenTelemetry tracing is enabled.
func Enabled() bool {
	return loadConfig().Enabled
}

// GetConfig returns the current telemetry configuration.
func GetConfig() *Config {
	return loadConfig()
}

// loadConfig loads configuration once and caches it.
func loadConfig() *Config {
	configOnce.Do(func() {
		globalConfig = LoadFromEnv()
	})
	return globalConfig
}
