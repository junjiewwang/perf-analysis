// Package query provides reusable query utilities for analysis data.
// These utilities can be used by any consumer (WebUI, CLI, API server) to query
// pre-computed analysis results without re-parsing raw profile data.
package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/junjiewwang/perf-analysis/perflib/model"
	"github.com/junjiewwang/perf-analysis/perflib/output"
)

// GoroutineQueryEngine provides query operations on goroutine analysis data.
// It loads pre-computed goroutine analysis results and provides filtering,
// sorting, and aggregation capabilities.
type GoroutineQueryEngine struct {
	data *model.PProfGoroutineData
}

// NewGoroutineQueryEngine creates a new engine from a PProfGoroutineData instance.
func NewGoroutineQueryEngine(data *model.PProfGoroutineData) *GoroutineQueryEngine {
	return &GoroutineQueryEngine{data: data}
}

// LoadGoroutineQueryEngine loads goroutine analysis data from a JSON file.
func LoadGoroutineQueryEngine(filePath string) (*GoroutineQueryEngine, error) {
	var data model.PProfGoroutineData
	if err := output.ReadJSON(filePath, &data); err != nil {
		return nil, fmt.Errorf("failed to load goroutine analysis: %w", err)
	}
	return NewGoroutineQueryEngine(&data), nil
}

// GoroutineGroupsResult represents the response for goroutine groups query.
type GoroutineGroupsResult struct {
	TotalCount int64                  `json:"total_count"`
	GroupCount int                    `json:"group_count"`
	Groups     []model.GoroutineGroup `json:"groups"`
	TopFuncs   []model.PProfTopFunc   `json:"top_funcs"`
}

// GoroutineStatsResult represents aggregated statistics for goroutine analysis.
type GoroutineStatsResult struct {
	TotalCount    int64                   `json:"total_count"`
	GroupCount    int                     `json:"group_count"`
	StateDistrib  []StateDistribution     `json:"state_distribution"`
	TopFuncs      []model.PProfTopFunc    `json:"top_funcs"`
	LargestGroup  *model.GoroutineGroup   `json:"largest_group,omitempty"`
}

// StateDistribution represents goroutine distribution by state.
type StateDistribution struct {
	State      string  `json:"state"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// GoroutineIssue represents a detected concurrency issue.
type GoroutineIssue struct {
	Severity     string   `json:"severity"`               // "critical", "warning", "info"
	Type         string   `json:"type"`                   // "excessive", "blocking", "io_wait", "mutex_contention", etc.
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	GroupIndex   int      `json:"group_index,omitempty"`
	Suggestion   string   `json:"suggestion,omitempty"`    // Actionable remediation advice
	RelatedFuncs []string `json:"related_funcs,omitempty"` // Related function names for context
}

// QueryGroups returns goroutine groups with optional sorting and limit.
func (e *GoroutineQueryEngine) QueryGroups(sortBy string, topN int) *GoroutineGroupsResult {
	if e.data == nil {
		return &GoroutineGroupsResult{}
	}

	groups := make([]model.GoroutineGroup, len(e.data.Distribution))
	copy(groups, e.data.Distribution)

	// Sort
	switch strings.ToLower(sortBy) {
	case "percentage":
		sort.Slice(groups, func(i, j int) bool {
			return groups[i].Percentage > groups[j].Percentage
		})
	default: // "count" or empty
		sort.Slice(groups, func(i, j int) bool {
			return groups[i].Count > groups[j].Count
		})
	}

	// Limit
	if topN > 0 && topN < len(groups) {
		groups = groups[:topN]
	}

	return &GoroutineGroupsResult{
		TotalCount: e.data.TotalCount,
		GroupCount: len(e.data.Distribution),
		Groups:     groups,
		TopFuncs:   e.data.TopFuncs,
	}
}

// QueryStats returns aggregated goroutine statistics.
func (e *GoroutineQueryEngine) QueryStats() *GoroutineStatsResult {
	if e.data == nil {
		return &GoroutineStatsResult{}
	}

	result := &GoroutineStatsResult{
		TotalCount: e.data.TotalCount,
		GroupCount: len(e.data.Distribution),
		TopFuncs:   e.data.TopFuncs,
	}

	// Aggregate state distribution
	stateMap := make(map[string]int64)
	for i := range e.data.Distribution {
		g := &e.data.Distribution[i]
		state := g.State
		if state == "" {
			state = "running"
		}
		stateMap[state] += g.Count
	}

	for state, count := range stateMap {
		var pct float64
		if e.data.TotalCount > 0 {
			pct = float64(count) * 100.0 / float64(e.data.TotalCount)
		}
		result.StateDistrib = append(result.StateDistrib, StateDistribution{
			State:      state,
			Count:      count,
			Percentage: pct,
		})
	}

	// Sort by count descending
	sort.Slice(result.StateDistrib, func(i, j int) bool {
		return result.StateDistrib[i].Count > result.StateDistrib[j].Count
	})

	// Find largest group
	if len(e.data.Distribution) > 0 {
		largest := e.data.Distribution[0]
		for i := 1; i < len(e.data.Distribution); i++ {
			if e.data.Distribution[i].Count > largest.Count {
				largest = e.data.Distribution[i]
			}
		}
		result.LargestGroup = &largest
	}

	return result
}

// QueryIssues detects potential concurrency issues from goroutine distribution.
// It delegates to the GoroutineRuleEngine which runs all registered rules.
func (e *GoroutineQueryEngine) QueryIssues() []GoroutineIssue {
	if e.data == nil {
		return nil
	}

	ruleEngine := NewGoroutineRuleEngine()
	return ruleEngine.Evaluate(e.data)
}

// truncateFunc truncates a function name for display.
func truncateFunc(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen-3] + "..."
}
