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

	// Read back
	loaded, err := ReadHeapIndex(indexPath)
	require.NoError(t, err, "ReadHeapIndex should succeed")

	// Verify all data matches
	verifyGraphEquality(t, graph, loaded)
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

// writeHeaderForTest writes just a header (for error case tests).
func writeHeaderForTest(buf *bytes.Buffer, header *IndexFileHeader) error {
	return writeHeaderBinary(buf, header)
}

func writeHeaderBinary(w *bytes.Buffer, header *IndexFileHeader) error {
	return binary.Write(w, indexByteOrder, header)
}
