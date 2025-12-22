// Package pprof provides performance profiling collection capabilities.
//
// It supports two modes:
//   - File mode: Periodically collects profiles and writes them to files.
//     Suitable for CLI tools and batch processing.
//   - HTTP mode: Exposes pprof endpoints via HTTP for on-demand collection.
//     Suitable for long-running services.
//
// Basic usage for CLI (file mode):
//
//	cfg := pprof.DefaultConfig()
//	cfg.Enabled = true
//	cfg.Mode = pprof.ModeFile
//	cfg.OutputDir = "./pprof"
//
//	collector, err := pprof.NewCollector(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	if err := collector.Start(); err != nil {
//	    log.Fatal(err)
//	}
//	defer collector.Stop()
//
//	// ... run your application ...
//
// Basic usage for services (HTTP mode):
//
//	cfg := pprof.DefaultConfig()
//	cfg.Enabled = true
//	cfg.Mode = pprof.ModeHTTP
//	cfg.HTTPConfig.Addr = ":6060"
//
//	collector, err := pprof.NewCollector(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	if err := collector.Start(); err != nil {
//	    log.Fatal(err)
//	}
//	defer collector.Stop()
//
//	// Access profiles at http://localhost:6060/debug/pprof/
//
// Integration with existing HTTP server:
//
//	httpMode := pprof.NewHTTPMode(cfg.HTTPConfig)
//	// Mount at your preferred path
//	mux.Handle("/debug/pprof/", httpMode.Handler())
package pprof

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// Global collector for convenience functions
var globalCollector *Collector

// StartGlobal starts a global pprof collector with the given config.
// It also sets up signal handling for graceful shutdown.
func StartGlobal(cfg *Config) error {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	collector, err := NewCollector(cfg)
	if err != nil {
		return err
	}

	if err := collector.Start(); err != nil {
		return err
	}

	globalCollector = collector

	// Setup signal handling for graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		StopGlobal()
	}()

	return nil
}

// StopGlobal stops the global pprof collector.
func StopGlobal() error {
	if globalCollector == nil {
		return nil
	}

	err := globalCollector.Stop()
	globalCollector = nil
	return err
}

// GetGlobal returns the global collector.
func GetGlobal() *Collector {
	return globalCollector
}

// RunWithPprof runs a function with pprof collection.
// It starts the collector before running the function and stops it after.
func RunWithPprof(cfg *Config, fn func(ctx context.Context) error) error {
	if cfg == nil || !cfg.Enabled {
		return fn(context.Background())
	}

	collector, err := NewCollector(cfg)
	if err != nil {
		return err
	}

	if err := collector.Start(); err != nil {
		return err
	}
	defer collector.Stop()

	return fn(collector.Context())
}
