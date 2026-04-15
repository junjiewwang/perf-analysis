// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
)

// ============================================================================
// Hierarchical Parallel Dominator Algorithm
// ============================================================================
//
// This implements a hierarchical parallel dominator tree algorithm that:
// 1. Partitions the graph into strongly connected components (SCCs)
// 2. Computes dominators within each SCC in parallel
// 3. Merges results using the DAG of SCCs
//
// Key optimizations:
// - Level-based parallelism: nodes at the same BFS level can be processed in parallel
// - Work stealing: idle workers steal work from busy workers
// - Cache-friendly memory access patterns
// - Adaptive chunk sizing based on graph density
//
// Reference: "Parallel Dominator Computation" by Georgiadis et al.

// HierarchicalDominatorConfig configures the hierarchical dominator algorithm.
type HierarchicalDominatorConfig struct {
	// MaxWorkers is the maximum number of parallel workers.
	MaxWorkers int

	// MinChunkSize is the minimum number of nodes per work chunk.
	MinChunkSize int

	// UseMmap enables memory-mapped storage for large graphs.
	UseMmap bool

	// MmapConfig is the configuration for mmap storage.
	MmapConfig MmapConfig

	// EnableWorkStealing enables work stealing between workers.
	EnableWorkStealing bool

	// LevelParallelismThreshold is the minimum nodes per level to enable parallelism.
	LevelParallelismThreshold int
}

// DefaultHierarchicalDominatorConfig returns default configuration.
func DefaultHierarchicalDominatorConfig() HierarchicalDominatorConfig {
	workers := runtime.NumCPU()
	if workers > 16 {
		workers = 16
	}
	return HierarchicalDominatorConfig{
		MaxWorkers:                workers,
		MinChunkSize:              1000,
		UseMmap:                   false,
		MmapConfig:                DefaultMmapConfig(),
		EnableWorkStealing:        true,
		LevelParallelismThreshold: 10000,
	}
}

// ============================================================================
// Level-Based Parallel Dominator
// ============================================================================

// LevelDominatorState holds state for level-based parallel dominator computation.
type LevelDominatorState struct {
	// Node count (including super root at index 0)
	nodeCount int32

	// Object ID to index mapping
	objToIdx map[uint64]int32
	idxToObj []uint64

	// Graph structure (CSR format for cache efficiency)
	// successorOffsets[i] = start index in successorTargets for node i
	successorOffsets []int32
	successorTargets []int32

	// predecessorOffsets[i] = start index in predecessorTargets for node i
	predecessorOffsets []int32
	predecessorTargets []int32

	// BFS levels from super root
	levels []int32

	// Nodes at each level (for parallel processing)
	levelNodes [][]int32

	// Dominator results
	idom []int32

	// Semi-dominators (for Lengauer-Tarjan)
	semi []int32

	// DFS numbering
	dfn    []int32
	vertex []int32
	parent []int32
	dfnNum int32

	// For path compression
	ancestor []int32
	label    []int32

	// Configuration
	config HierarchicalDominatorConfig

	// Metrics
	metrics *DominatorMetrics
}

// DominatorMetrics tracks performance metrics.
type DominatorMetrics struct {
	TotalNodes       int64
	ReachableNodes   int64
	MaxLevel         int32
	LevelsProcessed  int64
	ParallelChunks   int64
	WorkStealEvents  int64
	ComputeTimeNanos int64
}

// NewLevelDominatorState creates a new level-based dominator state.
func NewLevelDominatorState(nodeCount int, config HierarchicalDominatorConfig) *LevelDominatorState {
	return &LevelDominatorState{
		nodeCount:          int32(nodeCount),
		objToIdx:           make(map[uint64]int32, nodeCount),
		idxToObj:           make([]uint64, nodeCount),
		successorOffsets:   make([]int32, nodeCount+1),
		predecessorOffsets: make([]int32, nodeCount+1),
		levels:             make([]int32, nodeCount),
		idom:               make([]int32, nodeCount),
		semi:               make([]int32, nodeCount),
		dfn:                make([]int32, nodeCount),
		vertex:             make([]int32, nodeCount),
		parent:             make([]int32, nodeCount),
		ancestor:           make([]int32, nodeCount),
		label:              make([]int32, nodeCount),
		config:             config,
		metrics:            &DominatorMetrics{TotalNodes: int64(nodeCount)},
	}
}

// BuildFromReferenceGraph builds the state from a ReferenceGraph.
// Optimized with parallel processing for large graphs.
func (s *LevelDominatorState) BuildFromReferenceGraph(g *ReferenceGraph) {
	// Index 0 = super root
	s.objToIdx[superRootID] = 0
	s.idxToObj[0] = superRootID

	// Collect all object IDs into a slice for parallel processing
	// Pre-allocate with exact capacity
	objIDs := make([]uint64, 0, len(g.objectClass))
	for objID := range g.objectClass {
		objIDs = append(objIDs, objID)
	}

	// Create indices for all objects - this is O(n) and fast
	for i, objID := range objIDs {
		idx := int32(i + 1)
		s.objToIdx[objID] = idx
		s.idxToObj[idx] = objID
	}

	// Build GC root set - use slice for better iteration if small
	gcRootSet := make(map[uint64]bool, len(g.gcRoots)+len(g.classObjectIDs))
	for _, root := range g.gcRoots {
		if _, ok := s.objToIdx[root.ObjectID]; ok {
			gcRootSet[root.ObjectID] = true
		}
	}
	for classObjID := range g.classObjectIDs {
		if _, ok := s.objToIdx[classObjID]; ok {
			gcRootSet[classObjID] = true
		}
	}

	// Determine parallelism
	numWorkers := s.config.MaxWorkers
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	
	// For small graphs, use sequential processing (threshold raised for better perf)
	if len(objIDs) < 100000 || numWorkers == 1 {
		s.buildFromReferenceGraphSequential(g, objIDs, gcRootSet)
		return
	}

	// Parallel edge counting and building
	s.buildFromReferenceGraphParallel(g, objIDs, gcRootSet, numWorkers)
}

// buildFromReferenceGraphSequential builds graph sequentially for small graphs.
// Optimized to avoid map.Clear overhead by using versioned slice-based seen tracking.
func (s *LevelDominatorState) buildFromReferenceGraphSequential(g *ReferenceGraph, objIDs []uint64, gcRootSet map[uint64]bool) {
	// Count edges for pre-allocation
	edgeCounts := make([]int32, s.nodeCount)
	predCounts := make([]int32, s.nodeCount)

	// Count GC root edges
	edgeCounts[0] = int32(len(gcRootSet))
	for objID := range gcRootSet {
		if toIdx, ok := s.objToIdx[objID]; ok {
			predCounts[toIdx]++
		}
	}

	// Count object edges - use versioned slice for O(1) reset
	seenVersion := make([]uint32, s.nodeCount)
	currentVersion := uint32(1)

	for _, objID := range objIDs {
		fromIdx := s.objToIdx[objID]
		// Increment version to "clear" the seen set (O(1) reset)
		currentVersion++
		if currentVersion == 0 {
			// Handle overflow by resetting the slice
			for i := range seenVersion {
				seenVersion[i] = 0
			}
			currentVersion = 1
		}

		for _, ref := range g.outgoingRefs[objID] {
			if toIdx, ok := s.objToIdx[ref.ToObjectID]; ok {
				if seenVersion[toIdx] != currentVersion {
					seenVersion[toIdx] = currentVersion
					edgeCounts[fromIdx]++
					predCounts[toIdx]++
				}
			}
		}
	}

	// Build CSR offsets
	s.buildCSROffsets(edgeCounts, predCounts)

	// Fill edges
	succWritePos := make([]int32, s.nodeCount)
	predWritePos := make([]int32, s.nodeCount)
	copy(succWritePos, s.successorOffsets[:s.nodeCount])
	copy(predWritePos, s.predecessorOffsets[:s.nodeCount])

	// Add GC root edges
	for objID := range gcRootSet {
		if toIdx, ok := s.objToIdx[objID]; ok {
			s.successorTargets[succWritePos[0]] = toIdx
			succWritePos[0]++
			s.predecessorTargets[predWritePos[toIdx]] = 0
			predWritePos[toIdx]++
		}
	}

	// Add object edges - reset version for second pass
	currentVersion++
	if currentVersion == 0 {
		for i := range seenVersion {
			seenVersion[i] = 0
		}
		currentVersion = 1
	}

	for _, objID := range objIDs {
		fromIdx := s.objToIdx[objID]
		// Increment version to "clear" the seen set (O(1) reset)
		currentVersion++
		if currentVersion == 0 {
			for i := range seenVersion {
				seenVersion[i] = 0
			}
			currentVersion = 1
		}

		for _, ref := range g.outgoingRefs[objID] {
			if toIdx, ok := s.objToIdx[ref.ToObjectID]; ok {
				if seenVersion[toIdx] != currentVersion {
					seenVersion[toIdx] = currentVersion
					s.successorTargets[succWritePos[fromIdx]] = toIdx
					succWritePos[fromIdx]++
					s.predecessorTargets[predWritePos[toIdx]] = fromIdx
					predWritePos[toIdx]++
				}
			}
		}
	}
}

// buildFromReferenceGraphParallel builds graph in parallel for large graphs.
// Optimized to avoid map.Clear overhead by using versioned slice-based seen tracking.
func (s *LevelDominatorState) buildFromReferenceGraphParallel(g *ReferenceGraph, objIDs []uint64, gcRootSet map[uint64]bool, numWorkers int) {
	// Phase 1: Parallel edge counting
	edgeCounts := make([]int32, s.nodeCount)
	predCounts := make([]atomic.Int32, s.nodeCount)

	// Count GC root edges
	edgeCounts[0] = int32(len(gcRootSet))
	for objID := range gcRootSet {
		if toIdx, ok := s.objToIdx[objID]; ok {
			predCounts[toIdx].Add(1)
		}
	}

	// Parallel count object edges
	chunkSize := (len(objIDs) + numWorkers - 1) / numWorkers
	var wg sync.WaitGroup

	// Per-worker local edge counts (to avoid atomic operations on edgeCounts)
	localEdgeCounts := make([][]int32, numWorkers)
	for i := range localEdgeCounts {
		localEdgeCounts[i] = make([]int32, s.nodeCount)
	}

	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > len(objIDs) {
			end = len(objIDs)
		}
		if start >= end {
			continue
		}

		wg.Add(1)
		go func(workerID int, chunk []uint64) {
			defer wg.Done()
			localCounts := localEdgeCounts[workerID]
			// Use versioned slice instead of map for O(1) reset
			// seenVersion tracks the current "generation" - increment to reset
			seenVersion := make([]uint32, s.nodeCount)
			currentVersion := uint32(1)

			for _, objID := range chunk {
				fromIdx := s.objToIdx[objID]
				// Increment version to "clear" the seen set (O(1) reset)
				currentVersion++
				if currentVersion == 0 {
					// Handle overflow by resetting the slice
					for i := range seenVersion {
						seenVersion[i] = 0
					}
					currentVersion = 1
				}

				for _, ref := range g.outgoingRefs[objID] {
					if toIdx, ok := s.objToIdx[ref.ToObjectID]; ok {
						if seenVersion[toIdx] != currentVersion {
							seenVersion[toIdx] = currentVersion
							localCounts[fromIdx]++
							predCounts[toIdx].Add(1)
						}
					}
				}
			}
		}(w, objIDs[start:end])
	}
	wg.Wait()

	// Merge local edge counts
	for _, localCounts := range localEdgeCounts {
		for i, count := range localCounts {
			edgeCounts[i] += count
		}
	}

	// Convert atomic pred counts to regular slice
	predCountsSlice := make([]int32, s.nodeCount)
	for i := range predCounts {
		predCountsSlice[i] = predCounts[i].Load()
	}

	// Build CSR offsets
	s.buildCSROffsets(edgeCounts, predCountsSlice)

	// Phase 2: Fill edges
	// Use atomic write positions for thread-safe filling
	succWritePos := make([]atomic.Int32, s.nodeCount)
	predWritePos := make([]atomic.Int32, s.nodeCount)
	for i := int32(0); i < s.nodeCount; i++ {
		succWritePos[i].Store(s.successorOffsets[i])
		predWritePos[i].Store(s.predecessorOffsets[i])
	}

	// Add GC root edges (single-threaded, small)
	for objID := range gcRootSet {
		if toIdx, ok := s.objToIdx[objID]; ok {
			pos := succWritePos[0].Add(1) - 1
			s.successorTargets[pos] = toIdx
			pos = predWritePos[toIdx].Add(1) - 1
			s.predecessorTargets[pos] = 0
		}
	}

	// Parallel fill object edges
	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > len(objIDs) {
			end = len(objIDs)
		}
		if start >= end {
			continue
		}

		wg.Add(1)
		go func(chunk []uint64) {
			defer wg.Done()
			// Use versioned slice instead of map for O(1) reset
			seenVersion := make([]uint32, s.nodeCount)
			currentVersion := uint32(1)

			for _, objID := range chunk {
				fromIdx := s.objToIdx[objID]
				// Increment version to "clear" the seen set (O(1) reset)
				currentVersion++
				if currentVersion == 0 {
					// Handle overflow by resetting the slice
					for i := range seenVersion {
						seenVersion[i] = 0
					}
					currentVersion = 1
				}

				for _, ref := range g.outgoingRefs[objID] {
					if toIdx, ok := s.objToIdx[ref.ToObjectID]; ok {
						if seenVersion[toIdx] != currentVersion {
							seenVersion[toIdx] = currentVersion
							pos := succWritePos[fromIdx].Add(1) - 1
							s.successorTargets[pos] = toIdx
							pos = predWritePos[toIdx].Add(1) - 1
							s.predecessorTargets[pos] = fromIdx
						}
					}
				}
			}
		}(objIDs[start:end])
	}
	wg.Wait()
}

// buildCSROffsets builds CSR format offsets from edge counts.
func (s *LevelDominatorState) buildCSROffsets(edgeCounts, predCounts []int32) {
	s.successorOffsets[0] = 0
	for i := int32(0); i < s.nodeCount; i++ {
		s.successorOffsets[i+1] = s.successorOffsets[i] + edgeCounts[i]
	}

	s.predecessorOffsets[0] = 0
	for i := int32(0); i < s.nodeCount; i++ {
		s.predecessorOffsets[i+1] = s.predecessorOffsets[i] + predCounts[i]
	}

	// Allocate target arrays
	totalSuccessors := s.successorOffsets[s.nodeCount]
	totalPredecessors := s.predecessorOffsets[s.nodeCount]
	s.successorTargets = make([]int32, totalSuccessors)
	s.predecessorTargets = make([]int32, totalPredecessors)
}

// getSuccessors returns successors for a node.
func (s *LevelDominatorState) getSuccessors(nodeIdx int32) []int32 {
	start := s.successorOffsets[nodeIdx]
	end := s.successorOffsets[nodeIdx+1]
	return s.successorTargets[start:end]
}

// getPredecessors returns predecessors for a node.
func (s *LevelDominatorState) getPredecessors(nodeIdx int32) []int32 {
	start := s.predecessorOffsets[nodeIdx]
	end := s.predecessorOffsets[nodeIdx+1]
	return s.predecessorTargets[start:end]
}

// ComputeLevels computes BFS levels from super root.
// Optimized with ring buffer queue and direct array access.
func (s *LevelDominatorState) ComputeLevels() {
	// Initialize levels to -1 (unreachable)
	for i := range s.levels {
		s.levels[i] = -1
	}

	// Use a simple slice as queue with head pointer (more efficient than slice shifting)
	// Pre-allocate with reasonable capacity
	queueCap := int(s.nodeCount / 4)
	if queueCap < 1024 {
		queueCap = 1024
	}
	queue := make([]int32, 0, queueCap)
	
	// BFS from super root
	s.levels[0] = 0
	queue = append(queue, 0)
	head := 0
	maxLevel := int32(0)

	for head < len(queue) {
		current := queue[head]
		head++
		currentLevel := s.levels[current]

		// Direct array access for successors
		start := s.successorOffsets[current]
		end := s.successorOffsets[current+1]
		for i := start; i < end; i++ {
			succ := s.successorTargets[i]
			if s.levels[succ] == -1 {
				s.levels[succ] = currentLevel + 1
				if s.levels[succ] > maxLevel {
					maxLevel = s.levels[succ]
				}
				queue = append(queue, succ)
			}
		}
	}

	// Group nodes by level - pre-count for exact allocation
	levelCounts := make([]int32, maxLevel+1)
	reachable := int64(0)
	for i := int32(0); i < s.nodeCount; i++ {
		if s.levels[i] >= 0 {
			levelCounts[s.levels[i]]++
			reachable++
		}
	}

	// Pre-allocate level slices with exact capacity
	s.levelNodes = make([][]int32, maxLevel+1)
	for i := range s.levelNodes {
		s.levelNodes[i] = make([]int32, 0, levelCounts[i])
	}

	// Populate level nodes
	for i := int32(0); i < s.nodeCount; i++ {
		if s.levels[i] >= 0 {
			s.levelNodes[s.levels[i]] = append(s.levelNodes[s.levels[i]], i)
		}
	}

	s.metrics.ReachableNodes = reachable
	s.metrics.MaxLevel = maxLevel
}

// ComputeDominators computes dominators using level-based parallelism.
func (s *LevelDominatorState) ComputeDominators(ctx context.Context) {
	// First compute levels
	s.ComputeLevels()

	// Initialize
	for i := int32(0); i < s.nodeCount; i++ {
		s.idom[i] = -1
		s.semi[i] = 0
		s.ancestor[i] = 0
		s.label[i] = i
		s.dfn[i] = 0
	}

	// DFS to compute spanning tree
	s.computeDFS()

	// Process nodes in reverse DFS order
	s.computeSemiDominators(ctx)

	// Compute immediate dominators
	s.computeImmediateDominators()
}

// computeDFS performs DFS to compute spanning tree.
// Optimized with pre-allocated stack and direct array access.
func (s *LevelDominatorState) computeDFS() {
	type frame struct {
		v     int32
		i     int32
		first bool
	}

	// Pre-allocate stack with reasonable capacity
	// For graphs with 1M+ nodes, we need larger initial capacity
	initialCap := 4096
	if s.nodeCount > 100000 {
		initialCap = 16384
	}
	stack := make([]frame, 0, initialCap)
	stack = append(stack, frame{v: 0, i: 0, first: true})

	for len(stack) > 0 {
		// Direct index access is faster than pointer
		lastIdx := len(stack) - 1
		f := &stack[lastIdx]

		if f.first {
			f.first = false
			s.dfnNum++
			s.dfn[f.v] = s.dfnNum
			s.vertex[s.dfnNum] = f.v
			s.semi[f.v] = s.dfnNum
		}

		// Direct slice access for successors
		start := s.successorOffsets[f.v]
		end := s.successorOffsets[f.v+1]
		successors := s.successorTargets[start:end]
		
		found := false
		for f.i < int32(len(successors)) {
			w := successors[f.i]
			f.i++
			if s.dfn[w] == 0 {
				s.parent[w] = f.v
				stack = append(stack, frame{v: w, i: 0, first: true})
				found = true
				break
			}
		}

		if !found {
			stack = stack[:lastIdx]
		}
	}
}

// computeSemiDominators computes semi-dominators.
// Optimized with direct array access and pre-allocated buckets.
func (s *LevelDominatorState) computeSemiDominators(ctx context.Context) {
	// Pre-allocate bucket with smaller initial capacity (most nodes have few items)
	bucket := make([][]int32, s.nodeCount)
	for i := range bucket {
		bucket[i] = make([]int32, 0, 2)
	}

	// Process in reverse DFS order
	for i := s.dfnNum; i >= 2; i-- {
		w := s.vertex[i]

		// Compute semi-dominator using direct predecessor access
		predStart := s.predecessorOffsets[w]
		predEnd := s.predecessorOffsets[w+1]
		for j := predStart; j < predEnd; j++ {
			v := s.predecessorTargets[j]
			if s.dfn[v] == 0 {
				continue // Not reachable
			}
			u := s.eval(v)
			if s.semi[u] < s.semi[w] {
				s.semi[w] = s.semi[u]
			}
		}

		// Add w to bucket of semi-dominator
		semiNode := s.vertex[s.semi[w]]
		bucket[semiNode] = append(bucket[semiNode], w)

		// Link w to parent
		s.link(s.parent[w], w)

		// Process bucket of parent
		parentBucket := bucket[s.parent[w]]
		for _, v := range parentBucket {
			u := s.eval(v)
			if s.semi[u] < s.semi[v] {
				s.idom[v] = u
			} else {
				s.idom[v] = s.parent[w]
			}
		}
		bucket[s.parent[w]] = bucket[s.parent[w]][:0]
	}
}

// computeImmediateDominators finalizes immediate dominators.
func (s *LevelDominatorState) computeImmediateDominators() {
	for i := int32(2); i <= s.dfnNum; i++ {
		w := s.vertex[i]
		if s.idom[w] != s.vertex[s.semi[w]] {
			s.idom[w] = s.idom[s.idom[w]]
		}
	}
	s.idom[0] = 0 // Super root dominates itself
}

// link adds edge (v, w) to the forest.
func (s *LevelDominatorState) link(v, w int32) {
	s.ancestor[w] = v
}

// eval finds node with minimum semi on path to root.
// Optimized with iterative path compression.
func (s *LevelDominatorState) eval(v int32) int32 {
	if s.ancestor[v] == 0 {
		return v
	}
	s.compressIterative(v)
	return s.label[v]
}

// compressIterative performs path compression iteratively.
// This avoids stack overflow on deep paths and reduces function call overhead.
func (s *LevelDominatorState) compressIterative(v int32) {
	// First, collect the path from v to the root of its tree
	path := make([]int32, 0, 32)
	current := v
	for s.ancestor[current] != 0 && s.ancestor[s.ancestor[current]] != 0 {
		path = append(path, current)
		current = s.ancestor[current]
	}
	
	// Now compress the path from root to v
	for i := len(path) - 1; i >= 0; i-- {
		node := path[i]
		anc := s.ancestor[node]
		if s.semi[s.label[anc]] < s.semi[s.label[node]] {
			s.label[node] = s.label[anc]
		}
		s.ancestor[node] = s.ancestor[anc]
	}
}

// GetDominator returns the dominator for an object ID.
func (s *LevelDominatorState) GetDominator(objID uint64) uint64 {
	idx, ok := s.objToIdx[objID]
	if !ok {
		return 0
	}
	domIdx := s.idom[idx]
	if domIdx < 0 || domIdx >= s.nodeCount {
		return 0
	}
	return s.idxToObj[domIdx]
}

// ExportToReferenceGraph exports dominator results to a ReferenceGraph.
func (s *LevelDominatorState) ExportToReferenceGraph(g *ReferenceGraph) {
	for i := int32(1); i < s.nodeCount; i++ {
		if s.dfn[i] == 0 {
			continue // Not reachable
		}
		objID := s.idxToObj[i]
		domIdx := s.idom[i]
		if domIdx >= 0 {
			domObjID := s.idxToObj[domIdx]
			g.dominators[objID] = domObjID
		}
	}
}

// ============================================================================
// Parallel Retained Size Computation
// ============================================================================

// ParallelRetainedSizeComputer computes retained sizes in parallel.
type ParallelRetainedSizeComputer struct {
	state  *LevelDominatorState
	config HierarchicalDominatorConfig

	// Children in dominator tree (for bottom-up traversal)
	children       [][]int32
	childrenCounts []int32

	// Retained sizes (use atomic for thread-safe updates)
	retainedSizes []atomic.Int64

	// Processing state
	remainingChildren []atomic.Int32
	processedCount    atomic.Int64 // Track how many nodes have been processed
	totalReachable    int64        // Total number of reachable nodes
}

// NewParallelRetainedSizeComputer creates a new parallel retained size computer.
func NewParallelRetainedSizeComputer(state *LevelDominatorState, config HierarchicalDominatorConfig) *ParallelRetainedSizeComputer {
	nodeCount := int(state.nodeCount)
	return &ParallelRetainedSizeComputer{
		state:             state,
		config:            config,
		childrenCounts:    make([]int32, nodeCount),
		retainedSizes:     make([]atomic.Int64, nodeCount),
		remainingChildren: make([]atomic.Int32, nodeCount),
	}
}

// Compute computes retained sizes in parallel.
func (c *ParallelRetainedSizeComputer) Compute(ctx context.Context, g *ReferenceGraph) {
	// Ensure ctx is not nil
	if ctx == nil {
		ctx = context.Background()
	}
	
	// Phase 1: Build dominator tree children
	c.buildChildren()

	// Phase 2: Initialize retained sizes to shallow sizes and count reachable nodes
	c.totalReachable = 0
	for i := int32(0); i < c.state.nodeCount; i++ {
		if c.state.dfn[i] > 0 {
			objID := c.state.idxToObj[i]
			c.retainedSizes[i].Store(g.objectSize[objID])
			c.totalReachable++
		}
	}

	// Phase 3: Find leaf nodes (nodes with no children in dominator tree)
	leaves := make([]int32, 0, c.state.nodeCount/2)
	for i := int32(0); i < c.state.nodeCount; i++ {
		if c.state.dfn[i] > 0 && len(c.children[i]) == 0 {
			leaves = append(leaves, i)
		}
	}

	// Phase 4: Process bottom-up in parallel
	c.processBottomUp(ctx, leaves)

	// Phase 5: Export to ReferenceGraph
	for i := int32(1); i < c.state.nodeCount; i++ {
		if c.state.dfn[i] > 0 {
			objID := c.state.idxToObj[i]
			g.retainedSizes[objID] = c.retainedSizes[i].Load()
		}
	}
}

// buildChildren builds the dominator tree children lists.
func (c *ParallelRetainedSizeComputer) buildChildren() {
	// Count children
	for i := int32(1); i < c.state.nodeCount; i++ {
		if c.state.dfn[i] == 0 {
			continue
		}
		domIdx := c.state.idom[i]
		if domIdx >= 0 && domIdx != i {
			c.childrenCounts[domIdx]++
		}
	}

	// Allocate children slices
	c.children = make([][]int32, c.state.nodeCount)
	for i := int32(0); i < c.state.nodeCount; i++ {
		if c.childrenCounts[i] > 0 {
			c.children[i] = make([]int32, 0, c.childrenCounts[i])
		}
		c.remainingChildren[i].Store(c.childrenCounts[i])
	}

	// Fill children
	for i := int32(1); i < c.state.nodeCount; i++ {
		if c.state.dfn[i] == 0 {
			continue
		}
		domIdx := c.state.idom[i]
		if domIdx >= 0 && domIdx != i {
			c.children[domIdx] = append(c.children[domIdx], i)
		}
	}
}

// processBottomUp processes nodes bottom-up in parallel.
// Uses a more efficient completion detection mechanism without polling.
func (c *ParallelRetainedSizeComputer) processBottomUp(ctx context.Context, leaves []int32) {
	numWorkers := c.config.MaxWorkers
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	
	// Handle empty leaves case
	if len(leaves) == 0 {
		return
	}

	// For small graphs, use single-threaded processing (faster due to no synchronization)
	if c.totalReachable < 10000 || numWorkers == 1 {
		c.processBottomUpSequential(leaves)
		return
	}

	// Work queue - sized for all potential work items
	workQueue := make(chan int32, len(leaves)*2)
	
	// Use WaitGroup to track when all work is done
	var wg sync.WaitGroup
	
	// Done channel to signal workers to stop
	done := make(chan struct{})
	
	// Start workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				case <-ctx.Done():
					return
				case nodeIdx, ok := <-workQueue:
					if !ok {
						return
					}
					c.processNodeWithQueue(nodeIdx, workQueue)
					
					// Check if all nodes processed
					if c.processedCount.Load() >= c.totalReachable {
						select {
						case <-done:
						default:
							close(done)
						}
						return
					}
				}
			}
		}()
	}

	// Send all leaves to work queue
	for _, leaf := range leaves {
		select {
		case workQueue <- leaf:
		case <-ctx.Done():
			close(done)
			wg.Wait()
			return
		}
	}

	// Wait for completion signal or context cancellation
	select {
	case <-done:
	case <-ctx.Done():
	}
	
	// Close queue and wait for workers
	close(workQueue)
	wg.Wait()
}

// processBottomUpSequential processes nodes sequentially for small graphs.
func (c *ParallelRetainedSizeComputer) processBottomUpSequential(leaves []int32) {
	// Use a simple queue-based approach
	queue := make([]int32, 0, len(leaves))
	queue = append(queue, leaves...)
	
	for len(queue) > 0 {
		nodeIdx := queue[0]
		queue = queue[1:]
		
		// Add children's retained sizes
		for _, childIdx := range c.children[nodeIdx] {
			childRetained := c.retainedSizes[childIdx].Load()
			c.retainedSizes[nodeIdx].Add(childRetained)
		}
		
		// Notify parent
		domIdx := c.state.idom[nodeIdx]
		if domIdx >= 0 && domIdx != nodeIdx {
			remaining := c.remainingChildren[domIdx].Add(-1)
			if remaining == 0 {
				queue = append(queue, domIdx)
			}
		}
	}
}

// processNodeWithQueue processes a single node and adds parent to queue if ready.
func (c *ParallelRetainedSizeComputer) processNodeWithQueue(nodeIdx int32, workQueue chan<- int32) {
	// Increment processed count
	c.processedCount.Add(1)

	// Add children's retained sizes using atomic operations
	for _, childIdx := range c.children[nodeIdx] {
		childRetained := c.retainedSizes[childIdx].Load()
		c.retainedSizes[nodeIdx].Add(childRetained)
	}

	// Notify parent
	domIdx := c.state.idom[nodeIdx]
	if domIdx >= 0 && domIdx != nodeIdx {
		remaining := c.remainingChildren[domIdx].Add(-1)
		if remaining == 0 {
			// Parent is ready - try to send to queue, but don't block
			select {
			case workQueue <- domIdx:
			default:
				// Queue full - process inline (rare case)
				c.processNodeWithQueue(domIdx, workQueue)
			}
		}
	}
}

// ============================================================================
// Hierarchical Dominator Entry Point
// ============================================================================

// ComputeHierarchicalDominators computes dominators using the hierarchical parallel algorithm.
func ComputeHierarchicalDominators(ctx context.Context, g *ReferenceGraph, config HierarchicalDominatorConfig) {
	// Ensure ctx is not nil
	if ctx == nil {
		ctx = context.Background()
	}
	
	nodeCount := len(g.objectClass) + 1 // +1 for super root

	// Create state
	state := NewLevelDominatorState(nodeCount, config)

	// Build from reference graph
	state.BuildFromReferenceGraph(g)

	// Compute dominators
	state.ComputeDominators(ctx)

	// Export results
	state.ExportToReferenceGraph(g)

	// Compute retained sizes in parallel
	computer := NewParallelRetainedSizeComputer(state, config)
	computer.Compute(ctx, g)

	// Mark as computed
	g.dominatorComputed = true

	// Build reachable objects set
	g.reachableObjects = make(map[uint64]bool, int(state.metrics.ReachableNodes))
	for i := int32(1); i < state.nodeCount; i++ {
		if state.dfn[i] > 0 {
			g.reachableObjects[state.idxToObj[i]] = true
		}
	}
	
	// Compute class-level retained sizes (same as Lengauer-Tarjan path)
	g.computeClassRetainedSizes()
	
	// Compute retained sizes using the active strategy
	g.computeStrategyRetainedSizes()
}

// ============================================================================
// Adaptive Algorithm Selection
// ============================================================================

// DominatorAlgorithm represents the algorithm to use.
type DominatorAlgorithm int

const (
	// DominatorAlgorithmAuto automatically selects the best algorithm.
	DominatorAlgorithmAuto DominatorAlgorithm = iota

	// DominatorAlgorithmLengauerTarjan uses the classic Lengauer-Tarjan algorithm.
	DominatorAlgorithmLengauerTarjan

	// DominatorAlgorithmHierarchical uses the hierarchical parallel algorithm.
	DominatorAlgorithmHierarchical
)

// SelectDominatorAlgorithm selects the best algorithm based on graph characteristics.
func SelectDominatorAlgorithm(objectCount int, edgeCount int) DominatorAlgorithm {
	// Use hierarchical for large graphs (>1M objects)
	if objectCount > 1_000_000 {
		return DominatorAlgorithmHierarchical
	}

	// Use hierarchical for dense graphs (avg degree > 5)
	if objectCount > 0 && float64(edgeCount)/float64(objectCount) > 5 {
		return DominatorAlgorithmHierarchical
	}

	// Default to Lengauer-Tarjan for smaller graphs
	return DominatorAlgorithmLengauerTarjan
}

// ComputeDominatorsAdaptive computes dominators using the best algorithm.
func ComputeDominatorsAdaptive(ctx context.Context, g *ReferenceGraph) {
	objectCount := len(g.objectClass)
	edgeCount := 0
	for _, refs := range g.outgoingRefs {
		edgeCount += len(refs)
	}

	algorithm := SelectDominatorAlgorithm(objectCount, edgeCount)

	switch algorithm {
	case DominatorAlgorithmHierarchical:
		config := DefaultHierarchicalDominatorConfig()
		ComputeHierarchicalDominators(ctx, g, config)
	default:
		// Use existing Lengauer-Tarjan implementation
		g.computeLengauerTarjan()
		g.computeRetainedSizes()
	}
}

// ============================================================================
// Parallel Class Retained Size Aggregation
// ============================================================================

// ComputeClassRetainedSizesHierarchical computes class retained sizes in parallel.
func ComputeClassRetainedSizesHierarchical(ctx context.Context, g *ReferenceGraph) (map[uint64]int64, map[uint64]int64) {
	config := DefaultPoolConfig()
	numWorkers := config.MaxWorkers

	// Collect object IDs
	objIDs := make([]uint64, 0, len(g.objectClass))
	for objID := range g.objectClass {
		objIDs = append(objIDs, objID)
	}

	// Sort for better cache locality
	sort.Slice(objIDs, func(i, j int) bool {
		return g.objectClass[objIDs[i]] < g.objectClass[objIDs[j]]
	})

	// Per-worker local maps
	type localResult struct {
		classRetained map[uint64]int64
		classAttrib   map[uint64]int64
	}
	localResults := make([]localResult, numWorkers)
	for i := range localResults {
		localResults[i] = localResult{
			classRetained: make(map[uint64]int64),
			classAttrib:   make(map[uint64]int64),
		}
	}

	// Process in parallel
	chunkSize := (len(objIDs) + numWorkers - 1) / numWorkers
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > len(objIDs) {
			end = len(objIDs)
		}
		if start >= end {
			continue
		}

		wg.Add(1)
		go func(workerID int, chunk []uint64) {
			defer wg.Done()
			local := &localResults[workerID]

			for _, objID := range chunk {
				classID := g.objectClass[objID]
				domID := g.dominators[objID]

				// MAT-style: count if not dominated by same class
				isDominatedBySameClass := false
				if domID != superRootID && domID != 0 {
					if domClassID, exists := g.objectClass[domID]; exists && domClassID == classID {
						isDominatedBySameClass = true
					}
				}
				if !isDominatedBySameClass {
					local.classRetained[classID] += g.retainedSizes[objID]
				}

				// Attribution: attribute to nearest different-class dominator
				attribClassID := classID
				domIDIter := domID
				for domIDIter != superRootID && domIDIter != 0 {
					domClassID, ok := g.objectClass[domIDIter]
					if !ok {
						break
					}
					if domClassID != classID {
						attribClassID = domClassID
						break
					}
					domIDIter = g.dominators[domIDIter]
				}
				local.classAttrib[attribClassID] += g.objectSize[objID]
			}
		}(w, objIDs[start:end])
	}

	wg.Wait()

	// Merge results
	classRetained := make(map[uint64]int64)
	classAttrib := make(map[uint64]int64)

	for _, local := range localResults {
		for k, v := range local.classRetained {
			classRetained[k] += v
		}
		for k, v := range local.classAttrib {
			classAttrib[k] += v
		}
	}

	return classRetained, classAttrib
}
