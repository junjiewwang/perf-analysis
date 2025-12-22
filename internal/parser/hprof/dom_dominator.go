// Package hprof provides parsing functionality for Java HPROF heap dump files.
// This file contains the dominator tree computation and retained size calculation.
package hprof

import (
	"sync"
)

// superRootID is a special ID representing the super root that dominates all GC roots
const superRootID = ^uint64(0)

// retainedSizeEstimated indicates if retained sizes are estimated (not exact).
var retainedSizeEstimated bool

// dominatorState holds the state for dominator computation
type dominatorState struct {
	// Object ID to index mapping for array-based access
	objToIdx map[uint64]int32
	idxToObj []uint64

	// Algorithm data structures (indexed by node index, 1-based)
	// Index 0 is reserved for "undefined"
	parent   []int32   // parent in DFS spanning tree
	semi     []int32   // semidominator (as DFS number)
	idom     []int32   // immediate dominator
	ancestor []int32   // ancestor in forest for path compression
	label    []int32   // label for path compression (best semi on path)
	bucket   [][]int32 // bucket[w] = nodes whose semidominator is w

	// DFS data
	dfn    []int32 // dfn[v] = DFS number of node v (0 = not visited)
	vertex []int32 // vertex[i] = node with DFS number i (1-based)
	n      int32   // number of nodes visited by DFS

	// Optimization: pre-computed successor counts for capacity pre-allocation
	successorCounts []int32
}

// getNumWorkers returns the number of parallel workers to use.
// Deprecated: Use DefaultPoolConfig().MaxWorkers instead.
func getNumWorkers() int {
	return DefaultPoolConfig().MaxWorkers
}

// ComputeDominatorTree computes the dominator tree using the best available algorithm.
// For small graphs (<1M objects): Uses Lengauer-Tarjan algorithm with O(E·α(E,V)) complexity.
// For large graphs (>=1M objects): Uses hierarchical parallel algorithm for better performance.
func (g *ReferenceGraph) ComputeDominatorTree() {
	if g.dominatorComputed {
		return
	}

	// Select algorithm based on graph size
	objectCount := len(g.objectClass)
	edgeCount := 0
	for _, refs := range g.outgoingRefs {
		edgeCount += len(refs)
	}

	algorithm := SelectDominatorAlgorithm(objectCount, edgeCount)

	switch algorithm {
	case DominatorAlgorithmHierarchical:
		g.debugf("Using hierarchical parallel dominator algorithm for %d objects, %d edges", objectCount, edgeCount)
		config := DefaultHierarchicalDominatorConfig()
		ComputeHierarchicalDominators(nil, g, config)
	default:
		g.debugf("Using Lengauer-Tarjan dominator algorithm for %d objects, %d edges", objectCount, edgeCount)
		g.computeLengauerTarjan()
		g.computeRetainedSizes()
	}

	g.dominatorComputed = true
	retainedSizeEstimated = false
}

// ComputeDominatorTreeWithConfig computes the dominator tree with custom configuration.
func (g *ReferenceGraph) ComputeDominatorTreeWithConfig(config HierarchicalDominatorConfig) {
	if g.dominatorComputed {
		return
	}

	ComputeHierarchicalDominators(nil, g, config)
	g.dominatorComputed = true
	retainedSizeEstimated = false
}

// computeLengauerTarjan implements the Lengauer-Tarjan algorithm for computing dominators.
// This is the standard algorithm used by Eclipse MAT and other professional tools.
// Time complexity: O(E·α(E,V)) where α is the inverse Ackermann function (nearly linear).
//
// Optimizations applied:
// 1. Uses int32 instead of int to reduce memory footprint by 50%
// 2. Pre-allocates successor slice capacities based on outgoing reference counts
// 3. Uses compact index mapping (objToIdx) for O(1) lookups
//
// Reference: "A Fast Algorithm for Finding Dominators in a Flowgraph"
// by Thomas Lengauer and Robert Endre Tarjan, 1979
func (g *ReferenceGraph) computeLengauerTarjan() {
	numObjects := len(g.objectClass)
	if numObjects == 0 {
		return
	}

	// Total nodes = objects + 1 (for virtual super root at index 0)
	totalNodes := numObjects + 1

	// Initialize state with int32 for memory efficiency
	state := &dominatorState{
		objToIdx:        make(map[uint64]int32, totalNodes),
		idxToObj:        make([]uint64, totalNodes),
		parent:          make([]int32, totalNodes),
		semi:            make([]int32, totalNodes),
		idom:            make([]int32, totalNodes),
		ancestor:        make([]int32, totalNodes),
		label:           make([]int32, totalNodes),
		bucket:          make([][]int32, totalNodes),
		dfn:             make([]int32, totalNodes),
		vertex:          make([]int32, totalNodes),
		successorCounts: make([]int32, totalNodes),
		n:               0,
	}

	// Index 0 = super root (virtual node that dominates all GC roots)
	state.objToIdx[superRootID] = 0
	state.idxToObj[0] = superRootID

	// Create indices for all objects (1-based) and count successors
	idx := int32(1)
	for objID := range g.objectClass {
		state.objToIdx[objID] = idx
		state.idxToObj[idx] = objID
		// Pre-count successors for capacity pre-allocation
		state.successorCounts[idx] = int32(len(g.outgoingRefs[objID]))
		idx++
	}

	// Initialize arrays
	for i := int32(0); i < int32(totalNodes); i++ {
		state.semi[i] = 0     // 0 means undefined
		state.ancestor[i] = 0 // 0 means no ancestor
		state.label[i] = i    // initially, label[v] = v
		state.idom[i] = 0     // 0 means undefined
		state.dfn[i] = 0      // 0 means not visited
	}

	// Build successors list with pre-allocated capacity
	successors := make([][]int32, totalNodes)
	for i := range successors {
		// Pre-allocate based on known successor count
		cap := state.successorCounts[i]
		if cap == 0 {
			cap = 4 // Small default for nodes without pre-counted refs
		}
		successors[i] = make([]int32, 0, cap)
	}

	// Super root (index 0) has edges to all GC roots
	gcRootSet := make(map[uint64]bool, len(g.gcRoots)+len(g.classObjectIDs))
	gcRootsFound := 0
	gcRootsNotFound := 0
	for _, root := range g.gcRoots {
		if rootIdx, ok := state.objToIdx[root.ObjectID]; ok {
			if !gcRootSet[root.ObjectID] {
				gcRootSet[root.ObjectID] = true
				successors[0] = append(successors[0], rootIdx)
				gcRootsFound++
			}
		} else {
			gcRootsNotFound++
		}
	}

	// Treat all Class objects as implicit GC roots.
	classObjectsAdded := 0
	for classObjID := range g.classObjectIDs {
		if rootIdx, ok := state.objToIdx[classObjID]; ok {
			if !gcRootSet[classObjID] {
				gcRootSet[classObjID] = true
				successors[0] = append(successors[0], rootIdx)
				classObjectsAdded++
			}
		}
	}
	g.debugf("Class objects count: %d, added as implicit GC roots: %d", len(g.classObjectIDs), classObjectsAdded)

	// Debug: count objects with no incoming refs
	noIncomingCount := 0
	for objID := range g.objectClass {
		if len(g.incomingRefs[objID]) == 0 && !gcRootSet[objID] {
			noIncomingCount++
		}
	}
	g.debugf("Objects with no incoming refs (not added as roots): %d", noIncomingCount)

	// Add edges from each object to objects it references
	// PARALLEL OPTIMIZATION: Build successors in parallel using multiple workers
	numWorkers := getNumWorkers()

	// Collect all object IDs into a slice for parallel processing
	objIDs := make([]uint64, 0, len(g.objectClass))
	for objID := range g.objectClass {
		objIDs = append(objIDs, objID)
	}

	// Create per-worker local successors to avoid lock contention
	type localSuccessors struct {
		data []struct {
			fromIdx int32
			toIdx   int32
		}
	}
	workerResults := make([]localSuccessors, numWorkers)

	// Estimate capacity per worker
	avgRefsPerWorker := len(objIDs) * 3 / numWorkers // Assume avg 3 refs per object
	for i := range workerResults {
		workerResults[i].data = make([]struct {
			fromIdx int32
			toIdx   int32
		}, 0, avgRefsPerWorker)
	}

	// Process objects in parallel
	var wg sync.WaitGroup
	chunkSize := (len(objIDs) + numWorkers - 1) / numWorkers

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
		go func(workerID int, objSlice []uint64) {
			defer wg.Done()
			// Per-worker seen map to deduplicate
			seen := make(map[int32]bool, 16)

			for _, objID := range objSlice {
				fromIdx := state.objToIdx[objID]
				// Clear seen map for this object
				for k := range seen {
					delete(seen, k)
				}
				for _, ref := range g.outgoingRefs[objID] {
					if toIdx, ok := state.objToIdx[ref.ToObjectID]; ok {
						if !seen[toIdx] {
							seen[toIdx] = true
							workerResults[workerID].data = append(workerResults[workerID].data, struct {
								fromIdx int32
								toIdx   int32
							}{fromIdx, toIdx})
						}
					}
				}
			}
		}(w, objIDs[start:end])
	}
	wg.Wait()

	// Merge worker results into successors
	for _, wr := range workerResults {
		for _, edge := range wr.data {
			successors[edge.fromIdx] = append(successors[edge.fromIdx], edge.toIdx)
		}
	}

	// Build predecessors list with pre-allocated capacity
	// PARALLEL OPTIMIZATION: Count predecessors in parallel using worker pool
	predecessors := buildPredecessorsParallel(successors, totalNodes)

	// Step 1: DFS to compute spanning tree and DFS numbering
	// Use iterative DFS to avoid stack overflow on large graphs
	type dfsFrame struct {
		v     int32
		i     int32 // index into successors[v]
		first bool
	}

	stack := make([]dfsFrame, 0, 1024) // Pre-allocate stack
	stack = append(stack, dfsFrame{v: 0, i: 0, first: true})

	for len(stack) > 0 {
		frame := &stack[len(stack)-1]

		if frame.first {
			frame.first = false
			state.n++
			state.dfn[frame.v] = state.n
			state.vertex[state.n] = frame.v
			state.semi[frame.v] = state.n // Initialize semi to DFS number
		}

		// Find next unvisited successor
		found := false
		for frame.i < int32(len(successors[frame.v])) {
			w := successors[frame.v][frame.i]
			frame.i++
			if state.dfn[w] == 0 { // Not visited
				state.parent[w] = frame.v
				stack = append(stack, dfsFrame{v: w, i: 0, first: true})
				found = true
				break
			}
		}

		if !found {
			stack = stack[:len(stack)-1]
		}
	}

	g.debugf("DFS visited %d nodes out of %d total (%.1f%%)",
		state.n, totalNodes, float64(state.n)*100.0/float64(totalNodes))
	g.debugf("GC roots: %d explicit + %d class objects = %d total (found=%d, notFound=%d)",
		len(g.gcRoots), classObjectsAdded, len(gcRootSet), gcRootsFound, gcRootsNotFound)

	// LINK: add edge (v, w) to the forest
	link := func(v, w int32) {
		state.ancestor[w] = v
	}

	// EVAL: find the node with minimum semi on the path from v to root of its tree
	var eval func(v int32) int32
	eval = func(v int32) int32 {
		if state.ancestor[v] == 0 {
			return v
		}
		compressPath32(state, v)
		return state.label[v]
	}

	// Steps 2 & 3: Compute semidominators and implicitly define idom
	// Process nodes in reverse DFS order (excluding root)
	for i := state.n; i >= 2; i-- {
		w := state.vertex[i]

		// Step 2: Compute semidominator of w
		for _, v := range predecessors[w] {
			if state.dfn[v] == 0 {
				continue // v not reachable from root
			}
			var u int32
			if state.dfn[v] <= state.dfn[w] {
				u = v
			} else {
				u = eval(v)
			}
			if state.semi[u] < state.semi[w] {
				state.semi[w] = state.semi[u]
			}
		}

		// Add w to bucket of vertex[semi[w]]
		semiNode := state.vertex[state.semi[w]]
		state.bucket[semiNode] = append(state.bucket[semiNode], w)

		// Link w to its parent in DFS tree
		link(state.parent[w], w)

		// Step 3: Implicitly define idom for nodes in bucket of parent[w]
		for _, v := range state.bucket[state.parent[w]] {
			u := eval(v)
			if state.semi[u] < state.semi[v] {
				state.idom[v] = u
			} else {
				state.idom[v] = state.parent[w]
			}
		}
		state.bucket[state.parent[w]] = nil
	}

	// Step 4: Explicitly define idom
	for i := int32(2); i <= state.n; i++ {
		w := state.vertex[i]
		if state.idom[w] != state.vertex[state.semi[w]] {
			state.idom[w] = state.idom[state.idom[w]]
		}
	}

	// idom of root is 0 (undefined)
	state.idom[0] = 0

	// Convert results back to object IDs and mark reachable objects
	g.reachableObjects = make(map[uint64]bool, int(state.n))
	for i := int32(1); i <= state.n; i++ {
		v := state.vertex[i]
		if v == 0 {
			continue // skip super root
		}
		objID := state.idxToObj[v]
		g.reachableObjects[objID] = true // Mark as reachable
		idomNode := state.idom[v]
		if idomNode == 0 {
			g.dominators[objID] = superRootID
		} else {
			g.dominators[objID] = state.idxToObj[idomNode]
		}
	}

	// Handle unreachable objects (not visited by DFS)
	unreachableCount := 0
	for objID := range g.objectClass {
		if _, hasDom := g.dominators[objID]; !hasDom {
			g.dominators[objID] = superRootID
			unreachableCount++
		}
	}
	g.debugf("Unreachable objects (garbage): %d", unreachableCount)

	// Compute retained sizes
	g.computeRetainedSizes()
}

// compressPath32 performs path compression for EVAL using iterative approach (int32 version).
// After compression, label[v] contains the node with minimum semi on path from v to tree root.
// Optimization: Uses iterative approach to avoid stack overflow on deep paths.
func compressPath32(state *dominatorState, v int32) {
	// First, collect the path from v to the root of its tree
	path := make([]int32, 0, 32)
	current := v
	for state.ancestor[current] != 0 && state.ancestor[state.ancestor[current]] != 0 {
		path = append(path, current)
		current = state.ancestor[current]
	}

	// Now compress the path from root to v
	for i := len(path) - 1; i >= 0; i-- {
		node := path[i]
		anc := state.ancestor[node]
		if state.semi[state.label[anc]] < state.semi[state.label[node]] {
			state.label[node] = state.label[anc]
		}
		state.ancestor[node] = state.ancestor[anc]
	}
}

// IsRetainedSizeEstimated returns true if retained sizes are estimated rather than exact.
func (g *ReferenceGraph) IsRetainedSizeEstimated() bool {
	return retainedSizeEstimated
}

// computeRetainedSizes computes retained sizes based on dominator tree.
// Retained size of an object = its shallow size + sum of retained sizes of all objects it dominates
// Also pre-computes class-level retained sizes for fast lookup.
//
// Optimization: Uses iterative post-order traversal instead of recursion to avoid stack overflow
// on large heaps and reduce function call overhead.
// PARALLEL OPTIMIZATION: Parallelizes initialization and class retained size computation via worker pool.
// MEMORY OPTIMIZATION: Uses slice-based children tracking with pre-allocated capacity.
func (g *ReferenceGraph) computeRetainedSizes() {
	numObjects := len(g.objectClass)

	// Build object ID to index mapping for slice-based access
	objToIdx := make(map[uint64]int32, numObjects)
	idxToObj := make([]uint64, 0, numObjects)
	idx := int32(0)
	for objID := range g.objectClass {
		objToIdx[objID] = idx
		idxToObj = append(idxToObj, objID)
		idx++
	}

	// Phase 1: Count children for each node (for pre-allocation)
	childCounts := make([]int32, numObjects)
	dominatedBySuperRoot := 0
	dominatedByOther := 0

	for objID := range g.objectClass {
		domID := g.dominators[objID]
		if domID == superRootID {
			dominatedBySuperRoot++
		} else if domID != 0 && domID != objID {
			if domIdx, exists := objToIdx[domID]; exists {
				childCounts[domIdx]++
				dominatedByOther++
			}
		}
	}

	g.debugf("Dominator stats: dominatedBySuperRoot=%d, dominatedByOther=%d",
		dominatedBySuperRoot, dominatedByOther)

	// Phase 2: Allocate children slices with exact capacity
	children := make([][]uint64, numObjects)
	for i, count := range childCounts {
		if count > 0 {
			children[i] = make([]uint64, 0, count)
		}
	}

	// Phase 3: Populate children
	for objID := range g.objectClass {
		domID := g.dominators[objID]
		if domID != superRootID && domID != 0 && domID != objID {
			if domIdx, exists := objToIdx[domID]; exists {
				children[domIdx] = append(children[domIdx], objID)
			}
		}
	}

	// Initialize all retained sizes to shallow size
	for objID := range g.objectClass {
		g.retainedSizes[objID] = g.objectSize[objID]
	}

	// Compute retained sizes using iterative post-order traversal
	// Use slice-based tracking instead of maps

	// Find all leaf nodes (objects with no children in dominator tree)
	leafNodes := make([]uint64, 0, numObjects/2)
	for i := int32(0); i < int32(numObjects); i++ {
		if len(children[i]) == 0 {
			leafNodes = append(leafNodes, idxToObj[i])
		}
	}

	// Track remaining children count using slice (not map)
	remainingChildren := make([]int32, numObjects)
	copy(remainingChildren, childCounts)

	// Use bitset for processed tracking instead of map
	processed := NewBitset(numObjects)

	// Process nodes in bottom-up order using a work queue
	queue := make([]uint64, 0, numObjects)
	queue = append(queue, leafNodes...)

	for len(queue) > 0 {
		objID := queue[0]
		queue = queue[1:]

		objIdx := objToIdx[objID]
		if processed.Test(int(objIdx)) {
			continue
		}
		processed.Set(int(objIdx))

		// Retained size is already initialized to shallow size
		// Add retained sizes of all children (already computed since we process bottom-up)
		for _, childID := range children[objIdx] {
			g.retainedSizes[objID] += g.retainedSizes[childID]
		}

		// Notify parent that this child is done
		domID := g.dominators[objID]
		if domID != superRootID && domID != 0 {
			if domIdx, exists := objToIdx[domID]; exists {
				remainingChildren[domIdx]--
				// If all children of parent are processed, add parent to queue
				if remainingChildren[domIdx] == 0 {
					queue = append(queue, domID)
				}
			}
		}
	}

	// Handle any remaining unprocessed objects (shouldn't happen in a valid tree)
	for objID := range g.objectClass {
		objIdx := objToIdx[objID]
		if !processed.Test(int(objIdx)) {
			// Process remaining objects - their retained size stays as shallow size
			processed.Set(int(objIdx))
		}
	}

	// Debug: count objects with retained > shallow
	objectsWithRetained := 0
	for objID := range g.objectClass {
		if g.retainedSizes[objID] > g.objectSize[objID] {
			objectsWithRetained++
		}
	}
	g.debugf("Objects with retained > shallow: %d", objectsWithRetained)

	// Pre-compute class-level retained sizes for fast lookup
	g.computeClassRetainedSizes()

	// Compute retained sizes using the active strategy
	g.computeStrategyRetainedSizes()
}

// computeClassRetainedSizes pre-computes two views of class retained size:
// 1) MAT top-level view (classRetainedSizes): retained of instances not dominated by same class
// 2) Attribution view (classRetainedSizesAttributed): non-overlapping, attribute each object's
//
//	retained size to the nearest dominator of a different class; if dominator is super root,
//	attribute to the object's own class. This better matches IDE "Retained by class"口径。
//
// PARALLEL OPTIMIZATION: Uses parallel aggregation with per-worker local maps via worker pool.
func (g *ReferenceGraph) computeClassRetainedSizes() {
	// Reset maps in case ComputeDominatorTree is called multiple times
	g.classRetainedSizes = make(map[uint64]int64)
	g.classRetainedSizesAttributed = make(map[uint64]int64)

	// Collect object IDs for parallel processing
	objIDs := make([]uint64, 0, len(g.objectClass))
	for objID := range g.objectClass {
		objIDs = append(objIDs, objID)
	}

	// Use the unified parallel aggregation helper
	g.classRetainedSizes, g.classRetainedSizesAttributed = computeClassRetainedSizesParallel(g, objIDs)
}

// computeStrategyRetainedSizes computes retained sizes using the active strategy.
// This is the pluggable retained size calculation that uses the strategy pattern.
func (g *ReferenceGraph) computeStrategyRetainedSizes() {
	if g.retainedSizeCalculatorRegistry == nil {
		g.retainedSizeCalculatorRegistry = NewRetainedSizeCalculatorRegistry()
	}

	// Get the active calculator
	calc, ok := g.retainedSizeCalculatorRegistry.Get(g.activeRetainedSizeStrategy)
	if !ok {
		calc = g.retainedSizeCalculatorRegistry.GetDefault()
	}

	// Build the context for the calculator
	ctx := g.buildRetainedSizeContext()

	// Compute retained sizes using the strategy
	g.computedRetainedSizes = calc.ComputeRetainedSizes(g.retainedSizes, ctx)

	g.debugf("Computed retained sizes using strategy: %s", calc.Name())
}

// buildRetainedSizeContext creates a RetainedSizeContext from the current graph state.
// This method ensures object index is built for O(1) index-based access.
func (g *ReferenceGraph) buildRetainedSizeContext() *RetainedSizeContext {
	// Ensure object index is built for index-based access
	g.buildObjectIndex()
	// Build index-based structures for hot path optimization
	g.buildDominatorByIndex()
	g.buildOutgoingRefsByIndex()
	g.buildIncomingRefsByIndex()

	return &RetainedSizeContext{
		GetObjectSize: func(objectID uint64) int64 {
			return g.objectSize[objectID]
		},
		GetObjectClassID: func(objectID uint64) (uint64, bool) {
			classID, ok := g.objectClass[objectID]
			return classID, ok
		},
		GetClassName: func(classID uint64) string {
			return g.classNames[classID]
		},
		GetDominator: func(objectID uint64) uint64 {
			return g.dominators[objectID]
		},
		GetOutgoingRefs: func(objectID uint64) []ObjectReference {
			return g.outgoingRefs[objectID]
		},
		GetIncomingRefs: func(objectID uint64) []ObjectReference {
			return g.incomingRefs[objectID]
		},
		// Index-based accessors for O(1) access in hot paths
		GetObjectIndex: func(objectID uint64) int {
			if idx, ok := g.objectIDToIndex[objectID]; ok {
				return idx
			}
			return -1
		},
		GetObjectIDByIndex: func(idx int) uint64 {
			if idx < 0 || idx >= len(g.indexToObjectID) {
				return 0
			}
			return g.indexToObjectID[idx]
		},
		GetObjectClassIDByIdx: func(idx int) (uint64, bool) {
			if idx < 0 || idx >= len(g.objectClassByIndex) {
				return 0, false
			}
			return g.objectClassByIndex[idx], true
		},
		GetObjectSizeByIdx: func(idx int) int64 {
			if idx < 0 || idx >= len(g.objectSizeByIndex) {
				return 0
			}
			return g.objectSizeByIndex[idx]
		},
		GetDominatorByIdx: func(idx int) int {
			if idx < 0 || idx >= len(g.dominatorByIndex) {
				return -2
			}
			return g.dominatorByIndex[idx]
		},
		GetOutgoingRefsByIdx: func(idx int) []IndexedOutRef {
			if idx < 0 || idx >= len(g.outgoingRefsByIndex) {
				return nil
			}
			return g.outgoingRefsByIndex[idx]
		},
		GetIncomingRefsByIdx: func(idx int) []IndexedOutRef {
			if idx < 0 || idx >= len(g.incomingRefsByIndex) {
				return nil
			}
			return g.incomingRefsByIndex[idx]
		},
		ObjectCount: len(g.objectClass),
		ForEachObject: func(fn func(objectID uint64)) {
			for objID := range g.objectClass {
				fn(objID)
			}
		},
		SuperRootID: superRootID,
		Debugf:      g.debugf,
	}
}

// SetRetainedSizeStrategy sets the retained size calculation strategy.
// This will trigger recomputation of retained sizes if the dominator tree is already computed.
func (g *ReferenceGraph) SetRetainedSizeStrategy(strategy RetainedSizeStrategy) {
	if g.activeRetainedSizeStrategy == strategy {
		return
	}

	g.activeRetainedSizeStrategy = strategy

	// Recompute if dominator tree is already computed
	if g.dominatorComputed {
		g.computeStrategyRetainedSizes()
	}
}

// GetRetainedSizeStrategy returns the current retained size calculation strategy.
func (g *ReferenceGraph) GetRetainedSizeStrategy() RetainedSizeStrategy {
	return g.activeRetainedSizeStrategy
}

// GetAvailableStrategies returns all available retained size calculation strategies.
func (g *ReferenceGraph) GetAvailableStrategies() []RetainedSizeStrategy {
	if g.retainedSizeCalculatorRegistry == nil {
		g.retainedSizeCalculatorRegistry = NewRetainedSizeCalculatorRegistry()
	}
	return g.retainedSizeCalculatorRegistry.ListStrategies()
}

// RegisterRetainedSizeCalculator registers a custom retained size calculator.
// This allows extending the system with new calculation strategies.
func (g *ReferenceGraph) RegisterRetainedSizeCalculator(calc RetainedSizeCalculator) {
	if g.retainedSizeCalculatorRegistry == nil {
		g.retainedSizeCalculatorRegistry = NewRetainedSizeCalculatorRegistry()
	}
	g.retainedSizeCalculatorRegistry.Register(calc)
}

// GetRetainedSize returns the retained size for an object using the active strategy.
// By default, this uses IDEA-style calculation which includes logically owned objects.
// Use GetStandardRetainedSize for the strict dominator-tree based calculation.
func (g *ReferenceGraph) GetRetainedSize(objectID uint64) int64 {
	if !g.dominatorComputed {
		g.ComputeDominatorTree()
	}
	// Return computed retained size (using active strategy)
	if size, exists := g.computedRetainedSizes[objectID]; exists {
		return size
	}
	// Fallback to standard retained size
	return g.retainedSizes[objectID]
}

// GetStandardRetainedSize returns the strict dominator-tree based retained size.
// This is the traditional retained size calculation used by Eclipse MAT.
func (g *ReferenceGraph) GetStandardRetainedSize(objectID uint64) int64 {
	if !g.dominatorComputed {
		g.ComputeDominatorTree()
	}
	return g.retainedSizes[objectID]
}

// GetClassRetainedSize returns the MAT top-level retained size for a class (may overlap across classes).
// This matches Eclipse MAT "retained if delete all instances of this class" semantics.
func (g *ReferenceGraph) GetClassRetainedSize(className string) int64 {
	if !g.dominatorComputed {
		g.ComputeDominatorTree()
	}

	for classID, name := range g.classNames {
		if name == className {
			return g.classRetainedSizes[classID]
		}
	}
	return 0
}

// GetClassRetainedSizeAttributed returns the non-overlapping attribution size.
// Each object's shallow size is attributed to the nearest dominator of a different class
// (or itself if dominated only by same class / super root). Totals ~= heap size and
// are closer to IDEA 的"Retained by class"。
func (g *ReferenceGraph) GetClassRetainedSizeAttributed(className string) int64 {
	if !g.dominatorComputed {
		g.ComputeDominatorTree()
	}

	for classID, name := range g.classNames {
		if name == className {
			return g.classRetainedSizesAttributed[classID]
		}
	}
	return 0
}
