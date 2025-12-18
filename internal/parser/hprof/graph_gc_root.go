// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import "strings"

// GCRootType represents the type of GC root.
type GCRootType string

const (
	GCRootUnknown      GCRootType = "UNKNOWN"
	GCRootJNIGlobal    GCRootType = "JNI_GLOBAL"
	GCRootJNILocal     GCRootType = "JNI_LOCAL"
	GCRootJavaFrame    GCRootType = "JAVA_FRAME"
	GCRootNativeStack  GCRootType = "NATIVE_STACK"
	GCRootStickyClass  GCRootType = "STICKY_CLASS"
	GCRootThreadBlock  GCRootType = "THREAD_BLOCK"
	GCRootMonitorUsed  GCRootType = "MONITOR_USED"
	GCRootThreadObject GCRootType = "THREAD_OBJECT"
)

// GCRoot represents a garbage collection root.
type GCRoot struct {
	ObjectID   uint64     `json:"object_id"`
	Type       GCRootType `json:"type"`
	ThreadID   uint64     `json:"thread_id,omitempty"`
	FrameIndex int        `json:"frame_index,omitempty"`
}

// PathNode represents a node in a reference path from GC Root to object.
type PathNode struct {
	ObjectID  uint64 `json:"object_id"`
	ClassID   uint64 `json:"class_id"`
	ClassName string `json:"class_name"`
	FieldName string `json:"field_name,omitempty"`
	Size      int64  `json:"size"`
}

// GCRootPath represents a path from a GC Root to an object.
type GCRootPath struct {
	RootType GCRootType  `json:"root_type"`
	Path     []*PathNode `json:"path"`
	Depth    int         `json:"depth"`
}

// AddGCRoot adds a GC root to the graph.
func (g *ReferenceGraph) AddGCRoot(root *GCRoot) {
	g.gcRoots = append(g.gcRoots, root)
	g.gcRootSet[root.ObjectID] = root.Type
}

// IsGCRoot checks if an object is a GC root.
func (g *ReferenceGraph) IsGCRoot(objectID uint64) bool {
	_, ok := g.gcRootSet[objectID]
	return ok
}

// GetGCRootType returns the GC root type for an object.
func (g *ReferenceGraph) GetGCRootType(objectID uint64) GCRootType {
	if t, ok := g.gcRootSet[objectID]; ok {
		return t
	}
	return ""
}

// FindPathsToGCRoot finds paths from an object to GC roots using BFS.
// maxPaths limits the number of paths returned.
// maxDepth limits the search depth.
//
// OPTIMIZATION: Uses iterative deepening DFS instead of BFS with visited map copying.
// This reduces memory allocation from O(paths * depth * objects) to O(depth).
func (g *ReferenceGraph) FindPathsToGCRoot(objectID uint64, maxPaths, maxDepth int) []*GCRootPath {
	if maxPaths <= 0 {
		maxPaths = 3
	}
	if maxDepth <= 0 {
		maxDepth = 15
	}

	var paths []*GCRootPath

	// Use iterative deepening DFS for memory efficiency
	// Start with shorter paths first (like BFS) but with O(depth) memory
	for targetDepth := 1; targetDepth <= maxDepth && len(paths) < maxPaths; targetDepth++ {
		g.findPathsDFS(objectID, targetDepth, maxPaths-len(paths), &paths)
	}

	return paths
}

// findPathsDFS performs depth-limited DFS to find paths to GC roots.
// Uses a single visited set and path slice, avoiding repeated allocations.
func (g *ReferenceGraph) findPathsDFS(startID uint64, maxDepth, maxPaths int, paths *[]*GCRootPath) {
	if maxPaths <= 0 {
		return
	}

	// Use pooled slice for path building
	pathSlice := GetUint64Slice()
	defer PutUint64Slice(pathSlice)

	// Use pooled map for visited tracking (uint64 keys for object IDs)
	visited := GetUint64BoolMap()
	defer PutUint64BoolMap(visited)

	// Stack-based DFS state
	type stackFrame struct {
		objID    uint64
		refIndex int // Index into incomingRefs for resumption
	}

	stack := make([]stackFrame, 0, maxDepth+1)
	stack = append(stack, stackFrame{objID: startID, refIndex: 0})
	*pathSlice = append(*pathSlice, startID)
	visited[startID] = true

	for len(stack) > 0 && len(*paths) < maxPaths {
		frame := &stack[len(stack)-1]

		// Check if current node is a GC root
		if rootType, isRoot := g.gcRootSet[frame.objID]; isRoot && frame.refIndex == 0 {
			// Build and add path (reverse order: from GC root to target)
			pathNodes := make([]*PathNode, len(*pathSlice))
			for i, objID := range *pathSlice {
				classID := g.objectClass[objID]
				pathNodes[len(*pathSlice)-1-i] = &PathNode{
					ObjectID:  objID,
					ClassID:   classID,
					ClassName: g.classNames[classID],
					Size:      g.objectSize[objID],
				}
			}
			// Add field names (from the references)
			for i := 0; i < len(pathNodes)-1; i++ {
				// Find the reference from pathNodes[i] to pathNodes[i+1]
				for _, ref := range g.outgoingRefs[pathNodes[i].ObjectID] {
					if ref.ToObjectID == pathNodes[i+1].ObjectID {
						pathNodes[i+1].FieldName = ref.FieldName
						break
					}
				}
			}
			*paths = append(*paths, &GCRootPath{
				RootType: rootType,
				Path:     pathNodes,
				Depth:    len(pathNodes),
			})
		}

		// Check depth limit
		if len(stack) >= maxDepth {
			// Backtrack
			delete(visited, frame.objID)
			*pathSlice = (*pathSlice)[:len(*pathSlice)-1]
			stack = stack[:len(stack)-1]
			continue
		}

		// Find next unvisited incoming reference
		refs := g.incomingRefs[frame.objID]
		foundNext := false
		for frame.refIndex < len(refs) {
			ref := refs[frame.refIndex]
			frame.refIndex++

			if !visited[ref.FromObjectID] {
				// Push new frame
				visited[ref.FromObjectID] = true
				*pathSlice = append(*pathSlice, ref.FromObjectID)
				stack = append(stack, stackFrame{objID: ref.FromObjectID, refIndex: 0})
				foundNext = true
				break
			}
		}

		if !foundNext {
			// Backtrack
			delete(visited, frame.objID)
			*pathSlice = (*pathSlice)[:len(*pathSlice)-1]
			stack = stack[:len(stack)-1]
		}
	}
}

// FindNonArrayRetainers finds non-array classes that hold references to array objects.
// This helps identify business classes that might be the root cause of memory issues.
func (g *ReferenceGraph) FindNonArrayRetainers(topN int) map[string]int {
	result := make(map[string]int)

	// Iterate through all references
	for _, refs := range g.incomingRefs {
		for _, ref := range refs {
			fromClassName := g.classNames[ref.FromClassID]
			if fromClassName == "" {
				continue
			}

			// Skip array types as retainers
			if strings.HasSuffix(fromClassName, "[]") {
				continue
			}

			// Check if the target is an array type
			toClassID := g.objectClass[ref.ToObjectID]
			toClassName := g.classNames[toClassID]
			if strings.HasSuffix(toClassName, "[]") {
				result[fromClassName]++
			}
		}
	}

	return result
}

// GCRootInfo represents aggregated information about a GC root for display.
type GCRootInfo struct {
	ObjectID     uint64     `json:"object_id"`
	ClassName    string     `json:"class_name"`
	RootType     GCRootType `json:"root_type"`
	ShallowSize  int64      `json:"shallow_size"`
	RetainedSize int64      `json:"retained_size"`
	ThreadID     uint64     `json:"thread_id,omitempty"`
	FrameIndex   int        `json:"frame_index,omitempty"`
}

// GCRootSummary represents a summary of GC roots grouped by class (like IDEA).
type GCRootSummary struct {
	ClassName       string        `json:"class_name"`
	RootType        GCRootType    `json:"root_type,omitempty"`
	TotalShallow    int64         `json:"total_shallow"`
	TotalRetained   int64         `json:"total_retained"`
	InstanceCount   int           `json:"instance_count"`
	Roots           []*GCRootInfo `json:"roots,omitempty"`
	RetainedClasses []string      `json:"retained_classes,omitempty"`
}

// GetGCRootsList returns all GC roots with their information.
// Results are sorted by retained size descending.
func (g *ReferenceGraph) GetGCRootsList() []*GCRootInfo {
	// Ensure dominator tree is computed for accurate retained sizes
	g.ComputeDominatorTree()
	
	result := make([]*GCRootInfo, 0, len(g.gcRoots))
	
	for _, root := range g.gcRoots {
		classID, ok := g.objectClass[root.ObjectID]
		if !ok {
			continue
		}
		
		className := g.classNames[classID]
		if className == "" {
			className = "Unknown"
		}
		
		info := &GCRootInfo{
			ObjectID:     root.ObjectID,
			ClassName:    className,
			RootType:     root.Type,
			ShallowSize:  g.objectSize[root.ObjectID],
			RetainedSize: g.GetRetainedSize(root.ObjectID),
			ThreadID:     root.ThreadID,
			FrameIndex:   root.FrameIndex,
		}
		result = append(result, info)
	}
	
	// Also include Class objects as GC roots (they are held by ClassLoaders)
	for classObjID := range g.classObjectIDs {
		if _, isExplicitRoot := g.gcRootSet[classObjID]; isExplicitRoot {
			continue // Already included
		}
		
		classID, ok := g.objectClass[classObjID]
		if !ok {
			continue
		}
		
		className := g.classNames[classID]
		if className == "" {
			className = "java.lang.Class"
		}
		
		info := &GCRootInfo{
			ObjectID:     classObjID,
			ClassName:    className,
			RootType:     GCRootStickyClass,
			ShallowSize:  g.objectSize[classObjID],
			RetainedSize: g.GetRetainedSize(classObjID),
		}
		result = append(result, info)
	}
	
	// Sort by retained size descending
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].RetainedSize > result[i].RetainedSize {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	
	return result
}

// GetGCRootsSummary returns GC roots grouped by class name (like IDEA).
func (g *ReferenceGraph) GetGCRootsSummary() []*GCRootSummary {
	// Ensure dominator tree is computed
	g.ComputeDominatorTree()
	
	// Group by class name only (like IDEA)
	groups := make(map[string]*GCRootSummary)
	rootTypeCounts := make(map[string]map[GCRootType]int)
	
	allRoots := g.GetGCRootsList()
	for _, root := range allRoots {
		className := root.ClassName
		summary, ok := groups[className]
		if !ok {
			summary = &GCRootSummary{
				ClassName: className,
				Roots:     make([]*GCRootInfo, 0),
			}
			groups[className] = summary
			rootTypeCounts[className] = make(map[GCRootType]int)
		}
		summary.TotalShallow += root.ShallowSize
		summary.TotalRetained += root.RetainedSize
		summary.InstanceCount++
		summary.Roots = append(summary.Roots, root)
		rootTypeCounts[className][root.RootType]++
	}
	
	// Set the primary root type (most common) for each group
	for className, summary := range groups {
		maxCount := 0
		for rootType, count := range rootTypeCounts[className] {
			if count > maxCount {
				maxCount = count
				summary.RootType = rootType
			}
		}
		// Sort roots by retained size descending
		roots := summary.Roots
		for i := 0; i < len(roots)-1; i++ {
			for j := i + 1; j < len(roots); j++ {
				if roots[j].RetainedSize > roots[i].RetainedSize {
					roots[i], roots[j] = roots[j], roots[i]
				}
			}
		}
	}
	
	// Convert to slice and sort by retained size
	result := make([]*GCRootSummary, 0, len(groups))
	for _, summary := range groups {
		result = append(result, summary)
	}
	
	// Sort by retained size descending
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].TotalRetained > result[i].TotalRetained {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	
	return result
}

// GetRetainedObjectsByGCRoot returns the objects directly retained by a GC root.
func (g *ReferenceGraph) GetRetainedObjectsByGCRoot(rootObjectID uint64, maxObjects int) []*GCRootInfo {
	if maxObjects <= 0 {
		maxObjects = 50
	}
	
	result := make([]*GCRootInfo, 0)
	
	// Get outgoing references from the GC root
	refs := g.outgoingRefs[rootObjectID]
	for _, ref := range refs {
		if len(result) >= maxObjects {
			break
		}
		
		classID, ok := g.objectClass[ref.ToObjectID]
		if !ok {
			continue
		}
		
		className := g.classNames[classID]
		if className == "" {
			className = "Unknown"
		}
		
		info := &GCRootInfo{
			ObjectID:     ref.ToObjectID,
			ClassName:    className,
			ShallowSize:  g.objectSize[ref.ToObjectID],
			RetainedSize: g.GetRetainedSize(ref.ToObjectID),
		}
		result = append(result, info)
	}
	
	// Sort by retained size descending
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].RetainedSize > result[i].RetainedSize {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	
	return result
}
