package pprof

import (
	"testing"
	"time"
)

func TestLeakDetector_ProfileCount(t *testing.T) {
	d := NewLeakDetector()
	if d.ProfileCount() != 0 {
		t.Errorf("ProfileCount() = %d, want 0", d.ProfileCount())
	}
}

func TestLeakDetector_DetectHeapLeak_InsufficientProfiles(t *testing.T) {
	d := NewLeakDetector()
	_, err := d.DetectHeapLeak()
	if err == nil {
		t.Error("DetectHeapLeak() with no profiles should return error")
	}
}

func TestLeakDetector_DetectGoroutineLeak_InsufficientProfiles(t *testing.T) {
	d := NewLeakDetector()
	_, err := d.DetectGoroutineLeak()
	if err == nil {
		t.Error("DetectGoroutineLeak() with no profiles should return error")
	}
}

func TestLeakDetector_GetTrend_NoProfiles(t *testing.T) {
	d := NewLeakDetector()
	_, _, err := d.GetTrend(SampleTypeInuseSpace)
	if err == nil {
		t.Error("GetTrend() with no profiles should return error")
	}
}

func TestAggregateByFunction(t *testing.T) {
	collapsed := map[string]int64{
		"main;foo;bar": 100,
		"main;foo;baz": 200,
		"main;qux;bar": 150,
	}

	result := aggregateByFunction(collapsed)

	if result["bar"] != 250 { // 100 + 150
		t.Errorf("aggregateByFunction()[bar] = %d, want 250", result["bar"])
	}
	if result["baz"] != 200 {
		t.Errorf("aggregateByFunction()[baz] = %d, want 200", result["baz"])
	}
}

func TestLeakDetector_AnalyzeSeverity_Heap(t *testing.T) {
	d := NewLeakDetector()

	tests := []struct {
		name           string
		totalGrowth    int64
		totalGrowthPct float64
		durationSec    float64
		wantSeverity   string
	}{
		{"no leak", 100, 1.0, 60.0, "none"},
		{"low leak", 5 * 1024 * 1024, 15.0, 60.0, "low"},
		{"medium leak", 30 * 1024 * 1024, 30.0, 60.0, "medium"},
		{"high leak", 80 * 1024 * 1024, 80.0, 60.0, "high"},
		{"critical leak", 200 * 1024 * 1024, 200.0, 60.0, "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			severity, _ := d.analyzeSeverity(LeakTypeHeap, tt.totalGrowth, tt.totalGrowthPct, tt.durationSec, nil)
			if severity != tt.wantSeverity {
				t.Errorf("analyzeSeverity() severity = %s, want %s", severity, tt.wantSeverity)
			}
		})
	}
}

func TestLeakDetector_AnalyzeSeverity_Goroutine(t *testing.T) {
	d := NewLeakDetector()

	// Note: severity is determined by BOTH totalGrowthPct AND growthPerMin
	// growthPerMin = totalGrowth / (durationSec / 60)
	// For "none": totalGrowthPct <= 5 AND growthPerMin < 10
	// For "low": totalGrowthPct <= 20 AND growthPerMin < 50
	// For "medium": totalGrowthPct <= 50 AND growthPerMin < 100
	// For "high": totalGrowthPct <= 100 AND growthPerMin < 500
	tests := []struct {
		name           string
		totalGrowth    int64
		totalGrowthPct float64
		durationSec    float64
		wantSeverity   string
	}{
		{"no leak", 5, 2.0, 60.0, "none"},                 // 5/min, 2% -> none
		{"low leak", 30, 10.0, 60.0, "low"},               // 30/min, 10% -> low (both < thresholds)
		{"medium leak", 60, 30.0, 60.0, "medium"},         // 60/min, 30% -> medium
		{"high leak", 200, 60.0, 60.0, "high"},            // 200/min, 60% -> high
		{"critical leak", 600, 200.0, 60.0, "critical"},   // 600/min, 200% -> critical
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			severity, _ := d.analyzeSeverity(LeakTypeGoroutine, tt.totalGrowth, tt.totalGrowthPct, tt.durationSec, nil)
			if severity != tt.wantSeverity {
				t.Errorf("analyzeSeverity() severity = %s, want %s", severity, tt.wantSeverity)
			}
		})
	}
}

func TestLeakDetector_GenerateRecommendations_None(t *testing.T) {
	d := NewLeakDetector()
	recommendations := d.generateRecommendations(LeakTypeHeap, "none", nil)
	if recommendations != nil {
		t.Error("generateRecommendations() with severity 'none' should return nil")
	}
}

func TestLeakDetector_GenerateRecommendations_Heap(t *testing.T) {
	d := NewLeakDetector()
	growthItems := []GrowthItem{
		{Name: "test.Func", GrowthValue: 1000},
	}
	recommendations := d.generateRecommendations(LeakTypeHeap, "high", growthItems)
	if len(recommendations) == 0 {
		t.Error("generateRecommendations() for heap leak should return recommendations")
	}
}

func TestLeakDetector_GenerateRecommendations_Goroutine(t *testing.T) {
	d := NewLeakDetector()
	growthItems := []GrowthItem{
		{Name: "test.Func", GrowthValue: 100},
	}
	recommendations := d.generateRecommendations(LeakTypeGoroutine, "high", growthItems)
	if len(recommendations) == 0 {
		t.Error("generateRecommendations() for goroutine leak should return recommendations")
	}
}

func TestLeakDetector_CompareProfiles(t *testing.T) {
	d := NewLeakDetector()

	baseline := map[string]int64{
		"main;foo": 100,
		"main;bar": 200,
	}
	current := map[string]int64{
		"main;foo": 150, // +50
		"main;bar": 200, // no change
		"main;baz": 100, // new
	}

	baselineTime := time.Now().Add(-time.Minute)
	currentTime := time.Now()

	report, err := d.compareProfiles(LeakTypeHeap, baseline, current, baselineTime, currentTime)
	if err != nil {
		t.Fatalf("compareProfiles() error = %v", err)
	}

	if report.Type != LeakTypeHeap {
		t.Errorf("report.Type = %s, want heap", report.Type)
	}
	if report.BaselineTotal != 300 {
		t.Errorf("report.BaselineTotal = %d, want 300", report.BaselineTotal)
	}
	if report.CurrentTotal != 450 {
		t.Errorf("report.CurrentTotal = %d, want 450", report.CurrentTotal)
	}
	if report.TotalGrowth != 150 {
		t.Errorf("report.TotalGrowth = %d, want 150", report.TotalGrowth)
	}

	// Check growth items
	if len(report.GrowthItems) != 2 { // foo (+50) and baz (+100)
		t.Errorf("len(report.GrowthItems) = %d, want 2", len(report.GrowthItems))
	}
}

func TestGrowthItem_Fields(t *testing.T) {
	item := GrowthItem{
		Name:          "test.Func",
		BaselineValue: 100,
		CurrentValue:  200,
		GrowthValue:   100,
		GrowthPercent: 100.0,
		GrowthRate:    10.0,
		Module:        "test",
		SourceFile:    "test.go",
		SourceLine:    42,
		SampleStack:   "main;test.Func",
	}

	if item.Name != "test.Func" {
		t.Errorf("item.Name = %s, want test.Func", item.Name)
	}
	if item.GrowthValue != 100 {
		t.Errorf("item.GrowthValue = %d, want 100", item.GrowthValue)
	}
}

func TestLeakReport_Fields(t *testing.T) {
	report := LeakReport{
		Type:            LeakTypeHeap,
		BaselineTime:    time.Now().Add(-time.Minute),
		CurrentTime:     time.Now(),
		DurationSeconds: 60.0,
		BaselineTotal:   1000,
		CurrentTotal:    2000,
		TotalGrowth:     1000,
		TotalGrowthPct:  100.0,
		GrowthItems:     []GrowthItem{},
		Conclusion:      "Test conclusion",
		Severity:        "high",
		Recommendations: []string{"Test recommendation"},
	}

	if report.Type != LeakTypeHeap {
		t.Errorf("report.Type = %s, want heap", report.Type)
	}
	if report.Severity != "high" {
		t.Errorf("report.Severity = %s, want high", report.Severity)
	}
}
