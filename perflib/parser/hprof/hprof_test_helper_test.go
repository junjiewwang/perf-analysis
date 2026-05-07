package hprof

import (
	"bytes"
	"encoding/binary"
)

// hprofTestBuilder helps construct synthetic HPROF binary data for testing.
// It writes records in big-endian byte order (HPROF standard).
type hprofTestBuilder struct {
	buf    bytes.Buffer
	idSize int
}

// newHprofTestBuilder creates a new HPROF test data builder.
func newHprofTestBuilder(idSize int) *hprofTestBuilder {
	b := &hprofTestBuilder{idSize: idSize}
	b.writeHeader()
	return b
}

// writeHeader writes the HPROF file header.
func (b *hprofTestBuilder) writeHeader() {
	b.buf.WriteString("JAVA PROFILE 1.0.2")
	b.buf.WriteByte(0) // null terminator
	binary.Write(&b.buf, binary.BigEndian, uint32(b.idSize))
	binary.Write(&b.buf, binary.BigEndian, uint64(1700000000000)) // timestamp
}

// writeID writes an ID value with the configured idSize.
func (b *hprofTestBuilder) writeID(id uint64) {
	if b.idSize == 4 {
		binary.Write(&b.buf, binary.BigEndian, uint32(id))
	} else {
		binary.Write(&b.buf, binary.BigEndian, id)
	}
}

// addStringRecord adds a STRING_IN_UTF8 record.
func (b *hprofTestBuilder) addStringRecord(id uint64, value string) {
	var data bytes.Buffer
	if b.idSize == 4 {
		binary.Write(&data, binary.BigEndian, uint32(id))
	} else {
		binary.Write(&data, binary.BigEndian, id)
	}
	data.WriteString(value)

	b.writeRecordHeader(TagString, uint32(data.Len()))
	b.buf.Write(data.Bytes())
}

// addLoadClassRecord adds a LOAD_CLASS record.
func (b *hprofTestBuilder) addLoadClassRecord(serialNum uint32, classObjID uint64, stackSerial uint32, nameStringID uint64) {
	var data bytes.Buffer
	binary.Write(&data, binary.BigEndian, serialNum)
	if b.idSize == 4 {
		binary.Write(&data, binary.BigEndian, uint32(classObjID))
	} else {
		binary.Write(&data, binary.BigEndian, classObjID)
	}
	binary.Write(&data, binary.BigEndian, stackSerial)
	if b.idSize == 4 {
		binary.Write(&data, binary.BigEndian, uint32(nameStringID))
	} else {
		binary.Write(&data, binary.BigEndian, nameStringID)
	}

	b.writeRecordHeader(TagLoadClass, uint32(data.Len()))
	b.buf.Write(data.Bytes())
}

// heapDumpBuilder builds heap dump sub-records.
type heapDumpBuilder struct {
	buf    bytes.Buffer
	idSize int
}

// newHeapDumpBuilder creates a heap dump sub-record builder.
func newHeapDumpBuilder(idSize int) *heapDumpBuilder {
	return &heapDumpBuilder{idSize: idSize}
}

// writeID writes an ID value.
func (h *heapDumpBuilder) writeID(id uint64) {
	if h.idSize == 4 {
		binary.Write(&h.buf, binary.BigEndian, uint32(id))
	} else {
		binary.Write(&h.buf, binary.BigEndian, id)
	}
}

// addGCRootJNIGlobal adds a GC_ROOT_JNI_GLOBAL sub-record.
func (h *heapDumpBuilder) addGCRootJNIGlobal(objectID, jniGlobalRef uint64) {
	h.buf.WriteByte(byte(HeapTagRootJNIGlobal))
	h.writeID(objectID)
	h.writeID(jniGlobalRef)
}

// addGCRootStickyClass adds a GC_ROOT_STICKY_CLASS sub-record.
func (h *heapDumpBuilder) addGCRootStickyClass(objectID uint64) {
	h.buf.WriteByte(byte(HeapTagRootStickyClass))
	h.writeID(objectID)
}

// addGCRootThreadObject adds a GC_ROOT_THREAD_OBJ sub-record.
func (h *heapDumpBuilder) addGCRootThreadObject(objectID uint64, threadSerial, stackSerial uint32) {
	h.buf.WriteByte(byte(HeapTagRootThreadObject))
	h.writeID(objectID)
	binary.Write(&h.buf, binary.BigEndian, threadSerial)
	binary.Write(&h.buf, binary.BigEndian, stackSerial)
}

// addClassDump adds a CLASS_DUMP sub-record.
// staticFields: each is {nameID, type, value bytes}
// instanceFields: each is {nameID, type}
func (h *heapDumpBuilder) addClassDump(classObjID, superClassID uint64, instanceSize uint32,
	staticFields []testStaticField, instanceFields []testInstanceField) {

	h.buf.WriteByte(byte(HeapTagClassDump))
	h.writeID(classObjID)
	binary.Write(&h.buf, binary.BigEndian, uint32(0)) // stack trace serial
	h.writeID(superClassID)                           // super class
	h.writeID(0)                                      // class loader
	h.writeID(0)                                      // signers
	h.writeID(0)                                      // protection domain
	h.writeID(0)                                      // reserved 1
	h.writeID(0)                                      // reserved 2
	binary.Write(&h.buf, binary.BigEndian, instanceSize)

	// Constant pool (empty)
	binary.Write(&h.buf, binary.BigEndian, uint16(0))

	// Static fields
	binary.Write(&h.buf, binary.BigEndian, uint16(len(staticFields)))
	for _, sf := range staticFields {
		h.writeID(sf.nameID)
		h.buf.WriteByte(byte(sf.fieldType))
		h.buf.Write(sf.value)
	}

	// Instance fields
	binary.Write(&h.buf, binary.BigEndian, uint16(len(instanceFields)))
	for _, f := range instanceFields {
		h.writeID(f.nameID)
		h.buf.WriteByte(byte(f.fieldType))
	}
}

// addInstanceDump adds an INSTANCE_DUMP sub-record.
func (h *heapDumpBuilder) addInstanceDump(objectID, classID uint64, data []byte) {
	h.buf.WriteByte(byte(HeapTagInstanceDump))
	h.writeID(objectID)
	binary.Write(&h.buf, binary.BigEndian, uint32(0)) // stack trace serial
	h.writeID(classID)
	binary.Write(&h.buf, binary.BigEndian, uint32(len(data)))
	h.buf.Write(data)
}

// addObjectArrayDump adds an OBJECT_ARRAY_DUMP sub-record.
func (h *heapDumpBuilder) addObjectArrayDump(objectID, arrayClassID uint64, elements []uint64) {
	h.buf.WriteByte(byte(HeapTagObjectArrayDump))
	h.writeID(objectID)
	binary.Write(&h.buf, binary.BigEndian, uint32(0)) // stack trace serial
	binary.Write(&h.buf, binary.BigEndian, uint32(len(elements)))
	h.writeID(arrayClassID)
	for _, elem := range elements {
		h.writeID(elem)
	}
}

// addPrimitiveArrayDump adds a PRIMITIVE_ARRAY_DUMP sub-record.
func (h *heapDumpBuilder) addPrimitiveArrayDump(objectID uint64, elementType BasicType, numElements uint32) {
	h.buf.WriteByte(byte(HeapTagPrimitiveArrayDump))
	h.writeID(objectID)
	binary.Write(&h.buf, binary.BigEndian, uint32(0)) // stack trace serial
	binary.Write(&h.buf, binary.BigEndian, numElements)
	h.buf.WriteByte(byte(elementType))

	// Write dummy element data
	elemSize := BasicTypeSize(elementType, h.idSize)
	for i := 0; i < int(numElements); i++ {
		h.buf.Write(make([]byte, elemSize))
	}
}

// bytes returns the heap dump data.
func (h *heapDumpBuilder) bytes() []byte {
	return h.buf.Bytes()
}

// addHeapDumpSegment adds a HEAP_DUMP_SEGMENT record with the given heap dump data.
func (b *hprofTestBuilder) addHeapDumpSegment(heapData []byte) {
	b.writeRecordHeader(TagHeapDumpSegment, uint32(len(heapData)))
	b.buf.Write(heapData)
}

// addHeapDumpEnd adds a HEAP_DUMP_END record.
func (b *hprofTestBuilder) addHeapDumpEnd() {
	b.writeRecordHeader(TagHeapDumpEnd, 0)
}

// writeRecordHeader writes a record header (tag + timestamp + length).
func (b *hprofTestBuilder) writeRecordHeader(tag RecordTag, length uint32) {
	b.buf.WriteByte(byte(tag))
	binary.Write(&b.buf, binary.BigEndian, uint32(0)) // timestamp
	binary.Write(&b.buf, binary.BigEndian, length)
}

// bytes returns the complete HPROF binary data.
func (b *hprofTestBuilder) bytes() []byte {
	return b.buf.Bytes()
}

// reader returns a bytes.Reader for the built data (supports io.ReadSeeker).
func (b *hprofTestBuilder) reader() *bytes.Reader {
	return bytes.NewReader(b.buf.Bytes())
}

// testStaticField represents a static field for test data construction.
type testStaticField struct {
	nameID    uint64
	fieldType BasicType
	value     []byte
}

// testInstanceField represents an instance field descriptor for test data.
type testInstanceField struct {
	nameID    uint64
	fieldType BasicType
}

// makeObjectRefBytes creates byte representation of an object ID reference.
func makeObjectRefBytes(id uint64, idSize int) []byte {
	if idSize == 4 {
		buf := make([]byte, 4)
		buf[0] = byte(id >> 24)
		buf[1] = byte(id >> 16)
		buf[2] = byte(id >> 8)
		buf[3] = byte(id)
		return buf
	}
	buf := make([]byte, 8)
	buf[0] = byte(id >> 56)
	buf[1] = byte(id >> 48)
	buf[2] = byte(id >> 40)
	buf[3] = byte(id >> 32)
	buf[4] = byte(id >> 24)
	buf[5] = byte(id >> 16)
	buf[6] = byte(id >> 8)
	buf[7] = byte(id)
	return buf
}

// makeIntBytes creates byte representation of a 4-byte int.
func makeIntBytes(val int32) []byte {
	buf := make([]byte, 4)
	buf[0] = byte(val >> 24)
	buf[1] = byte(val >> 16)
	buf[2] = byte(val >> 8)
	buf[3] = byte(val)
	return buf
}

// buildSimpleHprofData creates a minimal HPROF dataset with known objects and references.
// Returns a reader suitable for ParseTwoPass.
//
// The test graph structure:
//
//	java.lang.Class (ID: 0x100) - meta class
//	ClassA (ID: 0x200) - has fields: refB (TypeObject), intField (TypeInt)
//	ClassB (ID: 0x300) - has fields: refC (TypeObject)
//	ClassC (ID: 0x400) - has no reference fields
//
//	Instance objA1 (ID: 0x1001) of ClassA → references objB1
//	Instance objA2 (ID: 0x1002) of ClassA → references objB2
//	Instance objB1 (ID: 0x2001) of ClassB → references objC1
//	Instance objB2 (ID: 0x2002) of ClassB → references objC1 (shared)
//	Instance objC1 (ID: 0x3001) of ClassC → no refs
//	Array arr1 (ID: 0x4001) → [objA1, objA2]
//
//	GC Roots: objA1 (JNI_GLOBAL), arr1 (STICKY_CLASS)
func buildSimpleHprofData(idSize int) *bytes.Reader {
	b := newHprofTestBuilder(idSize)

	// String records
	b.addStringRecord(1, "java/lang/Class")
	b.addStringRecord(2, "com/test/ClassA")
	b.addStringRecord(3, "com/test/ClassB")
	b.addStringRecord(4, "com/test/ClassC")
	b.addStringRecord(5, "refB")
	b.addStringRecord(6, "intField")
	b.addStringRecord(7, "refC")
	b.addStringRecord(8, "[Lcom/test/ClassA;") // array class name

	// Load class records
	b.addLoadClassRecord(1, 0x100, 0, 1) // java.lang.Class
	b.addLoadClassRecord(2, 0x200, 0, 2) // ClassA
	b.addLoadClassRecord(3, 0x300, 0, 3) // ClassB
	b.addLoadClassRecord(4, 0x400, 0, 4) // ClassC
	b.addLoadClassRecord(5, 0x500, 0, 8) // ClassA[]

	// Heap dump segment
	heap := newHeapDumpBuilder(idSize)

	// GC Roots
	heap.addGCRootJNIGlobal(0x1001, 0) // objA1 is a JNI global root
	heap.addGCRootStickyClass(0x4001)   // arr1 is a sticky class root

	// CLASS_DUMP: java.lang.Class (no instance fields for simplicity)
	heap.addClassDump(0x100, 0, 0, nil, nil)

	// CLASS_DUMP: ClassA - has refB (object) + intField (int) = idSize + 4 bytes instance data
	heap.addClassDump(0x200, 0, uint32(idSize+4), nil, []testInstanceField{
		{nameID: 5, fieldType: TypeObject}, // refB
		{nameID: 6, fieldType: TypeInt},    // intField
	})

	// CLASS_DUMP: ClassB - has refC (object) = idSize bytes instance data
	heap.addClassDump(0x300, 0, uint32(idSize), nil, []testInstanceField{
		{nameID: 7, fieldType: TypeObject}, // refC
	})

	// CLASS_DUMP: ClassC - no fields
	heap.addClassDump(0x400, 0, 0, nil, nil)

	// CLASS_DUMP: ClassA[] (array class, no fields)
	heap.addClassDump(0x500, 0, 0, nil, nil)

	// INSTANCE_DUMP: objA1 (0x1001) of ClassA → refB=objB1, intField=42
	instanceDataA1 := append(makeObjectRefBytes(0x2001, idSize), makeIntBytes(42)...)
	heap.addInstanceDump(0x1001, 0x200, instanceDataA1)

	// INSTANCE_DUMP: objA2 (0x1002) of ClassA → refB=objB2, intField=99
	instanceDataA2 := append(makeObjectRefBytes(0x2002, idSize), makeIntBytes(99)...)
	heap.addInstanceDump(0x1002, 0x200, instanceDataA2)

	// INSTANCE_DUMP: objB1 (0x2001) of ClassB → refC=objC1
	instanceDataB1 := makeObjectRefBytes(0x3001, idSize)
	heap.addInstanceDump(0x2001, 0x300, instanceDataB1)

	// INSTANCE_DUMP: objB2 (0x2002) of ClassB → refC=objC1 (shared reference)
	instanceDataB2 := makeObjectRefBytes(0x3001, idSize)
	heap.addInstanceDump(0x2002, 0x300, instanceDataB2)

	// INSTANCE_DUMP: objC1 (0x3001) of ClassC → no data
	heap.addInstanceDump(0x3001, 0x400, nil)

	// OBJECT_ARRAY_DUMP: arr1 (0x4001) of ClassA[] → [objA1, objA2]
	heap.addObjectArrayDump(0x4001, 0x500, []uint64{0x1001, 0x1002})

	// Primitive array for completeness: byteArr1 (0x5001)
	heap.addPrimitiveArrayDump(0x5001, TypeByte, 16)

	b.addHeapDumpSegment(heap.bytes())
	b.addHeapDumpEnd()

	return b.reader()
}
