// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

// HeapGraph defines the read-only query interface for heap dump graph data.
// It abstracts the concrete IndexedReferenceGraph, enabling:
// - Testability: upper-layer code can use mocks for unit testing
// - Extensibility: future implementations (e.g., mmap-based lazy loading) can
//   satisfy this interface without changing consumers
// - Decoupling: query engines depend on the interface, not the concrete struct
type HeapGraph interface {
	// ObjectCount returns the total number of objects in the heap.
	ObjectCount() int32

	// GetObjectIndex returns the internal index for an object ID, or -1 if not found.
	GetObjectIndex(objectID uint64) int32

	// GetObjectID returns the object ID for an internal index.
	GetObjectID(idx int32) uint64

	// GetClassID returns the class ID for an object index.
	GetClassID(idx int32) uint64

	// GetClassName returns the class name for a class ID.
	GetClassName(classID uint64) string

	// GetShallowSize returns the shallow size (in bytes) for an object index.
	GetShallowSize(idx int32) int64

	// GetRetainedSize returns the retained size (in bytes) for an object index.
	GetRetainedSize(idx int32) int64

	// GetDominator returns the dominator index for an object index, or -1 if none.
	GetDominator(idx int32) int32

	// IsGCRoot returns true if the object at the given index is a GC root.
	IsGCRoot(idx int32) bool

	// IsReachable returns true if the object at the given index is reachable from GC roots.
	IsReachable(idx int32) bool

	// IsClassObject returns true if the object at the given index is a Class instance.
	IsClassObject(idx int32) bool

	// GetOutgoingEdges returns outgoing reference edges for an object.
	// Returns target indices, field name IDs, and class IDs of the references.
	GetOutgoingEdges(idx int32) (targets []int32, fieldIDs []int32, classIDs []uint64)

	// GetIncomingEdges returns incoming reference edges for an object.
	// Returns source indices, field name IDs, and class IDs of the referrers.
	GetIncomingEdges(idx int32) (sources []int32, fieldIDs []int32, classIDs []uint64)

	// GetObjectsByClass returns all object indices belonging to a given class.
	GetObjectsByClass(classID uint64) []int32

	// GetFieldName resolves a field name ID to its string representation.
	// Field IDs come from the fieldIDs slice returned by GetOutgoingEdges/GetIncomingEdges.
	GetFieldName(fieldID int32) string

	// GetGCRoots returns the list of GC root entries.
	GetGCRoots() []GCRoot
}
