package statistics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/pkg/model"
)

func TestTopFuncsCalculator_Calculate_Basic(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "thread-1", TID: 1, CallStack: []string{"a", "b", "hot_func"}, Value: 100},
		{ThreadName: "thread-2", TID: 2, CallStack: []string{"a", "c", "hot_func"}, Value: 80},
		{ThreadName: "thread-3", TID: 3, CallStack: []string{"x", "y", "other_func"}, Value: 50},
		{ThreadName: "thread-4", TID: 4, CallStack: []string{"m", "n", "rare_func"}, Value: 10},
	}

	calc := NewTopFuncsCalculator(WithTopN(3))
	result := calc.Calculate(samples)

	require.NotNil(t, result)
	assert.Equal(t, int64(240), result.TotalSamples)
	assert.Len(t, result.TopFuncs, 3)

	// Check order (by samples descending)
	assert.Equal(t, "hot_func", result.TopFuncs[0].Name)
	assert.Equal(t, int64(180), result.TopFuncs[0].SelfSamples)

	assert.Equal(t, "other_func", result.TopFuncs[1].Name)
	assert.Equal(t, int64(50), result.TopFuncs[1].SelfSamples)

	assert.Equal(t, "rare_func", result.TopFuncs[2].Name)
	assert.Equal(t, int64(10), result.TopFuncs[2].SelfSamples)
}

func TestTopFuncsCalculator_Calculate_EmptySamples(t *testing.T) {
	calc := NewTopFuncsCalculator()
	result := calc.Calculate([]*model.Sample{})

	require.NotNil(t, result)
	assert.Equal(t, int64(0), result.TotalSamples)
	assert.Empty(t, result.TopFuncs)
}

func TestTopFuncsCalculator_Calculate_SwapperExclusion(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "main", TID: 1, CallStack: []string{"main_func"}, Value: 100},
		{ThreadName: "swapper-0", TID: 0, CallStack: []string{"idle_func"}, Value: 200},
	}

	// Without swapper
	calc := NewTopFuncsCalculator(WithSwapper(false))
	result := calc.Calculate(samples)

	assert.Equal(t, int64(300), result.TotalSamples)
	assert.Equal(t, int64(200), result.SwapperSamples)

	// main_func should be 100% (of non-swapper)
	require.Len(t, result.TopFuncs, 1)
	assert.Equal(t, "main_func", result.TopFuncs[0].Name)
	assert.InDelta(t, 100.0, result.TopFuncs[0].SelfPercent, 0.01)

	// With swapper included
	calcWithSwapper := NewTopFuncsCalculator(WithSwapper(true))
	resultWithSwapper := calcWithSwapper.Calculate(samples)

	require.Len(t, resultWithSwapper.TopFuncs, 2)
	// idle_func should be first (more samples)
	assert.Equal(t, "idle_func", resultWithSwapper.TopFuncs[0].Name)
}

func TestTopFuncsCalculator_Calculate_TopN(t *testing.T) {
	samples := make([]*model.Sample, 0)
	for i := 0; i < 20; i++ {
		samples = append(samples, &model.Sample{
			ThreadName: "thread",
			TID:        i,
			CallStack:  []string{"func_" + string(rune('a'+i))},
			Value:      int64(100 - i), // Descending values
		})
	}

	// Request top 5
	calc := NewTopFuncsCalculator(WithTopN(5))
	result := calc.Calculate(samples)

	assert.Len(t, result.TopFuncs, 5)
	// First should have highest samples
	assert.Equal(t, int64(100), result.TopFuncs[0].SelfSamples)
}

func TestTopFuncsCalculator_Calculate_Percentages(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "t1", TID: 1, CallStack: []string{"func_a"}, Value: 50},
		{ThreadName: "t2", TID: 2, CallStack: []string{"func_b"}, Value: 30},
		{ThreadName: "t3", TID: 3, CallStack: []string{"func_c"}, Value: 20},
	}

	calc := NewTopFuncsCalculator()
	result := calc.Calculate(samples)

	assert.InDelta(t, 50.0, result.TopFuncs[0].SelfPercent, 0.01)
	assert.InDelta(t, 30.0, result.TopFuncs[1].SelfPercent, 0.01)
	assert.InDelta(t, 20.0, result.TopFuncs[2].SelfPercent, 0.01)
}

func TestTopFuncsCalculator_Calculate_Callstacks(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "t1", TID: 1, CallStack: []string{"a", "b", "hot"}, Value: 50},
		{ThreadName: "t2", TID: 2, CallStack: []string{"a", "c", "hot"}, Value: 30},
		{ThreadName: "t3", TID: 3, CallStack: []string{"x", "y", "hot"}, Value: 20},
	}

	calc := NewTopFuncsCalculator()
	result := calc.Calculate(samples)

	callstacks := result.GetTopFuncsCallstacks(5)
	require.Contains(t, callstacks, "hot")
	assert.Equal(t, 3, callstacks["hot"].Count)
	assert.LessOrEqual(t, len(callstacks["hot"].CallStacks), 5)
}

func TestTopFuncsCalculator_EmptyCallStack(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "t1", TID: 1, CallStack: []string{}, Value: 100},
	}

	calc := NewTopFuncsCalculator()
	result := calc.Calculate(samples)

	// Samples with empty callstack are still counted in total
	// but don't contribute to top functions (no leaf function)
	assert.Equal(t, int64(100), result.TotalSamples)
	assert.Empty(t, result.TopFuncs)
}

func TestTopFuncsResult_TopFuncsMap(t *testing.T) {
	samples := []*model.Sample{
		{ThreadName: "t1", TID: 1, CallStack: []string{"func_a"}, Value: 100},
		{ThreadName: "t2", TID: 2, CallStack: []string{"func_b"}, Value: 50},
	}

	calc := NewTopFuncsCalculator()
	result := calc.Calculate(samples)

	// Check map
	assert.Contains(t, result.TopFuncsMap, "func_a")
	assert.Contains(t, result.TopFuncsMap, "func_b")

	// Verify percentages in map match
	for _, entry := range result.TopFuncs {
		mapValue, ok := result.TopFuncsMap[entry.Name]
		assert.True(t, ok)
		assert.InDelta(t, entry.SelfPercent, mapValue.Self, 0.01)
	}
}

// Benchmark
func BenchmarkTopFuncsCalculator_Calculate(b *testing.B) {
	samples := make([]*model.Sample, 10000)
	funcs := []string{"func_a", "func_b", "func_c", "func_d", "func_e"}

	for i := 0; i < 10000; i++ {
		samples[i] = &model.Sample{
			ThreadName: "thread",
			TID:        i % 100,
			CallStack:  []string{"root", "middle", funcs[i%len(funcs)]},
			Value:      int64(100 + i%50),
		}
	}

	calc := NewTopFuncsCalculator(WithTopN(15))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		calc.Calculate(samples)
	}
}
