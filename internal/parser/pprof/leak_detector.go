// Package pprof provides parsing functionality for Go pprof profile data.
// This is a thin wrapper around github.com/junjiewwang/perf-analysis/perflib/parser/pprof.
package pprof

import (
	libpprof "github.com/junjiewwang/perf-analysis/perflib/parser/pprof"
)

// Type aliases - delegate all types to perflib/parser/pprof.
type (
	// LeakType represents the type of leak being detected.
	LeakType = libpprof.LeakType

	// GrowthItem represents an item that has grown between two profile snapshots.
	GrowthItem = libpprof.GrowthItem

	// LeakReport represents a leak analysis report.
	LeakReport = libpprof.LeakReport

	// LeakDetector detects memory and goroutine leaks by comparing profile snapshots.
	LeakDetector = libpprof.LeakDetector
)

// LeakType constants - re-export from perflib.
const (
	// LeakTypeHeap represents heap memory leak.
	LeakTypeHeap = libpprof.LeakTypeHeap
	// LeakTypeGoroutine represents goroutine leak.
	LeakTypeGoroutine = libpprof.LeakTypeGoroutine
)

// NewLeakDetector creates a new LeakDetector.
var NewLeakDetector = libpprof.NewLeakDetector
