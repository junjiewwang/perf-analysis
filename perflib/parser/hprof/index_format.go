// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

// ============================================================================
// heap_index.bin Format Definition
// ============================================================================
//
// Binary format for efficient serialization of IndexedReferenceGraph.
// Designed for fast bulk read/write with minimal per-element overhead.
//
// Layout:
//   [Header: 40 bytes]
//   [Section: ObjectStore]
//   [Section: CSR OutEdges]
//   [Section: CSR InEdges]
//   [Section: DominatorTree]
//   [Section: Bitsets]
//   [Section: Metadata (zstd compressed)]
//
// All multi-byte values are little-endian.

import "encoding/binary"

// IndexFileMagic is the magic bytes identifying a heap index file.
var IndexFileMagic = [4]byte{'H', 'P', 'I', 'X'}

// IndexFileVersion is the current format version.
const IndexFileVersion uint32 = 1

// IndexFileHeader is the header of heap_index.bin (40 bytes).
type IndexFileHeader struct {
	Magic       [4]byte // "HPIX"
	Version     uint32  // format version (1)
	ObjectCount int32   // number of objects
	EdgeCount   int64   // total edges (outgoing)
	Flags       uint32  // feature flags
	ClassCount  int32   // number of unique classes
	GCRootCount int32   // number of GC roots
	InEdgeCount int64   // total incoming edges
}

// IndexFileFlags defines feature flags for the index file.
const (
	// FlagHasDominator indicates the file contains dominator tree data.
	FlagHasDominator uint32 = 1 << 0
	// FlagHasRetained indicates the file contains retained size data.
	FlagHasRetained uint32 = 1 << 1
	// FlagHasInEdges indicates the file contains incoming edge data.
	FlagHasInEdges uint32 = 1 << 2
	// FlagHasFieldNames indicates the file contains field name data.
	FlagHasFieldNames uint32 = 1 << 3
	// FlagCompressed indicates the metadata section uses zstd compression.
	FlagCompressed uint32 = 1 << 4
	// FlagHasBitsets indicates the file contains bitset data (gcRoot, classObject, reachable).
	FlagHasBitsets uint32 = 1 << 5
)

// SectionType identifies a section in the index file.
type SectionType uint32

const (
	// SectionObjectStore contains object IDs, class IDs, shallow sizes, retained sizes.
	SectionObjectStore SectionType = 1
	// SectionOutEdges contains CSR outgoing edges (offsets + targets + fieldIDs).
	SectionOutEdges SectionType = 2
	// SectionInEdges contains CSR incoming edges (offsets + targets).
	SectionInEdges SectionType = 3
	// SectionDominatorTree contains dominator parent indices.
	SectionDominatorTree SectionType = 4
	// SectionBitsets contains GC root, class object, and reachable bitsets.
	SectionBitsets SectionType = 5
	// SectionMetadata contains zstd-compressed metadata (class names, field names, GC roots, objToIdx).
	SectionMetadata SectionType = 6
)

// SectionHeader precedes each section in the file (12 bytes).
type SectionHeader struct {
	Type       SectionType // section type
	DataLength uint64      // length of the section data in bytes (excluding this header)
}

// indexByteOrder is the byte order used for all index file I/O.
var indexByteOrder = binary.LittleEndian
