package flamegraph

import (
	"context"
	"io"
	"sort"
	"time"

	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/profiling"
)

// GeneratorOptions holds configuration options for the flame graph generator.
type GeneratorOptions struct {
	// MinPercent is the minimum percentage for a node to be included.
	MinPercent float64

	// IncludeModule includes module information in the output.
	IncludeModule bool

	// EnableThreadAnalysis enables detailed thread-level analysis.
	EnableThreadAnalysis bool

	// TopNPerThread specifies how many top functions to track per thread.
	TopNPerThread int

	// TopNGlobal specifies how many top functions to track globally.
	TopNGlobal int

	// MaxCallStacksPerThread limits call stacks stored per thread.
	MaxCallStacksPerThread int

	// MaxCallStacksPerFunc limits call stacks stored per function.
	MaxCallStacksPerFunc int

	// IncludeSwapper includes swapper (idle) threads in analysis.
	IncludeSwapper bool

	// BuildPerThreadFlameGraphs builds individual flame graphs for each thread.
	BuildPerThreadFlameGraphs bool

	// IncludeThreadInStack prepends thread name as the first frame in call stacks.
	// This enables searching for threads in the flame graph visualization.
	IncludeThreadInStack bool
}

// DefaultGeneratorOptions returns default generator options.
// These defaults are optimized for typical Java/Go applications.
func DefaultGeneratorOptions() *GeneratorOptions {
	return &GeneratorOptions{
		MinPercent:                0.1,   // Filter noise below 0.1%
		IncludeModule:             true,  // Include module info for better analysis
		EnableThreadAnalysis:      true,  // Enable thread-level insights
		TopNPerThread:             15,    // Top 15 functions per thread
		TopNGlobal:                50,    // Top 50 global hotspots
		MaxCallStacksPerThread:    200,   // Sufficient for most thread patterns
		MaxCallStacksPerFunc:      10,    // Preserve call path diversity
		IncludeSwapper:            false, // Exclude idle threads
		BuildPerThreadFlameGraphs: true,  // Generate per-thread flame graphs
		IncludeThreadInStack:      true,  // Include thread name as first frame for searchability
	}
}

// Generator generates flame graph data from parsed samples.
type Generator struct {
	opts *GeneratorOptions
}

// NewGenerator creates a new flame graph generator.
func NewGenerator(opts *GeneratorOptions) *Generator {
	if opts == nil {
		opts = DefaultGeneratorOptions()
	}
	return &Generator{opts: opts}
}

// threadData holds intermediate data for a single thread during analysis (internal use).
type threadData struct {
	tid          int
	name         string
	group        string
	samples      int64
	isSwapper    bool
	funcCounts   map[string]int64 // function -> sample count
	callStacks   map[string]int64 // stack string -> count
	flameBuilder *NodeBuilder
}

// Generate generates a flame graph from the given samples.
// If EnableThreadAnalysis is true, it also generates thread-level analysis data.
func (g *Generator) Generate(ctx context.Context, samples []*model.Sample) (*FlameGraph, error) {
	startTime := time.Now()

	var fg *FlameGraph
	if g.opts.EnableThreadAnalysis {
		fg = NewFlameGraphWithAnalysis()
	} else {
		fg = NewFlameGraph()
	}

	// If thread analysis is disabled, use simple generation
	if !g.opts.EnableThreadAnalysis {
		return g.generateSimple(ctx, fg, samples)
	}

	// Full generation with thread analysis
	return g.generateWithAnalysis(ctx, fg, samples, startTime)
}

// generateSimple generates a flame graph without thread analysis.
func (g *Generator) generateSimple(ctx context.Context, fg *FlameGraph, samples []*model.Sample) (*FlameGraph, error) {
	for _, sample := range samples {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		g.appendStack(fg, sample)
	}

	fg.TotalSamples = fg.Root.Value
	fg.Cleanup(g.opts.MinPercent)
	fg.CalculateMaxDepth()

	return fg, nil
}

// generateWithAnalysis generates a flame graph with full thread analysis.
func (g *Generator) generateWithAnalysis(ctx context.Context, fg *FlameGraph, samples []*model.Sample, startTime time.Time) (*FlameGraph, error) {
	// Intermediate storage
	threads := make(map[int]*threadData)
	globalFuncCounts := make(map[string]int64)
	globalFuncThreads := make(map[string]map[int]int64) // func -> tid -> samples
	globalCallStacks := make(map[string]map[string]int64) // func -> stack -> count
	var totalSamples, totalSamplesWithSwapper int64
	var maxDepth int
	uniqueFuncs := make(map[string]struct{})

	// Process samples
	for _, sample := range samples {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		isSwapper := profiling.IsSwapperThread(sample.ThreadName)
		totalSamplesWithSwapper += sample.Value
		if !isSwapper {
			totalSamples += sample.Value
		}

		// Append to global flame graph
		g.appendStack(fg, sample)

		// Get or create thread data
		td, ok := threads[sample.TID]
		if !ok {
			td = &threadData{
				tid:        sample.TID,
				name:       sample.ThreadName,
				group:      profiling.ExtractThreadGroup(sample.ThreadName),
				isSwapper:  isSwapper,
				funcCounts: make(map[string]int64),
				callStacks: make(map[string]int64),
			}
			if g.opts.BuildPerThreadFlameGraphs {
				td.flameBuilder = NewNodeBuilder(sample.ThreadName)
			}
			threads[sample.TID] = td
		}

		td.samples += sample.Value

		// Track top function (leaf)
		if len(sample.CallStack) > 0 {
			topFunc := sample.CallStack[len(sample.CallStack)-1]
			td.funcCounts[topFunc] += sample.Value

			// Track call stack
			stackStr := profiling.StackToString(sample.CallStack)
			td.callStacks[stackStr] += sample.Value

			// Build per-thread flame graph
			if td.flameBuilder != nil {
				td.flameBuilder.AddStack(sample.CallStack, sample.Value)
			}

			// Update unique functions
			for _, f := range sample.CallStack {
				uniqueFuncs[f] = struct{}{}
			}

			// Track depth (add 1 if thread name is included as first frame)
			depth := len(sample.CallStack)
			if g.opts.IncludeThreadInStack {
				depth++
			}
			if depth > maxDepth {
				maxDepth = depth
			}

			// Update global stats (only for non-swapper)
			if !isSwapper {
				globalFuncCounts[topFunc] += sample.Value
				if globalFuncThreads[topFunc] == nil {
					globalFuncThreads[topFunc] = make(map[int]int64)
				}
				globalFuncThreads[topFunc][sample.TID] += sample.Value
				if globalCallStacks[topFunc] == nil {
					globalCallStacks[topFunc] = make(map[string]int64)
				}
				globalCallStacks[topFunc][stackStr] += sample.Value
			}
		}
	}

	// Set flame graph totals
	fg.TotalSamples = totalSamples
	fg.MaxDepth = maxDepth

	// Build thread analysis
	fg.ThreadAnalysis.TotalThreads = len(threads)
	fg.ThreadAnalysis.UniqueFunctions = len(uniqueFuncs)
	fg.ThreadAnalysis.AnalysisDurationMs = time.Since(startTime).Milliseconds()

	// Build thread info list
	threadGroups := make(map[string]*ThreadGroupInfo)
	activeThreads := 0

	for _, td := range threads {
		if td.isSwapper && !g.opts.IncludeSwapper {
			continue
		}

		if td.samples > 0 {
			activeThreads++
		}

		threadInfo := &ThreadInfo{
			TID:       td.tid,
			Name:      td.name,
			Group:     td.group,
			Samples:   td.samples,
			IsSwapper: td.isSwapper,
		}

		// Calculate percentage
		if totalSamplesWithSwapper > 0 {
			threadInfo.Percentage = float64(td.samples) / float64(totalSamplesWithSwapper) * 100
		}

		// Build top functions for this thread
		threadInfo.TopFunctions = buildThreadTopFunctions(td.funcCounts, td.samples, g.opts.TopNPerThread)

		// Build top call stacks for this thread
		threadInfo.TopCallStacks = buildThreadCallStacks(td.callStacks, td.samples, g.opts.MaxCallStacksPerThread)

		// Attach per-thread flame graph
		if td.flameBuilder != nil {
			threadInfo.FlameRoot = td.flameBuilder.Build()
		}

		fg.ThreadAnalysis.Threads = append(fg.ThreadAnalysis.Threads, threadInfo)

		// Aggregate thread groups
		group, ok := threadGroups[td.group]
		if !ok {
			group = &ThreadGroupInfo{Name: td.group}
			threadGroups[td.group] = group
		}
		group.ThreadCount++
		group.TotalSamples += td.samples
		if group.TopThread == "" || td.samples > 0 {
			// Track top thread in group
			for _, t := range fg.ThreadAnalysis.Threads {
				if t.Group == td.group && (group.TopThread == "" || t.Samples > 0) {
					group.TopThread = t.Name
				}
			}
		}
	}

	fg.ThreadAnalysis.ActiveThreads = activeThreads

	// Sort threads by samples descending
	sort.Slice(fg.ThreadAnalysis.Threads, func(i, j int) bool {
		return fg.ThreadAnalysis.Threads[i].Samples > fg.ThreadAnalysis.Threads[j].Samples
	})

	// Build thread groups list
	for _, group := range threadGroups {
		if totalSamples > 0 {
			group.Percentage = float64(group.TotalSamples) / float64(totalSamples) * 100
		}
		fg.ThreadAnalysis.ThreadGroups = append(fg.ThreadAnalysis.ThreadGroups, group)
	}

	// Sort thread groups by samples descending
	sort.Slice(fg.ThreadAnalysis.ThreadGroups, func(i, j int) bool {
		return fg.ThreadAnalysis.ThreadGroups[i].TotalSamples > fg.ThreadAnalysis.ThreadGroups[j].TotalSamples
	})

	// Build global top functions
	fg.ThreadAnalysis.TopFunctions = buildGlobalTopFunctions(
		globalFuncCounts,
		globalFuncThreads,
		globalCallStacks,
		threads,
		totalSamples,
		g.opts.TopNGlobal,
		g.opts.MaxCallStacksPerFunc,
	)

	// Cleanup flame graph
	fg.Cleanup(g.opts.MinPercent)

	return fg, nil
}

// appendStack appends a sample's call stack to the flame graph.
func (g *Generator) appendStack(fg *FlameGraph, sample *model.Sample) {
	if len(sample.CallStack) == 0 {
		return
	}

	node := fg.Root
	node.Value += sample.Value

	// If IncludeThreadInStack is enabled, add thread name as the first frame
	// This allows searching for threads in the flame graph visualization
	if g.opts.IncludeThreadInStack && sample.ThreadName != "" {
		threadNode := node.GetChild(sample.ThreadName)
		if threadNode == nil {
			threadNode = NewNode(sample.ThreadName, 0)
			node.AddChild(threadNode)
		}
		threadNode.Value += sample.Value
		node = threadNode
	}

	for _, frame := range sample.CallStack {
		function, module := profiling.SplitFuncAndModule(frame)

		var child *Node
		if g.opts.IncludeModule && module != "" {
			child = node.GetChildWithMetadata(function, module, sample.ThreadName, sample.TID)
			if child == nil {
				child = NewNodeWithMetadata(function, module, sample.ThreadName, sample.TID, 0)
				node.AddChild(child)
			}
		} else {
			child = node.GetChild(function)
			if child == nil {
				child = NewNode(function, 0)
				node.AddChild(child)
			}
		}

		child.Value += sample.Value
		node = child
	}

	// Mark leaf node's self value
	node.Self += sample.Value
}

// GenerateFromParseResult generates a flame graph from a parse result.
func (g *Generator) GenerateFromParseResult(ctx context.Context, result *model.ParseResult) (*FlameGraph, error) {
	return g.Generate(ctx, result.Samples)
}

// Writer defines the interface for writing flame graph output.
type Writer interface {
	Write(fg *FlameGraph, w io.Writer) error
}

// Helper functions - exported for use by other packages
// These are aliases to the profiling package functions for backward compatibility.

// StackToString converts a call stack to a semicolon-separated string.
func StackToString(stack []string) string {
	return profiling.StackToString(stack)
}

// StringToStack converts a semicolon-separated string back to a call stack.
func StringToStack(s string) []string {
	return profiling.StringToStack(s)
}

// ExtractThreadGroup extracts the thread group name by removing trailing numbers and separators.
func ExtractThreadGroup(threadName string) string {
	return profiling.ExtractThreadGroup(threadName)
}

// IsSwapperThread checks if the thread name indicates a swapper (idle) thread.
func IsSwapperThread(name string) bool {
	return profiling.IsSwapperThread(name)
}

// BuildThreadTopFunctions builds the top functions list for a thread.
// Exported for use by other packages.
func BuildThreadTopFunctions(funcCounts map[string]int64, totalSamples int64, topN int) []*ThreadTopFunction {
	type entry struct {
		name    string
		samples int64
	}

	entries := make([]entry, 0, len(funcCounts))
	for name, samples := range funcCounts {
		entries = append(entries, entry{name: name, samples: samples})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].samples > entries[j].samples
	})

	if topN > len(entries) {
		topN = len(entries)
	}

	result := make([]*ThreadTopFunction, topN)
	for i := 0; i < topN; i++ {
		pct := float64(0)
		if totalSamples > 0 {
			pct = float64(entries[i].samples) / float64(totalSamples) * 100
		}
		name, module := profiling.SplitFuncAndModule(entries[i].name)
		result[i] = &ThreadTopFunction{
			Name:       name,
			Module:     module,
			Samples:    entries[i].samples,
			Percentage: pct,
		}
	}

	return result
}

// BuildThreadCallStacks builds the call stacks list for a thread.
// Exported for use by other packages.
func BuildThreadCallStacks(callStacks map[string]int64, totalSamples int64, maxStacks int) []*CallStackEntry {
	type entry struct {
		stack   string
		samples int64
	}

	entries := make([]entry, 0, len(callStacks))
	for stack, samples := range callStacks {
		entries = append(entries, entry{stack: stack, samples: samples})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].samples > entries[j].samples
	})

	if maxStacks > len(entries) {
		maxStacks = len(entries)
	}

	result := make([]*CallStackEntry, maxStacks)
	for i := 0; i < maxStacks; i++ {
		pct := float64(0)
		if totalSamples > 0 {
			pct = float64(entries[i].samples) / float64(totalSamples) * 100
		}
		result[i] = &CallStackEntry{
			Stack:      profiling.StringToStack(entries[i].stack),
			Samples:    entries[i].samples,
			Percentage: pct,
		}
	}

	return result
}

// internal aliases for backward compatibility
func buildThreadTopFunctions(funcCounts map[string]int64, totalSamples int64, topN int) []*ThreadTopFunction {
	return BuildThreadTopFunctions(funcCounts, totalSamples, topN)
}

func buildThreadCallStacks(callStacks map[string]int64, totalSamples int64, maxStacks int) []*CallStackEntry {
	return BuildThreadCallStacks(callStacks, totalSamples, maxStacks)
}

func buildGlobalTopFunctions(
	funcCounts map[string]int64,
	funcThreads map[string]map[int]int64,
	funcCallStacks map[string]map[string]int64,
	threads map[int]*threadData,
	totalSamples int64,
	topN int,
	maxCallStacks int,
) []*TopFunction {
	type entry struct {
		name    string
		samples int64
	}

	entries := make([]entry, 0, len(funcCounts))
	for name, samples := range funcCounts {
		entries = append(entries, entry{name: name, samples: samples})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].samples > entries[j].samples
	})

	if topN > len(entries) {
		topN = len(entries)
	}

	result := make([]*TopFunction, topN)
	for i := 0; i < topN; i++ {
		e := entries[i]
		pct := float64(0)
		if totalSamples > 0 {
			pct = float64(e.samples) / float64(totalSamples) * 100
		}

		name, module := profiling.SplitFuncAndModule(e.name)

		// Build thread info for this function
		var threadInfos []*ThreadFunctionInfo
		if threadMap, ok := funcThreads[e.name]; ok {
			threadInfos = make([]*ThreadFunctionInfo, 0, len(threadMap))
			for tid, samples := range threadMap {
				td := threads[tid]
				threadPct := float64(0)
				threadName := ""
				if td != nil {
					threadName = td.name
					if td.samples > 0 {
						threadPct = float64(samples) / float64(td.samples) * 100
					}
				}
				threadInfos = append(threadInfos, &ThreadFunctionInfo{
					TID:        tid,
					ThreadName: threadName,
					Samples:    samples,
					Percentage: threadPct,
				})
			}
			sort.Slice(threadInfos, func(i, j int) bool {
				return threadInfos[i].Samples > threadInfos[j].Samples
			})
		}

		// Build top call stacks for this function
		var topStacks []string
		if stacks, ok := funcCallStacks[e.name]; ok {
			type stackEntry struct {
				stack string
				count int64
			}
			stackEntries := make([]stackEntry, 0, len(stacks))
			for stack, count := range stacks {
				stackEntries = append(stackEntries, stackEntry{stack: stack, count: count})
			}
			sort.Slice(stackEntries, func(i, j int) bool {
				return stackEntries[i].count > stackEntries[j].count
			})
			limit := maxCallStacks
			if limit > len(stackEntries) {
				limit = len(stackEntries)
			}
			topStacks = make([]string, limit)
			for j := 0; j < limit; j++ {
				topStacks[j] = stackEntries[j].stack
			}
		}

		result[i] = &TopFunction{
			Name:          name,
			Module:        module,
			Samples:       e.samples,
			Percentage:    pct,
			ThreadCount:   len(threadInfos),
			Threads:       threadInfos,
			TopCallStacks: topStacks,
		}
	}

	return result
}
