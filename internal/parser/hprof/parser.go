package hprof

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ParserOptions configures the HPROF parser.
type ParserOptions struct {
	// TopClassesN is the number of top classes to return.
	TopClassesN int
	// AnalyzeStrings enables string duplicate analysis.
	AnalyzeStrings bool
	// AnalyzeArrays enables array analysis.
	AnalyzeArrays bool
	// MaxLargestObjects is the number of largest objects to track.
	MaxLargestObjects int
	// AnalyzeRetainers enables retainer analysis (who holds references to objects).
	AnalyzeRetainers bool
	// TopRetainersN is the number of top retainers to track per class.
	TopRetainersN int
}

// DefaultParserOptions returns default parser options.
func DefaultParserOptions() *ParserOptions {
	return &ParserOptions{
		TopClassesN:       100,
		AnalyzeStrings:    true,
		AnalyzeArrays:     true,
		MaxLargestObjects: 50,
		AnalyzeRetainers:  true,
		TopRetainersN:     10,
	}
}

// Parser parses HPROF heap dump files.
type Parser struct {
	opts *ParserOptions
}

// NewParser creates a new HPROF parser.
func NewParser(opts *ParserOptions) *Parser {
	if opts == nil {
		opts = DefaultParserOptions()
	}
	return &Parser{opts: opts}
}

// parserState holds the parsing state.
type parserState struct {
	reader         *Reader
	header         *Header
	strings        map[uint64]string     // ID -> string value
	classNames     map[uint64]uint64     // classID -> nameStringID
	classInfo      map[uint64]*ClassInfo // classID -> class info
	classByName    map[string]*ClassInfo // className -> class info
	heapSummary    *HeapSummary
	totalHeapSize  int64
	totalInstances int64
	// Reference tracking for retainer analysis
	refGraph       *ReferenceGraph
	classFields    map[uint64][]FieldDescriptor // classID -> field descriptors
	// Debug counters
	classDumpCount    int64
	instanceDumpCount int64
	arrayDumpCount    int64
	loadClassCount    int64
	unknownTagCount   int64
	skippedBytes      int64
}

// FieldDescriptor describes a field in a class.
type FieldDescriptor struct {
	NameID uint64
	Type   BasicType
}

// newParserState creates a new parser state.
func newParserState(r *Reader, analyzeRetainers bool) *parserState {
	state := &parserState{
		reader:      r,
		strings:     make(map[uint64]string),
		classNames:  make(map[uint64]uint64),
		classInfo:   make(map[uint64]*ClassInfo),
		classByName: make(map[string]*ClassInfo),
		classFields: make(map[uint64][]FieldDescriptor),
	}
	if analyzeRetainers {
		state.refGraph = NewReferenceGraph()
	}
	return state
}

// Parse parses an HPROF file and returns analysis results.
func (p *Parser) Parse(ctx context.Context, r io.Reader) (*HeapAnalysisResult, error) {
	reader := NewReader(r)
	state := newParserState(reader, p.opts.AnalyzeRetainers)

	// Read header
	header, err := reader.ReadHeader()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}
	state.header = header

	// Parse all records
	if err := p.parseRecords(ctx, state); err != nil {
		return nil, fmt.Errorf("failed to parse records: %w", err)
	}

	// Build result
	return p.buildResult(state), nil
}

// parseRecords parses all records in the HPROF file.
func (p *Parser) parseRecords(ctx context.Context, state *parserState) error {
	for {
		// Check context cancellation
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
			if err := p.parseStringRecord(state, length); err != nil {
				return err
			}
		case TagLoadClass:
			if err := p.parseLoadClassRecord(state); err != nil {
				return err
			}
		case TagHeapDump, TagHeapDumpSegment:
			if err := p.parseHeapDumpRecord(ctx, state, length); err != nil {
				return err
			}
		case TagHeapSummary:
			if err := p.parseHeapSummaryRecord(state); err != nil {
				return err
			}
		default:
			// Skip unknown records
			if err := state.reader.Skip(int64(length)); err != nil {
				return err
			}
		}
	}
}

// parseStringRecord parses a STRING record.
func (p *Parser) parseStringRecord(state *parserState, length uint32) error {
	id, err := state.reader.ReadID()
	if err != nil {
		return err
	}

	strLen := int(length) - state.reader.IDSize()
	if strLen < 0 {
		return fmt.Errorf("invalid string length: %d", strLen)
	}

	strBytes, err := state.reader.ReadBytes(strLen)
	if err != nil {
		return err
	}

	state.strings[id] = string(strBytes)
	return nil
}

// parseLoadClassRecord parses a LOAD_CLASS record.
func (p *Parser) parseLoadClassRecord(state *parserState) error {
	state.loadClassCount++
	// Class serial number (4 bytes)
	if _, err := state.reader.ReadUint32(); err != nil {
		return err
	}

	// Class object ID
	classID, err := state.reader.ReadID()
	if err != nil {
		return err
	}

	// Stack trace serial number (4 bytes)
	if _, err := state.reader.ReadUint32(); err != nil {
		return err
	}

	// Class name string ID
	nameID, err := state.reader.ReadID()
	if err != nil {
		return err
	}

	state.classNames[classID] = nameID
	return nil
}

// parseHeapDumpRecord parses a HEAP_DUMP or HEAP_DUMP_SEGMENT record.
func (p *Parser) parseHeapDumpRecord(ctx context.Context, state *parserState, length uint32) error {
	endPos := int64(length)
	var bytesRead int64

	for bytesRead < endPos {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		tagByte, err := state.reader.ReadByte()
		if err != nil {
			return err
		}
		bytesRead++

		tag := HeapDumpTag(tagByte)
		n, err := p.parseHeapDumpSubRecord(state, tag, endPos-bytesRead)
		if err != nil {
			// For unknown tags, try to skip remaining bytes and continue
			if isUnknownTagError(err) {
				state.unknownTagCount++
				remaining := endPos - bytesRead
				state.skippedBytes += remaining
				if remaining > 0 {
					if skipErr := state.reader.Skip(remaining); skipErr != nil {
						return skipErr
					}
				}
				return nil
			}
			return err
		}
		bytesRead += n
	}

	return nil
}

// isUnknownTagError checks if the error is due to an unknown tag.
func isUnknownTagError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "unknown heap dump tag") ||
		strings.Contains(err.Error(), "skipping unknown tag")
}

// parseHeapDumpSubRecord parses a sub-record within a heap dump.
// remainingBytes is the number of bytes remaining in the heap dump record.
func (p *Parser) parseHeapDumpSubRecord(state *parserState, tag HeapDumpTag, remainingBytes int64) (int64, error) {
	idSize := state.reader.IDSize()

	switch tag {
	case 0x00:
		// Padding byte or end marker, skip it
		return 0, nil

	// Additional root types from Android/OpenJDK HPROF format
	case 0x89: // ROOT_INTERNED_STRING
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootStickyClass, // Treat as sticky class
			})
		}
		return int64(idSize), nil

	case 0x8A: // ROOT_FINALIZING
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootUnknown,
			})
		}
		return int64(idSize), nil

	case 0x8B: // ROOT_DEBUGGER
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootUnknown,
			})
		}
		return int64(idSize), nil

	case 0x8C: // ROOT_REFERENCE_CLEANUP
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootUnknown,
			})
		}
		return int64(idSize), nil

	case 0x8D: // ROOT_VM_INTERNAL
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootUnknown,
			})
		}
		return int64(idSize), nil

	case 0x8E: // ROOT_JNI_MONITOR
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		// Skip thread serial (4 bytes) and stack depth (4 bytes)
		if err := state.reader.Skip(8); err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootMonitorUsed,
			})
		}
		return int64(idSize + 8), nil

	case 0xC3: // HEAP_DUMP_INFO (Android specific)
		// heap type (4 bytes) + heap name string ID
		if _, err := state.reader.ReadUint32(); err != nil {
			return 0, err
		}
		if _, err := state.reader.ReadID(); err != nil {
			return 0, err
		}
		return int64(4 + idSize), nil

	case 0xFE: // ROOT_UNREACHABLE (some JVMs)
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootUnknown,
			})
		}
		return int64(idSize), nil

	case HeapTagRootUnknown:
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootUnknown,
			})
		}
		return int64(idSize), nil

	case HeapTagRootJNIGlobal:
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if err := state.reader.Skip(int64(idSize)); err != nil { // JNI global ref ID
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootJNIGlobal,
			})
		}
		return int64(idSize * 2), nil

	case HeapTagRootJNILocal:
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		threadSerial, err := state.reader.ReadUint32()
		if err != nil {
			return 0, err
		}
		frameIndex, err := state.reader.ReadUint32()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID:   objectID,
				Type:       GCRootJNILocal,
				ThreadID:   uint64(threadSerial),
				FrameIndex: int(frameIndex),
			})
		}
		return int64(idSize + 8), nil

	case HeapTagRootJavaFrame:
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		threadSerial, err := state.reader.ReadUint32()
		if err != nil {
			return 0, err
		}
		frameIndex, err := state.reader.ReadUint32()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID:   objectID,
				Type:       GCRootJavaFrame,
				ThreadID:   uint64(threadSerial),
				FrameIndex: int(frameIndex),
			})
		}
		return int64(idSize + 8), nil

	case HeapTagRootNativeStack:
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		threadSerial, err := state.reader.ReadUint32()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootNativeStack,
				ThreadID: uint64(threadSerial),
			})
		}
		return int64(idSize + 4), nil

	case HeapTagRootThreadBlock:
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		threadSerial, err := state.reader.ReadUint32()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootThreadBlock,
				ThreadID: uint64(threadSerial),
			})
		}
		return int64(idSize + 4), nil

	case HeapTagRootStickyClass:
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootStickyClass,
			})
		}
		return int64(idSize), nil

	case HeapTagRootMonitorUsed:
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootMonitorUsed,
			})
		}
		return int64(idSize), nil

	case HeapTagRootThreadObject:
		objectID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		threadSerial, err := state.reader.ReadUint32()
		if err != nil {
			return 0, err
		}
		if state.refGraph != nil {
			state.refGraph.AddGCRoot(&GCRoot{
				ObjectID: objectID,
				Type:     GCRootThreadObject,
				ThreadID: uint64(threadSerial),
			})
		}
		// Skip remaining 4 bytes (stack trace serial)
		if err := state.reader.Skip(4); err != nil {
			return 0, err
		}
		return int64(idSize + 8), nil

	case HeapTagClassDump:
		return p.parseClassDump(state)

	case HeapTagInstanceDump:
		return p.parseInstanceDump(state)

	case HeapTagObjectArrayDump:
		return p.parseObjectArrayDump(state)

	case HeapTagPrimitiveArrayDump:
		return p.parsePrimitiveArrayDump(state)

	default:
		// For unknown tags, we cannot determine the size, so we signal to skip remaining
		return 0, fmt.Errorf("skipping unknown tag: 0x%02X", tag)
	}
}

// parseClassDump parses a CLASS_DUMP sub-record.
func (p *Parser) parseClassDump(state *parserState) (int64, error) {
	state.classDumpCount++
	idSize := state.reader.IDSize()
	var bytesRead int64

	// Class object ID
	classID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Stack trace serial number
	if _, err := state.reader.ReadUint32(); err != nil {
		return 0, err
	}
	bytesRead += 4

	// Super class object ID
	superClassID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Class loader, signers, protection domain, reserved1, reserved2 (5 IDs)
	if err := state.reader.Skip(int64(idSize * 5)); err != nil {
		return 0, err
	}
	bytesRead += int64(idSize * 5)

	// Instance size
	instanceSize, err := state.reader.ReadUint32()
	if err != nil {
		return 0, err
	}
	bytesRead += 4

	// Constant pool size
	cpSize, err := state.reader.ReadUint16()
	if err != nil {
		return 0, err
	}
	bytesRead += 2

	// Skip constant pool entries
	for i := 0; i < int(cpSize); i++ {
		// Constant pool index
		if _, err := state.reader.ReadUint16(); err != nil {
			return 0, err
		}
		bytesRead += 2

		// Type
		typeByte, err := state.reader.ReadByte()
		if err != nil {
			return 0, err
		}
		bytesRead++

		// Value
		valueSize := BasicTypeSize(BasicType(typeByte), idSize)
		if err := state.reader.Skip(int64(valueSize)); err != nil {
			return 0, err
		}
		bytesRead += int64(valueSize)
	}

	// Static fields count
	staticFieldsCount, err := state.reader.ReadUint16()
	if err != nil {
		return 0, err
	}
	bytesRead += 2

	// Skip static fields
	for i := 0; i < int(staticFieldsCount); i++ {
		// Field name ID
		if err := state.reader.Skip(int64(idSize)); err != nil {
			return 0, err
		}
		bytesRead += int64(idSize)

		// Type
		typeByte, err := state.reader.ReadByte()
		if err != nil {
			return 0, err
		}
		bytesRead++

		// Value
		valueSize := BasicTypeSize(BasicType(typeByte), idSize)
		if err := state.reader.Skip(int64(valueSize)); err != nil {
			return 0, err
		}
		bytesRead += int64(valueSize)
	}

	// Instance fields count
	instanceFieldsCount, err := state.reader.ReadUint16()
	if err != nil {
		return 0, err
	}
	bytesRead += 2

	// Read instance fields (name ID + type) - need to store for retainer analysis
	var fields []FieldDescriptor
	for i := 0; i < int(instanceFieldsCount); i++ {
		nameID, err := state.reader.ReadID()
		if err != nil {
			return 0, err
		}
		bytesRead += int64(idSize)

		typeByte, err := state.reader.ReadByte()
		if err != nil {
			return 0, err
		}
		bytesRead++

		fields = append(fields, FieldDescriptor{
			NameID: nameID,
			Type:   BasicType(typeByte),
		})
	}

	// Store field descriptors for retainer analysis
	state.classFields[classID] = fields

	// Get class name
	className := p.getClassName(state, classID)

	// Store class info
	state.classInfo[classID] = &ClassInfo{
		ClassID:          classID,
		Name:             className,
		SuperClassID:     superClassID,
		InstanceSize:     int(instanceSize),
		FieldCount:       int(instanceFieldsCount),
		StaticFieldCount: int(staticFieldsCount),
	}

	if _, exists := state.classByName[className]; !exists {
		state.classByName[className] = state.classInfo[classID]
	}

	// Store class name in reference graph
	if state.refGraph != nil && className != "" {
		state.refGraph.SetClassName(classID, className)
	}

	return bytesRead, nil
}

// parseInstanceDump parses an INSTANCE_DUMP sub-record.
func (p *Parser) parseInstanceDump(state *parserState) (int64, error) {
	state.instanceDumpCount++
	idSize := state.reader.IDSize()
	var bytesRead int64

	// Object ID
	objectID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Stack trace serial number
	if _, err := state.reader.ReadUint32(); err != nil {
		return 0, err
	}
	bytesRead += 4

	// Class object ID
	classID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Number of bytes that follow
	dataSize, err := state.reader.ReadUint32()
	if err != nil {
		return 0, err
	}
	bytesRead += 4

	// Read instance data for reference extraction
	var instanceData []byte
	if state.refGraph != nil && dataSize > 0 {
		instanceData, err = state.reader.ReadBytes(int(dataSize))
		if err != nil {
			return 0, err
		}
	} else {
		// Skip instance data
		if err := state.reader.Skip(int64(dataSize)); err != nil {
			return 0, err
		}
	}
	bytesRead += int64(dataSize)

	// Calculate instance size
	var instanceSize int64
	if info, ok := state.classInfo[classID]; ok {
		info.InstanceCount++
		instanceSize = int64(info.InstanceSize) + int64(idSize) + 4 + int64(dataSize)
		info.TotalSize += instanceSize
		state.totalHeapSize += instanceSize
	} else {
		// Class info not found (CLASS_DUMP not yet processed for this class)
		instanceSize = int64(idSize) + 4 + int64(dataSize)
		state.totalHeapSize += instanceSize
		
		// Try to get class name from LOAD_CLASS records
		className := p.getClassName(state, classID)
		if className != "" {
			// Add to classByName if not exists
			if _, exists := state.classByName[className]; !exists {
				state.classByName[className] = &ClassInfo{
					ClassID:       classID,
					Name:          className,
					InstanceCount: 0,
					TotalSize:     0,
				}
			}
			state.classByName[className].InstanceCount++
			state.classByName[className].TotalSize += instanceSize
			
			// Also register in reference graph
			if state.refGraph != nil {
				state.refGraph.SetClassName(classID, className)
			}
		}
	}
	state.totalInstances++

	// Extract references for retainer analysis
	if state.refGraph != nil && len(instanceData) > 0 {
		state.refGraph.SetObjectInfo(objectID, classID, instanceSize)
		p.extractReferences(state, objectID, classID, instanceData)
	}

	return bytesRead, nil
}

// extractReferences extracts object references from instance data.
func (p *Parser) extractReferences(state *parserState, objectID, classID uint64, data []byte) {
	idSize := state.reader.IDSize()
	
	// Get all fields for this class hierarchy
	allFields := p.getClassHierarchyFields(state, classID)
	
	offset := 0
	for _, field := range allFields {
		fieldSize := BasicTypeSize(field.Type, idSize)
		if offset+fieldSize > len(data) {
			break
		}
		
		// Only track object references
		if field.Type == TypeObject && fieldSize == idSize {
			var refID uint64
			if idSize == 4 {
				refID = uint64(data[offset])<<24 | uint64(data[offset+1])<<16 |
					uint64(data[offset+2])<<8 | uint64(data[offset+3])
			} else {
				refID = uint64(data[offset])<<56 | uint64(data[offset+1])<<48 |
					uint64(data[offset+2])<<40 | uint64(data[offset+3])<<32 |
					uint64(data[offset+4])<<24 | uint64(data[offset+5])<<16 |
					uint64(data[offset+6])<<8 | uint64(data[offset+7])
			}
			
			if refID != 0 {
				fieldName := state.strings[field.NameID]
				state.refGraph.AddReference(ObjectReference{
					FromObjectID: objectID,
					ToObjectID:   refID,
					FieldName:    fieldName,
					FromClassID:  classID,
				})
			}
		}
		offset += fieldSize
	}
}

// getClassHierarchyFields returns all fields for a class and its superclasses.
func (p *Parser) getClassHierarchyFields(state *parserState, classID uint64) []FieldDescriptor {
	var allFields []FieldDescriptor
	
	currentClassID := classID
	for currentClassID != 0 {
		if fields, ok := state.classFields[currentClassID]; ok {
			// Prepend superclass fields (they come first in instance data)
			allFields = append(fields, allFields...)
		}
		
		if info, ok := state.classInfo[currentClassID]; ok {
			currentClassID = info.SuperClassID
		} else {
			break
		}
	}
	
	return allFields
}

// parseObjectArrayDump parses an OBJECT_ARRAY_DUMP sub-record.
func (p *Parser) parseObjectArrayDump(state *parserState) (int64, error) {
	state.arrayDumpCount++
	idSize := state.reader.IDSize()
	var bytesRead int64

	// Array object ID
	arrayObjectID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Stack trace serial number
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
	classID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Read or skip element IDs
	elemBytes := int64(numElements) * int64(idSize)
	var elemData []byte
	if state.refGraph != nil && numElements > 0 {
		elemData, err = state.reader.ReadBytes(int(elemBytes))
		if err != nil {
			return 0, err
		}
	} else {
		if err := state.reader.Skip(elemBytes); err != nil {
			return 0, err
		}
	}
	bytesRead += elemBytes

	// Update statistics
	arraySize := int64(idSize) + 4 + 4 + int64(idSize) + elemBytes
	state.totalHeapSize += arraySize
	state.totalInstances++

	// Update class statistics for array type
	className := p.getClassName(state, classID)
	if className == "" {
		className = "Object[]"
	}
	if info, ok := state.classByName[className]; ok {
		info.InstanceCount++
		info.TotalSize += arraySize
	} else {
		state.classByName[className] = &ClassInfo{
			ClassID:       classID,
			Name:          className,
			InstanceCount: 1,
			TotalSize:     arraySize,
		}
	}
	
	// Register class name in reference graph
	if state.refGraph != nil && className != "" {
		state.refGraph.SetClassName(classID, className)
	}

	// Extract array element references
	if state.refGraph != nil && len(elemData) > 0 {
		state.refGraph.SetObjectInfo(arrayObjectID, classID, arraySize)
		for i := 0; i < int(numElements); i++ {
			offset := i * idSize
			var refID uint64
			if idSize == 4 {
				refID = uint64(elemData[offset])<<24 | uint64(elemData[offset+1])<<16 |
					uint64(elemData[offset+2])<<8 | uint64(elemData[offset+3])
			} else {
				refID = uint64(elemData[offset])<<56 | uint64(elemData[offset+1])<<48 |
					uint64(elemData[offset+2])<<40 | uint64(elemData[offset+3])<<32 |
					uint64(elemData[offset+4])<<24 | uint64(elemData[offset+5])<<16 |
					uint64(elemData[offset+6])<<8 | uint64(elemData[offset+7])
			}
			if refID != 0 {
				state.refGraph.AddReference(ObjectReference{
					FromObjectID: arrayObjectID,
					ToObjectID:   refID,
					FieldName:    fmt.Sprintf("[%d]", i),
					FromClassID:  classID,
				})
			}
		}
	}

	return bytesRead, nil
}

// parsePrimitiveArrayDump parses a PRIMITIVE_ARRAY_DUMP sub-record.
func (p *Parser) parsePrimitiveArrayDump(state *parserState) (int64, error) {
	idSize := state.reader.IDSize()
	var bytesRead int64

	// Array object ID
	arrayObjectID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Stack trace serial number
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

	// Element type
	elemType, err := state.reader.ReadByte()
	if err != nil {
		return 0, err
	}
	bytesRead++

	// Calculate and skip element data
	elemSize := BasicTypeSize(BasicType(elemType), idSize)
	dataBytes := int64(numElements) * int64(elemSize)
	if err := state.reader.Skip(dataBytes); err != nil {
		return 0, err
	}
	bytesRead += dataBytes

	// Update statistics
	arraySize := int64(idSize) + 4 + 4 + 1 + dataBytes
	state.totalHeapSize += arraySize
	state.totalInstances++

	// Get array type name
	typeName := primitiveArrayTypeName(BasicType(elemType))
	
	// Get or create class ID for this primitive array type
	var classID uint64
	if info, ok := state.classByName[typeName]; ok {
		info.InstanceCount++
		info.TotalSize += arraySize
		classID = info.ClassID
	} else {
		// Use a synthetic class ID for primitive arrays
		classID = uint64(0x1000000 + int(elemType))
		state.classByName[typeName] = &ClassInfo{
			ClassID:       classID,
			Name:          typeName,
			InstanceCount: 1,
			TotalSize:     arraySize,
		}
		// Register class name in reference graph
		if state.refGraph != nil {
			state.refGraph.SetClassName(classID, typeName)
		}
	}

	// Register this array object for retainer analysis
	if state.refGraph != nil {
		state.refGraph.SetObjectInfo(arrayObjectID, classID, arraySize)
	}

	return bytesRead, nil
}

// parseHeapSummaryRecord parses a HEAP_SUMMARY record.
func (p *Parser) parseHeapSummaryRecord(state *parserState) error {
	totalLiveBytes, err := state.reader.ReadUint32()
	if err != nil {
		return err
	}

	totalLiveInstances, err := state.reader.ReadUint32()
	if err != nil {
		return err
	}

	totalAllocBytes, err := state.reader.ReadUint64()
	if err != nil {
		return err
	}

	totalAllocInstances, err := state.reader.ReadUint64()
	if err != nil {
		return err
	}

	state.heapSummary = &HeapSummary{
		TotalLiveBytes:    int64(totalLiveBytes),
		TotalLiveObjects:  int64(totalLiveInstances),
		TotalAllocBytes:   int64(totalAllocBytes),
		TotalAllocObjects: int64(totalAllocInstances),
	}

	return nil
}

// getClassName returns the class name for a class ID.
func (p *Parser) getClassName(state *parserState, classID uint64) string {
	if nameID, ok := state.classNames[classID]; ok {
		if name, ok := state.strings[nameID]; ok {
			return normalizeClassName(name)
		}
	}
	return ""
}

// buildResult builds the final analysis result.
func (p *Parser) buildResult(state *parserState) *HeapAnalysisResult {
	// Collect all classes with statistics
	var classes []*ClassStats
	for _, info := range state.classByName {
		if info.InstanceCount > 0 {
			avgSize := float64(0)
			if info.InstanceCount > 0 {
				avgSize = float64(info.TotalSize) / float64(info.InstanceCount)
			}
			pct := float64(0)
			if state.totalHeapSize > 0 {
				pct = float64(info.TotalSize) * 100.0 / float64(state.totalHeapSize)
			}

			classes = append(classes, &ClassStats{
				ClassName:     info.Name,
				InstanceCount: info.InstanceCount,
				TotalSize:     info.TotalSize,
				AvgSize:       avgSize,
				Percentage:    pct,
				ShallowSize:   info.TotalSize,
			})
		}
	}

	// Sort by total size descending
	sort.Slice(classes, func(i, j int) bool {
		return classes[i].TotalSize > classes[j].TotalSize
	})

	// Limit to top N
	topClasses := classes
	if len(topClasses) > p.opts.TopClassesN {
		topClasses = topClasses[:p.opts.TopClassesN]
	}

	result := &HeapAnalysisResult{
		Header:         state.header,
		Summary:        state.heapSummary,
		TopClasses:     topClasses,
		TotalClasses:   len(state.classByName),
		TotalInstances: state.totalInstances,
		TotalHeapSize:  state.totalHeapSize,
	}

	// Compute retainer analysis and reference graphs for top classes
	if state.refGraph != nil && p.opts.AnalyzeRetainers {
		// Debug: print parsing stats
		fmt.Printf("[DEBUG] Parsing stats: loadClass=%d, classDump=%d, instanceDump=%d, arrayDump=%d\n",
			state.loadClassCount, state.classDumpCount, state.instanceDumpCount, state.arrayDumpCount)
		fmt.Printf("[DEBUG] Unknown tags: %d, skipped bytes: %d\n", state.unknownTagCount, state.skippedBytes)
		
		// Debug: print reference graph stats
		objects, refs, gcRoots, objectsWithIncoming := state.refGraph.GetStats()
		fmt.Printf("[DEBUG] Reference graph stats: objects=%d, refs=%d, gcRoots=%d, objectsWithIncoming=%d\n", 
			objects, refs, gcRoots, objectsWithIncoming)
		
		// Debug: check class field info
		classesWithFields := 0
		totalFields := 0
		for _, fields := range state.classFields {
			if len(fields) > 0 {
				classesWithFields++
				totalFields += len(fields)
			}
		}
		fmt.Printf("[DEBUG] Classes with field info: %d, total fields: %d\n", classesWithFields, totalFields)
		fmt.Printf("[DEBUG] ClassInfo entries: %d, ClassFields entries: %d\n", len(state.classInfo), len(state.classFields))
		
		// Only analyze top 20 classes for performance
		topForRetainers := topClasses
		if len(topForRetainers) > 20 {
			topForRetainers = topForRetainers[:20]
		}
		result.ClassRetainers = state.refGraph.ComputeTopRetainers(topForRetainers, p.opts.TopRetainersN)

		// Generate reference graphs for top 5 classes (with increased depth)
		result.ReferenceGraphs = make(map[string]*ReferenceGraphData)
		topForGraphs := topClasses
		if len(topForGraphs) > 5 {
			topForGraphs = topForGraphs[:5]
		}
		for _, cls := range topForGraphs {
			graphData := state.refGraph.GetReferenceGraphForClass(cls.ClassName, 10, 100)
			if graphData != nil && len(graphData.Nodes) > 0 {
				result.ReferenceGraphs[cls.ClassName] = graphData
			}
		}

		// Compute business-level retainers for root cause analysis
		result.BusinessRetainers = make(map[string][]*BusinessRetainer)
		topForBusiness := topClasses
		if len(topForBusiness) > 10 {
			topForBusiness = topForBusiness[:10]
		}
		for _, cls := range topForBusiness {
			businessRetainers := state.refGraph.ComputeBusinessRetainers(cls.ClassName, 15, 10)
			if len(businessRetainers) > 0 {
				result.BusinessRetainers[cls.ClassName] = businessRetainers
			}
		}
	}

	return result
}

// normalizeClassName converts JVM internal class name to readable format.
func normalizeClassName(name string) string {
	// Convert slashes to dots
	name = strings.ReplaceAll(name, "/", ".")

	// Handle array types
	if strings.HasPrefix(name, "[") {
		return parseArrayTypeName(name)
	}

	return name
}

// parseArrayTypeName converts array type descriptors to readable names.
func parseArrayTypeName(name string) string {
	dims := 0
	for strings.HasPrefix(name, "[") {
		dims++
		name = name[1:]
	}

	var baseName string
	switch {
	case strings.HasPrefix(name, "L") && strings.HasSuffix(name, ";"):
		baseName = strings.ReplaceAll(name[1:len(name)-1], "/", ".")
	case name == "Z":
		baseName = "boolean"
	case name == "B":
		baseName = "byte"
	case name == "C":
		baseName = "char"
	case name == "S":
		baseName = "short"
	case name == "I":
		baseName = "int"
	case name == "J":
		baseName = "long"
	case name == "F":
		baseName = "float"
	case name == "D":
		baseName = "double"
	default:
		baseName = name
	}

	return baseName + strings.Repeat("[]", dims)
}

// primitiveArrayTypeName returns the type name for a primitive array.
func primitiveArrayTypeName(t BasicType) string {
	switch t {
	case TypeBoolean:
		return "boolean[]"
	case TypeByte:
		return "byte[]"
	case TypeChar:
		return "char[]"
	case TypeShort:
		return "short[]"
	case TypeInt:
		return "int[]"
	case TypeLong:
		return "long[]"
	case TypeFloat:
		return "float[]"
	case TypeDouble:
		return "double[]"
	default:
		return "unknown[]"
	}
}
