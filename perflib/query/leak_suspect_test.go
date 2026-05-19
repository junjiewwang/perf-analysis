package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ============================================================================
// LeakSuspectEngine Tests
// ============================================================================

func TestLeakSuspectEngine_NoProviders(t *testing.T) {
	engine := NewLeakSuspectEngine()
	result := engine.Detect("/nonexistent")

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TotalCount != 0 {
		t.Errorf("expected 0 suspects, got %d", result.TotalCount)
	}
	if result.Suspects == nil {
		t.Error("expected non-nil suspects slice")
	}
}

func TestLeakSuspectEngine_ProviderNotApplicable(t *testing.T) {
	// TimeSeriesProvider requires batch_analysis.json, which won't exist in temp dir
	engine := NewLeakSuspectEngine(NewTimeSeriesLeakProvider())
	tmpDir := t.TempDir()

	result := engine.Detect(tmpDir)
	if result.TotalCount != 0 {
		t.Errorf("expected 0 suspects when provider not applicable, got %d", result.TotalCount)
	}
}

func TestLeakSuspectEngine_SortsBySeverity(t *testing.T) {
	engine := NewLeakSuspectEngine(&mockProvider{
		suspects: []LeakSuspect{
			{Title: "info", Severity: LeakSeverityInfo},
			{Title: "critical", Severity: LeakSeverityCritical},
			{Title: "warning", Severity: LeakSeverityWarning},
		},
	})

	result := engine.Detect(t.TempDir())
	if result.TotalCount != 3 {
		t.Fatalf("expected 3 suspects, got %d", result.TotalCount)
	}
	if result.Suspects[0].Title != "critical" {
		t.Errorf("expected critical first, got %s", result.Suspects[0].Title)
	}
	if result.Suspects[1].Title != "warning" {
		t.Errorf("expected warning second, got %s", result.Suspects[1].Title)
	}
	if result.Suspects[2].Title != "info" {
		t.Errorf("expected info last, got %s", result.Suspects[2].Title)
	}
}

// ============================================================================
// FilterByType and FilterBySeverity Tests
// ============================================================================

func TestLeakSuspectsResult_FilterByType(t *testing.T) {
	result := &LeakSuspectsResult{
		TotalCount: 3,
		Suspects: []LeakSuspect{
			{Type: "heap", Title: "heap1"},
			{Type: "goroutine", Title: "goroutine1"},
			{Type: "heap", Title: "heap2"},
		},
	}

	filtered := result.FilterByType("heap")
	if filtered.TotalCount != 2 {
		t.Errorf("expected 2 heap suspects, got %d", filtered.TotalCount)
	}

	filtered = result.FilterByType("goroutine")
	if filtered.TotalCount != 1 {
		t.Errorf("expected 1 goroutine suspect, got %d", filtered.TotalCount)
	}

	filtered = result.FilterByType("all")
	if filtered.TotalCount != 3 {
		t.Errorf("expected 3 suspects for 'all', got %d", filtered.TotalCount)
	}

	filtered = result.FilterByType("")
	if filtered.TotalCount != 3 {
		t.Errorf("expected 3 suspects for empty type, got %d", filtered.TotalCount)
	}
}

func TestLeakSuspectsResult_FilterBySeverity(t *testing.T) {
	result := &LeakSuspectsResult{
		TotalCount: 3,
		Suspects: []LeakSuspect{
			{Severity: LeakSeverityCritical, Title: "crit"},
			{Severity: LeakSeverityWarning, Title: "warn"},
			{Severity: LeakSeverityInfo, Title: "info"},
		},
	}

	filtered := result.FilterBySeverity(LeakSeverityCritical)
	if filtered.TotalCount != 1 {
		t.Errorf("expected 1 critical suspect, got %d", filtered.TotalCount)
	}

	filtered = result.FilterBySeverity(LeakSeverityWarning)
	if filtered.TotalCount != 2 {
		t.Errorf("expected 2 suspects at warning+, got %d", filtered.TotalCount)
	}

	filtered = result.FilterBySeverity(LeakSeverityInfo)
	if filtered.TotalCount != 3 {
		t.Errorf("expected 3 suspects at info+, got %d", filtered.TotalCount)
	}
}

// ============================================================================
// TimeSeriesLeakProvider Tests
// ============================================================================

func TestTimeSeriesLeakProvider_CanDetect(t *testing.T) {
	provider := NewTimeSeriesLeakProvider()

	t.Run("no batch file", func(t *testing.T) {
		if provider.CanDetect(t.TempDir()) {
			t.Error("should not detect without batch_analysis.json")
		}
	})

	t.Run("with batch file", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "batch_analysis.json"), []byte(`{}`), 0644)
		if !provider.CanDetect(dir) {
			t.Error("should detect with batch_analysis.json present")
		}
	})
}

func TestTimeSeriesLeakProvider_Detect_WithLeakReports(t *testing.T) {
	dir := t.TempDir()

	batchData := map[string]interface{}{
		"leak_reports": map[string]interface{}{
			"heap": map[string]interface{}{
				"type":                 "heap",
				"baseline_total":       180000000,
				"current_total":        260000000,
				"total_growth":         80000000,
				"total_growth_percent": 44.4,
				"duration_seconds":     1800.0,
				"conclusion":           "Heap memory grew 44.4% in 30 minutes",
				"severity":             "high",
				"recommendations":      []string{"Check CacheEntry lifecycle"},
				"growth_items": []map[string]interface{}{
					{
						"name":           "com.app.cache.CacheEntry",
						"baseline_value": 50000000,
						"current_value":  120000000,
						"growth_value":   70000000,
						"growth_percent": 140.0,
					},
				},
			},
		},
	}

	data, _ := json.Marshal(batchData)
	os.WriteFile(filepath.Join(dir, "batch_analysis.json"), data, 0644)

	provider := NewTimeSeriesLeakProvider()
	suspects, err := provider.Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suspects) != 1 {
		t.Fatalf("expected 1 suspect, got %d", len(suspects))
	}

	s := suspects[0]
	if s.Type != "heap" {
		t.Errorf("expected type 'heap', got %q", s.Type)
	}
	if s.Source != LeakSourceTimeSeries {
		t.Errorf("expected source 'time_series', got %q", s.Source)
	}
	if s.Severity != LeakSeverityCritical {
		t.Errorf("expected severity 'critical' (mapped from 'high'), got %q", s.Severity)
	}
	if s.Metrics == nil {
		t.Fatal("expected non-nil metrics")
	}
	if s.Metrics.GrowthPercent != 44.4 {
		t.Errorf("expected growth 44.4%%, got %.1f%%", s.Metrics.GrowthPercent)
	}
	if len(s.Evidence) != 1 {
		t.Errorf("expected 1 evidence item, got %d", len(s.Evidence))
	}
	if len(s.Suggestions) != 1 {
		t.Errorf("expected 1 suggestion, got %d", len(s.Suggestions))
	}
}

func TestTimeSeriesLeakProvider_Detect_EmptyLeakReports(t *testing.T) {
	dir := t.TempDir()

	batchData := map[string]interface{}{
		"leak_reports": map[string]interface{}{},
	}
	data, _ := json.Marshal(batchData)
	os.WriteFile(filepath.Join(dir, "batch_analysis.json"), data, 0644)

	provider := NewTimeSeriesLeakProvider()
	suspects, err := provider.Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suspects) != 0 {
		t.Errorf("expected 0 suspects, got %d", len(suspects))
	}
}

// ============================================================================
// HprofSnapshotLeakProvider Tests
// ============================================================================

func TestHprofSnapshotLeakProvider_CanDetect(t *testing.T) {
	provider := NewHprofSnapshotLeakProvider()

	t.Run("no files", func(t *testing.T) {
		if provider.CanDetect(t.TempDir()) {
			t.Error("should not detect without data files")
		}
	})

	t.Run("with class_stats", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "class_stats.json"), []byte(`{}`), 0644)
		if !provider.CanDetect(dir) {
			t.Error("should detect with class_stats.json")
		}
	})

	t.Run("with heap_stats only", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "heap_stats.json"), []byte(`{}`), 0644)
		if !provider.CanDetect(dir) {
			t.Error("should detect with heap_stats.json")
		}
	})
}

func TestHprofSnapshotLeakProvider_DominantClassRule(t *testing.T) {
	dir := t.TempDir()

	classStats := snapshotClassStats{
		TotalClasses: 100,
		TotalObjects: 500000,
		TotalSize:    256 * 1024 * 1024,
		Classes: []snapshotClassEntry{
			{ClassName: "com.app.model.Order", ObjectCount: 200000, RetainedSize: 100 * 1024 * 1024, Percentage: 40.0},
			{ClassName: "java.lang.String", ObjectCount: 150000, RetainedSize: 50 * 1024 * 1024, Percentage: 20.0},
		},
	}
	data, _ := json.Marshal(classStats)
	os.WriteFile(filepath.Join(dir, "class_stats.json"), data, 0644)

	provider := NewHprofSnapshotLeakProvider()
	suspects, err := provider.Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// com.app.model.Order at 40% should trigger critical
	found := false
	for _, s := range suspects {
		if s.Type == "heap" && s.Severity == LeakSeverityCritical {
			found = true
			if len(s.Evidence) == 0 {
				t.Error("expected evidence for dominant class")
			}
			if len(s.Suggestions) == 0 {
				t.Error("expected suggestions for dominant class")
			}
		}
	}
	if !found {
		t.Error("expected critical suspect for 40% dominant class")
	}
}

func TestHprofSnapshotLeakProvider_DominantClassRule_SkipsPrimitiveArrays(t *testing.T) {
	dir := t.TempDir()

	classStats := snapshotClassStats{
		TotalClasses: 100,
		TotalObjects: 500000,
		TotalSize:    256 * 1024 * 1024,
		Classes: []snapshotClassEntry{
			{ClassName: "byte[]", ObjectCount: 300000, RetainedSize: 120 * 1024 * 1024, Percentage: 45.0},
			{ClassName: "java.lang.String", ObjectCount: 100000, RetainedSize: 30 * 1024 * 1024, Percentage: 12.0},
		},
	}
	data, _ := json.Marshal(classStats)
	os.WriteFile(filepath.Join(dir, "class_stats.json"), data, 0644)

	provider := NewHprofSnapshotLeakProvider()
	suspects, err := provider.Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// byte[] should be skipped even though it's 45%
	for _, s := range suspects {
		if s.Type == "heap" && s.Severity == LeakSeverityCritical {
			t.Errorf("should not flag byte[] as dominant class suspect, got: %s", s.Title)
		}
	}
}

func TestHprofSnapshotLeakProvider_CollectionAccumulationRule(t *testing.T) {
	dir := t.TempDir()

	classStats := snapshotClassStats{
		TotalClasses: 100,
		TotalObjects: 1000000,
		TotalSize:    512 * 1024 * 1024,
		Classes: []snapshotClassEntry{
			{ClassName: "java.util.HashMap$Node", ObjectCount: 600000, RetainedSize: 100 * 1024 * 1024, Percentage: 19.5},
			{ClassName: "java.util.ArrayList", ObjectCount: 120000, RetainedSize: 30 * 1024 * 1024, Percentage: 5.9},
			{ClassName: "com.app.model.Order", ObjectCount: 10000, RetainedSize: 20 * 1024 * 1024, Percentage: 3.9},
		},
	}
	data, _ := json.Marshal(classStats)
	os.WriteFile(filepath.Join(dir, "class_stats.json"), data, 0644)

	provider := NewHprofSnapshotLeakProvider()
	suspects, err := provider.Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// HashMap$Node at 600K instances should be critical (>=500K)
	// ArrayList at 120K should be warning (>=100K)
	criticalFound := false
	warningFound := false
	for _, s := range suspects {
		if s.Type == "class_accumulation" {
			if s.Severity == LeakSeverityCritical {
				criticalFound = true
			} else if s.Severity == LeakSeverityWarning {
				warningFound = true
			}
		}
	}
	if !criticalFound {
		t.Error("expected critical for HashMap$Node with 600K instances")
	}
	if !warningFound {
		t.Error("expected warning for ArrayList with 120K instances")
	}
}

func TestHprofSnapshotLeakProvider_ClassLoaderLeakRule(t *testing.T) {
	dir := t.TempDir()

	heapStats := snapshotHeapStats{
		TotalClasses: 85000,
		TotalObjects: 2000000,
	}
	data, _ := json.Marshal(heapStats)
	os.WriteFile(filepath.Join(dir, "heap_stats.json"), data, 0644)

	provider := NewHprofSnapshotLeakProvider()
	suspects, err := provider.Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, s := range suspects {
		if s.Type == "classloader_leak" && s.Severity == LeakSeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected critical classloader leak suspect for 85K classes")
	}
}

func TestHprofSnapshotLeakProvider_ClassLoaderLeakRule_Normal(t *testing.T) {
	dir := t.TempDir()

	heapStats := snapshotHeapStats{
		TotalClasses: 15000,
		TotalObjects: 500000,
	}
	data, _ := json.Marshal(heapStats)
	os.WriteFile(filepath.Join(dir, "heap_stats.json"), data, 0644)

	provider := NewHprofSnapshotLeakProvider()
	suspects, err := provider.Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, s := range suspects {
		if s.Type == "classloader_leak" {
			t.Errorf("should not flag classloader leak for normal class count (15K), got: %s", s.Title)
		}
	}
}

func TestHprofSnapshotLeakProvider_NoSuspectsForNormalHeap(t *testing.T) {
	dir := t.TempDir()

	classStats := snapshotClassStats{
		TotalClasses: 200,
		TotalObjects: 50000,
		TotalSize:    64 * 1024 * 1024,
		Classes: []snapshotClassEntry{
			{ClassName: "java.lang.String", ObjectCount: 10000, RetainedSize: 10 * 1024 * 1024, Percentage: 15.0},
			{ClassName: "com.app.model.Order", ObjectCount: 5000, RetainedSize: 5 * 1024 * 1024, Percentage: 7.8},
		},
	}
	data, _ := json.Marshal(classStats)
	os.WriteFile(filepath.Join(dir, "class_stats.json"), data, 0644)

	heapStats := snapshotHeapStats{TotalClasses: 200, TotalObjects: 50000}
	hsData, _ := json.Marshal(heapStats)
	os.WriteFile(filepath.Join(dir, "heap_stats.json"), hsData, 0644)

	provider := NewHprofSnapshotLeakProvider()
	suspects, err := provider.Detect(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suspects) != 0 {
		t.Errorf("expected no suspects for normal heap, got %d: %+v", len(suspects), suspects)
	}
}

// ============================================================================
// Integration: Engine with multiple providers
// ============================================================================

func TestLeakSuspectEngine_Integration(t *testing.T) {
	dir := t.TempDir()

	// Write batch_analysis.json (time-series)
	batchData := map[string]interface{}{
		"leak_reports": map[string]interface{}{
			"goroutine": map[string]interface{}{
				"type":                 "goroutine",
				"baseline_total":       100,
				"current_total":        500,
				"total_growth":         400,
				"total_growth_percent": 400.0,
				"duration_seconds":     600.0,
				"conclusion":           "Goroutine count grew 400%",
				"severity":             "critical",
				"growth_items":         []interface{}{},
			},
		},
	}
	batchJSON, _ := json.Marshal(batchData)
	os.WriteFile(filepath.Join(dir, "batch_analysis.json"), batchJSON, 0644)

	// Write class_stats.json (hprof snapshot)
	classStats := snapshotClassStats{
		TotalClasses: 100,
		TotalObjects: 500000,
		TotalSize:    256 * 1024 * 1024,
		Classes: []snapshotClassEntry{
			{ClassName: "com.app.cache.BigCache", ObjectCount: 200000, RetainedSize: 100 * 1024 * 1024, Percentage: 39.0},
		},
	}
	classJSON, _ := json.Marshal(classStats)
	os.WriteFile(filepath.Join(dir, "class_stats.json"), classJSON, 0644)

	// Run engine with both providers
	engine := NewLeakSuspectEngine(
		NewTimeSeriesLeakProvider(),
		NewHprofSnapshotLeakProvider(),
	)

	result := engine.Detect(dir)

	// Should have suspects from both providers
	if result.TotalCount < 2 {
		t.Errorf("expected at least 2 suspects from 2 providers, got %d", result.TotalCount)
	}

	// First suspect should be critical (sorted by severity)
	if result.Suspects[0].Severity != LeakSeverityCritical {
		t.Errorf("expected first suspect to be critical, got %q", result.Suspects[0].Severity)
	}

	// Verify both sources are represented
	sources := map[LeakSource]bool{}
	for _, s := range result.Suspects {
		sources[s.Source] = true
	}
	if !sources[LeakSourceTimeSeries] {
		t.Error("expected time_series source in results")
	}
	if !sources[LeakSourceSnapshotHeuristic] {
		t.Error("expected snapshot_heuristic source in results")
	}
}

// ============================================================================
// Helper: mock provider for testing
// ============================================================================

type mockProvider struct {
	suspects []LeakSuspect
}

func (m *mockProvider) Name() string              { return "mock" }
func (m *mockProvider) CanDetect(string) bool     { return true }
func (m *mockProvider) Detect(string) ([]LeakSuspect, error) {
	return m.suspects, nil
}
