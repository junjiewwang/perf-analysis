// Package analyzer provides profiling data analysis implementations.
package analyzer

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pprofparser "github.com/perf-analysis/internal/parser/pprof"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// PProfBatchAnalyzer analyzes a directory of pprof files.
type PProfBatchAnalyzer struct {
	name              string
	config            *BaseAnalyzerConfig
	logger            utils.Logger
	cpuAnalyzer       *PProfCPUAnalyzer
	heapAnalyzer      *PProfHeapAnalyzer
	goroutineAnalyzer *PProfGoroutineAnalyzer
	blockAnalyzer     *PProfContentionAnalyzer
	mutexAnalyzer     *PProfContentionAnalyzer
}

// NewPProfBatchAnalyzer creates a new PProfBatchAnalyzer.
func NewPProfBatchAnalyzer(config *BaseAnalyzerConfig) *PProfBatchAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}
	logger := config.Logger
	if logger == nil {
		logger = utils.NewStdLogger(utils.LevelInfo, os.Stdout)
	}
	return &PProfBatchAnalyzer{
		name:              "pprof_batch_analyzer",
		config:            config,
		logger:            logger,
		cpuAnalyzer:       NewPProfCPUAnalyzer(config),
		heapAnalyzer:      NewPProfHeapAnalyzer(config),
		goroutineAnalyzer: NewPProfGoroutineAnalyzer(config),
		blockAnalyzer:     NewPProfBlockAnalyzer(config),
		mutexAnalyzer:     NewPProfMutexAnalyzer(config),
	}
}

// Name returns the name of this analyzer.
func (a *PProfBatchAnalyzer) Name() string {
	return a.name
}

// SupportedTypes returns the task types supported by this analyzer.
func (a *PProfBatchAnalyzer) SupportedTypes() []model.TaskType {
	return []model.TaskType{
		model.TaskTypePProfCPU,
		model.TaskTypePProfHeap,
		model.TaskTypePProfGoroutine,
		model.TaskTypePProfBlock,
		model.TaskTypePProfMutex,
	}
}

// Analyze performs batch analysis on a pprof directory.
func (a *PProfBatchAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	// Check if input is a directory
	fileInfo, err := os.Stat(req.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to stat input: %w", err)
	}

	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("PProfBatchAnalyzer requires a directory input")
	}

	return a.analyzeDirectory(ctx, req)
}

// AnalyzeFromReader is not supported for batch analysis.
func (a *PProfBatchAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	return nil, fmt.Errorf("PProfBatchAnalyzer does not support reading from a single reader, use Analyze with a directory path")
}

// analyzeDirectory analyzes all pprof files in a directory.
func (a *PProfBatchAnalyzer) analyzeDirectory(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	baseDir := req.InputFile

	// Create output directory
	outputDir := req.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(baseDir, "analysis-output")
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Discover pprof files
	profileFiles := a.discoverProfiles(baseDir)

	results := &BatchAnalysisResult{
		BaseDir:     baseDir,
		OutputDir:   outputDir,
		AnalyzedAt:  time.Now(),
		ProfileSets: make(map[string]*ProfileSetResult),
	}

	// Analyze each profile type
	for profileType, files := range profileFiles {
		if len(files) == 0 {
			continue
		}

		setResult, err := a.analyzeProfileSet(ctx, req, profileType, files, outputDir)
		if err != nil {
			a.logger.Warn("Failed to analyze %s profiles: %v", profileType, err)
			continue
		}
		results.ProfileSets[profileType] = setResult
	}

	// Calculate total samples
	for _, ps := range results.ProfileSets {
		results.TotalSamples += ps.TotalSamples
	}

	// Perform leak detection if we have multiple profiles
	leakReports := a.performLeakDetection(profileFiles)
	results.LeakReports = leakReports

	// Save batch results
	if err := a.saveBatchResults(results, outputDir); err != nil {
		a.logger.Warn("Failed to save batch results: %v", err)
	}

	// Build PProfBatchData for response
	batchData := a.buildBatchData(results, outputDir)

	// Create response with PProfBatchData
	response := &model.AnalysisResponse{
		TaskUUID:     req.TaskUUID,
		TaskType:     model.TaskTypePProfCPU, // Primary type
		TotalRecords: int(results.TotalSamples),
		Data:         batchData,
	}

	return response, nil
}

// ProfileSetResult represents analysis results for a set of profiles.
type ProfileSetResult struct {
	ProfileType  string           `json:"profile_type"`
	FileCount    int              `json:"file_count"`
	Files        []string         `json:"files"`
	LatestFile   string           `json:"latest_file"`
	TotalSamples int64            `json:"total_samples"`
	OutputFiles  []string         `json:"output_files"`
	LeakReport   *pprofparser.LeakReport `json:"leak_report,omitempty"`
}

// BatchAnalysisResult represents the complete batch analysis result.
type BatchAnalysisResult struct {
	BaseDir      string                      `json:"base_dir"`
	OutputDir    string                      `json:"output_dir"`
	AnalyzedAt   time.Time                   `json:"analyzed_at"`
	ProfileSets  map[string]*ProfileSetResult `json:"profile_sets"`
	LeakReports  map[string]*pprofparser.LeakReport `json:"leak_reports,omitempty"`
	TotalSamples int64                       `json:"total_samples"`
}

// discoverProfiles discovers pprof files organized by type.
func (a *PProfBatchAnalyzer) discoverProfiles(baseDir string) map[string][]string {
	profiles := map[string][]string{
		"cpu":       {},
		"heap":      {},
		"goroutine": {},
		"block":     {},
		"mutex":     {},
		"allocs":    {},
	}

	// Check for subdirectories
	for profileType := range profiles {
		subDir := filepath.Join(baseDir, profileType)
		if files, err := a.findPProfFiles(subDir); err == nil {
			profiles[profileType] = files
		}
	}

	// Also check root directory for any pprof files
	rootFiles, _ := a.findPProfFiles(baseDir)
	for _, f := range rootFiles {
		name := strings.ToLower(filepath.Base(f))
		switch {
		case strings.Contains(name, "cpu"):
			profiles["cpu"] = append(profiles["cpu"], f)
		case strings.Contains(name, "heap"):
			profiles["heap"] = append(profiles["heap"], f)
		case strings.Contains(name, "goroutine"):
			profiles["goroutine"] = append(profiles["goroutine"], f)
		case strings.Contains(name, "block"):
			profiles["block"] = append(profiles["block"], f)
		case strings.Contains(name, "mutex"):
			profiles["mutex"] = append(profiles["mutex"], f)
		case strings.Contains(name, "alloc"):
			profiles["allocs"] = append(profiles["allocs"], f)
		}
	}

	return profiles
}

// findPProfFiles finds all pprof files in a directory.
func (a *PProfBatchAnalyzer) findPProfFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".pprof") || strings.HasSuffix(name, ".pb.gz") {
			files = append(files, filepath.Join(dir, name))
		}
	}

	// Sort by name (which typically includes timestamp)
	sort.Strings(files)
	return files, nil
}

// analyzeProfileSet analyzes a set of profiles of the same type.
func (a *PProfBatchAnalyzer) analyzeProfileSet(
	ctx context.Context,
	req *model.AnalysisRequest,
	profileType string,
	files []string,
	outputDir string,
) (*ProfileSetResult, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to analyze")
	}

	result := &ProfileSetResult{
		ProfileType: profileType,
		FileCount:   len(files),
		Files:       files,
		LatestFile:  files[len(files)-1], // Last file (most recent)
	}

	// Analyze the latest file
	latestFile := files[len(files)-1]
	subOutputDir := filepath.Join(outputDir, profileType)
	
	// Create output directory
	if err := os.MkdirAll(subOutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory %s: %w", subOutputDir, err)
	}
	
	subReq := &model.AnalysisRequest{
		TaskID:       req.TaskID,
		InputFile:    latestFile,
		OutputDir:    subOutputDir,
		ProfilerType: model.ProfilerTypePProf,
	}

	var resp *model.AnalysisResponse
	var err error

	switch profileType {
	case "cpu":
		subReq.TaskType = model.TaskTypePProfCPU
		resp, err = a.cpuAnalyzer.Analyze(ctx, subReq)
	case "heap", "allocs":
		subReq.TaskType = model.TaskTypePProfHeap
		resp, err = a.heapAnalyzer.Analyze(ctx, subReq)
	case "goroutine":
		subReq.TaskType = model.TaskTypePProfGoroutine
		resp, err = a.goroutineAnalyzer.Analyze(ctx, subReq)
	case "block":
		subReq.TaskType = model.TaskTypePProfBlock
		resp, err = a.blockAnalyzer.Analyze(ctx, subReq)
	case "mutex":
		subReq.TaskType = model.TaskTypePProfMutex
		resp, err = a.mutexAnalyzer.Analyze(ctx, subReq)
	default:
		return nil, fmt.Errorf("unknown profile type: %s", profileType)
	}

	if err != nil {
		return nil, err
	}

	result.TotalSamples = int64(resp.TotalRecords)
	result.OutputFiles = []string{subReq.OutputDir}

	return result, nil
}

// performLeakDetection performs leak detection on profile sets with multiple files.
func (a *PProfBatchAnalyzer) performLeakDetection(profileFiles map[string][]string) map[string]*pprofparser.LeakReport {
	reports := make(map[string]*pprofparser.LeakReport)

	// Heap leak detection
	if heapFiles := profileFiles["heap"]; len(heapFiles) >= 2 {
		if report := a.detectLeak(heapFiles, pprofparser.LeakTypeHeap); report != nil {
			reports["heap"] = report
		}
	}

	// Goroutine leak detection
	if goroutineFiles := profileFiles["goroutine"]; len(goroutineFiles) >= 2 {
		if report := a.detectLeak(goroutineFiles, pprofparser.LeakTypeGoroutine); report != nil {
			reports["goroutine"] = report
		}
	}

	return reports
}

// detectLeak detects leaks from a set of profile files.
func (a *PProfBatchAnalyzer) detectLeak(files []string, leakType pprofparser.LeakType) *pprofparser.LeakReport {
	if len(files) < 2 {
		return nil
	}

	detector := pprofparser.NewLeakDetector()

	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			a.logger.Warn("Failed to open %s: %v", file, err)
			continue
		}

		// Extract timestamp from filename
		timestamp := extractTimestampFromFilename(file)

		if err := detector.AddProfile(f, timestamp); err != nil {
			f.Close()
			a.logger.Warn("Failed to parse %s: %v", file, err)
			continue
		}
		f.Close()
	}

	if detector.ProfileCount() < 2 {
		return nil
	}

	var report *pprofparser.LeakReport
	var err error

	switch leakType {
	case pprofparser.LeakTypeHeap:
		report, err = detector.DetectHeapLeak()
	case pprofparser.LeakTypeGoroutine:
		report, err = detector.DetectGoroutineLeak()
	}

	if err != nil {
		a.logger.Warn("Failed to detect %s leak: %v", leakType, err)
		return nil
	}

	return report
}

// extractTimestampFromFilename extracts timestamp from pprof filename.
// Expected format: type_YYYYMMDD_HHMMSS.pprof
func extractTimestampFromFilename(filename string) time.Time {
	base := filepath.Base(filename)
	// Remove extension
	base = strings.TrimSuffix(base, ".pprof")
	base = strings.TrimSuffix(base, ".pb.gz")

	// Try to find timestamp pattern
	parts := strings.Split(base, "_")
	if len(parts) >= 3 {
		// Try last two parts as date and time
		dateStr := parts[len(parts)-2]
		timeStr := parts[len(parts)-1]

		if len(dateStr) == 8 && len(timeStr) == 6 {
			combined := dateStr + timeStr
			if t, err := time.Parse("20060102150405", combined); err == nil {
				return t
			}
		}
	}

	// Fallback to file modification time
	if info, err := os.Stat(filename); err == nil {
		return info.ModTime()
	}

	return time.Now()
}

// saveBatchResults saves the batch analysis results to a JSON file.
func (a *PProfBatchAnalyzer) saveBatchResults(results *BatchAnalysisResult, outputDir string) error {
	outputFile := filepath.Join(outputDir, "batch_analysis.json")

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write results: %w", err)
	}

	return nil
}

// buildBatchData builds PProfBatchData from analysis results.
func (a *PProfBatchAnalyzer) buildBatchData(results *BatchAnalysisResult, outputDir string) *model.PProfBatchData {
	// Build profile sets
	profileSets := make(map[string]*model.PProfBatchProfileSet)
	for name, ps := range results.ProfileSets {
		profileSets[name] = &model.PProfBatchProfileSet{
			ProfileType:  ps.ProfileType,
			FileCount:    ps.FileCount,
			TotalSamples: ps.TotalSamples,
			LatestFile:   ps.LatestFile,
		}
	}

	// Build leak reports summary
	leakReportsSummary := make(map[string]*model.PProfLeakReportSummary)
	detailedLeakReports := make(map[string]*model.PProfLeakReport)
	for name, lr := range results.LeakReports {
		leakReportsSummary[name] = &model.PProfLeakReportSummary{
			Type:          string(lr.Type),
			Severity:      string(lr.Severity),
			Conclusion:    lr.Conclusion,
			TotalGrowth:   lr.TotalGrowth,
			GrowthPercent: lr.TotalGrowthPct,
			ItemsCount:    len(lr.GrowthItems),
		}
		
		// Build detailed leak report
		growthItems := make([]model.PProfLeakGrowthItem, 0, len(lr.GrowthItems))
		for _, item := range lr.GrowthItems {
			growthItems = append(growthItems, model.PProfLeakGrowthItem{
				Name:          item.Name,
				BaselineValue: item.BaselineValue,
				CurrentValue:  item.CurrentValue,
				GrowthValue:   item.GrowthValue,
				GrowthPercent: item.GrowthPercent,
			})
		}
		detailedLeakReports[name] = &model.PProfLeakReport{
			Type:               string(lr.Type),
			Severity:           string(lr.Severity),
			Conclusion:         lr.Conclusion,
			BaselineTotal:      lr.BaselineTotal,
			CurrentTotal:       lr.CurrentTotal,
			TotalGrowth:        lr.TotalGrowth,
			TotalGrowthPercent: lr.TotalGrowthPct,
			GrowthItems:        growthItems,
		}
	}

	// Build top functions from CPU profile if available
	var topFuncs []model.PProfTopFunc
	if cpuSet, ok := results.ProfileSets["cpu"]; ok && cpuSet.TotalSamples > 0 {
		cpuDir := filepath.Join(outputDir, "cpu")
		topFuncs = a.extractTopFunctionsFromFlameGraph(cpuDir)
	}

	return &model.PProfBatchData{
		ProfileSets:         profileSets,
		LeakReports:         leakReportsSummary,
		DetailedLeakReports: detailedLeakReports,
		TopFuncs:            topFuncs,
		TotalSamples:        results.TotalSamples,
	}
}

// extractTopFunctionsFromFlameGraph extracts top functions from a flame graph JSON file.
func (a *PProfBatchAnalyzer) extractTopFunctionsFromFlameGraph(cpuDir string) []model.PProfTopFunc {
	// Try to read the collapsed_data.json.gz file
	flameGraphFile := filepath.Join(cpuDir, "collapsed_data.json.gz")
	
	f, err := os.Open(flameGraphFile)
	if err != nil {
		a.logger.Debug("Failed to open flame graph file: %v", err)
		return nil
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		a.logger.Debug("Failed to create gzip reader: %v", err)
		return nil
	}
	defer gzReader.Close()

	var fg struct {
		ThreadAnalysis *struct {
			TopFunctions []struct {
				Name       string  `json:"name"`
				Samples    int64   `json:"samples"`
				Percentage float64 `json:"percentage"`
			} `json:"top_functions"`
		} `json:"thread_analysis"`
	}

	if err := json.NewDecoder(gzReader).Decode(&fg); err != nil {
		a.logger.Debug("Failed to decode flame graph: %v", err)
		return nil
	}

	if fg.ThreadAnalysis == nil || len(fg.ThreadAnalysis.TopFunctions) == 0 {
		return nil
	}

	// Convert to PProfTopFunc format
	topFuncs := make([]model.PProfTopFunc, 0, len(fg.ThreadAnalysis.TopFunctions))
	for _, tf := range fg.ThreadAnalysis.TopFunctions {
		if len(topFuncs) >= 50 {
			break
		}
		topFuncs = append(topFuncs, model.PProfTopFunc{
			Name:    tf.Name,
			Flat:    tf.Samples,
			FlatPct: tf.Percentage,
		})
	}

	return topFuncs
}
