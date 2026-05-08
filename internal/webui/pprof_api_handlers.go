// Package webui provides the web UI server for performance analysis.
// This file implements the pprof analysis API handlers (goroutine, search, etc.).
package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/junjiewwang/perf-analysis/perflib/flamegraph"
	"github.com/junjiewwang/perf-analysis/perflib/output"
	"github.com/junjiewwang/perf-analysis/perflib/query"
)

// handleGoroutineGroups returns goroutine groups with distribution data.
// GET /api/goroutine/groups?task=<id>&sort=<count|percentage>&top=<N>
func (s *Server) handleGoroutineGroups(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "count"
	}

	topN := 0 // 0 means all
	if tn := r.URL.Query().Get("top"); tn != "" {
		if n, err := parseInt(tn); err == nil && n > 0 {
			topN = n
		}
	}

	taskDir := s.resolveTaskDir(r.Context(), taskID)

	// Try to load goroutine analysis data
	engine, err := s.loadGoroutineEngine(taskDir)
	if err != nil {
		http.Error(w, "Goroutine analysis data not found", http.StatusNotFound)
		return
	}

	result := engine.QueryGroups(sortBy, topN)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(result)
}

// handleGoroutineStats returns aggregated goroutine statistics.
// GET /api/goroutine/stats?task=<id>
func (s *Server) handleGoroutineStats(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	taskDir := s.resolveTaskDir(r.Context(), taskID)

	engine, err := s.loadGoroutineEngine(taskDir)
	if err != nil {
		http.Error(w, "Goroutine analysis data not found", http.StatusNotFound)
		return
	}

	result := engine.QueryStats()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(result)
}

// handleGoroutineIssues returns detected concurrency issues.
// GET /api/goroutine/issues?task=<id>
func (s *Server) handleGoroutineIssues(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	taskDir := s.resolveTaskDir(r.Context(), taskID)

	engine, err := s.loadGoroutineEngine(taskDir)
	if err != nil {
		http.Error(w, "Goroutine analysis data not found", http.StatusNotFound)
		return
	}

	issues := engine.QueryIssues()
	if issues == nil {
		issues = []query.GoroutineIssue{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(issues)
}

// handleSearch performs a global search across all analysis data.
// GET /api/search?task=<id>&q=<query>&type=<function|thread|goroutine_group>&limit=<N>
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	searchType := r.URL.Query().Get("type")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := parseInt(l); err == nil && n > 0 {
			limit = n
		}
	}

	taskDir := s.resolveTaskDir(r.Context(), taskID)
	searchEngine := query.NewSearchEngine()

	// Try to load CPU analysis data (from flamegraph with thread_analysis)
	ctx := r.Context()
	fg, err := s.fgService.GetFlameGraph(ctx, taskID, FlameGraphTypeCPU)
	if err == nil && fg != nil && fg.ThreadAnalysis != nil {
		// Build CPUAnalysisResult from ThreadAnalysisData for search
		cpuResult := s.buildCPUAnalysisFromFlameGraph(fg)
		if cpuResult != nil {
			searchEngine.WithCPUAnalysis(cpuResult)
		}
	}

	// Try to load goroutine data
	goroutineEngine, err := s.loadGoroutineEngine(taskDir)
	if err == nil {
		searchEngine.WithGoroutineData(goroutineEngine)
	}

	results := searchEngine.Search(q, searchType, limit)
	if results == nil {
		results = []query.SearchResult{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(results)
}

// handleClassHistogram returns class-level aggregated heap statistics.
// GET /api/refgraph/class-histogram?task=<id>&sort=<retained|shallow|count>&top=<N>&filter=<className>
func (s *Server) handleClassHistogram(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "retained"
	}

	topN := 100
	if tn := r.URL.Query().Get("top"); tn != "" {
		if n, err := parseInt(tn); err == nil && n > 0 {
			topN = n
		}
	}

	filter := r.URL.Query().Get("filter")

	// Get heap query helper through the provider
	helper, err := s.refGraphService.GetHeapQueryHelper(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	result := helper.QueryClassHistogram(sortBy, topN, filter)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(result)
}

// handleHeapStats returns high-level heap statistics.
// GET /api/refgraph/heap-stats?task=<id>
func (s *Server) handleHeapStats(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	helper, err := s.refGraphService.GetHeapQueryHelper(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	result := helper.QueryHeapStats()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(result)
}

// loadGoroutineEngine loads the goroutine query engine from analysis output files.
// It searches for goroutine_analysis.json in multiple locations.
func (s *Server) loadGoroutineEngine(taskDir string) (*query.GoroutineQueryEngine, error) {
	// Try multiple locations for goroutine analysis data
	candidates := []string{
		filepath.Join(taskDir, output.DirGoroutine, output.FileGoroutineAnalysis),
		filepath.Join(taskDir, output.FileGoroutineAnalysis),
	}

	for _, path := range candidates {
		engine, err := query.LoadGoroutineQueryEngine(path)
		if err == nil {
			return engine, nil
		}
	}

	return nil, fmt.Errorf("goroutine analysis data not found in %s", taskDir)
}

// buildCPUAnalysisFromFlameGraph converts FlameGraph thread analysis data
// to a CPUAnalysisResult for search compatibility.
func (s *Server) buildCPUAnalysisFromFlameGraph(fg *flamegraph.FlameGraph) *flamegraph.CPUAnalysisResult {
	if fg == nil || fg.ThreadAnalysis == nil {
		return nil
	}

	ta := fg.ThreadAnalysis
	result := flamegraph.NewCPUAnalysisResult()
	result.TotalSamples = fg.TotalSamples
	result.TotalThreads = ta.TotalThreads
	result.ActiveThreads = ta.ActiveThreads
	result.UniqueFunctions = ta.UniqueFunctions
	result.Threads = ta.Threads
	result.TopFuncs = ta.TopFunctions

	return result
}
