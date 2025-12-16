// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"sort"
	"strings"
)

// BiggestObjectsBuilder builds the list of biggest objects from the reference graph.
type BiggestObjectsBuilder struct {
	refGraph     *ReferenceGraph
	classLayouts map[uint64]*ClassFieldLayout
	strings      map[uint64]string
}

// filteredTopLevelClasses defines classes that should be filtered from top-level Biggest Objects view.
// These are basic types and collection classes that are typically not the root cause of memory issues.
// Note: ConcurrentHashMap is NOT filtered as it's often used as a cache and can be a leak source.
var filteredTopLevelClasses = map[string]bool{
	// Primitive arrays
	"byte[]":    true,
	"char[]":    true,
	"int[]":     true,
	"long[]":    true,
	"short[]":   true,
	"boolean[]": true,
	"float[]":   true,
	"double[]":  true,
	// Basic wrapper arrays
	"java.lang.Object[]": true,
	"java.lang.String[]": true,
	// Basic collection classes (when they appear as top-level objects)
	"java.util.ArrayList":  true,
	"java.util.HashMap":    true,
	"java.util.HashSet":    true,
	"java.util.LinkedList": true,
	"java.util.TreeMap":    true,
	"java.util.TreeSet":    true,
	"java.util.LinkedHashMap": true,
	"java.util.LinkedHashSet": true,
	// HashMap/HashSet internal nodes
	"java.util.HashMap$Node":     true,
	"java.util.HashMap$TreeNode": true,
	"java.util.HashSet$Node":     true,
	// Basic wrapper types
	"java.lang.Object": true,
	"java.lang.Class":  true,
}

// shouldFilterTopLevelClass checks if a class should be filtered from top-level Biggest Objects.
func shouldFilterTopLevelClass(className string) bool {
	// Direct match
	if filteredTopLevelClasses[className] {
		return true
	}
	// Filter generic array types (e.g., "SomeClass[]")
	if strings.HasSuffix(className, "[]") {
		// Only filter primitive and basic arrays, not business class arrays
		baseName := strings.TrimSuffix(className, "[]")
		if strings.HasPrefix(baseName, "java.lang.") || strings.HasPrefix(baseName, "java.util.") {
			return true
		}
	}
	return false
}

// NewBiggestObjectsBuilder creates a new BiggestObjectsBuilder.
func NewBiggestObjectsBuilder(refGraph *ReferenceGraph, classLayouts map[uint64]*ClassFieldLayout, strings map[uint64]string) *BiggestObjectsBuilder {
	return &BiggestObjectsBuilder{
		refGraph:     refGraph,
		classLayouts: classLayouts,
		strings:      strings,
	}
}

// objectWithSize is a helper struct for sorting objects by size.
type objectWithSize struct {
	objectID     uint64
	shallowSize  int64
	retainedSize int64
}

// BuildBiggestObjects builds the list of biggest objects sorted by retained size.
// topN specifies the maximum number of objects to return.
// sortBy can be "retained" (default) or "shallow".
// filterBasicTypes controls whether to filter out basic types and collection classes.
func (b *BiggestObjectsBuilder) BuildBiggestObjects(topN int, sortBy string) []*BiggestObject {
	return b.BuildBiggestObjectsFiltered(topN, sortBy, true)
}

// BuildBiggestObjectsFiltered builds the list of biggest objects with optional filtering.
// filterBasicTypes: if true, filters out basic types and collection classes from top level.
func (b *BiggestObjectsBuilder) BuildBiggestObjectsFiltered(topN int, sortBy string, filterBasicTypes bool) []*BiggestObject {
	if b.refGraph == nil {
		return nil
	}

	if topN <= 0 {
		topN = 100
	}

	// Ensure dominator tree is computed for retained sizes
	b.refGraph.ComputeDominatorTree()

	// Collect all objects with their sizes, optionally filtering basic types
	objects := make([]objectWithSize, 0, len(b.refGraph.objectClass))
	for objID := range b.refGraph.objectClass {
		// Only include reachable objects
		if !b.refGraph.IsObjectReachable(objID) {
			continue
		}
		
		// Filter basic types if requested
		if filterBasicTypes {
			classID := b.refGraph.objectClass[objID]
			className := b.refGraph.GetClassName(classID)
			if shouldFilterTopLevelClass(className) {
				continue
			}
		}
		
		objects = append(objects, objectWithSize{
			objectID:     objID,
			shallowSize:  b.refGraph.objectSize[objID],
			retainedSize: b.refGraph.retainedSizes[objID],
		})
	}

	// Sort by retained size (default) or shallow size
	if sortBy == "shallow" {
		sort.Slice(objects, func(i, j int) bool {
			return objects[i].shallowSize > objects[j].shallowSize
		})
	} else {
		sort.Slice(objects, func(i, j int) bool {
			return objects[i].retainedSize > objects[j].retainedSize
		})
	}

	// Take top N
	if len(objects) > topN {
		objects = objects[:topN]
	}

	// Build result with field information
	result := make([]*BiggestObject, 0, len(objects))
	for _, obj := range objects {
		bigObj := b.buildBiggestObject(obj.objectID)
		if bigObj != nil {
			result = append(result, bigObj)
		}
	}

	return result
}

// BuildBiggestObjectsByClass builds the list of biggest objects for a specific class.
func (b *BiggestObjectsBuilder) BuildBiggestObjectsByClass(className string, topN int, sortBy string) []*BiggestObject {
	if b.refGraph == nil {
		return nil
	}

	if topN <= 0 {
		topN = 50
	}

	// Find class ID
	classID, found := b.refGraph.getClassIDByName(className)
	if !found {
		return nil
	}

	// Ensure dominator tree is computed for retained sizes
	b.refGraph.ComputeDominatorTree()

	// Get objects of this class
	classObjects := b.refGraph.getObjectsByClass(classID)
	if len(classObjects) == 0 {
		return nil
	}

	// Collect objects with sizes
	objects := make([]objectWithSize, 0, len(classObjects))
	for _, objID := range classObjects {
		if !b.refGraph.IsObjectReachable(objID) {
			continue
		}
		objects = append(objects, objectWithSize{
			objectID:     objID,
			shallowSize:  b.refGraph.objectSize[objID],
			retainedSize: b.refGraph.retainedSizes[objID],
		})
	}

	// Sort
	if sortBy == "shallow" {
		sort.Slice(objects, func(i, j int) bool {
			return objects[i].shallowSize > objects[j].shallowSize
		})
	} else {
		sort.Slice(objects, func(i, j int) bool {
			return objects[i].retainedSize > objects[j].retainedSize
		})
	}

	// Take top N
	if len(objects) > topN {
		objects = objects[:topN]
	}

	// Build result
	result := make([]*BiggestObject, 0, len(objects))
	for _, obj := range objects {
		bigObj := b.buildBiggestObject(obj.objectID)
		if bigObj != nil {
			result = append(result, bigObj)
		}
	}

	return result
}

// buildBiggestObject builds a BiggestObject from an object ID.
func (b *BiggestObjectsBuilder) buildBiggestObject(objectID uint64) *BiggestObject {
	classID := b.refGraph.objectClass[objectID]
	className := b.refGraph.GetClassName(classID)
	if className == "" {
		className = "(unknown)"
	}

	bigObj := &BiggestObject{
		ObjectID:     objectID,
		ClassName:    className,
		ShallowSize:  b.refGraph.objectSize[objectID],
		RetainedSize: b.refGraph.retainedSizes[objectID],
	}

	// Extract field information if class layout is available
	if b.classLayouts != nil {
		if layout, ok := b.classLayouts[classID]; ok {
			bigObj.Fields = b.extractFields(objectID, layout)
		}
	}

	// Add GC root path (limited to 1 path for performance)
	paths := b.refGraph.FindPathsToGCRoot(objectID, 1, 15)
	if len(paths) > 0 {
		bigObj.GCRootPath = paths[0]
	}

	return bigObj
}

// extractFields extracts field values from an object using its class layout.
func (b *BiggestObjectsBuilder) extractFields(objectID uint64, layout *ClassFieldLayout) []*ObjectField {
	if layout == nil {
		return nil
	}

	var fields []*ObjectField

	// Add static fields first
	for _, sf := range layout.StaticFields {
		field := &ObjectField{
			Name:     sf.Name,
			Type:     basicTypeToString(sf.Type),
			IsStatic: true,
		}
		if sf.Type == TypeObject && sf.RefID != 0 {
			field.RefID = sf.RefID
			if refClassID, ok := b.refGraph.objectClass[sf.RefID]; ok {
				field.RefClass = b.refGraph.GetClassName(refClassID)
			}
		} else {
			field.Value = sf.Value
		}
		fields = append(fields, field)
	}

	// Add instance fields (from outgoing references)
	refs := b.refGraph.outgoingRefs[objectID]
	refByField := make(map[string]ObjectReference)
	for _, ref := range refs {
		if ref.FieldName != "" {
			refByField[ref.FieldName] = ref
		}
	}

	// Process instance fields from layout
	for _, f := range layout.InstanceFields {
		field := &ObjectField{
			Name: f.Name,
			Type: basicTypeToString(f.Type),
		}
		if f.Type == TypeObject {
			if ref, ok := refByField[f.Name]; ok {
				field.RefID = ref.ToObjectID
				if refClassID, ok := b.refGraph.objectClass[ref.ToObjectID]; ok {
					field.RefClass = b.refGraph.GetClassName(refClassID)
				}
			}
		}
		fields = append(fields, field)
	}

	// If no layout fields, extract from outgoing references
	if len(layout.InstanceFields) == 0 && len(refs) > 0 {
		for _, ref := range refs {
			field := &ObjectField{
				Name:  ref.FieldName,
				Type:  "object",
				RefID: ref.ToObjectID,
			}
			if refClassID, ok := b.refGraph.objectClass[ref.ToObjectID]; ok {
				field.RefClass = b.refGraph.GetClassName(refClassID)
			}
			fields = append(fields, field)
		}
	}

	return fields
}

// basicTypeToString converts a BasicType to a string representation.
func basicTypeToString(t BasicType) string {
	switch t {
	case TypeObject:
		return "object"
	case TypeBoolean:
		return "boolean"
	case TypeChar:
		return "char"
	case TypeFloat:
		return "float"
	case TypeDouble:
		return "double"
	case TypeByte:
		return "byte"
	case TypeShort:
		return "short"
	case TypeInt:
		return "int"
	case TypeLong:
		return "long"
	default:
		return "unknown"
	}
}

// GetBiggestObjectsByRetainedSize is a convenience method to get biggest objects sorted by retained size.
func (b *BiggestObjectsBuilder) GetBiggestObjectsByRetainedSize(topN int) []*BiggestObject {
	return b.BuildBiggestObjects(topN, "retained")
}

// GetBiggestObjectsByShallowSize is a convenience method to get biggest objects sorted by shallow size.
func (b *BiggestObjectsBuilder) GetBiggestObjectsByShallowSize(topN int) []*BiggestObject {
	return b.BuildBiggestObjects(topN, "shallow")
}

// GetObjectFields returns the fields of a specific object by its ID.
// This is used for lazy loading of child objects in the tree view.
func (b *BiggestObjectsBuilder) GetObjectFields(objectID uint64) []*ObjectFieldDetail {
	if b.refGraph == nil {
		return nil
	}

	classID, ok := b.refGraph.objectClass[objectID]
	if !ok {
		return nil
	}

	// Ensure dominator tree is computed for retained sizes
	b.refGraph.ComputeDominatorTree()

	var fields []*ObjectFieldDetail

	// Get outgoing references for this object
	refs := b.refGraph.outgoingRefs[objectID]
	refByField := make(map[string]ObjectReference)
	for _, ref := range refs {
		if ref.FieldName != "" {
			refByField[ref.FieldName] = ref
		}
	}

	// Try to get class layout for field names and types
	if layout, ok := b.classLayouts[classID]; ok {
		// Process instance fields from layout
		for _, f := range layout.InstanceFields {
			field := &ObjectFieldDetail{
				Name: f.Name,
				Type: basicTypeToString(f.Type),
			}
			if f.Type == TypeObject {
				if ref, ok := refByField[f.Name]; ok {
					field.RefID = ref.ToObjectID
					if refClassID, ok := b.refGraph.objectClass[ref.ToObjectID]; ok {
						field.RefClass = b.refGraph.GetClassName(refClassID)
						field.ShallowSize = b.refGraph.objectSize[ref.ToObjectID]
						field.RetainedSize = b.refGraph.retainedSizes[ref.ToObjectID]
						// Check if this object has children (outgoing references)
						field.HasChildren = len(b.refGraph.outgoingRefs[ref.ToObjectID]) > 0
					}
				}
			}
			fields = append(fields, field)
		}
	}

	// If no layout fields, extract from outgoing references directly
	if len(fields) == 0 && len(refs) > 0 {
		// Sort refs by retained size for better display
		sortedRefs := make([]ObjectReference, len(refs))
		copy(sortedRefs, refs)
		sort.Slice(sortedRefs, func(i, j int) bool {
			return b.refGraph.retainedSizes[sortedRefs[i].ToObjectID] > b.refGraph.retainedSizes[sortedRefs[j].ToObjectID]
		})

		for _, ref := range sortedRefs {
			refClassID, ok := b.refGraph.objectClass[ref.ToObjectID]
			if !ok {
				continue
			}
			refClassName := b.refGraph.GetClassName(refClassID)
			
			field := &ObjectFieldDetail{
				Name:         ref.FieldName,
				Type:         "object",
				RefID:        ref.ToObjectID,
				RefClass:     refClassName,
				ShallowSize:  b.refGraph.objectSize[ref.ToObjectID],
				RetainedSize: b.refGraph.retainedSizes[ref.ToObjectID],
				HasChildren:  len(b.refGraph.outgoingRefs[ref.ToObjectID]) > 0,
			}
			fields = append(fields, field)
		}
	}

	// Sort fields by retained size (largest first) for reference types
	sort.Slice(fields, func(i, j int) bool {
		// Put reference types first, sorted by retained size
		if fields[i].RefID != 0 && fields[j].RefID != 0 {
			return fields[i].RetainedSize > fields[j].RetainedSize
		}
		if fields[i].RefID != 0 {
			return true
		}
		if fields[j].RefID != 0 {
			return false
		}
		return false
	})

	return fields
}

// GetObjectInfo returns basic information about an object by its ID.
func (b *BiggestObjectsBuilder) GetObjectInfo(objectID uint64) *ObjectFieldDetail {
	if b.refGraph == nil {
		return nil
	}

	classID, ok := b.refGraph.objectClass[objectID]
	if !ok {
		return nil
	}

	// Ensure dominator tree is computed
	b.refGraph.ComputeDominatorTree()

	className := b.refGraph.GetClassName(classID)
	return &ObjectFieldDetail{
		RefID:        objectID,
		RefClass:     className,
		ShallowSize:  b.refGraph.objectSize[objectID],
		RetainedSize: b.refGraph.retainedSizes[objectID],
		HasChildren:  len(b.refGraph.outgoingRefs[objectID]) > 0,
	}
}
