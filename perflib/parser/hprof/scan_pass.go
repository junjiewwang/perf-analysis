// Package hprof provides parsing functionality for Java HPROF heap dump files.
// This file implements Pass 1 of the Two-Pass CSR parsing strategy:
// scan the HPROF file to collect metadata and edge degree counts
// without building the full reference graph.
package hprof

import (
	"context"
	"fmt"
	"io"

	timerPkg "github.com/junjiewwang/perf-analysis/perflib/internal/timer"
)

// ScanResult holds the output of Pass 1 (scan pass).
// It contains all metadata needed to pre-allocate CSR arrays in Pass 2.
type ScanResult struct {
	Header       *Header
	ObjectCount  int32
	EdgeCount    int64
	ObjectIndex  *IndexedObjectStore // objectID → compact int32 index
	DegreeCounts []int32             // outgoing edge count per object index
	ClassInfo    map[uint64]*ClassInfo
	ClassFields  map[uint64][]FieldDescriptor
	ClassNames   map[uint64]uint64  // classID → nameStringID
	Strings      map[uint64]string  // stringID → string value
	GCRoots      []GCRootEntry
	HeapSummary  *HeapSummary

	// SegmentOffsets records the file offset and length of each HEAP_DUMP/HEAP_DUMP_SEGMENT record body.
	// Used by parallel Build Pass to independently parse different segments.
	SegmentOffsets []SegmentInfo

	// Statistics for debugging
	TotalInstances int64
	TotalHeapSize  int64
	TotalClasses   int

	// Internal: for class name resolution
	classByName    map[string]*ClassInfo
	javaLangClassID uint64
}

// SegmentInfo records the file position and length of a heap dump segment body.
type SegmentInfo struct {
	Offset int64  // file offset of the segment body (after record header)
	Length uint32 // length of the segment body in bytes
}

// GCRootEntry records a GC root discovered in Pass 1.
type GCRootEntry struct {
	ObjectID uint64
	RootType GCRootType
	ThreadID uint64
}

// scanState holds temporary state during the scan pass.
type scanState struct {
	reader          *Reader
	header          *Header
	strings         map[uint64]string
	classNames      map[uint64]uint64
	classInfo       map[uint64]*ClassInfo
	classByName     map[string]*ClassInfo
	classFields     map[uint64][]FieldDescriptor
	objectStore     *IndexedObjectStore
	gcRoots         []GCRootEntry
	heapSummary     *HeapSummary
	totalHeapSize   int64
	totalInstances  int64
	edgeCount       int64
	sizeMode        SizeCalculationMode
	javaLangClassID uint64
	segmentOffsets  []SegmentInfo

	// Deferred instances (CLASS_DUMP not yet seen)
	deferredInstances []deferredScanInstance

	// Debug counters
	loadClassCount   int64
	classDumpCount   int64
	instanceDumpCount int64
	arrayDumpCount   int64
}

// deferredScanInstance holds instance data for deferred reference counting.
type deferredScanInstance struct {
	objectID uint64
	classID  uint64
	dataSize int
}

// ScanPass performs Pass 1: quickly scan the HPROF file to collect metadata
// and count edge degrees for each object. This enables pre-allocation of CSR arrays.
func (p *Parser) ScanPass(ctx context.Context, r io.ReadSeeker) (*ScanResult, error) {
	timer := timerPkg.New("HPROF Scan Pass",
		timerPkg.WithLogger(p.opts.Logger),
		timerPkg.WithEnabled(p.opts.Logger != nil))

	reader := NewReader(r)
	state := &scanState{
		reader:      reader,
		strings:     make(map[uint64]string, 10000),
		classNames:  make(map[uint64]uint64, 5000),
		classInfo:   make(map[uint64]*ClassInfo, 5000),
		classByName: make(map[string]*ClassInfo, 5000),
		classFields: make(map[uint64][]FieldDescriptor, 5000),
		objectStore: NewIndexedObjectStore(1000000), // start with 1M estimate
		sizeMode:    p.opts.SizeMode,
	}

	// Read header
	header, err := reader.ReadHeader()
	if err != nil {
		return nil, fmt.Errorf("scan pass: failed to read header: %w", err)
	}
	state.header = header

	// Phase 1: Parse all records (scan mode - only count, don't build references)
	pt := timer.Start("Scan records")
	if err := p.scanRecords(ctx, state); err != nil {
		return nil, fmt.Errorf("scan pass: failed to scan records: %w", err)
	}
	pt.Stop()

	// Process deferred instances: count their degrees
	timer.TimeFunc("Count deferred instance degrees", func() {
		p.countDeferredDegrees(state)
	})

	// Finalize object store
	state.objectStore.Finalize()

	// Build degree counts array
	objectCount := state.objectStore.Count()
	degreeCounts := make([]int32, objectCount)

	// The degree was temporarily stored via AddRetainedSize hack during scan
	// Actually, let's store it properly in a separate array during scan
	// We already have the data in objectStore - just need to extract

	// Build result
	result := &ScanResult{
		Header:         state.header,
		ObjectCount:    objectCount,
		EdgeCount:      state.edgeCount,
		ObjectIndex:    state.objectStore,
		DegreeCounts:   degreeCounts,
		ClassInfo:      state.classInfo,
		ClassFields:    state.classFields,
		ClassNames:     state.classNames,
		Strings:        state.strings,
		GCRoots:        state.gcRoots,
		HeapSummary:    state.heapSummary,
		SegmentOffsets: state.segmentOffsets,
		TotalInstances: state.totalInstances,
		TotalHeapSize:  state.totalHeapSize,
		TotalClasses:   len(state.classByName),
		classByName:    state.classByName,
		javaLangClassID: state.javaLangClassID,
	}

	timer.PrintSummary()

	if p.opts.Logger != nil {
		p.opts.Logger.Info("Scan pass complete: %d objects, %d edges, %d GC roots",
			objectCount, state.edgeCount, len(state.gcRoots))
	}

	return result, nil
}

// scanRecords scans all records in the HPROF file without building references.
func (p *Parser) scanRecords(ctx context.Context, state *scanState) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		tag, _, length, err := state.reader.ReadRecordHeader()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		switch tag {
		case TagString:
			if err := p.scanStringRecord(state, length); err != nil {
				return err
			}
		case TagLoadClass:
			if err := p.scanLoadClass(state); err != nil {
				return err
			}
		case TagHeapDump, TagHeapDumpSegment:
			if err := p.scanHeapDump(ctx, state, length); err != nil {
				return err
			}
		case TagHeapDumpEnd:
			// End of heap dump segments
		default:
			// Skip unknown tags
			if err := state.reader.Skip(int64(length)); err != nil {
				return err
			}
		}
	}
}

// scanStringRecord reads a STRING record.
func (p *Parser) scanStringRecord(state *scanState, length uint32) error {
	idSize := state.reader.IDSize()
	id, err := state.reader.ReadID()
	if err != nil {
		return err
	}

	strLen := int(length) - idSize
	if strLen <= 0 {
		state.strings[id] = ""
		return nil
	}

	strBytes, err := state.reader.ReadBytes(strLen)
	if err != nil {
		return err
	}
	state.strings[id] = string(strBytes)
	return nil
}

// scanLoadClass reads a LOAD_CLASS record.
func (p *Parser) scanLoadClass(state *scanState) error {
	state.loadClassCount++
	idSize := state.reader.IDSize()

	// Serial number
	if _, err := state.reader.ReadUint32(); err != nil {
		return err
	}
	// Class object ID
	classObjID, err := state.reader.ReadID()
	if err != nil {
		return err
	}
	// Stack trace serial number
	if _, err := state.reader.ReadUint32(); err != nil {
		return err
	}
	// Class name string ID
	nameID, err := state.reader.ReadID()
	if err != nil {
		return err
	}
	_ = idSize

	state.classNames[classObjID] = nameID

	// Track java.lang.Class for categorization
	if name, ok := state.strings[nameID]; ok {
		normalizedName := normalizeClassName(name)
		if normalizedName == "java.lang.Class" {
			state.javaLangClassID = classObjID
		}
	}

	return nil
}

// segmentChunkSize defines the target size for splitting large segments into virtual chunks.
// Large segments (>64MB) are split into chunks of approximately this size for parallel processing.
const segmentChunkSize = 64 * 1024 * 1024 // 64 MB

// scanHeapDump scans a HEAP_DUMP or HEAP_DUMP_SEGMENT record.
// It also records sub-record boundaries at regular intervals for parallel Build Pass.
func (p *Parser) scanHeapDump(ctx context.Context, state *scanState, length uint32) error {
	endPos := int64(length)
	var bytesRead int64

	// Track the file offset at the start of this segment body
	segBodyOffset := state.reader.Position() - endPos // reader.Position() already advanced past the body
	// Actually, Position tracks bytes consumed; at this point we just entered the segment body.
	// The segment body starts at the position BEFORE we start reading sub-records.
	segBodyOffset = state.reader.Position()

	// For large segments, record chunk boundaries
	lastChunkEnd := int64(0)
	chunkStartOffset := segBodyOffset

	for bytesRead < endPos {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if we've crossed a chunk boundary
		if bytesRead-lastChunkEnd >= segmentChunkSize && bytesRead > 0 {
			// Record the chunk: from lastChunkEnd to bytesRead within this segment
			chunkLen := bytesRead - lastChunkEnd
			state.segmentOffsets = append(state.segmentOffsets, SegmentInfo{
				Offset: chunkStartOffset,
				Length: uint32(chunkLen),
			})
			lastChunkEnd = bytesRead
			chunkStartOffset = state.reader.Position()
		}

		subTagByte, err := state.reader.ReadByte()
		if err != nil {
			return err
		}
		bytesRead++

		subTag := HeapDumpTag(subTagByte)
		n, err := p.scanHeapDumpSubRecord(state, subTag)
		if err != nil {
			return err
		}
		bytesRead += n
	}

	// Record the final chunk (remaining bytes)
	if bytesRead > lastChunkEnd {
		chunkLen := bytesRead - lastChunkEnd
		state.segmentOffsets = append(state.segmentOffsets, SegmentInfo{
			Offset: chunkStartOffset,
			Length: uint32(chunkLen),
		})
	}

	return nil
}

// scanHeapDumpSubRecord dispatches scanning of a heap dump sub-record.
// Returns the number of bytes consumed (excluding the tag byte).
func (p *Parser) scanHeapDumpSubRecord(state *scanState, tag HeapDumpTag) (int64, error) {
	idSize := state.reader.IDSize()

	switch tag {
	case 0x00:
		// Padding byte
		return 0, nil

	// Android/OpenJDK specific root types (single ID)
	case 0x89, 0x8A, 0x8B, 0x8C, 0x8D: // ROOT_INTERNED_STRING, ROOT_FINALIZING, ROOT_DEBUGGER, ROOT_REFERENCE_CLEANUP, ROOT_VM_INTERNAL
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if objectID != 0 {
			state.gcRoots = append(state.gcRoots, GCRootEntry{
				ObjectID: objectID,
				RootType: GCRootUnknown,
			})
		}
		return int64(idSize), nil

	case 0x8E: // ROOT_JNI_MONITOR
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if err := state.reader.Skip(8); err != nil {
			return 0, err
		}
		if objectID != 0 {
			state.gcRoots = append(state.gcRoots, GCRootEntry{
				ObjectID: objectID,
				RootType: GCRootMonitorUsed,
			})
		}
		return int64(idSize + 8), nil

	case 0xC3: // HEAP_DUMP_INFO (Android specific)
		if _, err := state.reader.ReadUint32(); err != nil {
			return 0, err
		}
		if _, err := state.reader.ReadID(); err != nil {
			return 0, err
		}
		return int64(4 + idSize), nil

	case 0xFE: // ROOT_UNREACHABLE
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if objectID != 0 {
			state.gcRoots = append(state.gcRoots, GCRootEntry{
				ObjectID: objectID,
				RootType: GCRootUnknown,
			})
		}
		return int64(idSize), nil

	case HeapTagRootUnknown:
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if objectID != 0 {
			state.gcRoots = append(state.gcRoots, GCRootEntry{
				ObjectID: objectID,
				RootType: GCRootUnknown,
			})
		}
		return int64(idSize), nil

	case HeapTagRootJNIGlobal:
		return p.scanGCRootJNIGlobal(state)

	case HeapTagRootJNILocal:
		return p.scanGCRootJNILocal(state)

	case HeapTagRootJavaFrame:
		return p.scanGCRootJavaFrame(state)

	case HeapTagRootNativeStack:
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if _, err := state.reader.ReadUint32(); err != nil {
			return 0, err
		}
		if objectID != 0 {
			state.gcRoots = append(state.gcRoots, GCRootEntry{
				ObjectID: objectID,
				RootType: GCRootNativeStack,
			})
		}
		return int64(idSize + 4), nil

	case HeapTagRootThreadBlock:
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if _, err := state.reader.ReadUint32(); err != nil {
			return 0, err
		}
		if objectID != 0 {
			state.gcRoots = append(state.gcRoots, GCRootEntry{
				ObjectID: objectID,
				RootType: GCRootThreadBlock,
			})
		}
		return int64(idSize + 4), nil

	case HeapTagRootStickyClass:
		return p.scanGCRootStickyClass(state)

	case HeapTagRootMonitorUsed:
		return p.scanGCRootMonitorUsed(state)

	case HeapTagRootThreadObject:
		return p.scanGCRootThreadObj(state)

	case HeapTagClassDump:
		return p.scanClassDump(state)

	case HeapTagInstanceDump:
		return p.scanInstanceDump(state)

	case HeapTagObjectArrayDump:
		return p.scanObjectArrayDump(state)

	case HeapTagPrimitiveArrayDump:
		return p.scanPrimitiveArrayDump(state)

	default:
		return 0, fmt.Errorf("scan pass: unknown heap dump sub-tag 0x%02x", tag)
	}
}

// scanGCRootJNIGlobal scans a GC_ROOT_JNI_GLOBAL record.
func (p *Parser) scanGCRootJNIGlobal(state *scanState) (int64, error) {
	idSize := state.reader.IDSize()
	objectID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	// Skip JNI global ref
	if err := state.reader.Skip(int64(idSize)); err != nil {
		return 0, err
	}
	if objectID != 0 {
		state.gcRoots = append(state.gcRoots, GCRootEntry{
			ObjectID: objectID,
			RootType: GCRootJNIGlobal,
		})
	}
	return int64(idSize * 2), nil
}

// scanGCRootJNILocal scans a GC_ROOT_JNI_LOCAL record.
func (p *Parser) scanGCRootJNILocal(state *scanState) (int64, error) {
	idSize := state.reader.IDSize()
	objectID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	threadSerial, err := state.reader.ReadUint32()
	if err != nil {
		return 0, err
	}
	// Skip frame number
	if _, err := state.reader.ReadUint32(); err != nil {
		return 0, err
	}
	if objectID != 0 {
		state.gcRoots = append(state.gcRoots, GCRootEntry{
			ObjectID: objectID,
			RootType: GCRootJNILocal,
			ThreadID: uint64(threadSerial),
		})
	}
	return int64(idSize + 8), nil
}

// scanGCRootJavaFrame scans a GC_ROOT_JAVA_FRAME record.
func (p *Parser) scanGCRootJavaFrame(state *scanState) (int64, error) {
	idSize := state.reader.IDSize()
	objectID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	threadSerial, err := state.reader.ReadUint32()
	if err != nil {
		return 0, err
	}
	// Skip frame index
	if _, err := state.reader.ReadUint32(); err != nil {
		return 0, err
	}
	if objectID != 0 {
		state.gcRoots = append(state.gcRoots, GCRootEntry{
			ObjectID: objectID,
			RootType: GCRootJavaFrame,
			ThreadID: uint64(threadSerial),
		})
	}
	return int64(idSize + 8), nil
}

// scanGCRootStickyClass scans a GC_ROOT_STICKY_CLASS record.
func (p *Parser) scanGCRootStickyClass(state *scanState) (int64, error) {
	idSize := state.reader.IDSize()
	objectID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	if objectID != 0 {
		state.gcRoots = append(state.gcRoots, GCRootEntry{
			ObjectID: objectID,
			RootType: GCRootStickyClass,
		})
	}
	return int64(idSize), nil
}

// scanGCRootThreadObj scans a GC_ROOT_THREAD_OBJ record.
func (p *Parser) scanGCRootThreadObj(state *scanState) (int64, error) {
	idSize := state.reader.IDSize()
	objectID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	threadSerial, err := state.reader.ReadUint32()
	if err != nil {
		return 0, err
	}
	// Skip stack trace serial
	if _, err := state.reader.ReadUint32(); err != nil {
		return 0, err
	}
	if objectID != 0 {
		state.gcRoots = append(state.gcRoots, GCRootEntry{
			ObjectID: objectID,
			RootType: GCRootThreadObject,
			ThreadID: uint64(threadSerial),
		})
	}
	return int64(idSize + 8), nil
}

// scanGCRootMonitorUsed scans a GC_ROOT_MONITOR_USED record.
func (p *Parser) scanGCRootMonitorUsed(state *scanState) (int64, error) {
	idSize := state.reader.IDSize()
	objectID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	if objectID != 0 {
		state.gcRoots = append(state.gcRoots, GCRootEntry{
			ObjectID: objectID,
			RootType: GCRootMonitorUsed,
		})
	}
	return int64(idSize), nil
}

// scanClassDump scans a CLASS_DUMP heap record to extract class metadata.
func (p *Parser) scanClassDump(state *scanState) (int64, error) {
	state.classDumpCount++
	idSize := state.reader.IDSize()
	var bytesRead int64

	classObjID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Stack trace serial
	if _, err := state.reader.ReadUint32(); err != nil {
		return 0, err
	}
	bytesRead += 4

	// Super class ID
	superClassID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Class loader, signers, protection domain, reserved1, reserved2
	for i := 0; i < 5; i++ {
		if _, err := state.reader.ReadID(); err != nil {
			return 0, err
		}
	}
	bytesRead += int64(idSize * 5)

	// Instance size
	instanceSize, err := state.reader.ReadUint32()
	if err != nil {
		return 0, err
	}
	bytesRead += 4

	// Constant pool
	cpCount, err := state.reader.ReadUint16()
	if err != nil {
		return 0, err
	}
	bytesRead += 2
	for i := 0; i < int(cpCount); i++ {
		// Pool index
		if _, err := state.reader.ReadUint16(); err != nil {
			return 0, err
		}
		bytesRead += 2
		ty, err := state.reader.ReadByte()
		if err != nil {
			return 0, err
		}
		bytesRead++
		size := BasicTypeSize(BasicType(ty), idSize)
		if err := state.reader.Skip(int64(size)); err != nil {
			return 0, err
		}
		bytesRead += int64(size)
	}

	// Static fields
	sfCount, err := state.reader.ReadUint16()
	if err != nil {
		return 0, err
	}
	bytesRead += 2
	for i := 0; i < int(sfCount); i++ {
		// Field name ID
		if _, err := state.reader.ReadID(); err != nil {
			return 0, err
		}
		bytesRead += int64(idSize)
		ty, err := state.reader.ReadByte()
		if err != nil {
			return 0, err
		}
		bytesRead++
		size := BasicTypeSize(BasicType(ty), idSize)
		// Static field value - check for object references
		if BasicType(ty) == TypeObject {
			refID, err := state.reader.ReadID()
			if err != nil {
				return 0, err
			}
			bytesRead += int64(size)
			// Count static reference as an edge from classObjID to refID
			if refID != 0 {
				objIdx := state.objectStore.GetIndex(classObjID)
				if objIdx >= 0 {
					state.edgeCount++
					// Increment degree for the source object
					state.objectStore.AddRetainedSize(objIdx, 1) // temporarily using retained as degree counter
				}
			}
		} else {
			if err := state.reader.Skip(int64(size)); err != nil {
				return 0, err
			}
			bytesRead += int64(size)
		}
	}

	// Instance fields (record descriptors for Pass 2)
	ifCount, err := state.reader.ReadUint16()
	if err != nil {
		return 0, err
	}
	bytesRead += 2
	fields := make([]FieldDescriptor, 0, ifCount)
	for i := 0; i < int(ifCount); i++ {
		nameID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		bytesRead += int64(idSize)
		ty, err := state.reader.ReadByte()
		if err != nil {
			return 0, err
		}
		bytesRead++
		fields = append(fields, FieldDescriptor{
			NameID: nameID,
			Type:   BasicType(ty),
		})
	}
	state.classFields[classObjID] = fields

	// Build class info
	className := ""
	if nameStringID, ok := state.classNames[classObjID]; ok {
		if name, ok := state.strings[nameStringID]; ok {
			className = normalizeClassName(name)
		}
	}

	info := &ClassInfo{
		ClassID:      classObjID,
		SuperClassID: superClassID,
		Name:         className,
		InstanceSize: int(instanceSize),
	}
	state.classInfo[classObjID] = info
	if className != "" {
		state.classByName[className] = info
	}

	// Register class object itself
	classShallowSize := alignTo8(objectHeaderSize(state.sizeMode) + int64(instanceSize))
	state.objectStore.AddObject(classObjID, state.javaLangClassID, classShallowSize)

	return bytesRead, nil
}

// scanInstanceDump scans an INSTANCE_DUMP record: registers object and counts references.
func (p *Parser) scanInstanceDump(state *scanState) (int64, error) {
	state.instanceDumpCount++
	idSize := state.reader.IDSize()
	var bytesRead int64

	objectID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Stack trace serial
	if _, err := state.reader.ReadUint32(); err != nil {
		return 0, err
	}
	bytesRead += 4

	classID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	dataSize, err := state.reader.ReadUint32()
	if err != nil {
		return 0, err
	}
	bytesRead += 4

	// Calculate shallow size
	var shallowSize int64
	if info, ok := state.classInfo[classID]; ok {
		shallowSize = alignTo8(objectHeaderSize(state.sizeMode) + int64(info.InstanceSize))
		info.InstanceCount++
		info.TotalSize += shallowSize
	} else {
		shallowSize = alignTo8(objectHeaderSize(state.sizeMode) + int64(dataSize))
	}
	state.totalHeapSize += shallowSize
	state.totalInstances++

	// Register object in index
	objIdx := state.objectStore.AddObject(objectID, classID, shallowSize)

	// Count references (edges) by scanning instance data field types
	refCount := p.countInstanceReferences(state, classID, int(dataSize))

	if refCount > 0 {
		state.edgeCount += int64(refCount)
		// Store degree count temporarily in retained size field
		state.objectStore.AddRetainedSize(objIdx, int64(refCount))
	}

	// Skip actual instance data
	if err := state.reader.Skip(int64(dataSize)); err != nil {
		return 0, err
	}
	bytesRead += int64(dataSize)

	return bytesRead, nil
}

// countInstanceReferences counts how many object references an instance has
// based on its class hierarchy field descriptors.
func (p *Parser) countInstanceReferences(state *scanState, classID uint64, dataSize int) int32 {
	// Get all fields for this class hierarchy
	allFields := p.getScanClassHierarchyFields(state, classID)
	if len(allFields) == 0 {
		// Class info not yet available, defer
		state.deferredInstances = append(state.deferredInstances, deferredScanInstance{
			objectID: 0, // not needed for counting
			classID:  classID,
			dataSize: dataSize,
		})
		return 0
	}

	var count int32
	for _, field := range allFields {
		if field.Type == TypeObject {
			count++
		}
	}
	return count
}

// getScanClassHierarchyFields returns all fields for a class hierarchy during scan.
func (p *Parser) getScanClassHierarchyFields(state *scanState, classID uint64) []FieldDescriptor {
	var allFields []FieldDescriptor
	currentID := classID

	for currentID != 0 {
		if fields, ok := state.classFields[currentID]; ok {
			allFields = append(allFields, fields...)
		}
		// Walk up to superclass
		if info, ok := state.classInfo[currentID]; ok {
			currentID = info.SuperClassID
		} else {
			break
		}
	}
	return allFields
}

// countDeferredDegrees processes deferred instances whose class info wasn't available initially.
func (p *Parser) countDeferredDegrees(state *scanState) {
	for _, deferred := range state.deferredInstances {
		allFields := p.getScanClassHierarchyFields(state, deferred.classID)
		var refCount int32
		for _, field := range allFields {
			if field.Type == TypeObject {
				refCount++
			}
		}
		if refCount > 0 {
			state.edgeCount += int64(refCount)
			// Note: for deferred instances without objectID, we can't attribute degrees
			// This is acceptable as it only slightly over-allocates CSR
		}
	}
	state.deferredInstances = nil
}

// scanObjectArrayDump scans an OBJECT_ARRAY_DUMP record.
func (p *Parser) scanObjectArrayDump(state *scanState) (int64, error) {
	state.arrayDumpCount++
	idSize := state.reader.IDSize()
	var bytesRead int64

	objectID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Stack trace serial
	if _, err := state.reader.ReadUint32(); err != nil {
		return 0, err
	}
	bytesRead += 4

	// Number of elements
	numElements, err := state.reader.ReadUint32()
	if err != nil {
		return 0, err
	}
	bytesRead += 4

	// Array class ID
	arrayClassID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Calculate shallow size: header + length field + elements
	arrHeaderSize := objectHeaderSize(state.sizeMode) + 4 // 4 bytes for array length
	shallowSize := alignTo8(arrHeaderSize + int64(numElements)*int64(idSize))
	state.totalHeapSize += shallowSize
	state.totalInstances++

	// Register object
	objIdx := state.objectStore.AddObject(objectID, arrayClassID, shallowSize)

	// All elements are potential references
	state.edgeCount += int64(numElements)
	state.objectStore.AddRetainedSize(objIdx, int64(numElements))

	// Skip array elements data
	elemBytes := int64(numElements) * int64(idSize)
	if err := state.reader.Skip(elemBytes); err != nil {
		return 0, err
	}
	bytesRead += elemBytes

	return bytesRead, nil
}

// scanPrimitiveArrayDump scans a PRIMITIVE_ARRAY_DUMP record.
func (p *Parser) scanPrimitiveArrayDump(state *scanState) (int64, error) {
	state.arrayDumpCount++
	idSize := state.reader.IDSize()
	var bytesRead int64

	objectID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Stack trace serial
	if _, err := state.reader.ReadUint32(); err != nil {
		return 0, err
	}
	bytesRead += 4

	numElements, err := state.reader.ReadUint32()
	if err != nil {
		return 0, err
	}
	bytesRead += 4

	elementTypeByte, err := state.reader.ReadByte()
	if err != nil {
		return 0, err
	}
	bytesRead++

	elementType := BasicType(elementTypeByte)

	// Calculate element size and shallow size
	elementSize := BasicTypeSize(elementType, idSize)
	arrHeaderSize := objectHeaderSize(state.sizeMode) + 4
	dataSize := int64(numElements) * int64(elementSize)
	shallowSize := alignTo8(arrHeaderSize + dataSize)
	state.totalHeapSize += shallowSize
	state.totalInstances++

	// Determine class name for primitive arrays
	var classID uint64
	typeName := primitiveArrayTypeName(elementType)
	if info, ok := state.classByName[typeName]; ok {
		classID = info.ClassID
		info.InstanceCount++
		info.TotalSize += shallowSize
	} else {
		// Create class info if not seen yet
		classID = uint64(0x1000000 + int(elementTypeByte))
		state.classByName[typeName] = &ClassInfo{
			ClassID:       classID,
			Name:          typeName,
			InstanceCount: 1,
			TotalSize:     shallowSize,
		}
	}

	// Register object (primitive arrays have no outgoing references)
	state.objectStore.AddObject(objectID, classID, shallowSize)

	// Skip array data
	if err := state.reader.Skip(dataSize); err != nil {
		return 0, err
	}
	bytesRead += dataSize

	return bytesRead, nil
}

// ExtractDegreeCounts extracts degree counts from the ScanResult.
// During scanning, degrees were temporarily stored in the retainedSizes field
// via AddRetainedSize. Since AddObject initializes retainedSize = shallowSize,
// the actual degree = retainedSize - shallowSize.
func (sr *ScanResult) ExtractDegreeCounts() []int32 {
	count := sr.ObjectIndex.Count()
	degrees := make([]int32, count)
	for i := int32(0); i < count; i++ {
		// Degree = retainedSize - shallowSize (we used AddRetainedSize to accumulate)
		retained := sr.ObjectIndex.GetRetainedSize(i)
		shallow := sr.ObjectIndex.GetShallowSize(i)
		degrees[i] = int32(retained - shallow)
	}
	// Reset retained sizes to shallow sizes (since we hijacked the field)
	for i := int32(0); i < count; i++ {
		sr.ObjectIndex.SetRetainedSize(i, sr.ObjectIndex.GetShallowSize(i))
	}
	sr.DegreeCounts = degrees
	return degrees
}
