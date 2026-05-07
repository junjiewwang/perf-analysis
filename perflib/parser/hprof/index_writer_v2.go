// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"unsafe"

	"github.com/klauspost/compress/zstd"
)

// WriteHeapIndexV2 writes the IndexedReferenceGraph to a v2 heap_index.bin file.
// The v2 format features:
//   - Section Table for random-access to any section
//   - Page-aligned data sections for direct mmap mapping (zero-copy)
//   - Bitset and metadata section lengths recorded for efficient parsing
func WriteHeapIndexV2(filePath string, graph *IndexedReferenceGraph) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	w := &v2Writer{
		file:  f,
		graph: graph,
	}
	return w.write()
}

// v2Writer handles the v2 format writing process.
type v2Writer struct {
	file  *os.File
	graph *IndexedReferenceGraph
}

// write performs the complete v2 file write.
func (w *v2Writer) write() error {
	objects := w.graph.GetObjects()
	outgoing := w.graph.GetOutgoing()
	incoming := w.graph.GetIncoming()
	gcRoots := w.graph.GetGCRoots()
	classNames := w.graph.GetClassNames()

	n := int(objects.Count())
	outEdges := 0
	inEdges := 0
	if outgoing != nil {
		outEdges = int(outgoing.TotalEdges())
	}
	if incoming != nil {
		inEdges = int(incoming.TotalEdges())
	}

	// Determine flags
	flags := uint32(FlagHasRetained | FlagHasFieldNames | FlagCompressed | FlagHasBitsets)
	if incoming != nil {
		flags |= FlagHasInEdges
	}

	hasDominator := false
	for i := int32(0); i < objects.Count(); i++ {
		if objects.GetDominator(i) != -1 {
			hasDominator = true
			break
		}
	}
	if hasDominator {
		flags |= FlagHasDominator
	}

	// === Phase 1: Calculate section sizes and offsets ===
	dataStart := v2DataStartOffset()
	cursor := dataStart

	// Section 1: ObjectStore
	// Layout: [objIDs: uint64[N]][classIDs: uint64[N]][shallowSizes: int64[N]][retainedSizes: int64[N]]
	objStoreOffset := cursor
	objStoreSize := int64(n) * 8 * 4
	cursor += objStoreSize

	// Section 2: OutEdges
	// Layout: [offsets: int32[N+1]][targets: int32[E]][fieldIDs: int32[E]][classIDs: uint64[E]]
	outEdgesOffset := cursor
	outEdgesSize := int64(n+1)*4 + int64(outEdges)*4 + int64(outEdges)*4 + int64(outEdges)*8
	cursor += outEdgesSize

	// Section 3: InEdges (optional)
	var inEdgesOffset int64
	var inEdgesSize int64
	if incoming != nil {
		inEdgesOffset = cursor
		inEdgesSize = int64(n+1)*4 + int64(inEdges)*4 + int64(inEdges)*4 + int64(inEdges)*8
		cursor += inEdgesSize
	}

	// Section 4: DominatorTree (optional)
	var domOffset int64
	var domSize int64
	if hasDominator {
		domOffset = cursor
		domSize = int64(n) * 4
		cursor += domSize
	}

	// Section 5: Bitsets (we'll write the data and record the offset)
	bitsetsOffset := cursor
	// Bitset size calculated below

	// Section 6: Metadata (calculated after encoding)

	// === Phase 2: Build header and section table ===
	numSections := uint32(v2SectionCount)
	header := IndexFileHeaderV2{
		Magic:       IndexFileMagic,
		Version:     IndexFileVersionV2,
		ObjectCount: int32(n),
		EdgeCount:   int64(outEdges),
		InEdgeCount: int64(inEdges),
		Flags:       flags,
		NumSections: numSections,
		ClassCount:  int32(len(classNames)),
		GCRootCount: int32(len(gcRoots)),
	}

	// We'll write in two passes: first write header placeholder + sections data,
	// then seek back and write the final header with section table.
	// Actually, since we know all offsets upfront except bitsets and metadata sizes,
	// let's encode bitsets and metadata first.

	// Encode bitsets
	bitsetsData := w.encodeBitsets()
	bitsetsSize := int64(len(bitsetsData))
	cursor = bitsetsOffset + bitsetsSize

	// Encode metadata
	metadataOffset := cursor
	metadataData, err := w.encodeMetadata()
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}

	// === Phase 3: Build section table ===
	sectionTable := [v2SectionCount]SectionTableEntry{
		{Type: SectionObjectStore, Offset: objStoreOffset},
		{Type: SectionOutEdges, Offset: outEdgesOffset},
		{Type: SectionInEdges, Offset: inEdgesOffset},
		{Type: SectionDominatorTree, Offset: domOffset},
		{Type: SectionBitsets, Offset: bitsetsOffset},
		{Type: SectionMetadata, Offset: metadataOffset},
	}

	// === Phase 4: Write everything sequentially ===

	// Write header (48 bytes)
	if err := w.writeHeader(header); err != nil {
		return err
	}

	// Write section table
	if err := w.writeSectionTable(sectionTable); err != nil {
		return err
	}

	// Write padding to page alignment
	currentPos := int64(v2HeaderSize) + v2SectionTableSize()
	paddingSize := dataStart - currentPos
	if paddingSize > 0 {
		padding := make([]byte, paddingSize)
		if _, err := w.file.Write(padding); err != nil {
			return fmt.Errorf("write padding: %w", err)
		}
	}

	// Section 1: ObjectStore
	if err := w.writeObjectStore(objects, n); err != nil {
		return fmt.Errorf("write object store: %w", err)
	}

	// Section 2: OutEdges
	if err := w.writeEdges(outgoing, n, outEdges); err != nil {
		return fmt.Errorf("write out edges: %w", err)
	}

	// Section 3: InEdges (optional)
	if incoming != nil {
		if err := w.writeEdges(incoming, n, inEdges); err != nil {
			return fmt.Errorf("write in edges: %w", err)
		}
	}

	// Section 4: DominatorTree (optional)
	if hasDominator {
		if err := w.writeDominators(objects, n); err != nil {
			return fmt.Errorf("write dominators: %w", err)
		}
	}

	// Section 5: Bitsets
	if _, err := w.file.Write(bitsetsData); err != nil {
		return fmt.Errorf("write bitsets: %w", err)
	}

	// Section 6: Metadata
	if _, err := w.file.Write(metadataData); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	return w.file.Sync()
}

// writeHeader writes the v2 file header.
func (w *v2Writer) writeHeader(h IndexFileHeaderV2) error {
	buf := make([]byte, v2HeaderSize)
	copy(buf[0:4], h.Magic[:])
	binary.LittleEndian.PutUint32(buf[4:8], h.Version)
	binary.LittleEndian.PutUint32(buf[8:12], uint32(h.ObjectCount))
	binary.LittleEndian.PutUint64(buf[12:20], uint64(h.EdgeCount))
	binary.LittleEndian.PutUint64(buf[20:28], uint64(h.InEdgeCount))
	binary.LittleEndian.PutUint32(buf[28:32], h.Flags)
	binary.LittleEndian.PutUint32(buf[32:36], h.NumSections)
	binary.LittleEndian.PutUint32(buf[36:40], uint32(h.ClassCount))
	binary.LittleEndian.PutUint32(buf[40:44], uint32(h.GCRootCount))
	// bytes 44-47: reserved (zeros)

	_, err := w.file.Write(buf)
	return err
}

// writeSectionTable writes the section table.
func (w *v2Writer) writeSectionTable(table [v2SectionCount]SectionTableEntry) error {
	buf := make([]byte, v2SectionTableEntrySize*v2SectionCount)
	for i, entry := range table {
		off := i * v2SectionTableEntrySize
		binary.LittleEndian.PutUint32(buf[off:off+4], uint32(entry.Type))
		// bytes off+4 : off+8 are reserved (zeros)
		binary.LittleEndian.PutUint64(buf[off+8:off+16], uint64(entry.Offset))
	}
	_, err := w.file.Write(buf)
	return err
}

// writeObjectStore writes the object store arrays.
func (w *v2Writer) writeObjectStore(objects *IndexedObjectStore, n int) error {
	// Write objIDs
	if err := writeSliceRaw(w.file, objects.idxToObj[:n]); err != nil {
		return fmt.Errorf("write objIDs: %w", err)
	}
	// Write classIDs
	if err := writeSliceRaw(w.file, objects.classIDs[:n]); err != nil {
		return fmt.Errorf("write classIDs: %w", err)
	}
	// Write shallowSizes
	if err := writeSliceRaw(w.file, objects.shallowSizes[:n]); err != nil {
		return fmt.Errorf("write shallowSizes: %w", err)
	}
	// Write retainedSizes
	if err := writeSliceRaw(w.file, objects.retainedSizes[:n]); err != nil {
		return fmt.Errorf("write retainedSizes: %w", err)
	}
	return nil
}

// writeEdges writes a CSR edge section.
func (w *v2Writer) writeEdges(edges *CompactEdgeList, n, edgeCount int) error {
	if edges == nil {
		// Write empty offsets
		zeros := make([]int32, n+1)
		return writeSliceRaw(w.file, zeros)
	}

	// Write offsets
	if err := writeSliceRaw(w.file, edges.offsets[:n+1]); err != nil {
		return fmt.Errorf("write offsets: %w", err)
	}
	// Write targets
	if err := writeSliceRaw(w.file, edges.targets[:edgeCount]); err != nil {
		return fmt.Errorf("write targets: %w", err)
	}
	// Write fieldIDs
	if err := writeSliceRaw(w.file, edges.fieldIDs[:edgeCount]); err != nil {
		return fmt.Errorf("write fieldIDs: %w", err)
	}
	// Write classIDs
	if err := writeSliceRaw(w.file, edges.classIDs[:edgeCount]); err != nil {
		return fmt.Errorf("write classIDs: %w", err)
	}
	return nil
}

// writeDominators writes the dominator tree section.
func (w *v2Writer) writeDominators(objects *IndexedObjectStore, n int) error {
	return writeSliceRaw(w.file, objects.dominators[:n])
}

// encodeBitsets encodes bitsets into a byte buffer.
// Format: [dataLen: uint64][numBitsets: int32][per bitset: size(int32) + wordCount(int32) + words(uint64[])]
func (w *v2Writer) encodeBitsets() []byte {
	bitsets := []*Bitset{w.graph.gcRootBits, w.graph.classObjectBits, w.graph.reachableBits}

	// Calculate inner data size
	innerSize := 4 // numBitsets
	for _, bs := range bitsets {
		innerSize += 8 // size + wordCount
		if bs != nil {
			innerSize += 8 * len(bs.Words())
		}
	}

	buf := make([]byte, 8+innerSize) // dataLen prefix + inner data
	binary.LittleEndian.PutUint64(buf[0:8], uint64(innerSize))

	off := 8
	binary.LittleEndian.PutUint32(buf[off:off+4], uint32(len(bitsets)))
	off += 4

	for _, bs := range bitsets {
		if bs == nil {
			binary.LittleEndian.PutUint32(buf[off:off+4], 0)
			off += 4
			binary.LittleEndian.PutUint32(buf[off:off+4], 0)
			off += 4
			continue
		}
		words := bs.Words()
		binary.LittleEndian.PutUint32(buf[off:off+4], uint32(bs.Size()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:off+4], uint32(len(words)))
		off += 4
		for _, word := range words {
			binary.LittleEndian.PutUint64(buf[off:off+8], word)
			off += 8
		}
	}

	return buf
}

// encodeMetadata encodes and compresses metadata.
// Format: [dataLen: uint64][compressed_data...]
// Inner content (same as v1): classNames + fieldNames + gcRoots
func (w *v2Writer) encodeMetadata() ([]byte, error) {
	outgoing := w.graph.GetOutgoing()
	gcRoots := w.graph.GetGCRoots()
	classNames := w.graph.GetClassNames()

	// Encode plaintext metadata (same format as v1)
	bufSize := 64 * 1024
	if len(classNames) > 1000 {
		bufSize = len(classNames) * 64
	}
	plaintext := make([]byte, 0, bufSize)

	// 1. Class names
	plaintext = appendInt32(plaintext, int32(len(classNames)))
	for classID, name := range classNames {
		plaintext = appendUint64(plaintext, classID)
		nameBytes := []byte(name)
		if len(nameBytes) > math.MaxUint16 {
			nameBytes = nameBytes[:math.MaxUint16]
		}
		plaintext = appendUint16(plaintext, uint16(len(nameBytes)))
		plaintext = append(plaintext, nameBytes...)
	}

	// 2. Field names
	if outgoing != nil && len(outgoing.fieldNames) > 0 {
		plaintext = appendInt32(plaintext, int32(len(outgoing.fieldNames)))
		for _, name := range outgoing.fieldNames {
			nameBytes := []byte(name)
			if len(nameBytes) > math.MaxUint16 {
				nameBytes = nameBytes[:math.MaxUint16]
			}
			plaintext = appendUint16(plaintext, uint16(len(nameBytes)))
			plaintext = append(plaintext, nameBytes...)
		}
	} else {
		plaintext = appendInt32(plaintext, 0)
	}

	// 3. GC roots
	plaintext = appendInt32(plaintext, int32(len(gcRoots)))
	for _, root := range gcRoots {
		plaintext = appendUint64(plaintext, root.ObjectID)
		plaintext = append(plaintext, gcRootTypeToUint8(root.Type))
		plaintext = appendUint64(plaintext, root.ThreadID)
		plaintext = appendInt32(plaintext, int32(root.FrameIndex))
	}

	// Compress with zstd
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, fmt.Errorf("create zstd encoder: %w", err)
	}
	defer encoder.Close()

	compressed := encoder.EncodeAll(plaintext, nil)

	// Prefix with dataLen
	result := make([]byte, 8+len(compressed))
	binary.LittleEndian.PutUint64(result[0:8], uint64(len(compressed)))
	copy(result[8:], compressed)

	return result, nil
}

// writeSliceRaw writes any numeric slice as raw bytes using unsafe.
func writeSliceRaw[T any](f *os.File, data []T) error {
	if len(data) == 0 {
		return nil
	}
	var zero T
	elemSize := int(unsafe.Sizeof(zero))
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*elemSize)
	_, err := f.Write(bytes)
	return err
}
