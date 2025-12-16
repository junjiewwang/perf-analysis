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

	"github.com/perf-analysis/internal/parser/hprof"
	"github.com/perf-analysis/pkg/model"
)

// JavaHeapAnalyzer analyzes Java heap dump (HPROF) files.
type JavaHeapAnalyzer struct {
	config     *BaseAnalyzerConfig
	hprofOpts  *hprof.ParserOptions
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

// SupportedTypes returns the task types supported by this analyzer.
func (a *JavaHeapAnalyzer) SupportedTypes() []model.TaskType {
	return []model.TaskType{model.TaskTypeJavaHeap}
}

// Analyze performs Java heap dump analysis using an input file.
func (a *JavaHeapAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	if req.TaskType != model.TaskTypeJavaHeap {
		return nil, fmt.Errorf("java heap analyzer only supports task type java_heap, got %v", req.TaskType)
	}

	file, err := os.Open(req.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	return a.AnalyzeFromReader(ctx, req, file)
}

// AnalyzeFromReader performs Java heap dump analysis from a reader.
func (a *JavaHeapAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	if req.TaskType != model.TaskTypeJavaHeap {
		return nil, fmt.Errorf("java heap analyzer only supports task type java_heap, got %v", req.TaskType)
	}

	// Step 1: Parse the HPROF data
	parser := hprof.NewParser(a.hprofOpts)
	heapResult, err := parser.Parse(ctx, dataReader)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseError, err)
	}

	if heapResult.TotalInstances == 0 {
		return nil, ErrEmptyData
	}

	// Step 2: Determine output directory
	taskDir := req.OutputDir
	if taskDir == "" {
		taskDir, err = a.ensureOutputDir(req.TaskUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Step 3: Generate heap analysis report
	heapReportFile := filepath.Join(taskDir, "heap_analysis.json")
	if err := a.writeHeapReport(heapResult, heapReportFile); err != nil {
		return nil, fmt.Errorf("failed to write heap report: %w", err)
	}

	// Step 4: Generate class histogram (similar to jmap -histo)
	histogramFile := filepath.Join(taskDir, "class_histogram.json")
	if err := a.writeClassHistogram(heapResult, histogramFile); err != nil {
		return nil, fmt.Errorf("failed to write class histogram: %w", err)
	}

	// Step 5: Build top classes
	topClasses := a.buildTopClasses(heapResult)

	// Step 6: Generate suggestions
	suggestions := a.generateSuggestions(heapResult)

	// Step 7: Build HeapAnalysisData
	heapData := &model.HeapAnalysisData{
		HeapReportFile:    heapReportFile,
		HistogramFile:     histogramFile,
		TotalClasses:      heapResult.TotalClasses,
		TotalInstances:    heapResult.TotalInstances,
		TotalHeapSize:     heapResult.TotalHeapSize,
		HeapSizeHuman:     formatBytes(heapResult.TotalHeapSize),
		TopClasses:        topClasses,
		BiggestObjects:    a.buildBiggestObjects(heapResult),
		ReferenceGraphs:   a.buildReferenceGraphs(heapResult),
		BusinessRetainers: a.buildBusinessRetainers(heapResult),
	}

	if heapResult.Header != nil {
		heapData.Format = heapResult.Header.Format
		heapData.IDSize = heapResult.Header.IDSize
		heapData.Timestamp = heapResult.Header.Timestamp.Unix()
	}

	if heapResult.Summary != nil {
		heapData.LiveBytes = heapResult.Summary.TotalLiveBytes
		heapData.LiveObjects = heapResult.Summary.TotalLiveObjects
	}

	// Step 8: Write biggest objects file
	if len(heapData.BiggestObjects) > 0 {
		biggestObjectsFile := filepath.Join(taskDir, "biggest_objects.json")
		if err := a.writeBiggestObjects(heapData.BiggestObjects, biggestObjectsFile); err != nil {
			// Log error but don't fail the analysis
			if a.config.Logger != nil {
				a.config.Logger.Warn("Failed to write biggest objects file: %v", err)
			}
		}
	}

	// Step 9: Build output files
	outputFiles := []model.OutputFile{
		{
			Name:        "Heap Report",
			LocalPath:   heapReportFile,
			COSKey:      req.TaskUUID + "/heap_analysis.json",
			ContentType: "application/json",
		},
		{
			Name:        "Class Histogram",
			LocalPath:   histogramFile,
			COSKey:      req.TaskUUID + "/class_histogram.json",
			ContentType: "application/json",
		},
	}

	// Step 10: Build response
	return &model.AnalysisResponse{
		TaskUUID:     req.TaskUUID,
		TaskType:     req.TaskType,
		TotalRecords: int(heapResult.TotalInstances),
		OutputFiles:  outputFiles,
		Data:         heapData,
		Suggestions:  suggestions,
	}, nil
}

// ensureOutputDir ensures the output directory exists.
func (a *JavaHeapAnalyzer) ensureOutputDir(taskUUID string) (string, error) {
	outputDir := a.config.OutputDir
	if outputDir == "" {
		outputDir = os.TempDir()
	}

	taskDir := filepath.Join(outputDir, taskUUID)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	return taskDir, nil
}

// writeHeapReport writes the complete heap analysis report.
func (a *JavaHeapAnalyzer) writeHeapReport(result *hprof.HeapAnalysisResult, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// writeClassHistogram writes the class histogram.
func (a *JavaHeapAnalyzer) writeClassHistogram(result *hprof.HeapAnalysisResult, outputPath string) error {
	histogram := &ClassHistogram{
		TotalClasses:   result.TotalClasses,
		TotalInstances: result.TotalInstances,
		TotalSize:      result.TotalHeapSize,
		Classes:        result.TopClasses,
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(histogram)
}

// ClassHistogram represents a class histogram report.
type ClassHistogram struct {
	TotalClasses   int                 `json:"total_classes"`
	TotalInstances int64               `json:"total_instances"`
	TotalSize      int64               `json:"total_size"`
	Classes        []*hprof.ClassStats `json:"classes"`
}

// buildTopClasses builds the top classes list from heap result.
func (a *JavaHeapAnalyzer) buildTopClasses(result *hprof.HeapAnalysisResult) []model.HeapClassStats {
	topClasses := make([]model.HeapClassStats, 0, len(result.TopClasses))
	for _, cls := range result.TopClasses {
		heapClass := model.HeapClassStats{
			ClassName:     cls.ClassName,
			InstanceCount: cls.InstanceCount,
			TotalSize:     cls.TotalSize,
			Percentage:    cls.Percentage,
			RetainedSize:  cls.RetainedSize, // Use retained size from parser (computed via dominator tree)
		}
		
		// Add retainer information if available
		if result.ClassRetainers != nil {
			if retainers, ok := result.ClassRetainers[cls.ClassName]; ok {
				// Override retained size if available from retainer analysis
				if retainers.RetainedSize > 0 {
					heapClass.RetainedSize = retainers.RetainedSize
				}
				
				// Add retainers with depth info
				if len(retainers.Retainers) > 0 {
					for _, r := range retainers.Retainers {
						heapClass.Retainers = append(heapClass.Retainers, model.HeapRetainer{
							RetainerClass: r.RetainerClass,
							FieldName:     r.FieldName,
							RetainedSize:  r.RetainedSize,
							RetainedCount: r.RetainedCount,
							Percentage:    r.Percentage,
							Depth:         r.Depth,
						})
					}
				}
				
				// Add GC root paths
				if len(retainers.GCRootPaths) > 0 {
					for _, path := range retainers.GCRootPaths {
						gcPath := &model.GCRootPath{
							RootType: string(path.RootType),
							Depth:    path.Depth,
						}
						for _, node := range path.Path {
							gcPath.Path = append(gcPath.Path, &model.GCRootPathNode{
								ClassName: node.ClassName,
								FieldName: node.FieldName,
								Size:      node.Size,
							})
						}
						heapClass.GCRootPaths = append(heapClass.GCRootPaths, gcPath)
					}
				}
			}
		}
		
		topClasses = append(topClasses, heapClass)
	}
	return topClasses
}

// buildReferenceGraphs builds reference graph data for visualization.
func (a *JavaHeapAnalyzer) buildReferenceGraphs(result *hprof.HeapAnalysisResult) map[string]*model.HeapReferenceGraph {
	if result.ReferenceGraphs == nil {
		return nil
	}
	
	graphs := make(map[string]*model.HeapReferenceGraph)
	for className, graphData := range result.ReferenceGraphs {
		graph := &model.HeapReferenceGraph{
			ClassName: className,
			Nodes:     make([]model.HeapReferenceNode, 0, len(graphData.Nodes)),
			Edges:     make([]model.HeapReferenceEdge, 0, len(graphData.Edges)),
		}
		
		for _, node := range graphData.Nodes {
			graph.Nodes = append(graph.Nodes, model.HeapReferenceNode{
				ID:           node.ID,
				ClassName:    node.ClassName,
				Size:         node.Size,
				RetainedSize: node.RetainedSize,
				IsGCRoot:     node.IsGCRoot,
				GCRootType:   node.GCRootType,
			})
		}
		
		for _, edge := range graphData.Edges {
			graph.Edges = append(graph.Edges, model.HeapReferenceEdge{
				Source:    edge.Source,
				Target:    edge.Target,
				FieldName: edge.FieldName,
			})
		}
		
		graphs[className] = graph
	}
	
	return graphs
}

// buildBusinessRetainers builds business-level retainer information for root cause analysis.
func (a *JavaHeapAnalyzer) buildBusinessRetainers(result *hprof.HeapAnalysisResult) map[string][]model.HeapBusinessRetainer {
	if result.BusinessRetainers == nil {
		return nil
	}
	
	retainers := make(map[string][]model.HeapBusinessRetainer)
	for className, businessRetainers := range result.BusinessRetainers {
		var modelRetainers []model.HeapBusinessRetainer
		for _, r := range businessRetainers {
			modelRetainers = append(modelRetainers, model.HeapBusinessRetainer{
				ClassName:     r.ClassName,
				FieldPath:     r.FieldPath,
				RetainedSize:  r.RetainedSize,
				RetainedCount: r.RetainedCount,
				Percentage:    r.Percentage,
				Depth:         r.Depth,
				IsGCRoot:      r.IsGCRoot,
				GCRootType:    r.GCRootType,
			})
		}
		if len(modelRetainers) > 0 {
			retainers[className] = modelRetainers
		}
	}
	
	return retainers
}

// buildBiggestObjects builds the biggest objects list from heap result.
func (a *JavaHeapAnalyzer) buildBiggestObjects(result *hprof.HeapAnalysisResult) []model.HeapBiggestObject {
	if result.BiggestObjects == nil {
		return nil
	}
	
	biggestObjects := make([]model.HeapBiggestObject, 0, len(result.BiggestObjects))
	for _, obj := range result.BiggestObjects {
		bigObj := model.HeapBiggestObject{
			ObjectID:     formatObjectID(obj.ObjectID),
			ClassName:    obj.ClassName,
			ShallowSize:  obj.ShallowSize,
			RetainedSize: obj.RetainedSize,
		}
		
		// Convert fields
		if len(obj.Fields) > 0 {
			for _, f := range obj.Fields {
				field := model.HeapObjectField{
					Name:     f.Name,
					Type:     f.Type,
					Value:    f.Value,
					IsStatic: f.IsStatic,
				}
				if f.RefID != 0 {
					field.RefID = formatObjectID(f.RefID)
					field.RefClass = f.RefClass
				}
				bigObj.Fields = append(bigObj.Fields, field)
			}
		}
		
		// Convert GC root path
		if obj.GCRootPath != nil {
			gcPath := &model.HeapGCRootPath{
				RootType: string(obj.GCRootPath.RootType),
				Depth:    obj.GCRootPath.Depth,
			}
			for _, node := range obj.GCRootPath.Path {
				gcPath.Path = append(gcPath.Path, model.HeapGCRootPathNode{
					ClassName: node.ClassName,
					FieldName: node.FieldName,
					Size:      node.Size,
				})
			}
			bigObj.GCRootPath = gcPath
		}
		
		biggestObjects = append(biggestObjects, bigObj)
	}
	
	return biggestObjects
}

// writeBiggestObjects writes the biggest objects to a JSON file.
func (a *JavaHeapAnalyzer) writeBiggestObjects(objects []model.HeapBiggestObject, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(objects)
}

// formatObjectID formats an object ID as a hex string.
func formatObjectID(id uint64) string {
	return fmt.Sprintf("0x%x", id)
}

// generateSuggestions generates heap-specific suggestions.
func (a *JavaHeapAnalyzer) generateSuggestions(result *hprof.HeapAnalysisResult) []model.SuggestionItem {
	var suggestions []model.SuggestionItem

	// Analyze top classes for potential issues
	for i, cls := range result.TopClasses {
		if i >= 10 {
			break
		}

		// Large memory consumers
		if cls.Percentage > 10.0 {
			suggestions = append(suggestions, model.SuggestionItem{
				Suggestion: fmt.Sprintf("类 %s 占用堆内存 %.2f%% (%.2f MB, %d 个实例)，建议检查是否存在内存泄漏或过度分配",
					cls.ClassName, cls.Percentage, float64(cls.TotalSize)/(1024*1024), cls.InstanceCount),
				FuncName: cls.ClassName,
			})
		}

		// Potential memory leak patterns
		if a.isPotentialLeakClass(cls.ClassName) && cls.InstanceCount > 10000 {
			suggestions = append(suggestions, model.SuggestionItem{
				Suggestion: fmt.Sprintf("类 %s 有 %d 个实例，可能存在集合类内存泄漏，建议检查是否有未清理的缓存或集合",
					cls.ClassName, cls.InstanceCount),
				FuncName: cls.ClassName,
			})
		}

		// Large number of String instances
		if cls.ClassName == "java.lang.String" && cls.InstanceCount > 100000 {
			suggestions = append(suggestions, model.SuggestionItem{
				Suggestion: fmt.Sprintf("String 对象数量过多 (%d 个)，建议检查是否有字符串拼接问题或考虑使用 String.intern()",
					cls.InstanceCount),
				FuncName: "java.lang.String",
			})
		}

		// Large byte arrays (often indicate large buffers or serialization issues)
		if cls.ClassName == "byte[]" && cls.TotalSize > 100*1024*1024 {
			suggestions = append(suggestions, model.SuggestionItem{
				Suggestion: fmt.Sprintf("byte[] 数组占用 %.2f MB，建议检查是否有大缓冲区或序列化问题",
					float64(cls.TotalSize)/(1024*1024)),
				FuncName: "byte[]",
			})
		}

		// char[] arrays (often from String objects)
		if cls.ClassName == "char[]" && cls.TotalSize > 100*1024*1024 {
			suggestions = append(suggestions, model.SuggestionItem{
				Suggestion: fmt.Sprintf("char[] 数组占用 %.2f MB (通常来自 String 对象)，建议优化字符串使用",
					float64(cls.TotalSize)/(1024*1024)),
				FuncName: "char[]",
			})
		}
	}

	// Overall heap size warning
	if result.TotalHeapSize > 1024*1024*1024 { // > 1GB
		suggestions = append(suggestions, model.SuggestionItem{
			Suggestion: fmt.Sprintf("堆内存总量 %.2f GB，建议分析是否可以优化内存使用或调整 JVM 堆大小",
				float64(result.TotalHeapSize)/(1024*1024*1024)),
		})
	}

	// Too many classes loaded
	if result.TotalClasses > 50000 {
		suggestions = append(suggestions, model.SuggestionItem{
			Suggestion: fmt.Sprintf("加载了 %d 个类，可能存在类加载器泄漏，建议检查动态代理或热部署机制",
				result.TotalClasses),
		})
	}

	return suggestions
}

// isPotentialLeakClass checks if a class name suggests potential memory leak.
func (a *JavaHeapAnalyzer) isPotentialLeakClass(className string) bool {
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

// GetOutputFiles returns the list of output files generated by the analyzer.
func (a *JavaHeapAnalyzer) GetOutputFiles(taskUUID, taskDir string) []model.OutputFile {
	return []model.OutputFile{
		{
			Name:        "Heap Report",
			LocalPath:   filepath.Join(taskDir, "heap_analysis.json"),
			COSKey:      taskUUID + "/heap_analysis.json",
			ContentType: "application/json",
		},
		{
			Name:        "Class Histogram",
			LocalPath:   filepath.Join(taskDir, "class_histogram.json"),
			COSKey:      taskUUID + "/class_histogram.json",
			ContentType: "application/json",
		},
	}
}

// HeapDominatorTree represents a dominator tree for retained size analysis.
type HeapDominatorTree struct {
	Roots []*DominatorNode `json:"roots"`
}

// DominatorNode represents a node in the dominator tree.
type DominatorNode struct {
	ClassName    string           `json:"class_name"`
	ObjectID     uint64           `json:"object_id,omitempty"`
	ShallowSize  int64            `json:"shallow_size"`
	RetainedSize int64            `json:"retained_size"`
	Children     []*DominatorNode `json:"children,omitempty"`
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

// HeapAnalysisReport represents the complete heap analysis report.
type HeapAnalysisReport struct {
	Summary        *model.HeapAnalysisData `json:"summary"`
	TopClasses     []*hprof.ClassStats     `json:"top_classes"`
	Suggestions    []model.SuggestionItem  `json:"suggestions"`
	DominatorTree  *HeapDominatorTree      `json:"dominator_tree,omitempty"`
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
