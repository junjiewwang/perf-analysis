// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"bytes"
	"fmt"
	"os"
	"time"

	pb "github.com/perf-analysis/internal/parser/hprof/proto"
	"google.golang.org/protobuf/proto"
)

const (
	// SerializerVersion is the current serialization format version
	// Version 2: Added support for zstd compression
	SerializerVersion = 2
	
	// Magic bytes for file format identification
	MagicBytes = "REFG"
)

// SerializeOptions controls serialization behavior.
type SerializeOptions struct {
	// IncludeDominatorData includes precomputed dominator tree data
	IncludeDominatorData bool
	
	// Compression specifies the compression type (zstd or gzip)
	Compression CompressionType
	
	// CompressionLevel controls the compression level (1=fastest, 9=best)
	CompressionLevel CompressionLevel
	
	// SourceFile is the original hprof file name (for metadata)
	SourceFile string
}

// DefaultSerializeOptions returns default serialization options.
// Uses zstd with default compression level for optimal speed/size balance.
func DefaultSerializeOptions() SerializeOptions {
	return SerializeOptions{
		IncludeDominatorData: true,
		Compression:          CompressionZstd,
		CompressionLevel:     CompressionDefault,
		SourceFile:           "",
	}
}

// FastSerializeOptions returns options optimized for speed.
func FastSerializeOptions() SerializeOptions {
	return SerializeOptions{
		IncludeDominatorData: true,
		Compression:          CompressionZstd,
		CompressionLevel:     CompressionFastest,
		SourceFile:           "",
	}
}

// LegacySerializeOptions returns options compatible with older versions (gzip).
func LegacySerializeOptions() SerializeOptions {
	return SerializeOptions{
		IncludeDominatorData: true,
		Compression:          CompressionGzip,
		CompressionLevel:     CompressionDefault,
		SourceFile:           "",
	}
}

// SerializationStats holds statistics about the serialization process.
type SerializationStats struct {
	Objects          int64
	References       int64
	GCRoots          int64
	Classes          int64
	UniqueFieldNames int
	RawSize          int64 // Size before compression
	CompressedSize   int64 // Size after compression
	CompressionRatio float64
	Duration         time.Duration
}

// Serialize serializes the ReferenceGraph to a compressed protobuf format.
// Returns the compressed bytes and serialization statistics.
func (g *ReferenceGraph) Serialize(opts SerializeOptions) ([]byte, *SerializationStats, error) {
	startTime := time.Now()
	stats := &SerializationStats{}
	
	// Build string table for field name deduplication
	fieldNameToIdx := make(map[string]uint32)
	fieldNames := []string{""}  // Index 0 is empty string
	
	getFieldNameIdx := func(name string) uint32 {
		if name == "" {
			return 0
		}
		if idx, ok := fieldNameToIdx[name]; ok {
			return idx
		}
		idx := uint32(len(fieldNames))
		fieldNameToIdx[name] = idx
		fieldNames = append(fieldNames, name)
		return idx
	}
	
	// Build protobuf message
	pbGraph := &pb.ReferenceGraphProto{
		Version: SerializerVersion,
	}
	
	// 1. Serialize objects (objectClass + objectSize)
	pbGraph.Objects = make([]*pb.ObjectInfoProto, 0, len(g.objectClass))
	var totalHeapSize int64
	for objID, classID := range g.objectClass {
		size := g.objectSize[objID]
		totalHeapSize += size
		pbGraph.Objects = append(pbGraph.Objects, &pb.ObjectInfoProto{
			ObjectId: objID,
			ClassId:  classID,
			Size:     size,
		})
	}
	stats.Objects = int64(len(pbGraph.Objects))
	
	// 2. Serialize class names
	pbGraph.ClassNames = make([]*pb.ClassNameEntry, 0, len(g.classNames))
	for classID, className := range g.classNames {
		pbGraph.ClassNames = append(pbGraph.ClassNames, &pb.ClassNameEntry{
			ClassId:   classID,
			ClassName: className,
		})
	}
	stats.Classes = int64(len(pbGraph.ClassNames))
	
	// 3. Serialize references (use outgoingRefs to avoid duplicates)
	totalRefs := 0
	for _, refs := range g.outgoingRefs {
		totalRefs += len(refs)
	}
	pbGraph.References = make([]*pb.ObjectReferenceProto, 0, totalRefs)
	for _, refs := range g.outgoingRefs {
		for _, ref := range refs {
			pbGraph.References = append(pbGraph.References, &pb.ObjectReferenceProto{
				FromObjectId: ref.FromObjectID,
				ToObjectId:   ref.ToObjectID,
				FromClassId:  ref.FromClassID,
				FieldNameIdx: getFieldNameIdx(ref.FieldName),
			})
		}
	}
	stats.References = int64(len(pbGraph.References))
	stats.UniqueFieldNames = len(fieldNames)
	
	// 4. Serialize GC roots
	pbGraph.GcRoots = make([]*pb.GCRootProto, 0, len(g.gcRoots))
	for _, root := range g.gcRoots {
		pbGraph.GcRoots = append(pbGraph.GcRoots, &pb.GCRootProto{
			ObjectId:   root.ObjectID,
			Type:       gcRootTypeToProto(root.Type),
			ThreadId:   root.ThreadID,
			FrameIndex: int32(root.FrameIndex),
		})
	}
	stats.GCRoots = int64(len(pbGraph.GcRoots))
	
	// 5. Serialize dominator data if requested and computed
	if opts.IncludeDominatorData && g.dominatorComputed {
		domData := &pb.DominatorDataProto{
			Computed: true,
		}
		
		// Dominators
		domData.Dominators = make([]*pb.DominatorEntry, 0, len(g.dominators))
		for objID, domID := range g.dominators {
			domData.Dominators = append(domData.Dominators, &pb.DominatorEntry{
				ObjectId:    objID,
				DominatorId: domID,
			})
		}
		
		// Retained sizes
		domData.RetainedSizes = make([]*pb.RetainedSizeEntry, 0, len(g.retainedSizes))
		for objID, size := range g.retainedSizes {
			domData.RetainedSizes = append(domData.RetainedSizes, &pb.RetainedSizeEntry{
				ObjectId:     objID,
				RetainedSize: size,
			})
		}
		
		// Class retained sizes
		domData.ClassRetainedSizes = make([]*pb.ClassRetainedSizeEntry, 0, len(g.classRetainedSizes))
		for classID, size := range g.classRetainedSizes {
			domData.ClassRetainedSizes = append(domData.ClassRetainedSizes, &pb.ClassRetainedSizeEntry{
				ClassId:      classID,
				RetainedSize: size,
			})
		}
		
		// Class retained sizes attributed
		domData.ClassRetainedSizesAttributed = make([]*pb.ClassRetainedSizeEntry, 0, len(g.classRetainedSizesAttributed))
		for classID, size := range g.classRetainedSizesAttributed {
			domData.ClassRetainedSizesAttributed = append(domData.ClassRetainedSizesAttributed, &pb.ClassRetainedSizeEntry{
				ClassId:      classID,
				RetainedSize: size,
			})
		}
		
		// Class object IDs
		domData.ClassObjectIds = make([]uint64, 0, len(g.classObjectIDs))
		for classObjID := range g.classObjectIDs {
			domData.ClassObjectIds = append(domData.ClassObjectIds, classObjID)
		}
		
		pbGraph.DominatorData = domData
	}
	
	// 6. Add metadata
	pbGraph.Metadata = &pb.GraphMetadata{
		TotalObjects:    stats.Objects,
		TotalReferences: stats.References,
		TotalGcRoots:    stats.GCRoots,
		TotalHeapSize:   totalHeapSize,
		CreatedAt:       time.Now().UnixMilli(),
		SourceFile:      opts.SourceFile,
	}
	
	// Marshal to protobuf bytes
	rawBytes, err := proto.Marshal(pbGraph)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal protobuf: %w", err)
	}
	stats.RawSize = int64(len(rawBytes))
	
	// Build header
	var buf bytes.Buffer
	
	// Write magic bytes for format identification
	buf.WriteString(MagicBytes)
	
	// Write version
	buf.WriteByte(byte(SerializerVersion))
	
	// Write compression type (1 byte)
	buf.WriteByte(byte(opts.Compression))
	
	// Write string table (for field names)
	stringTableProto := &pb.StringTable{Strings: fieldNames}
	stringTableBytes, err := proto.Marshal(stringTableProto)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal string table: %w", err)
	}
	
	// Write string table length (4 bytes, big-endian)
	stLen := uint32(len(stringTableBytes))
	buf.WriteByte(byte(stLen >> 24))
	buf.WriteByte(byte(stLen >> 16))
	buf.WriteByte(byte(stLen >> 8))
	buf.WriteByte(byte(stLen))
	buf.Write(stringTableBytes)
	
	// Compress main data using the specified compressor
	var compressor Compressor
	switch opts.Compression {
	case CompressionZstd:
		zstdComp, err := NewZstdCompressor(opts.CompressionLevel)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create zstd compressor: %w", err)
		}
		defer zstdComp.Close()
		compressor = zstdComp
	default:
		compressor = NewGzipCompressor(opts.CompressionLevel)
	}
	
	compressedData, err := compressor.Compress(rawBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compress data: %w", err)
	}
	buf.Write(compressedData)
	
	result := buf.Bytes()
	stats.CompressedSize = int64(len(result))
	stats.CompressionRatio = float64(stats.RawSize) / float64(stats.CompressedSize)
	stats.Duration = time.Since(startTime)
	
	return result, stats, nil
}

// SerializeToFile serializes the ReferenceGraph to a file.
func (g *ReferenceGraph) SerializeToFile(filename string, opts SerializeOptions) (*SerializationStats, error) {
	data, stats, err := g.Serialize(opts)
	if err != nil {
		return nil, err
	}
	
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	
	return stats, nil
}

// Deserialize deserializes a ReferenceGraph from compressed protobuf bytes.
// Supports both version 1 (gzip only) and version 2 (gzip or zstd).
func DeserializeReferenceGraph(data []byte) (*ReferenceGraph, error) {
	if len(data) < 10 { // Magic(4) + Version(1) + CompressionType(1) + StringTableLen(4)
		return nil, fmt.Errorf("data too short")
	}
	
	// Verify magic bytes
	if string(data[:4]) != MagicBytes {
		return nil, fmt.Errorf("invalid magic bytes: expected %q, got %q", MagicBytes, string(data[:4]))
	}
	
	// Read version
	version := data[4]
	
	var compressionType CompressionType
	var headerOffset int
	
	if version == 1 {
		// Version 1: no compression type byte, always gzip
		compressionType = CompressionGzip
		headerOffset = 5
	} else if version == 2 {
		// Version 2: has compression type byte
		compressionType = CompressionType(data[5])
		headerOffset = 6
	} else {
		return nil, fmt.Errorf("unsupported version: %d", version)
	}
	
	// Read string table length
	stLen := uint32(data[headerOffset])<<24 | uint32(data[headerOffset+1])<<16 | 
		uint32(data[headerOffset+2])<<8 | uint32(data[headerOffset+3])
	stringTableStart := headerOffset + 4
	if int(stringTableStart)+int(stLen) > len(data) {
		return nil, fmt.Errorf("invalid string table length")
	}
	
	// Unmarshal string table
	stringTableBytes := data[stringTableStart : stringTableStart+int(stLen)]
	var stringTable pb.StringTable
	if err := proto.Unmarshal(stringTableBytes, &stringTable); err != nil {
		return nil, fmt.Errorf("failed to unmarshal string table: %w", err)
	}
	fieldNames := stringTable.Strings
	
	// Decompress main data using appropriate decompressor
	compressedData := data[stringTableStart+int(stLen):]
	
	var rawBytes []byte
	var err error
	
	switch compressionType {
	case CompressionZstd:
		zstdComp, err := NewZstdCompressor(CompressionDefault)
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd decompressor: %w", err)
		}
		defer zstdComp.Close()
		rawBytes, err = zstdComp.Decompress(compressedData)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress zstd data: %w", err)
		}
	default:
		// Gzip decompression
		gzipComp := NewGzipCompressor(CompressionDefault)
		rawBytes, err = gzipComp.Decompress(compressedData)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress gzip data: %w", err)
		}
	}
	
	// Unmarshal protobuf
	var pbGraph pb.ReferenceGraphProto
	if err := proto.Unmarshal(rawBytes, &pbGraph); err != nil {
		return nil, fmt.Errorf("failed to unmarshal protobuf: %w", err)
	}
	
	// Build ReferenceGraph
	estimatedObjects := len(pbGraph.Objects)
	g := NewReferenceGraphWithCapacity(estimatedObjects)
	
	// 1. Restore class names
	for _, entry := range pbGraph.ClassNames {
		g.classNames[entry.ClassId] = entry.ClassName
	}
	
	// 2. Restore objects
	for _, obj := range pbGraph.Objects {
		g.objectClass[obj.ObjectId] = obj.ClassId
		g.objectSize[obj.ObjectId] = obj.Size
	}
	
	// 3. Restore references
	for _, ref := range pbGraph.References {
		fieldName := ""
		if int(ref.FieldNameIdx) < len(fieldNames) {
			fieldName = fieldNames[ref.FieldNameIdx]
		}
		objRef := ObjectReference{
			FromObjectID: ref.FromObjectId,
			ToObjectID:   ref.ToObjectId,
			FromClassID:  ref.FromClassId,
			FieldName:    fieldName,
		}
		g.AddReference(objRef)
	}
	
	// 4. Restore GC roots
	for _, root := range pbGraph.GcRoots {
		g.AddGCRoot(&GCRoot{
			ObjectID:   root.ObjectId,
			Type:       protoToGCRootType(root.Type),
			ThreadID:   root.ThreadId,
			FrameIndex: int(root.FrameIndex),
		})
	}
	
	// 5. Restore dominator data if present
	if pbGraph.DominatorData != nil && pbGraph.DominatorData.Computed {
		domData := pbGraph.DominatorData
		g.dominatorComputed = true
		
		for _, entry := range domData.Dominators {
			g.dominators[entry.ObjectId] = entry.DominatorId
		}
		
		for _, entry := range domData.RetainedSizes {
			g.retainedSizes[entry.ObjectId] = entry.RetainedSize
		}
		
		for _, entry := range domData.ClassRetainedSizes {
			g.classRetainedSizes[entry.ClassId] = entry.RetainedSize
		}
		
		for _, entry := range domData.ClassRetainedSizesAttributed {
			g.classRetainedSizesAttributed[entry.ClassId] = entry.RetainedSize
		}
		
		for _, classObjID := range domData.ClassObjectIds {
			g.classObjectIDs[classObjID] = true
		}
		
		// Rebuild reachable objects from dominators
		g.reachableObjects = make(map[uint64]bool, len(g.dominators))
		for objID := range g.dominators {
			g.reachableObjects[objID] = true
		}
	}
	
	return g, nil
}

// DeserializeFromFile deserializes a ReferenceGraph from a file.
func DeserializeReferenceGraphFromFile(filename string) (*ReferenceGraph, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return DeserializeReferenceGraph(data)
}

// gcRootTypeToProto converts GCRootType to protobuf enum.
func gcRootTypeToProto(t GCRootType) pb.GCRootTypeProto {
	switch t {
	case GCRootJNIGlobal:
		return pb.GCRootTypeProto_GC_ROOT_JNI_GLOBAL
	case GCRootJNILocal:
		return pb.GCRootTypeProto_GC_ROOT_JNI_LOCAL
	case GCRootJavaFrame:
		return pb.GCRootTypeProto_GC_ROOT_JAVA_FRAME
	case GCRootNativeStack:
		return pb.GCRootTypeProto_GC_ROOT_NATIVE_STACK
	case GCRootStickyClass:
		return pb.GCRootTypeProto_GC_ROOT_STICKY_CLASS
	case GCRootThreadBlock:
		return pb.GCRootTypeProto_GC_ROOT_THREAD_BLOCK
	case GCRootMonitorUsed:
		return pb.GCRootTypeProto_GC_ROOT_MONITOR_USED
	case GCRootThreadObject:
		return pb.GCRootTypeProto_GC_ROOT_THREAD_OBJECT
	default:
		return pb.GCRootTypeProto_GC_ROOT_UNKNOWN
	}
}

// protoToGCRootType converts protobuf enum to GCRootType.
func protoToGCRootType(t pb.GCRootTypeProto) GCRootType {
	switch t {
	case pb.GCRootTypeProto_GC_ROOT_JNI_GLOBAL:
		return GCRootJNIGlobal
	case pb.GCRootTypeProto_GC_ROOT_JNI_LOCAL:
		return GCRootJNILocal
	case pb.GCRootTypeProto_GC_ROOT_JAVA_FRAME:
		return GCRootJavaFrame
	case pb.GCRootTypeProto_GC_ROOT_NATIVE_STACK:
		return GCRootNativeStack
	case pb.GCRootTypeProto_GC_ROOT_STICKY_CLASS:
		return GCRootStickyClass
	case pb.GCRootTypeProto_GC_ROOT_THREAD_BLOCK:
		return GCRootThreadBlock
	case pb.GCRootTypeProto_GC_ROOT_MONITOR_USED:
		return GCRootMonitorUsed
	case pb.GCRootTypeProto_GC_ROOT_THREAD_OBJECT:
		return GCRootThreadObject
	default:
		return GCRootUnknown
	}
}
