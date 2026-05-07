// Package hprof provides parsing functionality for Java HPROF heap dump files.
// This file implements Pass 2 of the Two-Pass CSR parsing strategy:
// re-read the HPROF file using metadata from Pass 1 to extract actual reference
// targets and build the compact CSR edge lists.
package hprof

import (
	"context"
	"fmt"
	"io"

	timerPkg "github.com/junjiewwang/perf-analysis/perflib/internal/timer"
)

// BuildPass performs Pass 2: re-read the HPROF file to extract actual reference targets
// and build the CSR (Compressed Sparse Row) edge lists for the reference graph.
//
// It uses the ScanResult from Pass 1 which provides:
//   - ObjectIndex: mapping objectID → int32 index
//   - ClassFields: field descriptors for each class
//   - ClassInfo: class hierarchy (for walking superclass chains)
//   - EdgeCount: total edges for pre-allocation
//   - Strings: for field name resolution
func (p *Parser) BuildPass(ctx context.Context, r io.ReadSeeker, scanResult *ScanResult) (*IndexedReferenceGraph, error) {
	timer := timerPkg.New("HPROF Build Pass",
		timerPkg.WithLogger(p.opts.Logger),
		timerPkg.WithEnabled(p.opts.Logger != nil))

	// Seek back to file start for second read
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("build pass: failed to seek to start: %w", err)
	}

	reader := NewReader(r)

	// Re-read header (needed to set idSize on the new reader)
	header, err := reader.ReadHeader()
	if err != nil {
		return nil, fmt.Errorf("build pass: failed to read header: %w", err)
	}
	_ = header // header already captured in ScanResult

	// Extract degree counts from scan result (resets retainedSize field)
	pt := timer.Start("Extract degree counts")
	scanResult.ExtractDegreeCounts()
	pt.Stop()

	// Initialize build state
	nodeCount := int(scanResult.ObjectCount)
	estimatedEdges := int(scanResult.EdgeCount)

	state := &buildState{
		reader:      reader,
		scanResult:  scanResult,
		outBuilder:  NewCompactEdgeListBuilder(nodeCount, estimatedEdges),
		inBuilder:   NewCompactEdgeListBuilder(nodeCount, estimatedEdges),
		objectIndex: scanResult.ObjectIndex,
		classInfo:   scanResult.ClassInfo,
		classFields: scanResult.ClassFields,
		strings:     scanResult.Strings,
		idSize:      reader.IDSize(),
	}

	// Phase: Parse all records and extract references
	pt = timer.Start("Build reference edges")
	if err := p.buildRecords(ctx, state); err != nil {
		return nil, fmt.Errorf("build pass: failed to build edges: %w", err)
	}
	pt.Stop()

	// Process deferred instances (those parsed before their CLASS_DUMP in this pass too)
	timer.TimeFunc("Process deferred build instances", func() {
		p.processDeferredBuildInstances(state)
	})

	// Assemble the IndexedReferenceGraph
	pt = timer.Start("Assemble graph")
	graph := p.assembleGraph(scanResult, state)
	pt.Stop()

	// Compute reachability from GC roots
	pt = timer.Start("Compute reachability")
	p.computeReachability(graph)
	pt.Stop()

	timer.PrintSummary()

	if p.opts.Logger != nil {
		p.opts.Logger.Info("Build pass complete: %d outgoing edges, %d incoming edges",
			state.outBuilder.EdgeCount(), state.inBuilder.EdgeCount())
	}

	return graph, nil
}

// buildState holds temporary state during the build pass.
type buildState struct {
	reader      *Reader
	scanResult  *ScanResult
	outBuilder  *CompactEdgeListBuilder
	inBuilder   *CompactEdgeListBuilder
	objectIndex *IndexedObjectStore
	classInfo   map[uint64]*ClassInfo
	classFields map[uint64][]FieldDescriptor
	strings     map[uint64]string
	idSize      int

	// Deferred instances (CLASS_DUMP not yet seen during this pass)
	deferredInstances []deferredBuildInstance
}

// deferredBuildInstance holds instance data for deferred reference extraction in build pass.
type deferredBuildInstance struct {
	objectID uint64
	classID  uint64
	data     []byte
}

// EdgeCount returns the current number of edges added to the outgoing builder.
func (b *CompactEdgeListBuilder) EdgeCount() int {
	return len(b.edges)
}

// buildRecords parses all records in the HPROF file to extract actual references.
func (p *Parser) buildRecords(ctx context.Context, state *buildState) error {
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
		case TagHeapDump, TagHeapDumpSegment:
			if err := p.buildHeapDump(ctx, state, length); err != nil {
				return err
			}
		case TagHeapDumpEnd:
			// End of heap dump segments
		default:
			// Skip all non-heap-dump records (already captured in Pass 1)
			if err := state.reader.Skip(int64(length)); err != nil {
				return err
			}
		}
	}
}

// buildHeapDump processes a HEAP_DUMP or HEAP_DUMP_SEGMENT record to extract references.
func (p *Parser) buildHeapDump(ctx context.Context, state *buildState, length uint32) error {
	endPos := int64(length)
	var bytesRead int64

	for bytesRead < endPos {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		subTagByte, err := state.reader.ReadByte()
		if err != nil {
			return err
		}
		bytesRead++

		subTag := HeapDumpTag(subTagByte)
		n, err := p.buildHeapDumpSubRecord(state, subTag)
		if err != nil {
			return err
		}
		bytesRead += n
	}

	return nil
}

// buildHeapDumpSubRecord dispatches processing of a heap dump sub-record during build pass.
// Returns the number of bytes consumed (excluding the tag byte).
func (p *Parser) buildHeapDumpSubRecord(state *buildState, tag HeapDumpTag) (int64, error) {
	idSize := state.idSize

	switch tag {
	case 0x00:
		// Padding byte
		return 0, nil

	// GC root types - skip data (already captured in Pass 1)
	case HeapTagRootJNIGlobal:
		if err := state.reader.Skip(int64(idSize * 2)); err != nil {
			return 0, err
		}
		return int64(idSize * 2), nil

	case HeapTagRootJNILocal, HeapTagRootJavaFrame:
		if err := state.reader.Skip(int64(idSize + 8)); err != nil {
			return 0, err
		}
		return int64(idSize + 8), nil

	case HeapTagRootNativeStack, HeapTagRootThreadBlock:
		if err := state.reader.Skip(int64(idSize + 4)); err != nil {
			return 0, err
		}
		return int64(idSize + 4), nil

	case HeapTagRootStickyClass, HeapTagRootMonitorUsed:
		if err := state.reader.Skip(int64(idSize)); err != nil {
			return 0, err
		}
		return int64(idSize), nil

	case HeapTagRootThreadObject:
		if err := state.reader.Skip(int64(idSize + 8)); err != nil {
			return 0, err
		}
		return int64(idSize + 8), nil

	case HeapTagRootUnknown:
		if err := state.reader.Skip(int64(idSize)); err != nil {
			return 0, err
		}
		return int64(idSize), nil

	// Android/OpenJDK specific root types (single ID)
	case 0x89, 0x8A, 0x8B, 0x8C, 0x8D:
		if err := state.reader.Skip(int64(idSize)); err != nil {
			return 0, err
		}
		return int64(idSize), nil

	case 0x8E: // ROOT_JNI_MONITOR
		if err := state.reader.Skip(int64(idSize + 8)); err != nil {
			return 0, err
		}
		return int64(idSize + 8), nil

	case 0xC3: // HEAP_DUMP_INFO (Android specific)
		if err := state.reader.Skip(int64(4 + idSize)); err != nil {
			return 0, err
		}
		return int64(4 + idSize), nil

	case 0xFE: // ROOT_UNREACHABLE
		if err := state.reader.Skip(int64(idSize)); err != nil {
			return 0, err
		}
		return int64(idSize), nil

	// Core dump types - extract references
	case HeapTagClassDump:
		return p.buildClassDump(state)

	case HeapTagInstanceDump:
		return p.buildInstanceDump(state)

	case HeapTagObjectArrayDump:
		return p.buildObjectArrayDump(state)

	case HeapTagPrimitiveArrayDump:
		return p.buildPrimitiveArrayDump(state)

	default:
		return 0, fmt.Errorf("build pass: unknown heap dump sub-tag 0x%02x", tag)
	}
}

// buildClassDump processes a CLASS_DUMP record to extract static field references.
func (p *Parser) buildClassDump(state *buildState) (int64, error) {
	idSize := state.idSize
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
	if _, err := state.reader.ReadID(); err != nil {
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
	if _, err := state.reader.ReadUint32(); err != nil {
		return 0, err
	}
	bytesRead += 4

	// Constant pool - skip
	cpCount, err := state.reader.ReadUint16()
	if err != nil {
		return 0, err
	}
	bytesRead += 2
	for i := 0; i < int(cpCount); i++ {
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

	// Static fields - extract object references
	sfCount, err := state.reader.ReadUint16()
	if err != nil {
		return 0, err
	}
	bytesRead += 2

	srcIdx := state.objectIndex.GetIndex(classObjID)

	for i := 0; i < int(sfCount); i++ {
		fieldNameID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		bytesRead += int64(idSize)

		ty, err := state.reader.ReadByte()
		if err != nil {
			return 0, err
		}
		bytesRead++

		size := BasicTypeSize(BasicType(ty), idSize)

		if BasicType(ty) == TypeObject {
			refID, err := state.reader.ReadID()
			if err != nil {
				return 0, err
			}
			bytesRead += int64(size)

			// Add edge for static field reference
			if refID != 0 && srcIdx >= 0 {
				tgtIdx := state.objectIndex.GetIndex(refID)
				if tgtIdx >= 0 {
					fieldName := state.strings[fieldNameID]
					state.outBuilder.AddEdge(srcIdx, tgtIdx, fieldName, classObjID)
					state.inBuilder.AddEdge(tgtIdx, srcIdx, fieldName, classObjID)
				}
			}
		} else {
			if err := state.reader.Skip(int64(size)); err != nil {
				return 0, err
			}
			bytesRead += int64(size)
		}
	}

	// Instance fields - skip (metadata already in ScanResult)
	ifCount, err := state.reader.ReadUint16()
	if err != nil {
		return 0, err
	}
	bytesRead += 2
	for i := 0; i < int(ifCount); i++ {
		if _, err := state.reader.ReadID(); err != nil {
			return 0, err
		}
		bytesRead += int64(idSize)
		if _, err := state.reader.ReadByte(); err != nil {
			return 0, err
		}
		bytesRead++
	}

	return bytesRead, nil
}

// buildInstanceDump processes an INSTANCE_DUMP record to extract instance field references.
func (p *Parser) buildInstanceDump(state *buildState) (int64, error) {
	idSize := state.idSize
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

	// Read instance data
	data, err := state.reader.ReadBytes(int(dataSize))
	if err != nil {
		return 0, err
	}
	bytesRead += int64(dataSize)

	// Get class hierarchy fields
	allFields := p.getBuildClassHierarchyFields(state, classID)
	if len(allFields) == 0 {
		// Class info not available yet, defer for later
		state.deferredInstances = append(state.deferredInstances, deferredBuildInstance{
			objectID: objectID,
			classID:  classID,
			data:     data,
		})
		return bytesRead, nil
	}

	// Extract references from instance data
	p.extractBuildReferences(state, objectID, classID, data, allFields)

	return bytesRead, nil
}

// extractBuildReferences extracts object references from instance data and adds edges.
func (p *Parser) extractBuildReferences(state *buildState, objectID, classID uint64, data []byte, allFields []FieldDescriptor) {
	idSize := state.idSize
	srcIdx := state.objectIndex.GetIndex(objectID)
	if srcIdx < 0 {
		return
	}

	offset := 0
	for _, field := range allFields {
		fieldSize := BasicTypeSize(field.Type, idSize)
		if offset+fieldSize > len(data) {
			break
		}

		if field.Type == TypeObject {
			refID := readObjectID(data[offset:], idSize)
			if refID != 0 {
				tgtIdx := state.objectIndex.GetIndex(refID)
				if tgtIdx >= 0 {
					fieldName := state.strings[field.NameID]
					state.outBuilder.AddEdge(srcIdx, tgtIdx, fieldName, classID)
					state.inBuilder.AddEdge(tgtIdx, srcIdx, fieldName, classID)
				}
			}
		}
		offset += fieldSize
	}
}

// readObjectID reads an object ID from a byte slice.
func readObjectID(data []byte, idSize int) uint64 {
	if idSize == 4 {
		if len(data) < 4 {
			return 0
		}
		return uint64(data[0])<<24 | uint64(data[1])<<16 |
			uint64(data[2])<<8 | uint64(data[3])
	}
	if len(data) < 8 {
		return 0
	}
	return uint64(data[0])<<56 | uint64(data[1])<<48 |
		uint64(data[2])<<40 | uint64(data[3])<<32 |
		uint64(data[4])<<24 | uint64(data[5])<<16 |
		uint64(data[6])<<8 | uint64(data[7])
}

// buildObjectArrayDump processes an OBJECT_ARRAY_DUMP to extract element references.
func (p *Parser) buildObjectArrayDump(state *buildState) (int64, error) {
	idSize := state.idSize
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

	// Array class ID
	arrayClassID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	srcIdx := state.objectIndex.GetIndex(objectID)

	// Read and process each element
	for i := 0; i < int(numElements); i++ {
		elemID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}

		if elemID != 0 && srcIdx >= 0 {
			tgtIdx := state.objectIndex.GetIndex(elemID)
			if tgtIdx >= 0 {
				// Use array index as field name for array elements
				fieldName := fmt.Sprintf("[%d]", i)
				state.outBuilder.AddEdge(srcIdx, tgtIdx, fieldName, arrayClassID)
				state.inBuilder.AddEdge(tgtIdx, srcIdx, fieldName, arrayClassID)
			}
		}
	}
	bytesRead += int64(int(numElements) * idSize)

	return bytesRead, nil
}

// buildPrimitiveArrayDump skips a PRIMITIVE_ARRAY_DUMP (no object references).
func (p *Parser) buildPrimitiveArrayDump(state *buildState) (int64, error) {
	idSize := state.idSize
	var bytesRead int64

	// Object ID
	if _, err := state.reader.ReadID(); err != nil {
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

	elementSize := BasicTypeSize(BasicType(elementTypeByte), idSize)
	dataSize := int64(numElements) * int64(elementSize)

	if err := state.reader.Skip(dataSize); err != nil {
		return 0, err
	}
	bytesRead += dataSize

	return bytesRead, nil
}

// getBuildClassHierarchyFields returns all fields for a class hierarchy during build pass.
// Uses pre-computed data from ScanResult instead of walking class hierarchy each time.
func (p *Parser) getBuildClassHierarchyFields(state *buildState, classID uint64) []FieldDescriptor {
	var allFields []FieldDescriptor
	currentID := classID

	for currentID != 0 {
		if fields, ok := state.classFields[currentID]; ok {
			allFields = append(allFields, fields...)
		}
		if info, ok := state.classInfo[currentID]; ok {
			currentID = info.SuperClassID
		} else {
			break
		}
	}
	return allFields
}

// processDeferredBuildInstances processes instances whose class info wasn't available
// when they were first encountered during the build pass.
func (p *Parser) processDeferredBuildInstances(state *buildState) {
	for _, deferred := range state.deferredInstances {
		allFields := p.getBuildClassHierarchyFields(state, deferred.classID)
		if len(allFields) == 0 {
			continue
		}
		p.extractBuildReferences(state, deferred.objectID, deferred.classID, deferred.data, allFields)
	}
	state.deferredInstances = nil
}

// assembleGraph builds the final IndexedReferenceGraph from scan results and built edges.
func (p *Parser) assembleGraph(scanResult *ScanResult, state *buildState) *IndexedReferenceGraph {
	graph := &IndexedReferenceGraph{
		objects:    scanResult.ObjectIndex,
		classNames: make(map[uint64]string, len(scanResult.ClassInfo)),
	}

	// Set class names
	for classID, info := range scanResult.ClassInfo {
		if info.Name != "" {
			graph.classNames[classID] = info.Name
		}
	}
	// Also resolve class names from string table for classes without direct name
	for classID, nameStringID := range scanResult.ClassNames {
		if _, exists := graph.classNames[classID]; !exists {
			if name, ok := scanResult.Strings[nameStringID]; ok {
				graph.classNames[classID] = normalizeClassName(name)
			}
		}
	}

	// Build edge lists from builders
	graph.BuildEdges(state.outBuilder, state.inBuilder)

	// Setup GC root bitset
	count := int(scanResult.ObjectCount)
	graph.gcRootBits = NewBitset(count)
	graph.classObjectBits = NewBitset(count)
	graph.reachableBits = NewBitset(count)

	// Convert GCRootEntry to GCRoot and mark bitset
	graph.gcRoots = make([]GCRoot, 0, len(scanResult.GCRoots))
	for _, entry := range scanResult.GCRoots {
		root := GCRoot{
			ObjectID: entry.ObjectID,
			Type:     entry.RootType,
			ThreadID: entry.ThreadID,
		}
		graph.gcRoots = append(graph.gcRoots, root)
		idx := scanResult.ObjectIndex.GetIndex(entry.ObjectID)
		if idx >= 0 {
			graph.gcRootBits.Set(int(idx))
		}
	}

	// Mark class objects
	if scanResult.javaLangClassID != 0 {
		for classID := range scanResult.ClassInfo {
			idx := scanResult.ObjectIndex.GetIndex(classID)
			if idx >= 0 {
				graph.classObjectBits.Set(int(idx))
			}
		}
	}

	return graph
}

// computeReachability performs BFS from GC roots to mark reachable objects.
func (p *Parser) computeReachability(graph *IndexedReferenceGraph) {
	if graph.outgoing == nil {
		return
	}

	objectCount := graph.ObjectCount()
	queue := make([]int32, 0, objectCount/4)

	// Start BFS from all GC roots
	for _, root := range graph.gcRoots {
		idx := graph.objects.GetIndex(root.ObjectID)
		if idx >= 0 && !graph.reachableBits.Test(int(idx)) {
			graph.reachableBits.Set(int(idx))
			queue = append(queue, idx)
		}
	}

	// BFS traversal
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		targets, _, _ := graph.outgoing.GetEdges(current)
		for _, target := range targets {
			if !graph.reachableBits.Test(int(target)) {
				graph.reachableBits.Set(int(target))
				queue = append(queue, target)
			}
		}
	}
}

// ParseTwoPass orchestrates the two-pass parsing strategy.
// Pass 1 (ScanPass): scans to collect metadata and edge degree counts.
// Pass 2 (BuildPass): re-reads to extract actual references and build CSR graph.
//
// If the underlying reader implements io.ReaderAt and there are enough segments,
// Build Pass automatically uses parallel execution for improved throughput.
func (p *Parser) ParseTwoPass(ctx context.Context, r io.ReadSeeker) (*IndexedReferenceGraph, *ScanResult, error) {
	// Pass 1: Scan
	scanResult, err := p.ScanPass(ctx, r)
	if err != nil {
		return nil, nil, fmt.Errorf("two-pass parse: scan pass failed: %w", err)
	}

	// Pass 2: Build (try parallel if possible)
	var graph *IndexedReferenceGraph

	ra, isReaderAt := r.(io.ReaderAt)
	if isReaderAt && len(scanResult.SegmentOffsets) >= parallelBuildThreshold {
		// Use parallel Build Pass
		graph, err = p.BuildPassParallel(ctx, ra, scanResult)
		if err != nil {
			return nil, scanResult, fmt.Errorf("two-pass parse: parallel build pass failed: %w", err)
		}
	} else {
		// Fall back to sequential Build Pass
		graph, err = p.BuildPass(ctx, r, scanResult)
		if err != nil {
			return nil, scanResult, fmt.Errorf("two-pass parse: build pass failed: %w", err)
		}
	}

	return graph, scanResult, nil
}
