package telemetry

import (
	"context"
	"os"
	"sync"
	"testing"
)

func TestInit_Disabled(t *testing.T) {
	// Reset global state for test
	resetGlobalConfig()

	// Ensure OTEL_ENABLED is not set
	os.Unsetenv("OTEL_ENABLED")

	ctx := context.Background()
	shutdown, err := Init(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if shutdown == nil {
		t.Error("Expected shutdown function to be non-nil")
	}

	// Shutdown should not error
	if err := shutdown(ctx); err != nil {
		t.Errorf("Expected no error on shutdown, got %v", err)
	}
}

func TestEnabled(t *testing.T) {
	// Reset global state
	resetGlobalConfig()

	// Test disabled
	os.Unsetenv("OTEL_ENABLED")
	if Enabled() {
		t.Error("Expected Enabled() to return false")
	}
}

func TestGetConfig(t *testing.T) {
	// Reset global state
	resetGlobalConfig()

	os.Setenv("OTEL_SERVICE_NAME", "test-service")
	defer os.Unsetenv("OTEL_SERVICE_NAME")

	cfg := GetConfig()

	if cfg == nil {
		t.Fatal("Expected config to be non-nil")
	}

	if cfg.ServiceName != "test-service" {
		t.Errorf("Expected ServiceName 'test-service', got '%s'", cfg.ServiceName)
	}
}

// resetGlobalConfig resets the global config for testing
func resetGlobalConfig() {
	globalConfig = nil
	configOnce = sync.Once{}
}
