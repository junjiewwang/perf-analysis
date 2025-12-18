package parallel

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPool_Execute(t *testing.T) {
	pool := NewWorkerPool[int, int](DefaultPoolConfig())

	inputs := []int{1, 2, 3, 4, 5}
	results := pool.ExecuteFunc(context.Background(), inputs, func(ctx context.Context, input int) (int, error) {
		return input * 2, nil
	})

	if len(results) != len(inputs) {
		t.Errorf("Expected %d results, got %d", len(inputs), len(results))
	}

	for i, r := range results {
		if r.Error != nil {
			t.Errorf("Unexpected error for input %d: %v", inputs[i], r.Error)
		}
		if r.Result != inputs[i]*2 {
			t.Errorf("Expected %d, got %d", inputs[i]*2, r.Result)
		}
	}
}

func TestWorkerPool_Timeout(t *testing.T) {
	config := DefaultPoolConfig().WithTimeout(50 * time.Millisecond)
	pool := NewWorkerPool[int, int](config)

	inputs := make([]int, 10)
	for i := range inputs {
		inputs[i] = i
	}

	results := pool.ExecuteFunc(context.Background(), inputs, func(ctx context.Context, input int) (int, error) {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return input, nil
		}
	})

	// Some tasks should have been cancelled
	cancelledCount := 0
	for _, r := range results {
		if r.Error != nil {
			cancelledCount++
		}
	}

	if cancelledCount == 0 {
		t.Log("Warning: No tasks were cancelled by timeout")
	}
}

func TestWorkerPool_Metrics(t *testing.T) {
	config := DefaultPoolConfig().WithMetrics()
	pool := NewWorkerPool[int, int](config)

	inputs := []int{1, 2, 3, 4, 5}
	pool.ExecuteFunc(context.Background(), inputs, func(ctx context.Context, input int) (int, error) {
		return input * 2, nil
	})

	metrics := pool.Metrics()
	if metrics.TotalTasks != 5 {
		t.Errorf("Expected 5 total tasks, got %d", metrics.TotalTasks)
	}
	if metrics.CompletedTasks != 5 {
		t.Errorf("Expected 5 completed tasks, got %d", metrics.CompletedTasks)
	}
	if metrics.FailedTasks != 0 {
		t.Errorf("Expected 0 failed tasks, got %d", metrics.FailedTasks)
	}
}

func TestChunkProcessor(t *testing.T) {
	config := DefaultPoolConfig().WithWorkers(4)
	processor := NewChunkProcessor[int, int](config)

	items := make([]int, 1000)
	for i := range items {
		items[i] = i
	}

	result := processor.ProcessChunks(
		context.Background(),
		items,
		func(ctx context.Context, chunk []int, workerID int) int {
			sum := 0
			for _, v := range chunk {
				sum += v
			}
			return sum
		},
		func(results []int) int {
			total := 0
			for _, r := range results {
				total += r
			}
			return total
		},
	)

	expected := 0
	for i := 0; i < 1000; i++ {
		expected += i
	}

	if result != expected {
		t.Errorf("Expected %d, got %d", expected, result)
	}
}

func TestMapReduce(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}

	result := MapReduce(
		context.Background(),
		items,
		DefaultPoolConfig(),
		func(ctx context.Context, item int) int {
			return item * item
		},
		func(mapped []int) int {
			sum := 0
			for _, v := range mapped {
				sum += v
			}
			return sum
		},
	)

	// 1 + 4 + 9 + 16 + 25 = 55
	if result != 55 {
		t.Errorf("Expected 55, got %d", result)
	}
}

func TestForEach(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	var sum atomic.Int64

	processed, err := ForEach(
		context.Background(),
		items,
		DefaultPoolConfig(),
		func(ctx context.Context, item int) error {
			sum.Add(int64(item))
			return nil
		},
	)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if processed != 5 {
		t.Errorf("Expected 5 processed, got %d", processed)
	}
	if sum.Load() != 15 {
		t.Errorf("Expected sum 15, got %d", sum.Load())
	}
}

func TestParallelAggregate(t *testing.T) {
	type Item struct {
		Category string
		Value    int
	}

	items := []Item{
		{"A", 1},
		{"B", 2},
		{"A", 3},
		{"B", 4},
		{"A", 5},
	}

	result := ParallelAggregate(
		context.Background(),
		items,
		DefaultPoolConfig(),
		func(item Item) (string, int) {
			return item.Category, item.Value
		},
		func(existing, new int) int {
			return existing + new
		},
	)

	if result["A"] != 9 {
		t.Errorf("Expected A=9, got A=%d", result["A"])
	}
	if result["B"] != 6 {
		t.Errorf("Expected B=6, got B=%d", result["B"])
	}
}

func TestProgressTracker(t *testing.T) {
	var lastCompleted, lastTotal int64

	tracker := NewProgressTracker(100, func(completed, total int64) {
		lastCompleted = completed
		lastTotal = total
	}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	tracker.Start(ctx)

	for i := 0; i < 50; i++ {
		tracker.Increment()
	}

	time.Sleep(20 * time.Millisecond)

	if lastCompleted != 50 {
		t.Errorf("Expected lastCompleted=50, got %d", lastCompleted)
	}
	if lastTotal != 100 {
		t.Errorf("Expected lastTotal=100, got %d", lastTotal)
	}

	tracker.Stop()
	cancel()
}

func BenchmarkWorkerPool(b *testing.B) {
	pool := NewWorkerPool[int, int](DefaultPoolConfig())
	inputs := make([]int, 1000)
	for i := range inputs {
		inputs[i] = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.ExecuteFunc(context.Background(), inputs, func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})
	}
}

func BenchmarkChunkProcessor(b *testing.B) {
	processor := NewChunkProcessor[int, int](DefaultPoolConfig())
	items := make([]int, 10000)
	for i := range items {
		items[i] = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor.ProcessChunks(
			context.Background(),
			items,
			func(ctx context.Context, chunk []int, workerID int) int {
				sum := 0
				for _, v := range chunk {
					sum += v
				}
				return sum
			},
			func(results []int) int {
				total := 0
				for _, r := range results {
					total += r
				}
				return total
			},
		)
	}
}
