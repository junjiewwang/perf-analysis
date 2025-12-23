// Package pprof provides parsing functionality for Go pprof profile data.
package pprof

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// LeakType represents the type of leak being detected.
type LeakType string

const (
	// LeakTypeHeap represents heap memory leak.
	LeakTypeHeap LeakType = "heap"
	// LeakTypeGoroutine represents goroutine leak.
	LeakTypeGoroutine LeakType = "goroutine"
)

// GrowthItem represents an item that has grown between two profile snapshots.
type GrowthItem struct {
	Name           string  `json:"name"`
	BaselineValue  int64   `json:"baseline_value"`
	CurrentValue   int64   `json:"current_value"`
	GrowthValue    int64   `json:"growth_value"`
	GrowthPercent  float64 `json:"growth_percent"`
	GrowthRate     float64 `json:"growth_rate_per_sec,omitempty"` // Growth rate per second
	Module         string  `json:"module,omitempty"`
	SourceFile     string  `json:"source_file,omitempty"`
	SourceLine     int     `json:"source_line,omitempty"`
	SampleStack    string  `json:"sample_stack,omitempty"` // Representative call stack
}

// LeakReport represents a leak analysis report.
type LeakReport struct {
	Type             LeakType     `json:"type"`
	BaselineTime     time.Time    `json:"baseline_time"`
	CurrentTime      time.Time    `json:"current_time"`
	DurationSeconds  float64      `json:"duration_seconds"`
	BaselineTotal    int64        `json:"baseline_total"`
	CurrentTotal     int64        `json:"current_total"`
	TotalGrowth      int64        `json:"total_growth"`
	TotalGrowthPct   float64      `json:"total_growth_percent"`
	GrowthItems      []GrowthItem `json:"growth_items"`
	Conclusion       string       `json:"conclusion"`
	Severity         string       `json:"severity"` // "none", "low", "medium", "high", "critical"
	Recommendations  []string     `json:"recommendations,omitempty"`
}

// LeakDetector detects memory and goroutine leaks by comparing multiple profile snapshots.
type LeakDetector struct {
	profiles   []*Parser
	timestamps []time.Time
}

// NewLeakDetector creates a new LeakDetector.
func NewLeakDetector() *LeakDetector {
	return &LeakDetector{
		profiles:   make([]*Parser, 0),
		timestamps: make([]time.Time, 0),
	}
}

// AddProfile adds a profile to the detector.
// Profiles should be added in chronological order.
func (d *LeakDetector) AddProfile(r io.Reader, timestamp time.Time) error {
	parser := NewParser()
	if err := parser.Parse(r); err != nil {
		return fmt.Errorf("failed to parse profile: %w", err)
	}
	d.profiles = append(d.profiles, parser)
	d.timestamps = append(d.timestamps, timestamp)
	return nil
}

// AddParsedProfile adds an already parsed profile to the detector.
func (d *LeakDetector) AddParsedProfile(parser *Parser, timestamp time.Time) {
	d.profiles = append(d.profiles, parser)
	d.timestamps = append(d.timestamps, timestamp)
}

// ProfileCount returns the number of profiles added.
func (d *LeakDetector) ProfileCount() int {
	return len(d.profiles)
}

// DetectHeapLeak detects heap memory leaks by comparing profiles.
func (d *LeakDetector) DetectHeapLeak() (*LeakReport, error) {
	if len(d.profiles) < 2 {
		return nil, fmt.Errorf("at least 2 profiles required for leak detection, got %d", len(d.profiles))
	}

	baseline := d.profiles[0]
	current := d.profiles[len(d.profiles)-1]
	baselineTime := d.timestamps[0]
	currentTime := d.timestamps[len(d.timestamps)-1]

	// Get baseline and current data for inuse_space
	baselineCollapsed, err := baseline.ToCollapsed(SampleTypeInuseSpace)
	if err != nil {
		return nil, fmt.Errorf("failed to get baseline heap data: %w", err)
	}

	currentCollapsed, err := current.ToCollapsed(SampleTypeInuseSpace)
	if err != nil {
		return nil, fmt.Errorf("failed to get current heap data: %w", err)
	}

	return d.compareProfiles(LeakTypeHeap, baselineCollapsed, currentCollapsed, baselineTime, currentTime)
}

// DetectGoroutineLeak detects goroutine leaks by comparing profiles.
func (d *LeakDetector) DetectGoroutineLeak() (*LeakReport, error) {
	if len(d.profiles) < 2 {
		return nil, fmt.Errorf("at least 2 profiles required for leak detection, got %d", len(d.profiles))
	}

	baseline := d.profiles[0]
	current := d.profiles[len(d.profiles)-1]
	baselineTime := d.timestamps[0]
	currentTime := d.timestamps[len(d.timestamps)-1]

	// Get baseline and current data
	baselineCollapsed, err := baseline.ToCollapsed(SampleTypeGoroutine)
	if err != nil {
		// Try alternative sample type
		baselineCollapsed, err = baseline.ToCollapsed(SampleTypeSamples)
		if err != nil {
			return nil, fmt.Errorf("failed to get baseline goroutine data: %w", err)
		}
	}

	currentCollapsed, err := current.ToCollapsed(SampleTypeGoroutine)
	if err != nil {
		currentCollapsed, err = current.ToCollapsed(SampleTypeSamples)
		if err != nil {
			return nil, fmt.Errorf("failed to get current goroutine data: %w", err)
		}
	}

	return d.compareProfiles(LeakTypeGoroutine, baselineCollapsed, currentCollapsed, baselineTime, currentTime)
}

// compareProfiles compares two profile snapshots and generates a leak report.
func (d *LeakDetector) compareProfiles(
	leakType LeakType,
	baseline, current map[string]int64,
	baselineTime, currentTime time.Time,
) (*LeakReport, error) {
	duration := currentTime.Sub(baselineTime).Seconds()
	if duration <= 0 {
		duration = 1 // Avoid division by zero
	}

	// Calculate totals
	var baselineTotal, currentTotal int64
	for _, v := range baseline {
		baselineTotal += v
	}
	for _, v := range current {
		currentTotal += v
	}

	totalGrowth := currentTotal - baselineTotal
	var totalGrowthPct float64
	if baselineTotal > 0 {
		totalGrowthPct = float64(totalGrowth) * 100.0 / float64(baselineTotal)
	}

	// Aggregate by function (extract leaf function from stack)
	baselineByFunc := aggregateByFunction(baseline)
	currentByFunc := aggregateByFunction(current)

	// Find growth items
	growthItems := make([]GrowthItem, 0)

	// Check all functions in current
	allFuncs := make(map[string]bool)
	for funcName := range baselineByFunc {
		allFuncs[funcName] = true
	}
	for funcName := range currentByFunc {
		allFuncs[funcName] = true
	}

	for funcName := range allFuncs {
		baselineVal := baselineByFunc[funcName]
		currentVal := currentByFunc[funcName]
		growth := currentVal - baselineVal

		// Only include items with positive growth
		if growth > 0 {
			var growthPct float64
			if baselineVal > 0 {
				growthPct = float64(growth) * 100.0 / float64(baselineVal)
			} else {
				growthPct = 100.0 // New allocation
			}

			item := GrowthItem{
				Name:          funcName,
				BaselineValue: baselineVal,
				CurrentValue:  currentVal,
				GrowthValue:   growth,
				GrowthPercent: growthPct,
				GrowthRate:    float64(growth) / duration,
				Module:        getModuleName(funcName),
			}

			// Find a sample stack for this function
			for stack := range current {
				if strings.HasSuffix(stack, funcName) || strings.Contains(stack, ";"+funcName) {
					item.SampleStack = stack
					break
				}
			}

			growthItems = append(growthItems, item)
		}
	}

	// Sort by growth value (descending)
	sort.Slice(growthItems, func(i, j int) bool {
		return growthItems[i].GrowthValue > growthItems[j].GrowthValue
	})

	// Limit to top 50 items
	if len(growthItems) > 50 {
		growthItems = growthItems[:50]
	}

	// Determine severity and conclusion
	severity, conclusion := d.analyzeSeverity(leakType, totalGrowth, totalGrowthPct, duration, growthItems)

	// Generate recommendations
	recommendations := d.generateRecommendations(leakType, severity, growthItems)

	return &LeakReport{
		Type:            leakType,
		BaselineTime:    baselineTime,
		CurrentTime:     currentTime,
		DurationSeconds: duration,
		BaselineTotal:   baselineTotal,
		CurrentTotal:    currentTotal,
		TotalGrowth:     totalGrowth,
		TotalGrowthPct:  totalGrowthPct,
		GrowthItems:     growthItems,
		Conclusion:      conclusion,
		Severity:        severity,
		Recommendations: recommendations,
	}, nil
}

// aggregateByFunction aggregates collapsed stack data by leaf function.
func aggregateByFunction(collapsed map[string]int64) map[string]int64 {
	result := make(map[string]int64)
	for stack, value := range collapsed {
		// Get leaf function (last in the stack)
		parts := strings.Split(stack, ";")
		if len(parts) > 0 {
			leafFunc := parts[len(parts)-1]
			result[leafFunc] += value
		}
	}
	return result
}

// analyzeSeverity determines the severity level and conclusion.
func (d *LeakDetector) analyzeSeverity(
	leakType LeakType,
	totalGrowth int64,
	totalGrowthPct float64,
	durationSec float64,
	growthItems []GrowthItem,
) (severity, conclusion string) {
	// Calculate growth rate per minute
	growthPerMin := float64(totalGrowth) / (durationSec / 60.0)

	switch leakType {
	case LeakTypeHeap:
		// Memory leak severity thresholds (in bytes per minute)
		switch {
		case totalGrowthPct <= 5 && growthPerMin < 1024*1024: // < 5% and < 1MB/min
			severity = "none"
			conclusion = "No significant memory leak detected. Memory usage is stable."
		case totalGrowthPct <= 20 && growthPerMin < 10*1024*1024: // < 20% and < 10MB/min
			severity = "low"
			conclusion = fmt.Sprintf("Minor memory growth detected (%.1f%%, %.2f MB/min). Monitor for trends.", totalGrowthPct, growthPerMin/(1024*1024))
		case totalGrowthPct <= 50 && growthPerMin < 50*1024*1024: // < 50% and < 50MB/min
			severity = "medium"
			conclusion = fmt.Sprintf("Moderate memory growth detected (%.1f%%, %.2f MB/min). Investigation recommended.", totalGrowthPct, growthPerMin/(1024*1024))
		case totalGrowthPct <= 100 && growthPerMin < 100*1024*1024: // < 100% and < 100MB/min
			severity = "high"
			conclusion = fmt.Sprintf("Significant memory leak detected (%.1f%%, %.2f MB/min). Immediate investigation required.", totalGrowthPct, growthPerMin/(1024*1024))
		default:
			severity = "critical"
			conclusion = fmt.Sprintf("Critical memory leak detected (%.1f%%, %.2f MB/min). Urgent action required!", totalGrowthPct, growthPerMin/(1024*1024))
		}

	case LeakTypeGoroutine:
		// Goroutine leak severity thresholds (goroutines per minute)
		switch {
		case totalGrowthPct <= 5 && growthPerMin < 10:
			severity = "none"
			conclusion = "No significant goroutine leak detected. Goroutine count is stable."
		case totalGrowthPct <= 20 && growthPerMin < 50:
			severity = "low"
			conclusion = fmt.Sprintf("Minor goroutine growth detected (%.1f%%, %.1f/min). Monitor for trends.", totalGrowthPct, growthPerMin)
		case totalGrowthPct <= 50 && growthPerMin < 100:
			severity = "medium"
			conclusion = fmt.Sprintf("Moderate goroutine growth detected (%.1f%%, %.1f/min). Investigation recommended.", totalGrowthPct, growthPerMin)
		case totalGrowthPct <= 100 && growthPerMin < 500:
			severity = "high"
			conclusion = fmt.Sprintf("Significant goroutine leak detected (%.1f%%, %.1f/min). Immediate investigation required.", totalGrowthPct, growthPerMin)
		default:
			severity = "critical"
			conclusion = fmt.Sprintf("Critical goroutine leak detected (%.1f%%, %.1f/min). Urgent action required!", totalGrowthPct, growthPerMin)
		}
	}

	return severity, conclusion
}

// generateRecommendations generates recommendations based on the leak analysis.
func (d *LeakDetector) generateRecommendations(leakType LeakType, severity string, growthItems []GrowthItem) []string {
	if severity == "none" {
		return nil
	}

	recommendations := make([]string, 0)

	switch leakType {
	case LeakTypeHeap:
		recommendations = append(recommendations, "Review the top growing functions for potential memory leaks")
		if len(growthItems) > 0 {
			recommendations = append(recommendations, fmt.Sprintf("Focus on '%s' which shows the highest memory growth", growthItems[0].Name))
		}
		recommendations = append(recommendations, "Check for unclosed resources (files, connections, etc.)")
		recommendations = append(recommendations, "Verify that slices and maps are properly sized and cleared")
		recommendations = append(recommendations, "Consider using sync.Pool for frequently allocated objects")
		if severity == "high" || severity == "critical" {
			recommendations = append(recommendations, "Run with GODEBUG=gctrace=1 to monitor GC behavior")
			recommendations = append(recommendations, "Consider taking heap dump for detailed analysis")
		}

	case LeakTypeGoroutine:
		recommendations = append(recommendations, "Review the top growing goroutine stacks for potential leaks")
		if len(growthItems) > 0 {
			recommendations = append(recommendations, fmt.Sprintf("Focus on '%s' which shows the highest goroutine growth", growthItems[0].Name))
		}
		recommendations = append(recommendations, "Ensure all goroutines have proper exit conditions")
		recommendations = append(recommendations, "Check for blocked channel operations")
		recommendations = append(recommendations, "Verify context cancellation is properly propagated")
		recommendations = append(recommendations, "Review sync.WaitGroup usage for proper Done() calls")
		if severity == "high" || severity == "critical" {
			recommendations = append(recommendations, "Use runtime.NumGoroutine() to monitor goroutine count")
			recommendations = append(recommendations, "Consider using goleak in tests to detect goroutine leaks")
		}
	}

	return recommendations
}

// GetTrend analyzes the trend across all profiles.
// Returns a slice of (timestamp, total_value) pairs.
func (d *LeakDetector) GetTrend(sampleType SampleType) ([]time.Time, []int64, error) {
	if len(d.profiles) == 0 {
		return nil, nil, fmt.Errorf("no profiles added")
	}

	timestamps := make([]time.Time, len(d.profiles))
	values := make([]int64, len(d.profiles))

	for i, parser := range d.profiles {
		timestamps[i] = d.timestamps[i]
		values[i] = parser.GetTotalSamples(sampleType)
	}

	return timestamps, values, nil
}
