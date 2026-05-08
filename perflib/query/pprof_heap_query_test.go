package query

import (
	"testing"

	"github.com/junjiewwang/perf-analysis/perflib/model"
)

// ============================================================
// PProfHeapQueryHelper Tests
// ============================================================

func newTestPProfHeapData() *model.PProfHeapData {
	return &model.PProfHeapData{
		InuseSpace: &model.PProfMemoryStats{
			Total: 1048576, // 1MB
			Unit:  "bytes",
			TopFuncs: []model.PProfTopFunc{
				{Name: "myapp.allocBuffer", Flat: 524288, FlatPct: 50.0, Cum: 786432, CumPct: 75.0, Module: "myapp"},
				{Name: "github.com/lib/pq.(*conn).query", Flat: 262144, FlatPct: 25.0, Cum: 262144, CumPct: 25.0, Module: "github.com/lib/pq"},
				{Name: "net/http.(*Transport).dialConn", Flat: 131072, FlatPct: 12.5, Cum: 131072, CumPct: 12.5, Module: "net/http"},
				{Name: "encoding/json.(*Decoder).Decode", Flat: 65536, FlatPct: 6.25, Cum: 65536, CumPct: 6.25, Module: "encoding/json"},
				{Name: "bytes.makeSlice", Flat: 65536, FlatPct: 6.25, Cum: 65536, CumPct: 6.25, Module: "bytes"},
			},
			TopNCount: 5,
		},
		InuseObjects: &model.PProfMemoryStats{
			Total: 5000,
			Unit:  "objects",
			TopFuncs: []model.PProfTopFunc{
				{Name: "myapp.createWidget", Flat: 2000, FlatPct: 40.0, Cum: 3000, CumPct: 60.0, Module: "myapp"},
				{Name: "sync.(*Pool).Get", Flat: 1500, FlatPct: 30.0, Cum: 1500, CumPct: 30.0, Module: "sync"},
				{Name: "myapp.newRequest", Flat: 1000, FlatPct: 20.0, Cum: 1000, CumPct: 20.0, Module: "myapp"},
			},
			TopNCount: 3,
		},
		AllocSpace: &model.PProfMemoryStats{
			Total: 10485760, // 10MB total alloc
			Unit:  "bytes",
			TopFuncs: []model.PProfTopFunc{
				{Name: "myapp.processRequest", Flat: 5242880, FlatPct: 50.0, Cum: 7340032, CumPct: 70.0, Module: "myapp"},
				{Name: "runtime.mallocgc", Flat: 3145728, FlatPct: 30.0, Cum: 10485760, CumPct: 100.0, Module: "runtime"},
			},
			TopNCount: 2,
		},
		AllocObjects: nil, // deliberately nil to test nil handling
		HeapSummary: &model.PProfHeapSummary{
			TotalInuseBytes:   1048576,
			TotalInuseObjects: 5000,
			TotalAllocBytes:   10485760,
			TotalAllocObjects: 50000,
		},
	}
}

func TestPProfHeapQueryHelper_NilData(t *testing.T) {
	helper := NewPProfHeapQueryHelper(nil)

	result := helper.QueryAllocHistogram("inuse_space", "flat", 100, "")
	if result == nil {
		t.Fatal("expected non-nil result for nil data")
	}
	if result.Source != "go_pprof" {
		t.Errorf("expected source=go_pprof, got %s", result.Source)
	}
	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries for nil data, got %d", len(result.Entries))
	}

	stats := helper.QueryHeapStats()
	if stats == nil {
		t.Fatal("expected non-nil stats for nil data")
	}
	if stats.TotalHeapSize != 0 {
		t.Errorf("expected TotalHeapSize=0, got %d", stats.TotalHeapSize)
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_InuseSpace(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	result := helper.QueryAllocHistogram("inuse_space", "flat", 100, "")

	if result.Source != "go_pprof" {
		t.Errorf("expected source=go_pprof, got %s", result.Source)
	}
	if result.Metric != "inuse_space" {
		t.Errorf("expected metric=inuse_space, got %s", result.Metric)
	}
	if result.Total != 1048576 {
		t.Errorf("expected total=1048576, got %d", result.Total)
	}
	if result.Unit != "bytes" {
		t.Errorf("expected unit=bytes, got %s", result.Unit)
	}
	if result.EntryCount != 5 {
		t.Errorf("expected entry_count=5, got %d", result.EntryCount)
	}
	if len(result.Entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(result.Entries))
	}

	// Verify sorted by flat descending
	if result.Entries[0].Flat < result.Entries[1].Flat {
		t.Error("expected entries sorted by flat descending")
	}
	if result.Entries[0].Name != "myapp.allocBuffer" {
		t.Errorf("expected first entry=myapp.allocBuffer, got %s", result.Entries[0].Name)
	}
	if result.Entries[0].Module != "myapp" {
		t.Errorf("expected module=myapp, got %s", result.Entries[0].Module)
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_InuseObjects(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	result := helper.QueryAllocHistogram("inuse_objects", "flat", 100, "")

	if result.Metric != "inuse_objects" {
		t.Errorf("expected metric=inuse_objects, got %s", result.Metric)
	}
	if result.Total != 5000 {
		t.Errorf("expected total=5000, got %d", result.Total)
	}
	if result.Unit != "objects" {
		t.Errorf("expected unit=objects, got %s", result.Unit)
	}
	if len(result.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(result.Entries))
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_AllocSpace(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	result := helper.QueryAllocHistogram("alloc_space", "flat", 100, "")

	if result.Metric != "alloc_space" {
		t.Errorf("expected metric=alloc_space, got %s", result.Metric)
	}
	if result.Total != 10485760 {
		t.Errorf("expected total=10485760, got %d", result.Total)
	}
	if len(result.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result.Entries))
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_AllocObjects_Nil(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	// AllocObjects is nil in our test data
	result := helper.QueryAllocHistogram("alloc_objects", "flat", 100, "")

	if result.Metric != "alloc_objects" {
		t.Errorf("expected metric=alloc_objects, got %s", result.Metric)
	}
	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries for nil metric data, got %d", len(result.Entries))
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_DefaultMetric(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	// Empty metric should default to inuse_space
	result := helper.QueryAllocHistogram("", "flat", 100, "")

	if result.Metric != "inuse_space" {
		t.Errorf("expected default metric=inuse_space, got %s", result.Metric)
	}
	if len(result.Entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(result.Entries))
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_SortByCum(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	result := helper.QueryAllocHistogram("inuse_space", "cum", 100, "")

	if len(result.Entries) < 2 {
		t.Fatal("expected at least 2 entries")
	}
	// Verify sorted by cum descending
	if result.Entries[0].Cum < result.Entries[1].Cum {
		t.Error("expected entries sorted by cum descending")
	}
	if result.Entries[0].Name != "myapp.allocBuffer" {
		t.Errorf("expected first entry by cum=myapp.allocBuffer, got %s", result.Entries[0].Name)
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_SortByFlatPct(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	result := helper.QueryAllocHistogram("inuse_space", "flat_pct", 100, "")

	if len(result.Entries) < 2 {
		t.Fatal("expected at least 2 entries")
	}
	if result.Entries[0].FlatPct < result.Entries[1].FlatPct {
		t.Error("expected entries sorted by flat_pct descending")
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_TopN(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	result := helper.QueryAllocHistogram("inuse_space", "flat", 3, "")

	if len(result.Entries) != 3 {
		t.Errorf("expected 3 entries with topN=3, got %d", len(result.Entries))
	}
	// EntryCount should reflect total before limiting
	if result.EntryCount != 5 {
		t.Errorf("expected entry_count=5 (total before limit), got %d", result.EntryCount)
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_TopN_Default(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	// topN=0 should default to 100
	result := helper.QueryAllocHistogram("inuse_space", "flat", 0, "")

	if len(result.Entries) != 5 { // Only 5 entries exist, default limit is 100
		t.Errorf("expected 5 entries (all available), got %d", len(result.Entries))
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_Filter(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	// Filter by "myapp" should return only myapp functions
	result := helper.QueryAllocHistogram("inuse_space", "flat", 100, "myapp")

	if len(result.Entries) != 1 { // Only "myapp.allocBuffer" matches
		t.Errorf("expected 1 entry matching 'myapp', got %d", len(result.Entries))
	}
	if result.Entries[0].Name != "myapp.allocBuffer" {
		t.Errorf("expected entry=myapp.allocBuffer, got %s", result.Entries[0].Name)
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_Filter_CaseInsensitive(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	// Case-insensitive filter
	result := helper.QueryAllocHistogram("inuse_space", "flat", 100, "MYAPP")

	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry matching 'MYAPP' (case-insensitive), got %d", len(result.Entries))
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_Filter_NoMatch(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	result := helper.QueryAllocHistogram("inuse_space", "flat", 100, "nonexistent")

	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries for non-matching filter, got %d", len(result.Entries))
	}
	if result.EntryCount != 0 {
		t.Errorf("expected entry_count=0, got %d", result.EntryCount)
	}
}

func TestPProfHeapQueryHelper_QueryAllocHistogram_Filter_Partial(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	// "json" should match "encoding/json.(*Decoder).Decode"
	result := helper.QueryAllocHistogram("inuse_space", "flat", 100, "json")

	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry matching 'json', got %d", len(result.Entries))
	}
}

func TestPProfHeapQueryHelper_QueryHeapStats(t *testing.T) {
	data := newTestPProfHeapData()
	helper := NewPProfHeapQueryHelper(data)

	stats := helper.QueryHeapStats()

	if stats.TotalHeapSize != 1048576 {
		t.Errorf("expected TotalHeapSize=1048576, got %d", stats.TotalHeapSize)
	}
	if stats.TotalObjects != 5000 {
		t.Errorf("expected TotalObjects=5000, got %d", stats.TotalObjects)
	}
	if stats.TopClassName != "myapp.allocBuffer" {
		t.Errorf("expected TopClassName=myapp.allocBuffer, got %s", stats.TopClassName)
	}
	if stats.TotalClasses != 5 { // TopNCount from InuseSpace
		t.Errorf("expected TotalClasses=5, got %d", stats.TotalClasses)
	}
}

func TestPProfHeapQueryHelper_QueryHeapStats_NoInuseSpace(t *testing.T) {
	data := &model.PProfHeapData{
		InuseSpace:  nil,
		HeapSummary: &model.PProfHeapSummary{TotalInuseBytes: 0, TotalInuseObjects: 0},
	}
	helper := NewPProfHeapQueryHelper(data)

	stats := helper.QueryHeapStats()
	if stats.TotalHeapSize != 0 {
		t.Errorf("expected TotalHeapSize=0, got %d", stats.TotalHeapSize)
	}
	if stats.TopClassName != "" {
		t.Errorf("expected empty TopClassName, got %s", stats.TopClassName)
	}
}

func TestPProfHeapQueryHelper_QueryHeapStats_NoSummary(t *testing.T) {
	data := &model.PProfHeapData{
		InuseSpace: &model.PProfMemoryStats{
			Total:     500,
			Unit:      "bytes",
			TopFuncs:  []model.PProfTopFunc{{Name: "main.alloc", Flat: 500, FlatPct: 100}},
			TopNCount: 1,
		},
		HeapSummary: nil,
	}
	helper := NewPProfHeapQueryHelper(data)

	stats := helper.QueryHeapStats()
	// Without HeapSummary, heap size/objects come from summary which is nil
	if stats.TotalHeapSize != 0 {
		t.Errorf("expected TotalHeapSize=0 with nil summary, got %d", stats.TotalHeapSize)
	}
	// But TopClassName should still be set from InuseSpace
	if stats.TopClassName != "main.alloc" {
		t.Errorf("expected TopClassName=main.alloc, got %s", stats.TopClassName)
	}
}
