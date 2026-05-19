// Package query provides reusable query utilities for analysis data.
// This file defines the unified LeakSuspect model and LeakSuspectProvider interface.
// It follows the Strategy Pattern (like GoroutineRule) to make leak detection
// independently testable and extensible across different data sources.
package query

// LeakSeverity defines the severity level of a leak suspect.
type LeakSeverity string

const (
	// LeakSeverityInfo indicates an informational finding, not necessarily a leak.
	LeakSeverityInfo LeakSeverity = "info"

	// LeakSeverityWarning indicates a potential leak that warrants investigation.
	LeakSeverityWarning LeakSeverity = "warning"

	// LeakSeverityCritical indicates a confirmed or high-confidence leak.
	LeakSeverityCritical LeakSeverity = "critical"
)

// LeakSource identifies how the leak was detected.
type LeakSource string

const (
	// LeakSourceTimeSeries indicates detection by comparing multiple profiles over time.
	LeakSourceTimeSeries LeakSource = "time_series"

	// LeakSourceSnapshotHeuristic indicates detection by heuristic rules on a single snapshot.
	LeakSourceSnapshotHeuristic LeakSource = "snapshot_heuristic"

	// LeakSourceStaticAnalysis indicates detection by static analysis (e.g., unreleased resources).
	LeakSourceStaticAnalysis LeakSource = "static_analysis"
)

// LeakSuspect is the unified output model for all leak detection strategies.
// It provides a consistent interface for the frontend regardless of the detection method.
type LeakSuspect struct {
	// Type categorizes the leak (e.g., "heap", "goroutine", "class_accumulation").
	Type string `json:"type"`

	// Source identifies the detection strategy that produced this suspect.
	Source LeakSource `json:"source"`

	// Severity indicates how confident/severe the finding is.
	Severity LeakSeverity `json:"severity"`

	// Title is a concise human-readable summary (one line).
	Title string `json:"title"`

	// Description provides detailed explanation of the finding.
	Description string `json:"description"`

	// Evidence contains the supporting data points for this finding.
	Evidence []LeakEvidence `json:"evidence,omitempty"`

	// Metrics provides quantitative growth data (primarily for time-series detection).
	Metrics *LeakMetrics `json:"metrics,omitempty"`

	// Suggestions contains actionable recommendations for fixing the issue.
	Suggestions []string `json:"suggestions,omitempty"`
}

// LeakEvidence represents a single data point supporting a leak suspect finding.
type LeakEvidence struct {
	// Name identifies the entity (class name, function name, goroutine group, etc.).
	Name string `json:"name"`

	// Value is the numeric measurement.
	Value int64 `json:"value"`

	// Unit describes what the value represents.
	Unit string `json:"unit"` // "bytes", "count", "percent"

	// Detail provides optional human-readable context.
	Detail string `json:"detail,omitempty"`
}

// LeakMetrics provides quantitative growth data for time-series based detection.
type LeakMetrics struct {
	BaselineValue int64   `json:"baseline_value,omitempty"`
	CurrentValue  int64   `json:"current_value,omitempty"`
	GrowthValue   int64   `json:"growth_value,omitempty"`
	GrowthPercent float64 `json:"growth_percent,omitempty"`
}

// LeakSuspectsResult is the top-level response returned by the API.
type LeakSuspectsResult struct {
	// TotalCount is the total number of suspects found.
	TotalCount int `json:"total_count"`

	// Suspects is the list of detected leak suspects, sorted by severity (critical first).
	Suspects []LeakSuspect `json:"suspects"`
}

// LeakSuspectProvider defines the interface for a leak detection strategy.
// Each implementation encapsulates one detection approach and outputs the unified model.
//
// Design principles:
//   - SRP: each provider handles exactly one detection strategy
//   - OCP: new strategies are added by implementing this interface, not modifying existing code
//   - DIP: the API handler depends on this interface, not concrete implementations
type LeakSuspectProvider interface {
	// Name returns a unique identifier for this provider (for logging/debugging).
	Name() string

	// CanDetect checks whether the required data files exist in taskDir.
	// This enables the Chain of Responsibility pattern — providers that
	// lack data are skipped gracefully without errors.
	CanDetect(taskDir string) bool

	// Detect executes the detection logic and returns unified LeakSuspect results.
	// It should return an empty slice (not nil) if no issues are found.
	// Errors should only be returned for unexpected failures, not "no data" cases.
	Detect(taskDir string) ([]LeakSuspect, error)
}

// LeakSuspectEngine executes a collection of providers and aggregates results.
// It follows the same pattern as GoroutineRuleEngine for consistency.
type LeakSuspectEngine struct {
	providers []LeakSuspectProvider
}

// NewLeakSuspectEngine creates an engine with the given providers.
// Providers are evaluated in order; all applicable providers contribute results.
func NewLeakSuspectEngine(providers ...LeakSuspectProvider) *LeakSuspectEngine {
	return &LeakSuspectEngine{providers: providers}
}

// Detect runs all applicable providers and returns aggregated, sorted results.
func (e *LeakSuspectEngine) Detect(taskDir string) *LeakSuspectsResult {
	var allSuspects []LeakSuspect

	for _, p := range e.providers {
		if !p.CanDetect(taskDir) {
			continue
		}
		suspects, err := p.Detect(taskDir)
		if err != nil {
			// Non-fatal: skip this provider, continue with others
			continue
		}
		allSuspects = append(allSuspects, suspects...)
	}

	if allSuspects == nil {
		allSuspects = []LeakSuspect{}
	}

	// Sort by severity: critical > warning > info
	sortLeakSuspects(allSuspects)

	return &LeakSuspectsResult{
		TotalCount: len(allSuspects),
		Suspects:   allSuspects,
	}
}

// FilterByType returns only suspects matching the given type.
func (r *LeakSuspectsResult) FilterByType(leakType string) *LeakSuspectsResult {
	if leakType == "" || leakType == "all" {
		return r
	}

	var filtered []LeakSuspect
	for _, s := range r.Suspects {
		if s.Type == leakType {
			filtered = append(filtered, s)
		}
	}
	if filtered == nil {
		filtered = []LeakSuspect{}
	}

	return &LeakSuspectsResult{
		TotalCount: len(filtered),
		Suspects:   filtered,
	}
}

// FilterBySeverity returns only suspects at or above the given severity level.
func (r *LeakSuspectsResult) FilterBySeverity(minSeverity LeakSeverity) *LeakSuspectsResult {
	minLevel := severityLevel(minSeverity)
	var filtered []LeakSuspect
	for _, s := range r.Suspects {
		if severityLevel(s.Severity) >= minLevel {
			filtered = append(filtered, s)
		}
	}
	if filtered == nil {
		filtered = []LeakSuspect{}
	}

	return &LeakSuspectsResult{
		TotalCount: len(filtered),
		Suspects:   filtered,
	}
}

// severityLevel returns the numeric priority of a severity (higher = more severe).
func severityLevel(s LeakSeverity) int {
	switch s {
	case LeakSeverityCritical:
		return 3
	case LeakSeverityWarning:
		return 2
	case LeakSeverityInfo:
		return 1
	default:
		return 0
	}
}

// sortLeakSuspects sorts suspects by severity descending (critical first).
func sortLeakSuspects(suspects []LeakSuspect) {
	for i := 1; i < len(suspects); i++ {
		for j := i; j > 0 && severityLevel(suspects[j].Severity) > severityLevel(suspects[j-1].Severity); j-- {
			suspects[j], suspects[j-1] = suspects[j-1], suspects[j]
		}
	}
}
