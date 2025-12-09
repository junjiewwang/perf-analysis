package advisor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/perf-analysis/internal/statistics"
	"github.com/perf-analysis/pkg/model"
)

func TestNewAdvisor(t *testing.T) {
	advisor := NewAdvisor()

	assert.NotNil(t, advisor)
	assert.NotEmpty(t, advisor.rules)
}

func TestNewAdvisorWithRules(t *testing.T) {
	rules := []Rule{
		{Type: "test", Name: "test_rule"},
	}

	advisor := NewAdvisorWithRules(rules)

	assert.Len(t, advisor.rules, 1)
	assert.Equal(t, "test_rule", advisor.rules[0].Name)
}

func TestAdvisor_Advise_HighCPUFunction(t *testing.T) {
	advisor := NewAdvisor()

	ctx := &RuleContext{
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypePerf,
		TopFuncsResult: &statistics.TopFuncsResult{
			TopFuncs: []statistics.TopFuncEntry{
				{Name: "com.example.App.heavyCompute", SelfSamples: 2000, SelfPercent: 20.0},
				{Name: "com.example.App.lightTask", SelfSamples: 100, SelfPercent: 1.0},
			},
		},
	}

	suggestions := advisor.Advise(ctx)

	// Should have at least one suggestion for high CPU function
	var foundHighCPU bool
	for _, s := range suggestions {
		if s.Type == "cpu_hotspot" {
			foundHighCPU = true
			assert.Contains(t, s.Suggestion, "heavyCompute")
		}
	}
	assert.True(t, foundHighCPU, "Should find high CPU function suggestion")
}

func TestAdvisor_Advise_GCOverhead(t *testing.T) {
	advisor := NewAdvisor()

	ctx := &RuleContext{
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypePerf,
		TopFuncsResult: &statistics.TopFuncsResult{
			TopFuncs: []statistics.TopFuncEntry{
				{Name: "java.lang.ref.Reference$ReferenceHandler.run", SelfSamples: 100, SelfPercent: 1.0},
				{Name: "GC_concurrent_sweep", SelfSamples: 400, SelfPercent: 4.0},
				{Name: "GC_mark_phase", SelfSamples: 200, SelfPercent: 2.0},
			},
		},
	}

	suggestions := advisor.Advise(ctx)

	// Should have GC overhead suggestion
	var foundGC bool
	for _, s := range suggestions {
		if s.Type == "gc_overhead" {
			foundGC = true
			assert.Contains(t, s.Suggestion, "GC")
		}
	}
	assert.True(t, foundGC, "Should find GC overhead suggestion")
}

func TestAdvisor_Advise_LockContention(t *testing.T) {
	advisor := NewAdvisor()

	ctx := &RuleContext{
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypePerf,
		TopFuncsResult: &statistics.TopFuncsResult{
			TopFuncs: []statistics.TopFuncEntry{
				{Name: "java.lang.Object.monitorEnter", SelfSamples: 300, SelfPercent: 3.0},
				{Name: "pthread_mutex_lock", SelfSamples: 200, SelfPercent: 2.0},
			},
		},
	}

	suggestions := advisor.Advise(ctx)

	// Should have lock contention suggestion
	var foundLock bool
	for _, s := range suggestions {
		if s.Type == "lock_contention" {
			foundLock = true
			assert.Contains(t, s.Suggestion, "锁竞争")
		}
	}
	assert.True(t, foundLock, "Should find lock contention suggestion")
}

func TestAdvisor_Advise_FrequentAllocation(t *testing.T) {
	advisor := NewAdvisor()

	ctx := &RuleContext{
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypeAsyncAlloc,
		TopFuncsResult: &statistics.TopFuncsResult{
			TopFuncs: []statistics.TopFuncEntry{
				{Name: "java.lang.StringBuilder.toString", SelfSamples: 800, SelfPercent: 8.0},
				{Name: "java.util.ArrayList.grow", SelfSamples: 100, SelfPercent: 1.0},
			},
		},
	}

	suggestions := advisor.Advise(ctx)

	// Should have frequent allocation suggestion
	var foundAlloc bool
	for _, s := range suggestions {
		if s.Type == "frequent_allocation" {
			foundAlloc = true
			assert.Contains(t, s.Suggestion, "StringBuilder")
		}
	}
	assert.True(t, foundAlloc, "Should find frequent allocation suggestion")
}

func TestAdvisor_Advise_ReflectionUsage(t *testing.T) {
	advisor := NewAdvisor()

	ctx := &RuleContext{
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypePerf,
		TopFuncsResult: &statistics.TopFuncsResult{
			TopFuncs: []statistics.TopFuncEntry{
				{Name: "java.lang.reflect.Method.invoke", SelfSamples: 300, SelfPercent: 3.0},
				{Name: "java.lang.Class.forName", SelfSamples: 250, SelfPercent: 2.5},
			},
		},
	}

	suggestions := advisor.Advise(ctx)

	// Should have reflection suggestion
	var foundReflection bool
	for _, s := range suggestions {
		if s.Type == "reflection_usage" {
			foundReflection = true
			assert.Contains(t, s.Suggestion, "反射")
		}
	}
	assert.True(t, foundReflection, "Should find reflection usage suggestion")
}

func TestAdvisor_Advise_NoSuggestions(t *testing.T) {
	advisor := NewAdvisor()

	ctx := &RuleContext{
		TaskType:     model.TaskTypeJava,
		ProfilerType: model.ProfilerTypePerf,
		TopFuncsResult: &statistics.TopFuncsResult{
			TopFuncs: []statistics.TopFuncEntry{
				{Name: "com.example.App.normalFunction", SelfSamples: 100, SelfPercent: 1.0},
			},
		},
	}

	suggestions := advisor.Advise(ctx)

	// Should have no suggestions
	assert.Empty(t, suggestions)
}

func TestAdvisor_Advise_NilTopFuncs(t *testing.T) {
	advisor := NewAdvisor()

	ctx := &RuleContext{
		TaskType:       model.TaskTypeJava,
		ProfilerType:   model.ProfilerTypePerf,
		TopFuncsResult: nil,
	}

	suggestions := advisor.Advise(ctx)

	// Should not panic, return empty
	assert.Empty(t, suggestions)
}

func TestFormatPercent(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{10.0, "10"},
		{10.5, "10.5"},
		{10.55, "10.55"},
		{0.0, "0"},
		{0.5, "0.5"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatPercent(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckHighCPUFunction(t *testing.T) {
	ctx := &RuleContext{
		TopFuncsResult: &statistics.TopFuncsResult{
			TopFuncs: []statistics.TopFuncEntry{
				{Name: "hotFunction", SelfPercent: 20.0},
				{Name: "coldFunction", SelfPercent: 5.0},
			},
		},
	}

	suggestions := checkHighCPUFunction(ctx)

	require.Len(t, suggestions, 1)
	assert.Equal(t, "hotFunction", suggestions[0].FuncName)
}

func TestCheckGCOverhead_BelowThreshold(t *testing.T) {
	ctx := &RuleContext{
		TopFuncsResult: &statistics.TopFuncsResult{
			TopFuncs: []statistics.TopFuncEntry{
				{Name: "GC_sweep", SelfPercent: 2.0},
			},
		},
	}

	suggestions := checkGCOverhead(ctx)

	// Below 5% threshold
	assert.Empty(t, suggestions)
}

func TestCheckLockContention_BelowThreshold(t *testing.T) {
	ctx := &RuleContext{
		TopFuncsResult: &statistics.TopFuncsResult{
			TopFuncs: []statistics.TopFuncEntry{
				{Name: "pthread_mutex_lock", SelfPercent: 0.5},
			},
		},
	}

	suggestions := checkLockContention(ctx)

	// Lock function below 1% threshold is not counted
	assert.Empty(t, suggestions)
}
