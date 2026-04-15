// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// Bitset is an alias to perflib hprof Bitset.
type Bitset = libhprof.Bitset

// VersionedBitset is an alias to perflib hprof VersionedBitset.
type VersionedBitset = libhprof.VersionedBitset

// AtomicBitset is an alias to perflib hprof AtomicBitset.
type AtomicBitset = libhprof.AtomicBitset

// ============================================================================
// Constructor Forwarding
// ============================================================================

// NewBitset creates a new bitset with the given size.
func NewBitset(size int) *Bitset {
	return libhprof.NewBitset(size)
}

// NewBitsetWithCapacity creates a new bitset with extra capacity for growth.
func NewBitsetWithCapacity(size, capacity int) *Bitset {
	return libhprof.NewBitsetWithCapacity(size, capacity)
}

// NewVersionedBitset creates a new versioned bitset.
func NewVersionedBitset(size int) *VersionedBitset {
	return libhprof.NewVersionedBitset(size)
}

// NewAtomicBitset creates a new atomic bitset.
func NewAtomicBitset(size int) *AtomicBitset {
	return libhprof.NewAtomicBitset(size)
}
