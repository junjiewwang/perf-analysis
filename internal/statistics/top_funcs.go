// Package statistics provides utilities for calculating profiling statistics.
package statistics

import (
	"sort"

	"github.com/perf-analysis/pkg/model"
)

// TopFuncsCalculator calculates top function statistics from samples.
type TopFuncsCalculator struct {
	topN           int
	includeSwapper bool
}

// TopFuncsOption configures the TopFuncsCalculator.
type TopFuncsOption func(*TopFuncsCalculator)

// WithTopN sets the number of top functions to return.
func WithTopN(n int) TopFuncsOption {
	return func(c *TopFuncsCalculator) {
		c.topN = n
	}
}

// WithSwapper includes swapper threads in calculations.
func WithSwapper(include bool) TopFuncsOption {
	return func(c *TopFuncsCalculator) {
		c.includeSwapper = include
	}
}

// NewTopFuncsCalculator creates a new TopFuncsCalculator.
func NewTopFuncsCalculator(opts ...TopFuncsOption) *TopFuncsCalculator {
	c := &TopFuncsCalculator{
		topN:           15,
		includeSwapper: false,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// TopFuncEntry represents a function with its statistics.
type TopFuncEntry struct {
	Name        string
	SelfSamples int64
	SelfPercent float64
}

// Calculate calculates top functions from the given samples.
func (c *TopFuncsCalculator) Calculate(samples []*model.Sample) *TopFuncsResult {
	result := &TopFuncsResult{
		TopFuncs:       make([]TopFuncEntry, 0),
		TopFuncsMap:    make(model.TopFuncsMap),
		TotalSamples:   0,
		SwapperSamples: 0,
		FuncCallstacks: make(map[string]map[string]int64),
	}

	if len(samples) == 0 {
		return result
	}

	// Collect function samples
	funcSamples := make(map[string]int64)
	funcSamplesWithSwapper := make(map[string]int64)

	for _, sample := range samples {
		isSwapper := isSwapperThread(sample.ThreadName)

		// Always count samples towards totals
		result.TotalSamples += sample.Value
		if isSwapper {
			result.SwapperSamples += sample.Value
		}

		// Skip samples with empty call stack for function statistics
		if len(sample.CallStack) == 0 {
			continue
		}

		topFunc := sample.CallStack[len(sample.CallStack)-1]
		funcSamplesWithSwapper[topFunc] += sample.Value

		if !isSwapper {
			funcSamples[topFunc] += sample.Value
		}

		// Track call stacks
		callStack := joinCallStack(sample.CallStack)
		if _, ok := result.FuncCallstacks[topFunc]; !ok {
			result.FuncCallstacks[topFunc] = make(map[string]int64)
		}
		result.FuncCallstacks[topFunc][callStack] += sample.Value
	}

	// Calculate effective total (exclude swapper if not included)
	effectiveTotal := result.TotalSamples
	if !c.includeSwapper {
		effectiveTotal = result.TotalSamples - result.SwapperSamples
	}

	// Use appropriate function samples map
	targetSamples := funcSamples
	if c.includeSwapper {
		targetSamples = funcSamplesWithSwapper
	}

	// Sort functions by sample count
	entries := make([]TopFuncEntry, 0, len(targetSamples))
	for name, samples := range targetSamples {
		pct := 0.0
		if effectiveTotal > 0 {
			pct = float64(samples) / float64(effectiveTotal) * 100
		}
		entries = append(entries, TopFuncEntry{
			Name:        name,
			SelfSamples: samples,
			SelfPercent: pct,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].SelfSamples > entries[j].SelfSamples
	})

	// Take top N
	topN := c.topN
	if topN > len(entries) {
		topN = len(entries)
	}

	result.TopFuncs = entries[:topN]

	// Build map
	for _, entry := range result.TopFuncs {
		result.TopFuncsMap[entry.Name] = model.TopFuncValue{
			Self: entry.SelfPercent,
		}
	}

	return result
}

// TopFuncsResult holds the calculation result.
type TopFuncsResult struct {
	TopFuncs       []TopFuncEntry
	TopFuncsMap    model.TopFuncsMap
	TotalSamples   int64
	SwapperSamples int64
	FuncCallstacks map[string]map[string]int64
}

// GetTopFuncsCallstacks returns call stack information for top functions.
func (r *TopFuncsResult) GetTopFuncsCallstacks(maxCallstacks int) map[string]*model.CallStackInfo {
	result := make(map[string]*model.CallStackInfo)

	for _, entry := range r.TopFuncs {
		callstacks, ok := r.FuncCallstacks[entry.Name]
		if !ok {
			continue
		}

		// Sort and take top N callstacks
		type csEntry struct {
			stack string
			count int64
		}
		csEntries := make([]csEntry, 0, len(callstacks))
		for stack, count := range callstacks {
			csEntries = append(csEntries, csEntry{stack: stack, count: count})
		}

		sort.Slice(csEntries, func(i, j int) bool {
			return csEntries[i].count > csEntries[j].count
		})

		topStacks := make([]string, 0, maxCallstacks)
		for i := 0; i < len(csEntries) && i < maxCallstacks; i++ {
			topStacks = append(topStacks, csEntries[i].stack)
		}

		result[entry.Name] = &model.CallStackInfo{
			FunctionName: entry.Name,
			CallStacks:   topStacks,
			Count:        len(callstacks),
		}
	}

	return result
}

// Helper functions

func isSwapperThread(threadName string) bool {
	return threadName == "swapper" || len(threadName) > 8 && threadName[:8] == "swapper-"
}

func joinCallStack(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	result := stack[0]
	for i := 1; i < len(stack); i++ {
		result += ";" + stack[i]
	}
	return result
}
