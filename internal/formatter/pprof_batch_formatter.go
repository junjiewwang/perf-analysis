package formatter

import (
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// PProfBatchFormatter formats pprof batch analysis results.
type PProfBatchFormatter struct{}

// SupportedTypes returns the data types this formatter supports.
func (f *PProfBatchFormatter) SupportedTypes() []model.AnalysisDataType {
	return []model.AnalysisDataType{model.DataTypePProfBatch}
}

// Format outputs the pprof batch analysis result to the logger.
func (f *PProfBatchFormatter) Format(resp *model.AnalysisResponse, log utils.Logger) {
	log.Info("=== PProf Batch Analysis Results ===")
	log.Info("Task UUID:      %s", resp.TaskUUID)
	log.Info("Total Samples:  %d", resp.TotalRecords)
	log.Info("")

	data, ok := resp.Data.(*model.PProfBatchData)
	if !ok || data == nil {
		log.Warn("No batch data available")
		return
	}

	// Print profile sets
	log.Info("=== Profile Sets ===")
	for name, ps := range data.ProfileSets {
		log.Info("  %s:", name)
		log.Info("    Files:   %d", ps.FileCount)
		log.Info("    Samples: %d", ps.TotalSamples)
	}
	log.Info("")

	// Print leak reports
	if len(data.LeakReports) > 0 {
		log.Info("=== Leak Detection ===")
		for name, lr := range data.LeakReports {
			log.Info("  %s Leak:", name)
			log.Info("    Severity:   %s", lr.Severity)
			log.Info("    Conclusion: %s", lr.Conclusion)
			if lr.GrowthPercent != 0 {
				log.Info("    Growth:     %.2f%%", lr.GrowthPercent)
			}
		}
		log.Info("")
	}

	// Print top functions
	if len(data.TopFuncs) > 0 {
		log.Info("=== Top Functions (CPU) ===")
		count := min(10, len(data.TopFuncs))
		for i := 0; i < count; i++ {
			tf := data.TopFuncs[i]
			log.Info("  %2d. %6.2f%%  %s", i+1, tf.FlatPct, truncateString(tf.Name, 80))
		}
		log.Info("")
	}
}

// FormatSummary returns a summary map for serialization.
func (f *PProfBatchFormatter) FormatSummary(resp *model.AnalysisResponse) map[string]interface{} {
	summary := map[string]interface{}{
		"task_uuid":     resp.TaskUUID,
		"task_type":     "pprof_all",
		"total_records": resp.TotalRecords,
	}

	data, ok := resp.Data.(*model.PProfBatchData)
	if !ok || data == nil {
		return summary
	}

	// Add profile sets
	profileSets := make(map[string]interface{})
	for name, ps := range data.ProfileSets {
		profileSets[name] = map[string]interface{}{
			"profile_type":  ps.ProfileType,
			"file_count":    ps.FileCount,
			"total_samples": ps.TotalSamples,
			"latest_file":   ps.LatestFile,
		}
	}
	summary["profile_sets"] = profileSets

	// Add leak reports
	leakReports := make(map[string]interface{})
	for name, lr := range data.LeakReports {
		leakReports[name] = map[string]interface{}{
			"type":           lr.Type,
			"severity":       lr.Severity,
			"conclusion":     lr.Conclusion,
			"total_growth":   lr.TotalGrowth,
			"growth_percent": lr.GrowthPercent,
			"items_count":    lr.ItemsCount,
		}
	}
	summary["leak_reports"] = leakReports

	// Add top items
	topItems := make([]map[string]interface{}, 0, len(data.TopFuncs))
	for _, tf := range data.TopFuncs {
		topItems = append(topItems, map[string]interface{}{
			"name":       tf.Name,
			"samples":    tf.Flat,
			"percentage": tf.FlatPct,
		})
	}
	summary["top_items"] = topItems

	return summary
}
