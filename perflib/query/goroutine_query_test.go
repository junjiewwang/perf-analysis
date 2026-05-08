package query

import (
	"testing"

	"github.com/junjiewwang/perf-analysis/perflib/model"
)

func TestGoroutineQueryEngine_QueryGroups(t *testing.T) {
	data := &model.PProfGoroutineData{
		TotalCount: 150,
		Distribution: []model.GoroutineGroup{
			{Count: 100, Percentage: 66.7, TopFunc: "runtime.gopark", Stack: []string{"runtime.gopark", "runtime.selectgo"}},
			{Count: 30, Percentage: 20.0, TopFunc: "net/http.(*conn).serve", Stack: []string{"net/http.(*conn).serve"}},
			{Count: 20, Percentage: 13.3, TopFunc: "sync.(*WaitGroup).Wait", Stack: []string{"sync.(*WaitGroup).Wait"}},
		},
		TopFuncs: []model.PProfTopFunc{
			{Name: "runtime.gopark", Flat: 100, FlatPct: 66.7},
			{Name: "net/http.(*conn).serve", Flat: 30, FlatPct: 20.0},
		},
	}

	engine := NewGoroutineQueryEngine(data)

	t.Run("QueryGroups_default_sort", func(t *testing.T) {
		result := engine.QueryGroups("count", 0)
		if result.TotalCount != 150 {
			t.Errorf("expected TotalCount=150, got %d", result.TotalCount)
		}
		if result.GroupCount != 3 {
			t.Errorf("expected GroupCount=3, got %d", result.GroupCount)
		}
		if len(result.Groups) != 3 {
			t.Errorf("expected 3 groups, got %d", len(result.Groups))
		}
		if result.Groups[0].Count != 100 {
			t.Errorf("expected first group count=100, got %d", result.Groups[0].Count)
		}
	})

	t.Run("QueryGroups_with_limit", func(t *testing.T) {
		result := engine.QueryGroups("count", 2)
		if len(result.Groups) != 2 {
			t.Errorf("expected 2 groups with limit, got %d", len(result.Groups))
		}
	})

	t.Run("QueryGroups_sort_by_percentage", func(t *testing.T) {
		result := engine.QueryGroups("percentage", 0)
		if result.Groups[0].Percentage < result.Groups[1].Percentage {
			t.Error("expected groups sorted by percentage descending")
		}
	})
}

func TestGoroutineQueryEngine_QueryStats(t *testing.T) {
	data := &model.PProfGoroutineData{
		TotalCount: 100,
		Distribution: []model.GoroutineGroup{
			{Count: 60, Percentage: 60.0, State: "running", TopFunc: "main.doWork"},
			{Count: 30, Percentage: 30.0, State: "waiting", TopFunc: "runtime.gopark"},
			{Count: 10, Percentage: 10.0, State: "running", TopFunc: "main.processQueue"},
		},
		TopFuncs: []model.PProfTopFunc{
			{Name: "main.doWork", Flat: 60, FlatPct: 60.0},
		},
	}

	engine := NewGoroutineQueryEngine(data)
	stats := engine.QueryStats()

	if stats.TotalCount != 100 {
		t.Errorf("expected TotalCount=100, got %d", stats.TotalCount)
	}
	if stats.GroupCount != 3 {
		t.Errorf("expected GroupCount=3, got %d", stats.GroupCount)
	}
	if len(stats.StateDistrib) == 0 {
		t.Fatal("expected non-empty StateDistrib")
	}

	// Check state aggregation
	runningFound := false
	for _, sd := range stats.StateDistrib {
		if sd.State == "running" {
			runningFound = true
			if sd.Count != 70 {
				t.Errorf("expected running count=70, got %d", sd.Count)
			}
		}
	}
	if !runningFound {
		t.Error("expected 'running' state in distribution")
	}

	if stats.LargestGroup == nil {
		t.Fatal("expected non-nil LargestGroup")
	}
	if stats.LargestGroup.Count != 60 {
		t.Errorf("expected largest group count=60, got %d", stats.LargestGroup.Count)
	}
}

func TestGoroutineQueryEngine_QueryIssues(t *testing.T) {
	t.Run("excessive_goroutine_count", func(t *testing.T) {
		data := &model.PProfGoroutineData{
			TotalCount:   15000,
			Distribution: []model.GoroutineGroup{},
		}
		engine := NewGoroutineQueryEngine(data)
		issues := engine.QueryIssues()

		found := false
		for _, issue := range issues {
			if issue.Type == "excessive" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected 'excessive' issue for count > 10000")
		}
	})

	t.Run("dominant_group", func(t *testing.T) {
		data := &model.PProfGoroutineData{
			TotalCount: 200,
			Distribution: []model.GoroutineGroup{
				{Count: 150, Percentage: 75.0, TopFunc: "runtime.selectgo"},
				{Count: 50, Percentage: 25.0, TopFunc: "main.worker"},
			},
		}
		engine := NewGoroutineQueryEngine(data)
		issues := engine.QueryIssues()

		found := false
		for _, issue := range issues {
			if issue.Type == "blocking" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected 'blocking' issue for dominant group")
		}
	})

	t.Run("no_issues_for_normal_data", func(t *testing.T) {
		data := &model.PProfGoroutineData{
			TotalCount: 10,
			Distribution: []model.GoroutineGroup{
				{Count: 5, Percentage: 50.0, TopFunc: "main.doWork"},
				{Count: 5, Percentage: 50.0, TopFunc: "main.listen"},
			},
		}
		engine := NewGoroutineQueryEngine(data)
		issues := engine.QueryIssues()

		if len(issues) != 0 {
			t.Errorf("expected no issues for normal data, got %d", len(issues))
		}
	})
}

func TestGoroutineQueryEngine_NilData(t *testing.T) {
	engine := NewGoroutineQueryEngine(nil)

	groups := engine.QueryGroups("count", 0)
	if groups.TotalCount != 0 {
		t.Error("expected empty result for nil data")
	}

	stats := engine.QueryStats()
	if stats.TotalCount != 0 {
		t.Error("expected empty stats for nil data")
	}

	issues := engine.QueryIssues()
	if issues != nil {
		t.Error("expected nil issues for nil data")
	}
}
