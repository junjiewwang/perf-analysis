package query

import (
	"testing"

	"github.com/junjiewwang/perf-analysis/perflib/flamegraph"
	"github.com/junjiewwang/perf-analysis/perflib/model"
)

func TestSearchEngine_SearchFunctions(t *testing.T) {
	cpuResult := flamegraph.NewCPUAnalysisResult()
	cpuResult.TopFuncs = []*flamegraph.TopFunction{
		{Name: "main.processRequest", Samples: 100, Percentage: 50.0, Module: "main"},
		{Name: "net/http.ServeHTTP", Samples: 80, Percentage: 40.0, Module: "net/http"},
		{Name: "encoding/json.Marshal", Samples: 20, Percentage: 10.0, Module: "encoding/json"},
	}
	cpuResult.Threads = []*flamegraph.ThreadInfo{
		{TID: 1, Name: "main-thread", Samples: 200, Percentage: 100.0},
	}

	engine := NewSearchEngine().WithCPUAnalysis(cpuResult)

	t.Run("search_by_function_name", func(t *testing.T) {
		results := engine.Search("process", "function", 10)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Name != "main.processRequest" {
			t.Errorf("expected 'main.processRequest', got '%s'", results[0].Name)
		}
		if results[0].Type != "function" {
			t.Errorf("expected type='function', got '%s'", results[0].Type)
		}
	})

	t.Run("search_case_insensitive", func(t *testing.T) {
		results := engine.Search("MARSHAL", "function", 10)
		if len(results) != 1 {
			t.Fatalf("expected 1 result for case-insensitive search, got %d", len(results))
		}
	})

	t.Run("search_threads", func(t *testing.T) {
		results := engine.Search("main", "thread", 10)
		if len(results) != 1 {
			t.Fatalf("expected 1 thread result, got %d", len(results))
		}
		if results[0].Type != "thread" {
			t.Errorf("expected type='thread', got '%s'", results[0].Type)
		}
	})

	t.Run("search_all_types", func(t *testing.T) {
		results := engine.Search("main", "", 10)
		if len(results) < 2 {
			t.Fatalf("expected at least 2 results for 'main' across all types, got %d", len(results))
		}
	})

	t.Run("search_no_results", func(t *testing.T) {
		results := engine.Search("nonexistent", "", 10)
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("search_with_limit", func(t *testing.T) {
		results := engine.Search("http", "", 1)
		if len(results) > 1 {
			t.Errorf("expected at most 1 result with limit=1, got %d", len(results))
		}
	})

	t.Run("search_empty_query", func(t *testing.T) {
		results := engine.Search("", "", 10)
		if results != nil {
			t.Errorf("expected nil for empty query, got %d results", len(results))
		}
	})
}

func TestSearchEngine_WithGoroutineData(t *testing.T) {
	data := &model.PProfGoroutineData{
		TotalCount: 100,
		Distribution: []model.GoroutineGroup{
			{Count: 50, Percentage: 50.0, TopFunc: "runtime.gopark", State: "waiting"},
			{Count: 30, Percentage: 30.0, TopFunc: "main.workerLoop", State: "running"},
		},
	}

	goroutineEngine := NewGoroutineQueryEngine(data)
	searchEngine := NewSearchEngine().WithGoroutineData(goroutineEngine)

	t.Run("search_goroutine_groups", func(t *testing.T) {
		results := searchEngine.Search("worker", "goroutine_group", 10)
		if len(results) != 1 {
			t.Fatalf("expected 1 goroutine group result, got %d", len(results))
		}
		if results[0].Type != "goroutine_group" {
			t.Errorf("expected type='goroutine_group', got '%s'", results[0].Type)
		}
		if results[0].Samples != 30 {
			t.Errorf("expected samples=30, got %d", results[0].Samples)
		}
	})
}

func TestSearchEngine_SortBysamples(t *testing.T) {
	cpuResult := flamegraph.NewCPUAnalysisResult()
	cpuResult.TopFuncs = []*flamegraph.TopFunction{
		{Name: "a.func1", Samples: 10, Percentage: 10.0},
		{Name: "a.func2", Samples: 100, Percentage: 50.0},
		{Name: "a.func3", Samples: 50, Percentage: 25.0},
	}

	engine := NewSearchEngine().WithCPUAnalysis(cpuResult)
	results := engine.Search("a.func", "function", 10)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Should be sorted by samples descending
	if results[0].Samples != 100 {
		t.Errorf("expected first result samples=100, got %d", results[0].Samples)
	}
	if results[1].Samples != 50 {
		t.Errorf("expected second result samples=50, got %d", results[1].Samples)
	}
	if results[2].Samples != 10 {
		t.Errorf("expected third result samples=10, got %d", results[2].Samples)
	}
}
