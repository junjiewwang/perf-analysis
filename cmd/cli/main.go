package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/perf-analysis/internal/analyzer"
	"github.com/perf-analysis/internal/webui"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

var (
	inputFile    = flag.String("i", "", "Input collapsed data file (required for analysis)")
	outputDir    = flag.String("o", "./output", "Output directory for generated files")
	taskType     = flag.String("t", "java", "Task type: java, generic, pprof")
	profilerType = flag.String("p", "perf", "Profiler type: perf, async_alloc, pprof")
	taskUUID     = flag.String("uuid", "", "Task UUID (auto-generated if empty)")
	topN         = flag.Int("n", 50, "Number of top functions to report")
	verbose      = flag.Bool("v", false, "Verbose output")
	showHelp     = flag.Bool("h", false, "Show help message")

	// Serve mode flags
	serveMode   = flag.Bool("serve", false, "Start web server to view analysis results")
	serveDir    = flag.String("d", "", "Data directory for serve mode (defaults to output dir)")
	servePort   = flag.Int("port", 8080, "Port for web server")
	openBrowser = flag.Bool("open", false, "Open browser automatically after starting server")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `perf-analysis-cli - Analyze profiling data from local files

Usage:
  perf-analysis-cli -i <input_file> [options]
  perf-analysis-cli -serve [-d <data_dir>] [-port <port>]

Examples:
  # Analyze Java CPU profiling data
  perf-analysis-cli -i ./test/origin.data -o ./output -t java -p perf

  # Analyze with verbose output
  perf-analysis-cli -i ./test/origin.data -o ./output -v

  # Analyze memory allocation data
  perf-analysis-cli -i ./alloc.data -t java -p async_alloc

  # Start web server to view results
  perf-analysis-cli -serve -d ./output -port 8080

  # Analyze and immediately view results
  perf-analysis-cli -i ./test/origin.data -serve

Options:
`)
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	// Setup logger
	logLevel := utils.LevelInfo
	if *verbose {
		logLevel = utils.LevelDebug
	}
	logger := utils.NewDefaultLogger(logLevel, os.Stdout)

	// Determine mode: serve only or analyze (optionally followed by serve)
	if *serveMode && *inputFile == "" {
		// Serve only mode
		runServeMode(logger)
		return
	}

	if *inputFile == "" {
		fmt.Fprintln(os.Stderr, "Error: input file is required")
		fmt.Fprintln(os.Stderr, "Use -h for help")
		os.Exit(1)
	}

	// Validate input file
	if _, err := os.Stat(*inputFile); os.IsNotExist(err) {
		logger.Error("Input file not found: %s", *inputFile)
		os.Exit(1)
	}

	// Generate task UUID if not provided
	uuid := *taskUUID
	if uuid == "" {
		uuid = generateUUID()
	}

	// Parse task type
	tt, err := parseTaskType(*taskType)
	if err != nil {
		logger.Error("Invalid task type: %v", err)
		os.Exit(1)
	}

	// Parse profiler type
	pt, err := parseProfilerType(*profilerType)
	if err != nil {
		logger.Error("Invalid profiler type: %v", err)
		os.Exit(1)
	}

	// Create output directory
	taskOutputDir := filepath.Join(*outputDir, uuid)
	if err := os.MkdirAll(taskOutputDir, 0755); err != nil {
		logger.Error("Failed to create output directory: %v", err)
		os.Exit(1)
	}

	logger.Info("=== Perf Analysis CLI ===")
	logger.Info("Input file:    %s", *inputFile)
	logger.Info("Output dir:    %s", taskOutputDir)
	logger.Info("Task type:     %s (%d)", *taskType, tt)
	logger.Info("Profiler type: %s (%d)", *profilerType, pt)
	logger.Info("Task UUID:     %s", uuid)
	logger.Info("")

	// Create analyzer configuration
	config := &analyzer.BaseAnalyzerConfig{
		OutputDir: *outputDir,
		TopFuncsN: *topN,
	}

	// Create analyzer using factory
	factory := analyzer.NewFactory(config)
	ana, err := factory.CreateAnalyzer(tt, pt)
	if err != nil {
		logger.Error("Failed to create analyzer: %v", err)
		os.Exit(1)
	}

	logger.Info("Using analyzer: %s", ana.Name())
	logger.Info("")

	// Create analysis request
	req := &model.AnalysisRequest{
		TaskID:       1,
		TaskUUID:     uuid,
		TaskType:     tt,
		ProfilerType: pt,
		InputFile:    *inputFile,
		OutputDir:    taskOutputDir,
	}

	// Run analysis
	logger.Info("Starting analysis...")
	ctx := context.Background()
	result, err := ana.Analyze(ctx, req)
	if err != nil {
		logger.Error("Analysis failed: %v", err)
		os.Exit(1)
	}

	logger.Info("Analysis completed successfully!")
	logger.Info("")

	// Print results
	printResults(logger, result, taskOutputDir)

	// Save result summary
	saveSummary(result, taskOutputDir)

	logger.Info("")
	logger.Info("=== Analysis Complete ===")
	logger.Info("Output files are in: %s", taskOutputDir)

	// If serve mode is enabled, start the web server
	if *serveMode {
		logger.Info("")
		logger.Info("Starting web server...")
		*serveDir = *outputDir
		runServeMode(logger)
	}
}

// runServeMode starts the web server to view analysis results
func runServeMode(logger utils.Logger) {
	dataDir := *serveDir
	if dataDir == "" {
		dataDir = *outputDir
	}

	// Verify data directory exists
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		logger.Error("Data directory not found: %s", dataDir)
		os.Exit(1)
	}

	server := webui.NewServer(dataDir, *servePort, logger)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("\nShutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5)
		defer cancel()
		server.Shutdown(ctx)
		os.Exit(0)
	}()

	// Print access URL
	logger.Info("")
	logger.Info("╔════════════════════════════════════════════════════════╗")
	logger.Info("║  🔥 Perf Analysis Viewer                               ║")
	logger.Info("║                                                        ║")
	logger.Info("║  Open in browser: http://localhost:%d                ║", *servePort)
	logger.Info("║  Data directory:  %-36s ║", truncateString(dataDir, 36))
	logger.Info("║                                                        ║")
	logger.Info("║  Press Ctrl+C to stop                                  ║")
	logger.Info("╚════════════════════════════════════════════════════════╝")
	logger.Info("")

	if err := server.Start(); err != nil {
		logger.Error("Server error: %v", err)
		os.Exit(1)
	}
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
		return 0, fmt.Errorf("unknown task type: %s", s)
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
		return 0, fmt.Errorf("unknown profiler type: %s", s)
	}
}

func generateUUID() string {
	// Simple UUID generation
	return fmt.Sprintf("local-%d", os.Getpid())
}

func printResults(logger utils.Logger, result *model.AnalysisResponse, outputDir string) {
	logger.Info("=== Analysis Results ===")
	logger.Info("Task UUID:      %s", result.TaskUUID)
	logger.Info("Total Samples:  %d", result.TotalRecords)
	logger.Info("")

	// Print top functions
	logger.Info("=== Top Functions ===")
	var topFuncs map[string]interface{}
	if err := json.Unmarshal([]byte(result.TopFuncs), &topFuncs); err == nil {
		// Sort and print top 10
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
		// Simple bubble sort for top 10
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
			logger.Info("  %2d. %6.2f%%  %s", i+1, entries[i].Percent, truncateString(entries[i].Name, 80))
		}
	}
	logger.Info("")

	// Print thread statistics
	logger.Info("=== Thread Statistics ===")
	var threads []map[string]interface{}
	if err := json.Unmarshal([]byte(result.ActiveThreadsJSON), &threads); err == nil {
		count := 5
		if len(threads) < count {
			count = len(threads)
		}
		for i := 0; i < count; i++ {
			t := threads[i]
			logger.Info("  Thread: %v, Samples: %v (%.2f%%)",
				t["thread_name"], t["samples"], t["percentage"])
		}
	}
	logger.Info("")

	// Print output files
	logger.Info("=== Output Files ===")
	logger.Info("  Flame Graph: %s", result.FlameGraphFile)
	logger.Info("  Call Graph:  %s", result.CallGraphFile)

	// Print file sizes
	if info, err := os.Stat(result.FlameGraphFile); err == nil {
		logger.Info("  Flame Graph Size: %d bytes", info.Size())
	}
	if info, err := os.Stat(result.CallGraphFile); err == nil {
		logger.Info("  Call Graph Size: %d bytes", info.Size())
	}

	// Print suggestions
	if len(result.Suggestions) > 0 {
		logger.Info("")
		logger.Info("=== Suggestions ===")
		for i, sug := range result.Suggestions {
			if i >= 5 {
				logger.Info("  ... and %d more suggestions", len(result.Suggestions)-5)
				break
			}
			logger.Info("  - %s", truncateString(sug.Suggestion, 100))
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
