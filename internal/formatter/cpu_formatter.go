package formatter

import (
	"os"

	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// CPUFormatter formats CPU profiling analysis results.
type CPUFormatter struct{}

// SupportedTypes returns the data types this formatter supports.
func (f *CPUFormatter) SupportedTypes() []model.AnalysisDataType {
	return []model.AnalysisDataType{model.DataTypeCPUProfiling}
}

// Format outputs the CPU profiling result to the logger.
func (f *CPUFormatter) Format(resp *model.AnalysisResponse, log utils.Logger) {
	log.Info("=== Analysis Results ===")
	log.Info("Task UUID:      %s", resp.TaskUUID)
	log.Info("Task Type:      %s", resp.TaskType.String())
	log.Info("Total Samples:  %d", resp.TotalRecords)
	log.Info("")

	data, ok := resp.Data.(*model.CPUProfilingData)
	if !ok {
		log.Info("(No detailed data available)")
		return
	}

	// Print top functions
	log.Info("=== Top Functions ===")
	topItems := data.TopItems()
	count := min(10, len(topItems))
	for i := 0; i < count; i++ {
		item := topItems[i]
		log.Info("  %2d. %6.2f%%  %s", i+1, item.Percentage, truncateString(item.Name, 80))
	}
	log.Info("")

	// Print thread statistics
	log.Info("=== Thread Statistics ===")
	threadCount := min(5, len(data.ThreadStats))
	for i := 0; i < threadCount; i++ {
		t := data.ThreadStats[i]
		log.Info("  Thread: %s, Samples: %d (%.2f%%)", t.ThreadName, t.Samples, t.Percentage)
	}
	log.Info("")

	// Print output files
	f.printOutputFiles(resp, log)

	// Print suggestions
	f.printSuggestions(resp, log)
}

// FormatSummary returns a summary map for serialization.
func (f *CPUFormatter) FormatSummary(resp *model.AnalysisResponse) map[string]interface{} {
	summary := map[string]interface{}{
		"task_uuid":     resp.TaskUUID,
		"task_type":     resp.TaskType.String(),
		"total_records": resp.TotalRecords,
	}

	if resp.Data != nil {
		summary["data"] = resp.Data.Summary()
		summary["top_items"] = resp.Data.TopItems()

		// Include thread stats for frontend
		if cpuData, ok := resp.Data.(*model.CPUProfilingData); ok {
			summary["threads"] = cpuData.ThreadStats
		}
	}

	summary["output_files"] = resp.OutputFiles
	summary["suggestions_count"] = len(resp.Suggestions)
	summary["suggestions"] = resp.Suggestions

	return summary
}

func (f *CPUFormatter) printOutputFiles(resp *model.AnalysisResponse, log utils.Logger) {
	log.Info("=== Output Files ===")
	for _, file := range resp.OutputFiles {
		log.Info("  %s: %s", file.Name, file.LocalPath)
		if info, err := os.Stat(file.LocalPath); err == nil {
			log.Info("    Size: %d bytes", info.Size())
		}
	}
}

func (f *CPUFormatter) printSuggestions(resp *model.AnalysisResponse, log utils.Logger) {
	if len(resp.Suggestions) > 0 {
		log.Info("")
		log.Info("=== Suggestions ===")
		for i, sug := range resp.Suggestions {
			if i >= 5 {
				log.Info("  ... and %d more suggestions", len(resp.Suggestions)-5)
				break
			}
			log.Info("  - %s", truncateString(sug.Suggestion, 100))
		}
	}
}
