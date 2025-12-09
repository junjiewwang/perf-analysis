package statistics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/pkg/model"
)

func TestThreadStatsCalculator_Calculate_Basic(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "thread-A", TID: 1, CallStack: []string{"func1"}, Value: 100},
		{ThreadName: "thread-A", TID: 1, CallStack: []string{"func2"}, Value: 50},
		{ThreadName: "thread-B", TID: 2, CallStack: []string{"func1"}, Value: 30},
		{ThreadName: "thread-C", TID: 3, CallStack: []string{"func3"}, Value: 20},
	}

	calc := NewThreadStatsCalculator()
	result := calc.Calculate(samples)

	require.NotNil(t, result)
	assert.Equal(t, int64(200), result.TotalSamples)
	assert.Len(t, result.Threads, 3)

	// Check order (by samples descending)
	assert.Equal(t, "thread-A", result.Threads[0].ThreadName)
	assert.Equal(t, int64(150), result.Threads[0].Samples)
	assert.Equal(t, 1, result.Threads[0].TID)

	assert.Equal(t, "thread-B", result.Threads[1].ThreadName)
	assert.Equal(t, int64(30), result.Threads[1].Samples)

	assert.Equal(t, "thread-C", result.Threads[2].ThreadName)
	assert.Equal(t, int64(20), result.Threads[2].Samples)
}

func TestThreadStatsCalculator_Calculate_EmptySamples(t *testing.T) {
	calc := NewThreadStatsCalculator()
	result := calc.Calculate([]*model.Sample{})

	require.NotNil(t, result)
	assert.Equal(t, int64(0), result.TotalSamples)
	assert.Empty(t, result.Threads)
}

func TestThreadStatsCalculator_Calculate_MaxThreads(t *testing.T) {
	samples := make([]*model.Sample, 0)
	for i := 0; i < 20; i++ {
		samples = append(samples, &model.Sample{
			ThreadName: "thread-" + string(rune('A'+i)),
			TID:        i + 1,
			CallStack:  []string{"func"},
			Value:      int64(100 - i), // Descending values
		})
	}

	// Limit to top 5 threads
	calc := NewThreadStatsCalculator(WithMaxThreads(5))
	result := calc.Calculate(samples)

	assert.Len(t, result.Threads, 5)
	// First should have highest samples
	assert.Equal(t, int64(100), result.Threads[0].Samples)
}

func TestThreadStatsCalculator_Calculate_Percentages(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "t1", TID: 1, CallStack: []string{"f"}, Value: 50},
		{ThreadName: "t2", TID: 2, CallStack: []string{"f"}, Value: 30},
		{ThreadName: "t3", TID: 3, CallStack: []string{"f"}, Value: 20},
	}

	calc := NewThreadStatsCalculator()
	result := calc.Calculate(samples)

	assert.InDelta(t, 50.0, result.Threads[0].Percentage, 0.01)
	assert.InDelta(t, 30.0, result.Threads[1].Percentage, 0.01)
	assert.InDelta(t, 20.0, result.Threads[2].Percentage, 0.01)
}

func TestThreadStatsCalculator_Calculate_ThreadsMap(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"f"}, Value: 100},
		{ThreadName: "worker", TID: 2, CallStack: []string{"f"}, Value: 50},
	}

	calc := NewThreadStatsCalculator()
	result := calc.Calculate(samples)

	// Check map
	assert.Contains(t, result.ThreadsMap, "main")
	assert.Contains(t, result.ThreadsMap, "worker")

	mainInfo := result.ThreadsMap["main"]
	assert.Equal(t, int64(100), mainInfo.Samples)
	assert.Equal(t, 1, mainInfo.TID)
}

func TestThreadStatsResult_ToActiveThreadsList(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "thread-A", TID: 1, CallStack: []string{"f"}, Value: 100},
		{ThreadName: "thread-B", TID: 2, CallStack: []string{"f"}, Value: 50},
	}

	calc := NewThreadStatsCalculator()
	result := calc.Calculate(samples)

	activeList := result.ToActiveThreadsList()
	assert.Len(t, activeList, 2)

	assert.Equal(t, 1, activeList[0].TID)
	assert.Equal(t, "thread-A", activeList[0].Comm)
	assert.Equal(t, int64(100), activeList[0].Count)

	assert.Equal(t, 2, activeList[1].TID)
	assert.Equal(t, "thread-B", activeList[1].Comm)
	assert.Equal(t, int64(50), activeList[1].Count)
}

func TestThreadStatsResult_GetThreadByTID(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "thread-A", TID: 1, CallStack: []string{"f"}, Value: 100},
		{ThreadName: "thread-B", TID: 2, CallStack: []string{"f"}, Value: 50},
	}

	calc := NewThreadStatsCalculator()
	result := calc.Calculate(samples)

	// Find existing thread
	thread := result.GetThreadByTID(1)
	require.NotNil(t, thread)
	assert.Equal(t, "thread-A", thread.ThreadName)

	// Find non-existing thread
	thread = result.GetThreadByTID(999)
	assert.Nil(t, thread)
}

func TestThreadStatsResult_GetThreadByName(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"f"}, Value: 100},
		{ThreadName: "worker", TID: 2, CallStack: []string{"f"}, Value: 50},
	}

	calc := NewThreadStatsCalculator()
	result := calc.Calculate(samples)

	// Find existing thread
	thread := result.GetThreadByName("main")
	require.NotNil(t, thread)
	assert.Equal(t, 1, thread.TID)

	// Find non-existing thread
	thread = result.GetThreadByName("not-exists")
	assert.Nil(t, thread)
}

func TestThreadStatsCalculator_SameTIDDifferentNames(t *testing.T) {
	// This tests the case where a thread changes name during profiling
	samples := []*model.Sample{
		{ThreadName: "thread-old", TID: 1, CallStack: []string{"f"}, Value: 50},
		{ThreadName: "thread-new", TID: 1, CallStack: []string{"f"}, Value: 50},
	}

	calc := NewThreadStatsCalculator()
	result := calc.Calculate(samples)

	// Should be aggregated by TID, but thread name might be either
	assert.Len(t, result.Threads, 1)
	assert.Equal(t, int64(100), result.Threads[0].Samples)
	assert.Equal(t, 1, result.Threads[0].TID)
}

// Benchmark
func BenchmarkThreadStatsCalculator_Calculate(b *testing.B) {
	samples := make([]*model.Sample, 10000)
	threads := []string{"main", "worker-1", "worker-2", "worker-3", "gc"}

	for i := 0; i < 10000; i++ {
		idx := i % len(threads)
		samples[i] = &model.Sample{
			ThreadName: threads[idx],
			TID:        idx + 1,
			CallStack:  []string{"func"},
			Value:      int64(100 + i%50),
		}
	}

	calc := NewThreadStatsCalculator()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		calc.Calculate(samples)
	}
}
