package hprof

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIndexRoundtrip tests write→read roundtrip preserves all data.
func TestIndexRoundtrip(t *testing.T) {
	// Build a test graph
	graph := buildTestGraph(t)

	// Write to temp file
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "heap_index.bin")

	err := WriteHeapIndex(indexPath, graph)
	require.NoError(t, err, "WriteHeapIndex should succeed")

	// Check file exists and has reasonable size
	info, err := os.Stat(indexPath)
	require.NoError(t, err)
	assert.True(t, info.Size() > 0, "index file should not be empty")
	t.Logf("Index file size: %d bytes", info.Size())

	// Read back (v1 format)
	loaded, err := ReadHeapIndex(indexPath)
	require.NoError(t, err, "ReadHeapIndex should succeed")

	// Verify via HeapGraph interface
	loadedGraph, ok := loaded.(*IndexedReferenceGraph)
	require.True(t, ok, "v1 file should return *IndexedReferenceGraph")
	verifyGraphEquality(t, graph, loadedGraph)
}

// TestIndexV2Roundtrip tests v2 format write→read roundtrip.
func TestIndexV2Roundtrip(t *testing.T) {
	graph := buildTestGraph(t)

	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "heap_index_v2.bin")

	// Write v2 format
	err := WriteHeapIndexV2(indexPath, graph)
	require.NoError(t, err, "WriteHeapIndexV2 should succeed")

	info, err := os.Stat(indexPath)
	require.NoError(t, err)
	t.Logf("V2 index file size: %d bytes", info.Size())

	// Read back (should detect v2 and use mmap)
	loaded, err := ReadHeapIndex(indexPath)
	require.NoError(t, err, "ReadHeapIndex v2 should succeed")

	mmapIndex, ok := loaded.(*MmapHeapIndex)
	require.True(t, ok, "v2 file should return *MmapHeapIndex")
	defer mmapIndex.Close()

	// Verify via HeapGraph interface
	verifyHeapGraphEquality(t, graph, loaded)
}

// TestIndexRoundtripInMemory tests roundtrip using in-memory buffers.
func TestIndexRoundtripInMemory(t *testing.T) {
	graph := buildTestGraph(t)

	// Write to buffer
	var buf bytes.Buffer
	err := writeHeapIndexTo(&buf, graph)
	require.NoError(t, err, "writeHeapIndexTo should succeed")

	t.Logf("Buffer size: %d bytes", buf.Len())

	// Read from buffer
	loaded, err := readHeapIndexFrom(&buf)
	require.NoError(t, err, "readHeapIndexFrom should succeed")

	verifyGraphEquality(t, graph, loaded)
}

// TestIndexEmptyGraph tests handling of an empty graph.
func TestIndexEmptyGraph(t *testing.T) {
	graph := NewIndexedReferenceGraph(0)
	graph.FinalizeObjects()
	outBuilder := NewCompactEdgeListBuilder(0, 0)
	inBuilder := NewCompactEdgeListBuilder(0, 0)
	graph.BuildEdges(outBuilder, inBuilder)

	var buf bytes.Buffer
	err := writeHeapIndexTo(&buf, graph)
	require.NoError(t, err)

	loaded, err := readHeapIndexFrom(&buf)
	require.NoError(t, err)

	assert.Equal(t, int32(0), loaded.ObjectCount())
}

// TestIndexInvalidMagic tests detection of invalid file format.
func TestIndexInvalidMagic(t *testing.T) {
	data := []byte("NOT_A_VALID_INDEX_FILE_HEADER_PADDING_TO_40_BYTES__")
	_, err := readHeapIndexFrom(bytes.NewReader(data))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid magic")
}

// TestIndexVersionMismatch tests detection of unsupported format version.
func TestIndexVersionMismatch(t *testing.T) {
	header := IndexFileHeader{
		Magic:   IndexFileMagic,
		Version: 99, // unsupported version
	}
	var buf bytes.Buffer
	_ = writeHeaderForTest(&buf, &header)

	_, err := readHeapIndexFrom(&buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported version")
}

// TestIndexLargeGraph tests with a larger graph for performance validation.
func TestIndexLargeGraph(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large graph test in short mode")
	}

	const objectCount = 100000
	const edgesPerObject = 3

	graph := NewIndexedReferenceGraph(objectCount)

	// Add objects
	for i := 0; i < objectCount; i++ {
		objID := uint64(0x1000 + i)
		classID := uint64(100 + i%50) // 50 different classes
		graph.AddObject(objID, classID, int64(16+i%256))
	}

	// Set class names
	for i := 0; i < 50; i++ {
		graph.SetClassName(uint64(100+i), classNameForTest(i))
	}

	// Add GC roots (first 100 objects)
	for i := 0; i < 100; i++ {
		graph.AddGCRoot(GCRoot{
			ObjectID: uint64(0x1000 + i),
			Type:     GCRootJNIGlobal,
		})
	}

	graph.FinalizeObjects()

	// Build edges
	outBuilder := NewCompactEdgeListBuilder(objectCount, objectCount*edgesPerObject)
	inBuilder := NewCompactEdgeListBuilder(objectCount, objectCount*edgesPerObject)

	for i := 0; i < objectCount; i++ {
		for j := 0; j < edgesPerObject; j++ {
			target := (i + j + 1) % objectCount
			fieldName := fieldNameForTest(j)
			classID := uint64(100 + i%50)
			outBuilder.AddEdge(int32(i), int32(target), fieldName, classID)
			inBuilder.AddEdge(int32(target), int32(i), fieldName, classID)
		}
	}

	graph.BuildEdges(outBuilder, inBuilder)

	// Set some dominator values
	for i := 1; i < objectCount; i++ {
		graph.SetDominator(int32(i), int32(i/2))
	}

	// Mark some objects as reachable
	for i := 0; i < objectCount; i++ {
		if i%2 == 0 {
			graph.MarkReachable(int32(i))
		}
	}

	// Write and read
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "heap_index_large.bin")

	err := WriteHeapIndex(indexPath, graph)
	require.NoError(t, err)

	info, _ := os.Stat(indexPath)
	t.Logf("Large graph: %d objects, %d edges/obj, file size: %d bytes (%.2f MB)",
		objectCount, edgesPerObject, info.Size(), float64(info.Size())/(1024*1024))

	loaded, err := ReadHeapIndex(indexPath)
	require.NoError(t, err)

	// Verify key properties
	assert.Equal(t, graph.ObjectCount(), loaded.ObjectCount())

	// Verify a sample of objects
	for i := 0; i < 100; i++ {
		idx := int32(i * (objectCount / 100))
		assert.Equal(t, graph.GetObjectID(idx), loaded.GetObjectID(idx), "objectID mismatch at idx %d", idx)
		assert.Equal(t, graph.GetClassID(idx), loaded.GetClassID(idx), "classID mismatch at idx %d", idx)
		assert.Equal(t, graph.GetShallowSize(idx), loaded.GetShallowSize(idx), "shallowSize mismatch at idx %d", idx)
		assert.Equal(t, graph.GetRetainedSize(idx), loaded.GetRetainedSize(idx), "retainedSize mismatch at idx %d", idx)
		assert.Equal(t, graph.GetDominator(idx), loaded.GetDominator(idx), "dominator mismatch at idx %d", idx)
	}

	// Verify edges for a sample
	for i := 0; i < 10; i++ {
		idx := int32(i * 1000)
		origTargets, origFields, origClassIDs := graph.GetOutgoingEdges(idx)
		loadTargets, loadFields, loadClassIDs := loaded.GetOutgoingEdges(idx)
		assert.Equal(t, origTargets, loadTargets, "outgoing targets mismatch at idx %d", idx)
		assert.Equal(t, origFields, loadFields, "outgoing fieldIDs mismatch at idx %d", idx)
		assert.Equal(t, origClassIDs, loadClassIDs, "outgoing classIDs mismatch at idx %d", idx)
	}
}

// ============================================================================
// Test Helpers
// ============================================================================

// buildTestGraph creates a small but complete test graph for roundtrip testing.
func buildTestGraph(t *testing.T) *IndexedReferenceGraph {
	t.Helper()

	graph := NewIndexedReferenceGraph(10)

	// Add objects (10 objects, 3 classes)
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 32},
		{0x1002, 100, 48},
		{0x1003, 101, 64},
		{0x1004, 101, 80},
		{0x1005, 102, 16},
		{0x1006, 100, 24},
		{0x1007, 102, 96},
		{0x1008, 101, 128},
		{0x1009, 100, 256},
		{0x100A, 102, 512},
	}

	for _, obj := range objects {
		graph.AddObject(obj.objID, obj.classID, obj.size)
	}

	// Set class names
	graph.SetClassName(100, "java.lang.String")
	graph.SetClassName(101, "java.util.ArrayList")
	graph.SetClassName(102, "com.example.MyClass")

	// Add GC roots
	graph.AddGCRoot(GCRoot{ObjectID: 0x1001, Type: GCRootJNIGlobal})
	graph.AddGCRoot(GCRoot{ObjectID: 0x1003, Type: GCRootJavaFrame, ThreadID: 42, FrameIndex: 3})
	graph.AddGCRoot(GCRoot{ObjectID: 0x1005, Type: GCRootStickyClass})

	graph.FinalizeObjects()

	// Build edges
	type edge struct {
		from      int32
		to        int32
		fieldName string
		classID   uint64
	}

	outEdges := []edge{
		{0, 1, "value", 100},
		{0, 2, "list", 101},
		{1, 3, "items", 101},
		{2, 4, "data", 102},
		{3, 5, "next", 100},
		{4, 6, "handler", 102},
		{5, 7, "buffer", 101},
		{6, 8, "name", 100},
		{7, 9, "instance", 102},
	}

	outBuilder := NewCompactEdgeListBuilder(10, 20)
	inBuilder := NewCompactEdgeListBuilder(10, 20)

	for _, e := range outEdges {
		outBuilder.AddEdge(e.from, e.to, e.fieldName, e.classID)
		inBuilder.AddEdge(e.to, e.from, e.fieldName, e.classID)
	}

	graph.BuildEdges(outBuilder, inBuilder)

	// Set retained sizes (different from shallow)
	graph.SetRetainedSize(0, 1024)
	graph.SetRetainedSize(1, 512)
	graph.SetRetainedSize(2, 256)
	graph.SetRetainedSize(3, 128)

	// Set dominators
	graph.SetDominator(1, 0)
	graph.SetDominator(2, 0)
	graph.SetDominator(3, 1)
	graph.SetDominator(4, 2)
	graph.SetDominator(5, 3)

	// Mark reachable
	for i := int32(0); i < 8; i++ {
		graph.MarkReachable(i)
	}

	return graph
}

// verifyGraphEquality checks that two graphs have identical data.
func verifyGraphEquality(t *testing.T, original, loaded *IndexedReferenceGraph) {
	t.Helper()

	// Verify object count
	assert.Equal(t, original.ObjectCount(), loaded.ObjectCount(), "object count mismatch")

	n := original.ObjectCount()

	// Verify all object data
	for i := int32(0); i < n; i++ {
		assert.Equal(t, original.GetObjectID(i), loaded.GetObjectID(i), "objectID mismatch at idx %d", i)
		assert.Equal(t, original.GetClassID(i), loaded.GetClassID(i), "classID mismatch at idx %d", i)
		assert.Equal(t, original.GetShallowSize(i), loaded.GetShallowSize(i), "shallowSize mismatch at idx %d", i)
		assert.Equal(t, original.GetRetainedSize(i), loaded.GetRetainedSize(i), "retainedSize mismatch at idx %d", i)
		assert.Equal(t, original.GetDominator(i), loaded.GetDominator(i), "dominator mismatch at idx %d", i)
	}

	// Verify objectID→index mapping
	for i := int32(0); i < n; i++ {
		objID := original.GetObjectID(i)
		assert.Equal(t, i, loaded.GetObjectIndex(objID), "objectIndex mismatch for objID 0x%x", objID)
	}

	// Verify class names
	origClassNames := original.GetClassNames()
	loadClassNames := loaded.GetClassNames()
	assert.Equal(t, len(origClassNames), len(loadClassNames), "class name count mismatch")
	for classID, name := range origClassNames {
		assert.Equal(t, name, loadClassNames[classID], "class name mismatch for classID %d", classID)
	}

	// Verify outgoing edges
	for i := int32(0); i < n; i++ {
		origTargets, origFields, origClassIDs := original.GetOutgoingEdges(i)
		loadTargets, loadFields, loadClassIDs := loaded.GetOutgoingEdges(i)
		assert.Equal(t, origTargets, loadTargets, "outgoing targets mismatch at idx %d", i)
		assert.Equal(t, origFields, loadFields, "outgoing fieldIDs mismatch at idx %d", i)
		assert.Equal(t, origClassIDs, loadClassIDs, "outgoing classIDs mismatch at idx %d", i)
	}

	// Verify incoming edges
	for i := int32(0); i < n; i++ {
		origSources, origFields, origClassIDs := original.GetIncomingEdges(i)
		loadSources, loadFields, loadClassIDs := loaded.GetIncomingEdges(i)
		assert.Equal(t, origSources, loadSources, "incoming sources mismatch at idx %d", i)
		assert.Equal(t, origFields, loadFields, "incoming fieldIDs mismatch at idx %d", i)
		assert.Equal(t, origClassIDs, loadClassIDs, "incoming classIDs mismatch at idx %d", i)
	}

	// Verify GC roots
	origRoots := original.GetGCRoots()
	loadRoots := loaded.GetGCRoots()
	assert.Equal(t, len(origRoots), len(loadRoots), "GC root count mismatch")
	for i, origRoot := range origRoots {
		assert.Equal(t, origRoot.ObjectID, loadRoots[i].ObjectID, "GC root objectID mismatch at %d", i)
		assert.Equal(t, origRoot.Type, loadRoots[i].Type, "GC root type mismatch at %d", i)
		assert.Equal(t, origRoot.ThreadID, loadRoots[i].ThreadID, "GC root threadID mismatch at %d", i)
		assert.Equal(t, origRoot.FrameIndex, loadRoots[i].FrameIndex, "GC root frameIndex mismatch at %d", i)
	}

	// Verify bitsets
	for i := int32(0); i < n; i++ {
		assert.Equal(t, original.IsGCRoot(i), loaded.IsGCRoot(i), "gcRoot bit mismatch at idx %d", i)
		assert.Equal(t, original.IsClassObject(i), loaded.IsClassObject(i), "classObject bit mismatch at idx %d", i)
		assert.Equal(t, original.IsReachable(i), loaded.IsReachable(i), "reachable bit mismatch at idx %d", i)
	}

	// Verify field names
	origOutgoing := original.GetOutgoing()
	loadOutgoing := loaded.GetOutgoing()
	if origOutgoing != nil && loadOutgoing != nil {
		for i := int32(0); i < n; i++ {
			_, origFieldIDs, _ := original.GetOutgoingEdges(i)
			for _, fid := range origFieldIDs {
				origName := origOutgoing.GetFieldName(fid)
				loadName := loadOutgoing.GetFieldName(fid)
				assert.Equal(t, origName, loadName, "field name mismatch for fieldID %d", fid)
			}
		}
	}
}

// verifyHeapGraphEquality checks that two HeapGraph implementations have identical data.
// This validates through the interface without requiring concrete type access.
func verifyHeapGraphEquality(t *testing.T, original, loaded HeapGraph) {
	t.Helper()

	// Verify object count
	n := original.ObjectCount()
	assert.Equal(t, n, loaded.ObjectCount(), "object count mismatch")

	// Verify all object data
	for i := int32(0); i < n; i++ {
		assert.Equal(t, original.GetObjectID(i), loaded.GetObjectID(i), "objectID mismatch at idx %d", i)
		assert.Equal(t, original.GetClassID(i), loaded.GetClassID(i), "classID mismatch at idx %d", i)
		assert.Equal(t, original.GetShallowSize(i), loaded.GetShallowSize(i), "shallowSize mismatch at idx %d", i)
		assert.Equal(t, original.GetRetainedSize(i), loaded.GetRetainedSize(i), "retainedSize mismatch at idx %d", i)
		assert.Equal(t, original.GetDominator(i), loaded.GetDominator(i), "dominator mismatch at idx %d", i)
	}

	// Verify objectID→index mapping
	for i := int32(0); i < n; i++ {
		objID := original.GetObjectID(i)
		assert.Equal(t, i, loaded.GetObjectIndex(objID), "objectIndex mismatch for objID 0x%x", objID)
	}

	// Verify class names via GetClassName
	for i := int32(0); i < n; i++ {
		classID := original.GetClassID(i)
		assert.Equal(t, original.GetClassName(classID), loaded.GetClassName(classID),
			"className mismatch for classID %d at idx %d", classID, i)
	}

	// Verify outgoing edges
	for i := int32(0); i < n; i++ {
		origTargets, origFields, origClassIDs := original.GetOutgoingEdges(i)
		loadTargets, loadFields, loadClassIDs := loaded.GetOutgoingEdges(i)
		assert.Equal(t, origTargets, loadTargets, "outgoing targets mismatch at idx %d", i)
		assert.Equal(t, origFields, loadFields, "outgoing fieldIDs mismatch at idx %d", i)
		assert.Equal(t, origClassIDs, loadClassIDs, "outgoing classIDs mismatch at idx %d", i)
	}

	// Verify incoming edges
	for i := int32(0); i < n; i++ {
		origSources, origFields, origClassIDs := original.GetIncomingEdges(i)
		loadSources, loadFields, loadClassIDs := loaded.GetIncomingEdges(i)
		assert.Equal(t, origSources, loadSources, "incoming sources mismatch at idx %d", i)
		assert.Equal(t, origFields, loadFields, "incoming fieldIDs mismatch at idx %d", i)
		assert.Equal(t, origClassIDs, loadClassIDs, "incoming classIDs mismatch at idx %d", i)
	}

	// Verify GC roots
	origRoots := original.GetGCRoots()
	loadRoots := loaded.GetGCRoots()
	assert.Equal(t, len(origRoots), len(loadRoots), "GC root count mismatch")
	for i, origRoot := range origRoots {
		assert.Equal(t, origRoot.ObjectID, loadRoots[i].ObjectID, "GC root objectID mismatch at %d", i)
		assert.Equal(t, origRoot.Type, loadRoots[i].Type, "GC root type mismatch at %d", i)
		assert.Equal(t, origRoot.ThreadID, loadRoots[i].ThreadID, "GC root threadID mismatch at %d", i)
		assert.Equal(t, origRoot.FrameIndex, loadRoots[i].FrameIndex, "GC root frameIndex mismatch at %d", i)
	}

	// Verify bitsets
	for i := int32(0); i < n; i++ {
		assert.Equal(t, original.IsGCRoot(i), loaded.IsGCRoot(i), "gcRoot bit mismatch at idx %d", i)
		assert.Equal(t, original.IsClassObject(i), loaded.IsClassObject(i), "classObject bit mismatch at idx %d", i)
		assert.Equal(t, original.IsReachable(i), loaded.IsReachable(i), "reachable bit mismatch at idx %d", i)
	}

	// Verify field names resolution
	for i := int32(0); i < n; i++ {
		_, origFieldIDs, _ := original.GetOutgoingEdges(i)
		_, loadFieldIDs, _ := loaded.GetOutgoingEdges(i)
		for j, fid := range origFieldIDs {
			origName := original.GetFieldName(fid)
			loadName := loaded.GetFieldName(loadFieldIDs[j])
			assert.Equal(t, origName, loadName, "field name mismatch at idx %d, edge %d (fieldID %d)", i, j, fid)
		}
	}
}

func classNameForTest(i int) string {
	names := []string{
		"java.lang.String", "java.util.ArrayList", "java.util.HashMap",
		"java.lang.Object", "java.io.BufferedReader", "java.net.Socket",
		"com.example.Service", "com.example.Repository", "com.example.Controller",
		"org.springframework.context.ApplicationContext",
		"java.util.concurrent.ConcurrentHashMap", "java.lang.Thread",
		"java.lang.ref.WeakReference", "java.util.LinkedList",
		"java.util.TreeMap", "java.io.ByteArrayOutputStream",
		"java.lang.StringBuilder", "java.util.HashSet",
		"java.util.concurrent.locks.ReentrantLock", "java.lang.Class",
		"com.example.Model", "com.example.Dto", "com.example.Entity",
		"com.example.Factory", "com.example.Builder",
		"com.example.Handler", "com.example.Listener",
		"com.example.Adapter", "com.example.Proxy",
		"com.example.Cache", "com.example.Queue",
		"com.example.Pool", "com.example.Timer",
		"com.example.Scheduler", "com.example.Executor",
		"com.example.Pipeline", "com.example.Filter",
		"com.example.Decoder", "com.example.Encoder",
		"com.example.Serializer", "com.example.Deserializer",
		"com.example.Validator", "com.example.Parser",
		"com.example.Formatter", "com.example.Converter",
		"com.example.Mapper", "com.example.Resolver",
		"com.example.Injector", "com.example.Interceptor",
		"com.example.Middleware",
	}
	return names[i%len(names)]
}

func fieldNameForTest(i int) string {
	names := []string{"value", "next", "items", "data", "buffer", "handler", "name", "instance", "ref", "parent"}
	return names[i%len(names)]
}

// ============================================================================
// Boundary Tests
// ============================================================================

// TestIndexV2EmptyGraph tests v2 format roundtrip with an empty graph.
func TestIndexV2EmptyGraph(t *testing.T) {
	graph := NewIndexedReferenceGraph(0)
	graph.FinalizeObjects()
	outBuilder := NewCompactEdgeListBuilder(0, 0)
	inBuilder := NewCompactEdgeListBuilder(0, 0)
	graph.BuildEdges(outBuilder, inBuilder)

	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "heap_index_v2_empty.bin")

	err := WriteHeapIndexV2(indexPath, graph)
	require.NoError(t, err)

	loaded, err := ReadHeapIndex(indexPath)
	require.NoError(t, err)

	mmapIndex, ok := loaded.(*MmapHeapIndex)
	require.True(t, ok)
	defer mmapIndex.Close()

	assert.Equal(t, int32(0), loaded.ObjectCount())
}

// TestIndexSingleObjectRoundtrip tests roundtrip with exactly 1 object (v1 + v2).
func TestIndexSingleObjectRoundtrip(t *testing.T) {
	graph := NewIndexedReferenceGraph(1)
	graph.AddObject(0xDEADBEEF, 999, 128)
	graph.SetClassName(999, "com.example.SingleObject")
	graph.AddGCRoot(GCRoot{ObjectID: 0xDEADBEEF, Type: GCRootThreadObject, ThreadID: 7})
	graph.FinalizeObjects()

	outBuilder := NewCompactEdgeListBuilder(1, 0)
	inBuilder := NewCompactEdgeListBuilder(1, 0)
	graph.BuildEdges(outBuilder, inBuilder)

	graph.SetRetainedSize(0, 256)
	graph.SetDominator(0, -1)
	graph.MarkReachable(0)

	t.Run("v1", func(t *testing.T) {
		var buf bytes.Buffer
		err := writeHeapIndexTo(&buf, graph)
		require.NoError(t, err)

		loaded, err := readHeapIndexFrom(&buf)
		require.NoError(t, err)
		verifyGraphEquality(t, graph, loaded)
	})

	t.Run("v2", func(t *testing.T) {
		tmpDir := t.TempDir()
		indexPath := filepath.Join(tmpDir, "single_v2.bin")
		err := WriteHeapIndexV2(indexPath, graph)
		require.NoError(t, err)

		loaded, err := ReadHeapIndex(indexPath)
		require.NoError(t, err)

		mmapIndex, ok := loaded.(*MmapHeapIndex)
		require.True(t, ok)
		defer mmapIndex.Close()

		verifyHeapGraphEquality(t, graph, loaded)
	})
}

// TestIndexLargeFieldNames tests roundtrip with very long field names.
func TestIndexLargeFieldNames(t *testing.T) {
	graph := NewIndexedReferenceGraph(3)
	graph.AddObject(0x1001, 100, 32)
	graph.AddObject(0x1002, 100, 48)
	graph.AddObject(0x1003, 100, 64)
	graph.SetClassName(100, "TestClass")
	graph.AddGCRoot(GCRoot{ObjectID: 0x1001, Type: GCRootJNIGlobal})
	graph.FinalizeObjects()

	// Create a very long field name (1000 chars)
	longFieldName := ""
	for i := 0; i < 100; i++ {
		longFieldName += "longField_"
	}

	// Unicode field name
	unicodeFieldName := "字段名_フィールド_필드명_поле"

	outBuilder := NewCompactEdgeListBuilder(3, 2)
	inBuilder := NewCompactEdgeListBuilder(3, 2)
	outBuilder.AddEdge(0, 1, longFieldName, 100)
	outBuilder.AddEdge(0, 2, unicodeFieldName, 100)
	inBuilder.AddEdge(1, 0, longFieldName, 100)
	inBuilder.AddEdge(2, 0, unicodeFieldName, 100)
	graph.BuildEdges(outBuilder, inBuilder)

	t.Run("v1", func(t *testing.T) {
		var buf bytes.Buffer
		err := writeHeapIndexTo(&buf, graph)
		require.NoError(t, err)

		loaded, err := readHeapIndexFrom(&buf)
		require.NoError(t, err)
		verifyGraphEquality(t, graph, loaded)

		// Verify the long field name is preserved
		outgoing := loaded.GetOutgoing()
		_, fieldIDs, _ := loaded.GetOutgoingEdges(0)
		require.Len(t, fieldIDs, 2)
		assert.Equal(t, longFieldName, outgoing.GetFieldName(fieldIDs[0]))
		assert.Equal(t, unicodeFieldName, outgoing.GetFieldName(fieldIDs[1]))
	})

	t.Run("v2", func(t *testing.T) {
		tmpDir := t.TempDir()
		indexPath := filepath.Join(tmpDir, "longfield_v2.bin")
		err := WriteHeapIndexV2(indexPath, graph)
		require.NoError(t, err)

		loaded, err := ReadHeapIndex(indexPath)
		require.NoError(t, err)

		mmapIndex, ok := loaded.(*MmapHeapIndex)
		require.True(t, ok)
		defer mmapIndex.Close()

		verifyHeapGraphEquality(t, graph, loaded)
	})
}

// TestIndexLongUnicodeClassNames tests roundtrip with Unicode class names.
func TestIndexLongUnicodeClassNames(t *testing.T) {
	graph := NewIndexedReferenceGraph(2)
	graph.AddObject(0x1001, 100, 32)
	graph.AddObject(0x1002, 200, 48)

	// Long class names with various Unicode characters
	graph.SetClassName(100, "com.example.very.deep.package.hierarchy.level1.level2.level3.SuperLongClassName$$Lambda$12345/0x00000001234abcde")
	graph.SetClassName(200, "中文类名.日本語クラス.한국어클래스.КлассНаРусском")

	graph.AddGCRoot(GCRoot{ObjectID: 0x1001, Type: GCRootJNIGlobal})
	graph.FinalizeObjects()

	outBuilder := NewCompactEdgeListBuilder(2, 1)
	inBuilder := NewCompactEdgeListBuilder(2, 1)
	outBuilder.AddEdge(0, 1, "ref", 100)
	inBuilder.AddEdge(1, 0, "ref", 100)
	graph.BuildEdges(outBuilder, inBuilder)

	t.Run("v1", func(t *testing.T) {
		var buf bytes.Buffer
		err := writeHeapIndexTo(&buf, graph)
		require.NoError(t, err)

		loaded, err := readHeapIndexFrom(&buf)
		require.NoError(t, err)
		verifyGraphEquality(t, graph, loaded)
	})

	t.Run("v2", func(t *testing.T) {
		tmpDir := t.TempDir()
		indexPath := filepath.Join(tmpDir, "unicode_v2.bin")
		err := WriteHeapIndexV2(indexPath, graph)
		require.NoError(t, err)

		loaded, err := ReadHeapIndex(indexPath)
		require.NoError(t, err)

		mmapIndex, ok := loaded.(*MmapHeapIndex)
		require.True(t, ok)
		defer mmapIndex.Close()

		verifyHeapGraphEquality(t, graph, loaded)
	})
}

// TestIndexNoEdgesGraph tests roundtrip of graph with objects but no edges.
func TestIndexNoEdgesGraph(t *testing.T) {
	graph := NewIndexedReferenceGraph(5)
	for i := 0; i < 5; i++ {
		graph.AddObject(uint64(0x1000+i), 100, int64(16*(i+1)))
	}
	graph.SetClassName(100, "IsolatedObject")
	graph.AddGCRoot(GCRoot{ObjectID: 0x1000, Type: GCRootJNIGlobal})
	graph.FinalizeObjects()

	outBuilder := NewCompactEdgeListBuilder(5, 0)
	inBuilder := NewCompactEdgeListBuilder(5, 0)
	graph.BuildEdges(outBuilder, inBuilder)

	t.Run("v1", func(t *testing.T) {
		var buf bytes.Buffer
		err := writeHeapIndexTo(&buf, graph)
		require.NoError(t, err)

		loaded, err := readHeapIndexFrom(&buf)
		require.NoError(t, err)
		verifyGraphEquality(t, graph, loaded)

		// Verify all edges are empty
		for i := int32(0); i < 5; i++ {
			targets, _, _ := loaded.GetOutgoingEdges(i)
			assert.Empty(t, targets, "object %d should have no outgoing edges", i)
		}
	})

	t.Run("v2", func(t *testing.T) {
		tmpDir := t.TempDir()
		indexPath := filepath.Join(tmpDir, "noedges_v2.bin")
		err := WriteHeapIndexV2(indexPath, graph)
		require.NoError(t, err)

		loaded, err := ReadHeapIndex(indexPath)
		require.NoError(t, err)

		mmapIndex, ok := loaded.(*MmapHeapIndex)
		require.True(t, ok)
		defer mmapIndex.Close()

		verifyHeapGraphEquality(t, graph, loaded)
	})
}

// TestIndexMaxObjectIDValues tests roundtrip with extreme object ID values.
func TestIndexMaxObjectIDValues(t *testing.T) {
	graph := NewIndexedReferenceGraph(3)
	graph.AddObject(0, 1, 16)                      // minimum objectID
	graph.AddObject(0x7FFFFFFFFFFFFFFF, 2, 32)     // max signed int64
	graph.AddObject(0xFFFFFFFFFFFFFFFF, 3, 64)     // max uint64
	graph.SetClassName(1, "MinID")
	graph.SetClassName(2, "MaxSigned")
	graph.SetClassName(3, "MaxUnsigned")
	graph.AddGCRoot(GCRoot{ObjectID: 0, Type: GCRootJNIGlobal})
	graph.FinalizeObjects()

	outBuilder := NewCompactEdgeListBuilder(3, 2)
	inBuilder := NewCompactEdgeListBuilder(3, 2)
	outBuilder.AddEdge(0, 1, "next", 1)
	outBuilder.AddEdge(1, 2, "prev", 2)
	inBuilder.AddEdge(1, 0, "next", 1)
	inBuilder.AddEdge(2, 1, "prev", 2)
	graph.BuildEdges(outBuilder, inBuilder)

	t.Run("v1", func(t *testing.T) {
		var buf bytes.Buffer
		err := writeHeapIndexTo(&buf, graph)
		require.NoError(t, err)

		loaded, err := readHeapIndexFrom(&buf)
		require.NoError(t, err)
		verifyGraphEquality(t, graph, loaded)
	})

	t.Run("v2", func(t *testing.T) {
		tmpDir := t.TempDir()
		indexPath := filepath.Join(tmpDir, "maxid_v2.bin")
		err := WriteHeapIndexV2(indexPath, graph)
		require.NoError(t, err)

		loaded, err := ReadHeapIndex(indexPath)
		require.NoError(t, err)

		mmapIndex, ok := loaded.(*MmapHeapIndex)
		require.True(t, ok)
		defer mmapIndex.Close()

		verifyHeapGraphEquality(t, graph, loaded)
	})
}

// ============================================================================
// Internal helpers
// ============================================================================

// writeHeaderForTest writes just a header (for error case tests).
func writeHeaderForTest(buf *bytes.Buffer, header *IndexFileHeader) error {
	return writeHeaderBinary(buf, header)
}

func writeHeaderBinary(w *bytes.Buffer, header *IndexFileHeader) error {
	return binary.Write(w, indexByteOrder, header)
}
