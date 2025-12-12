package telemetry

import (
	"net"
	"testing"
)

func TestGetHostIP(t *testing.T) {
	ip := getHostIP()

	// Should return a non-empty string (unless running in a very restricted environment)
	if ip == "" {
		t.Skip("Could not get host IP, skipping test")
	}

	// Validate it's a valid IP address
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		t.Errorf("Expected valid IP address, got '%s'", ip)
	}

	// Should not be loopback
	if parsedIP.IsLoopback() {
		t.Errorf("Expected non-loopback IP, got '%s'", ip)
	}

	t.Logf("Host IP: %s", ip)
}

func TestGetFirstNonLoopbackIP(t *testing.T) {
	ip := getFirstNonLoopbackIP()

	if ip == "" {
		t.Skip("No non-loopback IP found, skipping test")
	}

	// Validate it's a valid IP address
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		t.Errorf("Expected valid IP address, got '%s'", ip)
	}

	// Should not be loopback
	if parsedIP.IsLoopback() {
		t.Errorf("Expected non-loopback IP, got '%s'", ip)
	}

	t.Logf("First non-loopback IP: %s", ip)
}
