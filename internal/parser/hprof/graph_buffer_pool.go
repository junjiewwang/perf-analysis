// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// QueueItem represents an item in the BFS queue.
type QueueItem = libhprof.QueueItem

// BFSContext holds context for BFS traversal.
type BFSContext = libhprof.BFSContext

// BFSContextPool manages a pool of BFS contexts.
type BFSContextPool = libhprof.BFSContextPool

// Int32Stack is a stack of int32 values.
type Int32Stack = libhprof.Int32Stack

// ChildrenBuilder builds children lists for dominator tree.
type ChildrenBuilder = libhprof.ChildrenBuilder

// RetainerBFSContext holds context for retainer BFS traversal.
type RetainerBFSContext = libhprof.RetainerBFSContext

// RetainerBFSContextPool manages a pool of retainer BFS contexts.
type RetainerBFSContextPool = libhprof.RetainerBFSContextPool

// ============================================================================
// Variable Aliases
// ============================================================================

var (
	// Int32SlicePool is a sync.Pool for int32 slices.
	Int32SlicePool = libhprof.Int32SlicePool

	// Int64SlicePool is a sync.Pool for int64 slices.
	Int64SlicePool = libhprof.Int64SlicePool

	// Uint64SlicePool is a sync.Pool for uint64 slices.
	Uint64SlicePool = libhprof.Uint64SlicePool

	// QueuePool is a sync.Pool for QueueItem slices.
	QueuePool = libhprof.QueuePool

	// PathPool is a sync.Pool for int32 path slices.
	PathPool = libhprof.PathPool

	// Int64MapPool is a sync.Pool for map[uint64]int64.
	Int64MapPool = libhprof.Int64MapPool

	// BoolMapPool is a sync.Pool for map[int32]bool.
	BoolMapPool = libhprof.BoolMapPool

	// Uint64BoolMapPool is a sync.Pool for map[uint64]bool.
	Uint64BoolMapPool = libhprof.Uint64BoolMapPool
)

// ============================================================================
// Function Forwarding - Slice Pool Operations
// ============================================================================

// GetInt32Slice gets an int32 slice from the pool.
func GetInt32Slice() *[]int32 {
	return libhprof.GetInt32Slice()
}

// PutInt32Slice returns an int32 slice to the pool.
func PutInt32Slice(s *[]int32) {
	libhprof.PutInt32Slice(s)
}

// GetInt64Slice gets an int64 slice from the pool.
func GetInt64Slice() *[]int64 {
	return libhprof.GetInt64Slice()
}

// PutInt64Slice returns an int64 slice to the pool.
func PutInt64Slice(s *[]int64) {
	libhprof.PutInt64Slice(s)
}

// GetUint64Slice gets a uint64 slice from the pool.
func GetUint64Slice() *[]uint64 {
	return libhprof.GetUint64Slice()
}

// PutUint64Slice returns a uint64 slice to the pool.
func PutUint64Slice(s *[]uint64) {
	libhprof.PutUint64Slice(s)
}

// ============================================================================
// Function Forwarding - Queue/Path/Map Pool Operations
// ============================================================================

// GetQueue gets a QueueItem slice from the pool.
func GetQueue() *[]QueueItem {
	return libhprof.GetQueue()
}

// PutQueue returns a QueueItem slice to the pool.
func PutQueue(q *[]QueueItem) {
	libhprof.PutQueue(q)
}

// GetPath gets an int32 path slice from the pool.
func GetPath() *[]int32 {
	return libhprof.GetPath()
}

// PutPath returns an int32 path slice to the pool.
func PutPath(p *[]int32) {
	libhprof.PutPath(p)
}

// GetInt64Map gets a map[uint64]int64 from the pool.
func GetInt64Map() map[uint64]int64 {
	return libhprof.GetInt64Map()
}

// PutInt64Map returns a map[uint64]int64 to the pool.
func PutInt64Map(m map[uint64]int64) {
	libhprof.PutInt64Map(m)
}

// GetBoolMap gets a map[int32]bool from the pool.
func GetBoolMap() map[int32]bool {
	return libhprof.GetBoolMap()
}

// PutBoolMap returns a map[int32]bool to the pool.
func PutBoolMap(m map[int32]bool) {
	libhprof.PutBoolMap(m)
}

// GetUint64BoolMap gets a map[uint64]bool from the pool.
func GetUint64BoolMap() map[uint64]bool {
	return libhprof.GetUint64BoolMap()
}

// PutUint64BoolMap returns a map[uint64]bool to the pool.
func PutUint64BoolMap(m map[uint64]bool) {
	libhprof.PutUint64BoolMap(m)
}

// ============================================================================
// Function Forwarding - Constructors
// ============================================================================

// NewBFSContext creates a new BFS context.
func NewBFSContext(maxSize int) *BFSContext {
	return libhprof.NewBFSContext(maxSize)
}

// NewBFSContextPool creates a new BFS context pool.
func NewBFSContextPool(maxSize int) *BFSContextPool {
	return libhprof.NewBFSContextPool(maxSize)
}

// NewInt32Stack creates a new int32 stack.
func NewInt32Stack(capacity int) *Int32Stack {
	return libhprof.NewInt32Stack(capacity)
}

// NewChildrenBuilder creates a new children builder.
func NewChildrenBuilder(nodeCount int) *ChildrenBuilder {
	return libhprof.NewChildrenBuilder(nodeCount)
}

// NewRetainerBFSContext creates a new retainer BFS context.
func NewRetainerBFSContext(maxObjects, maxRetainerKeys int) *RetainerBFSContext {
	return libhprof.NewRetainerBFSContext(maxObjects, maxRetainerKeys)
}

// NewRetainerBFSContextPool creates a new retainer BFS context pool.
func NewRetainerBFSContextPool(maxObjects, maxRetainerKeys int) *RetainerBFSContextPool {
	return libhprof.NewRetainerBFSContextPool(maxObjects, maxRetainerKeys)
}

// Note: All methods on BFSContext, BFSContextPool, ChildrenBuilder,
// RetainerBFSContext, and RetainerBFSContextPool are automatically
// available through type aliases.
