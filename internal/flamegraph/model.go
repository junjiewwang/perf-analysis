// Package flamegraph provides unified flame graph data structures and utilities.
// This package delegates to perflib/flamegraph and provides type aliases for backward compatibility.
package flamegraph

import (
	"context"
	"io"

	libfg "github.com/perf-analysis/perflib/flamegraph"
	"github.com/perf-analysis/perflib/model"
)

// ---- Type aliases delegating to perflib/flamegraph ----

// Node represents a node in the flame graph tree.
type Node = libfg.Node

// FlameGraph represents the complete flame graph structure with optional analysis data.
type FlameGraph = libfg.FlameGraph

// ThreadAnalysisData holds thread-level CPU analysis data.
type ThreadAnalysisData = libfg.ThreadAnalysisData

// ThreadInfo represents detailed CPU analysis for a single thread.
type ThreadInfo = libfg.ThreadInfo

// ThreadTopFunction represents a hot function within a thread.
type ThreadTopFunction = libfg.ThreadTopFunction

// CallStackEntry represents a unique call stack.
type CallStackEntry = libfg.CallStackEntry

// TopFunction represents a globally hot function across all threads.
type TopFunction = libfg.TopFunction

// ThreadFunctionInfo shows function statistics per thread.
type ThreadFunctionInfo = libfg.ThreadFunctionInfo

// ThreadGroupInfo provides aggregated statistics for a thread group.
type ThreadGroupInfo = libfg.ThreadGroupInfo

// NodeBuilder helps construct flame graph trees efficiently.
type NodeBuilder = libfg.NodeBuilder

// CPUAnalysisResult holds the complete CPU profiling analysis result.
type CPUAnalysisResult = libfg.CPUAnalysisResult

// CallStackInfo holds call stack information for a top function.
type CallStackInfo = libfg.CallStackInfo

// ThreadFilter defines criteria for filtering threads.
type ThreadFilter = libfg.ThreadFilter

// ThreadGroupSummary provides aggregated statistics for a thread group.
type ThreadGroupSummary = libfg.ThreadGroupSummary

// SearchResult represents a search result for functions or threads.
type SearchResult = libfg.SearchResult

// GeneratorOptions holds configuration options for the flame graph generator.
type GeneratorOptions = libfg.GeneratorOptions

// Generator generates flame graph data from parsed samples.
type Generator = libfg.Generator

// Writer defines the interface for writing flame graph output.
type Writer interface {
	Write(fg *FlameGraph, w io.Writer) error
}

// ---- Constructor functions delegating to perflib ----

// NewNode creates a new flame graph node.
func NewNode(name string, value int64) *Node {
	return libfg.NewNode(name, value)
}

// NewNodeWithMetadata creates a node with full metadata.
func NewNodeWithMetadata(name, module, process string, tid int, value int64) *Node {
	return libfg.NewNodeWithMetadata(name, module, process, tid, value)
}

// NewFlameGraph creates a new flame graph with a root node.
func NewFlameGraph() *FlameGraph {
	return libfg.NewFlameGraph()
}

// NewFlameGraphWithAnalysis creates a new flame graph with thread analysis support.
func NewFlameGraphWithAnalysis() *FlameGraph {
	return libfg.NewFlameGraphWithAnalysis()
}

// NewNodeBuilder creates a new NodeBuilder.
func NewNodeBuilder(rootName string) *NodeBuilder {
	return libfg.NewNodeBuilder(rootName)
}

// MergeNodes merges multiple flame nodes into one.
func MergeNodes(nodes []*Node) *Node {
	return libfg.MergeNodes(nodes)
}

// NewCPUAnalysisResult creates a new CPUAnalysisResult with initialized maps.
func NewCPUAnalysisResult() *CPUAnalysisResult {
	return libfg.NewCPUAnalysisResult()
}

// DefaultGeneratorOptions returns default generator options.
func DefaultGeneratorOptions() *GeneratorOptions {
	return libfg.DefaultGeneratorOptions()
}

// NewGenerator creates a new flame graph generator.
func NewGenerator(opts *GeneratorOptions) *Generator {
	return libfg.NewGenerator(opts)
}

// GenerateFromParseResult generates a flame graph from a parse result.
// This is a convenience function that creates a default generator and generates.
func GenerateFromParseResult(ctx context.Context, result *model.ParseResult) (*FlameGraph, error) {
	g := NewGenerator(nil)
	return g.GenerateFromParseResult(ctx, result)
}

// ---- Helper function aliases ----

// StackToString converts a call stack to a semicolon-separated string.
func StackToString(stack []string) string {
	return libfg.StackToString(stack)
}

// StringToStack converts a semicolon-separated string back to a call stack.
func StringToStack(s string) []string {
	return libfg.StringToStack(s)
}

// ExtractThreadGroup extracts the thread group name by removing trailing numbers and separators.
func ExtractThreadGroup(threadName string) string {
	return libfg.ExtractThreadGroup(threadName)
}

// IsSwapperThread checks if the thread name indicates a swapper (idle) thread.
func IsSwapperThread(name string) bool {
	return libfg.IsSwapperThread(name)
}

// BuildThreadTopFunctions builds the top functions list for a thread.
func BuildThreadTopFunctions(funcCounts map[string]int64, totalSamples int64, topN int) []*ThreadTopFunction {
	return libfg.BuildThreadTopFunctions(funcCounts, totalSamples, topN)
}

// BuildThreadCallStacks builds the call stacks list for a thread.
func BuildThreadCallStacks(callStacks map[string]int64, totalSamples int64, maxStacks int) []*CallStackEntry {
	return libfg.BuildThreadCallStacks(callStacks, totalSamples, maxStacks)
}
