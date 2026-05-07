// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import "fmt"

// HeapObjectInfo represents detailed information about a heap object for query results.
// It consolidates the repeated pattern of assembling object metadata
// (hex-formatted ID, class name, shallow size, retained size) found across query methods.
type HeapObjectInfo struct {
	ObjectID     string `json:"object_id"`
	ClassName    string `json:"class_name"`
	ShallowSize  int64  `json:"shallow_size"`
	RetainedSize int64  `json:"retained_size"`
}

// ObjectInfoAssembler provides helper methods for assembling object information
// from a HeapGraph. It eliminates the repeated pattern of:
//
//	idx → objectID(hex) + className + shallowSize + retainedSize
//
// that appears in every query method.
type ObjectInfoAssembler struct {
	graph HeapGraph
}

// NewObjectInfoAssembler creates a new assembler for the given graph.
func NewObjectInfoAssembler(graph HeapGraph) *ObjectInfoAssembler {
	return &ObjectInfoAssembler{graph: graph}
}

// AssembleByIndex builds HeapObjectInfo from an object's internal index.
// Returns nil if the index is invalid (< 0).
func (a *ObjectInfoAssembler) AssembleByIndex(idx int32) *HeapObjectInfo {
	if idx < 0 {
		return nil
	}
	classID := a.graph.GetClassID(idx)
	return &HeapObjectInfo{
		ObjectID:     formatHexID(a.graph.GetObjectID(idx)),
		ClassName:    a.graph.GetClassName(classID),
		ShallowSize:  a.graph.GetShallowSize(idx),
		RetainedSize: a.graph.GetRetainedSize(idx),
	}
}

// AssembleByObjectID builds HeapObjectInfo from an object ID.
// Returns nil if the objectID is not found in the graph.
func (a *ObjectInfoAssembler) AssembleByObjectID(objectID uint64) *HeapObjectInfo {
	idx := a.graph.GetObjectIndex(objectID)
	return a.AssembleByIndex(idx)
}

// GetClassNameByIndex returns the class name for an object at the given index.
func (a *ObjectInfoAssembler) GetClassNameByIndex(idx int32) string {
	if idx < 0 {
		return ""
	}
	classID := a.graph.GetClassID(idx)
	return a.graph.GetClassName(classID)
}

// ResolveFieldName resolves a field name ID to its string representation.
func (a *ObjectInfoAssembler) ResolveFieldName(fieldIDs []int32, index int) string {
	if fieldIDs == nil || index < 0 || index >= len(fieldIDs) {
		return ""
	}
	return a.graph.GetFieldName(fieldIDs[index])
}

// formatHexID formats an object ID as "0x" prefixed hexadecimal string.
func formatHexID(id uint64) string {
	return fmt.Sprintf("0x%x", id)
}
