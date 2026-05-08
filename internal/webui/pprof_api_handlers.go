// Package webui provides the web UI server for performance analysis.
// This file implements the pprof analysis API handlers (goroutine, search, etc.).
package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
// It first tries precomputed class_stats.json, then falls back to runtime computation.
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

	taskDir := s.resolveTaskDir(r.Context(), taskID)

	// Strategy 1: Try precomputed class_stats.json (fast path)
	classStatsFile := filepath.Join(taskDir, output.FileClassStats)
	if data, err := os.ReadFile(classStatsFile); err == nil {
		// Parse precomputed data and apply sort/filter/limit
		var precomputed query.ClassHistogramResult
		if json.Unmarshal(data, &precomputed) == nil {
			result := applyClassHistogramParams(&precomputed, sortBy, topN, filter)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			json.NewEncoder(w).Encode(result)
			return
		}
	}

	// Strategy 2: Fallback to runtime computation via HeapGraph
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
// It first tries precomputed heap_stats.json, then falls back to runtime computation.
// GET /api/refgraph/heap-stats?task=<id>
func (s *Server) handleHeapStats(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	taskDir := s.resolveTaskDir(r.Context(), taskID)

	// Strategy 1: Try precomputed heap_stats.json (fast path)
	heapStatsFile := filepath.Join(taskDir, output.FileHeapStats)
	if data, err := os.ReadFile(heapStatsFile); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(data)
		return
	}

	// Strategy 2: Fallback to runtime computation via HeapGraph
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

// handleHeapHistogram is the unified API for both Java hprof and Go pprof heap histograms.
// It auto-detects the data type and delegates to the appropriate query helper.
// GET /api/heap/histogram?task=<id>&metric=<inuse_space|...>&sort=<flat|cum>&top=<N>&filter=<name>
func (s *Server) handleHeapHistogram(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "inuse_space"
	}

	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "flat"
	}

	topN := 100
	if tn := r.URL.Query().Get("top"); tn != "" {
		if n, err := parseInt(tn); err == nil && n > 0 {
			topN = n
		}
	}

	filter := r.URL.Query().Get("filter")
	taskDir := s.resolveTaskDir(r.Context(), taskID)

	// Detection strategy: if heap_index.bin exists → Java hprof; else try pprof data
	indexFile := filepath.Join(taskDir, output.FileHeapIndex)
	if _, err := os.Stat(indexFile); err == nil {
		// Java hprof path: read precomputed or fallback to HeapGraph
		result := s.handleJavaHeapHistogram(taskID, taskDir, sortBy, topN, filter)
		if result != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			json.NewEncoder(w).Encode(result)
			return
		}
	}

	// Go pprof path: try heap_analysis.json
	pprofFile := filepath.Join(taskDir, output.FilePProfHeapAnalysis)
	pprofHelper, err := query.LoadPProfHeapQueryHelper(pprofFile)
	if err != nil {
		http.Error(w, "Heap analysis data not found", http.StatusNotFound)
		return
	}

	result := pprofHelper.QueryAllocHistogram(metric, sortBy, topN, filter)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(result)
}

// handleJavaHeapHistogram builds a unified histogram response from Java hprof data.
func (s *Server) handleJavaHeapHistogram(taskID, taskDir, sortBy string, topN int, filter string) *query.AllocHistogramResponse {
	// Try precomputed class_stats.json first
	classStatsFile := filepath.Join(taskDir, output.FileClassStats)
	if data, err := os.ReadFile(classStatsFile); err == nil {
		var precomputed query.ClassHistogramResult
		if json.Unmarshal(data, &precomputed) == nil {
			return convertClassHistogramToUnified(&precomputed, sortBy, topN, filter)
		}
	}

	// Fallback to runtime HeapGraph
	helper, err := s.refGraphService.GetHeapQueryHelper(taskID)
	if err != nil {
		return nil
	}

	result := helper.QueryClassHistogram(sortBy, topN, filter)
	return convertClassHistogramToUnified(result, "", 0, "") // already sorted/filtered
}

// convertClassHistogramToUnified adapts ClassHistogramResult to the unified AllocHistogramResponse.
func convertClassHistogramToUnified(ch *query.ClassHistogramResult, sortBy string, topN int, filter string) *query.AllocHistogramResponse {
	if ch == nil {
		return nil
	}

	entries := make([]query.AllocHistogramEntry, 0, len(ch.Classes))
	for _, c := range ch.Classes {
		if filter != "" {
			lower := toLowerSimple(c.ClassName)
			if !containsSubstring(lower, toLowerSimple(filter)) {
				continue
			}
		}
		entries = append(entries, query.AllocHistogramEntry{
			Name:    c.ClassName,
			Flat:    c.ShallowSize,
			FlatPct: 0, // shallow pct not precomputed
			Cum:     c.RetainedSize,
			CumPct:  c.Percentage,
			Count:   c.ObjectCount,
		})
	}

	if topN > 0 && topN < len(entries) {
		entries = entries[:topN]
	}

	return &query.AllocHistogramResponse{
		Source:     "java_hprof",
		Metric:     "retained_size",
		Total:      ch.TotalSize,
		Unit:       "bytes",
		EntryCount: ch.TotalClasses,
		Entries:    entries,
	}
}

// applyClassHistogramParams applies sort/filter/limit to precomputed class stats.
func applyClassHistogramParams(result *query.ClassHistogramResult, sortBy string, topN int, filter string) *query.ClassHistogramResult {
	if filter != "" {
		filtered := make([]query.ClassHistogramEntry, 0)
		filterLower := toLowerSimple(filter)
		for _, c := range result.Classes {
			if containsSubstring(toLowerSimple(c.ClassName), filterLower) {
				filtered = append(filtered, c)
			}
		}
		result.Classes = filtered
	}

	// Re-sort if needed (precomputed file is sorted by retained desc by default)
	switch sortBy {
	case "shallow":
		sortClassEntries(result.Classes, func(a, b query.ClassHistogramEntry) bool {
			return a.ShallowSize > b.ShallowSize
		})
	case "count":
		sortClassEntries(result.Classes, func(a, b query.ClassHistogramEntry) bool {
			return a.ObjectCount > b.ObjectCount
		})
	}

	if topN > 0 && topN < len(result.Classes) {
		result.Classes = result.Classes[:topN]
	}

	return result
}

// sortClassEntries sorts ClassHistogramEntry slice with a custom comparator.
func sortClassEntries(entries []query.ClassHistogramEntry, less func(a, b query.ClassHistogramEntry) bool) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && less(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

// toLowerSimple converts ASCII chars to lowercase.
func toLowerSimple(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// containsSubstring checks if s contains substr.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

// findSubstring finds index of substr in s.
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
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
