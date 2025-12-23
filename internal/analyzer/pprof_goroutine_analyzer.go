package analyzer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/pprof/profile"

	pprofparser "github.com/perf-analysis/internal/parser/pprof"
	"github.com/perf-analysis/pkg/model"
)

// PProfGoroutineAnalyzer analyzes Go pprof Goroutine profile data.
type PProfGoroutineAnalyzer struct {
	*BaseAnalyzer
}

// NewPProfGoroutineAnalyzer creates a new pprof Goroutine analyzer.
func NewPProfGoroutineAnalyzer(config *BaseAnalyzerConfig) *PProfGoroutineAnalyzer {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}
	if config.AnalysisProfile == "" {
		config.AnalysisProfile = ProfileStandard
	}

	return &PProfGoroutineAnalyzer{
		BaseAnalyzer: NewBaseAnalyzer(config),
	}
}

// Name returns the analyzer name.
func (a *PProfGoroutineAnalyzer) Name() string {
	return "pprof_goroutine_analyzer"
}

// SupportedTypes returns the task types supported by this analyzer.
func (a *PProfGoroutineAnalyzer) SupportedTypes() []model.TaskType {
	return []model.TaskType{model.TaskTypePProfGoroutine}
}

// Analyze performs pprof Goroutine analysis using an input file.
func (a *PProfGoroutineAnalyzer) Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error) {
	file, err := os.Open(req.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	return a.AnalyzeFromReader(ctx, req, file)
}

// AnalyzeFromReader performs pprof Goroutine analysis from a reader.
func (a *PProfGoroutineAnalyzer) AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	// Step 1: Parse the pprof data
	parser := pprofparser.NewParser()
	if err := parser.Parse(dataReader); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseError, err)
	}

	prof := parser.Profile()
	if prof == nil {
		return nil, ErrEmptyData
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

	// Step 3: Analyze goroutine distribution
	distribution, totalCount := a.analyzeGoroutineDistribution(prof)

	// Step 4: Get top functions
	topFuncsN := a.config.TopFuncsN
	if topFuncsN <= 0 {
		topFuncsN = 50
	}
	topFuncs := parser.GetTopFunctions(topFuncsN, pprofparser.SampleTypeGoroutine, false)

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

	// Step 5: Generate flame graph
	var flameGraphFile string
	var outputFiles []model.OutputFile

	samples, err := parser.ToSamples(pprofparser.SampleTypeGoroutine)
	if err == nil && len(samples) > 0 {
		fg, err := a.GenerateFlameGraphWithAnalysis(ctx, samples)
		if err == nil {
			flameGraphFile = filepath.Join(taskDir, "goroutine_flamegraph.json.gz")
			if err := a.WriteFlameGraphGzip(fg, flameGraphFile); err == nil {
				outputFiles = append(outputFiles, model.OutputFile{
					Name:        "Goroutine Flame Graph",
					LocalPath:   flameGraphFile,
					COSKey:      req.TaskUUID + "/goroutine_flamegraph.json.gz",
					ContentType: "application/gzip",
				})
			}
		}
	}

	// Step 6: Build PProfGoroutineData
	goroutineData := &model.PProfGoroutineData{
		TotalCount:     totalCount,
		Distribution:   distribution,
		TopFuncs:       pprofTopFuncs,
		FlameGraphFile: flameGraphFile,
	}

	// Step 7: Build response
	return &model.AnalysisResponse{
		TaskUUID:     req.TaskUUID,
		TaskType:     req.TaskType,
		TotalRecords: int(totalCount),
		OutputFiles:  outputFiles,
		Data:         goroutineData,
	}, nil
}

// analyzeGoroutineDistribution analyzes the distribution of goroutines by stack.
func (a *PProfGoroutineAnalyzer) analyzeGoroutineDistribution(prof *profile.Profile) ([]model.GoroutineGroup, int64) {
	// Find the sample type index for goroutine count
	sampleIdx := 0
	for i, st := range prof.SampleType {
		if st.Type == "goroutine" || st.Type == "count" {
			sampleIdx = i
			break
		}
	}

	// Group by stack signature
	type stackGroup struct {
		count   int64
		topFunc string
		stack   []string
	}
	groups := make(map[string]*stackGroup)
	var totalCount int64

	for _, sample := range prof.Sample {
		count := sample.Value[sampleIdx]
		if count == 0 {
			continue
		}
		totalCount += count

		// Build stack signature and extract top function
		var stackParts []string
		var topFunc string
		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				if line.Function != nil {
					funcName := line.Function.Name
					if topFunc == "" {
						topFunc = funcName
					}
					stackParts = append(stackParts, funcName)
				}
			}
		}

		signature := strings.Join(stackParts, ";")
		if g, ok := groups[signature]; ok {
			g.count += count
		} else {
			groups[signature] = &stackGroup{
				count:   count,
				topFunc: topFunc,
				stack:   stackParts,
			}
		}
	}

	// Convert to slice and sort by count
	result := make([]model.GoroutineGroup, 0, len(groups))
	for _, g := range groups {
		var pct float64
		if totalCount > 0 {
			pct = float64(g.count) * 100.0 / float64(totalCount)
		}
		result = append(result, model.GoroutineGroup{
			Count:      g.count,
			Percentage: pct,
			TopFunc:    g.topFunc,
			Stack:      g.stack,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	// Limit to top 100 groups
	if len(result) > 100 {
		result = result[:100]
	}

	return result, totalCount
}
