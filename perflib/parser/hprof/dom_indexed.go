// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"context"
)

// ComputeDominatorForIndexedGraph computes the dominator tree and retained sizes
// for an IndexedReferenceGraph. This reuses the core Lengauer-Tarjan algorithm
// from LevelDominatorState but builds directly from CSR data without intermediate
// map-based conversion.
//
// The function fills graph.objects.dominators[] and graph.objects.retainedSizes[].
func ComputeDominatorForIndexedGraph(ctx context.Context, graph *IndexedReferenceGraph) {
	if ctx == nil {
		ctx = context.Background()
	}

	objectCount := graph.ObjectCount()
	if objectCount == 0 {
		return
	}

	// nodeCount = objectCount + 1 (super root at index 0)
	nodeCount := int(objectCount) + 1
	config := DefaultHierarchicalDominatorConfig()

	// Create state
	state := NewLevelDominatorState(nodeCount, config)

	// Build from indexed graph (much faster than BuildFromReferenceGraph)
	buildFromIndexedGraph(state, graph)

	// Compute dominators (core Lengauer-Tarjan algorithm)
	state.ComputeDominators(ctx)

	// Export results and compute retained sizes
	exportToIndexedGraph(state, graph)
}

// buildFromIndexedGraph populates LevelDominatorState CSR from IndexedReferenceGraph CSR.
// This is O(E) where E = total edges, with no map lookups.
func buildFromIndexedGraph(state *LevelDominatorState, graph *IndexedReferenceGraph) {
	objectCount := graph.ObjectCount()
	nodeCount := state.nodeCount

	// State index mapping: stateIdx = graphIdx + 1 (index 0 = super root)
	// No objToIdx/idxToObj map needed for computation — only for export

	// Count edges for CSR pre-allocation
	edgeCounts := make([]int32, nodeCount) // successor counts per node
	predCounts := make([]int32, nodeCount) // predecessor counts per node

	// Count super root edges: one edge to each GC root + class object
	gcRootCount := int32(0)
	for i := int32(0); i < objectCount; i++ {
		if graph.IsGCRoot(i) || graph.IsClassObject(i) {
			gcRootCount++
		}
	}
	edgeCounts[0] = gcRootCount

	// Count predecessor edges from super root
	for i := int32(0); i < objectCount; i++ {
		if graph.IsGCRoot(i) || graph.IsClassObject(i) {
			stateIdx := i + 1
			predCounts[stateIdx]++
		}
	}

	// Count outgoing edges for each object (only to reachable targets)
	outgoing := graph.GetOutgoing()
	if outgoing != nil {
		for i := int32(0); i < objectCount; i++ {
			stateIdx := i + 1
			start := outgoing.offsets[i]
			end := outgoing.offsets[i+1]
			for j := start; j < end; j++ {
				targetGraphIdx := outgoing.targets[j]
				if targetGraphIdx >= 0 && targetGraphIdx < objectCount {
					edgeCounts[stateIdx]++
					targetStateIdx := targetGraphIdx + 1
					predCounts[targetStateIdx]++
				}
			}
		}
	}

	// Build CSR offsets
	state.buildCSROffsets(edgeCounts, predCounts)

	// Fill edges - use write positions
	succWritePos := make([]int32, nodeCount)
	predWritePos := make([]int32, nodeCount)
	copy(succWritePos, state.successorOffsets[:nodeCount])
	copy(predWritePos, state.predecessorOffsets[:nodeCount])

	// Fill super root → GC root/class object edges
	for i := int32(0); i < objectCount; i++ {
		if graph.IsGCRoot(i) || graph.IsClassObject(i) {
			stateIdx := i + 1
			state.successorTargets[succWritePos[0]] = stateIdx
			succWritePos[0]++
			state.predecessorTargets[predWritePos[stateIdx]] = 0
			predWritePos[stateIdx]++
		}
	}

	// Fill object → object edges
	if outgoing != nil {
		for i := int32(0); i < objectCount; i++ {
			fromStateIdx := i + 1
			start := outgoing.offsets[i]
			end := outgoing.offsets[i+1]
			for j := start; j < end; j++ {
				targetGraphIdx := outgoing.targets[j]
				if targetGraphIdx >= 0 && targetGraphIdx < objectCount {
					targetStateIdx := targetGraphIdx + 1
					state.successorTargets[succWritePos[fromStateIdx]] = targetStateIdx
					succWritePos[fromStateIdx]++
					state.predecessorTargets[predWritePos[targetStateIdx]] = fromStateIdx
					predWritePos[targetStateIdx]++
				}
			}
		}
	}

	// Set metrics
	state.metrics.TotalNodes = int64(nodeCount)
}

// exportToIndexedGraph writes dominator results and computes retained sizes
// directly into IndexedReferenceGraph.
func exportToIndexedGraph(state *LevelDominatorState, graph *IndexedReferenceGraph) {
	objectCount := graph.ObjectCount()

	// Phase 1: Export idom to graph.objects.dominators
	for i := int32(0); i < objectCount; i++ {
		stateIdx := i + 1
		if state.dfn[stateIdx] == 0 {
			// Not reachable from super root
			graph.SetDominator(i, -1)
			continue
		}
		domStateIdx := state.idom[stateIdx]
		if domStateIdx <= 0 {
			// Dominated by super root (i.e., it is a root itself)
			graph.SetDominator(i, -1)
		} else {
			domGraphIdx := domStateIdx - 1
			graph.SetDominator(i, domGraphIdx)
		}
	}

	// Phase 2: Compute retained sizes (bottom-up in dominator tree)
	computeRetainedSizesForIndexedGraph(state, graph)

	// Mark dominator as computed
	graph.dominatorComputed = true
}

// computeRetainedSizesForIndexedGraph computes retained sizes by bottom-up
// accumulation in the dominator tree. Uses topological sort for correctness.
func computeRetainedSizesForIndexedGraph(state *LevelDominatorState, graph *IndexedReferenceGraph) {
	objectCount := graph.ObjectCount()
	nodeCount := state.nodeCount

	// Phase 1: Initialize retained sizes to shallow sizes
	retainedSizes := make([]int64, nodeCount)
	for i := int32(0); i < objectCount; i++ {
		stateIdx := i + 1
		if state.dfn[stateIdx] > 0 {
			retainedSizes[stateIdx] = graph.GetShallowSize(i)
		}
	}

	// Phase 2: Collect reachable nodes ordered by DFS number (reverse DFS = bottom-up)
	// Nodes with higher DFS numbers are deeper in the tree, process them first
	maxDFN := state.dfnNum
	orderedNodes := make([]int32, 0, maxDFN)
	for i := int32(maxDFN); i >= 1; i-- {
		node := state.vertex[i]
		if node > 0 && node < nodeCount {
			orderedNodes = append(orderedNodes, node)
		}
	}

	// Phase 3: Bottom-up propagation in reverse DFS order
	// This guarantees all children are processed before their parent
	for _, node := range orderedNodes {
		parentIdx := state.idom[node]
		if parentIdx >= 0 && parentIdx != node {
			retainedSizes[parentIdx] += retainedSizes[node]
		}
	}

	// Phase 4: Export retained sizes to graph
	for i := int32(0); i < objectCount; i++ {
		stateIdx := i + 1
		if state.dfn[stateIdx] > 0 {
			graph.SetRetainedSize(i, retainedSizes[stateIdx])
		}
	}
}
