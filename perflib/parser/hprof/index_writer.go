// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"unsafe"

	"github.com/klauspost/compress/zstd"
)

// WriteHeapIndex writes the IndexedReferenceGraph to a compact binary index file.
// The format is designed for fast bulk read/write with minimal per-element overhead.
func WriteHeapIndex(filePath string, graph *IndexedReferenceGraph) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create index file: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriterSize(f, 4*1024*1024) // 4MB buffer

	if err := writeHeapIndexTo(w, graph); err != nil {
		return err
	}

	return w.Flush()
}

// writeHeapIndexTo writes the index to the given writer.
func writeHeapIndexTo(w io.Writer, graph *IndexedReferenceGraph) error {
	objects := graph.GetObjects()
	outgoing := graph.GetOutgoing()
	incoming := graph.GetIncoming()
	gcRoots := graph.GetGCRoots()

	// Build header
	header := IndexFileHeader{
		Magic:       IndexFileMagic,
		Version:     IndexFileVersion,
		ObjectCount: objects.Count(),
		Flags:       FlagHasRetained | FlagHasFieldNames | FlagCompressed | FlagHasBitsets,
		ClassCount:  int32(len(graph.GetClassNames())),
		GCRootCount: int32(len(gcRoots)),
	}

	if outgoing != nil {
		header.EdgeCount = int64(outgoing.TotalEdges())
	}
	if incoming != nil {
		header.Flags |= FlagHasInEdges
		header.InEdgeCount = int64(incoming.TotalEdges())
	}

	// Check if dominator tree is computed
	hasDominator := false
	for i := int32(0); i < objects.Count(); i++ {
		if objects.GetDominator(i) != -1 {
			hasDominator = true
			break
		}
	}
	if hasDominator {
		header.Flags |= FlagHasDominator
	}

	// Write header
	if err := binary.Write(w, indexByteOrder, &header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Section 1: ObjectStore
	if err := writeObjectStoreSection(w, objects); err != nil {
		return fmt.Errorf("write object store: %w", err)
	}

	// Section 2: OutEdges (CSR)
	if err := writeEdgeSection(w, SectionOutEdges, outgoing); err != nil {
		return fmt.Errorf("write out edges: %w", err)
	}

	// Section 3: InEdges (CSR)
	if incoming != nil {
		if err := writeEdgeSection(w, SectionInEdges, incoming); err != nil {
			return fmt.Errorf("write in edges: %w", err)
		}
	}

	// Section 4: DominatorTree
	if hasDominator {
		if err := writeDominatorSection(w, objects); err != nil {
			return fmt.Errorf("write dominator tree: %w", err)
		}
	}

	// Section 5: Bitsets
	if err := writeBitsetsSection(w, graph); err != nil {
		return fmt.Errorf("write bitsets: %w", err)
	}

	// Section 6: Metadata (compressed)
	if err := writeMetadataSection(w, graph); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	return nil
}

// writeObjectStoreSection writes the ObjectStore section.
// Layout: [SectionHeader][objIDs: uint64[N]][classIDs: uint64[N]][shallowSizes: int64[N]][retainedSizes: int64[N]]
func writeObjectStoreSection(w io.Writer, objects *IndexedObjectStore) error {
	n := int(objects.Count())

	// Calculate section size: 4 arrays of N elements
	// objIDs(8*N) + classIDs(8*N) + shallowSizes(8*N) + retainedSizes(8*N)
	dataLen := uint64(n) * 8 * 4

	sh := SectionHeader{Type: SectionObjectStore, DataLength: dataLen}
	if err := binary.Write(w, indexByteOrder, &sh); err != nil {
		return err
	}

	// Write objIDs (bulk)
	if err := writeUint64Slice(w, objects.idxToObj[:n]); err != nil {
		return fmt.Errorf("write objIDs: %w", err)
	}

	// Write classIDs (bulk)
	if err := writeUint64Slice(w, objects.classIDs[:n]); err != nil {
		return fmt.Errorf("write classIDs: %w", err)
	}

	// Write shallowSizes (bulk)
	if err := writeInt64Slice(w, objects.shallowSizes[:n]); err != nil {
		return fmt.Errorf("write shallowSizes: %w", err)
	}

	// Write retainedSizes (bulk)
	if err := writeInt64Slice(w, objects.retainedSizes[:n]); err != nil {
		return fmt.Errorf("write retainedSizes: %w", err)
	}

	return nil
}

// writeEdgeSection writes a CSR edge section.
// Layout: [SectionHeader][nodeCount: int32][edgeCount: int32][offsets: int32[N+1]][targets: int32[E]][fieldIDs: int32[E]][classIDs: uint64[E]]
func writeEdgeSection(w io.Writer, sectionType SectionType, edges *CompactEdgeList) error {
	if edges == nil {
		// Write empty section
		sh := SectionHeader{Type: sectionType, DataLength: 8} // just nodeCount + edgeCount
		if err := binary.Write(w, indexByteOrder, &sh); err != nil {
			return err
		}
		return binary.Write(w, indexByteOrder, [2]int32{0, 0})
	}

	nodeCount := edges.nodeCount
	edgeCount := edges.edgeCount

	// Calculate data length
	// nodeCount(4) + edgeCount(4) + offsets(4*(N+1)) + targets(4*E) + fieldIDs(4*E) + classIDs(8*E)
	dataLen := uint64(8 + 4*int64(nodeCount+1) + 4*int64(edgeCount) + 4*int64(edgeCount) + 8*int64(edgeCount))

	sh := SectionHeader{Type: sectionType, DataLength: dataLen}
	if err := binary.Write(w, indexByteOrder, &sh); err != nil {
		return err
	}

	// Write counts
	if err := binary.Write(w, indexByteOrder, nodeCount); err != nil {
		return err
	}
	if err := binary.Write(w, indexByteOrder, edgeCount); err != nil {
		return err
	}

	// Write offsets
	if err := writeInt32Slice(w, edges.offsets[:nodeCount+1]); err != nil {
		return fmt.Errorf("write offsets: %w", err)
	}

	// Write targets
	if err := writeInt32Slice(w, edges.targets[:edgeCount]); err != nil {
		return fmt.Errorf("write targets: %w", err)
	}

	// Write fieldIDs
	if err := writeInt32Slice(w, edges.fieldIDs[:edgeCount]); err != nil {
		return fmt.Errorf("write fieldIDs: %w", err)
	}

	// Write classIDs
	if err := writeUint64Slice(w, edges.classIDs[:edgeCount]); err != nil {
		return fmt.Errorf("write classIDs: %w", err)
	}

	return nil
}

// writeDominatorSection writes the dominator tree section.
// Layout: [SectionHeader][dominators: int32[N]]
func writeDominatorSection(w io.Writer, objects *IndexedObjectStore) error {
	n := int(objects.Count())
	dataLen := uint64(4 * n)

	sh := SectionHeader{Type: SectionDominatorTree, DataLength: dataLen}
	if err := binary.Write(w, indexByteOrder, &sh); err != nil {
		return err
	}

	return writeInt32Slice(w, objects.dominators[:n])
}

// writeBitsetsSection writes the bitsets section.
// Layout: [SectionHeader][numBitsets: int32]
//
//	For each bitset: [size: int32][wordCount: int32][words: uint64[wordCount]]
func writeBitsetsSection(w io.Writer, graph *IndexedReferenceGraph) error {
	bitsets := []*Bitset{graph.gcRootBits, graph.classObjectBits, graph.reachableBits}

	// Calculate data length
	dataLen := uint64(4) // numBitsets
	for _, bs := range bitsets {
		dataLen += 8 // size + wordCount
		if bs != nil {
			dataLen += uint64(8 * len(bs.Words()))
		}
	}

	sh := SectionHeader{Type: SectionBitsets, DataLength: dataLen}
	if err := binary.Write(w, indexByteOrder, &sh); err != nil {
		return err
	}

	// Number of bitsets
	if err := binary.Write(w, indexByteOrder, int32(len(bitsets))); err != nil {
		return err
	}

	for _, bs := range bitsets {
		if bs == nil {
			// Write zero-sized bitset
			if err := binary.Write(w, indexByteOrder, int32(0)); err != nil {
				return err
			}
			if err := binary.Write(w, indexByteOrder, int32(0)); err != nil {
				return err
			}
			continue
		}
		words := bs.Words()
		if err := binary.Write(w, indexByteOrder, int32(bs.Size())); err != nil {
			return err
		}
		if err := binary.Write(w, indexByteOrder, int32(len(words))); err != nil {
			return err
		}
		if err := writeUint64Slice(w, words); err != nil {
			return err
		}
	}

	return nil
}

// writeMetadataSection writes the compressed metadata section.
// Contains: class names, field names, GC roots, and edge classIDs.
func writeMetadataSection(w io.Writer, graph *IndexedReferenceGraph) error {
	// First, encode all metadata into a buffer, then compress
	outgoing := graph.GetOutgoing()
	gcRoots := graph.GetGCRoots()
	classNames := graph.GetClassNames()

	// Estimate buffer size
	bufSize := 64 * 1024 // start with 64KB
	if len(classNames) > 1000 {
		bufSize = len(classNames) * 64
	}
	buf := make([]byte, 0, bufSize)

	// 1. Write class names: [count: int32][entries: (classID: uint64, nameLen: uint16, name: []byte)...]
	buf = appendInt32(buf, int32(len(classNames)))
	for classID, name := range classNames {
		buf = appendUint64(buf, classID)
		nameBytes := []byte(name)
		if len(nameBytes) > math.MaxUint16 {
			nameBytes = nameBytes[:math.MaxUint16]
		}
		buf = appendUint16(buf, uint16(len(nameBytes)))
		buf = append(buf, nameBytes...)
	}

	// 2. Write field names from outgoing edges: [count: int32][entries: (nameLen: uint16, name: []byte)...]
	if outgoing != nil && len(outgoing.fieldNames) > 0 {
		buf = appendInt32(buf, int32(len(outgoing.fieldNames)))
		for _, name := range outgoing.fieldNames {
			nameBytes := []byte(name)
			if len(nameBytes) > math.MaxUint16 {
				nameBytes = nameBytes[:math.MaxUint16]
			}
			buf = appendUint16(buf, uint16(len(nameBytes)))
			buf = append(buf, nameBytes...)
		}
	} else {
		buf = appendInt32(buf, 0)
	}

	// 3. Write GC roots: [count: int32][entries: (objectID: uint64, type: uint8, threadID: uint64, frameIndex: int32)...]
	buf = appendInt32(buf, int32(len(gcRoots)))
	for _, root := range gcRoots {
		buf = appendUint64(buf, root.ObjectID)
		buf = append(buf, gcRootTypeToUint8(root.Type))
		buf = appendUint64(buf, root.ThreadID)
		buf = appendInt32(buf, int32(root.FrameIndex))
	}

	// Compress with zstd
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return fmt.Errorf("create zstd encoder: %w", err)
	}
	defer encoder.Close()

	compressed := encoder.EncodeAll(buf, nil)

	// Write section header + compressed data
	sh := SectionHeader{Type: SectionMetadata, DataLength: uint64(len(compressed))}
	if err := binary.Write(w, indexByteOrder, &sh); err != nil {
		return err
	}

	_, err = w.Write(compressed)
	return err
}

// ============================================================================
// Bulk write helpers - use unsafe for zero-copy writes of numeric slices
// ============================================================================

// writeUint64Slice writes a uint64 slice as raw bytes.
func writeUint64Slice(w io.Writer, data []uint64) error {
	if len(data) == 0 {
		return nil
	}
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*8)
	_, err := w.Write(bytes)
	return err
}

// writeInt64Slice writes an int64 slice as raw bytes.
func writeInt64Slice(w io.Writer, data []int64) error {
	if len(data) == 0 {
		return nil
	}
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*8)
	_, err := w.Write(bytes)
	return err
}

// writeInt32Slice writes an int32 slice as raw bytes.
func writeInt32Slice(w io.Writer, data []int32) error {
	if len(data) == 0 {
		return nil
	}
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*4)
	_, err := w.Write(bytes)
	return err
}

// ============================================================================
// Encoding helpers
// ============================================================================

func appendInt32(buf []byte, v int32) []byte {
	b := [4]byte{}
	indexByteOrder.PutUint32(b[:], uint32(v))
	return append(buf, b[:]...)
}

func appendInt64(buf []byte, v int64) []byte {
	b := [8]byte{}
	indexByteOrder.PutUint64(b[:], uint64(v))
	return append(buf, b[:]...)
}

func appendUint16(buf []byte, v uint16) []byte {
	b := [2]byte{}
	indexByteOrder.PutUint16(b[:], v)
	return append(buf, b[:]...)
}

func appendUint64(buf []byte, v uint64) []byte {
	b := [8]byte{}
	indexByteOrder.PutUint64(b[:], v)
	return append(buf, b[:]...)
}

// gcRootTypeToUint8 converts GCRootType to a compact uint8 representation.
func gcRootTypeToUint8(t GCRootType) uint8 {
	switch t {
	case GCRootUnknown:
		return 0
	case GCRootJNIGlobal:
		return 1
	case GCRootJNILocal:
		return 2
	case GCRootJavaFrame:
		return 3
	case GCRootNativeStack:
		return 4
	case GCRootStickyClass:
		return 5
	case GCRootThreadBlock:
		return 6
	case GCRootMonitorUsed:
		return 7
	case GCRootThreadObject:
		return 8
	default:
		return 0
	}
}
