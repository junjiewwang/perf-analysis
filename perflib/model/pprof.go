// Package model defines output data abstractions for different analysis types.
package model

// PProfTopFunc represents a top function in pprof analysis.
type PProfTopFunc struct {
	Name       string  `json:"name"`
	Flat       int64   `json:"flat"`
	FlatPct    float64 `json:"flat_pct"`
	Cum        int64   `json:"cum"`
	CumPct     float64 `json:"cum_pct"`
	Module     string  `json:"module,omitempty"`
	SourceFile string  `json:"source_file,omitempty"`
	SourceLine int     `json:"source_line,omitempty"`
}

// PProfMemoryStats holds memory statistics for a specific sample type.
type PProfMemoryStats struct {
	Total     int64          `json:"total"`
	Unit      string         `json:"unit"`
	TopFuncs  []PProfTopFunc `json:"top_funcs"`
	TopNCount int            `json:"top_n_count"`
}

// PProfCPUData holds Go pprof CPU analysis data.
type PProfCPUData struct {
	FlameGraphFile string         `json:"flamegraph_file"`
	CallGraphFile  string         `json:"callgraph_file,omitempty"`
	Duration       int64          `json:"duration_ns"`
	TotalSamples   int64          `json:"total_samples"`
	SampleUnit     string         `json:"sample_unit"`
	TopFuncs       []PProfTopFunc `json:"top_funcs"`
	TopFuncsByFlat []PProfTopFunc `json:"top_funcs_by_flat,omitempty"`
	TopFuncsByCum  []PProfTopFunc `json:"top_funcs_by_cum,omitempty"`
}

// Type returns the analysis data type.
func (d *PProfCPUData) Type() AnalysisDataType {
	return DataTypePProfCPU
}

// Summary returns a summary of the pprof CPU analysis.
func (d *PProfCPUData) Summary() map[string]interface{} {
	return map[string]interface{}{
		"duration_ns":     d.Duration,
		"total_samples":   d.TotalSamples,
		"sample_unit":     d.SampleUnit,
		"flamegraph_file": d.FlameGraphFile,
		"callgraph_file":  d.CallGraphFile,
	}
}

// TopItems returns the top functions from pprof CPU analysis.
func (d *PProfCPUData) TopItems() []TopItem {
	items := make([]TopItem, 0, len(d.TopFuncs))
	for _, tf := range d.TopFuncs {
		items = append(items, TopItem{
			Name:       tf.Name,
			Value:      tf.Flat,
			Percentage: tf.FlatPct,
			Extra: map[string]interface{}{
				"cum":     tf.Cum,
				"cum_pct": tf.CumPct,
			},
		})
	}
	return items
}

// PProfHeapSummary holds summary statistics for heap analysis.
type PProfHeapSummary struct {
	TotalInuseBytes   int64 `json:"total_inuse_bytes"`
	TotalInuseObjects int64 `json:"total_inuse_objects"`
	TotalAllocBytes   int64 `json:"total_alloc_bytes"`
	TotalAllocObjects int64 `json:"total_alloc_objects"`
}

// PProfHeapData holds Go pprof Heap analysis data.
type PProfHeapData struct {
	InuseSpace      *PProfMemoryStats `json:"inuse_space"`
	InuseObjects    *PProfMemoryStats `json:"inuse_objects"`
	AllocSpace      *PProfMemoryStats `json:"alloc_space"`
	AllocObjects    *PProfMemoryStats `json:"alloc_objects"`
	FlameGraphFiles map[string]string `json:"flamegraph_files"`
	HeapSummary     *PProfHeapSummary `json:"summary"`
}

// Type returns the analysis data type.
func (d *PProfHeapData) Type() AnalysisDataType {
	return DataTypePProfHeap
}

// Summary returns a summary of the pprof Heap analysis.
func (d *PProfHeapData) Summary() map[string]interface{} {
	result := map[string]interface{}{
		"flamegraph_files": d.FlameGraphFiles,
	}
	if d.HeapSummary != nil {
		result["total_inuse_bytes"] = d.HeapSummary.TotalInuseBytes
		result["total_inuse_objects"] = d.HeapSummary.TotalInuseObjects
		result["total_alloc_bytes"] = d.HeapSummary.TotalAllocBytes
		result["total_alloc_objects"] = d.HeapSummary.TotalAllocObjects
	}
	return result
}

// TopItems returns the top functions from pprof Heap analysis (by inuse_space).
func (d *PProfHeapData) TopItems() []TopItem {
	if d.InuseSpace == nil {
		return nil
	}
	items := make([]TopItem, 0, len(d.InuseSpace.TopFuncs))
	for _, tf := range d.InuseSpace.TopFuncs {
		items = append(items, TopItem{
			Name:       tf.Name,
			Value:      tf.Flat,
			Percentage: tf.FlatPct,
		})
	}
	return items
}

// GoroutineGroup represents a group of goroutines with the same stack.
type GoroutineGroup struct {
	Count      int64    `json:"count"`
	Percentage float64  `json:"percentage"`
	State      string   `json:"state,omitempty"`
	TopFunc    string   `json:"top_func"`
	Stack      []string `json:"stack,omitempty"`
}

// PProfGoroutineData holds Go pprof Goroutine analysis data.
type PProfGoroutineData struct {
	TotalCount     int64            `json:"total_count"`
	Distribution   []GoroutineGroup `json:"distribution"`
	TopFuncs       []PProfTopFunc   `json:"top_funcs"`
	FlameGraphFile string           `json:"flamegraph_file,omitempty"`
}

// Type returns the analysis data type.
func (d *PProfGoroutineData) Type() AnalysisDataType {
	return DataTypePProfGoroutine
}

// Summary returns a summary of the pprof Goroutine analysis.
func (d *PProfGoroutineData) Summary() map[string]interface{} {
	return map[string]interface{}{
		"total_count":     d.TotalCount,
		"group_count":     len(d.Distribution),
		"flamegraph_file": d.FlameGraphFile,
	}
}

// TopItems returns the top functions from pprof Goroutine analysis.
func (d *PProfGoroutineData) TopItems() []TopItem {
	items := make([]TopItem, 0, len(d.TopFuncs))
	for _, tf := range d.TopFuncs {
		items = append(items, TopItem{
			Name:       tf.Name,
			Value:      tf.Flat,
			Percentage: tf.FlatPct,
		})
	}
	return items
}

// PProfBlockData holds Go pprof Block/Mutex analysis data.
type PProfBlockData struct {
	TotalDelay     int64          `json:"total_delay"`
	TotalCount     int64          `json:"total_count"`
	Unit           string         `json:"unit"`
	TopFuncs       []PProfTopFunc `json:"top_funcs"`
	FlameGraphFile string         `json:"flamegraph_file,omitempty"`
}

// Type returns the analysis data type.
func (d *PProfBlockData) Type() AnalysisDataType {
	return DataTypePProfBlock
}

// Summary returns a summary of the pprof Block analysis.
func (d *PProfBlockData) Summary() map[string]interface{} {
	return map[string]interface{}{
		"total_delay":     d.TotalDelay,
		"total_count":     d.TotalCount,
		"unit":            d.Unit,
		"flamegraph_file": d.FlameGraphFile,
	}
}

// TopItems returns the top functions from pprof Block analysis.
func (d *PProfBlockData) TopItems() []TopItem {
	items := make([]TopItem, 0, len(d.TopFuncs))
	for _, tf := range d.TopFuncs {
		items = append(items, TopItem{
			Name:       tf.Name,
			Value:      tf.Flat,
			Percentage: tf.FlatPct,
		})
	}
	return items
}

// PProfBatchProfileSet represents analysis results for a set of profiles.
type PProfBatchProfileSet struct {
	ProfileType  string `json:"profile_type"`
	FileCount    int    `json:"file_count"`
	TotalSamples int64  `json:"total_samples"`
	LatestFile   string `json:"latest_file"`
}

// PProfLeakReportSummary represents a summary of leak detection.
type PProfLeakReportSummary struct {
	Type          string  `json:"type"`
	Severity      string  `json:"severity"`
	Conclusion    string  `json:"conclusion"`
	TotalGrowth   int64   `json:"total_growth"`
	GrowthPercent float64 `json:"growth_percent"`
	ItemsCount    int     `json:"items_count"`
}

// PProfLeakGrowthItem represents a single growth item in leak detection.
type PProfLeakGrowthItem struct {
	Name          string  `json:"name"`
	BaselineValue int64   `json:"baseline_value"`
	CurrentValue  int64   `json:"current_value"`
	GrowthValue   int64   `json:"growth_value"`
	GrowthPercent float64 `json:"growth_percent"`
}

// PProfLeakReport represents a detailed leak detection report.
type PProfLeakReport struct {
	Type               string                `json:"type"`
	Severity           string                `json:"severity"`
	Conclusion         string                `json:"conclusion"`
	BaselineTotal      int64                 `json:"baseline_total"`
	CurrentTotal       int64                 `json:"current_total"`
	TotalGrowth        int64                 `json:"total_growth"`
	TotalGrowthPercent float64               `json:"total_growth_percent"`
	GrowthItems        []PProfLeakGrowthItem `json:"growth_items,omitempty"`
}

// PProfBatchData holds Go pprof batch analysis data (pprof-all mode).
type PProfBatchData struct {
	ProfileSets         map[string]*PProfBatchProfileSet   `json:"profile_sets"`
	LeakReports         map[string]*PProfLeakReportSummary `json:"leak_reports,omitempty"`
	DetailedLeakReports map[string]*PProfLeakReport        `json:"detailed_leak_reports,omitempty"`
	TopFuncs            []PProfTopFunc                     `json:"top_funcs,omitempty"`
	TotalSamples        int64                              `json:"total_samples"`
}

// Type returns the analysis data type.
func (d *PProfBatchData) Type() AnalysisDataType {
	return DataTypePProfBatch
}

// Summary returns a summary of the pprof batch analysis.
func (d *PProfBatchData) Summary() map[string]interface{} {
	profileSetsSummary := make(map[string]interface{})
	for name, ps := range d.ProfileSets {
		profileSetsSummary[name] = map[string]interface{}{
			"profile_type":  ps.ProfileType,
			"file_count":    ps.FileCount,
			"total_samples": ps.TotalSamples,
			"latest_file":   ps.LatestFile,
		}
	}

	leakReportsSummary := make(map[string]interface{})
	for name, lr := range d.LeakReports {
		leakReportsSummary[name] = map[string]interface{}{
			"type":           lr.Type,
			"severity":       lr.Severity,
			"conclusion":     lr.Conclusion,
			"total_growth":   lr.TotalGrowth,
			"growth_percent": lr.GrowthPercent,
			"items_count":    lr.ItemsCount,
		}
	}

	return map[string]interface{}{
		"total_samples": d.TotalSamples,
		"profile_sets":  profileSetsSummary,
		"leak_reports":  leakReportsSummary,
	}
}

// TopItems returns the top functions from pprof batch analysis.
func (d *PProfBatchData) TopItems() []TopItem {
	items := make([]TopItem, 0, len(d.TopFuncs))
	for _, tf := range d.TopFuncs {
		items = append(items, TopItem{
			Name:       tf.Name,
			Value:      tf.Flat,
			Percentage: tf.FlatPct,
		})
	}
	return items
}
