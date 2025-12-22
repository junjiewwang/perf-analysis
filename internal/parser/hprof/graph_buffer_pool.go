// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"sync"

	"github.com/perf-analysis/pkg/collections"
)

// ============================================================================
// Buffer Pools - Reduce memory allocation overhead
// ============================================================================

// Int32SlicePool is a pool for []int32 slices.
// Use this for temporary slices in BFS/DFS queues, path building, etc.
var Int32SlicePool = collections.Int32SlicePool

// GetInt32Slice gets a slice from the pool.
func GetInt32Slice() *[]int32 {
	return collections.GetInt32Slice()
}

// PutInt32Slice returns a slice to the pool after clearing it.
func PutInt32Slice(s *[]int32) {
	collections.PutInt32Slice(s)
}

// Int64SlicePool is a pool for []int64 slices.
var Int64SlicePool = collections.Int64SlicePool

// GetInt64Slice gets a slice from the pool.
func GetInt64Slice() *[]int64 {
	return collections.GetInt64Slice()
}

// PutInt64Slice returns a slice to the pool after clearing it.
func PutInt64Slice(s *[]int64) {
	collections.PutInt64Slice(s)
}

// Uint64SlicePool is a pool for []uint64 slices.
var Uint64SlicePool = collections.Uint64SlicePool

// GetUint64Slice gets a slice from the pool.
func GetUint64Slice() *[]uint64 {
	return collections.GetUint64Slice()
}

// PutUint64Slice returns a slice to the pool after clearing it.
func PutUint64Slice(s *[]uint64) {
	collections.PutUint64Slice(s)
}

// ============================================================================
// BFS/DFS Queue Pool (HPROF-specific)
// ============================================================================

// QueueItem represents an item in a BFS queue with depth tracking.
type QueueItem struct {
	Index int32
	Depth int
}

// QueuePool is a pool for BFS queues.
var QueuePool = &sync.Pool{
	New: func() interface{} {
		q := make([]QueueItem, 0, 1024)
		return &q
	},
}

// GetQueue gets a queue from the pool.
func GetQueue() *[]QueueItem {
	return QueuePool.Get().(*[]QueueItem)
}

// PutQueue returns a queue to the pool after clearing it.
func PutQueue(q *[]QueueItem) {
	*q = (*q)[:0]
	QueuePool.Put(q)
}

// ============================================================================
// Path Building Pool (HPROF-specific)
// ============================================================================

// PathPool is a pool for path slices used in GC root path finding.
var PathPool = &sync.Pool{
	New: func() interface{} {
		p := make([]int32, 0, 32)
		return &p
	},
}

// GetPath gets a path slice from the pool.
func GetPath() *[]int32 {
	return PathPool.Get().(*[]int32)
}

// PutPath returns a path slice to the pool after clearing it.
func PutPath(p *[]int32) {
	*p = (*p)[:0]
	PathPool.Put(p)
}

// ============================================================================
// Map Pools - For temporary maps in parallel processing
// ============================================================================

// Int64MapPool is a pool for map[uint64]int64 maps.
var Int64MapPool = &sync.Pool{
	New: func() interface{} {
		return make(map[uint64]int64, 1024)
	},
}

// GetInt64Map gets a map from the pool.
func GetInt64Map() map[uint64]int64 {
	return Int64MapPool.Get().(map[uint64]int64)
}

// PutInt64Map returns a map to the pool after clearing it.
func PutInt64Map(m map[uint64]int64) {
	// Clear the map
	for k := range m {
		delete(m, k)
	}
	Int64MapPool.Put(m)
}

// BoolMapPool is a pool for map[int32]bool maps (for visited tracking).
var BoolMapPool = &sync.Pool{
	New: func() interface{} {
		return make(map[int32]bool, 1024)
	},
}

// GetBoolMap gets a map from the pool.
func GetBoolMap() map[int32]bool {
	return BoolMapPool.Get().(map[int32]bool)
}

// PutBoolMap returns a map to the pool after clearing it.
func PutBoolMap(m map[int32]bool) {
	// Clear the map
	for k := range m {
		delete(m, k)
	}
	BoolMapPool.Put(m)
}

// Uint64BoolMapPool is a pool for map[uint64]bool maps (for visited tracking with object IDs).
var Uint64BoolMapPool = &sync.Pool{
	New: func() interface{} {
		return make(map[uint64]bool, 1024)
	},
}

// GetUint64BoolMap gets a map from the pool.
func GetUint64BoolMap() map[uint64]bool {
	return Uint64BoolMapPool.Get().(map[uint64]bool)
}

// PutUint64BoolMap returns a map to the pool after clearing it.
func PutUint64BoolMap(m map[uint64]bool) {
	// Clear the map
	for k := range m {
		delete(m, k)
	}
	Uint64BoolMapPool.Put(m)
}

// ============================================================================
// Reusable BFS Context (HPROF-specific)
// ============================================================================

// BFSContext holds reusable state for BFS traversals.
// This avoids allocating new slices/maps for each BFS call.
type BFSContext struct {
	// Queue for BFS traversal
	queue []int32

	// Visited tracking using versioned bitset (O(1) reset)
	visited *collections.VersionedBitset

	// Distances (optional, only allocated if needed)
	distances []int

	// Path tracking (for path finding)
	parents []int32

	// Maximum size this context can handle
	maxSize int
}

// NewBFSContext creates a new BFS context.
func NewBFSContext(maxSize int) *BFSContext {
	return &BFSContext{
		queue:   make([]int32, 0, 1024),
		visited: collections.NewVersionedBitset(maxSize),
		maxSize: maxSize,
	}
}

// Reset resets the context for a new BFS traversal.
func (c *BFSContext) Reset() {
	c.queue = c.queue[:0]
	c.visited.Reset()
}

// EnsureDistances ensures the distances slice is allocated.
func (c *BFSContext) EnsureDistances() {
	if c.distances == nil || len(c.distances) < c.maxSize {
		c.distances = make([]int, c.maxSize)
	}
}

// EnsureParents ensures the parents slice is allocated.
func (c *BFSContext) EnsureParents() {
	if c.parents == nil || len(c.parents) < c.maxSize {
		c.parents = make([]int32, c.maxSize)
		for i := range c.parents {
			c.parents[i] = -1
		}
	} else {
		for i := range c.parents {
			c.parents[i] = -1
		}
	}
}

// Enqueue adds an item to the queue.
func (c *BFSContext) Enqueue(idx int32) {
	c.queue = append(c.queue, idx)
}

// Dequeue removes and returns the first item from the queue.
func (c *BFSContext) Dequeue() (int32, bool) {
	if len(c.queue) == 0 {
		return 0, false
	}
	idx := c.queue[0]
	c.queue = c.queue[1:]
	return idx, true
}

// IsEmpty returns true if the queue is empty.
func (c *BFSContext) IsEmpty() bool {
	return len(c.queue) == 0
}

// MarkVisited marks an index as visited.
func (c *BFSContext) MarkVisited(idx int32) {
	c.visited.Set(int(idx))
}

// IsVisited returns true if an index has been visited.
func (c *BFSContext) IsVisited(idx int32) bool {
	return c.visited.Test(int(idx))
}

// SetDistance sets the distance for an index.
func (c *BFSContext) SetDistance(idx int32, dist int) {
	if c.distances != nil && int(idx) < len(c.distances) {
		c.distances[idx] = dist
	}
}

// GetDistance returns the distance for an index.
func (c *BFSContext) GetDistance(idx int32) int {
	if c.distances != nil && int(idx) < len(c.distances) {
		return c.distances[idx]
	}
	return -1
}

// SetParent sets the parent for an index (for path reconstruction).
func (c *BFSContext) SetParent(idx, parent int32) {
	if c.parents != nil && int(idx) < len(c.parents) {
		c.parents[idx] = parent
	}
}

// GetParent returns the parent for an index.
func (c *BFSContext) GetParent(idx int32) int32 {
	if c.parents != nil && int(idx) < len(c.parents) {
		return c.parents[idx]
	}
	return -1
}

// ReconstructPath reconstructs the path from start to end using parent pointers.
func (c *BFSContext) ReconstructPath(start, end int32) []int32 {
	if c.parents == nil {
		return nil
	}

	// Build path backwards
	path := make([]int32, 0, 32)
	current := end
	for current != -1 && current != start {
		path = append(path, current)
		current = c.parents[current]
	}
	if current == start {
		path = append(path, start)
	}

	// Reverse path
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return path
}

// ============================================================================
// BFS Context Pool
// ============================================================================

// BFSContextPool manages reusable BFS contexts.
type BFSContextPool struct {
	pool    sync.Pool
	maxSize int
}

// NewBFSContextPool creates a new BFS context pool.
func NewBFSContextPool(maxSize int) *BFSContextPool {
	return &BFSContextPool{
		maxSize: maxSize,
		pool: sync.Pool{
			New: func() interface{} {
				return NewBFSContext(maxSize)
			},
		},
	}
}

// Get gets a BFS context from the pool.
func (p *BFSContextPool) Get() *BFSContext {
	ctx := p.pool.Get().(*BFSContext)
	ctx.Reset()
	return ctx
}

// Put returns a BFS context to the pool.
func (p *BFSContextPool) Put(ctx *BFSContext) {
	p.pool.Put(ctx)
}

// ============================================================================
// Compact Int32 Stack (for DFS)
// ============================================================================

// Int32Stack is a simple stack for int32 values.
type Int32Stack = collections.Stack[int32]

// NewInt32Stack creates a new stack with the given capacity.
func NewInt32Stack(capacity int) *Int32Stack {
	return collections.NewStack[int32](capacity)
}

// ============================================================================
// Pre-allocated Children Builder (HPROF-specific)
// ============================================================================

// ChildrenBuilder efficiently builds a children map for dominator tree.
// It uses two passes: first count, then allocate with exact capacity.
type ChildrenBuilder struct {
	counts   []int32
	children [][]int32
	built    bool
}

// NewChildrenBuilder creates a new children builder.
func NewChildrenBuilder(nodeCount int) *ChildrenBuilder {
	return &ChildrenBuilder{
		counts: make([]int32, nodeCount),
	}
}

// CountChild increments the child count for a parent.
func (b *ChildrenBuilder) CountChild(parentIdx int32) {
	if parentIdx >= 0 && int(parentIdx) < len(b.counts) {
		b.counts[parentIdx]++
	}
}

// Build allocates the children slices with exact capacity.
func (b *ChildrenBuilder) Build() {
	if b.built {
		return
	}
	b.children = make([][]int32, len(b.counts))
	for i, count := range b.counts {
		if count > 0 {
			b.children[i] = make([]int32, 0, count)
		}
	}
	b.built = true
}

// AddChild adds a child to a parent.
func (b *ChildrenBuilder) AddChild(parentIdx, childIdx int32) {
	if !b.built {
		b.Build()
	}
	if parentIdx >= 0 && int(parentIdx) < len(b.children) {
		b.children[parentIdx] = append(b.children[parentIdx], childIdx)
	}
}

// GetChildren returns the children for a parent.
func (b *ChildrenBuilder) GetChildren(parentIdx int32) []int32 {
	if parentIdx < 0 || int(parentIdx) >= len(b.children) {
		return nil
	}
	return b.children[parentIdx]
}

// GetChildrenSlice returns the underlying children slice.
func (b *ChildrenBuilder) GetChildrenSlice() [][]int32 {
	if !b.built {
		b.Build()
	}
	return b.children
}

// ============================================================================
// RetainerBFSContext - Optimized BFS context for retainer analysis
// ============================================================================

// RetainerBFSContext holds reusable state for retainer analysis BFS traversals.
// Key optimizations:
// 1. Uses VersionedBitset for O(1) reset instead of O(V) map clearing
// 2. Uses index-based levels to eliminate objectID -> index map lookups during BFS
type RetainerBFSContext struct {
	// Visited tracking using versioned bitset (O(1) reset)
	visited *collections.VersionedBitset

	// countedRetainers tracks which retainer keys have been counted for current target
	// Uses versioned bitset for O(1) reset
	countedRetainers *collections.VersionedBitset

	// Index-based level slices for BFS traversal (pre-allocated)
	// Using int (object index) instead of uint64 (objectID) eliminates map lookups
	currentLevelIdx []int
	nextLevelIdx    []int

	// Legacy: object ID based levels (kept for backward compatibility)
	currentLevel []uint64
	nextLevel    []uint64

	// Maximum object count this context can handle
	maxObjects int

	// Maximum retainer keys this context can handle
	maxRetainerKeys int
}

// NewRetainerBFSContext creates a new retainer BFS context.
// maxObjects: maximum number of objects in the graph
// maxRetainerKeys: estimated maximum number of unique retainer keys (classID + fieldNameID + depth combinations)
func NewRetainerBFSContext(maxObjects, maxRetainerKeys int) *RetainerBFSContext {
	if maxRetainerKeys <= 0 {
		maxRetainerKeys = 100000 // Default estimate
	}
	return &RetainerBFSContext{
		visited:          collections.NewVersionedBitset(maxObjects),
		countedRetainers: collections.NewVersionedBitset(maxRetainerKeys),
		currentLevelIdx:  make([]int, 0, 256),
		nextLevelIdx:     make([]int, 0, 256),
		currentLevel:     make([]uint64, 0, 256),
		nextLevel:        make([]uint64, 0, 256),
		maxObjects:       maxObjects,
		maxRetainerKeys:  maxRetainerKeys,
	}
}

// Reset resets the context for a new target object traversal.
// This is O(1) instead of O(V) for map clearing.
func (c *RetainerBFSContext) Reset() {
	c.visited.Reset()
	c.countedRetainers.Reset()
	c.currentLevelIdx = c.currentLevelIdx[:0]
	c.nextLevelIdx = c.nextLevelIdx[:0]
	c.currentLevel = c.currentLevel[:0]
	c.nextLevel = c.nextLevel[:0]
}

// ResetVisitedOnly resets only the visited tracking (for new sample object).
func (c *RetainerBFSContext) ResetVisitedOnly() {
	c.visited.Reset()
	c.currentLevelIdx = c.currentLevelIdx[:0]
	c.nextLevelIdx = c.nextLevelIdx[:0]
	c.currentLevel = c.currentLevel[:0]
	c.nextLevel = c.nextLevel[:0]
}

// ResetCountedOnly resets only the counted retainers (for new sample object).
func (c *RetainerBFSContext) ResetCountedOnly() {
	c.countedRetainers.Reset()
}

// MarkVisited marks an object index as visited.
func (c *RetainerBFSContext) MarkVisited(idx int) {
	if idx >= 0 {
		c.visited.Set(idx)
	}
}

// IsVisited returns true if an object index has been visited.
func (c *RetainerBFSContext) IsVisited(idx int) bool {
	if idx < 0 {
		return false
	}
	return c.visited.Test(idx)
}

// TestAndMarkVisited atomically tests and marks an index as visited.
// Returns true if the index was already visited, false if it was newly marked.
// This combines IsVisited + MarkVisited into a single operation.
func (c *RetainerBFSContext) TestAndMarkVisited(idx int) bool {
	if idx < 0 {
		return true // Treat invalid index as already visited
	}
	if c.visited.Test(idx) {
		return true // Already visited
	}
	c.visited.Set(idx)
	return false // Newly marked
}

// MarkRetainerCounted marks a retainer key as counted for current target.
// keyIndex should be a unique index for the (classID, fieldNameID, depth) combination.
func (c *RetainerBFSContext) MarkRetainerCounted(keyIndex int) {
	if keyIndex >= 0 && keyIndex < c.maxRetainerKeys {
		c.countedRetainers.Set(keyIndex)
	}
}

// IsRetainerCounted returns true if a retainer key has been counted for current target.
func (c *RetainerBFSContext) IsRetainerCounted(keyIndex int) bool {
	if keyIndex < 0 || keyIndex >= c.maxRetainerKeys {
		return false
	}
	return c.countedRetainers.Test(keyIndex)
}

// ============================================================================
// Index-based level operations (optimized - no map lookups)
// ============================================================================

// AddToCurrentLevelIdx adds an object index to the current BFS level.
func (c *RetainerBFSContext) AddToCurrentLevelIdx(idx int) {
	c.currentLevelIdx = append(c.currentLevelIdx, idx)
}

// AddToNextLevelIdx adds an object index to the next BFS level.
func (c *RetainerBFSContext) AddToNextLevelIdx(idx int) {
	c.nextLevelIdx = append(c.nextLevelIdx, idx)
}

// SwapLevelsIdx swaps current and next index-based levels for the next BFS iteration.
func (c *RetainerBFSContext) SwapLevelsIdx() {
	c.currentLevelIdx, c.nextLevelIdx = c.nextLevelIdx, c.currentLevelIdx
	c.nextLevelIdx = c.nextLevelIdx[:0]
}

// CurrentLevelIdx returns the current BFS level (index-based).
func (c *RetainerBFSContext) CurrentLevelIdx() []int {
	return c.currentLevelIdx
}

// ClearNextLevelIdx clears the next level slice (index-based).
func (c *RetainerBFSContext) ClearNextLevelIdx() {
	c.nextLevelIdx = c.nextLevelIdx[:0]
}

// ============================================================================
// Legacy object ID based level operations (kept for backward compatibility)
// ============================================================================

// AddToCurrentLevel adds an object ID to the current BFS level.
func (c *RetainerBFSContext) AddToCurrentLevel(objID uint64) {
	c.currentLevel = append(c.currentLevel, objID)
}

// AddToNextLevel adds an object ID to the next BFS level.
func (c *RetainerBFSContext) AddToNextLevel(objID uint64) {
	c.nextLevel = append(c.nextLevel, objID)
}

// SwapLevels swaps current and next levels for the next BFS iteration.
func (c *RetainerBFSContext) SwapLevels() {
	c.currentLevel, c.nextLevel = c.nextLevel, c.currentLevel
	c.nextLevel = c.nextLevel[:0]
}

// CurrentLevel returns the current BFS level.
func (c *RetainerBFSContext) CurrentLevel() []uint64 {
	return c.currentLevel
}

// ClearNextLevel clears the next level slice.
func (c *RetainerBFSContext) ClearNextLevel() {
	c.nextLevel = c.nextLevel[:0]
}

// ============================================================================
// RetainerBFSContext Pool
// ============================================================================

// RetainerBFSContextPool manages reusable RetainerBFSContext instances.
type RetainerBFSContextPool struct {
	pool            sync.Pool
	maxObjects      int
	maxRetainerKeys int
}

// NewRetainerBFSContextPool creates a new pool for RetainerBFSContext.
func NewRetainerBFSContextPool(maxObjects, maxRetainerKeys int) *RetainerBFSContextPool {
	return &RetainerBFSContextPool{
		maxObjects:      maxObjects,
		maxRetainerKeys: maxRetainerKeys,
		pool: sync.Pool{
			New: func() interface{} {
				return NewRetainerBFSContext(maxObjects, maxRetainerKeys)
			},
		},
	}
}

// Get gets a RetainerBFSContext from the pool.
func (p *RetainerBFSContextPool) Get() *RetainerBFSContext {
	ctx := p.pool.Get().(*RetainerBFSContext)
	ctx.Reset()
	return ctx
}

// Put returns a RetainerBFSContext to the pool.
func (p *RetainerBFSContextPool) Put(ctx *RetainerBFSContext) {
	p.pool.Put(ctx)
}
