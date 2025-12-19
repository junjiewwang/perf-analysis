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
	inputFile       string
	outputDir       string
	analysisMode    string
	analysisProfile string
	taskUUID        string
	topN            int
	serveAfter      bool
	servePort       int
)

// analyzeCmd represents the analyze command
var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze profiling data from input file",
	Long:  buildAnalyzeLongHelp(),
	RunE:  runAnalyze,
}

func init() {
	rootCmd.AddCommand(analyzeCmd)

	// Set dynamic example using actual binary name
	binName := BinName()
	analyzeCmd.Example = fmt.Sprintf(`  # Analyze Java CPU profiling data
  %s analyze -i ./test/origin.data -m java-cpu

  # Analyze Java memory allocation data
  %s analyze -i ./alloc.data -m java-alloc

  # Analyze Java heap dump
  %s analyze -i ./heap.hprof -m java-heap

  # Use detailed analysis profile for deep investigation
  %s analyze -i ./data.collapsed -m java-cpu --profile detailed

  # Quick analysis for fast results
  %s analyze -i ./data.collapsed -m java-cpu --profile quick

  # Analyze and start web server to view results
  %s analyze -i ./test/origin.data -m java-cpu --serve --port 8080

  # Specify custom output directory and task UUID
  %s analyze -i ./data.txt -m cpu -o ./results --uuid my-analysis-001`,
		binName, binName, binName, binName, binName, binName, binName)

	// Input/Output flags
	analyzeCmd.Flags().StringVarP(&inputFile, "input", "i", "", "Input profiling data file (required)")
	analyzeCmd.Flags().StringVarP(&outputDir, "output", "o", "./output", "Output directory for generated files")
	analyzeCmd.MarkFlagRequired("input")

	// Analysis mode flag (replaces type + profiler)
	analyzeCmd.Flags().StringVarP(&analysisMode, "mode", "m", "java-cpu",
		fmt.Sprintf("Analysis mode: %s", analyzer.ValidModes()))

	// Analysis profile flag (controls analysis depth)
	analyzeCmd.Flags().StringVar(&analysisProfile, "profile", "standard",
		"Analysis depth: quick (fast), standard (balanced), detailed (comprehensive)")

	// Other flags
	analyzeCmd.Flags().StringVar(&taskUUID, "uuid", "", "Task UUID (auto-generated if empty)")
	analyzeCmd.Flags().IntVarP(&topN, "top", "n", 50, "Number of top functions to report")

	// Serve flags
	analyzeCmd.Flags().BoolVar(&serveAfter, "serve", false, "Start web server after analysis")
	analyzeCmd.Flags().IntVar(&servePort, "port", 8080, "Port for web server (used with --serve)")
}

// buildAnalyzeLongHelp builds the long help message with mode descriptions.
func buildAnalyzeLongHelp() string {
	var sb strings.Builder
	sb.WriteString(`Analyze profiling data and generate flame graphs, call graphs, and suggestions.

The analyze command processes profiling data and generates:
  - Flame graph data (JSON format, gzipped)
  - Call graph data (JSON format, gzipped)
  - Performance summary with top functions
  - Optimization suggestions

`)
	sb.WriteString("Available analysis modes:\n")
	for _, info := range analyzer.AllModes() {
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", info.Mode, info.Description))
		sb.WriteString(fmt.Sprintf("               Input: %s\n", info.InputFormat))
	}

	sb.WriteString(`
Analysis profiles control the depth of analysis:
  quick      Fast analysis with minimal overhead (good for large files)
  standard   Balanced analysis with thread analysis (default)
  detailed   Comprehensive analysis for deep investigation`)

	return sb.String()
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	log := GetLogger()

	// Validate input file
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputFile)
	}

	// Parse analysis mode
	mode, err := analyzer.ParseMode(analysisMode)
	if err != nil {
		return err
	}

	// Parse analysis profile
	profile, err := parseAnalysisProfile(analysisProfile)
	if err != nil {
		return err
	}

	// Get mode info for display
	modeInfo := mode.Info()

	// Generate task UUID if not provided
	uuid := taskUUID
	if uuid == "" {
		uuid = generateUUID()
	}

	// Create output directory
	taskOutputDir := filepath.Join(outputDir, uuid)
	if err := os.MkdirAll(taskOutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	log.Info("=== Perf Analysis CLI ===")
	log.Info("Input file:    %s", inputFile)
	log.Info("Output dir:    %s", taskOutputDir)
	log.Info("Analysis mode: %s (%s)", mode, modeInfo.Description)
	log.Info("Profile:       %s", profile)
	log.Info("Task UUID:     %s", uuid)
	log.Info("")

	// Create analyzer configuration
	config := &analyzer.BaseAnalyzerConfig{
		OutputDir:       outputDir,
		TopFuncsN:       topN,
		Logger:          log,
		Verbose:         verbose,
		AnalysisProfile: profile,
	}

	// Create analyzer using factory
	factory := analyzer.NewFactory(config)
	ana, err := factory.CreateAnalyzerForMode(mode)
	if err != nil {
		return fmt.Errorf("failed to create analyzer: %w", err)
	}

	log.Info("Using analyzer: %s", ana.Name())
	log.Info("")

	// Create analysis request
	req := &model.AnalysisRequest{
		TaskID:       1,
		TaskUUID:     uuid,
		TaskType:     mode.ToTaskType(),
		ProfilerType: mode.ToProfilerType(),
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
		Mode:           string(mode),
		ModeDesc:       modeInfo.Description,
		Profile:        string(profile),
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

// parseAnalysisProfile parses the profile string into AnalysisProfile.
func parseAnalysisProfile(s string) (analyzer.AnalysisProfile, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "quick", "fast":
		return analyzer.ProfileQuick, nil
	case "standard", "normal", "default", "":
		return analyzer.ProfileStandard, nil
	case "detailed", "deep", "full":
		return analyzer.ProfileDetailed, nil
	default:
		return "", fmt.Errorf("unknown analysis profile: %q (valid: quick, standard, detailed)", s)
	}
}

func generateUUID() string {
	return fmt.Sprintf("local-%s", time.Now().Format("20060102-150405"))
}

func printResults(_ any, result *model.AnalysisResponse) {
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
		summary["metadata"] = map[string]any{
			"mode":             metadata.Mode,
			"mode_description": metadata.ModeDesc,
			"profile":          metadata.Profile,
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

// AnalysisMetadata holds metadata about the analysis task.
type AnalysisMetadata struct {
	Mode           string `json:"mode"`
	ModeDesc       string `json:"mode_description"`
	Profile        string `json:"profile"`
	InputFile      string `json:"input_file"`
	CreatedAt      string `json:"created_at"`
	AnalysisTimeMs int64  `json:"analysis_time_ms"`
}
