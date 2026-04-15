// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"github.com/junjiewwang/perf-analysis/perflib/internal/collections"
)

// ============================================================================
// Type Aliases for internal collections usage
// ============================================================================

// Bitset is an alias to collections.Bitset.
type Bitset = collections.Bitset

// VersionedBitset is an alias to collections.VersionedBitset.
type VersionedBitset = collections.VersionedBitset

// AtomicBitset is an alias to collections.AtomicBitset.
type AtomicBitset = collections.AtomicBitset

// ============================================================================
// Constructor Wrappers
// ============================================================================

// NewBitset creates a new bitset with the given size.
func NewBitset(size int) *Bitset {
	return collections.NewBitset(size)
}

// NewBitsetWithCapacity creates a new bitset with extra capacity for growth.
func NewBitsetWithCapacity(size, capacity int) *Bitset {
	return collections.NewBitsetWithCapacity(size, capacity)
}

// NewVersionedBitset creates a new versioned bitset.
func NewVersionedBitset(size int) *VersionedBitset {
	return collections.NewVersionedBitset(size)
}

// NewAtomicBitset creates a new atomic bitset.
func NewAtomicBitset(size int) *AtomicBitset {
	return collections.NewAtomicBitset(size)
}
