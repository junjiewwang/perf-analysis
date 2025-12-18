package collections

import (
	"sync"
	"testing"
)

func TestBitset_Basic(t *testing.T) {
	b := NewBitset(100)

	// Test Set and Test
	b.Set(0)
	b.Set(50)
	b.Set(99)

	if !b.Test(0) {
		t.Error("Expected bit 0 to be set")
	}
	if !b.Test(50) {
		t.Error("Expected bit 50 to be set")
	}
	if !b.Test(99) {
		t.Error("Expected bit 99 to be set")
	}
	if b.Test(1) {
		t.Error("Expected bit 1 to be clear")
	}

	// Test Count
	if b.Count() != 3 {
		t.Errorf("Expected count 3, got %d", b.Count())
	}

	// Test Clear
	b.Clear(50)
	if b.Test(50) {
		t.Error("Expected bit 50 to be clear after Clear")
	}
	if b.Count() != 2 {
		t.Errorf("Expected count 2 after Clear, got %d", b.Count())
	}
}

func TestBitset_Grow(t *testing.T) {
	b := NewBitset(64)

	// Set bit beyond initial size
	b.Set(200)
	if !b.Test(200) {
		t.Error("Expected bit 200 to be set after grow")
	}
	if b.Size() < 200 {
		t.Errorf("Expected size >= 200, got %d", b.Size())
	}
}

func TestBitset_SetAll_ClearAll(t *testing.T) {
	b := NewBitset(100)

	b.SetAll()
	for i := 0; i < 100; i++ {
		if !b.Test(i) {
			t.Errorf("Expected bit %d to be set after SetAll", i)
		}
	}

	b.ClearAll()
	for i := 0; i < 100; i++ {
		if b.Test(i) {
			t.Errorf("Expected bit %d to be clear after ClearAll", i)
		}
	}
}

func TestBitset_Or(t *testing.T) {
	a := NewBitset(100)
	b := NewBitset(100)

	a.Set(0)
	a.Set(50)
	b.Set(50)
	b.Set(99)

	a.Or(b)

	if !a.Test(0) || !a.Test(50) || !a.Test(99) {
		t.Error("Or operation failed")
	}
	if a.Count() != 3 {
		t.Errorf("Expected count 3 after Or, got %d", a.Count())
	}
}

func TestBitset_And(t *testing.T) {
	a := NewBitset(100)
	b := NewBitset(100)

	a.Set(0)
	a.Set(50)
	b.Set(50)
	b.Set(99)

	a.And(b)

	if a.Test(0) || !a.Test(50) || a.Test(99) {
		t.Error("And operation failed")
	}
	if a.Count() != 1 {
		t.Errorf("Expected count 1 after And, got %d", a.Count())
	}
}

func TestBitset_AndNot(t *testing.T) {
	a := NewBitset(100)
	b := NewBitset(100)

	a.Set(0)
	a.Set(50)
	b.Set(50)
	b.Set(99)

	a.AndNot(b)

	if !a.Test(0) || a.Test(50) || a.Test(99) {
		t.Error("AndNot operation failed")
	}
	if a.Count() != 1 {
		t.Errorf("Expected count 1 after AndNot, got %d", a.Count())
	}
}

func TestBitset_Iterate(t *testing.T) {
	b := NewBitset(100)
	b.Set(5)
	b.Set(10)
	b.Set(50)

	var indices []int
	b.Iterate(func(i int) bool {
		indices = append(indices, i)
		return true
	})

	if len(indices) != 3 {
		t.Errorf("Expected 3 indices, got %d", len(indices))
	}
	if indices[0] != 5 || indices[1] != 10 || indices[2] != 50 {
		t.Errorf("Unexpected indices: %v", indices)
	}
}

func TestBitset_Clone(t *testing.T) {
	a := NewBitset(100)
	a.Set(10)
	a.Set(20)

	b := a.Clone()

	// Modify original
	a.Set(30)

	// Clone should be independent
	if b.Test(30) {
		t.Error("Clone should be independent")
	}
	if !b.Test(10) || !b.Test(20) {
		t.Error("Clone should have original bits")
	}
}

func TestVersionedBitset_Basic(t *testing.T) {
	v := NewVersionedBitset(100)

	v.Set(10)
	v.Set(50)

	if !v.Test(10) || !v.Test(50) {
		t.Error("Expected bits to be set")
	}

	// Reset should clear logically
	v.Reset()

	if v.Test(10) || v.Test(50) {
		t.Error("Expected bits to be clear after Reset")
	}

	// Can set again
	v.Set(10)
	if !v.Test(10) {
		t.Error("Expected bit 10 to be set after Reset")
	}
}

func TestVersionedBitset_Grow(t *testing.T) {
	v := NewVersionedBitset(64)

	v.Set(200)
	if !v.Test(200) {
		t.Error("Expected bit 200 to be set after grow")
	}
}

func TestAtomicBitset_Concurrent(t *testing.T) {
	b := NewAtomicBitset(1000)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				b.Set(base*100 + j)
			}
		}(i)
	}
	wg.Wait()

	// All bits should be set
	for i := 0; i < 1000; i++ {
		if !b.Test(i) {
			t.Errorf("Expected bit %d to be set", i)
		}
	}
}

func TestAtomicBitset_TestAndSet(t *testing.T) {
	b := NewAtomicBitset(100)

	// First TestAndSet should return false (was not set)
	if b.TestAndSet(10) {
		t.Error("Expected TestAndSet to return false for unset bit")
	}

	// Second TestAndSet should return true (was set)
	if !b.TestAndSet(10) {
		t.Error("Expected TestAndSet to return true for set bit")
	}
}

func BenchmarkBitset_Set(b *testing.B) {
	bs := NewBitset(1000000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bs.Set(i % 1000000)
	}
}

func BenchmarkBitset_Test(b *testing.B) {
	bs := NewBitset(1000000)
	for i := 0; i < 1000000; i++ {
		if i%2 == 0 {
			bs.Set(i)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bs.Test(i % 1000000)
	}
}

func BenchmarkVersionedBitset_Reset(b *testing.B) {
	v := NewVersionedBitset(1000000)
	for i := 0; i < 1000; i++ {
		v.Set(i * 1000)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Reset()
	}
}
