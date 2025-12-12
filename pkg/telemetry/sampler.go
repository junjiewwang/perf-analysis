package telemetry

import (
	"strconv"

	"go.opentelemetry.io/otel/sdk/trace"
)

// createSampler creates a trace sampler based on configuration.
// Defaults to AlwaysSample (full sampling) if no sampler is specified.
func createSampler(cfg *Config) trace.Sampler {
	switch cfg.Sampler {
	case "always_on":
		return trace.AlwaysSample()

	case "always_off":
		return trace.NeverSample()

	case "traceidratio":
		ratio := parseRatio(cfg.SamplerArg)
		return trace.TraceIDRatioBased(ratio)

	case "parentbased_always_on":
		return trace.ParentBased(trace.AlwaysSample())

	case "parentbased_always_off":
		return trace.ParentBased(trace.NeverSample())

	case "parentbased_traceidratio":
		ratio := parseRatio(cfg.SamplerArg)
		return trace.ParentBased(trace.TraceIDRatioBased(ratio))

	default:
		// Default: full sampling
		return trace.AlwaysSample()
	}
}

// parseRatio parses a sampling ratio string to float64.
// Returns 1.0 (full sampling) if parsing fails or value is out of range.
func parseRatio(s string) float64 {
	if s == "" {
		return 1.0
	}

	ratio, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 1.0
	}

	// Clamp to valid range [0, 1]
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1.0
	}

	return ratio
}
