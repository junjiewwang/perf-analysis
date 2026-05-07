// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/junjiewwang/perf-analysis/perflib/internal/collections"
	"github.com/klauspost/compress/zstd"
)

// Compile-time interface compliance check.
var _ HeapGraph = (*MmapHeapIndex)(nil)

// MmapHeapIndex implements HeapGraph using memory-mapped access to a v2 heap_index.bin file.
// It keeps only metadata in resident memory (~50-100MB for 16M objects); large data arrays
// (objects, edges, dominators) are accessed via OS page cache through mmap.
type MmapHeapIndex struct {
	file   *os.File
	data   []byte // full mmap region
	header IndexFileHeaderV2

	// Section offsets (from section table)
	sections [v2SectionCount]SectionTableEntry

	// ============================================================
	// Zero-copy views into mmap data (OS page-cache managed)
	// ============================================================

	// ObjectStore arrays
	objIDs       []uint64 // N elements
	classIDs     []uint64 // N elements
	shallowSizes []int64  // N elements
	retainedSizes []int64 // N elements

	// CSR OutEdges
	outOffsets  []int32  // N+1 elements
	outTargets  []int32  // E elements
	outFieldIDs []int32  // E elements
	outClassIDs []uint64 // E elements

	// CSR InEdges
	inOffsets  []int32  // N+1 elements
	inTargets  []int32  // E_in elements
	inFieldIDs []int32  // E_in elements
	inClassIDs []uint64 // E_in elements

	// DominatorTree
	dominators []int32 // N elements

	// ============================================================
	// Resident metadata (always in memory)
	// ============================================================

	// Bitsets
	gcRootBits      *Bitset
	classObjectBits *Bitset
	reachableBits   *Bitset

	// String tables
	classNames map[uint64]string
	fieldNames []string

	// GC roots
	gcRoots []GCRoot

	// Object ID → index mapping
	objToIdx map[uint64]int32

	// Class → objects index (lazy built)
	classToObjects     map[uint64][]int32
	classToObjectsOnce sync.Once

	// Closed flag
	closed bool
}

// OpenMmapHeapIndex opens a v2 heap_index.bin file using memory mapping.
// Only metadata (classNames, fieldNames, gcRoots, bitsets, objToIdx) is loaded into
// resident memory; the bulk data arrays are mapped but not read until accessed.
func OpenMmapHeapIndex(filePath string) (*MmapHeapIndex, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}

	// Get file size
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat file: %w", err)
	}
	fileSize := int(stat.Size())

	// Memory-map the entire file (read-only)
	data, err := syscall.Mmap(int(f.Fd()), 0, fileSize,
		syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("mmap file: %w", err)
	}

	m := &MmapHeapIndex{
		file: f,
		data: data,
	}

	// Parse header
	if err := m.parseHeader(); err != nil {
		m.Close()
		return nil, err
	}

	// Parse section table
	if err := m.parseSectionTable(); err != nil {
		m.Close()
		return nil, err
	}

	// Create zero-copy views for data sections
	if err := m.mapDataSections(); err != nil {
		m.Close()
		return nil, err
	}

	// Load resident metadata (bitsets, classNames, fieldNames, gcRoots, objToIdx)
	if err := m.loadMetadata(); err != nil {
		m.Close()
		return nil, err
	}

	return m, nil
}

// Close releases the memory mapping and closes the file.
func (m *MmapHeapIndex) Close() error {
	if m.closed {
		return nil
	}
	m.closed = true

	// Nil out all slice views to prevent use-after-unmap
	m.objIDs = nil
	m.classIDs = nil
	m.shallowSizes = nil
	m.retainedSizes = nil
	m.outOffsets = nil
	m.outTargets = nil
	m.outFieldIDs = nil
	m.outClassIDs = nil
	m.inOffsets = nil
	m.inTargets = nil
	m.inFieldIDs = nil
	m.inClassIDs = nil
	m.dominators = nil

	var errs []error
	if m.data != nil {
		if err := syscall.Munmap(m.data); err != nil {
			errs = append(errs, fmt.Errorf("munmap: %w", err))
		}
		m.data = nil
	}
	if m.file != nil {
		if err := m.file.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close file: %w", err))
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// ============================================================================
// HeapGraph interface implementation
// ============================================================================

// ObjectCount returns the total number of objects.
func (m *MmapHeapIndex) ObjectCount() int32 {
	return m.header.ObjectCount
}

// GetObjectIndex returns the internal index for an object ID, or -1 if not found.
func (m *MmapHeapIndex) GetObjectIndex(objectID uint64) int32 {
	if idx, ok := m.objToIdx[objectID]; ok {
		return idx
	}
	return -1
}

// GetObjectID returns the object ID for an internal index.
func (m *MmapHeapIndex) GetObjectID(idx int32) uint64 {
	if idx < 0 || idx >= m.header.ObjectCount {
		return 0
	}
	return m.objIDs[idx]
}

// GetClassID returns the class ID for an object index.
func (m *MmapHeapIndex) GetClassID(idx int32) uint64 {
	if idx < 0 || idx >= m.header.ObjectCount {
		return 0
	}
	return m.classIDs[idx]
}

// GetClassName returns the class name for a class ID.
func (m *MmapHeapIndex) GetClassName(classID uint64) string {
	return m.classNames[classID]
}

// GetShallowSize returns the shallow size for an object index.
func (m *MmapHeapIndex) GetShallowSize(idx int32) int64 {
	if idx < 0 || idx >= m.header.ObjectCount {
		return 0
	}
	return m.shallowSizes[idx]
}

// GetRetainedSize returns the retained size for an object index.
func (m *MmapHeapIndex) GetRetainedSize(idx int32) int64 {
	if idx < 0 || idx >= m.header.ObjectCount {
		return 0
	}
	return m.retainedSizes[idx]
}

// GetDominator returns the dominator index for an object index.
func (m *MmapHeapIndex) GetDominator(idx int32) int32 {
	if m.dominators == nil || idx < 0 || idx >= m.header.ObjectCount {
		return -1
	}
	return m.dominators[idx]
}

// IsGCRoot returns true if the object is a GC root.
func (m *MmapHeapIndex) IsGCRoot(idx int32) bool {
	if m.gcRootBits == nil {
		return false
	}
	return m.gcRootBits.Test(int(idx))
}

// IsReachable returns true if the object is reachable from GC roots.
func (m *MmapHeapIndex) IsReachable(idx int32) bool {
	if m.reachableBits == nil {
		return false
	}
	return m.reachableBits.Test(int(idx))
}

// IsClassObject returns true if the object is a Class instance.
func (m *MmapHeapIndex) IsClassObject(idx int32) bool {
	if m.classObjectBits == nil {
		return false
	}
	return m.classObjectBits.Test(int(idx))
}

// GetOutgoingEdges returns outgoing reference edges for an object.
func (m *MmapHeapIndex) GetOutgoingEdges(idx int32) (targets []int32, fieldIDs []int32, classIDs []uint64) {
	if m.outOffsets == nil || idx < 0 || idx >= m.header.ObjectCount {
		return nil, nil, nil
	}
	start := m.outOffsets[idx]
	end := m.outOffsets[idx+1]
	return m.outTargets[start:end], m.outFieldIDs[start:end], m.outClassIDs[start:end]
}

// GetIncomingEdges returns incoming reference edges for an object.
func (m *MmapHeapIndex) GetIncomingEdges(idx int32) (sources []int32, fieldIDs []int32, classIDs []uint64) {
	if m.inOffsets == nil || idx < 0 || idx >= m.header.ObjectCount {
		return nil, nil, nil
	}
	start := m.inOffsets[idx]
	end := m.inOffsets[idx+1]
	return m.inTargets[start:end], m.inFieldIDs[start:end], m.inClassIDs[start:end]
}

// GetObjectsByClass returns all object indices belonging to a given class.
func (m *MmapHeapIndex) GetObjectsByClass(classID uint64) []int32 {
	m.classToObjectsOnce.Do(func() {
		m.classToObjects = make(map[uint64][]int32)
		for i := int32(0); i < m.header.ObjectCount; i++ {
			cid := m.classIDs[i]
			m.classToObjects[cid] = append(m.classToObjects[cid], i)
		}
	})
	return m.classToObjects[classID]
}

// GetFieldName resolves a field name ID to its string representation.
func (m *MmapHeapIndex) GetFieldName(fieldID int32) string {
	if fieldID < 0 || int(fieldID) >= len(m.fieldNames) {
		return ""
	}
	return m.fieldNames[fieldID]
}

// GetGCRoots returns the list of GC root entries.
func (m *MmapHeapIndex) GetGCRoots() []GCRoot {
	return m.gcRoots
}

// ============================================================================
// Internal parsing methods
// ============================================================================

// parseHeader reads and validates the v2 file header.
func (m *MmapHeapIndex) parseHeader() error {
	if len(m.data) < v2HeaderSize {
		return fmt.Errorf("file too small for v2 header: %d bytes", len(m.data))
	}

	// Read header struct (48 bytes)
	m.header.Magic = [4]byte{m.data[0], m.data[1], m.data[2], m.data[3]}
	if m.header.Magic != IndexFileMagic {
		return fmt.Errorf("invalid magic: expected HPIX, got %s", string(m.header.Magic[:]))
	}

	m.header.Version = binary.LittleEndian.Uint32(m.data[4:8])
	if m.header.Version != IndexFileVersionV2 {
		return fmt.Errorf("not a v2 file: version=%d", m.header.Version)
	}

	m.header.ObjectCount = int32(binary.LittleEndian.Uint32(m.data[8:12]))
	m.header.EdgeCount = int64(binary.LittleEndian.Uint64(m.data[12:20]))
	m.header.InEdgeCount = int64(binary.LittleEndian.Uint64(m.data[20:28]))
	m.header.Flags = binary.LittleEndian.Uint32(m.data[28:32])
	m.header.NumSections = binary.LittleEndian.Uint32(m.data[32:36])
	m.header.ClassCount = int32(binary.LittleEndian.Uint32(m.data[36:40]))
	m.header.GCRootCount = int32(binary.LittleEndian.Uint32(m.data[40:44]))
	// bytes 44-47: reserved padding

	return nil
}

// parseSectionTable reads the section table entries.
func (m *MmapHeapIndex) parseSectionTable() error {
	tableStart := int64(v2HeaderSize)
	numSections := int(m.header.NumSections)
	if numSections > v2SectionCount {
		numSections = v2SectionCount
	}

	tableEnd := tableStart + int64(numSections)*v2SectionTableEntrySize
	if int64(len(m.data)) < tableEnd {
		return fmt.Errorf("file too small for section table: need %d, have %d", tableEnd, len(m.data))
	}

	for i := 0; i < numSections; i++ {
		off := tableStart + int64(i)*v2SectionTableEntrySize
		m.sections[i] = SectionTableEntry{
			Type:   SectionType(binary.LittleEndian.Uint32(m.data[off : off+4])),
			Offset: int64(binary.LittleEndian.Uint64(m.data[off+8 : off+16])),
		}
	}

	return nil
}

// mapDataSections creates zero-copy slice views over mmap data.
func (m *MmapHeapIndex) mapDataSections() error {
	n := int(m.header.ObjectCount)
	outEdges := int(m.header.EdgeCount)
	inEdges := int(m.header.InEdgeCount)

	for i := 0; i < int(m.header.NumSections) && i < v2SectionCount; i++ {
		entry := m.sections[i]
		offset := entry.Offset
		if offset <= 0 {
			continue
		}

		switch entry.Type {
		case SectionObjectStore:
			if err := m.mapObjectStore(offset, n); err != nil {
				return fmt.Errorf("map object store: %w", err)
			}

		case SectionOutEdges:
			if err := m.mapEdgeSection(offset, n, outEdges, true); err != nil {
				return fmt.Errorf("map out edges: %w", err)
			}

		case SectionInEdges:
			if m.header.Flags&FlagHasInEdges != 0 {
				if err := m.mapEdgeSection(offset, n, inEdges, false); err != nil {
					return fmt.Errorf("map in edges: %w", err)
				}
			}

		case SectionDominatorTree:
			if m.header.Flags&FlagHasDominator != 0 {
				if err := m.mapDominatorSection(offset, n); err != nil {
					return fmt.Errorf("map dominator: %w", err)
				}
			}

		case SectionBitsets, SectionMetadata:
			// Handled in loadMetadata
		}
	}

	return nil
}

// mapObjectStore creates slice views for the object store section.
// Layout: [objIDs: uint64[N]][classIDs: uint64[N]][shallowSizes: int64[N]][retainedSizes: int64[N]]
func (m *MmapHeapIndex) mapObjectStore(offset int64, n int) error {
	required := offset + int64(n)*8*4
	if int64(len(m.data)) < required {
		return fmt.Errorf("insufficient data for object store: need %d, have %d", required, len(m.data))
	}

	off := int(offset)
	m.objIDs = unsafe.Slice((*uint64)(unsafe.Pointer(&m.data[off])), n)
	off += n * 8
	m.classIDs = unsafe.Slice((*uint64)(unsafe.Pointer(&m.data[off])), n)
	off += n * 8
	m.shallowSizes = unsafe.Slice((*int64)(unsafe.Pointer(&m.data[off])), n)
	off += n * 8
	m.retainedSizes = unsafe.Slice((*int64)(unsafe.Pointer(&m.data[off])), n)

	return nil
}

// mapEdgeSection creates slice views for a CSR edge section.
// Layout: [offsets: int32[N+1]][targets: int32[E]][fieldIDs: int32[E]][classIDs: uint64[E]]
func (m *MmapHeapIndex) mapEdgeSection(offset int64, n, edgeCount int, isOut bool) error {
	// Calculate required size:
	// offsets: (N+1)*4 + targets: E*4 + fieldIDs: E*4 + classIDs: E*8
	required := offset + int64(n+1)*4 + int64(edgeCount)*4 + int64(edgeCount)*4 + int64(edgeCount)*8
	if int64(len(m.data)) < required {
		return fmt.Errorf("insufficient data for edge section: need %d, have %d", required, len(m.data))
	}

	off := int(offset)
	offsets := unsafe.Slice((*int32)(unsafe.Pointer(&m.data[off])), n+1)
	off += (n + 1) * 4
	targets := unsafe.Slice((*int32)(unsafe.Pointer(&m.data[off])), edgeCount)
	off += edgeCount * 4
	fieldIDs := unsafe.Slice((*int32)(unsafe.Pointer(&m.data[off])), edgeCount)
	off += edgeCount * 4
	classIDs := unsafe.Slice((*uint64)(unsafe.Pointer(&m.data[off])), edgeCount)

	if isOut {
		m.outOffsets = offsets
		m.outTargets = targets
		m.outFieldIDs = fieldIDs
		m.outClassIDs = classIDs
	} else {
		m.inOffsets = offsets
		m.inTargets = targets
		m.inFieldIDs = fieldIDs
		m.inClassIDs = classIDs
	}

	return nil
}

// mapDominatorSection creates a slice view for the dominator tree section.
// Layout: [dominators: int32[N]]
func (m *MmapHeapIndex) mapDominatorSection(offset int64, n int) error {
	required := offset + int64(n)*4
	if int64(len(m.data)) < required {
		return fmt.Errorf("insufficient data for dominator: need %d, have %d", required, len(m.data))
	}

	m.dominators = unsafe.Slice((*int32)(unsafe.Pointer(&m.data[offset])), n)
	return nil
}

// loadMetadata loads bitsets and compressed metadata into resident memory.
func (m *MmapHeapIndex) loadMetadata() error {
	for i := 0; i < int(m.header.NumSections) && i < v2SectionCount; i++ {
		entry := m.sections[i]
		if entry.Offset <= 0 {
			continue
		}

		switch entry.Type {
		case SectionBitsets:
			if err := m.loadBitsets(entry.Offset); err != nil {
				return fmt.Errorf("load bitsets: %w", err)
			}

		case SectionMetadata:
			if err := m.loadCompressedMetadata(entry.Offset); err != nil {
				return fmt.Errorf("load metadata: %w", err)
			}
		}
	}

	// Build objToIdx map from objIDs
	if m.objToIdx == nil && m.objIDs != nil {
		m.objToIdx = make(map[uint64]int32, m.header.ObjectCount)
		for i := int32(0); i < m.header.ObjectCount; i++ {
			m.objToIdx[m.objIDs[i]] = i
		}
	}

	return nil
}

// loadBitsets reads the bitsets section.
// Layout: [dataLen: uint64][numBitsets: int32][per bitset: size(int32) + wordCount(int32) + words(uint64[])]
func (m *MmapHeapIndex) loadBitsets(offset int64) error {
	pos := offset

	// Read data length (uint64)
	if int64(len(m.data)) < pos+8 {
		return fmt.Errorf("insufficient data for bitset section length")
	}
	dataLen := int64(binary.LittleEndian.Uint64(m.data[pos : pos+8]))
	pos += 8

	if int64(len(m.data)) < offset+8+dataLen {
		return fmt.Errorf("insufficient data for bitset section: need %d", offset+8+dataLen)
	}

	// Read numBitsets
	numBitsets := int32(binary.LittleEndian.Uint32(m.data[pos : pos+4]))
	pos += 4

	readOneBitset := func() (*Bitset, error) {
		size := int32(binary.LittleEndian.Uint32(m.data[pos : pos+4]))
		pos += 4
		wordCount := int32(binary.LittleEndian.Uint32(m.data[pos : pos+4]))
		pos += 4
		if size == 0 && wordCount == 0 {
			return nil, nil
		}
		words := make([]uint64, wordCount)
		for j := int32(0); j < wordCount; j++ {
			words[j] = binary.LittleEndian.Uint64(m.data[pos : pos+8])
			pos += 8
		}
		return collections.NewBitsetFromWords(words, int(size)), nil
	}

	var err error
	if numBitsets >= 1 {
		m.gcRootBits, err = readOneBitset()
		if err != nil {
			return err
		}
	}
	if numBitsets >= 2 {
		m.classObjectBits, err = readOneBitset()
		if err != nil {
			return err
		}
	}
	if numBitsets >= 3 {
		m.reachableBits, err = readOneBitset()
		if err != nil {
			return err
		}
	}

	return nil
}

// loadCompressedMetadata decompresses and parses the metadata section.
// Layout: [dataLen: uint64][compressed_data...]
// Decompressed content: classNames + fieldNames + gcRoots (same as v1 metadata encoding)
func (m *MmapHeapIndex) loadCompressedMetadata(offset int64) error {
	pos := offset

	// Read data length
	if int64(len(m.data)) < pos+8 {
		return fmt.Errorf("insufficient data for metadata section length")
	}
	dataLen := int64(binary.LittleEndian.Uint64(m.data[pos : pos+8]))
	pos += 8

	if int64(len(m.data)) < pos+dataLen {
		return fmt.Errorf("insufficient data for metadata: need %d", pos+dataLen)
	}

	compressed := m.data[pos : pos+dataLen]

	// Decompress
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return fmt.Errorf("create zstd decoder: %w", err)
	}
	defer decoder.Close()

	data, err := decoder.DecodeAll(compressed, nil)
	if err != nil {
		return fmt.Errorf("decompress metadata: %w", err)
	}

	off := 0

	// 1. Class names
	if off+4 > len(data) {
		return fmt.Errorf("metadata too short for class name count")
	}
	classNameCount := int32(indexByteOrder.Uint32(data[off:]))
	off += 4

	m.classNames = make(map[uint64]string, classNameCount)
	for i := int32(0); i < classNameCount; i++ {
		if off+10 > len(data) {
			return fmt.Errorf("metadata too short for class name entry %d", i)
		}
		classID := indexByteOrder.Uint64(data[off:])
		off += 8
		nameLen := int(indexByteOrder.Uint16(data[off:]))
		off += 2
		if off+nameLen > len(data) {
			return fmt.Errorf("metadata too short for class name %d", i)
		}
		m.classNames[classID] = string(data[off : off+nameLen])
		off += nameLen
	}

	// 2. Field names
	if off+4 > len(data) {
		return fmt.Errorf("metadata too short for field name count")
	}
	fieldNameCount := int32(indexByteOrder.Uint32(data[off:]))
	off += 4

	m.fieldNames = make([]string, fieldNameCount)
	for i := int32(0); i < fieldNameCount; i++ {
		if off+2 > len(data) {
			return fmt.Errorf("metadata too short for field name entry %d", i)
		}
		nameLen := int(indexByteOrder.Uint16(data[off:]))
		off += 2
		if off+nameLen > len(data) {
			return fmt.Errorf("metadata too short for field name %d", i)
		}
		m.fieldNames[i] = string(data[off : off+nameLen])
		off += nameLen
	}

	// 3. GC roots
	if off+4 > len(data) {
		return fmt.Errorf("metadata too short for GC root count")
	}
	gcRootCount := int32(indexByteOrder.Uint32(data[off:]))
	off += 4

	m.gcRoots = make([]GCRoot, gcRootCount)
	for i := int32(0); i < gcRootCount; i++ {
		if off+21 > len(data) {
			return fmt.Errorf("metadata too short for GC root entry %d", i)
		}
		m.gcRoots[i].ObjectID = indexByteOrder.Uint64(data[off:])
		off += 8
		m.gcRoots[i].Type = uint8ToGCRootType(data[off])
		off++
		m.gcRoots[i].ThreadID = indexByteOrder.Uint64(data[off:])
		off += 8
		m.gcRoots[i].FrameIndex = int(int32(indexByteOrder.Uint32(data[off:])))
		off += 4
	}

	return nil
}
