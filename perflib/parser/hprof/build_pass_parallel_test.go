package hprof

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildMultiSegmentHprofData creates HPROF data with multiple HEAP_DUMP_SEGMENT records
// to test the parallel Build Pass path. The graph structure is the same as buildSimpleHprofData
// but spread across multiple segments (≥ parallelBuildThreshold).
//
// Segment 1: GC roots + CLASS_DUMPs
// Segment 2: INSTANCE_DUMPs for ClassA (objA1, objA2)
// Segment 3: INSTANCE_DUMPs for ClassB (objB1, objB2)
// Segment 4: INSTANCE_DUMPs for ClassC (objC1) + OBJECT_ARRAY_DUMP (arr1)
// Segment 5: PRIMITIVE_ARRAY_DUMP (byteArr1)
func buildMultiSegmentHprofData(idSize int) *bytes.Reader {
	b := newHprofTestBuilder(idSize)

	// String records (must be before heap dump)
	b.addStringRecord(1, "java/lang/Class")
	b.addStringRecord(2, "com/test/ClassA")
	b.addStringRecord(3, "com/test/ClassB")
	b.addStringRecord(4, "com/test/ClassC")
	b.addStringRecord(5, "refB")
	b.addStringRecord(6, "intField")
	b.addStringRecord(7, "refC")
	b.addStringRecord(8, "[Lcom/test/ClassA;")

	// Load class records
	b.addLoadClassRecord(1, 0x100, 0, 1)
	b.addLoadClassRecord(2, 0x200, 0, 2)
	b.addLoadClassRecord(3, 0x300, 0, 3)
	b.addLoadClassRecord(4, 0x400, 0, 4)
	b.addLoadClassRecord(5, 0x500, 0, 8)

	// Segment 1: GC roots + CLASS_DUMPs
	seg1 := newHeapDumpBuilder(idSize)
	seg1.addGCRootJNIGlobal(0x1001, 0)
	seg1.addGCRootStickyClass(0x4001)
	seg1.addClassDump(0x100, 0, 0, nil, nil)
	seg1.addClassDump(0x200, 0, uint32(idSize+4), nil, []testInstanceField{
		{nameID: 5, fieldType: TypeObject},
		{nameID: 6, fieldType: TypeInt},
	})
	seg1.addClassDump(0x300, 0, uint32(idSize), nil, []testInstanceField{
		{nameID: 7, fieldType: TypeObject},
	})
	seg1.addClassDump(0x400, 0, 0, nil, nil)
	seg1.addClassDump(0x500, 0, 0, nil, nil)
	b.addHeapDumpSegment(seg1.bytes())

	// Segment 2: ClassA instances
	seg2 := newHeapDumpBuilder(idSize)
	instanceDataA1 := append(makeObjectRefBytes(0x2001, idSize), makeIntBytes(42)...)
	seg2.addInstanceDump(0x1001, 0x200, instanceDataA1)
	instanceDataA2 := append(makeObjectRefBytes(0x2002, idSize), makeIntBytes(99)...)
	seg2.addInstanceDump(0x1002, 0x200, instanceDataA2)
	b.addHeapDumpSegment(seg2.bytes())

	// Segment 3: ClassB instances
	seg3 := newHeapDumpBuilder(idSize)
	seg3.addInstanceDump(0x2001, 0x300, makeObjectRefBytes(0x3001, idSize))
	seg3.addInstanceDump(0x2002, 0x300, makeObjectRefBytes(0x3001, idSize))
	b.addHeapDumpSegment(seg3.bytes())

	// Segment 4: ClassC instance + Object array
	seg4 := newHeapDumpBuilder(idSize)
	seg4.addInstanceDump(0x3001, 0x400, nil)
	seg4.addObjectArrayDump(0x4001, 0x500, []uint64{0x1001, 0x1002})
	b.addHeapDumpSegment(seg4.bytes())

	// Segment 5: Primitive array
	seg5 := newHeapDumpBuilder(idSize)
	seg5.addPrimitiveArrayDump(0x5001, TypeByte, 16)
	b.addHeapDumpSegment(seg5.bytes())

	b.addHeapDumpEnd()

	return b.reader()
}

func TestBuildPassParallel_BasicEdges(t *testing.T) {
	tests := []struct {
		name   string
		idSize int
	}{
		{"4-byte IDs", 4},
		{"8-byte IDs", 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := buildMultiSegmentHprofData(tt.idSize)
			parser := NewParser(nil)
			ctx := context.Background()

			// Verify we have enough segments for parallel path
			scanResult, err := parser.ScanPass(ctx, reader)
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(scanResult.SegmentOffsets), parallelBuildThreshold,
				"test data should have >= %d segments to trigger parallel path", parallelBuildThreshold)

			// Run parallel build pass
			graph, err := parser.BuildPassParallel(ctx, reader, scanResult)
			require.NoError(t, err)
			require.NotNil(t, graph)

			// Verify same results as sequential path
			assert.Equal(t, scanResult.ObjectCount, graph.ObjectCount())

			// Verify outgoing edges for objA1 (should reference objB1)
			idxA1 := graph.GetObjectIndex(0x1001)
			require.GreaterOrEqual(t, idxA1, int32(0))
			targets, fieldIDs, _ := graph.GetOutgoingEdges(idxA1)
			assert.Len(t, targets, 1)
			assert.Equal(t, graph.GetObjectIndex(0x2001), targets[0])
			outgoing := graph.GetOutgoing()
			require.NotNil(t, outgoing)
			assert.Equal(t, "refB", outgoing.GetFieldName(fieldIDs[0]))

			// Verify objB1 → objC1
			idxB1 := graph.GetObjectIndex(0x2001)
			targets, fieldIDs, _ = graph.GetOutgoingEdges(idxB1)
			assert.Len(t, targets, 1)
			assert.Equal(t, graph.GetObjectIndex(0x3001), targets[0])
			assert.Equal(t, "refC", outgoing.GetFieldName(fieldIDs[0]))

			// Verify arr1 → [objA1, objA2]
			idxArr := graph.GetObjectIndex(0x4001)
			targets, _, _ = graph.GetOutgoingEdges(idxArr)
			assert.Len(t, targets, 2)
			targetSet := map[int32]bool{targets[0]: true, targets[1]: true}
			assert.True(t, targetSet[graph.GetObjectIndex(0x1001)])
			assert.True(t, targetSet[graph.GetObjectIndex(0x1002)])

			// Verify objC1 has no outgoing edges
			idxC1 := graph.GetObjectIndex(0x3001)
			targets, _, _ = graph.GetOutgoingEdges(idxC1)
			assert.Len(t, targets, 0)
		})
	}
}

func TestBuildPassParallel_IncomingEdges(t *testing.T) {
	reader := buildMultiSegmentHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	scanResult, err := parser.ScanPass(ctx, reader)
	require.NoError(t, err)

	graph, err := parser.BuildPassParallel(ctx, reader, scanResult)
	require.NoError(t, err)

	// objC1 should have 2 incoming edges (from objB1.refC and objB2.refC)
	idxC1 := graph.GetObjectIndex(0x3001)
	sources, fieldIDs, _ := graph.GetIncomingEdges(idxC1)
	assert.Len(t, sources, 2)

	incoming := graph.GetIncoming()
	require.NotNil(t, incoming)
	for _, fid := range fieldIDs {
		assert.Equal(t, "refC", incoming.GetFieldName(fid))
	}

	sourceSet := map[int32]bool{sources[0]: true, sources[1]: true}
	assert.True(t, sourceSet[graph.GetObjectIndex(0x2001)])
	assert.True(t, sourceSet[graph.GetObjectIndex(0x2002)])
}

func TestBuildPassParallel_EdgeCount(t *testing.T) {
	reader := buildMultiSegmentHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	scanResult, err := parser.ScanPass(ctx, reader)
	require.NoError(t, err)

	graph, err := parser.BuildPassParallel(ctx, reader, scanResult)
	require.NoError(t, err)

	outgoing := graph.GetOutgoing()
	require.NotNil(t, outgoing)
	// Expected: objA1→objB1, objA2→objB2, objB1→objC1, objB2→objC1, arr1→objA1, arr1→objA2 = 6
	assert.Equal(t, int32(6), outgoing.TotalEdges())

	incoming := graph.GetIncoming()
	require.NotNil(t, incoming)
	assert.Equal(t, int32(6), incoming.TotalEdges())
}

func TestBuildPassParallel_Reachability(t *testing.T) {
	reader := buildMultiSegmentHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	scanResult, err := parser.ScanPass(ctx, reader)
	require.NoError(t, err)

	graph, err := parser.BuildPassParallel(ctx, reader, scanResult)
	require.NoError(t, err)

	// All objects reachable from GC roots
	assert.True(t, graph.IsReachable(graph.GetObjectIndex(0x1001)))
	assert.True(t, graph.IsReachable(graph.GetObjectIndex(0x1002)))
	assert.True(t, graph.IsReachable(graph.GetObjectIndex(0x2001)))
	assert.True(t, graph.IsReachable(graph.GetObjectIndex(0x2002)))
	assert.True(t, graph.IsReachable(graph.GetObjectIndex(0x3001)))
	assert.True(t, graph.IsReachable(graph.GetObjectIndex(0x4001)))

	// Primitive array is NOT reachable
	idxPrim := graph.GetObjectIndex(0x5001)
	assert.False(t, graph.IsReachable(idxPrim))
}

func TestBuildPassParallel_VsSequential(t *testing.T) {
	// Compare parallel and sequential results on the same data
	reader := buildMultiSegmentHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	// Run sequential
	scanResult1, err := parser.ScanPass(ctx, reader)
	require.NoError(t, err)
	seqGraph, err := parser.BuildPass(ctx, reader, scanResult1)
	require.NoError(t, err)

	// Run parallel (need to re-scan since BuildPass consumed the reader)
	reader = buildMultiSegmentHprofData(8)
	scanResult2, err := parser.ScanPass(ctx, reader)
	require.NoError(t, err)
	parGraph, err := parser.BuildPassParallel(ctx, reader, scanResult2)
	require.NoError(t, err)

	// Compare results
	assert.Equal(t, seqGraph.ObjectCount(), parGraph.ObjectCount())
	assert.Equal(t, seqGraph.GetOutgoing().TotalEdges(), parGraph.GetOutgoing().TotalEdges())
	assert.Equal(t, seqGraph.GetIncoming().TotalEdges(), parGraph.GetIncoming().TotalEdges())

	// Compare edge-by-edge for each node
	for i := int32(0); i < seqGraph.ObjectCount(); i++ {
		seqTargets, _, _ := seqGraph.GetOutgoingEdges(i)
		parTargets, _, _ := parGraph.GetOutgoingEdges(i)
		assert.Equal(t, len(seqTargets), len(parTargets), "node %d outgoing edge count mismatch", i)
	}
}

func TestBuildPassParallel_ParseTwoPassAutoDetect(t *testing.T) {
	// Verify ParseTwoPass automatically uses parallel path for multi-segment data
	reader := buildMultiSegmentHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	graph, scanResult, err := parser.ParseTwoPass(ctx, reader)
	require.NoError(t, err)
	require.NotNil(t, graph)

	// Should have used parallel path (bytes.Reader implements io.ReaderAt and we have 5 segments)
	assert.GreaterOrEqual(t, len(scanResult.SegmentOffsets), parallelBuildThreshold)
	assert.Equal(t, int32(6), graph.GetOutgoing().TotalEdges())
}

func TestDistributeSegments(t *testing.T) {
	segments := []SegmentInfo{
		{Offset: 0, Length: 1000},
		{Offset: 1000, Length: 2000},
		{Offset: 3000, Length: 500},
		{Offset: 3500, Length: 1500},
		{Offset: 5000, Length: 1000},
	}

	assignments := distributeSegments(segments, 3)

	// All segments should be assigned
	totalAssigned := 0
	for _, a := range assignments {
		totalAssigned += len(a)
	}
	assert.Equal(t, len(segments), totalAssigned)

	// No empty workers (with 5 segments and 3 workers, each should get at least 1)
	for i, a := range assignments {
		assert.Greater(t, len(a), 0, "worker %d should have segments", i)
	}
}
