package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/perf-analysis/internal/analyzer"
	"github.com/perf-analysis/internal/formatter"
	"github.com/perf-analysis/pkg/model"
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
	startTime := time.Now()
	result, err := ana.Analyze(ctx, req)
	analysisTime := time.Since(startTime)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	log.Info("Analysis completed successfully!")
	log.Info("")

	// Print results
	printResults(log, result)

	// Save result summary with metadata
	metadata := &AnalysisMetadata{
		TaskType:       int(tt),
		TaskTypeName:   tt.String(),
		ProfilerType:   int(pt),
		ProfilerName:   pt.String(),
		InputFile:      filepath.Base(inputFile),
		CreatedAt:      startTime.Format(time.RFC3339),
		AnalysisTimeMs: analysisTime.Milliseconds(),
	}
	saveSummary(result, taskOutputDir, metadata)

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

func printResults(log interface{ Info(format string, args ...interface{}) }, result *model.AnalysisResponse) {
	// Use the formatter registry to format results based on data type
	registry := formatter.NewRegistry()
	registry.Format(result, GetLogger())
}

func saveSummary(result *model.AnalysisResponse, outputDir string, metadata *AnalysisMetadata) {
	// Use the formatter registry to generate summary
	registry := formatter.NewRegistry()
	summary := registry.FormatSummary(result)

	// Add metadata if provided
	if metadata != nil {
		summary["metadata"] = map[string]interface{}{
			"task_type":        metadata.TaskType,
			"task_type_name":   metadata.TaskTypeName,
			"profiler_type":    metadata.ProfilerType,
			"profiler_name":    metadata.ProfilerName,
			"input_file":       metadata.InputFile,
			"created_at":       metadata.CreatedAt,
			"analysis_time_ms": metadata.AnalysisTimeMs,
		}
	}

	summaryFile := filepath.Join(outputDir, "summary.json")
	data, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile(summaryFile, data, 0644)

	// For heap analysis, write detailed retainer data to separate file
	if result.Data != nil && result.Data.Type() == model.DataTypeHeapDump {
		heapFormatter := &formatter.HeapFormatter{}
		if err := heapFormatter.WriteDetailedRetainers(result, outputDir); err != nil {
			// Log but don't fail - detailed file is optional
			GetLogger().Warn("Failed to write detailed retainer file: %v", err)
		}
	}
}

// AnalysisMetadata holds metadata about the analysis task
type AnalysisMetadata struct {
	TaskType       int    `json:"task_type"`
	TaskTypeName   string `json:"task_type_name"`
	ProfilerType   int    `json:"profiler_type"`
	ProfilerName   string `json:"profiler_name"`
	InputFile      string `json:"input_file"`
	CreatedAt      string `json:"created_at"`
	AnalysisTimeMs int64  `json:"analysis_time_ms"`
}
