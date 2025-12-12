package formatter

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// HeapFormatter formats Java heap dump analysis results.
type HeapFormatter struct{}

// SupportedTypes returns the data types this formatter supports.
func (f *HeapFormatter) SupportedTypes() []model.AnalysisDataType {
	return []model.AnalysisDataType{model.DataTypeHeapDump}
}

// Format outputs the heap analysis result to the logger.
func (f *HeapFormatter) Format(resp *model.AnalysisResponse, log utils.Logger) {
	log.Info("=== Heap Analysis Results ===")
	log.Info("Task UUID:      %s", resp.TaskUUID)
	log.Info("Task Type:      %s", resp.TaskType.String())
	log.Info("")

	data, ok := resp.Data.(*model.HeapAnalysisData)
	if !ok {
		log.Info("(No detailed data available)")
		return
	}

	// Print heap summary
	log.Info("=== Heap Summary ===")
	if data.Format != "" {
		log.Info("  Format:          %s", data.Format)
	}
	log.Info("  Total Classes:   %d", data.TotalClasses)
	log.Info("  Total Instances: %d", data.TotalInstances)
	log.Info("  Total Heap Size: %s (%d bytes)", data.HeapSizeHuman, data.TotalHeapSize)
	if data.LiveBytes > 0 {
		log.Info("  Live Bytes:      %d", data.LiveBytes)
		log.Info("  Live Objects:    %d", data.LiveObjects)
	}
	log.Info("")

	// Print top classes (class histogram)
	log.Info("=== Top Classes by Memory ===")
	topItems := data.TopItems()
	count := min(10, len(topItems))
	for i := 0; i < count; i++ {
		item := topItems[i]
		instanceCount := int64(0)
		if extra, ok := item.Extra["instance_count"]; ok {
			if ic, ok := extra.(int64); ok {
				instanceCount = ic
			}
		}
		log.Info("  %2d. %6.2f%%  %s", i+1, item.Percentage, truncateString(item.Name, 60))
		log.Info("              Size: %s, Instances: %d", formatBytes(item.Value), instanceCount)
	}
	log.Info("")

	// Print business retainers summary (root cause analysis)
	if data.BusinessRetainers != nil && len(data.BusinessRetainers) > 0 {
		log.Info("=== Root Cause Analysis (Business Retainers) ===")
		printed := 0
		for className, retainers := range data.BusinessRetainers {
			if printed >= 5 {
				log.Info("  ... and %d more classes with business retainers", len(data.BusinessRetainers)-5)
				break
			}
			log.Info("  %s:", truncateString(className, 50))
			for i, r := range retainers {
				if i >= 3 {
					log.Info("    ... and %d more retainers", len(retainers)-3)
					break
				}
				log.Info("    - %s (%.1f%%, %s)", truncateString(r.ClassName, 40), r.Percentage, formatBytes(r.RetainedSize))
			}
			printed++
		}
		log.Info("")
	}

	// Print output files
	f.printOutputFiles(resp, log)

	// Print suggestions
	f.printSuggestions(resp, log)
}

// FormatSummary returns a lightweight summary map for serialization.
// Detailed retainer data is written to a separate file.
func (f *HeapFormatter) FormatSummary(resp *model.AnalysisResponse) map[string]interface{} {
	summary := map[string]interface{}{
		"task_uuid":     resp.TaskUUID,
		"task_type":     resp.TaskType.String(),
		"total_records": resp.TotalRecords,
	}

	if resp.Data != nil {
		heapData, ok := resp.Data.(*model.HeapAnalysisData)
		if !ok {
			summary["data"] = resp.Data.Summary()
			summary["top_items"] = resp.Data.TopItems()
			return summary
		}

		// Create lightweight overview (no detailed retainer data)
		overview := map[string]interface{}{
			"format":          heapData.Format,
			"id_size":         heapData.IDSize,
			"timestamp":       heapData.Timestamp,
			"total_classes":   heapData.TotalClasses,
			"total_instances": heapData.TotalInstances,
			"total_heap_size": heapData.TotalHeapSize,
			"heap_size_human": heapData.HeapSizeHuman,
			"live_bytes":      heapData.LiveBytes,
			"live_objects":    heapData.LiveObjects,
		}

		// Create lightweight top_classes (only basic info, no retainers)
		lightweightClasses := make([]map[string]interface{}, 0, len(heapData.TopClasses))
		for i, cls := range heapData.TopClasses {
			if i >= 20 { // Only include top 20 in summary
				break
			}
			lightweightClasses = append(lightweightClasses, map[string]interface{}{
				"class_name":     cls.ClassName,
				"instance_count": cls.InstanceCount,
				"total_size":     cls.TotalSize,
				"percentage":     cls.Percentage,
				"retained_size":  cls.RetainedSize,
				"has_retainers":  len(cls.Retainers) > 0,
				"has_gc_paths":   len(cls.GCRootPaths) > 0,
			})
		}
		overview["top_classes"] = lightweightClasses

		// Create business retainers summary (lightweight)
		if heapData.BusinessRetainers != nil && len(heapData.BusinessRetainers) > 0 {
			retainerSummary := make(map[string]interface{})
			for className, retainers := range heapData.BusinessRetainers {
				// Only include summary info
				retainerSummary[className] = map[string]interface{}{
					"count":        len(retainers),
					"has_gc_roots": f.hasGCRoots(retainers),
				}
			}
			overview["business_retainers_summary"] = retainerSummary
		}

		summary["data"] = overview

		// Create lightweight top_items (only top 15)
		allItems := resp.Data.TopItems()
		topItems := allItems
		if len(allItems) > 15 {
			topItems = allItems[:15]
		}
		summary["top_items"] = topItems

		// Add file references for detailed data
		summary["detail_files"] = map[string]string{
			"retainer_analysis": "retainer_analysis.json",
			"heap_report":       heapData.HeapReportFile,
			"histogram":         heapData.HistogramFile,
		}
	}

	summary["output_files"] = resp.OutputFiles
	summary["suggestions"] = resp.Suggestions // Include suggestions in summary

	return summary
}

// FormatDetailedRetainers generates detailed retainer analysis data.
// This should be called separately to write to retainer_analysis.json.
func (f *HeapFormatter) FormatDetailedRetainers(resp *model.AnalysisResponse) map[string]interface{} {
	heapData, ok := resp.Data.(*model.HeapAnalysisData)
	if !ok {
		return nil
	}

	detailed := map[string]interface{}{
		"task_uuid": resp.TaskUUID,
	}

	// Full top_classes with retainers
	detailed["top_classes"] = heapData.TopClasses

	// Full business retainers
	if heapData.BusinessRetainers != nil {
		detailed["business_retainers"] = heapData.BusinessRetainers
	}

	// Reference graphs for visualization
	if heapData.ReferenceGraphs != nil {
		detailed["reference_graphs"] = heapData.ReferenceGraphs
	}

	return detailed
}

// WriteDetailedRetainers writes detailed retainer analysis to a separate file.
func (f *HeapFormatter) WriteDetailedRetainers(resp *model.AnalysisResponse, outputDir string) error {
	detailed := f.FormatDetailedRetainers(resp)
	if detailed == nil {
		return nil
	}

	retainerFile := filepath.Join(outputDir, "retainer_analysis.json")
	data, err := json.MarshalIndent(detailed, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(retainerFile, data, 0644)
}

// hasGCRoots checks if any retainer is a GC root.
func (f *HeapFormatter) hasGCRoots(retainers []model.HeapBusinessRetainer) bool {
	for _, r := range retainers {
		if r.IsGCRoot {
			return true
		}
	}
	return false
}

func (f *HeapFormatter) printOutputFiles(resp *model.AnalysisResponse, log utils.Logger) {
	log.Info("=== Output Files ===")
	for _, file := range resp.OutputFiles {
		log.Info("  %s: %s", file.Name, file.LocalPath)
		if info, err := os.Stat(file.LocalPath); err == nil {
			log.Info("    Size: %d bytes", info.Size())
		}
	}
}

func (f *HeapFormatter) printSuggestions(resp *model.AnalysisResponse, log utils.Logger) {
	if len(resp.Suggestions) > 0 {
		log.Info("")
		log.Info("=== Optimization Suggestions ===")
		for i, sug := range resp.Suggestions {
			if i >= 5 {
				log.Info("  ... and %d more suggestions", len(resp.Suggestions)-5)
				break
			}
			log.Info("  - %s", truncateString(sug.Suggestion, 100))
		}
	}
}
