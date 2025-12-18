package collections

import (
	"testing"
)

func TestSlicePool(t *testing.T) {
	pool := NewSlicePool[int](256)

	// Get a slice
	s := pool.Get()
	if s == nil {
		t.Fatal("Get returned nil")
	}
	if cap(*s) < 256 {
		t.Errorf("Expected capacity >= 256, got %d", cap(*s))
	}

	// Use the slice
	*s = append(*s, 1, 2, 3)
	if len(*s) != 3 {
		t.Errorf("Expected length 3, got %d", len(*s))
	}

	// Put it back
	pool.Put(s)

	// Get again (should be cleared)
	s2 := pool.Get()
	if len(*s2) != 0 {
		t.Errorf("Expected length 0 after Put, got %d", len(*s2))
	}
}

func TestMapPool(t *testing.T) {
	pool := NewMapPool[string, int](1024)

	// Get a map
	m := pool.Get()
	if m == nil {
		t.Fatal("Get returned nil")
	}

	// Use the map
	m["a"] = 1
	m["b"] = 2
	if len(m) != 2 {
		t.Errorf("Expected length 2, got %d", len(m))
	}

	// Put it back
	pool.Put(m)

	// Get again (should be cleared)
	m2 := pool.Get()
	if len(m2) != 0 {
		t.Errorf("Expected length 0 after Put, got %d", len(m2))
	}
}

func TestStack(t *testing.T) {
	s := NewStack[int](10)

	if !s.IsEmpty() {
		t.Error("New stack should be empty")
	}

	s.Push(1)
	s.Push(2)
	s.Push(3)

	if s.Len() != 3 {
		t.Errorf("Expected length 3, got %d", s.Len())
	}

	// Peek
	v, ok := s.Peek()
	if !ok || v != 3 {
		t.Errorf("Expected Peek to return 3, got %d", v)
	}
	if s.Len() != 3 {
		t.Error("Peek should not modify length")
	}

	// Pop
	v, ok = s.Pop()
	if !ok || v != 3 {
		t.Errorf("Expected Pop to return 3, got %d", v)
	}

	v, ok = s.Pop()
	if !ok || v != 2 {
		t.Errorf("Expected Pop to return 2, got %d", v)
	}

	v, ok = s.Pop()
	if !ok || v != 1 {
		t.Errorf("Expected Pop to return 1, got %d", v)
	}

	// Pop from empty
	_, ok = s.Pop()
	if ok {
		t.Error("Pop from empty stack should return false")
	}

	if !s.IsEmpty() {
		t.Error("Stack should be empty after popping all elements")
	}
}

func TestQueue(t *testing.T) {
	q := NewQueue[int](10)

	if !q.IsEmpty() {
		t.Error("New queue should be empty")
	}

	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)

	if q.Len() != 3 {
		t.Errorf("Expected length 3, got %d", q.Len())
	}

	// Peek
	v, ok := q.Peek()
	if !ok || v != 1 {
		t.Errorf("Expected Peek to return 1, got %d", v)
	}

	// Dequeue (FIFO)
	v, ok = q.Dequeue()
	if !ok || v != 1 {
		t.Errorf("Expected Dequeue to return 1, got %d", v)
	}

	v, ok = q.Dequeue()
	if !ok || v != 2 {
		t.Errorf("Expected Dequeue to return 2, got %d", v)
	}

	v, ok = q.Dequeue()
	if !ok || v != 3 {
		t.Errorf("Expected Dequeue to return 3, got %d", v)
	}

	// Dequeue from empty
	_, ok = q.Dequeue()
	if ok {
		t.Error("Dequeue from empty queue should return false")
	}
}

func TestQueue_Compact(t *testing.T) {
	q := NewQueue[int](10)

	// Add many items
	for i := 0; i < 2000; i++ {
		q.Enqueue(i)
	}

	// Dequeue most of them
	for i := 0; i < 1500; i++ {
		q.Dequeue()
	}

	// Should still work correctly
	if q.Len() != 500 {
		t.Errorf("Expected length 500, got %d", q.Len())
	}

	v, _ := q.Dequeue()
	if v != 1500 {
		t.Errorf("Expected 1500, got %d", v)
	}
}

func TestRingBuffer(t *testing.T) {
	r := NewRingBuffer[int](3)

	if !r.IsEmpty() {
		t.Error("New ring buffer should be empty")
	}

	// Push items
	if !r.Push(1) {
		t.Error("Push should succeed")
	}
	if !r.Push(2) {
		t.Error("Push should succeed")
	}
	if !r.Push(3) {
		t.Error("Push should succeed")
	}

	if !r.IsFull() {
		t.Error("Ring buffer should be full")
	}

	// Push to full buffer should fail
	if r.Push(4) {
		t.Error("Push to full buffer should fail")
	}

	// Pop
	v, ok := r.Pop()
	if !ok || v != 1 {
		t.Errorf("Expected 1, got %d", v)
	}

	// Now we can push again
	if !r.Push(4) {
		t.Error("Push should succeed after Pop")
	}

	// Pop remaining
	v, _ = r.Pop()
	if v != 2 {
		t.Errorf("Expected 2, got %d", v)
	}
	v, _ = r.Pop()
	if v != 3 {
		t.Errorf("Expected 3, got %d", v)
	}
	v, _ = r.Pop()
	if v != 4 {
		t.Errorf("Expected 4, got %d", v)
	}

	if !r.IsEmpty() {
		t.Error("Ring buffer should be empty")
	}
}

func TestRingBuffer_Wrap(t *testing.T) {
	r := NewRingBuffer[int](3)

	// Fill and empty multiple times to test wrap-around
	for round := 0; round < 5; round++ {
		r.Push(1)
		r.Push(2)
		r.Push(3)

		v, _ := r.Pop()
		if v != 1 {
			t.Errorf("Round %d: Expected 1, got %d", round, v)
		}
		v, _ = r.Pop()
		if v != 2 {
			t.Errorf("Round %d: Expected 2, got %d", round, v)
		}
		v, _ = r.Pop()
		if v != 3 {
			t.Errorf("Round %d: Expected 3, got %d", round, v)
		}
	}
}

func BenchmarkStack_PushPop(b *testing.B) {
	s := NewStack[int](1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Push(i)
		s.Pop()
	}
}

func BenchmarkQueue_EnqueueDequeue(b *testing.B) {
	q := NewQueue[int](1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
		q.Dequeue()
	}
}

func BenchmarkRingBuffer_PushPop(b *testing.B) {
	r := NewRingBuffer[int](1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Push(i)
		r.Pop()
	}
}
