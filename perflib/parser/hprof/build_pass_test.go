package hprof

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPass_BasicEdges(t *testing.T) {
	tests := []struct {
		name   string
		idSize int
	}{
		{"4-byte IDs", 4},
		{"8-byte IDs", 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := buildSimpleHprofData(tt.idSize)
			parser := NewParser(nil)
			ctx := context.Background()

			graph, scanResult, err := parser.ParseTwoPass(ctx, reader)
			require.NoError(t, err)
			require.NotNil(t, graph)
			require.NotNil(t, scanResult)

			// Verify graph has the expected number of objects
			assert.Equal(t, scanResult.ObjectCount, graph.ObjectCount())

			// Verify outgoing edges for objA1 (should reference objB1)
			idxA1 := graph.GetObjectIndex(0x1001)
			require.GreaterOrEqual(t, idxA1, int32(0))
			targets, fieldIDs, _ := graph.GetOutgoingEdges(idxA1)
			assert.Len(t, targets, 1)
			assert.Equal(t, graph.GetObjectIndex(0x2001), targets[0]) // objA1 → objB1
			// Check field name
			outgoing := graph.GetOutgoing()
			require.NotNil(t, outgoing)
			assert.Equal(t, "refB", outgoing.GetFieldName(fieldIDs[0]))

			// Verify outgoing edges for objB1 (should reference objC1)
			idxB1 := graph.GetObjectIndex(0x2001)
			targets, fieldIDs, _ = graph.GetOutgoingEdges(idxB1)
			assert.Len(t, targets, 1)
			assert.Equal(t, graph.GetObjectIndex(0x3001), targets[0]) // objB1 → objC1
			assert.Equal(t, "refC", outgoing.GetFieldName(fieldIDs[0]))

			// Verify outgoing edges for arr1 (should reference objA1, objA2)
			idxArr := graph.GetObjectIndex(0x4001)
			targets, _, _ = graph.GetOutgoingEdges(idxArr)
			assert.Len(t, targets, 2)
			// Array elements should be objA1 and objA2
			targetSet := map[int32]bool{targets[0]: true, targets[1]: true}
			assert.True(t, targetSet[graph.GetObjectIndex(0x1001)]) // objA1
			assert.True(t, targetSet[graph.GetObjectIndex(0x1002)]) // objA2

			// Verify objC1 has no outgoing edges
			idxC1 := graph.GetObjectIndex(0x3001)
			targets, _, _ = graph.GetOutgoingEdges(idxC1)
			assert.Len(t, targets, 0)
		})
	}
}

func TestBuildPass_IncomingEdges(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	graph, _, err := parser.ParseTwoPass(ctx, reader)
	require.NoError(t, err)

	// objC1 should have 2 incoming edges (from objB1.refC and objB2.refC)
	idxC1 := graph.GetObjectIndex(0x3001)
	sources, fieldIDs, _ := graph.GetIncomingEdges(idxC1)
	assert.Len(t, sources, 2)

	incoming := graph.GetIncoming()
	require.NotNil(t, incoming)

	// Both incoming edges should have fieldName "refC"
	for _, fid := range fieldIDs {
		assert.Equal(t, "refC", incoming.GetFieldName(fid))
	}

	// Sources should be objB1 and objB2
	sourceSet := map[int32]bool{sources[0]: true, sources[1]: true}
	assert.True(t, sourceSet[graph.GetObjectIndex(0x2001)]) // from objB1
	assert.True(t, sourceSet[graph.GetObjectIndex(0x2002)]) // from objB2

	// objB1 should have 1 incoming edge (from objA1.refB)
	idxB1 := graph.GetObjectIndex(0x2001)
	sources, _, _ = graph.GetIncomingEdges(idxB1)
	assert.Len(t, sources, 1)
	assert.Equal(t, graph.GetObjectIndex(0x1001), sources[0])
}

func TestBuildPass_GCRootBitset(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	graph, _, err := parser.ParseTwoPass(ctx, reader)
	require.NoError(t, err)

	// objA1 should be a GC root
	idxA1 := graph.GetObjectIndex(0x1001)
	assert.True(t, graph.IsGCRoot(idxA1))

	// arr1 should be a GC root
	idxArr := graph.GetObjectIndex(0x4001)
	assert.True(t, graph.IsGCRoot(idxArr))

	// objB1 should NOT be a GC root
	idxB1 := graph.GetObjectIndex(0x2001)
	assert.False(t, graph.IsGCRoot(idxB1))

	// objC1 should NOT be a GC root
	idxC1 := graph.GetObjectIndex(0x3001)
	assert.False(t, graph.IsGCRoot(idxC1))
}

func TestBuildPass_Reachability(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	graph, _, err := parser.ParseTwoPass(ctx, reader)
	require.NoError(t, err)

	// All objects reachable from GC roots should be marked
	// GC roots: objA1 (→ objB1 → objC1), arr1 (→ objA1, objA2 → objB2 → objC1)
	assert.True(t, graph.IsReachable(graph.GetObjectIndex(0x1001))) // objA1 (root)
	assert.True(t, graph.IsReachable(graph.GetObjectIndex(0x1002))) // objA2 (reachable via arr1)
	assert.True(t, graph.IsReachable(graph.GetObjectIndex(0x2001))) // objB1 (reachable via objA1)
	assert.True(t, graph.IsReachable(graph.GetObjectIndex(0x2002))) // objB2 (reachable via objA2)
	assert.True(t, graph.IsReachable(graph.GetObjectIndex(0x3001))) // objC1 (reachable via objB1/B2)
	assert.True(t, graph.IsReachable(graph.GetObjectIndex(0x4001))) // arr1 (root)

	// Primitive array byteArr1 is NOT a GC root and not referenced, should be unreachable
	idxPrim := graph.GetObjectIndex(0x5001)
	assert.False(t, graph.IsReachable(idxPrim))
}

func TestBuildPass_ClassNames(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	graph, _, err := parser.ParseTwoPass(ctx, reader)
	require.NoError(t, err)

	// Verify class name resolution
	classNames := graph.GetClassNames()
	assert.Equal(t, "com.test.ClassA", classNames[0x200])
	assert.Equal(t, "com.test.ClassB", classNames[0x300])
	assert.Equal(t, "com.test.ClassC", classNames[0x400])
}

func TestBuildPass_ClassObjectBitset(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	graph, _, err := parser.ParseTwoPass(ctx, reader)
	require.NoError(t, err)

	// Class objects should be marked
	assert.True(t, graph.IsClassObject(graph.GetObjectIndex(0x100)))  // java.lang.Class
	assert.True(t, graph.IsClassObject(graph.GetObjectIndex(0x200)))  // ClassA
	assert.True(t, graph.IsClassObject(graph.GetObjectIndex(0x300)))  // ClassB
	assert.True(t, graph.IsClassObject(graph.GetObjectIndex(0x400)))  // ClassC

	// Instance objects should NOT be class objects
	assert.False(t, graph.IsClassObject(graph.GetObjectIndex(0x1001))) // objA1
	assert.False(t, graph.IsClassObject(graph.GetObjectIndex(0x2001))) // objB1
}

func TestBuildPass_EdgeCount(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	graph, _, err := parser.ParseTwoPass(ctx, reader)
	require.NoError(t, err)

	outgoing := graph.GetOutgoing()
	require.NotNil(t, outgoing)

	// Total outgoing edges should be 6 (same as scan pass edge count)
	// objA1→objB1, objA2→objB2, objB1→objC1, objB2→objC1, arr1→objA1, arr1→objA2
	assert.Equal(t, int32(6), outgoing.TotalEdges())

	// Total incoming edges should also be 6
	incoming := graph.GetIncoming()
	require.NotNil(t, incoming)
	assert.Equal(t, int32(6), incoming.TotalEdges())
}

func TestBuildPass_ContextCancellation(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := parser.ParseTwoPass(ctx, reader)
	assert.Error(t, err)
}

func TestParseTwoPass_Integration_RealFile(t *testing.T) {
	// Integration test with real heap dump file
	testFile := "../../../test/heap-1.hprof"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("Test HPROF file not found (test/heap-1.hprof), skipping integration test")
	}

	file, err := os.Open(testFile)
	require.NoError(t, err)
	defer file.Close()

	parser := NewParser(nil)
	ctx := context.Background()

	graph, scanResult, err := parser.ParseTwoPass(ctx, file)
	require.NoError(t, err)
	require.NotNil(t, graph)
	require.NotNil(t, scanResult)

	// Basic sanity checks
	assert.Greater(t, graph.ObjectCount(), int32(0))
	assert.Greater(t, scanResult.EdgeCount, int64(0))
	assert.Greater(t, len(scanResult.GCRoots), 0)
	assert.Greater(t, scanResult.TotalClasses, 0)

	// Verify outgoing edge list was built
	outgoing := graph.GetOutgoing()
	require.NotNil(t, outgoing)
	assert.Greater(t, outgoing.TotalEdges(), int32(0))

	// Verify incoming edge list was built
	incoming := graph.GetIncoming()
	require.NotNil(t, incoming)
	assert.Equal(t, outgoing.TotalEdges(), incoming.TotalEdges())

	// Verify reachability was computed
	reachableCount := 0
	for i := int32(0); i < graph.ObjectCount(); i++ {
		if graph.IsReachable(i) {
			reachableCount++
		}
	}
	assert.Greater(t, reachableCount, 0)

	t.Logf("ParseTwoPass Integration Results:")
	t.Logf("  Objects:        %d", graph.ObjectCount())
	t.Logf("  Edges:          %d", outgoing.TotalEdges())
	t.Logf("  GC Roots:       %d", len(scanResult.GCRoots))
	t.Logf("  Classes:        %d", scanResult.TotalClasses)
	t.Logf("  Reachable:      %d (%.1f%%)", reachableCount, float64(reachableCount)*100/float64(graph.ObjectCount()))
	t.Logf("  Total Heap:     %.2f MB", float64(scanResult.TotalHeapSize)/(1024*1024))
}
