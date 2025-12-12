package telemetry

import (
	"context"
	"net"
	"os"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// buildResource creates an OpenTelemetry Resource with service information.
// The host.name attribute is set to the IP address resolved from the hostname.
func buildResource(ctx context.Context, cfg *Config) (*resource.Resource, error) {
	// Get host IP (hostname resolved to IP)
	hostIP := getHostIP()

	// Build base attributes
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
	}

	// Add host.name as IP address
	if hostIP != "" {
		attrs = append(attrs, semconv.HostName(hostIP))
	}

	// Add user-defined resource attributes
	for k, v := range cfg.ResourceAttrs {
		attrs = append(attrs, attribute.String(k, v))
	}

	// Merge with default resource
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
}

// getHostIP returns the IP address resolved from the hostname.
// Returns empty string if resolution fails.
func getHostIP() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}

	addrs, err := net.LookupIP(hostname)
	if err != nil {
		// Fallback: try to get IP from network interfaces
		return getFirstNonLoopbackIP()
	}

	// Prefer IPv4 address
	for _, addr := range addrs {
		if ipv4 := addr.To4(); ipv4 != nil && !ipv4.IsLoopback() {
			return ipv4.String()
		}
	}

	// Fallback to first non-loopback address
	for _, addr := range addrs {
		if !addr.IsLoopback() {
			return addr.String()
		}
	}

	// Last resort: try network interfaces
	return getFirstNonLoopbackIP()
}

// getFirstNonLoopbackIP returns the first non-loopback IP address from network interfaces.
func getFirstNonLoopbackIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range ifaces {
		// Skip down or loopback interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			// Prefer IPv4
			if ipv4 := ip.To4(); ipv4 != nil {
				return ipv4.String()
			}
		}
	}

	return ""
}
