package webui

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// buildTestGraph creates a test IndexedReferenceGraph using ParseTwoPass
// with synthetic HPROF data for HeapQueryEngine testing.
func buildTestGraph(t *testing.T) *hprof.IndexedReferenceGraph {
	t.Helper()
	reader := buildSimpleHprofDataForWebUI(8)
	parser := hprof.NewParser(nil)
	ctx := context.Background()

	graph, _, err := parser.ParseTwoPass(ctx, reader)
	require.NoError(t, err)
	require.NotNil(t, graph)
	return graph
}

func TestHeapQueryEngine_QueryBiggestObjects(t *testing.T) {
	graph := buildTestGraph(t)
	engine := NewHeapQueryEngine(graph)

	t.Run("returns top N objects", func(t *testing.T) {
		results := engine.QueryBiggestObjects(5, "retained", "")
		assert.NotEmpty(t, results)
		// Objects should be sorted by retained size descending
		for i := 1; i < len(results); i++ {
			assert.GreaterOrEqual(t, results[i-1].RetainedSize, results[i].RetainedSize)
		}
	})

	t.Run("filter by class name", func(t *testing.T) {
		results := engine.QueryBiggestObjects(10, "retained", "com.test.ClassA")
		for _, r := range results {
			assert.Equal(t, "com.test.ClassA", r.ClassName)
		}
	})

	t.Run("non-existent class returns nil", func(t *testing.T) {
		results := engine.QueryBiggestObjects(10, "retained", "non.existent.Class")
		assert.Nil(t, results)
	})

	t.Run("sort by shallow", func(t *testing.T) {
		results := engine.QueryBiggestObjects(5, "shallow", "")
		assert.NotEmpty(t, results)
		for i := 1; i < len(results); i++ {
			assert.GreaterOrEqual(t, results[i-1].ShallowSize, results[i].ShallowSize)
		}
	})
}

func TestHeapQueryEngine_QueryRetainers(t *testing.T) {
	graph := buildTestGraph(t)
	engine := NewHeapQueryEngine(graph)

	t.Run("objC1 retained by objB1 and objB2", func(t *testing.T) {
		results := engine.QueryRetainers(0x3001, 10)
		assert.Len(t, results, 2)
		// Both retainers should be ClassB instances
		for _, r := range results {
			assert.Equal(t, "com.test.ClassB", r.ClassName)
			assert.Equal(t, "refC", r.FieldName)
		}
	})

	t.Run("objB1 retained by objA1", func(t *testing.T) {
		results := engine.QueryRetainers(0x2001, 10)
		assert.Len(t, results, 1)
		assert.Equal(t, "com.test.ClassA", results[0].ClassName)
		assert.Equal(t, "refB", results[0].FieldName)
	})

	t.Run("non-existent object returns nil", func(t *testing.T) {
		results := engine.QueryRetainers(0x9999, 10)
		assert.Nil(t, results)
	})
}

func TestHeapQueryEngine_QueryObjectFields(t *testing.T) {
	graph := buildTestGraph(t)
	engine := NewHeapQueryEngine(graph)

	t.Run("objA1 has refB field pointing to objB1", func(t *testing.T) {
		results := engine.QueryObjectFields(0x1001)
		assert.Len(t, results, 1)
		assert.Equal(t, "refB", results[0].Name)
		assert.Equal(t, "com.test.ClassB", results[0].RefClass)
		assert.True(t, results[0].HasChildren) // objB1 has outgoing edge to objC1
	})

	t.Run("objB1 has refC field pointing to objC1", func(t *testing.T) {
		results := engine.QueryObjectFields(0x2001)
		assert.Len(t, results, 1)
		assert.Equal(t, "refC", results[0].Name)
		assert.Equal(t, "com.test.ClassC", results[0].RefClass)
		assert.False(t, results[0].HasChildren) // objC1 has no outgoing edges
	})

	t.Run("arr1 has array elements", func(t *testing.T) {
		results := engine.QueryObjectFields(0x4001)
		assert.Len(t, results, 2) // [0] and [1]
	})

	t.Run("objC1 has no fields", func(t *testing.T) {
		results := engine.QueryObjectFields(0x3001)
		assert.Nil(t, results)
	})
}

func TestHeapQueryEngine_QueryClassInstances(t *testing.T) {
	graph := buildTestGraph(t)
	engine := NewHeapQueryEngine(graph)

	t.Run("ClassA instances", func(t *testing.T) {
		results := engine.QueryClassInstances("com.test.ClassA", 10, "retained")
		assert.Len(t, results, 2) // objA1, objA2
		for _, r := range results {
			assert.Equal(t, "com.test.ClassA", r.ClassName)
		}
		// Should be sorted by retained size descending
		if len(results) > 1 {
			assert.GreaterOrEqual(t, results[0].RetainedSize, results[1].RetainedSize)
		}
	})

	t.Run("ClassB instances", func(t *testing.T) {
		results := engine.QueryClassInstances("com.test.ClassB", 10, "retained")
		assert.Len(t, results, 2) // objB1, objB2
	})

	t.Run("ClassC instances", func(t *testing.T) {
		results := engine.QueryClassInstances("com.test.ClassC", 10, "retained")
		assert.Len(t, results, 1) // objC1
	})

	t.Run("non-existent class", func(t *testing.T) {
		results := engine.QueryClassInstances("does.not.Exist", 10, "retained")
		assert.Nil(t, results)
	})
}

func TestHeapQueryEngine_QueryGCRootsSummary(t *testing.T) {
	graph := buildTestGraph(t)
	engine := NewHeapQueryEngine(graph)

	results := engine.QueryGCRootsSummary()
	assert.NotEmpty(t, results)

	// There should be at least 2 GC root groups (ClassA instance + array)
	totalRoots := 0
	for _, r := range results {
		totalRoots += r.InstanceCount
		assert.Greater(t, r.TotalShallow, int64(0))
	}
	assert.Equal(t, 2, totalRoots) // objA1 + arr1
}

func TestHeapQueryEngine_QueryGCRootPath(t *testing.T) {
	graph := buildTestGraph(t)
	engine := NewHeapQueryEngine(graph)

	t.Run("objC1 should have path to GC root", func(t *testing.T) {
		// objC1 ← objB1 ← objA1 (GC root)
		// objC1 ← objB2 ← objA2 ← arr1 (GC root)
		results := engine.QueryGCRootPath(0x3001, 3, 10)
		assert.NotEmpty(t, results)
		// Each path should end at a GC root
		for _, r := range results {
			assert.Greater(t, r.Depth, 0)
			assert.NotEmpty(t, r.Path)
		}
	})

	t.Run("GC root object has no path to itself", func(t *testing.T) {
		// objA1 IS a GC root - should find path from itself
		// The implementation skips startIdx == rootIdx
		results := engine.QueryGCRootPath(0x1001, 3, 10)
		// objA1 itself is a root but the algorithm checks current.idx != startIdx
		// So it would look through incoming edges to find another root
		// arr1 → objA1, and arr1 is a root, so path should be found
		assert.NotEmpty(t, results)
	})

	t.Run("non-existent object", func(t *testing.T) {
		results := engine.QueryGCRootPath(0x9999, 3, 10)
		assert.Nil(t, results)
	})
}

// ===================================================================
// HPROF test data builder for webui package
// (duplicates minimal logic since test helper is in different package)
// ===================================================================

func buildSimpleHprofDataForWebUI(idSize int) *bytes.Reader {
	b := &hprofBuilder{idSize: idSize}
	b.writeHeader()

	// String records
	b.addString(1, "java/lang/Class")
	b.addString(2, "com/test/ClassA")
	b.addString(3, "com/test/ClassB")
	b.addString(4, "com/test/ClassC")
	b.addString(5, "refB")
	b.addString(6, "intField")
	b.addString(7, "refC")
	b.addString(8, "[Lcom/test/ClassA;")

	// Load class records
	b.addLoadClass(1, 0x100, 0, 1)
	b.addLoadClass(2, 0x200, 0, 2)
	b.addLoadClass(3, 0x300, 0, 3)
	b.addLoadClass(4, 0x400, 0, 4)
	b.addLoadClass(5, 0x500, 0, 8)

	// Heap dump
	var heap bytes.Buffer

	// GC Roots
	heap.WriteByte(0x01) // JNI_GLOBAL
	writeID(&heap, 0x1001, idSize)
	writeID(&heap, 0, idSize)

	heap.WriteByte(0x05) // STICKY_CLASS
	writeID(&heap, 0x4001, idSize)

	// CLASS_DUMP: java.lang.Class
	writeClassDump(&heap, 0x100, 0, 0, nil, nil, idSize)
	// CLASS_DUMP: ClassA
	writeClassDump(&heap, 0x200, 0, uint32(idSize+4), nil, []fieldDesc{{5, 2}, {6, 10}}, idSize)
	// CLASS_DUMP: ClassB
	writeClassDump(&heap, 0x300, 0, uint32(idSize), nil, []fieldDesc{{7, 2}}, idSize)
	// CLASS_DUMP: ClassC
	writeClassDump(&heap, 0x400, 0, 0, nil, nil, idSize)
	// CLASS_DUMP: ClassA[]
	writeClassDump(&heap, 0x500, 0, 0, nil, nil, idSize)

	// INSTANCE_DUMP: objA1 → refB=objB1
	dataA1 := append(makeRef(0x2001, idSize), makeInt32(42)...)
	writeInstanceDump(&heap, 0x1001, 0x200, dataA1, idSize)
	// INSTANCE_DUMP: objA2 → refB=objB2
	dataA2 := append(makeRef(0x2002, idSize), makeInt32(99)...)
	writeInstanceDump(&heap, 0x1002, 0x200, dataA2, idSize)
	// INSTANCE_DUMP: objB1 → refC=objC1
	writeInstanceDump(&heap, 0x2001, 0x300, makeRef(0x3001, idSize), idSize)
	// INSTANCE_DUMP: objB2 → refC=objC1
	writeInstanceDump(&heap, 0x2002, 0x300, makeRef(0x3001, idSize), idSize)
	// INSTANCE_DUMP: objC1
	writeInstanceDump(&heap, 0x3001, 0x400, nil, idSize)

	// OBJECT_ARRAY_DUMP: arr1 → [objA1, objA2]
	heap.WriteByte(0x22) // OBJECT_ARRAY_DUMP
	writeID(&heap, 0x4001, idSize)
	binary.Write(&heap, binary.BigEndian, uint32(0)) // stack trace
	binary.Write(&heap, binary.BigEndian, uint32(2)) // num elements
	writeID(&heap, 0x500, idSize)                    // array class
	writeID(&heap, 0x1001, idSize)
	writeID(&heap, 0x1002, idSize)

	// PRIMITIVE_ARRAY_DUMP: byteArr1
	heap.WriteByte(0x23)
	writeID(&heap, 0x5001, idSize)
	binary.Write(&heap, binary.BigEndian, uint32(0))  // stack trace
	binary.Write(&heap, binary.BigEndian, uint32(16)) // num elements
	heap.WriteByte(8)                                 // TypeByte
	heap.Write(make([]byte, 16))

	// Write HEAP_DUMP_SEGMENT record
	b.writeRecord(0x1C, heap.Bytes())
	// HEAP_DUMP_END
	b.writeRecord(0x2C, nil)

	return bytes.NewReader(b.buf.Bytes())
}

// Helper types and functions for building HPROF data in webui_test package

type hprofBuilder struct {
	buf    bytes.Buffer
	idSize int
}

func (b *hprofBuilder) writeHeader() {
	b.buf.WriteString("JAVA PROFILE 1.0.2")
	b.buf.WriteByte(0)
	binary.Write(&b.buf, binary.BigEndian, uint32(b.idSize))
	binary.Write(&b.buf, binary.BigEndian, uint64(1700000000000))
}

func (b *hprofBuilder) writeRecord(tag byte, data []byte) {
	b.buf.WriteByte(tag)
	binary.Write(&b.buf, binary.BigEndian, uint32(0)) // timestamp
	binary.Write(&b.buf, binary.BigEndian, uint32(len(data)))
	if data != nil {
		b.buf.Write(data)
	}
}

func (b *hprofBuilder) addString(id uint64, value string) {
	var data bytes.Buffer
	writeID(&data, id, b.idSize)
	data.WriteString(value)
	b.writeRecord(0x01, data.Bytes())
}

func (b *hprofBuilder) addLoadClass(serial uint32, classID uint64, stackSerial uint32, nameID uint64) {
	var data bytes.Buffer
	binary.Write(&data, binary.BigEndian, serial)
	writeID(&data, classID, b.idSize)
	binary.Write(&data, binary.BigEndian, stackSerial)
	writeID(&data, nameID, b.idSize)
	b.writeRecord(0x02, data.Bytes())
}

type fieldDesc struct {
	nameID    uint64
	fieldType byte
}

func writeID(buf *bytes.Buffer, id uint64, idSize int) {
	if idSize == 4 {
		binary.Write(buf, binary.BigEndian, uint32(id))
	} else {
		binary.Write(buf, binary.BigEndian, id)
	}
}

func writeClassDump(buf *bytes.Buffer, classID, superClassID uint64, instanceSize uint32,
	staticFields []fieldDesc, instanceFields []fieldDesc, idSize int) {
	buf.WriteByte(0x20) // CLASS_DUMP tag
	writeID(buf, classID, idSize)
	binary.Write(buf, binary.BigEndian, uint32(0)) // stack trace
	writeID(buf, superClassID, idSize)             // super class
	writeID(buf, 0, idSize)                        // class loader
	writeID(buf, 0, idSize)                        // signers
	writeID(buf, 0, idSize)                        // protection domain
	writeID(buf, 0, idSize)                        // reserved1
	writeID(buf, 0, idSize)                        // reserved2
	binary.Write(buf, binary.BigEndian, instanceSize)

	// Constant pool (empty)
	binary.Write(buf, binary.BigEndian, uint16(0))

	// Static fields
	binary.Write(buf, binary.BigEndian, uint16(len(staticFields)))
	for _, sf := range staticFields {
		writeID(buf, sf.nameID, idSize)
		buf.WriteByte(sf.fieldType)
		// Write zero value
		size := hprof.BasicTypeSize(hprof.BasicType(sf.fieldType), idSize)
		buf.Write(make([]byte, size))
	}

	// Instance fields
	binary.Write(buf, binary.BigEndian, uint16(len(instanceFields)))
	for _, f := range instanceFields {
		writeID(buf, f.nameID, idSize)
		buf.WriteByte(f.fieldType)
	}
}

func writeInstanceDump(buf *bytes.Buffer, objectID, classID uint64, data []byte, idSize int) {
	buf.WriteByte(0x21) // INSTANCE_DUMP tag
	writeID(buf, objectID, idSize)
	binary.Write(buf, binary.BigEndian, uint32(0)) // stack trace
	writeID(buf, classID, idSize)
	binary.Write(buf, binary.BigEndian, uint32(len(data)))
	if data != nil {
		buf.Write(data)
	}
}

func makeRef(id uint64, idSize int) []byte {
	var buf bytes.Buffer
	writeID(&buf, id, idSize)
	return buf.Bytes()
}

func makeInt32(val int32) []byte {
	buf := make([]byte, 4)
	buf[0] = byte(val >> 24)
	buf[1] = byte(val >> 16)
	buf[2] = byte(val >> 8)
	buf[3] = byte(val)
	return buf
}
