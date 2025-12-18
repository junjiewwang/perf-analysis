package hprof

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/perf-analysis/pkg/utils"
)

// SizeCalculationMode defines how shallow sizes are calculated.
type SizeCalculationMode int

const (
	// SizeModeCompressedOops uses compressed oops (12-byte header, 4-byte refs).
	// This matches IDEA's behavior and is the default for JVMs with heaps < 32GB.
	SizeModeCompressedOops SizeCalculationMode = iota
	// SizeModeNonCompressed uses non-compressed oops (16-byte header, 8-byte refs).
	// This matches MAT's behavior.
	SizeModeNonCompressed
	// SizeModeAuto automatically detects based on heap size (not implemented yet).
	SizeModeAuto
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
	// ParallelConfig configures parallel analysis.
	ParallelConfig ParallelConfig
	// SizeMode controls how shallow sizes are calculated.
	// Default is SizeModeCompressedOops to match IDEA behavior.
	SizeMode SizeCalculationMode
	// FastMode skips deep analysis (business retainers, multi-level retainers, reference graphs).
	// Only computes class histogram, basic retainer info, and dominator tree.
	// This can reduce analysis time by 70-90% for large heaps.
	FastMode bool
	// SkipBusinessRetainers skips only business retainer analysis (the most expensive part).
	// Retainer analysis and reference graphs are still computed.
	SkipBusinessRetainers bool
	// Logger is used for debug logging. If nil, debug logs are suppressed.
	Logger utils.Logger
	// IncludeUnreachable includes unreachable objects in the histogram (like IDEA).
	// Default is false (only show reachable objects, like MAT).
	IncludeUnreachable bool
	// Verbose enables verbose debug output including detailed retained size analysis.
	// This is typically enabled via the -v command line flag.
	Verbose bool
}

// DefaultParserOptions returns default parser options.
func DefaultParserOptions() *ParserOptions {
	return &ParserOptions{
		TopClassesN:        50,  // 0 means no limit - return all classes
		AnalyzeStrings:     true,
		AnalyzeArrays:      true,
		MaxLargestObjects:  100, // Increased to show more objects in Biggest Objects view
		AnalyzeRetainers:   true,
		TopRetainersN:      10,
		ParallelConfig:     DefaultParallelConfig(),
		SizeMode:           SizeModeCompressedOops, // Default to IDEA-compatible mode
		IncludeUnreachable: true,                   // Default to include all objects (like IDEA)
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

// debugf logs a debug message if logger is configured.
func (p *Parser) debugf(format string, args ...interface{}) {
	if p.opts.Logger != nil {
		p.opts.Logger.Debug(format, args...)
	}
}

// deferredInstance holds instance data for deferred reference extraction.
type deferredInstance struct {
	objectID uint64
	classID  uint64
	data     []byte
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
	refGraph    *ReferenceGraph
	classFields map[uint64][]FieldDescriptor // classID -> field descriptors
	// Class layouts for BiggestObjects feature
	classLayouts map[uint64]*ClassFieldLayout // classID -> field layout
	// Deferred reference extraction for instances parsed before their CLASS_DUMP
	deferredInstances []deferredInstance
	// Size calculation mode
	sizeMode SizeCalculationMode
	// java.lang.Class classID - used to properly categorize Class objects
	javaLangClassID uint64
	// Debug counters
	classDumpCount    int64
	instanceDumpCount int64
	arrayDumpCount    int64
	loadClassCount    int64
	unknownTagCount   int64
	skippedBytes      int64
	deferredCount     int64 // count of deferred instances
}

// objectHeaderSize returns the size of object header in JVM.
// For HotSpot JVM with compressed oops (default for heaps < 32GB):
// - mark word (8 bytes) + klass pointer (4 bytes) = 12 bytes
// For HotSpot JVM without compressed oops (heaps >= 32GB):
// - mark word (8 bytes) + klass pointer (8 bytes) = 16 bytes
func objectHeaderSize(mode SizeCalculationMode) int64 {
	if mode == SizeModeNonCompressed {
		return 16 // MAT-compatible: 8 (mark) + 8 (klass)
	}
	return 12 // IDEA-compatible: 8 (mark) + 4 (compressed klass)
}

// referenceSize returns the size of a reference field in JVM.
func referenceSize(mode SizeCalculationMode) int64 {
	if mode == SizeModeNonCompressed {
		return 8 // MAT-compatible: non-compressed oops
	}
	return 4 // IDEA-compatible: compressed oops
}

// classObjectShallowSize returns the shallow size of a java.lang.Class object.
// Class objects have a complex internal structure that varies by JVM version.
// MAT calculates this as: object header + internal fields
// For HotSpot JVM, Class objects contain:
// - Object header (12/16 bytes depending on compressed oops)
// - Various internal fields (classLoader, module, name, etc.)
// - Static field values storage
func classObjectShallowSize(mode SizeCalculationMode, staticFieldsCount int) int64 {
	// Base Class object size (header + core fields)
	baseSize := objectHeaderSize(mode)
	refSize := referenceSize(mode)

	// Core fields in java.lang.Class (approximate):
	// - classLoader, module, name, packageName, etc. (reference fields)
	// - Various int/boolean fields for flags
	// Estimate: ~15 reference fields + ~20 bytes of primitives
	coreFields := int64(15)*refSize + 20

	// Static fields are stored in the Class object
	staticStorage := int64(staticFieldsCount) * refSize

	return alignTo8(baseSize + coreFields + staticStorage)
}

// arrayHeaderSize returns the size of array header in JVM.
// Array header = object header + array length (4 bytes)
// For compressed oops: 12 + 4 = 16 bytes
// For non-compressed: 16 + 4 = 20 bytes (aligned to 24)
func arrayHeaderSize(mode SizeCalculationMode) int64 {
	return objectHeaderSize(mode) + 4
}

// alignTo8 aligns a size to 8-byte boundary (JVM object alignment).
func alignTo8(size int64) int64 {
	return (size + 7) &^ 7
}

// FieldDescriptor describes a field in a class.
type FieldDescriptor struct {
	NameID uint64
	Type   BasicType
}

// newParserState creates a new parser state.
func newParserState(r *Reader, opts *ParserOptions) *parserState {
	state := &parserState{
		reader:            r,
		strings:           make(map[uint64]string),
		classNames:        make(map[uint64]uint64),
		classInfo:         make(map[uint64]*ClassInfo),
		classByName:       make(map[string]*ClassInfo),
		classFields:       make(map[uint64][]FieldDescriptor),
		classLayouts:      make(map[uint64]*ClassFieldLayout),
		deferredInstances: make([]deferredInstance, 0),
		sizeMode:          opts.SizeMode,
	}
	if opts.AnalyzeRetainers {
		state.refGraph = NewReferenceGraph()
		if opts.Logger != nil {
			state.refGraph.SetLogger(opts.Logger)
		}
	}
	return state
}

// Parse parses an HPROF file and returns analysis results.
func (p *Parser) Parse(ctx context.Context, r io.Reader) (*HeapAnalysisResult, error) {
	// Create timer for performance tracking (uses dependency injection via Logger)
	timer := utils.NewTimer("HPROF Parse", utils.WithLogger(p.opts.Logger), utils.WithEnabled(p.opts.Logger != nil))

	reader := NewReader(r)
	state := newParserState(reader, p.opts)

	// Read header
	header, err := reader.ReadHeader()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}
	state.header = header

	// Phase 1: Parse all records
	pt := timer.Start("Parse HPROF records")
	if err := p.parseRecords(ctx, state); err != nil {
		return nil, fmt.Errorf("failed to parse records: %w", err)
	}
	pt.Stop()

	// Process deferred instances (those parsed before their CLASS_DUMP)
	// This ensures all references are extracted even when INSTANCE_DUMP appears before CLASS_DUMP
	timer.TimeFunc("Process deferred instances", func() {
		p.processDeferredInstances(state)
	})

	// Fix Class object categorization: all Class objects should be instances of java.lang.Class
	p.fixClassObjectCategorization(state)

	// Phase 2: Build result (includes dominator tree computation and analysis)
	var result *HeapAnalysisResult
	timer.TimeFunc("Build result", func() {
		result = p.buildResult(state, timer)
	})

	// Print timing summary
	timer.PrintSummary()

	return result, nil
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

	// Class loader object ID
	classLoaderID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Signers object ID
	signersID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Protection domain object ID
	protectionDomainID, err := state.reader.ReadID()
	if err != nil {
		return 0, err
	}
	bytesRead += int64(idSize)

	// Reserved1, Reserved2 (skip these)
	if err := state.reader.Skip(int64(idSize * 2)); err != nil {
		return 0, err
	}
	bytesRead += int64(idSize * 2)

	// Add references from Class object to superclass, classloader, etc.
	if state.refGraph != nil {
		if superClassID != 0 {
			state.refGraph.AddReference(ObjectReference{
				FromObjectID: classID,
				ToObjectID:   superClassID,
				FieldName:    "<superclass>",
				FromClassID:  classID,
			})
		}
		if classLoaderID != 0 {
			// Reference from Class to its ClassLoader (Class.classLoader field)
			state.refGraph.AddReference(ObjectReference{
				FromObjectID: classID,
				ToObjectID:   classLoaderID,
				FieldName:    "<classloader>",
				FromClassID:  classID,
			})
			// Also add reverse reference: ClassLoader -> Class
			// This models the fact that ClassLoader holds references to all classes it loaded.
			// This is important for correct dominator tree calculation:
			// Classes should be dominated by their ClassLoader, not by super root.
			state.refGraph.AddReference(ObjectReference{
				FromObjectID: classLoaderID,
				ToObjectID:   classID,
				FieldName:    "<loaded_class>",
				FromClassID:  0, // ClassLoader's classID is unknown at this point
			})
			// IMPORTANT: Register the ClassLoader object if not already registered.
			// This ensures ClassLoader objects are included in the object graph even if
			// their INSTANCE_DUMP hasn't been processed yet.
			// Without this, ClassLoader objects might be missing from objectClass,
			// causing Class -> ClassLoader references to be ignored in dominator tree.
			if _, exists := state.refGraph.GetObjectClassID(classLoaderID); !exists {
				// Register with placeholder size (will be updated when INSTANCE_DUMP is processed)
				// Use 0 as classID temporarily (will be corrected when INSTANCE_DUMP is processed)
				state.refGraph.SetObjectInfo(classLoaderID, 0, 0)
			}
		}
		if signersID != 0 {
			state.refGraph.AddReference(ObjectReference{
				FromObjectID: classID,
				ToObjectID:   signersID,
				FieldName:    "<signers>",
				FromClassID:  classID,
			})
		}
		if protectionDomainID != 0 {
			state.refGraph.AddReference(ObjectReference{
				FromObjectID: classID,
				ToObjectID:   protectionDomainID,
				FieldName:    "<protectionDomain>",
				FromClassID:  classID,
			})
		}
	}

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

	// Read static fields - these contain references from the Class object to other objects
	for i := 0; i < int(staticFieldsCount); i++ {
		// Field name ID
		fieldNameID, err := state.reader.ReadID()
		if err != nil {
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

		// If this is an object reference, extract it for the reference graph
		if BasicType(typeByte) == TypeObject && state.refGraph != nil {
			var refID uint64
			if idSize == 4 {
				val, err := state.reader.ReadUint32()
				if err != nil {
					return 0, err
				}
				refID = uint64(val)
			} else {
				val, err := state.reader.ReadUint64()
				if err != nil {
					return 0, err
				}
				refID = val
			}
			bytesRead += int64(valueSize)

			// Add reference from the Class object to the static field value
			if refID != 0 {
				fieldName := state.strings[fieldNameID]
				state.refGraph.AddReference(ObjectReference{
					FromObjectID: classID,
					ToObjectID:   refID,
					FieldName:    fieldName,
					FromClassID:  classID,
				})
			}
		} else {
			// Skip non-object fields
			if err := state.reader.Skip(int64(valueSize)); err != nil {
				return 0, err
			}
			bytesRead += int64(valueSize)
		}
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

	// Track java.lang.Class classID for proper Class object categorization
	if className == "java.lang.Class" {
		state.javaLangClassID = classID
	}

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

	// Build ClassFieldLayout for BiggestObjects feature
	layout := &ClassFieldLayout{
		ClassID:      classID,
		ClassName:    className,
		SuperClassID: superClassID,
		InstanceSize: int(instanceSize),
	}
	// Convert FieldDescriptors to FieldInfo with names
	offset := 0
	for _, fd := range fields {
		fieldName := state.strings[fd.NameID]
		layout.InstanceFields = append(layout.InstanceFields, FieldInfo{
			NameID: fd.NameID,
			Name:   fieldName,
			Type:   fd.Type,
			Offset: offset,
		})
		offset += BasicTypeSize(fd.Type, idSize)
	}
	state.classLayouts[classID] = layout

	// Store class name in reference graph and register the Class object itself
	if state.refGraph != nil {
		if className != "" {
			state.refGraph.SetClassName(classID, className)
		}
		// Register the Class object itself with proper size calculation
		// Use the new classObjectShallowSize function for accurate sizing
		classObjectSize := classObjectShallowSize(state.sizeMode, int(staticFieldsCount))
		// IMPORTANT: Class objects should be categorized as instances of java.lang.Class
		// We defer this until we know the java.lang.Class classID
		// For now, register with self as classID, will be fixed in post-processing
		state.refGraph.SetObjectInfo(classID, classID, classObjectSize)
		// Register this as a Class object - Class objects are implicit GC roots
		// They are held by ClassLoaders and should always be considered reachable
		state.refGraph.RegisterClassObject(classID)
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

	// Number of bytes that follow (instance field data)
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

	// Calculate JVM heap shallow size (not HPROF record size)
	// Shallow size = object header + instance field data, aligned to 8 bytes
	// The dataSize from HPROF is the actual instance field data size
	var shallowSize int64
	if info, ok := state.classInfo[classID]; ok {
		info.InstanceCount++
		// Use the instanceSize from CLASS_DUMP which is the JVM's reported instance size
		// This already includes all instance fields from the class hierarchy
		// Add object header and align to 8 bytes
		shallowSize = alignTo8(objectHeaderSize(state.sizeMode) + int64(info.InstanceSize))
		info.TotalSize += shallowSize
		state.totalHeapSize += shallowSize
	} else {
		// Class info not found (CLASS_DUMP not yet processed for this class)
		// Estimate: object header + field data, aligned to 8 bytes
		shallowSize = alignTo8(objectHeaderSize(state.sizeMode) + int64(dataSize))
		state.totalHeapSize += shallowSize

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
			state.classByName[className].TotalSize += shallowSize

			// Also register in reference graph
			if state.refGraph != nil {
				state.refGraph.SetClassName(classID, className)
			}
		}
	}
	state.totalInstances++

	// Register object info and extract references for retainer analysis
	if state.refGraph != nil {
		// Always register object info, even if no instance data
		// This ensures all objects appear in Biggest Objects list
		state.refGraph.SetObjectInfo(objectID, classID, shallowSize)
		
		// Extract references if there's instance data
		if len(instanceData) > 0 {
			p.extractReferences(state, objectID, classID, instanceData)
		}
	}

	return bytesRead, nil
}

// extractReferences extracts object references from instance data.
func (p *Parser) extractReferences(state *parserState, objectID, classID uint64, data []byte) {
	idSize := state.reader.IDSize()

	// Get all fields for this class hierarchy
	allFields := p.getClassHierarchyFields(state, classID)

	// If no fields found, the CLASS_DUMP might not have been processed yet.
	// Defer this instance for later processing after all CLASS_DUMPs are parsed.
	if len(allFields) == 0 {
		// Only defer if we have data to process (non-empty instance data)
		if len(data) > 0 {
			state.deferredInstances = append(state.deferredInstances, deferredInstance{
				objectID: objectID,
				classID:  classID,
				data:     append([]byte(nil), data...), // copy data
			})
			state.deferredCount++
		}
		return
	}

	p.extractReferencesWithFields(state, objectID, classID, data, allFields, idSize)
}

// extractReferencesWithFields extracts references using known field descriptors.
func (p *Parser) extractReferencesWithFields(state *parserState, objectID, classID uint64, data []byte, allFields []FieldDescriptor, idSize int) {
	offset := 0
	for _, field := range allFields {
		fieldSize := BasicTypeSize(field.Type, idSize)
		if offset+fieldSize > len(data) {
			break
		}

		// Only track object references
		// Note: TypeObject fields should have fieldSize == idSize
		if field.Type == TypeObject {
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
// In HPROF, instance data contains fields in order: current class fields first, then superclass fields.
// This is the reverse of what you might expect!
// See: https://hg.openjdk.java.net/jdk8/jdk8/jdk/file/tip/src/share/demo/jvmti/hprof/hprof_io.c
func (p *Parser) getClassHierarchyFields(state *parserState, classID uint64) []FieldDescriptor {
	var allFields []FieldDescriptor

	// Collect class hierarchy from current class to root
	var classHierarchy []uint64
	currentClassID := classID
	for currentClassID != 0 {
		classHierarchy = append(classHierarchy, currentClassID)
		if info, ok := state.classInfo[currentClassID]; ok {
			currentClassID = info.SuperClassID
		} else {
			break
		}
	}

	// Build fields in order: from current class to root class
	// (NOT reversed - current class fields come first in instance data)
	for _, cid := range classHierarchy {
		if fields, ok := state.classFields[cid]; ok {
			allFields = append(allFields, fields...)
		}
	}

	return allFields
}

// processDeferredInstances processes instances that were deferred because their CLASS_DUMP
// hadn't been parsed yet. This should be called after all records are parsed.
func (p *Parser) processDeferredInstances(state *parserState) {
	if state.refGraph == nil || len(state.deferredInstances) == 0 {
		return
	}

	idSize := state.reader.IDSize()

	for _, inst := range state.deferredInstances {
		allFields := p.getClassHierarchyFields(state, inst.classID)
		if len(allFields) > 0 {
			p.extractReferencesWithFields(state, inst.objectID, inst.classID, inst.data, allFields, idSize)
		}
	}

	// Clear deferred instances to free memory
	state.deferredInstances = nil
}

// fixClassObjectCategorization fixes the classID of all Class objects to be java.lang.Class.
// During parsing, Class objects are temporarily registered with their own classID,
// but they should actually be categorized as instances of java.lang.Class.
func (p *Parser) fixClassObjectCategorization(state *parserState) {
	if state.refGraph == nil || state.javaLangClassID == 0 {
		return
	}

	// Update all Class objects to have java.lang.Class as their classID
	classObjectCount := state.refGraph.FixClassObjectClassIDs(state.javaLangClassID)

	p.debugf("Fixed %d Class objects to be instances of java.lang.Class (classID=%d)",
		classObjectCount, state.javaLangClassID)
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

	// Calculate JVM heap shallow size for object array
	// For object arrays, we use the HPROF-recorded element size (idSize) as the reference size
	// This reflects the actual JVM memory layout at dump time
	// Shallow size = array header (object header + 4 bytes length) + element references, aligned to 8 bytes
	shallowSize := alignTo8(arrayHeaderSize(state.sizeMode) + int64(numElements)*int64(idSize))
	state.totalHeapSize += shallowSize
	state.totalInstances++

	// Update class statistics for array type
	className := p.getClassName(state, classID)
	if className == "" {
		className = fmt.Sprintf("(unknown array 0x%x)", classID)
	}
	if info, ok := state.classByName[className]; ok {
		info.InstanceCount++
		info.TotalSize += shallowSize
	} else {
		state.classByName[className] = &ClassInfo{
			ClassID:       classID,
			Name:          className,
			InstanceCount: 1,
			TotalSize:     shallowSize,
		}
	}

	// Register class name in reference graph
	if state.refGraph != nil && className != "" {
		state.refGraph.SetClassName(classID, className)
	}

	// Extract array element references
	if state.refGraph != nil && len(elemData) > 0 {
		state.refGraph.SetObjectInfo(arrayObjectID, classID, shallowSize)

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

	// Calculate JVM heap shallow size for primitive array
	// Shallow size = array header (object header + 4 bytes length) + element data, aligned to 8 bytes
	shallowSize := alignTo8(arrayHeaderSize(state.sizeMode) + dataBytes)
	state.totalHeapSize += shallowSize
	state.totalInstances++

	// Get array type name
	typeName := primitiveArrayTypeName(BasicType(elemType))

	// Get or create class ID for this primitive array type
	var classID uint64
	if info, ok := state.classByName[typeName]; ok {
		info.InstanceCount++
		info.TotalSize += shallowSize
		classID = info.ClassID
	} else {
		// Use a synthetic class ID for primitive arrays
		classID = uint64(0x1000000 + int(elemType))
		state.classByName[typeName] = &ClassInfo{
			ClassID:       classID,
			Name:          typeName,
			InstanceCount: 1,
			TotalSize:     shallowSize,
		}
		// Register class name in reference graph
		if state.refGraph != nil {
			state.refGraph.SetClassName(classID, typeName)
		}
	}

	// Register this array object for retainer analysis
	if state.refGraph != nil {
		state.refGraph.SetObjectInfo(arrayObjectID, classID, shallowSize)
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
// This delegates to ResultBuilder for cleaner separation of concerns.
func (p *Parser) buildResult(state *parserState, timer *utils.Timer) *HeapAnalysisResult {
	builder := NewResultBuilder(state, p.opts, timer)
	return builder.Build()
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
