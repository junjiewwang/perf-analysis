package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/junjiewwang/perf-analysis/perflib"
	timerPkg "github.com/junjiewwang/perf-analysis/perflib/internal/timer"
	"github.com/junjiewwang/perf-analysis/perflib/model"
	"github.com/junjiewwang/perf-analysis/perflib/output"
	"github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
	"github.com/junjiewwang/perf-analysis/perflib/query"
)

// JavaHeapAnalyzer analyzes Java heap dump (HPROF) files.
type JavaHeapAnalyzer struct {
	config    *BaseAnalyzerConfig
	hprofOpts *hprof.ParserOptions
}

// JavaHeapAnalyzerOption configures the JavaHeapAnalyzer.
type JavaHeapAnalyzerOption func(*JavaHeapAnalyzer)

// WithHprofOptions sets custom HPROF parser options.
func WithHprofOptions(opts *hprof.ParserOptions) JavaHeapAnalyzerOption {
	return func(a *JavaHeapAnalyzer) {
		a.hprofOpts = opts
	}
}

// NewJavaHeapAnalyzer creates a new Java heap analyzer.
func NewJavaHeapAnalyzer(config *BaseAnalyzerConfig, opts ...JavaHeapAnalyzerOption) *JavaHeapAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}

	hprofOpts := hprof.DefaultParserOptions()
	// Pass logger to hprof parser
	if config.Logger != nil {
		hprofOpts.Logger = config.Logger
	}
	// Pass verbose flag to hprof parser (dependency injection)
	hprofOpts.Verbose = config.Verbose

	a := &JavaHeapAnalyzer{
		config:    config,
		hprofOpts: hprofOpts,
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

// Name returns the analyzer name.
func (a *JavaHeapAnalyzer) Name() string {
	return "java_heap_analyzer"
}

// Analyze performs Java heap dump analysis using an input file.
// It uses the Two-Pass CSR strategy which produces a compact IndexedReferenceGraph
// for on-demand queries, with lightweight pre-computation (class histogram + summary).
func (a *JavaHeapAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	file, err := os.Open(req.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	return a.analyzeTwoPass(ctx, req, file)
}

// AnalyzeFromReader performs Java heap dump analysis from a reader.
// The reader must implement io.ReadSeeker for Two-Pass CSR parsing.
// Non-seekable readers are not supported for heap dump analysis.
func (a *JavaHeapAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	rs, ok := dataReader.(io.ReadSeeker)
	if !ok {
		return nil, fmt.Errorf("heap dump analysis requires a seekable reader (io.ReadSeeker); streaming from non-seekable sources is not supported")
	}
	return a.analyzeTwoPass(ctx, req, rs)
}

// createTimer creates a perflib.Timer with the appropriate logger configuration.
func (a *JavaHeapAnalyzer) createTimer(name string) perflib.Timer {
	if a.config.Logger == nil {
		return perflib.NullTimer
	}
	return timerPkg.New(name,
		timerPkg.WithLogger(a.config.Logger),
		timerPkg.WithEnabled(true),
	)
}

// ensureOutputDir ensures the output directory exists.
func (a *JavaHeapAnalyzer) ensureOutputDir(subDir string) (string, error) {
	outputDir := a.config.OutputDir
	if outputDir == "" {
		outputDir = os.TempDir()
	}

	taskDir := filepath.Join(outputDir, subDir)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	return taskDir, nil
}

// classHistogram represents a class histogram report.
type classHistogram struct {
	TotalClasses   int                 `json:"total_classes"`
	TotalInstances int64               `json:"total_instances"`
	TotalSize      int64               `json:"total_size"`
	Classes        []*hprof.ClassStats `json:"classes"`
}

// isPotentialLeakClass checks if a class name suggests potential memory leak.
func isPotentialLeakClass(className string) bool {
	leakPatterns := []string{
		"HashMap",
		"ArrayList",
		"LinkedList",
		"HashSet",
		"ConcurrentHashMap",
		"LinkedHashMap",
		"TreeMap",
		"WeakHashMap",
		"IdentityHashMap",
	}

	for _, pattern := range leakPatterns {
		if strings.Contains(className, pattern) {
			return true
		}
	}
	return false
}

// formatBytes formats bytes to human-readable string.
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// SortClassesBySize sorts classes by total size in descending order.
func SortClassesBySize(classes []*hprof.ClassStats) {
	sort.Slice(classes, func(i, j int) bool {
		return classes[i].TotalSize > classes[j].TotalSize
	})
}

// SortClassesByCount sorts classes by instance count in descending order.
func SortClassesByCount(classes []*hprof.ClassStats) {
	sort.Slice(classes, func(i, j int) bool {
		return classes[i].InstanceCount > classes[j].InstanceCount
	})
}

// analyzeTwoPass performs heap analysis using the Two-Pass CSR strategy.
// It produces a compact IndexedReferenceGraph for on-demand queries and
// generates lightweight output files (summary + class histogram).
// Heavy computations (GC root paths, retainers, biggest objects expansion)
// are deferred to serve-time HeapQueryEngine.
func (a *JavaHeapAnalyzer) analyzeTwoPass(ctx context.Context, req *model.AnalysisRequest, readSeeker io.ReadSeeker) (*model.AnalysisResponse, error) {
	timer := a.createTimer("Two-Pass Analysis")

	// Step 1: Two-Pass CSR parsing
	parser := hprof.NewParser(a.hprofOpts)
	var graph *hprof.IndexedReferenceGraph
	var scanResult *hprof.ScanResult
	var parseErr error

	timer.TimeFunc("ParseTwoPass", func() {
		graph, scanResult, parseErr = parser.ParseTwoPass(ctx, readSeeker)
	})
	if parseErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseError, parseErr)
	}

	if scanResult.TotalInstances == 0 {
		return nil, ErrEmptyData
	}

	// Step 2: Determine output directory
	var taskDir string
	var err error
	timer.TimeFunc("Ensure output directory", func() {
		taskDir = req.OutputDir
		if taskDir == "" {
			taskDir, err = a.ensureOutputDir("heap-analysis")
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Step 3: Build class histogram from scan result
	var topClasses []*hprof.ClassStats
	timer.TimeFunc("Build class histogram", func() {
		topClasses = a.buildClassHistogramFromScan(scanResult, graph)
	})

	// Step 4: Write class histogram
	histogramFile := filepath.Join(taskDir, output.FileClassHistogram)
	if _, err = timer.TimeFuncWithError("Write class histogram", func() error {
		histogram := &classHistogram{
			TotalClasses:   scanResult.TotalClasses,
			TotalInstances: scanResult.TotalInstances,
			TotalSize:      scanResult.TotalHeapSize,
			Classes:        topClasses,
		}
		file, createErr := os.Create(histogramFile)
		if createErr != nil {
			return createErr
		}
		defer file.Close()
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		return encoder.Encode(histogram)
	}); err != nil {
		return nil, fmt.Errorf("failed to write class histogram: %w", err)
	}

	// Step 5: Build response data
	var heapData *model.HeapAnalysisData
	timer.TimeFunc("Build heap data", func() {
		modelTopClasses := make([]model.HeapClassStats, 0, len(topClasses))
		for _, cls := range topClasses {
			modelTopClasses = append(modelTopClasses, model.HeapClassStats{
				ClassName:     cls.ClassName,
				InstanceCount: cls.InstanceCount,
				TotalSize:     cls.TotalSize,
				Percentage:    cls.Percentage,
				RetainedSize:  cls.RetainedSize,
			})
		}

		heapData = &model.HeapAnalysisData{
			HistogramFile:  histogramFile,
			TotalClasses:   scanResult.TotalClasses,
			TotalInstances: scanResult.TotalInstances,
			TotalHeapSize:  scanResult.TotalHeapSize,
			HeapSizeHuman:  formatBytes(scanResult.TotalHeapSize),
			TopClasses:     modelTopClasses,
			// Note: BiggestObjects, ReferenceGraphs, BusinessRetainers are
			// intentionally empty — they will be computed on-demand by HeapQueryEngine.
		}

		if scanResult.Header != nil {
			heapData.Format = scanResult.Header.Format
			heapData.IDSize = scanResult.Header.IDSize
			heapData.Timestamp = scanResult.Header.Timestamp.Unix()
		}
		if scanResult.HeapSummary != nil {
			heapData.LiveBytes = scanResult.HeapSummary.TotalLiveBytes
			heapData.LiveObjects = scanResult.HeapSummary.TotalLiveObjects
		}
	})

	// Step 7: Generate suggestions
	var suggestions []model.SuggestionItem
	timer.TimeFunc("Generate suggestions", func() {
		suggestions = generateTwoPassSuggestions(topClasses, scanResult)
	})

	// Step 8: Compute dominator tree and retained sizes
	timer.TimeFunc("Compute dominator tree", func() {
		hprof.ComputeDominatorForIndexedGraph(ctx, graph)
	})

	// Step 9: Write heap_index.bin (for fast serve-time loading)
	indexFile := filepath.Join(taskDir, output.FileHeapIndex)
	if _, err = timer.TimeFuncWithError("Write heap index", func() error {
		return hprof.WriteHeapIndex(indexFile, graph)
	}); err != nil {
		// Non-fatal: log warning but don't fail the analysis
		if a.config.Logger != nil {
			a.config.Logger.Warn("Failed to write heap index file: %v", err)
		}
	}

	// Step 9b: Precompute class_stats.json and heap_stats.json for fast API serving
	writer := output.NewWriter(taskDir)
	timer.TimeFunc("Precompute class stats", func() {
		classStats := a.buildPrecomputedClassStats(graph, scanResult)
		if err := writer.WriteJSON(output.FileClassStats, classStats); err != nil {
			if a.config.Logger != nil {
				a.config.Logger.Warn("Failed to write class stats: %v", err)
			}
		}
	})
	timer.TimeFunc("Precompute heap stats", func() {
		heapStats := a.buildPrecomputedHeapStats(graph, scanResult, topClasses)
		if err := writer.WriteJSON(output.FileHeapStats, heapStats); err != nil {
			if a.config.Logger != nil {
				a.config.Logger.Warn("Failed to write heap stats: %v", err)
			}
		}
	})

	// Step 9c: Precompute leak_suspects.json using HprofSnapshotLeakProvider
	timer.TimeFunc("Precompute leak suspects", func() {
		provider := query.NewHprofSnapshotLeakProvider()
		if provider.CanDetect(taskDir) {
			suspects, err := provider.Detect(taskDir)
			if err == nil && len(suspects) > 0 {
				result := &query.LeakSuspectsResult{
					TotalCount: len(suspects),
					Suspects:   suspects,
				}
				if writeErr := writer.WriteJSON(output.FileLeakSuspects, result); writeErr != nil {
					if a.config.Logger != nil {
						a.config.Logger.Warn("Failed to write leak suspects: %v", writeErr)
					}
				}
			}
		}
	})

	timer.PrintSummary()

	if a.config.Logger != nil {
		a.config.Logger.Info("Two-pass analysis complete: %d objects, %d edges, %d GC roots, %d classes",
			scanResult.ObjectCount, scanResult.EdgeCount, len(scanResult.GCRoots), scanResult.TotalClasses)
		reachableCount := int32(0)
		for i := int32(0); i < graph.ObjectCount(); i++ {
			if graph.IsReachable(i) {
				reachableCount++
			}
		}
		a.config.Logger.Info("Reachable objects: %d (%.1f%%)",
			reachableCount, float64(reachableCount)/float64(scanResult.ObjectCount)*100)
	}

	// Step 9: Build output files list
	outputFiles := []model.OutputFile{
		{
			Name:         "Class Histogram",
			LocalPath:    histogramFile,
			RelativePath: output.FileClassHistogram,
			ContentType:  "application/json",
		},
	}
	// Add heap index if successfully written
	if _, statErr := os.Stat(indexFile); statErr == nil {
		outputFiles = append(outputFiles, model.OutputFile{
			Name:         "Heap Index",
			LocalPath:    indexFile,
			RelativePath: output.FileHeapIndex,
			ContentType:  "application/octet-stream",
		})
	}

	return &model.AnalysisResponse{
		Mode:         req.Mode,
		TotalRecords: int(scanResult.TotalInstances),
		OutputFiles:  outputFiles,
		Data:         heapData,
		Suggestions:  suggestions,
	}, nil
}

// buildClassHistogramFromScan builds class statistics from scan result and graph.
// It aggregates shallow sizes and instance counts per class from the IndexedObjectStore.
func (a *JavaHeapAnalyzer) buildClassHistogramFromScan(scanResult *hprof.ScanResult, graph *hprof.IndexedReferenceGraph) []*hprof.ClassStats {
	type classAgg struct {
		className     string
		instanceCount int64
		totalSize     int64
		retainedSize  int64
	}

	classMap := make(map[uint64]*classAgg)
	objectCount := graph.ObjectCount()

	for i := int32(0); i < objectCount; i++ {
		if !graph.IsReachable(i) {
			continue
		}
		classID := graph.GetClassID(i)
		agg, ok := classMap[classID]
		if !ok {
			className := graph.GetClassName(classID)
			if className == "" {
				className = fmt.Sprintf("<class@0x%x>", classID)
			}
			agg = &classAgg{className: className}
			classMap[classID] = agg
		}
		agg.instanceCount++
		agg.totalSize += graph.GetShallowSize(i)
		agg.retainedSize += graph.GetRetainedSize(i)
	}

	// Convert to ClassStats slice and sort
	classes := make([]*hprof.ClassStats, 0, len(classMap))
	for _, agg := range classMap {
		pct := 0.0
		if scanResult.TotalHeapSize > 0 {
			pct = float64(agg.totalSize) / float64(scanResult.TotalHeapSize) * 100
		}
		classes = append(classes, &hprof.ClassStats{
			ClassName:     agg.className,
			InstanceCount: agg.instanceCount,
			TotalSize:     agg.totalSize,
			Percentage:    pct,
			RetainedSize:  agg.retainedSize,
		})
	}

	sort.Slice(classes, func(i, j int) bool {
		return classes[i].TotalSize > classes[j].TotalSize
	})

	// Keep top 200 classes for the histogram
	if len(classes) > 200 {
		classes = classes[:200]
	}

	return classes
}

// generateTwoPassSuggestions generates heap-specific suggestions based on TwoPass results.
func generateTwoPassSuggestions(topClasses []*hprof.ClassStats, scanResult *hprof.ScanResult) []model.SuggestionItem {
	var suggestions []model.SuggestionItem

	for i, cls := range topClasses {
		if i >= 10 {
			break
		}

		if cls.Percentage > 10.0 {
			suggestions = append(suggestions, model.SuggestionItem{
				Suggestion: fmt.Sprintf("类 %s 占用堆内存 %.2f%% (%.2f MB, %d 个实例)，建议检查是否存在内存泄漏或过度分配",
					cls.ClassName, cls.Percentage, float64(cls.TotalSize)/(1024*1024), cls.InstanceCount),
				FuncName: cls.ClassName,
			})
		}

		if isPotentialLeakClass(cls.ClassName) && cls.InstanceCount > 10000 {
			suggestions = append(suggestions, model.SuggestionItem{
				Suggestion: fmt.Sprintf("类 %s 有 %d 个实例，可能存在集合类内存泄漏，建议检查是否有未清理的缓存或集合",
					cls.ClassName, cls.InstanceCount),
				FuncName: cls.ClassName,
			})
		}
	}

	if scanResult.TotalHeapSize > 1024*1024*1024 {
		suggestions = append(suggestions, model.SuggestionItem{
			Suggestion: fmt.Sprintf("堆内存总量 %.2f GB，建议分析是否可以优化内存使用或调整 JVM 堆大小",
				float64(scanResult.TotalHeapSize)/(1024*1024*1024)),
		})
	}

	if scanResult.TotalClasses > 50000 {
		suggestions = append(suggestions, model.SuggestionItem{
			Suggestion: fmt.Sprintf("加载了 %d 个类，可能存在类加载器泄漏，建议检查动态代理或热部署机制",
				scanResult.TotalClasses),
		})
	}

	return suggestions
}

// precomputedClassStats is the structure written to class_stats.json.
type precomputedClassStats struct {
	TotalClasses int                        `json:"total_classes"`
	TotalObjects int64                      `json:"total_objects"`
	TotalSize    int64                      `json:"total_size"`
	Classes      []precomputedClassEntry    `json:"classes"`
}

// precomputedClassEntry is a single class entry in the precomputed stats.
type precomputedClassEntry struct {
	ClassName    string  `json:"class_name"`
	ObjectCount  int64   `json:"object_count"`
	ShallowSize  int64   `json:"shallow_size"`
	RetainedSize int64   `json:"retained_size"`
	Percentage   float64 `json:"percentage"`
}

// precomputedHeapStats is the structure written to heap_stats.json.
type precomputedHeapStats struct {
	TotalHeapSize   int64  `json:"total_heap_size"`
	TotalObjects    int64  `json:"total_objects"`
	TotalClasses    int    `json:"total_classes"`
	TotalGCRoots    int    `json:"total_gc_roots"`
	MaxObjectSize   int64  `json:"max_object_size"`
	MaxRetainedSize int64  `json:"max_retained_size"`
	TopClassName    string `json:"top_class_name"`
}

// buildPrecomputedClassStats builds the precomputed class statistics from the graph.
func (a *JavaHeapAnalyzer) buildPrecomputedClassStats(graph *hprof.IndexedReferenceGraph, scanResult *hprof.ScanResult) *precomputedClassStats {
	type classAgg struct {
		className    string
		objectCount  int64
		shallowSize  int64
		retainedSize int64
	}

	classMap := make(map[uint64]*classAgg)
	var totalObjects int64
	var totalRetained int64

	objectCount := graph.ObjectCount()
	for i := int32(0); i < objectCount; i++ {
		if !graph.IsReachable(i) {
			continue
		}
		classID := graph.GetClassID(i)
		shallow := graph.GetShallowSize(i)
		retained := graph.GetRetainedSize(i)
		totalObjects++
		totalRetained += retained

		agg, ok := classMap[classID]
		if !ok {
			className := graph.GetClassName(classID)
			if className == "" {
				className = fmt.Sprintf("<class@0x%x>", classID)
			}
			agg = &classAgg{className: className}
			classMap[classID] = agg
		}
		agg.objectCount++
		agg.shallowSize += shallow
		agg.retainedSize += retained
	}

	// Sort by retained size descending
	entries := make([]precomputedClassEntry, 0, len(classMap))
	for _, agg := range classMap {
		var pct float64
		if totalRetained > 0 {
			pct = float64(agg.retainedSize) * 100.0 / float64(totalRetained)
		}
		entries = append(entries, precomputedClassEntry{
			ClassName:    agg.className,
			ObjectCount:  agg.objectCount,
			ShallowSize:  agg.shallowSize,
			RetainedSize: agg.retainedSize,
			Percentage:   pct,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].RetainedSize > entries[j].RetainedSize
	})

	// Keep top 500 for the precomputed file
	if len(entries) > 500 {
		entries = entries[:500]
	}

	return &precomputedClassStats{
		TotalClasses: len(classMap),
		TotalObjects: totalObjects,
		TotalSize:    totalRetained,
		Classes:      entries,
	}
}

// buildPrecomputedHeapStats builds the precomputed heap statistics.
func (a *JavaHeapAnalyzer) buildPrecomputedHeapStats(graph *hprof.IndexedReferenceGraph, scanResult *hprof.ScanResult, topClasses []*hprof.ClassStats) *precomputedHeapStats {
	result := &precomputedHeapStats{
		TotalHeapSize: scanResult.TotalHeapSize,
		TotalClasses:  scanResult.TotalClasses,
		TotalGCRoots:  len(scanResult.GCRoots),
	}

	var maxShallow, maxRetained int64
	objectCount := graph.ObjectCount()
	var reachableCount int64
	for i := int32(0); i < objectCount; i++ {
		if !graph.IsReachable(i) {
			continue
		}
		reachableCount++
		shallow := graph.GetShallowSize(i)
		retained := graph.GetRetainedSize(i)
		if retained > maxRetained {
			maxRetained = retained
			maxShallow = shallow
		}
	}
	result.TotalObjects = reachableCount
	result.MaxObjectSize = maxShallow
	result.MaxRetainedSize = maxRetained

	if len(topClasses) > 0 {
		result.TopClassName = topClasses[0].ClassName
	}

	return result
}
