// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"context"
	"time"

	libhprof "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// AsyncSerializationResult holds the result of an async serialization operation.
type AsyncSerializationResult = libhprof.AsyncSerializationResult

// AsyncSerializer provides asynchronous serialization capabilities.
type AsyncSerializer = libhprof.AsyncSerializer

// ============================================================================
// Function Forwarding
// ============================================================================

// NewAsyncSerializer creates a new async serializer.
func NewAsyncSerializer(maxConcurrentJobs int) *AsyncSerializer {
	return libhprof.NewAsyncSerializer(maxConcurrentJobs)
}

// SerializeToFileAsync is a convenience function that serializes asynchronously.
func SerializeToFileAsync(ctx context.Context, g *ReferenceGraph, filename string, opts SerializeOptions) (<-chan *AsyncSerializationResult, error) {
	return libhprof.SerializeToFileAsync(ctx, g, filename, opts)
}

// SerializeToFileWithCallback serializes asynchronously and calls the callback when done.
func SerializeToFileWithCallback(ctx context.Context, g *ReferenceGraph, filename string, opts SerializeOptions, callback func(*AsyncSerializationResult)) {
	libhprof.SerializeToFileWithCallback(ctx, g, filename, opts, callback)
}

// GetBackgroundSerializer returns the global background serializer instance.
func GetBackgroundSerializer() *AsyncSerializer {
	return libhprof.GetBackgroundSerializer()
}

// EnsureSerializationComplete waits for any pending background serialization to complete.
func EnsureSerializationComplete(filename string, timeout time.Duration) (*SerializationStats, error) {
	return libhprof.EnsureSerializationComplete(filename, timeout)
}

// Note: All methods on AsyncSerializer (SerializeAsync, GetResult, Wait,
// WaitWithTimeout, Cancel, CancelAll, WaitAll, Cleanup, PendingJobs)
// and ReferenceGraph (SerializeInBackground, SerializeToFileOrBackground)
// are automatically available through type aliases.
