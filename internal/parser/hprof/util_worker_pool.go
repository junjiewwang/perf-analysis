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

// PoolConfig is an alias to perflib hprof PoolConfig.
type PoolConfig = libhprof.PoolConfig

// PoolMetrics is an alias to perflib hprof PoolMetrics.
type PoolMetrics = libhprof.PoolMetrics

// Task is an alias to perflib hprof Task.
type Task[T any, R any] = libhprof.Task[T, R]

// TaskFunc is an alias to perflib hprof TaskFunc.
type TaskFunc[T any, R any] = libhprof.TaskFunc[T, R]

// TaskResult is an alias to perflib hprof TaskResult.
type TaskResult[T any, R any] = libhprof.TaskResult[T, R]

// WorkerPool is an alias to perflib hprof WorkerPool.
type WorkerPool[T any, R any] = libhprof.WorkerPool[T, R]

// ChunkProcessor is an alias to perflib hprof ChunkProcessor.
type ChunkProcessor[T any, R any] = libhprof.ChunkProcessor[T, R]

// AggregateResult is an alias to perflib hprof AggregateResult.
type AggregateResult[K comparable, V any] = libhprof.AggregateResult[K, V]

// ProgressTracker is an alias to perflib hprof ProgressTracker.
type ProgressTracker = libhprof.ProgressTracker

// ============================================================================
// Function Forwarding
// ============================================================================

// DefaultPoolConfig returns a default pool configuration.
func DefaultPoolConfig() PoolConfig {
	return libhprof.DefaultPoolConfig()
}

// NewTask creates a new task from a function.
func NewTask[T any, R any](input T, fn func(ctx context.Context, input T) (R, error)) *TaskFunc[T, R] {
	return libhprof.NewTask(input, fn)
}

// NewWorkerPool creates a new worker pool with the given configuration.
func NewWorkerPool[T any, R any](config PoolConfig) *WorkerPool[T, R] {
	return libhprof.NewWorkerPool[T, R](config)
}

// NewChunkProcessor creates a new chunk processor.
func NewChunkProcessor[T any, R any](config PoolConfig) *ChunkProcessor[T, R] {
	return libhprof.NewChunkProcessor[T, R](config)
}

// MapReduce applies a map function to each item in parallel and reduces the results.
func MapReduce[T any, M any, R any](
	ctx context.Context,
	items []T,
	config PoolConfig,
	mapper func(ctx context.Context, item T) M,
	reducer func(mapped []M) R,
) R {
	return libhprof.MapReduce(ctx, items, config, mapper, reducer)
}

// ForEach executes a function for each item in parallel.
func ForEach[T any](
	ctx context.Context,
	items []T,
	config PoolConfig,
	fn func(ctx context.Context, item T) error,
) (processed int64, firstError error) {
	return libhprof.ForEach(ctx, items, config, fn)
}

// ParallelAggregate aggregates data in parallel using per-worker local maps.
func ParallelAggregate[T any, K comparable, V any](
	ctx context.Context,
	items []T,
	config PoolConfig,
	extractor func(item T) (key K, value V),
	merger func(existing, new V) V,
) map[K]V {
	return libhprof.ParallelAggregate(ctx, items, config, extractor, merger)
}

// NewProgressTracker creates a new progress tracker.
func NewProgressTracker(total int64, callback func(completed, total int64), interval time.Duration) *ProgressTracker {
	return libhprof.NewProgressTracker(total, callback, interval)
}
