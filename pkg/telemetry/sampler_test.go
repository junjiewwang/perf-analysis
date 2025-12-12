package telemetry

import (
	"testing"

	"go.opentelemetry.io/otel/sdk/trace"
)

func TestCreateSampler(t *testing.T) {
	tests := []struct {
		name       string
		sampler    string
		samplerArg string
	}{
		{"default_always_on", "", ""},
		{"always_on", "always_on", ""},
		{"always_off", "always_off", ""},
		{"traceidratio", "traceidratio", "0.5"},
		{"parentbased_always_on", "parentbased_always_on", ""},
		{"parentbased_always_off", "parentbased_always_off", ""},
		{"parentbased_traceidratio", "parentbased_traceidratio", "0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Sampler:    tt.sampler,
				SamplerArg: tt.samplerArg,
			}

			sampler := createSampler(cfg)
			if sampler == nil {
				t.Error("Expected sampler to be non-nil")
			}

			// Verify it implements the Sampler interface
			var _ trace.Sampler = sampler
		})
	}
}

func TestParseRatio(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
	}{
		{"empty", "", 1.0},
		{"valid_half", "0.5", 0.5},
		{"valid_zero", "0", 0},
		{"valid_one", "1", 1.0},
		{"valid_small", "0.001", 0.001},
		{"invalid_string", "invalid", 1.0},
		{"negative", "-0.5", 0},
		{"greater_than_one", "1.5", 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRatio(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}
