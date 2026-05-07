// Package hprof provides parsing functionality for Java HPROF heap dump files.
// This file implements parallel Build Pass: multiple goroutines independently parse
// different HEAP_DUMP_SEGMENT regions of the hprof file in parallel, each producing
// its own edge list, then merge all results into the final CSR graph.
package hprof

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync"

	timerPkg "github.com/junjiewwang/perf-analysis/perflib/internal/timer"
)

// parallelBuildThreshold is the minimum number of segments required to trigger parallel build.
// Below this threshold, the sequential build pass is used.
const parallelBuildThreshold = 4

// BuildPassParallel performs Pass 2 in parallel: multiple goroutines independently
// parse different heap dump segments using io.ReaderAt for concurrent random access.
//
// Prerequisites:
//   - scanResult.SegmentOffsets must be populated (from Scan Pass)
//   - ra must support concurrent random-access reads (e.g., *os.File)
//
// The algorithm:
//  1. Divide segments among N workers (N = NumCPU, capped at segment count)
//  2. Each worker reads its assigned segments via SectionReader and builds local edge lists
//  3. Merge all worker edge lists into unified CompactEdgeListBuilders
//  4. Assemble the final IndexedReferenceGraph (same as sequential path)
func (p *Parser) BuildPassParallel(ctx context.Context, ra io.ReaderAt, scanResult *ScanResult) (*IndexedReferenceGraph, error) {
	timer := timerPkg.New("HPROF Build Pass (Parallel)",
		timerPkg.WithLogger(p.opts.Logger),
		timerPkg.WithEnabled(p.opts.Logger != nil))

	segments := scanResult.SegmentOffsets
	if len(segments) == 0 {
		return nil, fmt.Errorf("parallel build pass: no segment offsets available")
	}

	// Extract degree counts from scan result
	pt := timer.Start("Extract degree counts")
	scanResult.ExtractDegreeCounts()
	pt.Stop()

	// Determine worker count
	numWorkers := runtime.NumCPU()
	if numWorkers > len(segments) {
		numWorkers = len(segments)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	// Distribute segments among workers (contiguous chunks for locality)
	assignments := distributeSegments(segments, numWorkers)

	nodeCount := int(scanResult.ObjectCount)
	estimatedEdgesPerWorker := int(scanResult.EdgeCount) / numWorkers

	// Phase: Parse segments in parallel
	pt = timer.Start("Build reference edges (parallel)")

	results := make([]workerResult, numWorkers)
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerIdx int) {
			defer wg.Done()

			workerSegs := assignments[workerIdx]
			if len(workerSegs) == 0 {
				return
			}

			wr := p.processSegmentsWorker(ctx, ra, scanResult, workerSegs, estimatedEdgesPerWorker)
			results[workerIdx] = wr
		}(w)
	}

	wg.Wait()
	pt.Stop()

	// Check for worker errors
	for i, r := range results {
		if r.err != nil {
			return nil, fmt.Errorf("parallel build pass: worker %d failed: %w", i, r.err)
		}
	}

	// Phase: Merge worker results
	pt = timer.Start("Merge worker edges")
	outBuilder, inBuilder := mergeWorkerEdges(results, nodeCount, int(scanResult.EdgeCount))

	var allDeferred []deferredBuildInstance
	for _, r := range results {
		allDeferred = append(allDeferred, r.deferredInstances...)
	}
	pt.Stop()

	// Phase: Process deferred instances
	timer.TimeFunc("Process deferred build instances", func() {
		p.processDeferredBuildParallel(scanResult, outBuilder, inBuilder, allDeferred)
	})

	// Phase: Assemble the graph
	pt = timer.Start("Assemble graph")
	state := &buildState{
		scanResult:  scanResult,
		outBuilder:  outBuilder,
		inBuilder:   inBuilder,
		objectIndex: scanResult.ObjectIndex,
		classInfo:   scanResult.ClassInfo,
		classFields: scanResult.ClassFields,
		strings:     scanResult.Strings,
		idSize:      scanResult.Header.IDSize,
	}
	graph := p.assembleGraph(scanResult, state)
	pt.Stop()

	// Phase: Compute reachability
	pt = timer.Start("Compute reachability")
	p.computeReachability(graph)
	pt.Stop()

	timer.PrintSummary()

	if p.opts.Logger != nil {
		p.opts.Logger.Info("Parallel build pass complete (%d workers, %d segments): %d outgoing edges, %d incoming edges",
			numWorkers, len(segments), outBuilder.EdgeCount(), inBuilder.EdgeCount())
	}

	return graph, nil
}

// edgeRecord holds a single edge captured by a worker.
type edgeRecord struct {
	from      int32
	to        int32
	fieldID   int32  // local field ID within the worker (not used in merge)
	fieldName string // actual field name string
	classID   uint64
}

// workerResult holds the output of a single worker goroutine.
type workerResult struct {
	outEdges          []edgeRecord
	inEdges           []edgeRecord
	fieldNames        []string
	fieldToID         map[string]int32
	deferredInstances []deferredBuildInstance
	err               error
}

// processSegmentsWorker processes a set of segments for a single worker goroutine.
// It creates a local Reader via SectionReader for each segment and extracts edges.
func (p *Parser) processSegmentsWorker(
	ctx context.Context,
	ra io.ReaderAt,
	scanResult *ScanResult,
	segs []SegmentInfo,
	estimatedEdges int,
) workerResult {
	var result workerResult
	result.outEdges = make([]edgeRecord, 0, estimatedEdges)
	result.inEdges = make([]edgeRecord, 0, estimatedEdges)
	result.fieldNames = make([]string, 0, 256)
	result.fieldToID = make(map[string]int32, 256)

	idSize := scanResult.Header.IDSize
	objectIndex := scanResult.ObjectIndex
	classInfo := scanResult.ClassInfo
	classFields := scanResult.ClassFields
	strings := scanResult.Strings

	for _, seg := range segs {
		select {
		case <-ctx.Done():
			result.err = ctx.Err()
			return result
		default:
		}

		// Create a SectionReader for this segment
		sr := io.NewSectionReader(ra, seg.Offset, int64(seg.Length))
		reader := NewReader(sr)
		reader.SetIDSize(idSize)

		// Parse all sub-records in this segment
		var bytesRead int64
		endPos := int64(seg.Length)

		for bytesRead < endPos {
			select {
			case <-ctx.Done():
				result.err = ctx.Err()
				return result
			default:
			}

			subTagByte, err := reader.ReadByte()
			if err != nil {
				if err == io.EOF {
					break
				}
				result.err = fmt.Errorf("segment at offset %d: read sub-tag: %w", seg.Offset, err)
				return result
			}
			bytesRead++

			subTag := HeapDumpTag(subTagByte)
			n, err := p.buildSubRecordParallel(reader, subTag, idSize, objectIndex, classInfo, classFields, strings, &result)
			if err != nil {
				result.err = fmt.Errorf("segment at offset %d, sub-tag 0x%02x: %w", seg.Offset, subTag, err)
				return result
			}
			bytesRead += n
		}
	}

	return result
}

// buildSubRecordParallel dispatches a heap dump sub-record during parallel build.
// Returns bytes consumed (excluding tag byte).
func (p *Parser) buildSubRecordParallel(
	reader *Reader,
	tag HeapDumpTag,
	idSize int,
	objectIndex *IndexedObjectStore,
	classInfo map[uint64]*ClassInfo,
	classFields map[uint64][]FieldDescriptor,
	strings map[uint64]string,
	result *workerResult,
) (int64, error) {
	switch tag {
	case 0x00:
		return 0, nil

	// GC root types - skip (already captured in Pass 1)
	case HeapTagRootJNIGlobal:
		if err := reader.Skip(int64(idSize * 2)); err != nil {
			return 0, err
		}
		return int64(idSize * 2), nil

	case HeapTagRootJNILocal, HeapTagRootJavaFrame:
		if err := reader.Skip(int64(idSize + 8)); err != nil {
			return 0, err
		}
		return int64(idSize + 8), nil

	case HeapTagRootNativeStack, HeapTagRootThreadBlock:
		if err := reader.Skip(int64(idSize + 4)); err != nil {
			return 0, err
		}
		return int64(idSize + 4), nil

	case HeapTagRootStickyClass, HeapTagRootMonitorUsed:
		if err := reader.Skip(int64(idSize)); err != nil {
			return 0, err
		}
		return int64(idSize), nil

	case HeapTagRootThreadObject:
		if err := reader.Skip(int64(idSize + 8)); err != nil {
			return 0, err
		}
		return int64(idSize + 8), nil

	case HeapTagRootUnknown:
		if err := reader.Skip(int64(idSize)); err != nil {
			return 0, err
		}
		return int64(idSize), nil

	// Android/OpenJDK specific root types
	case 0x89, 0x8A, 0x8B, 0x8C, 0x8D:
		if err := reader.Skip(int64(idSize)); err != nil {
			return 0, err
		}
		return int64(idSize), nil

	case 0x8E: // ROOT_JNI_MONITOR
		if err := reader.Skip(int64(idSize + 8)); err != nil {
			return 0, err
		}
		return int64(idSize + 8), nil

	case 0xC3: // HEAP_DUMP_INFO
		if err := reader.Skip(int64(4 + idSize)); err != nil {
			return 0, err
		}
		return int64(4 + idSize), nil

	case 0xFE: // ROOT_UNREACHABLE
		if err := reader.Skip(int64(idSize)); err != nil {
			return 0, err
		}
		return int64(idSize), nil

	// Core dump types - extract references
	case HeapTagClassDump:
		return p.buildClassDumpParallel(reader, idSize, objectIndex, strings, result)

	case HeapTagInstanceDump:
		return p.buildInstanceDumpParallel(reader, idSize, objectIndex, classInfo, classFields, strings, result)

	case HeapTagObjectArrayDump:
		return p.buildObjectArrayDumpParallel(reader, idSize, objectIndex, result)

	case HeapTagPrimitiveArrayDump:
		return p.buildPrimitiveArrayDumpParallel(reader, idSize)

	default:
		return 0, fmt.Errorf("unknown heap dump sub-tag 0x%02x", tag)
	}
}

// buildClassDumpParallel extracts static field references from a CLASS_DUMP record.
func (p *Parser) buildClassDumpParallel(
	reader *Reader,
	idSize int,
	objectIndex *IndexedObjectStore,
	strings map[uint64]string,
	result *workerResult,
) (int64, error) {
	var bytesRead int64

	classObjID, err := reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Stack trace serial
	if _, err := reader.ReadUint32(); err != nil {
		return 0, err
	}
	bytesRead += 4

	// Super class ID
	if _, err := reader.ReadID(); err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Class loader, signers, protection domain, reserved1, reserved2
	for i := 0; i < 5; i++ {
		if _, err := reader.ReadID(); err != nil {
			return 0, err
		}
	}
	bytesRead += int64(idSize * 5)

	// Instance size
	if _, err := reader.ReadUint32(); err != nil {
		return 0, err
	}
	bytesRead += 4

	// Constant pool - skip
	cpCount, err := reader.ReadUint16()
	if err != nil {
		return 0, err
	}
	bytesRead += 2
	for i := 0; i < int(cpCount); i++ {
		if _, err := reader.ReadUint16(); err != nil {
			return 0, err
		}
		bytesRead += 2
		ty, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		bytesRead++
		size := BasicTypeSize(BasicType(ty), idSize)
		if err := reader.Skip(int64(size)); err != nil {
			return 0, err
		}
		bytesRead += int64(size)
	}

	// Static fields - extract object references
	sfCount, err := reader.ReadUint16()
	if err != nil {
		return 0, err
	}
	bytesRead += 2

	srcIdx := objectIndex.GetIndex(classObjID)

	for i := 0; i < int(sfCount); i++ {
		fieldNameID, err := reader.ReadID()
		if err != nil {
			return 0, err
		}
		bytesRead += int64(idSize)

		ty, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		bytesRead++

		size := BasicTypeSize(BasicType(ty), idSize)

		if BasicType(ty) == TypeObject {
			refID, err := reader.ReadID()
			if err != nil {
				return 0, err
			}
			bytesRead += int64(size)

			if refID != 0 && srcIdx >= 0 {
				tgtIdx := objectIndex.GetIndex(refID)
				if tgtIdx >= 0 {
					fieldName := strings[fieldNameID]
					result.outEdges = append(result.outEdges, edgeRecord{
						from: srcIdx, to: tgtIdx, fieldName: fieldName, classID: classObjID,
					})
					result.inEdges = append(result.inEdges, edgeRecord{
						from: tgtIdx, to: srcIdx, fieldName: fieldName, classID: classObjID,
					})
				}
			}
		} else {
			if err := reader.Skip(int64(size)); err != nil {
				return 0, err
			}
			bytesRead += int64(size)
		}
	}

	// Instance fields - skip
	ifCount, err := reader.ReadUint16()
	if err != nil {
		return 0, err
	}
	bytesRead += 2
	for i := 0; i < int(ifCount); i++ {
		if _, err := reader.ReadID(); err != nil {
			return 0, err
		}
		bytesRead += int64(idSize)
		if _, err := reader.ReadByte(); err != nil {
			return 0, err
		}
		bytesRead++
	}

	return bytesRead, nil
}

// buildInstanceDumpParallel extracts instance field references from an INSTANCE_DUMP record.
func (p *Parser) buildInstanceDumpParallel(
	reader *Reader,
	idSize int,
	objectIndex *IndexedObjectStore,
	classInfo map[uint64]*ClassInfo,
	classFields map[uint64][]FieldDescriptor,
	strings map[uint64]string,
	result *workerResult,
) (int64, error) {
	var bytesRead int64

	objectID, err := reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Stack trace serial
	if _, err := reader.ReadUint32(); err != nil {
		return 0, err
	}
	bytesRead += 4

	classID, err := reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	dataSize, err := reader.ReadUint32()
	if err != nil {
		return 0, err
	}
	bytesRead += 4

	// Read instance data
	data, err := reader.ReadBytes(int(dataSize))
	if err != nil {
		return 0, err
	}
	bytesRead += int64(dataSize)

	// Get class hierarchy fields
	allFields := getClassHierarchyFieldsFromMaps(classID, classInfo, classFields)
	if len(allFields) == 0 {
		// Defer for later processing
		result.deferredInstances = append(result.deferredInstances, deferredBuildInstance{
			objectID: objectID,
			classID:  classID,
			data:     data,
		})
		return bytesRead, nil
	}

	// Extract references
	extractReferencesParallel(objectID, classID, data, allFields, idSize, objectIndex, strings, result)

	return bytesRead, nil
}

// buildObjectArrayDumpParallel extracts element references from an OBJECT_ARRAY_DUMP.
func (p *Parser) buildObjectArrayDumpParallel(
	reader *Reader,
	idSize int,
	objectIndex *IndexedObjectStore,
	result *workerResult,
) (int64, error) {
	var bytesRead int64

	objectID, err := reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Stack trace serial
	if _, err := reader.ReadUint32(); err != nil {
		return 0, err
	}
	bytesRead += 4

	numElements, err := reader.ReadUint32()
	if err != nil {
		return 0, err
	}
	bytesRead += 4

	// Array class ID
	arrayClassID, err := reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	srcIdx := objectIndex.GetIndex(objectID)

	// Read and process each element
	for i := 0; i < int(numElements); i++ {
		elemID, err := reader.ReadID()
		if err != nil {
			return 0, err
		}

		if elemID != 0 && srcIdx >= 0 {
			tgtIdx := objectIndex.GetIndex(elemID)
			if tgtIdx >= 0 {
				fieldName := fmt.Sprintf("[%d]", i)
				result.outEdges = append(result.outEdges, edgeRecord{
					from: srcIdx, to: tgtIdx, fieldName: fieldName, classID: arrayClassID,
				})
				result.inEdges = append(result.inEdges, edgeRecord{
					from: tgtIdx, to: srcIdx, fieldName: fieldName, classID: arrayClassID,
				})
			}
		}
	}
	bytesRead += int64(int(numElements) * idSize)

	return bytesRead, nil
}

// buildPrimitiveArrayDumpParallel skips a PRIMITIVE_ARRAY_DUMP (no references).
func (p *Parser) buildPrimitiveArrayDumpParallel(reader *Reader, idSize int) (int64, error) {
	var bytesRead int64

	// Object ID
	if _, err := reader.ReadID(); err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Stack trace serial
	if _, err := reader.ReadUint32(); err != nil {
		return 0, err
	}
	bytesRead += 4

	numElements, err := reader.ReadUint32()
	if err != nil {
		return 0, err
	}
	bytesRead += 4

	elementTypeByte, err := reader.ReadByte()
	if err != nil {
		return 0, err
	}
	bytesRead++

	elementSize := BasicTypeSize(BasicType(elementTypeByte), idSize)
	dataSize := int64(numElements) * int64(elementSize)

	if err := reader.Skip(dataSize); err != nil {
		return 0, err
	}
	bytesRead += dataSize

	return bytesRead, nil
}

// getClassHierarchyFieldsFromMaps returns all fields for a class hierarchy using map lookups.
// This is the same logic as getBuildClassHierarchyFields but takes maps directly for reuse.
func getClassHierarchyFieldsFromMaps(classID uint64, classInfo map[uint64]*ClassInfo, classFields map[uint64][]FieldDescriptor) []FieldDescriptor {
	var allFields []FieldDescriptor
	currentID := classID

	for currentID != 0 {
		if fields, ok := classFields[currentID]; ok {
			allFields = append(allFields, fields...)
		}
		if info, ok := classInfo[currentID]; ok {
			currentID = info.SuperClassID
		} else {
			break
		}
	}
	return allFields
}

// extractReferencesParallel extracts object references from instance data and appends edges to result.
func extractReferencesParallel(
	objectID, classID uint64,
	data []byte,
	allFields []FieldDescriptor,
	idSize int,
	objectIndex *IndexedObjectStore,
	strings map[uint64]string,
	result *workerResult,
) {
	srcIdx := objectIndex.GetIndex(objectID)
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
				tgtIdx := objectIndex.GetIndex(refID)
				if tgtIdx >= 0 {
					fieldName := strings[field.NameID]
					result.outEdges = append(result.outEdges, edgeRecord{
						from: srcIdx, to: tgtIdx, fieldName: fieldName, classID: classID,
					})
					result.inEdges = append(result.inEdges, edgeRecord{
						from: tgtIdx, to: srcIdx, fieldName: fieldName, classID: classID,
					})
				}
			}
		}
		offset += fieldSize
	}
}

// processDeferredBuildParallel processes deferred instances from all workers.
func (p *Parser) processDeferredBuildParallel(
	scanResult *ScanResult,
	outBuilder, inBuilder *CompactEdgeListBuilder,
	deferred []deferredBuildInstance,
) {
	for _, d := range deferred {
		allFields := getClassHierarchyFieldsFromMaps(d.classID, scanResult.ClassInfo, scanResult.ClassFields)
		if len(allFields) == 0 {
			continue
		}

		idSize := scanResult.Header.IDSize
		srcIdx := scanResult.ObjectIndex.GetIndex(d.objectID)
		if srcIdx < 0 {
			continue
		}

		offset := 0
		for _, field := range allFields {
			fieldSize := BasicTypeSize(field.Type, idSize)
			if offset+fieldSize > len(d.data) {
				break
			}
			if field.Type == TypeObject {
				refID := readObjectID(d.data[offset:], idSize)
				if refID != 0 {
					tgtIdx := scanResult.ObjectIndex.GetIndex(refID)
					if tgtIdx >= 0 {
						fieldName := scanResult.Strings[field.NameID]
						outBuilder.AddEdge(srcIdx, tgtIdx, fieldName, d.classID)
						inBuilder.AddEdge(tgtIdx, srcIdx, fieldName, d.classID)
					}
				}
			}
			offset += fieldSize
		}
	}
}

// mergeWorkerEdges efficiently merges all worker edge lists into unified builders.
// Uses pre-computed total size to avoid repeated slice growth.
func mergeWorkerEdges(results []workerResult, nodeCount, estimatedEdges int) (*CompactEdgeListBuilder, *CompactEdgeListBuilder) {
	// Calculate total edges across all workers
	totalOutEdges := 0
	totalInEdges := 0
	for _, r := range results {
		totalOutEdges += len(r.outEdges)
		totalInEdges += len(r.inEdges)
	}

	outBuilder := NewCompactEdgeListBuilder(nodeCount, totalOutEdges)
	inBuilder := NewCompactEdgeListBuilder(nodeCount, totalInEdges)

	// Batch merge - directly append edges with field name interning
	for _, r := range results {
		for i := range r.outEdges {
			e := &r.outEdges[i]
			outBuilder.AddEdge(e.from, e.to, e.fieldName, e.classID)
		}
		for i := range r.inEdges {
			e := &r.inEdges[i]
			inBuilder.AddEdge(e.from, e.to, e.fieldName, e.classID)
		}
	}

	return outBuilder, inBuilder
}

// distributeSegments divides segments among workers in contiguous chunks for IO locality.
func distributeSegments(segments []SegmentInfo, numWorkers int) [][]SegmentInfo {
	assignments := make([][]SegmentInfo, numWorkers)
	if len(segments) == 0 {
		return assignments
	}

	// Distribute by total bytes for balanced workload
	totalBytes := int64(0)
	for _, s := range segments {
		totalBytes += int64(s.Length)
	}

	targetBytesPerWorker := totalBytes / int64(numWorkers)
	workerIdx := 0
	currentBytes := int64(0)

	for _, seg := range segments {
		assignments[workerIdx] = append(assignments[workerIdx], seg)
		currentBytes += int64(seg.Length)

		// Move to next worker if we've exceeded the target (but don't exceed numWorkers)
		if currentBytes >= targetBytesPerWorker && workerIdx < numWorkers-1 {
			workerIdx++
			currentBytes = 0
		}
	}

	return assignments
}


