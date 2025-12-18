// Package parallel provides generic parallel processing utilities.
package parallel

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Worker Pool Configuration
// ============================================================================

// PoolConfig configures the worker pool behavior.
type PoolConfig struct {
	// MaxWorkers is the maximum number of concurrent workers.
	// Default: min(runtime.NumCPU(), 8)
	MaxWorkers int

	// TaskBufferSize is the buffer size for the task channel.
	// Default: MaxWorkers * 2
	TaskBufferSize int

	// Timeout is the maximum time for the entire operation.
	// Default: 0 (no timeout)
	Timeout time.Duration

	// CollectMetrics enables collection of execution metrics.
	CollectMetrics bool
}

// DefaultPoolConfig returns a default pool configuration.
func DefaultPoolConfig() PoolConfig {
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8 // Cap at 8 to avoid excessive overhead
	}
	if workers < 2 {
		workers = 2
	}
	return PoolConfig{
		MaxWorkers:     workers,
		TaskBufferSize: workers * 2,
		Timeout:        0,
		CollectMetrics: false,
	}
}

// WithWorkers returns a new config with the specified number of workers.
func (c PoolConfig) WithWorkers(n int) PoolConfig {
	c.MaxWorkers = n
	return c
}

// WithTimeout returns a new config with the specified timeout.
func (c PoolConfig) WithTimeout(d time.Duration) PoolConfig {
	c.Timeout = d
	return c
}

// WithMetrics returns a new config with metrics collection enabled.
func (c PoolConfig) WithMetrics() PoolConfig {
	c.CollectMetrics = true
	return c
}

// ============================================================================
// Execution Metrics
// ============================================================================

// PoolMetrics holds execution statistics.
type PoolMetrics struct {
	TotalTasks     int64
	CompletedTasks int64
	FailedTasks    int64
	TotalDuration  time.Duration
	AvgTaskTime    time.Duration
	MaxTaskTime    time.Duration
	MinTaskTime    time.Duration
}

// ============================================================================
// Generic Task Interface
// ============================================================================

// Task represents a unit of work that can be executed by the worker pool.
type Task[T any, R any] interface {
	// Execute performs the task and returns the result.
	Execute(ctx context.Context) (R, error)
	// Input returns the input data for this task.
	Input() T
}

// TaskFunc is a function type that implements Task interface.
type TaskFunc[T any, R any] struct {
	input   T
	execute func(ctx context.Context, input T) (R, error)
}

// NewTask creates a new task from a function.
func NewTask[T any, R any](input T, fn func(ctx context.Context, input T) (R, error)) *TaskFunc[T, R] {
	return &TaskFunc[T, R]{
		input:   input,
		execute: fn,
	}
}

// Execute implements Task interface.
func (t *TaskFunc[T, R]) Execute(ctx context.Context) (R, error) {
	return t.execute(ctx, t.input)
}

// Input implements Task interface.
func (t *TaskFunc[T, R]) Input() T {
	return t.input
}

// ============================================================================
// Task Result
// ============================================================================

// TaskResult holds the result of a task execution.
type TaskResult[T any, R any] struct {
	Input    T
	Result   R
	Error    error
	Duration time.Duration
}

// ============================================================================
// Worker Pool
// ============================================================================

// WorkerPool manages a pool of workers for parallel task execution.
type WorkerPool[T any, R any] struct {
	config  PoolConfig
	metrics *PoolMetrics
	mu      sync.Mutex
}

// NewWorkerPool creates a new worker pool with the given configuration.
func NewWorkerPool[T any, R any](config PoolConfig) *WorkerPool[T, R] {
	if config.MaxWorkers <= 0 {
		config.MaxWorkers = DefaultPoolConfig().MaxWorkers
	}
	if config.TaskBufferSize <= 0 {
		config.TaskBufferSize = config.MaxWorkers * 2
	}
	return &WorkerPool[T, R]{
		config: config,
		metrics: &PoolMetrics{
			MinTaskTime: time.Hour, // Initialize to large value
		},
	}
}

// Execute runs all tasks in parallel and returns results.
// Results are returned in the same order as input tasks.
func (p *WorkerPool[T, R]) Execute(ctx context.Context, tasks []Task[T, R]) []TaskResult[T, R] {
	if len(tasks) == 0 {
		return nil
	}

	startTime := time.Now()

	// Apply timeout if configured
	if p.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.config.Timeout)
		defer cancel()
	}

	// Create result slice with same length as tasks
	results := make([]TaskResult[T, R], len(tasks))

	// Create task channel
	taskCh := make(chan int, p.config.TaskBufferSize)

	// Start workers
	var wg sync.WaitGroup
	numWorkers := min(p.config.MaxWorkers, len(tasks))

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case idx, ok := <-taskCh:
					if !ok {
						return
					}
					task := tasks[idx]
					taskStart := time.Now()
					result, err := task.Execute(ctx)
					duration := time.Since(taskStart)

					results[idx] = TaskResult[T, R]{
						Input:    task.Input(),
						Result:   result,
						Error:    err,
						Duration: duration,
					}

					// Update metrics if enabled
					if p.config.CollectMetrics {
						p.updateMetrics(duration, err)
					}
				}
			}
		}()
	}

	// Submit tasks
	go func() {
		for i := range tasks {
			select {
			case <-ctx.Done():
				break
			case taskCh <- i:
			}
		}
		close(taskCh)
	}()

	wg.Wait()

	// Update total duration
	if p.config.CollectMetrics {
		p.mu.Lock()
		p.metrics.TotalDuration = time.Since(startTime)
		if p.metrics.CompletedTasks > 0 {
			p.metrics.AvgTaskTime = p.metrics.TotalDuration / time.Duration(p.metrics.CompletedTasks)
		}
		p.mu.Unlock()
	}

	return results
}

// ExecuteFunc is a convenience method that creates tasks from a function.
func (p *WorkerPool[T, R]) ExecuteFunc(ctx context.Context, inputs []T, fn func(ctx context.Context, input T) (R, error)) []TaskResult[T, R] {
	tasks := make([]Task[T, R], len(inputs))
	for i, input := range inputs {
		tasks[i] = NewTask(input, fn)
	}
	return p.Execute(ctx, tasks)
}

// updateMetrics updates the pool metrics (thread-safe).
func (p *WorkerPool[T, R]) updateMetrics(duration time.Duration, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.metrics.TotalTasks++
	if err != nil {
		p.metrics.FailedTasks++
	} else {
		p.metrics.CompletedTasks++
	}

	if duration > p.metrics.MaxTaskTime {
		p.metrics.MaxTaskTime = duration
	}
	if duration < p.metrics.MinTaskTime {
		p.metrics.MinTaskTime = duration
	}
}

// Metrics returns the current execution metrics.
func (p *WorkerPool[T, R]) Metrics() PoolMetrics {
	p.mu.Lock()
	defer p.mu.Unlock()
	return *p.metrics
}

// ============================================================================
// Chunk Processor - For processing large datasets in parallel chunks
// ============================================================================

// ChunkProcessor processes large datasets by splitting them into chunks
// and processing each chunk in parallel.
type ChunkProcessor[T any, R any] struct {
	config PoolConfig
}

// NewChunkProcessor creates a new chunk processor.
func NewChunkProcessor[T any, R any](config PoolConfig) *ChunkProcessor[T, R] {
	return &ChunkProcessor[T, R]{config: config}
}

// ProcessChunks splits the input into chunks and processes each chunk in parallel.
// The reducer function combines results from all chunks into a single result.
func (p *ChunkProcessor[T, R]) ProcessChunks(
	ctx context.Context,
	items []T,
	processor func(ctx context.Context, chunk []T, workerID int) R,
	reducer func(results []R) R,
) R {
	if len(items) == 0 {
		var zero R
		return zero
	}

	numWorkers := p.config.MaxWorkers
	if numWorkers <= 0 {
		numWorkers = DefaultPoolConfig().MaxWorkers
	}
	if numWorkers > len(items) {
		numWorkers = len(items)
	}

	chunkSize := (len(items) + numWorkers - 1) / numWorkers
	results := make([]R, numWorkers)

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > len(items) {
			end = len(items)
		}
		if start >= end {
			continue
		}

		wg.Add(1)
		go func(workerID int, chunk []T) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
				results[workerID] = processor(ctx, chunk, workerID)
			}
		}(w, items[start:end])
	}

	wg.Wait()
	return reducer(results)
}

// ============================================================================
// Map-Reduce Pattern
// ============================================================================

// MapReduce applies a map function to each item in parallel and reduces the results.
func MapReduce[T any, M any, R any](
	ctx context.Context,
	items []T,
	config PoolConfig,
	mapper func(ctx context.Context, item T) M,
	reducer func(mapped []M) R,
) R {
	if len(items) == 0 {
		var zero R
		return zero
	}

	pool := NewWorkerPool[T, M](config)
	results := pool.ExecuteFunc(ctx, items, func(ctx context.Context, item T) (M, error) {
		return mapper(ctx, item), nil
	})

	mapped := make([]M, len(results))
	for i, r := range results {
		mapped[i] = r.Result
	}

	return reducer(mapped)
}

// ============================================================================
// Parallel For-Each
// ============================================================================

// ForEach executes a function for each item in parallel.
// Returns the number of items processed and any error that occurred.
func ForEach[T any](
	ctx context.Context,
	items []T,
	config PoolConfig,
	fn func(ctx context.Context, item T) error,
) (processed int64, firstError error) {
	if len(items) == 0 {
		return 0, nil
	}

	var processedCount atomic.Int64
	var errOnce sync.Once
	var mu sync.Mutex

	pool := NewWorkerPool[T, struct{}](config)
	pool.ExecuteFunc(ctx, items, func(ctx context.Context, item T) (struct{}, error) {
		err := fn(ctx, item)
		if err != nil {
			errOnce.Do(func() {
				mu.Lock()
				firstError = err
				mu.Unlock()
			})
			return struct{}{}, err
		}
		processedCount.Add(1)
		return struct{}{}, nil
	})

	return processedCount.Load(), firstError
}

// ============================================================================
// Parallel Aggregation
// ============================================================================

// AggregateResult holds the result of parallel aggregation.
type AggregateResult[K comparable, V any] struct {
	Data map[K]V
}

// ParallelAggregate aggregates data in parallel using per-worker local maps.
// This avoids lock contention by having each worker maintain its own map,
// then merging results at the end.
func ParallelAggregate[T any, K comparable, V any](
	ctx context.Context,
	items []T,
	config PoolConfig,
	extractor func(item T) (key K, value V),
	merger func(existing, new V) V,
) map[K]V {
	if len(items) == 0 {
		return make(map[K]V)
	}

	numWorkers := config.MaxWorkers
	if numWorkers <= 0 {
		numWorkers = DefaultPoolConfig().MaxWorkers
	}

	// Per-worker local maps
	localMaps := make([]map[K]V, numWorkers)
	for i := range localMaps {
		localMaps[i] = make(map[K]V)
	}

	chunkSize := (len(items) + numWorkers - 1) / numWorkers

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > len(items) {
			end = len(items)
		}
		if start >= end {
			continue
		}

		wg.Add(1)
		go func(workerID int, chunk []T) {
			defer wg.Done()
			localMap := localMaps[workerID]
			for _, item := range chunk {
				select {
				case <-ctx.Done():
					return
				default:
					key, value := extractor(item)
					if existing, ok := localMap[key]; ok {
						localMap[key] = merger(existing, value)
					} else {
						localMap[key] = value
					}
				}
			}
		}(w, items[start:end])
	}

	wg.Wait()

	// Merge all local maps
	result := make(map[K]V)
	for _, localMap := range localMaps {
		for k, v := range localMap {
			if existing, ok := result[k]; ok {
				result[k] = merger(existing, v)
			} else {
				result[k] = v
			}
		}
	}

	return result
}

// ============================================================================
// Progress Tracking
// ============================================================================

// ProgressTracker tracks progress of parallel operations.
type ProgressTracker struct {
	total     int64
	completed atomic.Int64
	callback  func(completed, total int64)
	interval  time.Duration
	stopCh    chan struct{}
	stopped   atomic.Bool
}

// NewProgressTracker creates a new progress tracker.
func NewProgressTracker(total int64, callback func(completed, total int64), interval time.Duration) *ProgressTracker {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return &ProgressTracker{
		total:    total,
		callback: callback,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins progress tracking in a background goroutine.
func (pt *ProgressTracker) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(pt.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-pt.stopCh:
				return
			case <-ticker.C:
				if pt.callback != nil {
					pt.callback(pt.completed.Load(), pt.total)
				}
			}
		}
	}()
}

// Increment increments the completed count.
func (pt *ProgressTracker) Increment() {
	pt.completed.Add(1)
}

// Add adds n to the completed count.
func (pt *ProgressTracker) Add(n int64) {
	pt.completed.Add(n)
}

// Stop stops progress tracking.
func (pt *ProgressTracker) Stop() {
	if pt.stopped.CompareAndSwap(false, true) {
		close(pt.stopCh)
	}
}

// Completed returns the current completed count.
func (pt *ProgressTracker) Completed() int64 {
	return pt.completed.Load()
}
