// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import "time"

// RecordTag represents the type of record in HPROF format.
type RecordTag uint8

const (
	TagString          RecordTag = 0x01
	TagLoadClass       RecordTag = 0x02
	TagUnloadClass     RecordTag = 0x03
	TagStackFrame      RecordTag = 0x04
	TagStackTrace      RecordTag = 0x05
	TagAllocSites      RecordTag = 0x06
	TagHeapSummary     RecordTag = 0x07
	TagStartThread     RecordTag = 0x0A
	TagEndThread       RecordTag = 0x0B
	TagHeapDump        RecordTag = 0x0C
	TagHeapDumpSegment RecordTag = 0x1C
	TagHeapDumpEnd     RecordTag = 0x2C
	TagCPUSamples      RecordTag = 0x0D
	TagControlSettings RecordTag = 0x0E
)

// HeapDumpTag represents sub-tags within a heap dump record.
type HeapDumpTag uint8

const (
	HeapTagRootUnknown       HeapDumpTag = 0xFF
	HeapTagRootJNIGlobal     HeapDumpTag = 0x01
	HeapTagRootJNILocal      HeapDumpTag = 0x02
	HeapTagRootJavaFrame     HeapDumpTag = 0x03
	HeapTagRootNativeStack   HeapDumpTag = 0x04
	HeapTagRootStickyClass   HeapDumpTag = 0x05
	HeapTagRootThreadBlock   HeapDumpTag = 0x06
	HeapTagRootMonitorUsed   HeapDumpTag = 0x07
	HeapTagRootThreadObject  HeapDumpTag = 0x08
	HeapTagClassDump         HeapDumpTag = 0x20
	HeapTagInstanceDump      HeapDumpTag = 0x21
	HeapTagObjectArrayDump   HeapDumpTag = 0x22
	HeapTagPrimitiveArrayDump HeapDumpTag = 0x23
)

// BasicType represents Java primitive types.
type BasicType uint8

const (
	TypeObject  BasicType = 2
	TypeBoolean BasicType = 4
	TypeChar    BasicType = 5
	TypeFloat   BasicType = 6
	TypeDouble  BasicType = 7
	TypeByte    BasicType = 8
	TypeShort   BasicType = 9
	TypeInt     BasicType = 10
	TypeLong    BasicType = 11
)

// BasicTypeSize returns the size in bytes for a basic type.
func BasicTypeSize(t BasicType, idSize int) int {
	switch t {
	case TypeObject:
		return idSize
	case TypeBoolean, TypeByte:
		return 1
	case TypeChar, TypeShort:
		return 2
	case TypeFloat, TypeInt:
		return 4
	case TypeDouble, TypeLong:
		return 8
	default:
		return 0
	}
}

// Header represents the HPROF file header.
type Header struct {
	Format    string    // e.g., "JAVA PROFILE 1.0.2"
	IDSize    int       // Size of identifiers (4 or 8 bytes)
	Timestamp time.Time // Dump timestamp
}

// ClassInfo holds class metadata.
type ClassInfo struct {
	ClassID         uint64
	Name            string
	SuperClassID    uint64
	InstanceSize    int
	InstanceCount   int64
	TotalSize       int64
	FieldCount      int
	StaticFieldCount int
}

// InstanceInfo holds instance metadata.
type InstanceInfo struct {
	ObjectID  uint64
	ClassID   uint64
	Size      int
	FieldData []byte
}

// ArrayInfo holds array metadata.
type ArrayInfo struct {
	ObjectID    uint64
	ElementType BasicType
	Length      int
	Size        int
	ClassName   string // For object arrays
}

// HeapSummary holds heap summary statistics.
type HeapSummary struct {
	TotalLiveBytes   int64
	TotalLiveObjects int64
	TotalAllocBytes  int64
	TotalAllocObjects int64
}

// ClassStats holds aggregated statistics for a class.
type ClassStats struct {
	ClassName     string  `json:"class_name"`
	InstanceCount int64   `json:"instance_count"`
	TotalSize     int64   `json:"total_size"`
	AvgSize       float64 `json:"avg_size"`
	Percentage    float64 `json:"percentage"`
	ShallowSize   int64   `json:"shallow_size"`
	RetainedSize  int64   `json:"retained_size,omitempty"`
}

// HeapAnalysisResult holds the complete analysis result.
type HeapAnalysisResult struct {
	Header           *Header                       `json:"header"`
	Summary          *HeapSummary                  `json:"summary"`
	TopClasses       []*ClassStats                 `json:"top_classes"`
	TotalClasses     int                           `json:"total_classes"`
	TotalInstances   int64                         `json:"total_instances"`
	TotalHeapSize    int64                         `json:"total_heap_size"`
	LargestObjects   []*ObjectInfo                 `json:"largest_objects,omitempty"`
	BiggestObjects   []*BiggestObject              `json:"biggest_objects,omitempty"`
	GCRootsAnalysis  *GCRootsAnalysis              `json:"gc_roots_analysis,omitempty"`
	StringStats      *StringStats                  `json:"string_stats,omitempty"`
	ArrayStats       *ArrayStats                   `json:"array_stats,omitempty"`
	ClassRetainers   map[string]*ClassRetainers    `json:"class_retainers,omitempty"`
	ReferenceGraphs  map[string]*ReferenceGraphData `json:"reference_graphs,omitempty"`
	BusinessRetainers map[string][]*BusinessRetainer `json:"business_retainers,omitempty"`
	// ClassLayouts holds field layout information for classes (used by BiggestObjectsBuilder)
	ClassLayouts     map[uint64]*ClassFieldLayout  `json:"-"`
	// Strings holds string table (used by BiggestObjectsBuilder)
	Strings          map[uint64]string             `json:"-"`
	// RefGraph holds the reference graph for advanced analysis (not serialized to JSON)
	RefGraph         *ReferenceGraph               `json:"-"`
}

// GCRootsAnalysis holds GC roots analysis data for persistence.
type GCRootsAnalysis struct {
	TotalRoots    int                   `json:"total_roots"`
	TotalClasses  int                   `json:"total_classes"`
	TotalRetained int64                 `json:"total_retained"`
	TotalShallow  int64                 `json:"total_shallow"`
	Classes       []*GCRootClassSummary `json:"classes"`
}

// GCRootClassSummary represents GC roots grouped by class name.
type GCRootClassSummary struct {
	ClassName     string                `json:"class_name"`
	RootType      GCRootType            `json:"root_type,omitempty"`
	TotalShallow  int64                 `json:"total_shallow"`
	TotalRetained int64                 `json:"total_retained"`
	InstanceCount int                   `json:"instance_count"`
	Roots         []*GCRootInstanceInfo `json:"roots,omitempty"`
}

// GCRootInstanceInfo represents a single GC root instance.
type GCRootInstanceInfo struct {
	ObjectID     uint64     `json:"object_id"`
	RootType     GCRootType `json:"root_type"`
	ShallowSize  int64      `json:"shallow_size"`
	RetainedSize int64      `json:"retained_size"`
	ThreadID     uint64     `json:"thread_id,omitempty"`
	FrameIndex   int        `json:"frame_index,omitempty"`
}

// ObjectInfo holds information about a specific object.
type ObjectInfo struct {
	ObjectID  uint64 `json:"object_id"`
	ClassName string `json:"class_name"`
	Size      int64  `json:"size"`
}

// StringStats holds string-related statistics.
type StringStats struct {
	TotalCount       int64   `json:"total_count"`
	TotalSize        int64   `json:"total_size"`
	UniqueCount      int64   `json:"unique_count"`
	DuplicateCount   int64   `json:"duplicate_count"`
	DuplicateWaste   int64   `json:"duplicate_waste"`
	AvgLength        float64 `json:"avg_length"`
	MaxLength        int     `json:"max_length"`
}

// ArrayStats holds array-related statistics.
type ArrayStats struct {
	TotalArrays      int64            `json:"total_arrays"`
	TotalSize        int64            `json:"total_size"`
	EmptyArrays      int64            `json:"empty_arrays"`
	EmptyArraysWaste int64            `json:"empty_arrays_waste"`
	ByType           map[string]int64 `json:"by_type"`
}

// BiggestObject represents a large object with its field values.
type BiggestObject struct {
	ObjectID     uint64         `json:"object_id"`
	ClassName    string         `json:"class_name"`
	ShallowSize  int64          `json:"shallow_size"`
	RetainedSize int64          `json:"retained_size"`
	Fields       []*ObjectField `json:"fields,omitempty"`
	GCRootPath   *GCRootPath    `json:"gc_root_path,omitempty"`
}

// ObjectField represents a field value in an object.
type ObjectField struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	Value        interface{} `json:"value,omitempty"`
	RefID        uint64      `json:"ref_id,omitempty"`
	RefClass     string      `json:"ref_class,omitempty"`
	ShallowSize  int64       `json:"shallow_size,omitempty"`
	RetainedSize int64       `json:"retained_size,omitempty"`
	HasChildren  bool        `json:"has_children,omitempty"`
	IsStatic     bool        `json:"is_static,omitempty"`
}

// ObjectFieldDetail represents a field with detailed information for tree expansion.
// This is used for lazy loading of child objects in the Biggest Objects tree view.
type ObjectFieldDetail struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	Value        interface{} `json:"value,omitempty"`
	RefID        uint64      `json:"ref_id,omitempty"`
	RefClass     string      `json:"ref_class,omitempty"`
	ShallowSize  int64       `json:"shallow_size,omitempty"`
	RetainedSize int64       `json:"retained_size,omitempty"`
	HasChildren  bool        `json:"has_children"`
	IsStatic     bool        `json:"is_static,omitempty"`
}

// ClassFieldLayout describes the field layout of a class for field value extraction.
type ClassFieldLayout struct {
	ClassID       uint64
	ClassName     string
	SuperClassID  uint64
	InstanceSize  int
	InstanceFields []FieldInfo
	StaticFields   []StaticFieldInfo
}

// FieldInfo describes an instance field.
type FieldInfo struct {
	NameID uint64
	Name   string
	Type   BasicType
	Offset int // Offset in instance data
}

// StaticFieldInfo describes a static field with its value.
type StaticFieldInfo struct {
	NameID uint64
	Name   string
	Type   BasicType
	Value  interface{}
	RefID  uint64 // For object references
}
