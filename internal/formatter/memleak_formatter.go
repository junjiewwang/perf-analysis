package formatter

import (
	"os"

	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// MemLeakFormatter formats memory leak analysis results.
type MemLeakFormatter struct{}

// SupportedTypes returns the data types this formatter supports.
func (f *MemLeakFormatter) SupportedTypes() []model.AnalysisDataType {
	return []model.AnalysisDataType{model.DataTypeMemoryLeak}
}

// Format outputs the memory leak analysis result to the logger.
func (f *MemLeakFormatter) Format(resp *model.AnalysisResponse, log utils.Logger) {
	log.Info("=== Memory Leak Analysis Results ===")
	log.Info("Task UUID:      %s", resp.TaskUUID)
	log.Info("Task Type:      %s", resp.TaskType.String())
	log.Info("")

	data, ok := resp.Data.(*model.MemoryLeakData)
	if !ok {
		log.Info("(No detailed data available)")
		return
	}

	// Print leak summary
	log.Info("=== Leak Summary ===")
	log.Info("  Total Leak Bytes: %s (%d bytes)", formatBytes(data.TotalLeakBytes), data.TotalLeakBytes)
	log.Info("  Total Leak Count: %d", data.TotalLeakCount)
	log.Info("  Suspect Count:    %d", len(data.LeakSuspects))
	log.Info("")

	// Print leak suspects
	log.Info("=== Leak Suspects ===")
	topItems := data.TopItems()
	count := min(10, len(topItems))
	for i := 0; i < count; i++ {
		item := topItems[i]
		log.Info("  %2d. %6.2f%%  %s", i+1, item.Percentage, truncateString(item.Name, 70))
		log.Info("              Leaked: %s", formatBytes(item.Value))
		if desc, ok := item.Extra["description"]; ok && desc != "" {
			log.Info("              %s", desc)
		}
	}
	log.Info("")

	// Print output files
	f.printOutputFiles(resp, log)

	// Print suggestions
	f.printSuggestions(resp, log)
}

// FormatSummary returns a summary map for serialization.
func (f *MemLeakFormatter) FormatSummary(resp *model.AnalysisResponse) map[string]interface{} {
	summary := map[string]interface{}{
		"task_uuid":     resp.TaskUUID,
		"task_type":     resp.TaskType.String(),
		"total_records": resp.TotalRecords,
	}

	if resp.Data != nil {
		summary["data"] = resp.Data.Summary()
		summary["top_items"] = resp.Data.TopItems()
	}

	summary["output_files"] = resp.OutputFiles
	summary["suggestions_count"] = len(resp.Suggestions)

	return summary
}

func (f *MemLeakFormatter) printOutputFiles(resp *model.AnalysisResponse, log utils.Logger) {
	log.Info("=== Output Files ===")
	for _, file := range resp.OutputFiles {
		log.Info("  %s: %s", file.Name, file.LocalPath)
		if info, err := os.Stat(file.LocalPath); err == nil {
			log.Info("    Size: %d bytes", info.Size())
		}
	}
}

func (f *MemLeakFormatter) printSuggestions(resp *model.AnalysisResponse, log utils.Logger) {
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
