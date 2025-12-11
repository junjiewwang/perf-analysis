package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perf-analysis/internal/analyzer"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

var (
	// Analyze command flags
	inputFile    string
	outputDir    string
	taskType     string
	profilerType string
	taskUUID     string
	topN         int
	serveAfter   bool
	servePort    int
)

// analyzeCmd represents the analyze command
var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze profiling data from input file",
	Long: `Analyze profiling data and generate flame graphs, call graphs, and suggestions.

The analyze command processes collapsed stack trace data and generates:
  - Flame graph data (JSON format, gzipped)
  - Call graph data (JSON format)
  - Performance summary with top functions
  - Optimization suggestions

Supported task types:
  - java     : Java application profiling (default)
  - generic  : Generic application profiling
  - tracing  : Tracing data analysis
  - timing   : Timing analysis
  - memleak  : Memory leak detection
  - pprof_mem: Go pprof memory profiling
  - java_heap: Java heap analysis

Supported profiler types:
  - perf       : Linux perf profiler (default)
  - async_alloc: async-profiler allocation mode
  - pprof      : Go pprof profiler`,
	RunE: runAnalyze,
}

func init() {
	rootCmd.AddCommand(analyzeCmd)

	// Set dynamic example using actual binary name
	binName := BinName()
	analyzeCmd.Example = `  # Analyze Java CPU profiling data
  ` + binName + ` analyze -i ./test/origin.data -o ./output -t java -p perf

  # Analyze memory allocation data with verbose output
  ` + binName + ` analyze -i ./alloc.data -t java -p async_alloc -v

  # Analyze and immediately start web server to view results
  ` + binName + ` analyze -i ./test/origin.data --serve --port 8080

  # Specify custom task UUID
  ` + binName + ` analyze -i ./data.txt --uuid my-analysis-001`

	// Input/Output flags
	analyzeCmd.Flags().StringVarP(&inputFile, "input", "i", "", "Input collapsed data file (required)")
	analyzeCmd.Flags().StringVarP(&outputDir, "output", "o", "./output", "Output directory for generated files")
	analyzeCmd.MarkFlagRequired("input")

	// Analysis configuration flags
	analyzeCmd.Flags().StringVarP(&taskType, "type", "t", "java", "Task type: java, generic, tracing, timing, memleak, pprof_mem, java_heap")
	analyzeCmd.Flags().StringVarP(&profilerType, "profiler", "p", "perf", "Profiler type: perf, async_alloc, pprof")
	analyzeCmd.Flags().StringVar(&taskUUID, "uuid", "", "Task UUID (auto-generated if empty)")
	analyzeCmd.Flags().IntVarP(&topN, "top", "n", 50, "Number of top functions to report")

	// Serve flags
	analyzeCmd.Flags().BoolVar(&serveAfter, "serve", false, "Start web server after analysis")
	analyzeCmd.Flags().IntVar(&servePort, "port", 8080, "Port for web server (used with --serve)")
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	log := GetLogger()

	// Validate input file
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputFile)
	}

	// Generate task UUID if not provided
	uuid := taskUUID
	if uuid == "" {
		uuid = generateUUID()
	}

	// Parse task type
	tt, err := parseTaskType(taskType)
	if err != nil {
		return fmt.Errorf("invalid task type: %w", err)
	}

	// Parse profiler type
	pt, err := parseProfilerType(profilerType)
	if err != nil {
		return fmt.Errorf("invalid profiler type: %w", err)
	}

	// Create output directory
	taskOutputDir := filepath.Join(outputDir, uuid)
	if err := os.MkdirAll(taskOutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	log.Info("=== Perf Analysis CLI ===")
	log.Info("Input file:    %s", inputFile)
	log.Info("Output dir:    %s", taskOutputDir)
	log.Info("Task type:     %s (%d)", taskType, tt)
	log.Info("Profiler type: %s (%d)", profilerType, pt)
	log.Info("Task UUID:     %s", uuid)
	log.Info("")

	// Create analyzer configuration
	config := &analyzer.BaseAnalyzerConfig{
		OutputDir: outputDir,
		TopFuncsN: topN,
	}

	// Create analyzer using factory
	factory := analyzer.NewFactory(config)
	ana, err := factory.CreateAnalyzer(tt, pt)
	if err != nil {
		return fmt.Errorf("failed to create analyzer: %w", err)
	}

	log.Info("Using analyzer: %s", ana.Name())
	log.Info("")

	// Create analysis request
	req := &model.AnalysisRequest{
		TaskID:       1,
		TaskUUID:     uuid,
		TaskType:     tt,
		ProfilerType: pt,
		InputFile:    inputFile,
		OutputDir:    taskOutputDir,
	}

	// Run analysis
	log.Info("Starting analysis...")
	ctx := context.Background()
	result, err := ana.Analyze(ctx, req)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	log.Info("Analysis completed successfully!")
	log.Info("")

	// Print results
	printResults(log, result)

	// Save result summary
	saveSummary(result, taskOutputDir)

	log.Info("")
	log.Info("=== Analysis Complete ===")
	log.Info("Output files are in: %s", taskOutputDir)

	// If serve mode is enabled, start the web server
	if serveAfter {
		log.Info("")
		log.Info("Starting web server...")
		return startServeMode(outputDir, servePort, log)
	}

	return nil
}

func parseTaskType(s string) (model.TaskType, error) {
	switch strings.ToLower(s) {
	case "generic", "0":
		return model.TaskTypeGeneric, nil
	case "java", "1":
		return model.TaskTypeJava, nil
	case "tracing", "2":
		return model.TaskTypeTracing, nil
	case "timing", "3":
		return model.TaskTypeTiming, nil
	case "memleak", "4":
		return model.TaskTypeMemLeak, nil
	case "pprof_mem", "5":
		return model.TaskTypePProfMem, nil
	case "java_heap", "6":
		return model.TaskTypeJavaHeap, nil
	default:
		return 0, fmt.Errorf("unknown task type: %s (valid: java, generic, tracing, timing, memleak, pprof_mem, java_heap)", s)
	}
}

func parseProfilerType(s string) (model.ProfilerType, error) {
	switch strings.ToLower(s) {
	case "perf", "0":
		return model.ProfilerTypePerf, nil
	case "async_alloc", "1":
		return model.ProfilerTypeAsyncAlloc, nil
	case "pprof", "2":
		return model.ProfilerTypePProf, nil
	default:
		return 0, fmt.Errorf("unknown profiler type: %s (valid: perf, async_alloc, pprof)", s)
	}
}

func generateUUID() string {
	return fmt.Sprintf("local-%d", os.Getpid())
}

func printResults(log utils.Logger, result *model.AnalysisResponse) {
	log.Info("=== Analysis Results ===")
	log.Info("Task UUID:      %s", result.TaskUUID)
	log.Info("Total Samples:  %d", result.TotalRecords)
	log.Info("")

	// Print top functions
	log.Info("=== Top Functions ===")
	var topFuncs map[string]interface{}
	if err := json.Unmarshal([]byte(result.TopFuncs), &topFuncs); err == nil {
		type funcEntry struct {
			Name    string
			Percent float64
		}
		entries := make([]funcEntry, 0, len(topFuncs))
		for name, val := range topFuncs {
			if m, ok := val.(map[string]interface{}); ok {
				if self, ok := m["self"].(float64); ok {
					entries = append(entries, funcEntry{Name: name, Percent: self})
				}
			}
		}
		// Sort by percentage (descending)
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[j].Percent > entries[i].Percent {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}
		// Print top 10
		count := 10
		if len(entries) < count {
			count = len(entries)
		}
		for i := 0; i < count; i++ {
			log.Info("  %2d. %6.2f%%  %s", i+1, entries[i].Percent, truncateString(entries[i].Name, 80))
		}
	}
	log.Info("")

	// Print thread statistics
	log.Info("=== Thread Statistics ===")
	var threads []map[string]interface{}
	if err := json.Unmarshal([]byte(result.ActiveThreadsJSON), &threads); err == nil {
		count := 5
		if len(threads) < count {
			count = len(threads)
		}
		for i := 0; i < count; i++ {
			t := threads[i]
			log.Info("  Thread: %v, Samples: %v (%.2f%%)",
				t["thread_name"], t["samples"], t["percentage"])
		}
	}
	log.Info("")

	// Print output files
	log.Info("=== Output Files ===")
	log.Info("  Flame Graph: %s", result.FlameGraphFile)
	log.Info("  Call Graph:  %s", result.CallGraphFile)

	// Print file sizes
	if info, err := os.Stat(result.FlameGraphFile); err == nil {
		log.Info("  Flame Graph Size: %d bytes", info.Size())
	}
	if info, err := os.Stat(result.CallGraphFile); err == nil {
		log.Info("  Call Graph Size: %d bytes", info.Size())
	}

	// Print suggestions
	if len(result.Suggestions) > 0 {
		log.Info("")
		log.Info("=== Suggestions ===")
		for i, sug := range result.Suggestions {
			if i >= 5 {
				log.Info("  ... and %d more suggestions", len(result.Suggestions)-5)
				break
			}
			log.Info("  - %s", truncateString(sug.Suggestion, 100))
		}
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func saveSummary(result *model.AnalysisResponse, outputDir string) {
	summary := map[string]interface{}{
		"task_uuid":     result.TaskUUID,
		"total_records": result.TotalRecords,
		"top_funcs":     json.RawMessage(result.TopFuncs),
		"threads":       json.RawMessage(result.ActiveThreadsJSON),
		"files": map[string]string{
			"flamegraph": result.FlameGraphFile,
			"callgraph":  result.CallGraphFile,
		},
		"suggestions_count": len(result.Suggestions),
	}

	summaryFile := filepath.Join(outputDir, "summary.json")
	data, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile(summaryFile, data, 0644)
}
