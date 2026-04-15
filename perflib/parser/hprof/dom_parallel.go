// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"context"
	"sync"
	"sync/atomic"
)

// ============================================================================
// Parallel Dominator Tree Computation Helpers
// ============================================================================

// edgePair represents an edge in the dominator graph.
type edgePair struct {
	fromIdx int32
	toIdx   int32
}

// buildSuccessorsParallel builds the successors list in parallel using the worker pool.
// This is used during dominator tree computation to parallelize edge collection.
func buildSuccessorsParallel(
	g *ReferenceGraph,
	state *dominatorState,
	objIDs []uint64,
	successors [][]int32,
) {
	ctx := context.Background()
	config := DefaultPoolConfig()

	// Use ChunkProcessor for parallel edge collection
	processor := NewChunkProcessor[uint64, []edgePair](config)

	allEdges := processor.ProcessChunks(
		ctx,
		objIDs,
		func(ctx context.Context, chunk []uint64, workerID int) []edgePair {
			// Per-worker seen map to deduplicate
			seen := make(map[int32]bool, 16)
			edges := make([]edgePair, 0, len(chunk)*3) // Assume avg 3 refs per object

			for _, objID := range chunk {
				fromIdx := state.objToIdx[objID]
				// Clear seen map for this object
				for k := range seen {
					delete(seen, k)
				}
				for _, ref := range g.outgoingRefs[objID] {
					if toIdx, ok := state.objToIdx[ref.ToObjectID]; ok {
						if !seen[toIdx] {
							seen[toIdx] = true
							edges = append(edges, edgePair{fromIdx: fromIdx, toIdx: toIdx})
						}
					}
				}
			}
			return edges
		},
		func(results [][]edgePair) []edgePair {
			// Merge all edge slices
			totalLen := 0
			for _, r := range results {
				totalLen += len(r)
			}
			merged := make([]edgePair, 0, totalLen)
			for _, r := range results {
				merged = append(merged, r...)
			}
			return merged
		},
	)

	// Add edges to successors
	for _, edge := range allEdges {
		successors[edge.fromIdx] = append(successors[edge.fromIdx], edge.toIdx)
	}
}

// initRetainedSizesParallel initializes retained sizes to shallow sizes.
// Note: This is intentionally sequential because Go maps are not safe for concurrent writes,
// even when writing to different keys. The map may rehash/resize during writes.
// Since this is a simple assignment operation, sequential execution is fast enough.
func initRetainedSizesParallel(g *ReferenceGraph, objIDs []uint64) {
	for _, objID := range objIDs {
		g.retainedSizes[objID] = g.objectSize[objID]
	}
}

// classRetainedResult holds the result of class retained size computation for a chunk.
type classRetainedResult struct {
	classRetained map[uint64]int64
	classAttrib   map[uint64]int64
}

// computeClassRetainedSizesParallel computes class retained sizes in parallel.
// Returns two maps: MAT-style retained sizes and attribution-style sizes.
func computeClassRetainedSizesParallel(g *ReferenceGraph, objIDs []uint64) (map[uint64]int64, map[uint64]int64) {
	ctx := context.Background()
	config := DefaultPoolConfig()

	processor := NewChunkProcessor[uint64, classRetainedResult](config)

	result := processor.ProcessChunks(
		ctx,
		objIDs,
		func(ctx context.Context, chunk []uint64, workerID int) classRetainedResult {
			localRetained := make(map[uint64]int64)
			localAttrib := make(map[uint64]int64)

			for _, objID := range chunk {
				classID := g.objectClass[objID]
				domID := g.dominators[objID]

				// --- View 1: MAT top-level ---
				isDominatedBySameClass := false
				if domID != superRootID && domID != 0 {
					if domClassID, exists := g.objectClass[domID]; exists && domClassID == classID {
						isDominatedBySameClass = true
					}
				}
				if !isDominatedBySameClass {
					localRetained[classID] += g.retainedSizes[objID]
				}

				// --- View 2: Attribution ---
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
				localAttrib[attribClassID] += g.objectSize[objID]
			}

			return classRetainedResult{
				classRetained: localRetained,
				classAttrib:   localAttrib,
			}
		},
		func(results []classRetainedResult) classRetainedResult {
			// Merge all results
			merged := classRetainedResult{
				classRetained: make(map[uint64]int64),
				classAttrib:   make(map[uint64]int64),
			}
			for _, r := range results {
				for k, v := range r.classRetained {
					merged.classRetained[k] += v
				}
				for k, v := range r.classAttrib {
					merged.classAttrib[k] += v
				}
			}
			return merged
		},
	)

	return result.classRetained, result.classAttrib
}

// ============================================================================
// Parallel Graph Building Utilities
// ============================================================================

// GraphEdge represents an edge in the reference graph.
type GraphEdge struct {
	FromID    uint64
	ToID      uint64
	FieldName string
	ClassID   uint64
}

// BuildIncomingRefsParallel builds incoming references map in parallel.
func BuildIncomingRefsParallel(refs []GraphEdge) map[uint64][]ObjectReference {
	ctx := context.Background()
	config := DefaultPoolConfig()

	return ParallelAggregate(
		ctx,
		refs,
		config,
		func(edge GraphEdge) (uint64, []ObjectReference) {
			return edge.ToID, []ObjectReference{{
				FromObjectID: edge.FromID,
				ToObjectID:   edge.ToID,
				FieldName:    edge.FieldName,
				FromClassID:  edge.ClassID,
			}}
		},
		func(existing, new []ObjectReference) []ObjectReference {
			return append(existing, new...)
		},
	)
}

// ============================================================================
// Parallel Object Processing
// ============================================================================

// ProcessObjectsParallel processes objects in parallel with a custom function.
// This is a convenience wrapper around the worker pool for object-level operations.
func ProcessObjectsParallel[R any](
	g *ReferenceGraph,
	processor func(objID uint64, classID uint64, size int64) R,
	reducer func(results []R) R,
) R {
	ctx := context.Background()
	config := DefaultPoolConfig()

	// Collect object IDs
	objIDs := make([]uint64, 0, len(g.objectClass))
	for objID := range g.objectClass {
		objIDs = append(objIDs, objID)
	}

	chunkProcessor := NewChunkProcessor[uint64, []R](config)

	allResults := chunkProcessor.ProcessChunks(
		ctx,
		objIDs,
		func(ctx context.Context, chunk []uint64, workerID int) []R {
			results := make([]R, 0, len(chunk))
			for _, objID := range chunk {
				classID := g.objectClass[objID]
				size := g.objectSize[objID]
				results = append(results, processor(objID, classID, size))
			}
			return results
		},
		func(chunkResults [][]R) []R {
			totalLen := 0
			for _, r := range chunkResults {
				totalLen += len(r)
			}
			merged := make([]R, 0, totalLen)
			for _, r := range chunkResults {
				merged = append(merged, r...)
			}
			return merged
		},
	)

	return reducer(allResults)
}

// ============================================================================
// Parallel Class Statistics
// ============================================================================

// ClassStatsAccumulator accumulates class statistics.
type ClassStatsAccumulator struct {
	InstanceCount int64
	TotalSize     int64
	RetainedSize  int64
}

// ComputeClassStatsParallel computes class statistics in parallel.
func ComputeClassStatsParallel(g *ReferenceGraph, includeRetained bool) map[uint64]ClassStatsAccumulator {
	ctx := context.Background()
	config := DefaultPoolConfig()

	// Collect object IDs
	objIDs := make([]uint64, 0, len(g.objectClass))
	for objID := range g.objectClass {
		objIDs = append(objIDs, objID)
	}

	return ParallelAggregate(
		ctx,
		objIDs,
		config,
		func(objID uint64) (uint64, ClassStatsAccumulator) {
			classID := g.objectClass[objID]
			acc := ClassStatsAccumulator{
				InstanceCount: 1,
				TotalSize:     g.objectSize[objID],
			}
			if includeRetained {
				acc.RetainedSize = g.retainedSizes[objID]
			}
			return classID, acc
		},
		func(existing, new ClassStatsAccumulator) ClassStatsAccumulator {
			return ClassStatsAccumulator{
				InstanceCount: existing.InstanceCount + new.InstanceCount,
				TotalSize:     existing.TotalSize + new.TotalSize,
				RetainedSize:  existing.RetainedSize + new.RetainedSize,
			}
		},
	)
}

// ============================================================================
// Parallel BFS/DFS Utilities
// ============================================================================

// ParallelBFSResult holds the result of parallel BFS traversal.
type ParallelBFSResult struct {
	Visited   map[uint64]bool
	Distances map[uint64]int
}

// ParallelBFSFromRoots performs BFS from multiple roots in parallel.
// Each root is processed by a separate worker, and results are merged.
func ParallelBFSFromRoots(
	g *ReferenceGraph,
	roots []uint64,
	maxDepth int,
	getNeighbors func(objID uint64) []uint64,
) ParallelBFSResult {
	if len(roots) == 0 {
		return ParallelBFSResult{
			Visited:   make(map[uint64]bool),
			Distances: make(map[uint64]int),
		}
	}

	ctx := context.Background()
	config := DefaultPoolConfig()

	// Process each root in parallel
	pool := NewWorkerPool[uint64, ParallelBFSResult](config)
	results := pool.ExecuteFunc(ctx, roots, func(ctx context.Context, root uint64) (ParallelBFSResult, error) {
		visited := make(map[uint64]bool)
		distances := make(map[uint64]int)

		queue := []uint64{root}
		visited[root] = true
		distances[root] = 0

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			currentDist := distances[current]
			if currentDist >= maxDepth {
				continue
			}

			for _, neighbor := range getNeighbors(current) {
				if !visited[neighbor] {
					visited[neighbor] = true
					distances[neighbor] = currentDist + 1
					queue = append(queue, neighbor)
				}
			}
		}

		return ParallelBFSResult{
			Visited:   visited,
			Distances: distances,
		}, nil
	})

	// Merge results (take minimum distance for each node)
	merged := ParallelBFSResult{
		Visited:   make(map[uint64]bool),
		Distances: make(map[uint64]int),
	}

	for _, r := range results {
		for objID := range r.Result.Visited {
			merged.Visited[objID] = true
			if existingDist, exists := merged.Distances[objID]; !exists || r.Result.Distances[objID] < existingDist {
				merged.Distances[objID] = r.Result.Distances[objID]
			}
		}
	}

	return merged
}

// ============================================================================
// Concurrent Map Utilities
// ============================================================================

// ConcurrentMap is a thread-safe map wrapper.
type ConcurrentMap[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]V
}

// NewConcurrentMap creates a new concurrent map.
func NewConcurrentMap[K comparable, V any]() *ConcurrentMap[K, V] {
	return &ConcurrentMap[K, V]{
		data: make(map[K]V),
	}
}

// NewConcurrentMapWithCapacity creates a new concurrent map with initial capacity.
func NewConcurrentMapWithCapacity[K comparable, V any](capacity int) *ConcurrentMap[K, V] {
	return &ConcurrentMap[K, V]{
		data: make(map[K]V, capacity),
	}
}

// Get retrieves a value from the map.
func (m *ConcurrentMap[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	return v, ok
}

// Set stores a value in the map.
func (m *ConcurrentMap[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
}

// Delete removes a key from the map.
func (m *ConcurrentMap[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
}

// Len returns the number of items in the map.
func (m *ConcurrentMap[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// Range iterates over all items in the map.
func (m *ConcurrentMap[K, V]) Range(fn func(key K, value V) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.data {
		if !fn(k, v) {
			break
		}
	}
}

// ToMap returns a copy of the underlying map.
func (m *ConcurrentMap[K, V]) ToMap() map[K]V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[K]V, len(m.data))
	for k, v := range m.data {
		result[k] = v
	}
	return result
}

// Update atomically updates a value using the provided function.
func (m *ConcurrentMap[K, V]) Update(key K, fn func(existing V, exists bool) V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, exists := m.data[key]
	m.data[key] = fn(existing, exists)
}

// ============================================================================
// Parallel Predecessors Building
// ============================================================================

// buildPredecessorsParallel builds the predecessors list in parallel.
// This is a two-phase algorithm:
// Phase 1: Count predecessors for each node in parallel
// Phase 2: Populate predecessors using CSR format with atomic write positions
func buildPredecessorsParallel(successors [][]int32, totalNodes int) [][]int32 {
	config := DefaultPoolConfig()
	numWorkers := config.MaxWorkers

	// For small graphs, use sequential processing
	if totalNodes < 50000 || numWorkers == 1 {
		return buildPredecessorsSequential(successors, totalNodes)
	}

	// Phase 1: Count predecessors in parallel
	// Each worker counts predecessors for a chunk of successors
	predCounts := make([]int32, totalNodes)

	// Divide work by source nodes
	chunkSize := (totalNodes + numWorkers - 1) / numWorkers

	var wg sync.WaitGroup
	// Use per-worker local counts to avoid atomic operations
	workerCounts := make([][]int32, numWorkers)
	for w := 0; w < numWorkers; w++ {
		workerCounts[w] = make([]int32, totalNodes)
	}

	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > totalNodes {
			end = totalNodes
		}
		if start >= end {
			continue
		}

		wg.Add(1)
		go func(workerID, start, end int) {
			defer wg.Done()
			localCounts := workerCounts[workerID]
			for v := start; v < end; v++ {
				for _, w := range successors[v] {
					localCounts[w]++
				}
			}
		}(w, start, end)
	}
	wg.Wait()

	// Merge worker counts (can be parallelized for very large graphs)
	for w := 0; w < numWorkers; w++ {
		for i := 0; i < totalNodes; i++ {
			predCounts[i] += workerCounts[w][i]
		}
	}

	// Phase 2: Build CSR-style predecessors with atomic write positions
	// First, compute offsets
	offsets := make([]int32, totalNodes+1)
	offsets[0] = 0
	for i := 0; i < totalNodes; i++ {
		offsets[i+1] = offsets[i] + predCounts[i]
	}

	// Allocate flat array for all predecessors
	totalEdges := offsets[totalNodes]
	flatPreds := make([]int32, totalEdges)

	// Use atomic write positions
	writePos := make([]atomic.Int32, totalNodes)
	for i := 0; i < totalNodes; i++ {
		writePos[i].Store(offsets[i])
	}

	// Parallel populate using atomic write positions
	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > totalNodes {
			end = totalNodes
		}
		if start >= end {
			continue
		}

		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for v := start; v < end; v++ {
				for _, target := range successors[v] {
					pos := writePos[target].Add(1) - 1
					flatPreds[pos] = int32(v)
				}
			}
		}(start, end)
	}
	wg.Wait()

	// Convert flat array to slice-of-slices (zero-copy using subslices)
	predecessors := make([][]int32, totalNodes)
	for i := 0; i < totalNodes; i++ {
		start := offsets[i]
		end := offsets[i+1]
		predecessors[i] = flatPreds[start:end]
	}

	return predecessors
}

// buildPredecessorsSequential builds predecessors sequentially for small graphs.
func buildPredecessorsSequential(successors [][]int32, totalNodes int) [][]int32 {
	// Count predecessors
	predCounts := make([]int32, totalNodes)
	for v := 0; v < totalNodes; v++ {
		for _, w := range successors[v] {
			predCounts[w]++
		}
	}

	// Allocate predecessors with exact capacity
	predecessors := make([][]int32, totalNodes)
	for i := range predecessors {
		predecessors[i] = make([]int32, 0, predCounts[i])
	}

	// Populate predecessors
	for v := int32(0); v < int32(totalNodes); v++ {
		for _, w := range successors[v] {
			predecessors[w] = append(predecessors[w], v)
		}
	}

	return predecessors
}
