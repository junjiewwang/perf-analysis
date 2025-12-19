package callgraph

import (
	"context"
	"sort"
	"strings"

	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/profiling"
)

// GeneratorOptions holds configuration options for the call graph generator.
type GeneratorOptions struct {
	// MinNodePct is the minimum percentage for a node to be included.
	MinNodePct float64

	// MinEdgePct is the minimum percentage for an edge to be included.
	MinEdgePct float64

	// IncludeModule includes module information in the output.
	IncludeModule bool

	// EnableThreadAnalysis enables thread-level call graph generation.
	EnableThreadAnalysis bool

	// EnableHotPathAnalysis enables hot path detection.
	EnableHotPathAnalysis bool

	// EnableModuleAnalysis enables module-level aggregation.
	EnableModuleAnalysis bool

	// TopNFunctions specifies how many top functions to include in analysis.
	TopNFunctions int

	// TopNHotPaths specifies how many hot paths to include.
	TopNHotPaths int

	// MaxThreadCallGraphs limits the number of per-thread call graphs.
	MaxThreadCallGraphs int

	// IncludeSwapper includes swapper (idle) threads in analysis.
	IncludeSwapper bool
}

// DefaultGeneratorOptions returns default generator options.
func DefaultGeneratorOptions() *GeneratorOptions {
	return &GeneratorOptions{
		MinNodePct:           0.5,  // 0.5% minimum for nodes
		MinEdgePct:           0.1,  // 0.1% minimum for edges
		IncludeModule:        true,
		EnableThreadAnalysis: true,
		EnableHotPathAnalysis: true,
		EnableModuleAnalysis: true,
		TopNFunctions:        20,
		TopNHotPaths:         10,
		MaxThreadCallGraphs:  50,
		IncludeSwapper:       false,
	}
}

// Generator generates call graph data from parsed samples.
type Generator struct {
	opts *GeneratorOptions
}

// NewGenerator creates a new call graph generator.
func NewGenerator(opts *GeneratorOptions) *Generator {
	if opts == nil {
		opts = DefaultGeneratorOptions()
	}
	return &Generator{opts: opts}
}

// threadData holds intermediate data for a single thread during analysis.
type threadData struct {
	tid         int
	name        string
	group       string
	samples     int64
	isSwapper   bool
	callGraph   *ThreadCallGraph
	selfTime    map[string]int64
	hotPaths    map[string]int64 // path string -> count
	funcInStack map[string]bool  // tracks functions in current stack for recursion detection
}

// Generate generates a call graph from the given samples.
func (g *Generator) Generate(ctx context.Context, samples []*model.Sample) (*CallGraph, error) {
	cg := NewCallGraph()

	if !g.opts.EnableThreadAnalysis {
		return g.generateSimple(ctx, cg, samples)
	}

	return g.generateWithAnalysis(ctx, cg, samples)
}

// generateSimple generates a basic call graph without thread analysis.
func (g *Generator) generateSimple(ctx context.Context, cg *CallGraph, samples []*model.Sample) (*CallGraph, error) {
	selfTime := make(map[string]int64)

	for _, sample := range samples {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if len(sample.CallStack) == 0 {
			continue
		}

		cg.TotalSamples += sample.Value
		g.processSampleNoTotal(cg, sample, selfTime, nil)
	}

	// Update self times
	for nodeID, time := range selfTime {
		if node := cg.nodeMap[nodeID]; node != nil {
			node.SelfTime = time
		}
	}

	cg.CalculatePercentages()
	cg.Cleanup(g.opts.MinNodePct, g.opts.MinEdgePct)

	return cg, nil
}

// generateWithAnalysis generates a call graph with full analysis.
func (g *Generator) generateWithAnalysis(ctx context.Context, cg *CallGraph, samples []*model.Sample) (*CallGraph, error) {
	// Intermediate storage
	threads := make(map[int]*threadData)
	globalSelfTime := make(map[string]int64)
	globalHotPaths := make(map[string]int64)
	moduleStats := make(map[string]*moduleData)
	recursiveFuncs := make(map[string]bool)
	maxDepth := 0
	var totalSamples, totalSamplesWithSwapper int64

	// Process samples
	for _, sample := range samples {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if len(sample.CallStack) == 0 {
			continue
		}

		isSwapper := profiling.IsSwapperThread(sample.ThreadName)
		totalSamplesWithSwapper += sample.Value
		if !isSwapper {
			totalSamples += sample.Value
		}

		// Track max depth
		if len(sample.CallStack) > maxDepth {
			maxDepth = len(sample.CallStack)
		}

		// Get or create thread data
		td, ok := threads[sample.TID]
		if !ok {
			td = &threadData{
				tid:         sample.TID,
				name:        sample.ThreadName,
				group:       profiling.ExtractThreadGroup(sample.ThreadName),
				isSwapper:   isSwapper,
				selfTime:    make(map[string]int64),
				hotPaths:    make(map[string]int64),
				funcInStack: make(map[string]bool),
			}
			if g.opts.EnableThreadAnalysis {
				td.callGraph = NewThreadCallGraph(sample.TID, sample.ThreadName)
				td.callGraph.ThreadGroup = td.group
			}
			threads[sample.TID] = td
		}

		td.samples += sample.Value

		// Process sample for global call graph (don't add to TotalSamples here, we track it separately)
		funcInStack := make(map[string]bool)
		g.processSampleNoTotal(cg, sample, globalSelfTime, funcInStack)

		// Detect recursion
		for funcID := range funcInStack {
			if recursiveFuncs[funcID] {
				continue
			}
			// Check if function appears multiple times in stack
			count := 0
			for _, frame := range sample.CallStack {
				fn, mod := g.splitFuncAndModule(frame)
				if makeNodeID(fn, mod) == funcID {
					count++
				}
			}
			if count > 1 {
				recursiveFuncs[funcID] = true
			}
		}

		// Process for thread-specific call graph
		if td.callGraph != nil && (!isSwapper || g.opts.IncludeSwapper) {
			td.callGraph.TotalSamples += sample.Value
			g.processThreadSample(td, sample)
		}

		// Track hot paths
		if g.opts.EnableHotPathAnalysis && (!isSwapper || g.opts.IncludeSwapper) {
			pathStr := strings.Join(sample.CallStack, ";")
			globalHotPaths[pathStr] += sample.Value
			td.hotPaths[pathStr] += sample.Value
		}

		// Track module stats
		if g.opts.EnableModuleAnalysis && (!isSwapper || g.opts.IncludeSwapper) {
			g.updateModuleStats(moduleStats, sample)
		}
	}

	cg.TotalSamples = totalSamples

	// Update self times and recursion flags
	for nodeID, time := range globalSelfTime {
		if node := cg.nodeMap[nodeID]; node != nil {
			node.SelfTime = time
			node.IsRecursive = recursiveFuncs[nodeID]
		}
	}

	// Calculate percentages (keep maps for analysis)
	cg.CalculatePercentages()

	// Build analysis data
	cg.Analysis = &CallGraphAnalysis{
		TotalSamples:   totalSamples,
		TotalThreads:   len(threads),
		TotalFunctions: len(cg.Nodes),
		TotalEdges:     len(cg.Edges),
		MaxCallDepth:   maxDepth,
	}

	// Count recursive functions
	for _, isRec := range recursiveFuncs {
		if isRec {
			cg.Analysis.RecursiveFunctions++
		}
	}

	// Build top functions
	cg.Analysis.TopFunctionsBySelf = cg.GetTopFunctionsBySelf(g.opts.TopNFunctions)
	cg.Analysis.TopFunctionsByTotal = cg.GetTopFunctionsByTotal(g.opts.TopNFunctions)

	// Build hot paths
	if g.opts.EnableHotPathAnalysis {
		cg.Analysis.HotPaths = g.buildHotPaths(globalHotPaths, totalSamples)
	}

	// Build module analysis
	if g.opts.EnableModuleAnalysis {
		cg.Analysis.ModuleAnalysis = g.buildModuleAnalysis(moduleStats, totalSamples)
	}

	// Build thread group analysis
	cg.Analysis.ThreadGroupAnalysis = g.buildThreadGroupAnalysis(threads, totalSamples)

	// Build per-thread call graphs
	if g.opts.EnableThreadAnalysis {
		cg.Analysis.ThreadCallGraphs = g.buildThreadCallGraphs(threads, totalSamplesWithSwapper)
	}

	// Cleanup but keep analysis
	cg.CleanupKeepMaps(g.opts.MinNodePct, g.opts.MinEdgePct)

	return cg, nil
}

// processSample processes a single sample for the call graph (also updates TotalSamples).
func (g *Generator) processSample(cg *CallGraph, sample *model.Sample, selfTime map[string]int64, funcInStack map[string]bool) {
	cg.TotalSamples += sample.Value
	g.processSampleNoTotal(cg, sample, selfTime, funcInStack)
}

// processSampleNoTotal processes a single sample without updating TotalSamples.
func (g *Generator) processSampleNoTotal(cg *CallGraph, sample *model.Sample, selfTime map[string]int64, funcInStack map[string]bool) {
	for i, frame := range sample.CallStack {
		function, module := g.splitFuncAndModule(frame)
		nodeID := makeNodeID(function, module)

		// Track function in stack for recursion detection
		if funcInStack != nil {
			funcInStack[nodeID] = true
		}

		// Add node (accumulate total time)
		node := cg.AddNode(function, module, 0, sample.Value)

		// Track max depth
		if i+1 > node.MaxDepth {
			node.MaxDepth = i + 1
		}

		// Track self time (only for leaf/top function)
		if i == len(sample.CallStack)-1 {
			selfTime[nodeID] += sample.Value
		}

		// Add edge from caller to callee
		if i > 0 {
			callerFrame := sample.CallStack[i-1]
			callerFunc, callerModule := g.splitFuncAndModule(callerFrame)
			cg.AddEdge(callerFunc, callerModule, function, module, sample.Value)
		}
	}
}

// processThreadSample processes a sample for a thread-specific call graph.
func (g *Generator) processThreadSample(td *threadData, sample *model.Sample) {
	tcg := td.callGraph

	for i, frame := range sample.CallStack {
		function, module := g.splitFuncAndModule(frame)
		nodeID := makeNodeID(function, module)

		// Add node
		tcg.AddNode(function, module, 0, sample.Value)

		// Track self time
		if i == len(sample.CallStack)-1 {
			td.selfTime[nodeID] += sample.Value
		}

		// Add edge
		if i > 0 {
			callerFrame := sample.CallStack[i-1]
			callerFunc, callerModule := g.splitFuncAndModule(callerFrame)
			tcg.AddEdge(callerFunc, callerModule, function, module, sample.Value)
		}
	}
}

// moduleData holds intermediate module statistics.
type moduleData struct {
	module      string
	totalTime   int64
	selfTime    int64
	functions   map[string]int64 // function -> samples
}

// updateModuleStats updates module-level statistics.
func (g *Generator) updateModuleStats(moduleStats map[string]*moduleData, sample *model.Sample) {
	for i, frame := range sample.CallStack {
		_, module := g.splitFuncAndModule(frame)
		if module == "" {
			module = "[unknown]"
		}

		md, ok := moduleStats[module]
		if !ok {
			md = &moduleData{
				module:    module,
				functions: make(map[string]int64),
			}
			moduleStats[module] = md
		}

		md.totalTime += sample.Value
		md.functions[frame] += sample.Value

		// Self time only for leaf
		if i == len(sample.CallStack)-1 {
			md.selfTime += sample.Value
		}
	}
}

// buildHotPaths builds the hot paths list.
func (g *Generator) buildHotPaths(hotPaths map[string]int64, totalSamples int64) []*HotPath {
	type pathEntry struct {
		path    string
		samples int64
	}

	entries := make([]pathEntry, 0, len(hotPaths))
	for path, samples := range hotPaths {
		entries = append(entries, pathEntry{path: path, samples: samples})
	}

	// Sort by samples descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].samples > entries[j].samples
	})

	n := g.opts.TopNHotPaths
	if n > len(entries) {
		n = len(entries)
	}

	result := make([]*HotPath, n)
	for i := 0; i < n; i++ {
		e := entries[i]
		pathParts := strings.Split(e.path, ";")
		pct := float64(0)
		if totalSamples > 0 {
			pct = float64(e.samples) / float64(totalSamples) * 100
		}
		result[i] = &HotPath{
			Path:       pathParts,
			Samples:    e.samples,
			Percentage: pct,
			Depth:      len(pathParts),
		}
	}

	return result
}

// buildModuleAnalysis builds module-level analysis.
func (g *Generator) buildModuleAnalysis(moduleStats map[string]*moduleData, totalSamples int64) []*ModuleAnalysis {
	result := make([]*ModuleAnalysis, 0, len(moduleStats))

	for _, md := range moduleStats {
		totalPct := float64(0)
		selfPct := float64(0)
		if totalSamples > 0 {
			totalPct = float64(md.totalTime) / float64(totalSamples) * 100
			selfPct = float64(md.selfTime) / float64(totalSamples) * 100
		}

		// Get top functions in this module
		type funcEntry struct {
			name    string
			samples int64
		}
		funcEntries := make([]funcEntry, 0, len(md.functions))
		for name, samples := range md.functions {
			funcEntries = append(funcEntries, funcEntry{name: name, samples: samples})
		}
		sort.Slice(funcEntries, func(i, j int) bool {
			return funcEntries[i].samples > funcEntries[j].samples
		})

		topN := 5
		if topN > len(funcEntries) {
			topN = len(funcEntries)
		}
		topFuncs := make([]string, topN)
		for i := 0; i < topN; i++ {
			topFuncs[i] = funcEntries[i].name
		}

		result = append(result, &ModuleAnalysis{
			Module:        md.module,
			FunctionCount: len(md.functions),
			TotalSamples:  md.totalTime,
			SelfSamples:   md.selfTime,
			TotalPct:      totalPct,
			SelfPct:       selfPct,
			TopFunctions:  topFuncs,
		})
	}

	// Sort by total samples descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalSamples > result[j].TotalSamples
	})

	return result
}

// buildThreadGroupAnalysis builds thread group analysis.
func (g *Generator) buildThreadGroupAnalysis(threads map[int]*threadData, totalSamples int64) []*ThreadGroupAnalysis {
	groups := make(map[string]*ThreadGroupAnalysis)

	for _, td := range threads {
		if td.isSwapper && !g.opts.IncludeSwapper {
			continue
		}

		group, ok := groups[td.group]
		if !ok {
			group = &ThreadGroupAnalysis{
				GroupName: td.group,
			}
			groups[td.group] = group
		}

		group.ThreadCount++
		group.TotalSamples += td.samples
	}

	// Calculate percentages and collect top functions per group
	result := make([]*ThreadGroupAnalysis, 0, len(groups))
	for _, group := range groups {
		if totalSamples > 0 {
			group.Percentage = float64(group.TotalSamples) / float64(totalSamples) * 100
		}
		result = append(result, group)
	}

	// Sort by total samples descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalSamples > result[j].TotalSamples
	})

	return result
}

// buildThreadCallGraphs builds per-thread call graphs.
func (g *Generator) buildThreadCallGraphs(threads map[int]*threadData, totalSamples int64) []*ThreadCallGraph {
	// Collect non-swapper threads
	threadList := make([]*threadData, 0, len(threads))
	for _, td := range threads {
		if td.isSwapper && !g.opts.IncludeSwapper {
			continue
		}
		threadList = append(threadList, td)
	}

	// Sort by samples descending
	sort.Slice(threadList, func(i, j int) bool {
		return threadList[i].samples > threadList[j].samples
	})

	// Limit number of thread call graphs
	n := g.opts.MaxThreadCallGraphs
	if n > len(threadList) {
		n = len(threadList)
	}

	result := make([]*ThreadCallGraph, n)
	for i := 0; i < n; i++ {
		td := threadList[i]
		tcg := td.callGraph

		// Update self times
		for nodeID, time := range td.selfTime {
			if node := tcg.nodeMap[nodeID]; node != nil {
				node.SelfTime = time
			}
		}

		// Calculate percentages
		tcg.CalculatePercentages()

		// Set global percentage
		if totalSamples > 0 {
			tcg.Percentage = float64(tcg.TotalSamples) / float64(totalSamples) * 100
		}

		// Cleanup
		tcg.Cleanup()

		result[i] = tcg
	}

	return result
}

// splitFuncAndModule splits a frame into function name and module.
func (g *Generator) splitFuncAndModule(frame string) (function, module string) {
	function, module = profiling.SplitFuncAndModule(frame)
	if !g.opts.IncludeModule {
		module = ""
	}
	return function, module
}

// GenerateFromParseResult generates a call graph from a parse result.
func (g *Generator) GenerateFromParseResult(ctx context.Context, result *model.ParseResult) (*CallGraph, error) {
	return g.Generate(ctx, result.Samples)
}
