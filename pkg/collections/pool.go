// Package collections provides generic data structures for efficient data processing.
package collections

import (
	"sync"
)

// ============================================================================
// Generic Slice Pools - Reduce memory allocation overhead
// ============================================================================

// SlicePool is a generic pool for slices of any type.
type SlicePool[T any] struct {
	pool        sync.Pool
	initialCap  int
}

// NewSlicePool creates a new slice pool with the given initial capacity.
func NewSlicePool[T any](initialCap int) *SlicePool[T] {
	if initialCap <= 0 {
		initialCap = 256
	}
	return &SlicePool[T]{
		initialCap: initialCap,
		pool: sync.Pool{
			New: func() interface{} {
				s := make([]T, 0, initialCap)
				return &s
			},
		},
	}
}

// Get gets a slice from the pool.
func (p *SlicePool[T]) Get() *[]T {
	return p.pool.Get().(*[]T)
}

// Put returns a slice to the pool after clearing it.
func (p *SlicePool[T]) Put(s *[]T) {
	*s = (*s)[:0]
	p.pool.Put(s)
}

// ============================================================================
// Pre-defined Slice Pools for Common Types
// ============================================================================

// Int32SlicePool is a pool for []int32 slices.
var Int32SlicePool = NewSlicePool[int32](256)

// GetInt32Slice gets a slice from the pool.
func GetInt32Slice() *[]int32 {
	return Int32SlicePool.Get()
}

// PutInt32Slice returns a slice to the pool after clearing it.
func PutInt32Slice(s *[]int32) {
	Int32SlicePool.Put(s)
}

// Int64SlicePool is a pool for []int64 slices.
var Int64SlicePool = NewSlicePool[int64](256)

// GetInt64Slice gets a slice from the pool.
func GetInt64Slice() *[]int64 {
	return Int64SlicePool.Get()
}

// PutInt64Slice returns a slice to the pool after clearing it.
func PutInt64Slice(s *[]int64) {
	Int64SlicePool.Put(s)
}

// Uint64SlicePool is a pool for []uint64 slices.
var Uint64SlicePool = NewSlicePool[uint64](256)

// GetUint64Slice gets a slice from the pool.
func GetUint64Slice() *[]uint64 {
	return Uint64SlicePool.Get()
}

// PutUint64Slice returns a slice to the pool after clearing it.
func PutUint64Slice(s *[]uint64) {
	Uint64SlicePool.Put(s)
}

// ============================================================================
// Generic Map Pools
// ============================================================================

// MapPool is a generic pool for maps.
type MapPool[K comparable, V any] struct {
	pool       sync.Pool
	initialCap int
}

// NewMapPool creates a new map pool with the given initial capacity.
func NewMapPool[K comparable, V any](initialCap int) *MapPool[K, V] {
	if initialCap <= 0 {
		initialCap = 1024
	}
	return &MapPool[K, V]{
		initialCap: initialCap,
		pool: sync.Pool{
			New: func() interface{} {
				return make(map[K]V, initialCap)
			},
		},
	}
}

// Get gets a map from the pool.
func (p *MapPool[K, V]) Get() map[K]V {
	return p.pool.Get().(map[K]V)
}

// Put returns a map to the pool after clearing it.
func (p *MapPool[K, V]) Put(m map[K]V) {
	// Clear the map
	for k := range m {
		delete(m, k)
	}
	p.pool.Put(m)
}

// ============================================================================
// Stack - Generic LIFO data structure
// ============================================================================

// Stack is a generic LIFO stack.
type Stack[T any] struct {
	data []T
}

// NewStack creates a new stack with the given capacity.
func NewStack[T any](capacity int) *Stack[T] {
	return &Stack[T]{
		data: make([]T, 0, capacity),
	}
}

// Push pushes a value onto the stack.
func (s *Stack[T]) Push(v T) {
	s.data = append(s.data, v)
}

// Pop pops a value from the stack.
func (s *Stack[T]) Pop() (T, bool) {
	if len(s.data) == 0 {
		var zero T
		return zero, false
	}
	v := s.data[len(s.data)-1]
	s.data = s.data[:len(s.data)-1]
	return v, true
}

// Peek returns the top value without removing it.
func (s *Stack[T]) Peek() (T, bool) {
	if len(s.data) == 0 {
		var zero T
		return zero, false
	}
	return s.data[len(s.data)-1], true
}

// IsEmpty returns true if the stack is empty.
func (s *Stack[T]) IsEmpty() bool {
	return len(s.data) == 0
}

// Len returns the number of items in the stack.
func (s *Stack[T]) Len() int {
	return len(s.data)
}

// Clear clears the stack.
func (s *Stack[T]) Clear() {
	s.data = s.data[:0]
}

// ============================================================================
// Queue - Generic FIFO data structure with efficient dequeue
// ============================================================================

// Queue is a generic FIFO queue with efficient dequeue using head pointer.
type Queue[T any] struct {
	data []T
	head int
}

// NewQueue creates a new queue with the given capacity.
func NewQueue[T any](capacity int) *Queue[T] {
	return &Queue[T]{
		data: make([]T, 0, capacity),
		head: 0,
	}
}

// Enqueue adds a value to the queue.
func (q *Queue[T]) Enqueue(v T) {
	q.data = append(q.data, v)
}

// Dequeue removes and returns the first value from the queue.
func (q *Queue[T]) Dequeue() (T, bool) {
	if q.head >= len(q.data) {
		var zero T
		return zero, false
	}
	v := q.data[q.head]
	q.head++
	// Compact if we've consumed more than half
	if q.head > len(q.data)/2 && q.head > 1024 {
		q.compact()
	}
	return v, true
}

// Peek returns the first value without removing it.
func (q *Queue[T]) Peek() (T, bool) {
	if q.head >= len(q.data) {
		var zero T
		return zero, false
	}
	return q.data[q.head], true
}

// IsEmpty returns true if the queue is empty.
func (q *Queue[T]) IsEmpty() bool {
	return q.head >= len(q.data)
}

// Len returns the number of items in the queue.
func (q *Queue[T]) Len() int {
	return len(q.data) - q.head
}

// Clear clears the queue.
func (q *Queue[T]) Clear() {
	q.data = q.data[:0]
	q.head = 0
}

// compact moves remaining elements to the front of the slice.
func (q *Queue[T]) compact() {
	remaining := q.data[q.head:]
	copy(q.data, remaining)
	q.data = q.data[:len(remaining)]
	q.head = 0
}

// ============================================================================
// Ring Buffer - Fixed-size circular buffer
// ============================================================================

// RingBuffer is a fixed-size circular buffer.
type RingBuffer[T any] struct {
	data  []T
	head  int
	tail  int
	count int
	cap   int
}

// NewRingBuffer creates a new ring buffer with the given capacity.
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	return &RingBuffer[T]{
		data: make([]T, capacity),
		cap:  capacity,
	}
}

// Push adds a value to the buffer. Returns false if buffer is full.
func (r *RingBuffer[T]) Push(v T) bool {
	if r.count == r.cap {
		return false
	}
	r.data[r.tail] = v
	r.tail = (r.tail + 1) % r.cap
	r.count++
	return true
}

// Pop removes and returns the oldest value. Returns false if buffer is empty.
func (r *RingBuffer[T]) Pop() (T, bool) {
	if r.count == 0 {
		var zero T
		return zero, false
	}
	v := r.data[r.head]
	r.head = (r.head + 1) % r.cap
	r.count--
	return v, true
}

// Peek returns the oldest value without removing it.
func (r *RingBuffer[T]) Peek() (T, bool) {
	if r.count == 0 {
		var zero T
		return zero, false
	}
	return r.data[r.head], true
}

// IsFull returns true if the buffer is full.
func (r *RingBuffer[T]) IsFull() bool {
	return r.count == r.cap
}

// IsEmpty returns true if the buffer is empty.
func (r *RingBuffer[T]) IsEmpty() bool {
	return r.count == 0
}

// Len returns the number of items in the buffer.
func (r *RingBuffer[T]) Len() int {
	return r.count
}

// Cap returns the capacity of the buffer.
func (r *RingBuffer[T]) Cap() int {
	return r.cap
}

// Clear clears the buffer.
func (r *RingBuffer[T]) Clear() {
	r.head = 0
	r.tail = 0
	r.count = 0
}
