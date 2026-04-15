// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"context"
	"time"

	"github.com/junjiewwang/perf-analysis/perflib/internal/parallel"
)

// ============================================================================
// Type Aliases for internal parallel usage
// ============================================================================

// PoolConfig is an alias to parallel.PoolConfig.
type PoolConfig = parallel.PoolConfig

// PoolMetrics is an alias to parallel.PoolMetrics.
type PoolMetrics = parallel.PoolMetrics

// Task is an alias to parallel.Task.
type Task[T any, R any] = parallel.Task[T, R]

// TaskFunc is an alias to parallel.TaskFunc.
type TaskFunc[T any, R any] = parallel.TaskFunc[T, R]

// TaskResult is an alias to parallel.TaskResult.
type TaskResult[T any, R any] = parallel.TaskResult[T, R]

// WorkerPool is an alias to parallel.WorkerPool.
type WorkerPool[T any, R any] = parallel.WorkerPool[T, R]

// ChunkProcessor is an alias to parallel.ChunkProcessor.
type ChunkProcessor[T any, R any] = parallel.ChunkProcessor[T, R]

// AggregateResult is an alias to parallel.AggregateResult.
type AggregateResult[K comparable, V any] = parallel.AggregateResult[K, V]

// ProgressTracker is an alias to parallel.ProgressTracker.
type ProgressTracker = parallel.ProgressTracker

// ============================================================================
// Function Wrappers
// ============================================================================

// DefaultPoolConfig returns a default pool configuration.
func DefaultPoolConfig() PoolConfig {
	return parallel.DefaultPoolConfig()
}

// NewTask creates a new task from a function.
func NewTask[T any, R any](input T, fn func(ctx context.Context, input T) (R, error)) *TaskFunc[T, R] {
	return parallel.NewTask(input, fn)
}

// NewWorkerPool creates a new worker pool with the given configuration.
func NewWorkerPool[T any, R any](config PoolConfig) *WorkerPool[T, R] {
	return parallel.NewWorkerPool[T, R](config)
}

// NewChunkProcessor creates a new chunk processor.
func NewChunkProcessor[T any, R any](config PoolConfig) *ChunkProcessor[T, R] {
	return parallel.NewChunkProcessor[T, R](config)
}

// MapReduce applies a map function to each item in parallel and reduces the results.
func MapReduce[T any, M any, R any](
	ctx context.Context,
	items []T,
	config PoolConfig,
	mapper func(ctx context.Context, item T) M,
	reducer func(mapped []M) R,
) R {
	return parallel.MapReduce(ctx, items, config, mapper, reducer)
}

// ForEach executes a function for each item in parallel.
func ForEach[T any](
	ctx context.Context,
	items []T,
	config PoolConfig,
	fn func(ctx context.Context, item T) error,
) (processed int64, firstError error) {
	return parallel.ForEach(ctx, items, config, fn)
}

// ParallelAggregate aggregates data in parallel using per-worker local maps.
func ParallelAggregate[T any, K comparable, V any](
	ctx context.Context,
	items []T,
	config PoolConfig,
	extractor func(item T) (key K, value V),
	merger func(existing, new V) V,
) map[K]V {
	return parallel.ParallelAggregate(ctx, items, config, extractor, merger)
}

// NewProgressTracker creates a new progress tracker.
func NewProgressTracker(total int64, callback func(completed, total int64), interval time.Duration) *ProgressTracker {
	return parallel.NewProgressTracker(total, callback, interval)
}
