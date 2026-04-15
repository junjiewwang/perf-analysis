// Package callgraph provides unified call graph data structures and utilities.
// This package delegates to perflib/callgraph and provides type aliases for backward compatibility.
package callgraph

import (
	libcg "github.com/junjiewwang/perf-analysis/perflib/callgraph"
)

// ---- Type aliases delegating to perflib/callgraph ----

// Node represents a node in the call graph.
type Node = libcg.Node

// Edge represents an edge (call relationship) in the call graph.
type Edge = libcg.Edge

// CallerInfo represents information about a caller of a function.
type CallerInfo = libcg.CallerInfo

// CalleeInfo represents information about a callee of a function.
type CalleeInfo = libcg.CalleeInfo

// FunctionAnalysis provides detailed analysis for a single function.
type FunctionAnalysis = libcg.FunctionAnalysis

// HotPath represents a critical/hot execution path.
type HotPath = libcg.HotPath

// ThreadCallGraph represents a call graph for a single thread.
type ThreadCallGraph = libcg.ThreadCallGraph

// ModuleAnalysis provides aggregated analysis by module/package.
type ModuleAnalysis = libcg.ModuleAnalysis

// ThreadGroupAnalysis provides aggregated analysis by thread group.
type ThreadGroupAnalysis = libcg.ThreadGroupAnalysis

// CallGraphAnalysis holds the complete enhanced analysis data.
type CallGraphAnalysis = libcg.CallGraphAnalysis

// CallGraph represents the complete call graph structure.
type CallGraph = libcg.CallGraph

// Stats returns statistics about the call graph.
type Stats = libcg.Stats

// GeneratorOptions holds configuration options for the call graph generator.
type GeneratorOptions = libcg.GeneratorOptions

// Generator generates call graph data from parsed samples.
type Generator = libcg.Generator

// ---- Constructor functions delegating to perflib ----

// NewCallGraph creates a new call graph.
func NewCallGraph() *CallGraph {
	return libcg.NewCallGraph()
}

// NewThreadCallGraph creates a new thread-specific call graph.
func NewThreadCallGraph(tid int, threadName string) *ThreadCallGraph {
	return libcg.NewThreadCallGraph(tid, threadName)
}

// DefaultGeneratorOptions returns default generator options.
func DefaultGeneratorOptions() *GeneratorOptions {
	return libcg.DefaultGeneratorOptions()
}

// NewGenerator creates a new call graph generator.
func NewGenerator(opts *GeneratorOptions) *Generator {
	return libcg.NewGenerator(opts)
}
