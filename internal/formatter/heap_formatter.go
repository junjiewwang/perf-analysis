package formatter

import (
	"encoding/json"
	"fmt"
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

		// Create top_classes with retainer info for visualization
		topClassesData := make([]map[string]interface{}, 0, len(heapData.TopClasses))
		for i, cls := range heapData.TopClasses {
			if i >= 20 { // Only include top 20 in summary
				break
			}
			classInfo := map[string]interface{}{
				"class_name":     cls.ClassName,
				"instance_count": cls.InstanceCount,
				"total_size":     cls.TotalSize,
				"percentage":     cls.Percentage,
				"retained_size":  cls.RetainedSize,
				"has_retainers":  len(cls.Retainers) > 0,
				"has_gc_paths":   len(cls.GCRootPaths) > 0,
				"is_business":    f.isBusinessClass(cls.ClassName),
			}
			// Include retainers for top 10 classes
			if i < 10 && len(cls.Retainers) > 0 {
				classInfo["retainers"] = cls.Retainers
			}
			// Include GC root paths for top 10 classes
			if i < 10 && len(cls.GCRootPaths) > 0 {
				classInfo["gc_root_paths"] = cls.GCRootPaths
			}
			topClassesData = append(topClassesData, classInfo)
		}
		overview["top_classes"] = topClassesData

		// Include full business retainers data for root cause analysis
		if heapData.BusinessRetainers != nil && len(heapData.BusinessRetainers) > 0 {
			overview["business_retainers"] = heapData.BusinessRetainers
		}

		// Include reference graphs for visualization
		if heapData.ReferenceGraphs != nil && len(heapData.ReferenceGraphs) > 0 {
			overview["reference_graphs"] = heapData.ReferenceGraphs
		}

		// Generate quick diagnosis for root cause analysis
		overview["quick_diagnosis"] = f.generateQuickDiagnosis(heapData)

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

// isBusinessClass checks if a class is likely a business/application class.
func (f *HeapFormatter) isBusinessClass(className string) bool {
	// Primitive arrays are not business classes
	primitiveArrays := []string{"byte[]", "int[]", "char[]", "long[]", "short[]", "boolean[]", "float[]", "double[]"}
	for _, arr := range primitiveArrays {
		if className == arr {
			return false
		}
	}

	// JDK classes
	jdkPrefixes := []string{
		"java.", "javax.", "sun.", "com.sun.", "jdk.",
		"[", // arrays
	}
	for _, prefix := range jdkPrefixes {
		if len(className) >= len(prefix) && className[:len(prefix)] == prefix {
			return false
		}
	}

	// Common framework classes
	frameworkPrefixes := []string{
		"org.springframework.", "org.apache.", "org.hibernate.",
		"com.google.", "io.netty.", "org.slf4j.", "ch.qos.logback.",
		"com.fasterxml.", "org.aspectj.", "org.jboss.",
		"io.micrometer.", "reactor.", "rx.", "akka.",
		"io.opentelemetry.", "net.bytebuddy.",
	}
	for _, prefix := range frameworkPrefixes {
		if len(className) >= len(prefix) && className[:len(prefix)] == prefix {
			return false
		}
	}

	return true
}

// generateQuickDiagnosis generates a quick diagnosis summary for root cause analysis.
func (f *HeapFormatter) generateQuickDiagnosis(heapData *model.HeapAnalysisData) map[string]interface{} {
	diagnosis := map[string]interface{}{}

	// 1. Find top business classes (non-JDK, non-framework)
	var businessClasses []map[string]interface{}
	for _, cls := range heapData.TopClasses {
		if f.isBusinessClass(cls.ClassName) {
			businessClasses = append(businessClasses, map[string]interface{}{
				"class_name":     cls.ClassName,
				"total_size":     cls.TotalSize,
				"instance_count": cls.InstanceCount,
				"percentage":     cls.Percentage,
				"retained_size":  cls.RetainedSize,
			})
			if len(businessClasses) >= 10 {
				break
			}
		}
	}
	diagnosis["top_business_classes"] = businessClasses

	// 2. Identify leak suspects based on patterns
	var leakSuspects []map[string]interface{}
	for _, cls := range heapData.TopClasses {
		suspect := f.analyzeLeakSuspect(cls, heapData.TotalHeapSize)
		if suspect != nil {
			leakSuspects = append(leakSuspects, suspect)
			if len(leakSuspects) >= 5 {
				break
			}
		}
	}
	diagnosis["leak_suspects"] = leakSuspects

	// 3. Identify collection-based issues
	var collectionIssues []map[string]interface{}
	collectionPatterns := map[string]string{
		"HashMap":           "HashMap 可能存在过多条目或未清理",
		"ConcurrentHashMap": "ConcurrentHashMap 可能作为缓存未设置过期策略",
		"ArrayList":         "ArrayList 可能持续增长未清理",
		"LinkedList":        "LinkedList 可能作为队列未消费",
		"HashSet":           "HashSet 可能存在重复添加或未清理",
	}
	for _, cls := range heapData.TopClasses {
		for pattern, desc := range collectionPatterns {
			if containsPattern(cls.ClassName, pattern) && cls.InstanceCount > 1000 {
				collectionIssues = append(collectionIssues, map[string]interface{}{
					"class_name":     cls.ClassName,
					"instance_count": cls.InstanceCount,
					"total_size":     cls.TotalSize,
					"issue":          desc,
				})
				break
			}
		}
		if len(collectionIssues) >= 5 {
			break
		}
	}
	diagnosis["collection_issues"] = collectionIssues

	// 4. Generate action items
	diagnosis["action_items"] = f.generateActionItems(heapData, businessClasses, leakSuspects)

	return diagnosis
}

// analyzeLeakSuspect analyzes a class for potential memory leak patterns.
func (f *HeapFormatter) analyzeLeakSuspect(cls model.HeapClassStats, totalHeapSize int64) map[string]interface{} {
	var riskLevel string
	var reasons []string

	// High memory percentage
	if cls.Percentage > 15 {
		riskLevel = "high"
		reasons = append(reasons, "占用超过 15% 堆内存")
	} else if cls.Percentage > 8 {
		if riskLevel == "" {
			riskLevel = "medium"
		}
		reasons = append(reasons, "占用超过 8% 堆内存")
	}

	// High instance count for certain types
	if cls.InstanceCount > 50000 {
		if riskLevel == "" {
			riskLevel = "medium"
		}
		reasons = append(reasons, "实例数量过多 (>50000)")
	}

	// Byte arrays often indicate buffer leaks
	if cls.ClassName == "byte[]" && cls.Percentage > 20 {
		riskLevel = "high"
		reasons = append(reasons, "大量 byte[] 可能表示缓冲区泄漏或大对象问题")
	}

	// String accumulation
	if cls.ClassName == "java.lang.String" && cls.InstanceCount > 100000 {
		if riskLevel == "" {
			riskLevel = "medium"
		}
		reasons = append(reasons, "String 对象过多，检查字符串拼接和缓存")
	}

	if len(reasons) == 0 {
		return nil
	}

	return map[string]interface{}{
		"class_name":     cls.ClassName,
		"risk_level":     riskLevel,
		"reasons":        reasons,
		"total_size":     cls.TotalSize,
		"instance_count": cls.InstanceCount,
		"percentage":     cls.Percentage,
	}
}

// generateActionItems generates specific action items for diagnosis.
func (f *HeapFormatter) generateActionItems(heapData *model.HeapAnalysisData, businessClasses []map[string]interface{}, leakSuspects []map[string]interface{}) []map[string]interface{} {
	var actions []map[string]interface{}

	// Action 1: Check top business classes
	if len(businessClasses) > 0 {
		topBusiness := businessClasses[0]["class_name"].(string)
		actions = append(actions, map[string]interface{}{
			"priority": 1,
			"action":   "检查业务类",
			"detail":   "在 Class Histogram 中搜索 '" + getShortClassName(topBusiness) + "'，点击 'Root Causes' 查看持有者链",
			"target":   topBusiness,
		})
	}

	// Action 2: Check byte[] if it's top
	for _, cls := range heapData.TopClasses[:min(3, len(heapData.TopClasses))] {
		if cls.ClassName == "byte[]" && cls.Percentage > 10 {
			actions = append(actions, map[string]interface{}{
				"priority": 2,
				"action":   "分析 byte[] 来源",
				"detail":   "byte[] 占用 " + formatPercentage(cls.Percentage) + " 内存，在 Reference Graph 中选择 byte[] 查看哪些类持有这些数组",
				"target":   "byte[]",
			})
			break
		}
	}

	// Action 3: Check HashMap/ConcurrentHashMap
	for _, cls := range heapData.TopClasses {
		if (containsPattern(cls.ClassName, "HashMap") || containsPattern(cls.ClassName, "ConcurrentHashMap")) && cls.InstanceCount > 10000 {
			actions = append(actions, map[string]interface{}{
				"priority": 3,
				"action":   "检查 Map 缓存",
				"detail":   cls.ClassName + " 有 " + formatNumber(cls.InstanceCount) + " 个实例，检查是否有未清理的缓存",
				"target":   cls.ClassName,
			})
			break
		}
	}

	// Action 4: Check for GC root paths if available
	hasGCPaths := false
	for _, cls := range heapData.TopClasses[:min(5, len(heapData.TopClasses))] {
		if len(cls.GCRootPaths) > 0 {
			hasGCPaths = true
			break
		}
	}
	if hasGCPaths {
		actions = append(actions, map[string]interface{}{
			"priority": 4,
			"action":   "追踪 GC Root 路径",
			"detail":   "在 Reference Graph 中查看 GC Root Paths，找出阻止对象被回收的根引用",
			"target":   "",
		})
	}

	return actions
}

// Helper functions
func containsPattern(s, pattern string) bool {
	return len(s) >= len(pattern) && (s == pattern || 
		(len(s) > len(pattern) && (s[len(s)-len(pattern):] == pattern || 
		 findSubstring(s, pattern))))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func getShortClassName(fullName string) string {
	lastDot := -1
	for i := len(fullName) - 1; i >= 0; i-- {
		if fullName[i] == '.' {
			lastDot = i
			break
		}
	}
	if lastDot >= 0 && lastDot < len(fullName)-1 {
		return fullName[lastDot+1:]
	}
	return fullName
}

func formatPercentage(p float64) string {
	return fmt.Sprintf("%.1f%%", p)
}

func formatNumber(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
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
