// Package model defines output data abstractions for different analysis types.
package model

import "fmt"

// Profiler represents the profiling tool used to collect data.
type Profiler string

const (
	// ProfilerPerf is the Linux perf profiler.
	ProfilerPerf Profiler = "perf"

	// ProfilerAsyncProfiler is the async-profiler for Java.
	ProfilerAsyncProfiler Profiler = "async-profiler"

	// ProfilerPProf is the Go pprof profiler.
	ProfilerPProf Profiler = "pprof"

	// ProfilerHeapDump is the Java heap dump tool.
	ProfilerHeapDump Profiler = "heapdump"

	// ProfilerJeprof is the jemalloc profiler.
	ProfilerJeprof Profiler = "jeprof"
)

// knownProfilers is the set of valid profiler values.
var knownProfilers = map[Profiler]bool{
	ProfilerPerf:          true,
	ProfilerAsyncProfiler: true,
	ProfilerPProf:         true,
	ProfilerHeapDump:      true,
	ProfilerJeprof:        true,
}

// IsValid returns true if the profiler is a known value.
func (p Profiler) IsValid() bool {
	return knownProfilers[p]
}

// String returns the string representation of the profiler.
func (p Profiler) String() string {
	return string(p)
}

// EventType represents the type of profiling event.
type EventType string

const (
	// EventCPU is CPU sampling event.
	EventCPU EventType = "cpu"

	// EventAlloc is memory allocation event.
	EventAlloc EventType = "alloc"

	// EventHeap is heap snapshot event.
	EventHeap EventType = "heap"

	// EventWall is wall-clock time event.
	EventWall EventType = "wall"

	// EventLock is lock contention event.
	EventLock EventType = "lock"

	// EventGoroutine is Go goroutine profiling event.
	EventGoroutine EventType = "goroutine"

	// EventBlock is Go block profiling event.
	EventBlock EventType = "block"

	// EventMutex is Go mutex profiling event.
	EventMutex EventType = "mutex"

	// EventIO is IO tracing event.
	EventIO EventType = "io"
)

// knownEventTypes is the set of valid event type values.
var knownEventTypes = map[EventType]bool{
	EventCPU:       true,
	EventAlloc:     true,
	EventHeap:      true,
	EventWall:      true,
	EventLock:      true,
	EventGoroutine: true,
	EventBlock:     true,
	EventMutex:     true,
	EventIO:        true,
}

// IsValid returns true if the event type is a known value.
func (e EventType) IsValid() bool {
	return knownEventTypes[e]
}

// String returns the string representation of the event type.
func (e EventType) String() string {
	return string(e)
}

// ResourceType represents the resource category that an analysis mode targets.
type ResourceType string

const (
	// ResourceCPU is CPU-related profiling.
	ResourceCPU ResourceType = "CPU"

	// ResourceMemory is memory-related profiling.
	ResourceMemory ResourceType = "Memory"

	// ResourceIO is disk/IO-related profiling.
	ResourceIO ResourceType = "Disk"

	// ResourceApp is application-level profiling (e.g. Java wall/lock).
	ResourceApp ResourceType = "App"

	// ResourceGoroutine is Go goroutine profiling.
	ResourceGoroutine ResourceType = "Goroutine"

	// ResourceConcurrency is concurrency-related profiling (block, mutex).
	ResourceConcurrency ResourceType = "Concurrency"
)

// AnalysisMode returns the composite mode string "{profiler}-{event}".
func AnalysisModeString(profiler Profiler, event EventType) string {
	return fmt.Sprintf("%s-%s", profiler, event)
}
