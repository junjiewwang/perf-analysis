// Package query provides reusable query utilities for analysis data.
// This file implements the TimeSeriesLeakProvider which detects leaks by reading
// existing batch_analysis.json results (produced by PProfBatchAnalyzer).
// It adapts the legacy LeakReport format to the unified LeakSuspect model.
package query

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/junjiewwang/perf-analysis/perflib/output"
)

// TimeSeriesLeakProvider detects leaks from batch analysis results.
// It reads the existing batch_analysis.json and converts LeakReports to unified LeakSuspects.
// This provider requires >= 2 profile snapshots of the same type to detect growth trends.
type TimeSeriesLeakProvider struct{}

// NewTimeSeriesLeakProvider creates a new TimeSeriesLeakProvider.
func NewTimeSeriesLeakProvider() *TimeSeriesLeakProvider {
	return &TimeSeriesLeakProvider{}
}

// Name returns the provider identifier.
func (p *TimeSeriesLeakProvider) Name() string {
	return "time_series"
}

// CanDetect checks if batch_analysis.json exists in the task directory.
func (p *TimeSeriesLeakProvider) CanDetect(taskDir string) bool {
	batchFile := filepath.Join(taskDir, output.FileBatchAnalysis)
	_, err := os.Stat(batchFile)
	return err == nil
}

// Detect reads batch_analysis.json and converts leak reports to unified suspects.
func (p *TimeSeriesLeakProvider) Detect(taskDir string) ([]LeakSuspect, error) {
	batchFile := filepath.Join(taskDir, output.FileBatchAnalysis)
	data, err := os.ReadFile(batchFile)
	if err != nil {
		return nil, fmt.Errorf("read batch analysis: %w", err)
	}

	var batchResult batchAnalysisData
	if err := json.Unmarshal(data, &batchResult); err != nil {
		return nil, fmt.Errorf("parse batch analysis: %w", err)
	}

	var suspects []LeakSuspect
	for leakType, report := range batchResult.LeakReports {
		if report == nil {
			continue
		}
		suspect := p.convertReport(leakType, report)
		suspects = append(suspects, suspect)
	}

	if suspects == nil {
		suspects = []LeakSuspect{}
	}
	return suspects, nil
}

// convertReport adapts a single legacy LeakReport to the unified LeakSuspect model.
func (p *TimeSeriesLeakProvider) convertReport(leakType string, report *timeSeriesLeakReport) LeakSuspect {
	suspect := LeakSuspect{
		Type:   leakType,
		Source: LeakSourceTimeSeries,
		Severity: mapSeverity(report.Severity),
		Title:  report.Conclusion,
		Metrics: &LeakMetrics{
			BaselineValue: report.BaselineTotal,
			CurrentValue:  report.CurrentTotal,
			GrowthValue:   report.TotalGrowth,
			GrowthPercent: report.TotalGrowthPct,
		},
		Suggestions: report.Recommendations,
	}

	// Build description from growth data
	if report.TotalGrowthPct > 0 {
		suspect.Description = fmt.Sprintf(
			"%s grew from %d to %d (+%.1f%%) over %.0f seconds",
			leakType, report.BaselineTotal, report.CurrentTotal,
			report.TotalGrowthPct, report.DurationSeconds,
		)
	} else {
		suspect.Description = report.Conclusion
	}

	// Convert top growth items to evidence
	maxEvidence := 5
	if len(report.GrowthItems) < maxEvidence {
		maxEvidence = len(report.GrowthItems)
	}
	for i := 0; i < maxEvidence; i++ {
		item := report.GrowthItems[i]
		unit := "bytes"
		if leakType == "goroutine" {
			unit = "count"
		}
		suspect.Evidence = append(suspect.Evidence, LeakEvidence{
			Name:   item.Name,
			Value:  item.GrowthValue,
			Unit:   unit,
			Detail: fmt.Sprintf("grew %.1f%% (from %d to %d)", item.GrowthPercent, item.BaselineValue, item.CurrentValue),
		})
	}

	return suspect
}

// mapSeverity converts legacy severity strings to the unified LeakSeverity type.
func mapSeverity(severity string) LeakSeverity {
	switch severity {
	case "critical", "high":
		return LeakSeverityCritical
	case "medium", "low":
		return LeakSeverityWarning
	default:
		return LeakSeverityInfo
	}
}

// ============================================================================
// Internal data structures for parsing batch_analysis.json
// These are local to this file to avoid coupling with the analyzer package.
// ============================================================================

// batchAnalysisData represents the subset of batch_analysis.json we need.
type batchAnalysisData struct {
	LeakReports map[string]*timeSeriesLeakReport `json:"leak_reports"`
}

// timeSeriesLeakReport is the on-disk format of a leak report in batch_analysis.json.
type timeSeriesLeakReport struct {
	Type            string                `json:"type"`
	BaselineTotal   int64                 `json:"baseline_total"`
	CurrentTotal    int64                 `json:"current_total"`
	TotalGrowth     int64                 `json:"total_growth"`
	TotalGrowthPct  float64               `json:"total_growth_percent"`
	DurationSeconds float64               `json:"duration_seconds"`
	GrowthItems     []timeSeriesGrowthItem `json:"growth_items"`
	Conclusion      string                `json:"conclusion"`
	Severity        string                `json:"severity"`
	Recommendations []string              `json:"recommendations,omitempty"`
}

// timeSeriesGrowthItem is the on-disk format of a growth item.
type timeSeriesGrowthItem struct {
	Name          string  `json:"name"`
	BaselineValue int64   `json:"baseline_value"`
	CurrentValue  int64   `json:"current_value"`
	GrowthValue   int64   `json:"growth_value"`
	GrowthPercent float64 `json:"growth_percent"`
}
