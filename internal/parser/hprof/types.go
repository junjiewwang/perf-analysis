// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// RecordTag represents the type of record in HPROF format.
type RecordTag = libhprof.RecordTag

// HeapDumpTag represents sub-tags within a heap dump record.
type HeapDumpTag = libhprof.HeapDumpTag

// BasicType represents Java primitive types.
type BasicType = libhprof.BasicType

// Header represents the HPROF file header.
type Header = libhprof.Header

// ClassInfo holds class metadata.
type ClassInfo = libhprof.ClassInfo

// InstanceInfo holds instance metadata.
type InstanceInfo = libhprof.InstanceInfo

// ArrayInfo holds array metadata.
type ArrayInfo = libhprof.ArrayInfo

// HeapSummary holds heap summary statistics.
type HeapSummary = libhprof.HeapSummary

// ClassStats holds aggregated statistics for a class.
type ClassStats = libhprof.ClassStats

// HeapAnalysisResult holds the complete analysis result.
type HeapAnalysisResult = libhprof.HeapAnalysisResult

// GCRootsAnalysis holds GC roots analysis data for persistence.
type GCRootsAnalysis = libhprof.GCRootsAnalysis

// GCRootClassSummary represents GC roots grouped by class name.
type GCRootClassSummary = libhprof.GCRootClassSummary

// GCRootInstanceInfo represents a single GC root instance.
type GCRootInstanceInfo = libhprof.GCRootInstanceInfo

// ObjectInfo holds information about a specific object.
type ObjectInfo = libhprof.ObjectInfo

// StringStats holds string-related statistics.
type StringStats = libhprof.StringStats

// ArrayStats holds array-related statistics.
type ArrayStats = libhprof.ArrayStats

// BiggestObject represents a large object with its field values.
type BiggestObject = libhprof.BiggestObject

// ObjectField represents a field value in an object.
type ObjectField = libhprof.ObjectField

// ObjectFieldDetail represents a field with detailed information for tree expansion.
type ObjectFieldDetail = libhprof.ObjectFieldDetail

// ClassFieldLayout describes the field layout of a class for field value extraction.
type ClassFieldLayout = libhprof.ClassFieldLayout

// FieldInfo describes an instance field.
type FieldInfo = libhprof.FieldInfo

// StaticFieldInfo describes a static field with its value.
type StaticFieldInfo = libhprof.StaticFieldInfo

// ============================================================================
// RecordTag Constants
// ============================================================================

const (
	TagString          = libhprof.TagString
	TagLoadClass       = libhprof.TagLoadClass
	TagUnloadClass     = libhprof.TagUnloadClass
	TagStackFrame      = libhprof.TagStackFrame
	TagStackTrace      = libhprof.TagStackTrace
	TagAllocSites      = libhprof.TagAllocSites
	TagHeapSummary     = libhprof.TagHeapSummary
	TagStartThread     = libhprof.TagStartThread
	TagEndThread       = libhprof.TagEndThread
	TagHeapDump        = libhprof.TagHeapDump
	TagHeapDumpSegment = libhprof.TagHeapDumpSegment
	TagHeapDumpEnd     = libhprof.TagHeapDumpEnd
	TagCPUSamples      = libhprof.TagCPUSamples
	TagControlSettings = libhprof.TagControlSettings
)

// ============================================================================
// HeapDumpTag Constants
// ============================================================================

const (
	HeapTagRootUnknown        = libhprof.HeapTagRootUnknown
	HeapTagRootJNIGlobal      = libhprof.HeapTagRootJNIGlobal
	HeapTagRootJNILocal       = libhprof.HeapTagRootJNILocal
	HeapTagRootJavaFrame      = libhprof.HeapTagRootJavaFrame
	HeapTagRootNativeStack    = libhprof.HeapTagRootNativeStack
	HeapTagRootStickyClass    = libhprof.HeapTagRootStickyClass
	HeapTagRootThreadBlock    = libhprof.HeapTagRootThreadBlock
	HeapTagRootMonitorUsed    = libhprof.HeapTagRootMonitorUsed
	HeapTagRootThreadObject   = libhprof.HeapTagRootThreadObject
	HeapTagClassDump          = libhprof.HeapTagClassDump
	HeapTagInstanceDump       = libhprof.HeapTagInstanceDump
	HeapTagObjectArrayDump    = libhprof.HeapTagObjectArrayDump
	HeapTagPrimitiveArrayDump = libhprof.HeapTagPrimitiveArrayDump
)

// ============================================================================
// BasicType Constants
// ============================================================================

const (
	TypeObject  = libhprof.TypeObject
	TypeBoolean = libhprof.TypeBoolean
	TypeChar    = libhprof.TypeChar
	TypeFloat   = libhprof.TypeFloat
	TypeDouble  = libhprof.TypeDouble
	TypeByte    = libhprof.TypeByte
	TypeShort   = libhprof.TypeShort
	TypeInt     = libhprof.TypeInt
	TypeLong    = libhprof.TypeLong
)

// ============================================================================
// Function Forwarding
// ============================================================================

// BasicTypeSize returns the size in bytes for a basic type.
func BasicTypeSize(t BasicType, idSize int) int {
	return libhprof.BasicTypeSize(t, idSize)
}
