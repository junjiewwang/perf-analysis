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

func TestLeakDetector_AddParsedProfile(t *testing.T) {
	d := NewLeakDetector()
	p := NewParser()

	d.AddParsedProfile(p, time.Now())
	if d.ProfileCount() != 1 {
		t.Errorf("ProfileCount() after AddParsedProfile = %d, want 1", d.ProfileCount())
	}
}

func TestLeakType_Constants(t *testing.T) {
	if LeakTypeHeap != "heap" {
		t.Errorf("LeakTypeHeap = %s, want heap", LeakTypeHeap)
	}
	if LeakTypeGoroutine != "goroutine" {
		t.Errorf("LeakTypeGoroutine = %s, want goroutine", LeakTypeGoroutine)
	}
}
