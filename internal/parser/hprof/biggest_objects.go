// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"sort"
)

// BiggestObjectsBuilder builds the list of biggest objects from the reference graph.
type BiggestObjectsBuilder struct {
	refGraph     *ReferenceGraph
	classLayouts map[uint64]*ClassFieldLayout
	strings      map[uint64]string
}

// filteredTopLevelClasses defines classes that should be filtered from top-level Biggest Objects view.
// These are basic types and collection classes that are typically not the root cause of memory issues.
// This matches IDEA's Biggest Objects view which filters out container/infrastructure classes.
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
	// JVM internal classes (class metadata, not real memory issues)
	"java.lang.Class": true,
	// HashMap/HashSet internal nodes (these are always children of their parent collections)
	"java.util.HashMap$Node":     true,
	"java.util.HashMap$Node[]":   true,
	"java.util.HashMap$TreeNode": true,
	"java.util.HashSet$Node":     true,
	// ConcurrentHashMap internal nodes
	"java.util.concurrent.ConcurrentHashMap$Node":   true,
	"java.util.concurrent.ConcurrentHashMap$Node[]": true,
	// Collection classes - these are containers, not root causes
	"java.util.ArrayList":                       true,
	"java.util.LinkedList":                      true,
	"java.util.HashMap":                         true,
	"java.util.LinkedHashMap":                   true,
	"java.util.TreeMap":                         true,
	"java.util.HashSet":                         true,
	"java.util.LinkedHashSet":                   true,
	"java.util.TreeSet":                         true,
	"java.util.concurrent.ConcurrentHashMap":    true,
	"java.util.concurrent.CopyOnWriteArrayList": true,
}

// filteredTopLevelPrefixes defines class name prefixes that should be filtered.
var filteredTopLevelPrefixes = []string{
	// JDK proxy classes
	"jdk.proxy",
	"com.sun.proxy",
}

// filteredTopLevelSuffixes defines class name suffixes that should be filtered.
var filteredTopLevelSuffixes = []string{
	// Allocator classes - these manage memory but don't hold the actual data
	"Allocator",
	"ByteBufAllocator",
}

// filteredTopLevelContains defines substrings that should be filtered if found anywhere in class name.
var filteredTopLevelContains = []string{
	// Lambda expressions (appear as $$Lambda$ in class names)
	"$$Lambda",
}

// shouldFilterTopLevelClass checks if a class should be filtered from top-level Biggest Objects.
// This matches IDEA's behavior of filtering out container classes, proxies, lambdas, and allocators.
func shouldFilterTopLevelClass(className string) bool {
	// Direct match
	if filteredTopLevelClasses[className] {
		return true
	}
	
	// Check prefixes
	for _, prefix := range filteredTopLevelPrefixes {
		if len(className) >= len(prefix) && className[:len(prefix)] == prefix {
			return true
		}
	}
	
	// Check suffixes
	for _, suffix := range filteredTopLevelSuffixes {
		if len(className) >= len(suffix) && className[len(className)-len(suffix):] == suffix {
			return true
		}
	}
	
	// Check contains (for patterns that can appear anywhere in class name)
	for _, substr := range filteredTopLevelContains {
		for i := 0; i <= len(className)-len(substr); i++ {
			if className[i:i+len(substr)] == substr {
				return true
			}
		}
	}
	
	return false
}

// containsSubstring checks if s contains substr (case-insensitive).
func containsSubstring(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	// Simple case-insensitive contains
	sLower := toLower(s)
	substrLower := toLower(substr)
	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return true
		}
	}
	return false
}

// toLower converts a string to lowercase (ASCII only).
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		} else {
			b[i] = c
		}
	}
	return string(b)
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
// This matches IDEA's Biggest Objects view: shows all objects sorted by retained size,
// with basic types (primitive arrays, Object[], etc.) filtered out.
// filterBasicTypes: if true, filters out basic types like primitive arrays.
func (b *BiggestObjectsBuilder) BuildBiggestObjectsFiltered(topN int, sortBy string, filterBasicTypes bool) []*BiggestObject {
	if b.refGraph == nil {
		return nil
	}

	if topN <= 0 {
		topN = 100
	}

	// Ensure dominator tree is computed for retained sizes
	b.refGraph.ComputeDominatorTree()

	// Collect all reachable objects (not just dominator tree roots).
	// This matches IDEA's Biggest Objects behavior which shows all objects by retained size.
	objects := make([]objectWithSize, 0, len(b.refGraph.objectClass)/10)
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
			retainedSize: b.refGraph.GetRetainedSize(objID),
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
			retainedSize: b.refGraph.GetRetainedSize(objID),
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
		RetainedSize: b.refGraph.GetRetainedSize(objectID),
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
// Also populates ShallowSize and RetainedSize for reference fields.
// This method now traverses the entire class hierarchy to get all fields including inherited ones.
func (b *BiggestObjectsBuilder) extractFields(objectID uint64, layout *ClassFieldLayout) []*ObjectField {
	if layout == nil {
		return nil
	}

	// Ensure dominator tree is computed for retained sizes
	b.refGraph.ComputeDominatorTree()

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
				field.ShallowSize = b.refGraph.objectSize[sf.RefID]
				field.RetainedSize = b.refGraph.GetRetainedSize(sf.RefID)
				field.HasChildren = len(b.refGraph.outgoingRefs[sf.RefID]) > 0
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

	// Collect all instance fields from the class hierarchy (including parent classes)
	allInstanceFields := b.getClassHierarchyFields(layout)

	// Process all instance fields from class hierarchy
	for _, f := range allInstanceFields {
		field := &ObjectField{
			Name: f.Name,
			Type: basicTypeToString(f.Type),
		}
		if f.Type == TypeObject {
			if ref, ok := refByField[f.Name]; ok {
				field.RefID = ref.ToObjectID
				if refClassID, ok := b.refGraph.objectClass[ref.ToObjectID]; ok {
					field.RefClass = b.refGraph.GetClassName(refClassID)
					field.ShallowSize = b.refGraph.objectSize[ref.ToObjectID]
					field.RetainedSize = b.refGraph.GetRetainedSize(ref.ToObjectID)
					field.HasChildren = len(b.refGraph.outgoingRefs[ref.ToObjectID]) > 0
				}
			}
		}
		fields = append(fields, field)
	}

	// If no layout fields, extract from outgoing references directly
	if len(allInstanceFields) == 0 && len(refs) > 0 {
		for _, ref := range refs {
			field := &ObjectField{
				Name:  ref.FieldName,
				Type:  "object",
				RefID: ref.ToObjectID,
			}
			if refClassID, ok := b.refGraph.objectClass[ref.ToObjectID]; ok {
				field.RefClass = b.refGraph.GetClassName(refClassID)
				field.ShallowSize = b.refGraph.objectSize[ref.ToObjectID]
				field.RetainedSize = b.refGraph.GetRetainedSize(ref.ToObjectID)
				field.HasChildren = len(b.refGraph.outgoingRefs[ref.ToObjectID]) > 0
			}
			fields = append(fields, field)
		}
	}

	return fields
}

// getClassHierarchyFields returns all instance fields from the class hierarchy.
// This includes fields from the current class and all parent classes.
func (b *BiggestObjectsBuilder) getClassHierarchyFields(layout *ClassFieldLayout) []FieldInfo {
	if layout == nil || b.classLayouts == nil {
		return nil
	}

	var allFields []FieldInfo

	// Traverse class hierarchy from current class to root
	currentLayout := layout
	for currentLayout != nil {
		// Add fields from current class
		allFields = append(allFields, currentLayout.InstanceFields...)

		// Move to parent class
		if currentLayout.SuperClassID == 0 {
			break
		}
		parentLayout, ok := b.classLayouts[currentLayout.SuperClassID]
		if !ok {
			break
		}
		currentLayout = parentLayout
	}

	return allFields
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
// This method now traverses the entire class hierarchy to get all fields including inherited ones.
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

	// Try to get class layout and collect all fields from class hierarchy
	if layout, ok := b.classLayouts[classID]; ok {
		// Get all instance fields from class hierarchy (including parent classes)
		allInstanceFields := b.getClassHierarchyFields(layout)
		
		// Process all instance fields from class hierarchy
		for _, f := range allInstanceFields {
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
						field.RetainedSize = b.refGraph.GetRetainedSize(ref.ToObjectID)
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
			return b.refGraph.GetRetainedSize(sortedRefs[i].ToObjectID) > b.refGraph.GetRetainedSize(sortedRefs[j].ToObjectID)
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
				RetainedSize: b.refGraph.GetRetainedSize(ref.ToObjectID),
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
		RetainedSize: b.refGraph.GetRetainedSize(objectID),
		HasChildren:  len(b.refGraph.outgoingRefs[objectID]) > 0,
	}
}

// DebugClassLoaderRetainedSize analyzes why a ClassLoader's retained size differs from IDEA.
// This function prints detailed debug information about the ClassLoader instance,
// its fields, their dominators, and why certain objects are not counted in retained size.
//
// Deprecated: Use RetainedSizeAnalyzer.AnalyzeClassWithDebug() for more structured analysis.
func (b *BiggestObjectsBuilder) DebugClassLoaderRetainedSize(className string) {
	analyzer := NewRetainedSizeAnalyzer(b.refGraph)
	analyzer.AnalyzeClassWithDebug(className)
}

// GetRetainedSizeAnalyzer returns a RetainedSizeAnalyzer for advanced analysis.
func (b *BiggestObjectsBuilder) GetRetainedSizeAnalyzer() *RetainedSizeAnalyzer {
	return NewRetainedSizeAnalyzer(b.refGraph)
}

// GetRetainedSizeAnalyzerWithConfig returns a RetainedSizeAnalyzer with custom configuration.
func (b *BiggestObjectsBuilder) GetRetainedSizeAnalyzerWithConfig(config *AnalyzerConfig) *RetainedSizeAnalyzer {
	return NewRetainedSizeAnalyzerWithConfig(b.refGraph, config)
}
