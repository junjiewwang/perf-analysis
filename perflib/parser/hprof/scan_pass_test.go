package hprof

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanPass_BasicMetadata(t *testing.T) {
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

			result, err := parser.ScanPass(ctx, reader)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify header
			assert.Equal(t, "JAVA PROFILE 1.0.2", result.Header.Format)
			assert.Equal(t, tt.idSize, result.Header.IDSize)

			// Verify string table
			assert.Equal(t, "java/lang/Class", result.Strings[1])
			assert.Equal(t, "com/test/ClassA", result.Strings[2])
			assert.Equal(t, "com/test/ClassB", result.Strings[3])
			assert.Equal(t, "refB", result.Strings[5])
			assert.Equal(t, "intField", result.Strings[6])
			assert.Equal(t, "refC", result.Strings[7])

			// Verify class info
			assert.NotNil(t, result.ClassInfo[0x200]) // ClassA
			assert.Equal(t, "com.test.ClassA", result.ClassInfo[0x200].Name)
			assert.Equal(t, tt.idSize+4, result.ClassInfo[0x200].InstanceSize) // refB (idSize) + intField (4)

			assert.NotNil(t, result.ClassInfo[0x300]) // ClassB
			assert.Equal(t, "com.test.ClassB", result.ClassInfo[0x300].Name)
			assert.Equal(t, tt.idSize, result.ClassInfo[0x300].InstanceSize) // refC (idSize)

			assert.NotNil(t, result.ClassInfo[0x400]) // ClassC
			assert.Equal(t, "com.test.ClassC", result.ClassInfo[0x400].Name)

			// Verify class fields
			assert.Len(t, result.ClassFields[0x200], 2) // ClassA: refB + intField
			assert.Len(t, result.ClassFields[0x300], 1) // ClassB: refC
			assert.Len(t, result.ClassFields[0x400], 0) // ClassC: none

			t.Logf("Objects: %d, Edges: %d, GCRoots: %d, Classes: %d",
				result.ObjectCount, result.EdgeCount, len(result.GCRoots), result.TotalClasses)
		})
	}
}

func TestScanPass_ObjectCounting(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	result, err := parser.ScanPass(ctx, reader)
	require.NoError(t, err)

	// Expected objects:
	// - Class objects: 0x100, 0x200, 0x300, 0x400, 0x500 (5 classes)
	// - Instances: 0x1001, 0x1002, 0x2001, 0x2002, 0x3001 (5 instances)
	// - Object array: 0x4001 (1 array)
	// - Primitive array: 0x5001 (1 array)
	// Total: 12
	assert.Equal(t, int32(12), result.ObjectCount)

	// Verify each object is in the index
	assert.GreaterOrEqual(t, result.ObjectIndex.GetIndex(0x1001), int32(0)) // objA1
	assert.GreaterOrEqual(t, result.ObjectIndex.GetIndex(0x1002), int32(0)) // objA2
	assert.GreaterOrEqual(t, result.ObjectIndex.GetIndex(0x2001), int32(0)) // objB1
	assert.GreaterOrEqual(t, result.ObjectIndex.GetIndex(0x2002), int32(0)) // objB2
	assert.GreaterOrEqual(t, result.ObjectIndex.GetIndex(0x3001), int32(0)) // objC1
	assert.GreaterOrEqual(t, result.ObjectIndex.GetIndex(0x4001), int32(0)) // arr1
	assert.GreaterOrEqual(t, result.ObjectIndex.GetIndex(0x5001), int32(0)) // byteArr1
	assert.Equal(t, int32(-1), result.ObjectIndex.GetIndex(0x9999))         // non-existent
}

func TestScanPass_EdgeCounting(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	result, err := parser.ScanPass(ctx, reader)
	require.NoError(t, err)

	// Expected edges:
	// - objA1.refB → objB1 (1 edge)
	// - objA2.refB → objB2 (1 edge)
	// - objB1.refC → objC1 (1 edge)
	// - objB2.refC → objC1 (1 edge)
	// - arr1[0] → objA1 (1 edge)
	// - arr1[1] → objA2 (1 edge)
	// Total: 6 edges
	assert.Equal(t, int64(6), result.EdgeCount)
}

func TestScanPass_GCRoots(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	result, err := parser.ScanPass(ctx, reader)
	require.NoError(t, err)

	// Verify GC roots
	assert.Len(t, result.GCRoots, 2)

	// Find roots by type
	var jniRoot, stickyRoot *GCRootEntry
	for i := range result.GCRoots {
		switch result.GCRoots[i].RootType {
		case GCRootJNIGlobal:
			jniRoot = &result.GCRoots[i]
		case GCRootStickyClass:
			stickyRoot = &result.GCRoots[i]
		}
	}

	require.NotNil(t, jniRoot)
	assert.Equal(t, uint64(0x1001), jniRoot.ObjectID)

	require.NotNil(t, stickyRoot)
	assert.Equal(t, uint64(0x4001), stickyRoot.ObjectID)
}

func TestScanPass_DegreeCounts(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	result, err := parser.ScanPass(ctx, reader)
	require.NoError(t, err)

	// Extract degree counts
	degrees := result.ExtractDegreeCounts()
	require.NotNil(t, degrees)
	assert.Len(t, degrees, int(result.ObjectCount))

	// Check specific degrees
	// objA1 (has 1 object ref: refB)
	idxA1 := result.ObjectIndex.GetIndex(0x1001)
	assert.Equal(t, int32(1), degrees[idxA1])

	// objA2 (has 1 object ref: refB)
	idxA2 := result.ObjectIndex.GetIndex(0x1002)
	assert.Equal(t, int32(1), degrees[idxA2])

	// objB1 (has 1 object ref: refC)
	idxB1 := result.ObjectIndex.GetIndex(0x2001)
	assert.Equal(t, int32(1), degrees[idxB1])

	// objC1 (has 0 refs)
	idxC1 := result.ObjectIndex.GetIndex(0x3001)
	assert.Equal(t, int32(0), degrees[idxC1])

	// arr1 (has 2 element refs)
	idxArr := result.ObjectIndex.GetIndex(0x4001)
	assert.Equal(t, int32(2), degrees[idxArr])

	// byteArr1 (primitive array, 0 refs)
	idxPrim := result.ObjectIndex.GetIndex(0x5001)
	assert.Equal(t, int32(0), degrees[idxPrim])
}

func TestScanPass_ContextCancellation(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := parser.ScanPass(ctx, reader)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestScanPass_InstanceCount(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	result, err := parser.ScanPass(ctx, reader)
	require.NoError(t, err)

	// ClassA should have 2 instances (objA1, objA2)
	classA := result.ClassInfo[0x200]
	require.NotNil(t, classA)
	assert.Equal(t, int64(2), classA.InstanceCount)

	// ClassB should have 2 instances (objB1, objB2)
	classB := result.ClassInfo[0x300]
	require.NotNil(t, classB)
	assert.Equal(t, int64(2), classB.InstanceCount)

	// ClassC should have 1 instance (objC1)
	classC := result.ClassInfo[0x400]
	require.NotNil(t, classC)
	assert.Equal(t, int64(1), classC.InstanceCount)
}

func TestScanPass_ExtractDegreeCounts_ResetsRetainedSize(t *testing.T) {
	reader := buildSimpleHprofData(8)
	parser := NewParser(nil)
	ctx := context.Background()

	result, err := parser.ScanPass(ctx, reader)
	require.NoError(t, err)

	// After ExtractDegreeCounts, retained sizes should be reset to shallow sizes
	result.ExtractDegreeCounts()

	idxA1 := result.ObjectIndex.GetIndex(0x1001)
	shallowA1 := result.ObjectIndex.GetShallowSize(idxA1)
	retainedA1 := result.ObjectIndex.GetRetainedSize(idxA1)
	assert.Equal(t, shallowA1, retainedA1, "retained size should be reset to shallow size")
}
