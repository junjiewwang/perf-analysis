package analyzer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	pprofparser "github.com/perf-analysis/internal/parser/pprof"
	"github.com/perf-analysis/pkg/model"
)

// PProfHeapAnalyzer analyzes Go pprof Heap profile data.
type PProfHeapAnalyzer struct {
	*BaseAnalyzer
}

// NewPProfHeapAnalyzer creates a new pprof Heap analyzer.
func NewPProfHeapAnalyzer(config *BaseAnalyzerConfig) *PProfHeapAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}
	if config.AnalysisProfile == "" {
		config.AnalysisProfile = ProfileStandard
	}

	return &PProfHeapAnalyzer{
		BaseAnalyzer: NewBaseAnalyzer(config),
	}
}

// Name returns the analyzer name.
func (a *PProfHeapAnalyzer) Name() string {
	return "pprof_heap_analyzer"
}

// SupportedTypes returns the task types supported by this analyzer.
func (a *PProfHeapAnalyzer) SupportedTypes() []model.TaskType {
	return []model.TaskType{model.TaskTypePProfHeap}
}

// Analyze performs pprof Heap analysis using an input file.
func (a *PProfHeapAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	file, err := os.Open(req.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	return a.AnalyzeFromReader(ctx, req, file)
}

// AnalyzeFromReader performs pprof Heap analysis from a reader.
func (a *PProfHeapAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	// Step 1: Parse the pprof data
	parser := pprofparser.NewParser()
	if err := parser.Parse(dataReader); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseError, err)
	}

	// Step 2: Determine output directory
	taskDir := req.OutputDir
	if taskDir == "" {
		var err error
		taskDir, err = a.EnsureOutputDir(req.TaskUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	topFuncsN := a.config.TopFuncsN
	if topFuncsN <= 0 {
		topFuncsN = 50
	}

	// Step 3: Analyze each sample type
	heapData := &model.PProfHeapData{
		FlameGraphFiles: make(map[string]string),
		HeapSummary:     &model.PProfHeapSummary{},
	}

	sampleTypes := []struct {
		sampleType pprofparser.SampleType
		field      **model.PProfMemoryStats
		unit       string
		filePrefix string
	}{
		{pprofparser.SampleTypeInuseSpace, &heapData.InuseSpace, "bytes", "inuse_space"},
		{pprofparser.SampleTypeInuseObjects, &heapData.InuseObjects, "objects", "inuse_objects"},
		{pprofparser.SampleTypeAllocSpace, &heapData.AllocSpace, "bytes", "alloc_space"},
		{pprofparser.SampleTypeAllocObjects, &heapData.AllocObjects, "objects", "alloc_objects"},
	}

	var totalRecords int64
	var outputFiles []model.OutputFile

	for _, st := range sampleTypes {
		// Get top functions for this sample type
		topFuncs := parser.GetTopFunctions(topFuncsN, st.sampleType, false)
		if len(topFuncs) == 0 {
			continue
		}

		// Convert to model types
		pprofTopFuncs := make([]model.PProfTopFunc, 0, len(topFuncs))
		for _, tf := range topFuncs {
			pprofTopFuncs = append(pprofTopFuncs, model.PProfTopFunc{
				Name:       tf.Name,
				Flat:       tf.Flat,
				FlatPct:    tf.FlatPct,
				Cum:        tf.Cum,
				CumPct:     tf.CumPct,
				Module:     tf.Module,
				SourceFile: tf.SourceFile,
				SourceLine: tf.SourceLine,
			})
		}

		total := parser.GetTotalSamples(st.sampleType)
		unit := parser.GetUnit(st.sampleType)
		if unit == "" {
			unit = st.unit
		}

		*st.field = &model.PProfMemoryStats{
			Total:     total,
			Unit:      unit,
			TopFuncs:  pprofTopFuncs,
			TopNCount: len(pprofTopFuncs),
		}

		// Update summary
		switch st.sampleType {
		case pprofparser.SampleTypeInuseSpace:
			heapData.HeapSummary.TotalInuseBytes = total
		case pprofparser.SampleTypeInuseObjects:
			heapData.HeapSummary.TotalInuseObjects = total
		case pprofparser.SampleTypeAllocSpace:
			heapData.HeapSummary.TotalAllocBytes = total
		case pprofparser.SampleTypeAllocObjects:
			heapData.HeapSummary.TotalAllocObjects = total
		}

		// Generate flame graph for this sample type
		samples, err := parser.ToSamples(st.sampleType)
		if err == nil && len(samples) > 0 {
			fg, err := a.GenerateFlameGraphWithAnalysis(ctx, samples)
			if err == nil {
				flameGraphFile := filepath.Join(taskDir, fmt.Sprintf("%s_flamegraph.json.gz", st.filePrefix))
				if err := a.WriteFlameGraphGzip(fg, flameGraphFile); err == nil {
					heapData.FlameGraphFiles[string(st.sampleType)] = flameGraphFile
					outputFiles = append(outputFiles, model.OutputFile{
						Name:        fmt.Sprintf("Flame Graph (%s)", st.filePrefix),
						LocalPath:   flameGraphFile,
						COSKey:      req.TaskUUID + "/" + fmt.Sprintf("%s_flamegraph.json.gz", st.filePrefix),
						ContentType: "application/gzip",
					})
				}
			}
			totalRecords += int64(len(samples))
		}
	}

	if heapData.InuseSpace == nil && heapData.AllocSpace == nil {
		return nil, ErrEmptyData
	}

	// Step 4: Build response
	return &model.AnalysisResponse{
		TaskUUID:     req.TaskUUID,
		TaskType:     req.TaskType,
		TotalRecords: int(totalRecords),
		OutputFiles:  outputFiles,
		Data:         heapData,
	}, nil
}
