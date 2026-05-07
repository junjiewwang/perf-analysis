// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

// ============================================================================
// heap_index.bin v2 Format Definition
// ============================================================================
//
// Key changes from v1:
//   - Section Table after header enables random-access to any section
//   - Data arrays are page-aligned for direct mmap mapping (zero-copy)
//   - Metadata includes objToIdx serialization to avoid O(N) rebuild
//   - Version field = 2 distinguishes from v1
//
// Layout:
//   [File Header: 48 bytes]
//   [Section Table: 16 bytes × NumSections]
//   [Padding to 4096-byte alignment]
//   [Section Data: ObjectStore]
//   [Section Data: CSR OutEdges]
//   [Section Data: CSR InEdges]
//   [Section Data: DominatorTree]
//   [Section Data: Bitsets]
//   [Section Data: Metadata (zstd compressed)]
//
// All multi-byte values are little-endian (native x86/ARM).
// Each section's data starts at its recorded offset (from file start).

// IndexFileVersionV2 is the v2 format version.
const IndexFileVersionV2 uint32 = 2

// PageAlignment is the alignment boundary for mmap-friendly data sections.
const PageAlignment = 4096

// IndexFileHeaderV2 is the header of heap_index.bin v2 (48 bytes).
type IndexFileHeaderV2 struct {
	Magic         [4]byte // "HPIX"
	Version       uint32  // format version (2)
	ObjectCount   int32   // number of objects
	EdgeCount     int64   // total edges (outgoing)
	InEdgeCount   int64   // total incoming edges
	Flags         uint32  // feature flags (same as v1)
	NumSections   uint32  // number of section table entries
	ClassCount    int32   // number of unique classes
	GCRootCount   int32   // number of GC roots
}

// SectionTableEntry records the position and size of a section in the file.
// Each entry is 16 bytes.
type SectionTableEntry struct {
	Type   SectionType // section type identifier
	_      uint32      // reserved (padding for alignment)
	Offset int64       // byte offset from file start
}

// v2SectionCount is the number of sections in v2 format.
const v2SectionCount = 6

// v2HeaderSize is the total size of the v2 file header.
const v2HeaderSize = 48

// v2SectionTableEntrySize is the size of each section table entry.
const v2SectionTableEntrySize = 16

// v2SectionTableSize returns the total size of the section table.
func v2SectionTableSize() int64 {
	return int64(v2SectionCount) * v2SectionTableEntrySize
}

// v2DataStartOffset returns the page-aligned start offset for section data.
// The header + section table are followed by padding to the next page boundary.
func v2DataStartOffset() int64 {
	raw := int64(v2HeaderSize) + v2SectionTableSize()
	return alignToPage(raw)
}

// alignToPage rounds up an offset to the next page boundary.
func alignToPage(offset int64) int64 {
	if offset%PageAlignment == 0 {
		return offset
	}
	return ((offset / PageAlignment) + 1) * PageAlignment
}
