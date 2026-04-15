package hprof

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPool_Execute(t *testing.T) {
	config := DefaultPoolConfig()
	pool := NewWorkerPool[int, int](config)

	inputs := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	results := pool.ExecuteFunc(context.Background(), inputs, func(ctx context.Context, n int) (int, error) {
		return n * 2, nil
	})

	if len(results) != len(inputs) {
		t.Errorf("Expected %d results, got %d", len(inputs), len(results))
	}

	for i, r := range results {
		expected := inputs[i] * 2
		if r.Result != expected {
			t.Errorf("Result[%d]: expected %d, got %d", i, expected, r.Result)
		}
		if r.Error != nil {
			t.Errorf("Result[%d]: unexpected error: %v", i, r.Error)
		}
	}
}

func TestWorkerPool_Timeout(t *testing.T) {
	config := DefaultPoolConfig().WithTimeout(50 * time.Millisecond)
	pool := NewWorkerPool[int, int](config)

	inputs := []int{1, 2, 3, 4, 5}
	ctx := context.Background()

	results := pool.ExecuteFunc(ctx, inputs, func(ctx context.Context, n int) (int, error) {
		time.Sleep(100 * time.Millisecond) // Sleep longer than timeout
		return n, nil
	})

	// Some results may be incomplete due to timeout
	// This is expected behavior
	t.Logf("Got %d results with timeout", len(results))
}

func TestChunkProcessor(t *testing.T) {
	config := DefaultPoolConfig()
	processor := NewChunkProcessor[int, int](config)

	items := make([]int, 1000)
	for i := range items {
		items[i] = i
	}

	sum := processor.ProcessChunks(
		context.Background(),
		items,
		func(ctx context.Context, chunk []int, workerID int) int {
			localSum := 0
			for _, n := range chunk {
				localSum += n
			}
			return localSum
		},
		func(results []int) int {
			total := 0
			for _, r := range results {
				total += r
			}
			return total
		},
	)

	expected := 999 * 1000 / 2 // Sum of 0 to 999
	if sum != expected {
		t.Errorf("Expected sum %d, got %d", expected, sum)
	}
}

func TestParallelAggregate(t *testing.T) {
	config := DefaultPoolConfig()
	ctx := context.Background()

	items := []string{"apple", "banana", "apple", "cherry", "banana", "apple"}

	counts := ParallelAggregate(
		ctx,
		items,
		config,
		func(item string) (string, int) {
			return item, 1
		},
		func(existing, new int) int {
			return existing + new
		},
	)

	if counts["apple"] != 3 {
		t.Errorf("Expected apple count 3, got %d", counts["apple"])
	}
	if counts["banana"] != 2 {
		t.Errorf("Expected banana count 2, got %d", counts["banana"])
	}
	if counts["cherry"] != 1 {
		t.Errorf("Expected cherry count 1, got %d", counts["cherry"])
	}
}

func TestForEach(t *testing.T) {
	config := DefaultPoolConfig()
	ctx := context.Background()

	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}

	var sum atomic.Int64
	processed, err := ForEach(ctx, items, config, func(ctx context.Context, n int) error {
		sum.Add(int64(n))
		return nil
	})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if processed != int64(len(items)) {
		t.Errorf("Expected %d processed, got %d", len(items), processed)
	}

	expected := int64(99 * 100 / 2)
	if sum.Load() != expected {
		t.Errorf("Expected sum %d, got %d", expected, sum.Load())
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
	time.Sleep(20 * time.Millisecond) // Wait for callback

	tracker.Stop()
	cancel()

	if lastTotal != 100 {
		t.Errorf("Expected total 100, got %d", lastTotal)
	}
	if lastCompleted < 50 {
		t.Errorf("Expected completed >= 50, got %d", lastCompleted)
	}
}

func TestConcurrentMap(t *testing.T) {
	m := NewConcurrentMap[string, int]()

	m.Set("a", 1)
	m.Set("b", 2)

	if v, ok := m.Get("a"); !ok || v != 1 {
		t.Errorf("Expected a=1, got %d, ok=%v", v, ok)
	}

	m.Update("a", func(existing int, exists bool) int {
		return existing + 10
	})

	if v, ok := m.Get("a"); !ok || v != 11 {
		t.Errorf("Expected a=11 after update, got %d", v)
	}

	if m.Len() != 2 {
		t.Errorf("Expected len 2, got %d", m.Len())
	}

	m.Delete("b")
	if m.Len() != 1 {
		t.Errorf("Expected len 1 after delete, got %d", m.Len())
	}
}

func TestMapReduce(t *testing.T) {
	config := DefaultPoolConfig()
	ctx := context.Background()

	items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	// Map: square each number, Reduce: sum
	result := MapReduce(
		ctx,
		items,
		config,
		func(ctx context.Context, n int) int {
			return n * n
		},
		func(mapped []int) int {
			sum := 0
			for _, v := range mapped {
				sum += v
			}
			return sum
		},
	)

	// Sum of squares: 1 + 4 + 9 + 16 + 25 + 36 + 49 + 64 + 81 + 100 = 385
	expected := 385
	if result != expected {
		t.Errorf("Expected %d, got %d", expected, result)
	}
}

func BenchmarkWorkerPool(b *testing.B) {
	config := DefaultPoolConfig()
	pool := NewWorkerPool[int, int](config)

	inputs := make([]int, 10000)
	for i := range inputs {
		inputs[i] = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.ExecuteFunc(context.Background(), inputs, func(ctx context.Context, n int) (int, error) {
			// Simulate some work
			result := 0
			for j := 0; j < 100; j++ {
				result += n * j
			}
			return result, nil
		})
	}
}

func BenchmarkChunkProcessor(b *testing.B) {
	config := DefaultPoolConfig()
	processor := NewChunkProcessor[int, int](config)

	items := make([]int, 100000)
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
				for _, n := range chunk {
					sum += n
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
