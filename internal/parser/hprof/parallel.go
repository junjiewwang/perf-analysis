// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/perf-analysis/pkg/utils"
	"golang.org/x/sync/errgroup"
)

// ParallelConfig configures parallel analysis behavior.
type ParallelConfig struct {
	// Enabled controls whether parallel analysis is enabled.
	// If false, all analysis runs sequentially.
	Enabled bool

	// MaxWorkers is the maximum number of concurrent workers.
	// Default: runtime.NumCPU()
	MaxWorkers int

	// RetainerWorkers is the number of workers for retainer analysis.
	// Default: MaxWorkers
	RetainerWorkers int

	// GraphWorkers is the number of workers for reference graph generation.
	// Default: MaxWorkers / 2 (graph generation is memory intensive)
	GraphWorkers int

	// Timeout is the maximum time for the entire analysis.
	// Default: 5 minutes. Set to 0 for no timeout.
	Timeout time.Duration

	// ProgressCallback is called periodically with progress updates.
	// Can be nil if progress reporting is not needed.
	ProgressCallback func(phase string, current, total int)
}

// DefaultParallelConfig returns the default parallel configuration.
func DefaultParallelConfig() ParallelConfig {
	numCPU := runtime.NumCPU()
	return ParallelConfig{
		Enabled:         true,
		MaxWorkers:      numCPU,
		RetainerWorkers: numCPU,
		GraphWorkers:    max(1, numCPU/2),
		Timeout:         5 * time.Minute,
	}
}

// AnalysisTask represents a unit of work for parallel analysis.
type AnalysisTask struct {
	ClassName string
	ClassInfo *ClassStats
}

// RetainerResult holds the result of retainer analysis for a class.
type RetainerResult struct {
	ClassName string
	Retainers *ClassRetainers
	Error     error
}

// GraphResult holds the result of reference graph generation for a class.
type GraphResult struct {
	ClassName string
	GraphData *ReferenceGraphData
	Error     error
}

// BusinessRetainerResult holds the result of business retainer analysis.
type BusinessRetainerResult struct {
	ClassName string
	Retainers []*BusinessRetainer
	Error     error
}

// ParallelAnalyzer performs parallel analysis on heap data.
type ParallelAnalyzer struct {
	config   ParallelConfig
	refGraph *ReferenceGraph
	logger   utils.Logger
}

// NewParallelAnalyzer creates a new parallel analyzer.
func NewParallelAnalyzer(refGraph *ReferenceGraph, config ParallelConfig) *ParallelAnalyzer {
	if config.MaxWorkers <= 0 {
		config.MaxWorkers = runtime.NumCPU()
	}
	if config.RetainerWorkers <= 0 {
		config.RetainerWorkers = config.MaxWorkers
	}
	if config.GraphWorkers <= 0 {
		config.GraphWorkers = max(1, config.MaxWorkers/2)
	}

	return &ParallelAnalyzer{
		config:   config,
		refGraph: refGraph,
		logger:   refGraph.logger, // Inherit logger from refGraph
	}
}

// debugf logs a debug message if logger is configured.
func (pa *ParallelAnalyzer) debugf(format string, args ...interface{}) {
	if pa.logger != nil {
		pa.logger.Debug(format, args...)
	}
}

// AnalyzeRetainersParallel analyzes retainers for multiple classes in parallel.
func (pa *ParallelAnalyzer) AnalyzeRetainersParallel(ctx context.Context, classes []*ClassStats, topN int) map[string]*ClassRetainers {
	if !pa.config.Enabled || len(classes) == 0 {
		return pa.analyzeRetainersSequential(classes, topN)
	}

	results := make(map[string]*ClassRetainers)
	var mu sync.Mutex

	// Create task channel
	tasks := make(chan *ClassStats, len(classes))
	for _, cls := range classes {
		tasks <- cls
	}
	close(tasks)

	// Create worker pool
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(pa.config.RetainerWorkers)

	for i := 0; i < pa.config.RetainerWorkers; i++ {
		g.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case cls, ok := <-tasks:
					if !ok {
						return nil
					}
					// Perform retainer analysis
					retainers := pa.refGraph.ComputeMultiLevelRetainers(cls.ClassName, 5, topN)
					if retainers != nil && len(retainers.Retainers) > 0 {
						retainers.RetainedSize = pa.refGraph.GetClassRetainedSize(cls.ClassName)
						mu.Lock()
						results[cls.ClassName] = retainers
						mu.Unlock()
					}
				}
			}
		})
	}

	_ = g.Wait() // Errors are logged but don't stop other workers
	return results
}

// analyzeRetainersSequential is the fallback sequential implementation.
func (pa *ParallelAnalyzer) analyzeRetainersSequential(classes []*ClassStats, topN int) map[string]*ClassRetainers {
	results := make(map[string]*ClassRetainers)
	for _, cls := range classes {
		retainers := pa.refGraph.ComputeMultiLevelRetainers(cls.ClassName, 5, topN)
		if retainers != nil && len(retainers.Retainers) > 0 {
			retainers.RetainedSize = pa.refGraph.GetClassRetainedSize(cls.ClassName)
			results[cls.ClassName] = retainers
		}
	}
	return results
}

// GenerateGraphsParallel generates reference graphs for multiple classes in parallel.
func (pa *ParallelAnalyzer) GenerateGraphsParallel(ctx context.Context, classes []*ClassStats, maxDepth, maxNodes int) map[string]*ReferenceGraphData {
	if !pa.config.Enabled || len(classes) == 0 {
		return pa.generateGraphsSequential(classes, maxDepth, maxNodes)
	}

	results := make(map[string]*ReferenceGraphData)
	var mu sync.Mutex

	// Create task channel
	tasks := make(chan *ClassStats, len(classes))
	for _, cls := range classes {
		tasks <- cls
	}
	close(tasks)

	// Create worker pool with limited concurrency (graph generation is memory intensive)
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(pa.config.GraphWorkers)

	for i := 0; i < pa.config.GraphWorkers; i++ {
		g.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case cls, ok := <-tasks:
					if !ok {
						return nil
					}
					graphData := pa.refGraph.GetReferenceGraphForClass(cls.ClassName, maxDepth, maxNodes)
					if graphData != nil && len(graphData.Nodes) > 0 {
						mu.Lock()
						results[cls.ClassName] = graphData
						mu.Unlock()
					}
				}
			}
		})
	}

	_ = g.Wait()
	return results
}

// generateGraphsSequential is the fallback sequential implementation.
func (pa *ParallelAnalyzer) generateGraphsSequential(classes []*ClassStats, maxDepth, maxNodes int) map[string]*ReferenceGraphData {
	results := make(map[string]*ReferenceGraphData)
	for _, cls := range classes {
		graphData := pa.refGraph.GetReferenceGraphForClass(cls.ClassName, maxDepth, maxNodes)
		if graphData != nil && len(graphData.Nodes) > 0 {
			results[cls.ClassName] = graphData
		}
	}
	return results
}

// AnalyzeBusinessRetainersParallel analyzes business retainers for multiple classes in parallel.
func (pa *ParallelAnalyzer) AnalyzeBusinessRetainersParallel(ctx context.Context, classes []*ClassStats, maxDepth, topN int) map[string][]*BusinessRetainer {
	if !pa.config.Enabled || len(classes) == 0 {
		return pa.analyzeBusinessRetainersSequential(classes, maxDepth, topN)
	}

	results := make(map[string][]*BusinessRetainer)
	var mu sync.Mutex

	// Create task channel
	tasks := make(chan *ClassStats, len(classes))
	for _, cls := range classes {
		tasks <- cls
	}
	close(tasks)

	// Create worker pool
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(pa.config.RetainerWorkers)

	for i := 0; i < pa.config.RetainerWorkers; i++ {
		g.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case cls, ok := <-tasks:
					if !ok {
						return nil
					}
					businessRetainers := pa.refGraph.ComputeBusinessRetainers(cls.ClassName, maxDepth, topN)
					if len(businessRetainers) > 0 {
						mu.Lock()
						results[cls.ClassName] = businessRetainers
						mu.Unlock()
					}
				}
			}
		})
	}

	_ = g.Wait()
	return results
}

// analyzeBusinessRetainersSequential is the fallback sequential implementation.
func (pa *ParallelAnalyzer) analyzeBusinessRetainersSequential(classes []*ClassStats, maxDepth, topN int) map[string][]*BusinessRetainer {
	results := make(map[string][]*BusinessRetainer)
	for _, cls := range classes {
		businessRetainers := pa.refGraph.ComputeBusinessRetainers(cls.ClassName, maxDepth, topN)
		if len(businessRetainers) > 0 {
			results[cls.ClassName] = businessRetainers
		}
	}
	return results
}

// FullAnalysisResult holds all parallel analysis results.
type FullAnalysisResult struct {
	ClassRetainers    map[string]*ClassRetainers
	ReferenceGraphs   map[string]*ReferenceGraphData
	BusinessRetainers map[string][]*BusinessRetainer
	// Stats holds timing and progress statistics
	Stats AnalysisStats
}

// AnalysisStats holds statistics about the analysis run.
type AnalysisStats struct {
	TotalDuration      time.Duration
	RetainerDuration   time.Duration
	GraphDuration      time.Duration
	BusinessDuration   time.Duration
	ClassesAnalyzed    int
	GraphsGenerated    int
	BusinessAnalyzed   int
	ParallelEnabled    bool
	WorkerCount        int
}

// RunFullAnalysis runs all analysis tasks in parallel using a pipeline pattern.
// This is the main entry point for parallel heap analysis.
func (pa *ParallelAnalyzer) RunFullAnalysis(ctx context.Context, topClasses []*ClassStats, opts AnalysisOptions) *FullAnalysisResult {
	startTime := time.Now()
	
	result := &FullAnalysisResult{
		ClassRetainers:    make(map[string]*ClassRetainers),
		ReferenceGraphs:   make(map[string]*ReferenceGraphData),
		BusinessRetainers: make(map[string][]*BusinessRetainer),
	}

	// Apply timeout if configured
	if pa.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, pa.config.Timeout)
		defer cancel()
	}

	if !pa.config.Enabled {
		// Sequential fallback
		seqResult := pa.runFullAnalysisSequential(topClasses, opts)
		seqResult.Stats.TotalDuration = time.Since(startTime)
		seqResult.Stats.ParallelEnabled = false
		return seqResult
	}

	// Prepare class lists for different analysis types
	topForRetainers := topClasses
	if len(topForRetainers) > opts.MaxRetainerClasses {
		topForRetainers = topForRetainers[:opts.MaxRetainerClasses]
	}

	topForGraphs := topClasses
	if len(topForGraphs) > opts.MaxGraphClasses {
		topForGraphs = topForGraphs[:opts.MaxGraphClasses]
	}

	topForBusiness := topClasses
	if len(topForBusiness) > opts.MaxBusinessClasses {
		topForBusiness = topForBusiness[:opts.MaxBusinessClasses]
	}

	// Progress tracking
	var progress atomic.Int32
	totalTasks := len(topForRetainers) + len(topForGraphs) + len(topForBusiness)
	
	// Report progress periodically
	if pa.config.ProgressCallback != nil {
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					current := int(progress.Load())
					pa.config.ProgressCallback("analyzing", current, totalTasks)
					if current >= totalTasks {
						return
					}
				}
			}
		}()
	}

	// Timing for each phase
	var retainerDuration, graphDuration, businessDuration time.Duration
	var retainerMu, graphMu, businessMu sync.Mutex

	// Run all three analysis types concurrently
	var wg sync.WaitGroup
	wg.Add(3)

	// Retainer analysis
	go func() {
		defer wg.Done()
		phaseStart := time.Now()
		result.ClassRetainers = pa.analyzeRetainersWithProgress(ctx, topForRetainers, opts.TopRetainersN, &progress)
		retainerMu.Lock()
		retainerDuration = time.Since(phaseStart)
		retainerMu.Unlock()
	}()

	// Reference graph generation
	go func() {
		defer wg.Done()
		phaseStart := time.Now()
		result.ReferenceGraphs = pa.generateGraphsWithProgress(ctx, topForGraphs, opts.GraphMaxDepth, opts.GraphMaxNodes, &progress)
		graphMu.Lock()
		graphDuration = time.Since(phaseStart)
		graphMu.Unlock()
	}()

	// Business retainer analysis
	go func() {
		defer wg.Done()
		phaseStart := time.Now()
		result.BusinessRetainers = pa.analyzeBusinessWithProgress(ctx, topForBusiness, opts.BusinessMaxDepth, opts.TopRetainersN, &progress)
		businessMu.Lock()
		businessDuration = time.Since(phaseStart)
		businessMu.Unlock()
	}()

	wg.Wait()
	
	// Populate stats
	result.Stats = AnalysisStats{
		TotalDuration:    time.Since(startTime),
		RetainerDuration: retainerDuration,
		GraphDuration:    graphDuration,
		BusinessDuration: businessDuration,
		ClassesAnalyzed:  len(result.ClassRetainers),
		GraphsGenerated:  len(result.ReferenceGraphs),
		BusinessAnalyzed: len(result.BusinessRetainers),
		ParallelEnabled:  true,
		WorkerCount:      pa.config.MaxWorkers,
	}
	
	// Log stats
	pa.debugf("Parallel analysis completed in %v (retainer: %v, graph: %v, business: %v)",
		result.Stats.TotalDuration, retainerDuration, graphDuration, businessDuration)
	pa.debugf("Results: %d class retainers, %d graphs, %d business retainers (workers: %d)",
		result.Stats.ClassesAnalyzed, result.Stats.GraphsGenerated, result.Stats.BusinessAnalyzed, pa.config.MaxWorkers)
	
	return result
}

// analyzeRetainersWithProgress analyzes retainers with progress tracking.
func (pa *ParallelAnalyzer) analyzeRetainersWithProgress(ctx context.Context, classes []*ClassStats, topN int, progress *atomic.Int32) map[string]*ClassRetainers {
	if len(classes) == 0 {
		return make(map[string]*ClassRetainers)
	}

	results := make(map[string]*ClassRetainers)
	var mu sync.Mutex

	tasks := make(chan *ClassStats, len(classes))
	for _, cls := range classes {
		tasks <- cls
	}
	close(tasks)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(pa.config.RetainerWorkers)

	for i := 0; i < pa.config.RetainerWorkers; i++ {
		g.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case cls, ok := <-tasks:
					if !ok {
						return nil
					}
					retainers := pa.refGraph.ComputeMultiLevelRetainers(cls.ClassName, 5, topN)
					if retainers != nil && len(retainers.Retainers) > 0 {
						retainers.RetainedSize = pa.refGraph.GetClassRetainedSize(cls.ClassName)
						mu.Lock()
						results[cls.ClassName] = retainers
						mu.Unlock()
					}
					progress.Add(1)
				}
			}
		})
	}

	_ = g.Wait()
	return results
}

// generateGraphsWithProgress generates graphs with progress tracking.
func (pa *ParallelAnalyzer) generateGraphsWithProgress(ctx context.Context, classes []*ClassStats, maxDepth, maxNodes int, progress *atomic.Int32) map[string]*ReferenceGraphData {
	if len(classes) == 0 {
		return make(map[string]*ReferenceGraphData)
	}

	results := make(map[string]*ReferenceGraphData)
	var mu sync.Mutex

	tasks := make(chan *ClassStats, len(classes))
	for _, cls := range classes {
		tasks <- cls
	}
	close(tasks)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(pa.config.GraphWorkers)

	for i := 0; i < pa.config.GraphWorkers; i++ {
		g.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case cls, ok := <-tasks:
					if !ok {
						return nil
					}
					graphData := pa.refGraph.GetReferenceGraphForClass(cls.ClassName, maxDepth, maxNodes)
					if graphData != nil && len(graphData.Nodes) > 0 {
						mu.Lock()
						results[cls.ClassName] = graphData
						mu.Unlock()
					}
					progress.Add(1)
				}
			}
		})
	}

	_ = g.Wait()
	return results
}

// analyzeBusinessWithProgress analyzes business retainers with progress tracking.
func (pa *ParallelAnalyzer) analyzeBusinessWithProgress(ctx context.Context, classes []*ClassStats, maxDepth, topN int, progress *atomic.Int32) map[string][]*BusinessRetainer {
	if len(classes) == 0 {
		return make(map[string][]*BusinessRetainer)
	}

	results := make(map[string][]*BusinessRetainer)
	var mu sync.Mutex

	tasks := make(chan *ClassStats, len(classes))
	for _, cls := range classes {
		tasks <- cls
	}
	close(tasks)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(pa.config.RetainerWorkers)

	for i := 0; i < pa.config.RetainerWorkers; i++ {
		g.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case cls, ok := <-tasks:
					if !ok {
						return nil
					}
					businessRetainers := pa.refGraph.ComputeBusinessRetainers(cls.ClassName, maxDepth, topN)
					if len(businessRetainers) > 0 {
						mu.Lock()
						results[cls.ClassName] = businessRetainers
						mu.Unlock()
					}
					progress.Add(1)
				}
			}
		})
	}

	_ = g.Wait()
	return results
}

// runFullAnalysisSequential runs all analysis sequentially.
func (pa *ParallelAnalyzer) runFullAnalysisSequential(topClasses []*ClassStats, opts AnalysisOptions) *FullAnalysisResult {
	startTime := time.Now()
	
	result := &FullAnalysisResult{
		ClassRetainers:    make(map[string]*ClassRetainers),
		ReferenceGraphs:   make(map[string]*ReferenceGraphData),
		BusinessRetainers: make(map[string][]*BusinessRetainer),
	}

	// Retainer analysis
	retainerStart := time.Now()
	topForRetainers := topClasses
	if len(topForRetainers) > opts.MaxRetainerClasses {
		topForRetainers = topForRetainers[:opts.MaxRetainerClasses]
	}
	result.ClassRetainers = pa.analyzeRetainersSequential(topForRetainers, opts.TopRetainersN)
	retainerDuration := time.Since(retainerStart)

	// Reference graphs
	graphStart := time.Now()
	topForGraphs := topClasses
	if len(topForGraphs) > opts.MaxGraphClasses {
		topForGraphs = topForGraphs[:opts.MaxGraphClasses]
	}
	result.ReferenceGraphs = pa.generateGraphsSequential(topForGraphs, opts.GraphMaxDepth, opts.GraphMaxNodes)
	graphDuration := time.Since(graphStart)

	// Business retainers
	businessStart := time.Now()
	topForBusiness := topClasses
	if len(topForBusiness) > opts.MaxBusinessClasses {
		topForBusiness = topForBusiness[:opts.MaxBusinessClasses]
	}
	result.BusinessRetainers = pa.analyzeBusinessRetainersSequential(topForBusiness, opts.BusinessMaxDepth, opts.TopRetainersN)
	businessDuration := time.Since(businessStart)

	// Populate stats
	result.Stats = AnalysisStats{
		TotalDuration:    time.Since(startTime),
		RetainerDuration: retainerDuration,
		GraphDuration:    graphDuration,
		BusinessDuration: businessDuration,
		ClassesAnalyzed:  len(result.ClassRetainers),
		GraphsGenerated:  len(result.ReferenceGraphs),
		BusinessAnalyzed: len(result.BusinessRetainers),
		ParallelEnabled:  false,
		WorkerCount:      1,
	}

	pa.debugf("Sequential analysis completed in %v (retainer: %v, graph: %v, business: %v)",
		result.Stats.TotalDuration, retainerDuration, graphDuration, businessDuration)

	return result
}

// AnalysisOptions configures analysis parameters.
type AnalysisOptions struct {
	// MaxRetainerClasses is the max number of classes for retainer analysis.
	MaxRetainerClasses int
	// MaxGraphClasses is the max number of classes for graph generation.
	MaxGraphClasses int
	// MaxBusinessClasses is the max number of classes for business retainer analysis.
	MaxBusinessClasses int
	// TopRetainersN is the max number of retainers per class.
	TopRetainersN int
	// GraphMaxDepth is the max depth for reference graph.
	GraphMaxDepth int
	// GraphMaxNodes is the max nodes for reference graph.
	GraphMaxNodes int
	// BusinessMaxDepth is the max depth for business retainer search.
	BusinessMaxDepth int
}

// DefaultAnalysisOptions returns default analysis options.
func DefaultAnalysisOptions() AnalysisOptions {
	return AnalysisOptions{
		MaxRetainerClasses: 20,
		MaxGraphClasses:    5,
		MaxBusinessClasses: 10,
		TopRetainersN:      10,
		GraphMaxDepth:      10,
		GraphMaxNodes:      100,
		BusinessMaxDepth:   15,
	}
}
