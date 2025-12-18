// Package hprof provides parsing functionality for Java HPROF heap dump files.
// This file contains the result building logic for heap analysis.
package hprof

import (
	"context"
	"sort"

	"github.com/perf-analysis/pkg/utils"
)

// ResultBuilder builds the final HeapAnalysisResult from parsed state.
// This separates the result construction logic from the parsing logic.
type ResultBuilder struct {
	state  *parserState
	opts   *ParserOptions
	timer  *utils.Timer
	logger utils.Logger
}

// NewResultBuilder creates a new ResultBuilder.
func NewResultBuilder(state *parserState, opts *ParserOptions, timer *utils.Timer) *ResultBuilder {
	return &ResultBuilder{
		state:  state,
		opts:   opts,
		timer:  timer,
		logger: opts.Logger,
	}
}

// debugf logs a debug message if logger is configured.
func (rb *ResultBuilder) debugf(format string, args ...interface{}) {
	if rb.logger != nil {
		rb.logger.Debug(format, args...)
	}
}

// Build constructs the HeapAnalysisResult from the parsed state.
func (rb *ResultBuilder) Build() *HeapAnalysisResult {
	// Compute dominator tree first if retainer analysis is enabled
	rb.computeDominatorTree()

	// Collect class statistics
	classes, totalHeapSize, totalInstances := rb.collectClassStatistics()

	// Limit to top N
	topClasses := rb.limitTopClasses(classes)

	// Build base result
	result := &HeapAnalysisResult{
		Header:         rb.state.header,
		Summary:        rb.state.heapSummary,
		TopClasses:     topClasses,
		TotalClasses:   len(rb.state.classByName),
		TotalInstances: totalInstances,
		TotalHeapSize:  totalHeapSize,
	}

	// Compute retainer analysis and reference graphs
	rb.computeRetainerAnalysis(result, topClasses)

	// Build BiggestObjects
	rb.buildBiggestObjects(result)

	return result
}

// computeDominatorTree computes the dominator tree if retainer analysis is enabled.
func (rb *ResultBuilder) computeDominatorTree() {
	if rb.state.refGraph == nil || !rb.opts.AnalyzeRetainers {
		return
	}

	// Debug: print parsing stats
	rb.debugf("Parsing stats: loadClass=%d, classDump=%d, instanceDump=%d, arrayDump=%d",
		rb.state.loadClassCount, rb.state.classDumpCount, rb.state.instanceDumpCount, rb.state.arrayDumpCount)
	rb.debugf("Unknown tags: %d, skipped bytes: %d", rb.state.unknownTagCount, rb.state.skippedBytes)

	// Debug: print reference graph stats
	objects, refs, gcRoots, objectsWithIncoming := rb.state.refGraph.GetStats()
	rb.debugf("Reference graph stats: objects=%d, refs=%d, gcRoots=%d, objectsWithIncoming=%d",
		objects, refs, gcRoots, objectsWithIncoming)

	// Debug: check class field info
	classesWithFields := 0
	totalFields := 0
	for _, fields := range rb.state.classFields {
		if len(fields) > 0 {
			classesWithFields++
			totalFields += len(fields)
		}
	}
	rb.debugf("Classes with field info: %d, total fields: %d", classesWithFields, totalFields)
	rb.debugf("ClassInfo entries: %d, ClassFields entries: %d", len(rb.state.classInfo), len(rb.state.classFields))

	// Compute dominator tree to get retained sizes
	rb.timer.TimeFunc("Dominator tree computation", func() {
		rb.state.refGraph.ComputeDominatorTree()
	})
}

// collectClassStatistics collects class statistics from the parsed state.
func (rb *ResultBuilder) collectClassStatistics() ([]*ClassStats, int64, int64) {
	var classes []*ClassStats
	var totalHeapSize int64
	var totalInstances int64

	rb.timer.TimeFunc("Class statistics collection", func() {
		if rb.state.refGraph != nil && rb.opts.AnalyzeRetainers {
			classes, totalHeapSize, totalInstances = rb.collectFromRefGraph()
		} else {
			classes, totalHeapSize, totalInstances = rb.collectFromClassByName()
		}

		// Sort by total size descending
		sort.Slice(classes, func(i, j int) bool {
			return classes[i].TotalSize > classes[j].TotalSize
		})
	})

	return classes, totalHeapSize, totalInstances
}

// collectFromRefGraph collects statistics from the reference graph.
func (rb *ResultBuilder) collectFromRefGraph() ([]*ClassStats, int64, int64) {
	var classes []*ClassStats
	var totalHeapSize int64
	var totalInstances int64

	// Get reachable stats for debug output
	reachableHeapSize := rb.state.refGraph.GetTotalReachableHeapSize()
	reachableInstances := int64(rb.state.refGraph.GetReachableObjectCount())

	rb.debugf("Reachable objects: %d (total parsed: %d, diff: %d)",
		reachableInstances, rb.state.totalInstances, rb.state.totalInstances-reachableInstances)
	rb.debugf("Reachable heap size: %d (total parsed: %d, diff: %d)",
		reachableHeapSize, rb.state.totalHeapSize, rb.state.totalHeapSize-reachableHeapSize)

	if rb.opts.IncludeUnreachable {
		// Use ALL objects (like IDEA)
		allStats := rb.state.refGraph.GetAllClassStats()
		totalHeapSize = rb.state.totalHeapSize
		totalInstances = rb.state.totalInstances

		rb.debugf("Using all objects mode (like IDEA): %d instances, %d bytes",
			totalInstances, totalHeapSize)

		classes = rb.buildClassStatsFromMap(allStats, totalHeapSize)
	} else {
		// Use reachable objects only (like MAT)
		reachableStats := rb.state.refGraph.GetReachableClassStats()
		totalHeapSize = reachableHeapSize
		totalInstances = reachableInstances

		rb.debugf("Using reachable objects mode (like MAT): %d instances, %d bytes",
			totalInstances, totalHeapSize)

		classes = rb.buildClassStatsFromMap(reachableStats, totalHeapSize)
	}

	return classes, totalHeapSize, totalInstances
}

// buildClassStatsFromMap builds ClassStats slice from a stats map.
func (rb *ResultBuilder) buildClassStatsFromMap(statsMap map[uint64]struct {
	InstanceCount int64
	TotalSize     int64
}, totalHeapSize int64) []*ClassStats {
	var classes []*ClassStats

	for classID, stats := range statsMap {
		className := rb.state.refGraph.GetClassName(classID)
		if className == "" {
			continue
		}

		avgSize := float64(0)
		if stats.InstanceCount > 0 {
			avgSize = float64(stats.TotalSize) / float64(stats.InstanceCount)
		}
		pct := float64(0)
		if totalHeapSize > 0 {
			pct = float64(stats.TotalSize) * 100.0 / float64(totalHeapSize)
		}

		// Get retained size from dominator tree
		retainedSize := rb.state.refGraph.GetClassRetainedSize(className)

		classes = append(classes, &ClassStats{
			ClassName:     className,
			InstanceCount: stats.InstanceCount,
			TotalSize:     stats.TotalSize,
			AvgSize:       avgSize,
			Percentage:    pct,
			ShallowSize:   stats.TotalSize,
			RetainedSize:  retainedSize,
		})
	}

	return classes
}

// collectFromClassByName collects statistics from classByName map (fallback).
func (rb *ResultBuilder) collectFromClassByName() ([]*ClassStats, int64, int64) {
	var classes []*ClassStats
	totalHeapSize := rb.state.totalHeapSize
	totalInstances := rb.state.totalInstances

	for _, info := range rb.state.classByName {
		if info.InstanceCount > 0 {
			avgSize := float64(0)
			if info.InstanceCount > 0 {
				avgSize = float64(info.TotalSize) / float64(info.InstanceCount)
			}
			pct := float64(0)
			if rb.state.totalHeapSize > 0 {
				pct = float64(info.TotalSize) * 100.0 / float64(rb.state.totalHeapSize)
			}

			// Get retained size from dominator tree if available
			var retainedSize int64
			if rb.state.refGraph != nil {
				retainedSize = rb.state.refGraph.GetClassRetainedSize(info.Name)
			}

			classes = append(classes, &ClassStats{
				ClassName:     info.Name,
				InstanceCount: info.InstanceCount,
				TotalSize:     info.TotalSize,
				AvgSize:       avgSize,
				Percentage:    pct,
				ShallowSize:   info.TotalSize,
				RetainedSize:  retainedSize,
			})
		}
	}

	return classes, totalHeapSize, totalInstances
}

// limitTopClasses limits the classes to top N.
func (rb *ResultBuilder) limitTopClasses(classes []*ClassStats) []*ClassStats {
	if rb.opts.TopClassesN > 0 && len(classes) > rb.opts.TopClassesN {
		return classes[:rb.opts.TopClassesN]
	}
	return classes
}

// computeRetainerAnalysis computes retainer analysis and reference graphs.
func (rb *ResultBuilder) computeRetainerAnalysis(result *HeapAnalysisResult, topClasses []*ClassStats) {
	if rb.state.refGraph == nil || !rb.opts.AnalyzeRetainers || rb.opts.FastMode {
		return
	}

	rb.timer.TimeFunc("Parallel analysis (retainers/graphs/business)", func() {
		// Use parallel analyzer for better performance
		analyzer := NewParallelAnalyzer(rb.state.refGraph, rb.opts.ParallelConfig)

		analysisOpts := AnalysisOptions{
			MaxRetainerClasses: 20,
			MaxGraphClasses:    5,
			MaxBusinessClasses: 10,
			TopRetainersN:      rb.opts.TopRetainersN,
			GraphMaxDepth:      10,
			GraphMaxNodes:      100,
			BusinessMaxDepth:   15,
		}

		// Skip business retainers if configured (they are the most expensive)
		if rb.opts.SkipBusinessRetainers {
			analysisOpts.MaxBusinessClasses = 0
		}

		// Run all analysis in parallel
		ctx := context.Background()
		analysisResult := analyzer.RunFullAnalysis(ctx, topClasses, analysisOpts)

		result.ClassRetainers = analysisResult.ClassRetainers
		result.ReferenceGraphs = analysisResult.ReferenceGraphs
		result.BusinessRetainers = analysisResult.BusinessRetainers
	})
}

// buildBiggestObjects builds the BiggestObjects analysis.
func (rb *ResultBuilder) buildBiggestObjects(result *HeapAnalysisResult) {
	if rb.state.refGraph == nil || !rb.opts.AnalyzeRetainers {
		return
	}

	rb.timer.TimeFunc("Biggest objects analysis", func() {
		builder := NewBiggestObjectsBuilder(rb.state.refGraph, rb.state.classLayouts, rb.state.strings)
		result.BiggestObjects = builder.GetBiggestObjectsByRetainedSize(rb.opts.MaxLargestObjects)
		// Store class layouts and strings for later use (e.g., API queries)
		result.ClassLayouts = rb.state.classLayouts
		result.Strings = rb.state.strings
		// Store reference graph for serialization and advanced analysis
		result.RefGraph = rb.state.refGraph

		// Debug: Analyze ClassLoader retained size differences (only in verbose mode)
		if rb.opts.Verbose {
			builder.DebugClassLoaderRetainedSize("com.taobao.arthas.agent.ArthasClassloader")
		}
	})
}
