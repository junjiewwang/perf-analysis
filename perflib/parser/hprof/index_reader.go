// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"unsafe"

	"github.com/junjiewwang/perf-analysis/perflib/internal/collections"
	"github.com/klauspost/compress/zstd"
)

// ReadHeapIndex loads a compact heap index file.
// It auto-detects format version:
//   - v1: loads into IndexedReferenceGraph (full memory allocation)
//   - v2: returns MmapHeapIndex (mmap-based, lazy page-fault access)
//
// Both return types implement HeapGraph interface.
func ReadHeapIndex(filePath string) (HeapGraph, error) {
	// Peek at the version to determine format
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open index file: %w", err)
	}

	var magic [4]byte
	var version uint32
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		f.Close()
		return nil, fmt.Errorf("read magic: %w", err)
	}
	if err := binary.Read(f, indexByteOrder, &version); err != nil {
		f.Close()
		return nil, fmt.Errorf("read version: %w", err)
	}
	f.Close()

	if magic != IndexFileMagic {
		return nil, fmt.Errorf("invalid magic: expected HPIX, got %s", string(magic[:]))
	}

	switch version {
	case IndexFileVersionV2:
		return OpenMmapHeapIndex(filePath)
	case IndexFileVersion:
		return readHeapIndexV1(filePath)
	default:
		return nil, fmt.Errorf("unsupported version: %d", version)
	}
}

// readHeapIndexV1 loads a v1 format heap index file into an IndexedReferenceGraph.
func readHeapIndexV1(filePath string) (*IndexedReferenceGraph, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open index file: %w", err)
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 4*1024*1024) // 4MB buffer
	return readHeapIndexFrom(r)
}

// readHeapIndexFrom reads the index from the given reader.
func readHeapIndexFrom(r io.Reader) (*IndexedReferenceGraph, error) {
	// Read header
	var header IndexFileHeader
	if err := binary.Read(r, indexByteOrder, &header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Validate magic
	if header.Magic != IndexFileMagic {
		return nil, fmt.Errorf("invalid magic: expected HPIX, got %s", string(header.Magic[:]))
	}

	// Validate version
	if header.Version != IndexFileVersion {
		return nil, fmt.Errorf("unsupported version: %d (expected %d)", header.Version, IndexFileVersion)
	}

	graph := &IndexedReferenceGraph{
		classNames: make(map[uint64]string, header.ClassCount),
	}

	// Read sections in order
	// Section 1: ObjectStore
	objects, err := readObjectStoreSection(r, header.ObjectCount)
	if err != nil {
		return nil, fmt.Errorf("read object store: %w", err)
	}
	graph.objects = objects

	// Section 2: OutEdges
	outgoing, err := readEdgeSection(r)
	if err != nil {
		return nil, fmt.Errorf("read out edges: %w", err)
	}
	graph.outgoing = outgoing

	// Section 3: InEdges (optional)
	if header.Flags&FlagHasInEdges != 0 {
		incoming, err := readEdgeSection(r)
		if err != nil {
			return nil, fmt.Errorf("read in edges: %w", err)
		}
		graph.incoming = incoming
	}

	// Section 4: DominatorTree (optional)
	if header.Flags&FlagHasDominator != 0 {
		if err := readDominatorSection(r, objects); err != nil {
			return nil, fmt.Errorf("read dominator tree: %w", err)
		}
	}

	// Section 5: Bitsets (optional)
	if header.Flags&FlagHasBitsets != 0 {
		gcRootBits, classObjectBits, reachableBits, err := readBitsetsSection(r)
		if err != nil {
			return nil, fmt.Errorf("read bitsets: %w", err)
		}
		graph.gcRootBits = gcRootBits
		graph.classObjectBits = classObjectBits
		graph.reachableBits = reachableBits
	}

	// Section 6: Metadata (compressed)
	if err := readMetadataSection(r, graph, header.Flags); err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}

	return graph, nil
}

// readObjectStoreSection reads the ObjectStore section.
func readObjectStoreSection(r io.Reader, objectCount int32) (*IndexedObjectStore, error) {
	var sh SectionHeader
	if err := binary.Read(r, indexByteOrder, &sh); err != nil {
		return nil, fmt.Errorf("read section header: %w", err)
	}
	if sh.Type != SectionObjectStore {
		return nil, fmt.Errorf("expected ObjectStore section (type %d), got type %d", SectionObjectStore, sh.Type)
	}

	n := int(objectCount)
	store := &IndexedObjectStore{
		objToIdx:      make(map[uint64]int32, n),
		idxToObj:      make([]uint64, n),
		classIDs:      make([]uint64, n),
		shallowSizes:  make([]int64, n),
		retainedSizes: make([]int64, n),
		dominators:    make([]int32, n),
		count:         objectCount,
		capacity:      objectCount,
		finalized:     true,
	}

	// Initialize dominators to -1
	for i := range store.dominators {
		store.dominators[i] = -1
	}

	// Read objIDs
	if err := readUint64Slice(r, store.idxToObj); err != nil {
		return nil, fmt.Errorf("read objIDs: %w", err)
	}

	// Read classIDs
	if err := readUint64Slice(r, store.classIDs); err != nil {
		return nil, fmt.Errorf("read classIDs: %w", err)
	}

	// Read shallowSizes
	if err := readInt64Slice(r, store.shallowSizes); err != nil {
		return nil, fmt.Errorf("read shallowSizes: %w", err)
	}

	// Read retainedSizes
	if err := readInt64Slice(r, store.retainedSizes); err != nil {
		return nil, fmt.Errorf("read retainedSizes: %w", err)
	}

	// Build objToIdx map
	for i := int32(0); i < objectCount; i++ {
		store.objToIdx[store.idxToObj[i]] = i
	}

	return store, nil
}

// readEdgeSection reads a CSR edge section.
// Layout: [SectionHeader][nodeCount: int32][edgeCount: int32][offsets: int32[N+1]][targets: int32[E]][fieldIDs: int32[E]][classIDs: uint64[E]]
func readEdgeSection(r io.Reader) (*CompactEdgeList, error) {
	var sh SectionHeader
	if err := binary.Read(r, indexByteOrder, &sh); err != nil {
		return nil, fmt.Errorf("read section header: %w", err)
	}

	var nodeCount, edgeCount int32
	if err := binary.Read(r, indexByteOrder, &nodeCount); err != nil {
		return nil, fmt.Errorf("read nodeCount: %w", err)
	}
	if err := binary.Read(r, indexByteOrder, &edgeCount); err != nil {
		return nil, fmt.Errorf("read edgeCount: %w", err)
	}

	if nodeCount == 0 && edgeCount == 0 {
		// Still need to consume the remaining section data
		// offsets has 1 element (nodeCount+1 = 1), others are empty
		remaining := int64(sh.DataLength) - 8 // subtract the 8 bytes already read (nodeCount + edgeCount)
		if remaining > 0 {
			if _, err := io.CopyN(io.Discard, r, remaining); err != nil {
				return nil, fmt.Errorf("skip remaining empty section data: %w", err)
			}
		}
		return &CompactEdgeList{
			offsets:    make([]int32, 1),
			targets:    make([]int32, 0),
			fieldIDs:   make([]int32, 0),
			classIDs:   make([]uint64, 0),
			fieldNames: make([]string, 0),
			fieldToID:  make(map[string]int32),
			nodeCount:  0,
			edgeCount:  0,
		}, nil
	}

	edges := &CompactEdgeList{
		offsets:    make([]int32, nodeCount+1),
		targets:    make([]int32, edgeCount),
		fieldIDs:   make([]int32, edgeCount),
		classIDs:   make([]uint64, edgeCount),
		fieldNames: make([]string, 0),
		fieldToID:  make(map[string]int32),
		nodeCount:  nodeCount,
		edgeCount:  edgeCount,
	}

	// Read offsets
	if err := readInt32Slice(r, edges.offsets); err != nil {
		return nil, fmt.Errorf("read offsets: %w", err)
	}

	// Read targets
	if err := readInt32Slice(r, edges.targets); err != nil {
		return nil, fmt.Errorf("read targets: %w", err)
	}

	// Read fieldIDs
	if err := readInt32Slice(r, edges.fieldIDs); err != nil {
		return nil, fmt.Errorf("read fieldIDs: %w", err)
	}

	// Read classIDs
	if err := readUint64Slice(r, edges.classIDs); err != nil {
		return nil, fmt.Errorf("read classIDs: %w", err)
	}

	return edges, nil
}

// readDominatorSection reads the dominator tree section.
func readDominatorSection(r io.Reader, objects *IndexedObjectStore) error {
	var sh SectionHeader
	if err := binary.Read(r, indexByteOrder, &sh); err != nil {
		return fmt.Errorf("read section header: %w", err)
	}
	if sh.Type != SectionDominatorTree {
		return fmt.Errorf("expected DominatorTree section (type %d), got type %d", SectionDominatorTree, sh.Type)
	}

	n := int(objects.Count())
	dominators := make([]int32, n)
	if err := readInt32Slice(r, dominators); err != nil {
		return fmt.Errorf("read dominators: %w", err)
	}
	objects.dominators = dominators
	return nil
}

// readBitsetsSection reads the bitsets section.
func readBitsetsSection(r io.Reader) (*Bitset, *Bitset, *Bitset, error) {
	var sh SectionHeader
	if err := binary.Read(r, indexByteOrder, &sh); err != nil {
		return nil, nil, nil, fmt.Errorf("read section header: %w", err)
	}
	if sh.Type != SectionBitsets {
		return nil, nil, nil, fmt.Errorf("expected Bitsets section (type %d), got type %d", SectionBitsets, sh.Type)
	}

	var numBitsets int32
	if err := binary.Read(r, indexByteOrder, &numBitsets); err != nil {
		return nil, nil, nil, err
	}

	readOneBitset := func() (*Bitset, error) {
		var size, wordCount int32
		if err := binary.Read(r, indexByteOrder, &size); err != nil {
			return nil, err
		}
		if err := binary.Read(r, indexByteOrder, &wordCount); err != nil {
			return nil, err
		}
		if size == 0 && wordCount == 0 {
			return nil, nil
		}
		words := make([]uint64, wordCount)
		if err := readUint64Slice(r, words); err != nil {
			return nil, err
		}
		return collections.NewBitsetFromWords(words, int(size)), nil
	}

	var gcRootBits, classObjectBits, reachableBits *Bitset
	var err error

	if numBitsets >= 1 {
		gcRootBits, err = readOneBitset()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read gcRootBits: %w", err)
		}
	}
	if numBitsets >= 2 {
		classObjectBits, err = readOneBitset()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read classObjectBits: %w", err)
		}
	}
	if numBitsets >= 3 {
		reachableBits, err = readOneBitset()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read reachableBits: %w", err)
		}
	}

	return gcRootBits, classObjectBits, reachableBits, nil
}

// readMetadataSection reads and decompresses the metadata section.
func readMetadataSection(r io.Reader, graph *IndexedReferenceGraph, flags uint32) error {
	var sh SectionHeader
	if err := binary.Read(r, indexByteOrder, &sh); err != nil {
		return fmt.Errorf("read section header: %w", err)
	}
	if sh.Type != SectionMetadata {
		return fmt.Errorf("expected Metadata section (type %d), got type %d", SectionMetadata, sh.Type)
	}

	// Read compressed data
	compressed := make([]byte, sh.DataLength)
	if _, err := io.ReadFull(r, compressed); err != nil {
		return fmt.Errorf("read compressed metadata: %w", err)
	}

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

	offset := 0

	// 1. Read class names
	if offset+4 > len(data) {
		return fmt.Errorf("metadata too short for class name count")
	}
	classNameCount := int32(indexByteOrder.Uint32(data[offset:]))
	offset += 4

	for i := int32(0); i < classNameCount; i++ {
		if offset+10 > len(data) {
			return fmt.Errorf("metadata too short for class name entry %d", i)
		}
		classID := indexByteOrder.Uint64(data[offset:])
		offset += 8
		nameLen := int(indexByteOrder.Uint16(data[offset:]))
		offset += 2
		if offset+nameLen > len(data) {
			return fmt.Errorf("metadata too short for class name %d (len=%d)", i, nameLen)
		}
		name := string(data[offset : offset+nameLen])
		offset += nameLen
		graph.classNames[classID] = name
	}

	// 2. Read field names
	if offset+4 > len(data) {
		return fmt.Errorf("metadata too short for field name count")
	}
	fieldNameCount := int32(indexByteOrder.Uint32(data[offset:]))
	offset += 4

	if fieldNameCount > 0 && graph.outgoing != nil {
		graph.outgoing.fieldNames = make([]string, fieldNameCount)
		graph.outgoing.fieldToID = make(map[string]int32, fieldNameCount)
		for i := int32(0); i < fieldNameCount; i++ {
			if offset+2 > len(data) {
				return fmt.Errorf("metadata too short for field name entry %d", i)
			}
			nameLen := int(indexByteOrder.Uint16(data[offset:]))
			offset += 2
			if offset+nameLen > len(data) {
				return fmt.Errorf("metadata too short for field name %d (len=%d)", i, nameLen)
			}
			name := string(data[offset : offset+nameLen])
			offset += nameLen
			graph.outgoing.fieldNames[i] = name
			graph.outgoing.fieldToID[name] = i
		}
	}

	// 3. Read GC roots
	if offset+4 > len(data) {
		return fmt.Errorf("metadata too short for GC root count")
	}
	gcRootCount := int32(indexByteOrder.Uint32(data[offset:]))
	offset += 4

	graph.gcRoots = make([]GCRoot, gcRootCount)
	for i := int32(0); i < gcRootCount; i++ {
		if offset+21 > len(data) { // 8+1+8+4=21
			return fmt.Errorf("metadata too short for GC root entry %d", i)
		}
		graph.gcRoots[i].ObjectID = indexByteOrder.Uint64(data[offset:])
		offset += 8
		graph.gcRoots[i].Type = uint8ToGCRootType(data[offset])
		offset++
		graph.gcRoots[i].ThreadID = indexByteOrder.Uint64(data[offset:])
		offset += 8
		graph.gcRoots[i].FrameIndex = int(int32(indexByteOrder.Uint32(data[offset:])))
		offset += 4
	}

	// Mark dominator as computed if present
	if flags&FlagHasDominator != 0 {
		graph.dominatorComputed = true
	}

	return nil
}

// ============================================================================
// Bulk read helpers - use unsafe for zero-copy reads of numeric slices
// ============================================================================

// readUint64Slice reads raw bytes into a uint64 slice.
func readUint64Slice(r io.Reader, data []uint64) error {
	if len(data) == 0 {
		return nil
	}
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*8)
	_, err := io.ReadFull(r, bytes)
	return err
}

// readInt64Slice reads raw bytes into an int64 slice.
func readInt64Slice(r io.Reader, data []int64) error {
	if len(data) == 0 {
		return nil
	}
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*8)
	_, err := io.ReadFull(r, bytes)
	return err
}

// readInt32Slice reads raw bytes into an int32 slice.
func readInt32Slice(r io.Reader, data []int32) error {
	if len(data) == 0 {
		return nil
	}
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*4)
	_, err := io.ReadFull(r, bytes)
	return err
}

// uint8ToGCRootType converts a uint8 back to GCRootType.
func uint8ToGCRootType(v uint8) GCRootType {
	switch v {
	case 1:
		return GCRootJNIGlobal
	case 2:
		return GCRootJNILocal
	case 3:
		return GCRootJavaFrame
	case 4:
		return GCRootNativeStack
	case 5:
		return GCRootStickyClass
	case 6:
		return GCRootThreadBlock
	case 7:
		return GCRootMonitorUsed
	case 8:
		return GCRootThreadObject
	default:
		return GCRootUnknown
	}
}
