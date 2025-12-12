package formatter

import (
	"os"

	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// DefaultFormatter is a fallback formatter for unknown data types.
type DefaultFormatter struct{}

// SupportedTypes returns an empty slice as this is a fallback formatter.
func (f *DefaultFormatter) SupportedTypes() []model.AnalysisDataType {
	return nil
}

// Format outputs a generic analysis result to the logger.
func (f *DefaultFormatter) Format(resp *model.AnalysisResponse, log utils.Logger) {
	log.Info("=== Analysis Results ===")
	log.Info("Task UUID:      %s", resp.TaskUUID)
	log.Info("Task Type:      %s", resp.TaskType.String())
	log.Info("Total Records:  %d", resp.TotalRecords)
	log.Info("")

	// Print data summary if available
	if resp.Data != nil {
		log.Info("=== Data Summary ===")
		log.Info("  Data Type: %s", resp.Data.Type())
		for k, v := range resp.Data.Summary() {
			log.Info("  %s: %v", k, v)
		}
		log.Info("")

		// Print top items
		topItems := resp.Data.TopItems()
		if len(topItems) > 0 {
			log.Info("=== Top Items ===")
			count := min(10, len(topItems))
			for i := 0; i < count; i++ {
				item := topItems[i]
				log.Info("  %2d. %6.2f%%  %s", i+1, item.Percentage, truncateString(item.Name, 80))
			}
			log.Info("")
		}
	}

	// Print output files
	if len(resp.OutputFiles) > 0 {
		log.Info("=== Output Files ===")
		for _, file := range resp.OutputFiles {
			log.Info("  %s: %s", file.Name, file.LocalPath)
			if info, err := os.Stat(file.LocalPath); err == nil {
				log.Info("    Size: %d bytes", info.Size())
			}
		}
		log.Info("")
	}

	// Print suggestions
	if len(resp.Suggestions) > 0 {
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

// FormatSummary returns a summary map for serialization.
func (f *DefaultFormatter) FormatSummary(resp *model.AnalysisResponse) map[string]interface{} {
	summary := map[string]interface{}{
		"task_uuid":     resp.TaskUUID,
		"task_type":     resp.TaskType.String(),
		"total_records": resp.TotalRecords,
	}

	if resp.Data != nil {
		summary["data_type"] = resp.Data.Type()
		summary["data"] = resp.Data.Summary()
		summary["top_items"] = resp.Data.TopItems()
	}

	summary["output_files"] = resp.OutputFiles
	summary["suggestions_count"] = len(resp.Suggestions)

	return summary
}
