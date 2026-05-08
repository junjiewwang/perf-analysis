// Package query provides reusable query utilities for analysis data.
// This file implements a generic search engine for profiling analysis data.
package query

import (
	"sort"
	"strings"

	"github.com/junjiewwang/perf-analysis/perflib/flamegraph"
)

// SearchResult represents a single search result item.
type SearchResult struct {
	Type       string      `json:"type"`                 // "function", "thread", "goroutine_group"
	Name       string      `json:"name"`
	Samples    int64       `json:"samples"`
	Percentage float64     `json:"percentage"`
	Context    interface{} `json:"context,omitempty"`    // Additional context data
}

// SearchEngine provides cross-data search capabilities for profiling analysis.
// It can search across flame graphs, thread analysis, and goroutine data.
type SearchEngine struct {
	cpuAnalysis *flamegraph.CPUAnalysisResult
	goroutine   *GoroutineQueryEngine
}

// NewSearchEngine creates a new SearchEngine.
func NewSearchEngine() *SearchEngine {
	return &SearchEngine{}
}

// WithCPUAnalysis sets the CPU analysis data source for searching.
func (s *SearchEngine) WithCPUAnalysis(cpu *flamegraph.CPUAnalysisResult) *SearchEngine {
	s.cpuAnalysis = cpu
	return s
}

// WithGoroutineData sets the goroutine data source for searching.
func (s *SearchEngine) WithGoroutineData(engine *GoroutineQueryEngine) *SearchEngine {
	s.goroutine = engine
	return s
}

// Search performs a search across all configured data sources.
// Parameters:
//   - query: search string (case-insensitive substring match)
//   - searchType: "function", "thread", "goroutine_group", or "" (all types)
//   - limit: maximum number of results (0 = default 50)
func (s *SearchEngine) Search(query string, searchType string, limit int) []SearchResult {
	if query == "" {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}

	queryLower := strings.ToLower(query)
	var results []SearchResult

	// Search CPU analysis data
	if s.cpuAnalysis != nil {
		if searchType == "" || searchType == "function" {
			results = append(results, s.searchCPUFunctions(queryLower)...)
		}
		if searchType == "" || searchType == "thread" {
			results = append(results, s.searchThreads(queryLower)...)
		}
	}

	// Search goroutine data
	if s.goroutine != nil && (searchType == "" || searchType == "goroutine_group") {
		results = append(results, s.searchGoroutineGroups(queryLower)...)
	}

	// Sort by samples descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Samples > results[j].Samples
	})

	// Limit
	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

// searchCPUFunctions searches top functions in CPU analysis.
func (s *SearchEngine) searchCPUFunctions(queryLower string) []SearchResult {
	if s.cpuAnalysis == nil {
		return nil
	}

	var results []SearchResult
	for _, f := range s.cpuAnalysis.TopFuncs {
		if strings.Contains(strings.ToLower(f.Name), queryLower) {
			results = append(results, SearchResult{
				Type:       "function",
				Name:       f.Name,
				Samples:    f.Samples,
				Percentage: f.Percentage,
				Context: map[string]interface{}{
					"module":       f.Module,
					"thread_count": f.ThreadCount,
				},
			})
		}
	}
	return results
}

// searchThreads searches threads in CPU analysis.
func (s *SearchEngine) searchThreads(queryLower string) []SearchResult {
	if s.cpuAnalysis == nil {
		return nil
	}

	var results []SearchResult
	for _, t := range s.cpuAnalysis.Threads {
		if strings.Contains(strings.ToLower(t.Name), queryLower) {
			results = append(results, SearchResult{
				Type:       "thread",
				Name:       t.Name,
				Samples:    t.Samples,
				Percentage: t.Percentage,
				Context: map[string]interface{}{
					"tid":   t.TID,
					"group": t.Group,
				},
			})
		}
	}
	return results
}

// searchGoroutineGroups searches goroutine groups by top function name.
func (s *SearchEngine) searchGoroutineGroups(queryLower string) []SearchResult {
	if s.goroutine == nil || s.goroutine.data == nil {
		return nil
	}

	var results []SearchResult
	for _, g := range s.goroutine.data.Distribution {
		if strings.Contains(strings.ToLower(g.TopFunc), queryLower) {
			results = append(results, SearchResult{
				Type:       "goroutine_group",
				Name:       g.TopFunc,
				Samples:    g.Count,
				Percentage: g.Percentage,
				Context: map[string]interface{}{
					"state": g.State,
					"stack": g.Stack,
				},
			})
		}
	}
	return results
}
