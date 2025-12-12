package telemetry

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"google.golang.org/grpc/credentials/insecure"
)

// createExporter creates an OTLP trace exporter based on configuration.
func createExporter(ctx context.Context, cfg *Config) (*otlptrace.Exporter, error) {
	protocol := strings.ToLower(cfg.Protocol)

	switch protocol {
	case "http/protobuf", "http":
		return createHTTPExporter(ctx, cfg)
	default:
		// Default to gRPC
		return createGRPCExporter(ctx, cfg)
	}
}

// createGRPCExporter creates a gRPC-based OTLP exporter.
func createGRPCExporter(ctx context.Context, cfg *Config) (*otlptrace.Exporter, error) {
	opts := []otlptracegrpc.Option{}

	// Set endpoint
	if cfg.Endpoint != "" {
		// Remove scheme prefix if present (gRPC client handles this differently)
		endpoint := cfg.Endpoint
		endpoint = strings.TrimPrefix(endpoint, "https://")
		endpoint = strings.TrimPrefix(endpoint, "http://")
		opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))
	}

	// Set headers (including Authorization token)
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(cfg.Headers))
	}

	// Set TLS configuration
	if cfg.Insecure || strings.HasPrefix(cfg.Endpoint, "http://") {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(insecure.NewCredentials()))
	}

	return otlptracegrpc.New(ctx, opts...)
}

// createHTTPExporter creates an HTTP-based OTLP exporter.
func createHTTPExporter(ctx context.Context, cfg *Config) (*otlptrace.Exporter, error) {
	opts := []otlptracehttp.Option{}

	// Set endpoint
	if cfg.Endpoint != "" {
		// For HTTP, we need to handle the URL properly
		endpoint := cfg.Endpoint
		if strings.HasPrefix(endpoint, "https://") {
			endpoint = strings.TrimPrefix(endpoint, "https://")
		} else if strings.HasPrefix(endpoint, "http://") {
			endpoint = strings.TrimPrefix(endpoint, "http://")
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		opts = append(opts, otlptracehttp.WithEndpoint(endpoint))
	}

	// Set headers (including Authorization token)
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(cfg.Headers))
	}

	// Set insecure if configured
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	return otlptracehttp.New(ctx, opts...)
}
