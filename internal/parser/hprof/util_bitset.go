// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"github.com/perf-analysis/pkg/collections"
)

// ============================================================================
// Type Aliases for backward compatibility
// ============================================================================

// Bitset is an alias to collections.Bitset for backward compatibility.
// Deprecated: Use collections.Bitset directly.
type Bitset = collections.Bitset

// VersionedBitset is an alias to collections.VersionedBitset for backward compatibility.
// Deprecated: Use collections.VersionedBitset directly.
type VersionedBitset = collections.VersionedBitset

// AtomicBitset is an alias to collections.AtomicBitset for backward compatibility.
// Deprecated: Use collections.AtomicBitset directly.
type AtomicBitset = collections.AtomicBitset

// ============================================================================
// Constructor Aliases for backward compatibility
// ============================================================================

// NewBitset creates a new bitset with the given size.
// Deprecated: Use collections.NewBitset directly.
func NewBitset(size int) *Bitset {
	return collections.NewBitset(size)
}

// NewBitsetWithCapacity creates a new bitset with extra capacity for growth.
// Deprecated: Use collections.NewBitsetWithCapacity directly.
func NewBitsetWithCapacity(size, capacity int) *Bitset {
	return collections.NewBitsetWithCapacity(size, capacity)
}

// NewVersionedBitset creates a new versioned bitset.
// Deprecated: Use collections.NewVersionedBitset directly.
func NewVersionedBitset(size int) *VersionedBitset {
	return collections.NewVersionedBitset(size)
}

// NewAtomicBitset creates a new atomic bitset.
// Deprecated: Use collections.NewAtomicBitset directly.
func NewAtomicBitset(size int) *AtomicBitset {
	return collections.NewAtomicBitset(size)
}
