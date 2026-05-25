// Package webui provides the web UI server for performance analysis.
// This file implements HeapQueryEngine - the on-demand query engine for heap analysis.
// It operates on the HeapGraph interface (backed by compact CSR-format data) and provides
// efficient runtime queries without requiring full pre-computation.
package webui

import (
	"container/heap"
	"sort"
	"sync"

	"github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// HeapQueryEngine provides on-demand heap analysis queries using
// pre-computed compact index data (CSR format). It depends on the
// HeapGraph interface for decoupling from concrete graph implementations.
type HeapQueryEngine struct {
	graph     hprof.HeapGraph
	assembler *hprof.ObjectInfoAssembler

	// Lazy-computed lookup tables
	classNameToID     map[string]uint64
	classNameToIDOnce sync.Once

	// Lazy-computed dominator children set (which nodes have dominated children)
	domHasChildren     map[int32]bool
	domHasChildrenOnce sync.Once
}

// NewHeapQueryEngine creates a new HeapQueryEngine from a HeapGraph implementation.
func NewHeapQueryEngine(graph hprof.HeapGraph) *HeapQueryEngine {
	return &HeapQueryEngine{
		graph:     graph,
		assembler: hprof.NewObjectInfoAssembler(graph),
	}
}

// GetGraph returns the underlying HeapGraph for use by external query utilities.
func (e *HeapQueryEngine) GetGraph() hprof.HeapGraph {
	return e.graph
}

// BiggestObjectResult represents a single result from biggest objects query.
type BiggestObjectResult struct {
	ObjectID     string `json:"object_id"`
	ClassName    string `json:"class_name"`
	ShallowSize  int64  `json:"shallow_size"`
	RetainedSize int64  `json:"retained_size"`
}

// GCRootPathResult represents a path from an object to a GC root.
type GCRootPathResult struct {
	RootType string               `json:"root_type"`
	Depth    int                  `json:"depth"`
	Path     []GCRootPathNodeResult `json:"path"`
}

// GCRootPathNodeResult represents a node in a GC root path.
type GCRootPathNodeResult struct {
	ObjectID  string `json:"object_id"`
	ClassName string `json:"class_name"`
	FieldName string `json:"field_name,omitempty"`
	Size      int64  `json:"size"`
}

// RetainerResult represents an object that retains (holds reference to) another.
type RetainerResult struct {
	ObjectID     string `json:"object_id"`
	ClassName    string `json:"class_name"`
	FieldName    string `json:"field_name"`
	ShallowSize  int64  `json:"shallow_size"`
	RetainedSize int64  `json:"retained_size"`
}

// ObjectFieldResult represents a field (outgoing reference) of an object.
type ObjectFieldResult struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	RefID        string `json:"ref_id,omitempty"`
	RefClass     string `json:"ref_class,omitempty"`
	ShallowSize  int64  `json:"shallow_size,omitempty"`
	RetainedSize int64  `json:"retained_size,omitempty"`
	HasChildren  bool   `json:"has_children"`
}

// GCRootSummaryResult represents a group of GC roots by class/type.
type GCRootSummaryResult struct {
	ClassName     string           `json:"class_name"`
	RootType      string           `json:"root_type"`
	InstanceCount int              `json:"instance_count"`
	TotalShallow  int64            `json:"total_shallow"`
	TotalRetained int64            `json:"total_retained"`
	Roots         []GCRootInstance `json:"roots,omitempty"`
}

// GCRootInstance represents a single GC root object instance within a class group.
type GCRootInstance struct {
	ObjectID     string `json:"object_id"`
	ShallowSize  int64  `json:"shallow_size"`
	RetainedSize int64  `json:"retained_size"`
}

// QueryObjectInfo returns basic information about a single object by its ID.
// Returns nil if the object is not found.
func (e *HeapQueryEngine) QueryObjectInfo(objectID uint64) *hprof.HeapObjectInfo {
	return e.assembler.AssembleByObjectID(objectID)
}

// QueryBiggestObjects returns top N objects sorted by retained size.
// Supports filtering by class name. Time: O(N) scan + O(topN log topN) sort.
func (e *HeapQueryEngine) QueryBiggestObjects(topN int, sortBy string, classFilter string) []BiggestObjectResult {
	if topN <= 0 {
		topN = 50
	}

	// Use a min-heap to efficiently find top N
	h := &biggestObjectHeap{}
	heap.Init(h)

	var filterClassID uint64
	if classFilter != "" {
		filterClassID = e.resolveClassID(classFilter)
		if filterClassID == 0 {
			return nil // class not found
		}
	}

	objectCount := e.graph.ObjectCount()
	for i := int32(0); i < objectCount; i++ {
		if !e.graph.IsReachable(i) {
			continue
		}

		if filterClassID != 0 && e.graph.GetClassID(i) != filterClassID {
			continue
		}

		sortValue := e.getSortValue(i, sortBy)

		if h.Len() < topN {
			heap.Push(h, biggestObjectEntry{idx: i, sortValue: sortValue})
		} else if sortValue > (*h)[0].sortValue {
			(*h)[0] = biggestObjectEntry{idx: i, sortValue: sortValue}
			heap.Fix(h, 0)
		}
	}

	// Extract results in descending order
	results := make([]BiggestObjectResult, h.Len())
	for i := len(results) - 1; i >= 0; i-- {
		entry := heap.Pop(h).(biggestObjectEntry)
		info := e.assembler.AssembleByIndex(entry.idx)
		results[i] = BiggestObjectResult{
			ObjectID:     info.ObjectID,
			ClassName:    info.ClassName,
			ShallowSize:  info.ShallowSize,
			RetainedSize: info.RetainedSize,
		}
	}

	return results
}

// bfsNode represents a node in BFS traversal for GC root path finding.
type bfsNode struct {
	idx    int32
	parent int32
	field  string
	depth  int
}

// QueryGCRootPath finds shortest paths from an object to GC roots.
// Uses BFS on inEdges CSR, bounded by maxPaths and maxDepth.
func (e *HeapQueryEngine) QueryGCRootPath(objectID uint64, maxPaths int, maxDepth int) []GCRootPathResult {
	if maxPaths <= 0 {
		maxPaths = 3
	}
	if maxDepth <= 0 {
		maxDepth = 15
	}

	startIdx := e.graph.GetObjectIndex(objectID)
	if startIdx < 0 {
		return nil
	}

	var results []GCRootPathResult
	visited := make(map[int32]bool)
	queue := []bfsNode{{idx: startIdx, parent: -1, depth: 0}}
	visited[startIdx] = true

	// Store parent chain for path reconstruction
	parents := make(map[int32]bfsNode)

	for len(queue) > 0 && len(results) < maxPaths {
		current := queue[0]
		queue = queue[1:]

		if current.depth > maxDepth {
			continue
		}

		// Check if current is a GC root
		if e.graph.IsGCRoot(current.idx) && current.idx != startIdx {
			// Reconstruct path
			path := e.reconstructPath(startIdx, current.idx, parents)
			if path != nil {
				results = append(results, *path)
			}
			if len(results) >= maxPaths {
				break
			}
			continue
		}

		// Expand: get incoming references (who points to this object)
		sources, fieldIDs, _ := e.graph.GetIncomingEdges(current.idx)
		for i, sourceIdx := range sources {
			if visited[sourceIdx] {
				continue
			}
			visited[sourceIdx] = true

			fieldName := e.assembler.ResolveFieldName(fieldIDs, i)

			parents[sourceIdx] = bfsNode{
				idx:    sourceIdx,
				parent: current.idx,
				field:  fieldName,
			}
			queue = append(queue, bfsNode{
				idx:    sourceIdx,
				parent: current.idx,
				field:  fieldName,
				depth:  current.depth + 1,
			})
		}
	}

	return results
}

// reconstructPath builds a GCRootPathResult by walking the parent chain.
func (e *HeapQueryEngine) reconstructPath(startIdx, rootIdx int32, parents map[int32]bfsNode) *GCRootPathResult {
	var pathNodes []GCRootPathNodeResult

	// Walk from root back to start
	current := rootIdx
	for current != startIdx {
		info := e.assembler.AssembleByIndex(current)
		node := GCRootPathNodeResult{
			ObjectID:  info.ObjectID,
			ClassName: info.ClassName,
			Size:      info.RetainedSize,
		}
		if parent, ok := parents[current]; ok {
			node.FieldName = parent.field
			current = parent.parent
		} else {
			break
		}
		pathNodes = append(pathNodes, node)
	}

	// Add the start object
	startInfo := e.assembler.AssembleByIndex(startIdx)
	pathNodes = append(pathNodes, GCRootPathNodeResult{
		ObjectID:  startInfo.ObjectID,
		ClassName: startInfo.ClassName,
		Size:      startInfo.RetainedSize,
	})

	// Reverse to get root → object order
	for i, j := 0, len(pathNodes)-1; i < j; i, j = i+1, j-1 {
		pathNodes[i], pathNodes[j] = pathNodes[j], pathNodes[i]
	}

	// Determine root type
	rootType := "UNKNOWN"
	// GC root type would be stored in the graph - for now use generic
	if e.graph.IsGCRoot(rootIdx) {
		rootType = "GC_ROOT"
	}

	return &GCRootPathResult{
		RootType: rootType,
		Depth:    len(pathNodes),
		Path:     pathNodes,
	}
}

// QueryRetainers returns objects that hold a reference to the given object.
// Direct lookup in inEdges CSR - O(degree) for the target object.
func (e *HeapQueryEngine) QueryRetainers(objectID uint64, maxRetainers int) []RetainerResult {
	if maxRetainers <= 0 {
		maxRetainers = 20
	}

	objIdx := e.graph.GetObjectIndex(objectID)
	if objIdx < 0 {
		return nil
	}

	sources, fieldIDs, _ := e.graph.GetIncomingEdges(objIdx)
	if len(sources) == 0 {
		return nil
	}

	limit := maxRetainers
	if limit > len(sources) {
		limit = len(sources)
	}

	results := make([]RetainerResult, 0, limit)
	for i := 0; i < limit; i++ {
		info := e.assembler.AssembleByIndex(sources[i])
		fieldName := e.assembler.ResolveFieldName(fieldIDs, i)

		results = append(results, RetainerResult{
			ObjectID:     info.ObjectID,
			ClassName:    info.ClassName,
			FieldName:    fieldName,
			ShallowSize:  info.ShallowSize,
			RetainedSize: info.RetainedSize,
		})
	}

	// Sort by retained size descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].RetainedSize > results[j].RetainedSize
	})

	return results
}

// QueryObjectFields returns outgoing references (fields) of an object.
// Direct lookup in outEdges CSR - O(degree) for the source object.
func (e *HeapQueryEngine) QueryObjectFields(objectID uint64) []ObjectFieldResult {
	objIdx := e.graph.GetObjectIndex(objectID)
	if objIdx < 0 {
		return nil
	}

	targets, fieldIDs, _ := e.graph.GetOutgoingEdges(objIdx)
	if len(targets) == 0 {
		return nil
	}

	results := make([]ObjectFieldResult, 0, len(targets))
	for i, targetIdx := range targets {
		if targetIdx < 0 {
			continue
		}

		info := e.assembler.AssembleByIndex(targetIdx)
		fieldName := e.assembler.ResolveFieldName(fieldIDs, i)

		// Check if target has outgoing edges (children)
		_, tgtFieldIDs, _ := e.graph.GetOutgoingEdges(targetIdx)
		hasChildren := len(tgtFieldIDs) > 0

		results = append(results, ObjectFieldResult{
			Name:         fieldName,
			Type:         info.ClassName,
			RefID:        info.ObjectID,
			RefClass:     info.ClassName,
			ShallowSize:  info.ShallowSize,
			RetainedSize: info.RetainedSize,
			HasChildren:  hasChildren,
		})
	}

	return results
}

// QueryClassInstances returns instances of a class sorted by retained size.
func (e *HeapQueryEngine) QueryClassInstances(className string, topN int, sortBy string) []BiggestObjectResult {
	if topN <= 0 {
		topN = 50
	}

	classID := e.resolveClassID(className)
	if classID == 0 {
		return nil
	}

	objects := e.graph.GetObjectsByClass(classID)
	if len(objects) == 0 {
		return nil
	}

	// Sort by size
	type indexedObj struct {
		idx       int32
		sortValue int64
	}
	sorted := make([]indexedObj, 0, len(objects))
	for _, idx := range objects {
		if !e.graph.IsReachable(idx) {
			continue
		}
		sorted = append(sorted, indexedObj{idx: idx, sortValue: e.getSortValue(idx, sortBy)})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].sortValue > sorted[j].sortValue
	})

	if len(sorted) > topN {
		sorted = sorted[:topN]
	}

	results := make([]BiggestObjectResult, len(sorted))
	for i, obj := range sorted {
		info := e.assembler.AssembleByIndex(obj.idx)
		results[i] = BiggestObjectResult{
			ObjectID:     info.ObjectID,
			ClassName:    info.ClassName,
			ShallowSize:  info.ShallowSize,
			RetainedSize: info.RetainedSize,
		}
	}

	return results
}

// ClassRetainerResult represents a class that retains (holds references to) instances of a target class.
// This is class-level aggregation: "which classes hold references to instances of the target class?"
type ClassRetainerResult struct {
	RetainerClass string  `json:"retainer_class"`
	FieldName     string  `json:"field_name,omitempty"`
	RetainedSize  int64   `json:"retained_size"`
	RetainedCount int64   `json:"retained_count"`
	Percentage    float64 `json:"percentage"`
}

// QueryClassRetainers returns class-level retainers for a given target class.
// It finds all instances of the target class, then aggregates their incoming edges
// by source class name. This answers: "who holds references to instances of this class?"
// Complexity: O(instances_of_class × avg_in_degree) — efficient with CSR format.
func (e *HeapQueryEngine) QueryClassRetainers(className string, topN int) []ClassRetainerResult {
	if topN <= 0 {
		topN = 20
	}

	classID := e.resolveClassID(className)
	if classID == 0 {
		return nil
	}

	objects := e.graph.GetObjectsByClass(classID)
	if len(objects) == 0 {
		return nil
	}

	// Aggregate retainers by (retainerClassName, fieldName)
	type retainerKey struct {
		className string
		fieldName string
	}
	aggMap := make(map[retainerKey]*ClassRetainerResult)
	var totalRetainedSize int64

	for _, objIdx := range objects {
		if !e.graph.IsReachable(objIdx) {
			continue
		}

		objShallow := e.graph.GetShallowSize(objIdx)
		totalRetainedSize += objShallow

		sources, fieldIDs, _ := e.graph.GetIncomingEdges(objIdx)
		for i, srcIdx := range sources {
			srcClassName := e.assembler.GetClassNameByIndex(srcIdx)
			if srcClassName == "" {
				srcClassName = "<unknown>"
			}
			fieldName := e.assembler.ResolveFieldName(fieldIDs, i)

			key := retainerKey{className: srcClassName, fieldName: fieldName}
			if agg, ok := aggMap[key]; ok {
				agg.RetainedCount++
				agg.RetainedSize += objShallow
			} else {
				aggMap[key] = &ClassRetainerResult{
					RetainerClass: srcClassName,
					FieldName:     fieldName,
					RetainedCount: 1,
					RetainedSize:  objShallow,
				}
			}
		}
	}

	// Convert to slice, compute percentages, and sort
	results := make([]ClassRetainerResult, 0, len(aggMap))
	for _, r := range aggMap {
		if totalRetainedSize > 0 {
			r.Percentage = float64(r.RetainedSize) * 100.0 / float64(totalRetainedSize)
		}
		results = append(results, *r)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].RetainedSize > results[j].RetainedSize
	})

	if len(results) > topN {
		results = results[:topN]
	}

	return results
}

// QueryGCRootsSummary returns GC roots grouped by class and type.
// Each class group includes up to maxInstancesPerClass concrete instances for drill-down.
func (e *HeapQueryEngine) QueryGCRootsSummary() []GCRootSummaryResult {
	const maxInstancesPerClass = 50

	// Group GC roots by class
	type key struct {
		className string
		rootType  string
	}
	groups := make(map[key]*GCRootSummaryResult)
	// Collect instance indices per class for later assembly
	instanceIndices := make(map[key][]int32)

	objectCount := e.graph.ObjectCount()
	for i := int32(0); i < objectCount; i++ {
		if !e.graph.IsGCRoot(i) {
			continue
		}
		className := e.assembler.GetClassNameByIndex(i)
		if className == "" {
			className = "<unknown>"
		}

		k := key{className: className, rootType: "GC_ROOT"}
		if result, ok := groups[k]; ok {
			result.InstanceCount++
			result.TotalShallow += e.graph.GetShallowSize(i)
			result.TotalRetained += e.graph.GetRetainedSize(i)
		} else {
			groups[k] = &GCRootSummaryResult{
				ClassName:     className,
				RootType:      "GC_ROOT",
				InstanceCount: 1,
				TotalShallow:  e.graph.GetShallowSize(i),
				TotalRetained: e.graph.GetRetainedSize(i),
			}
		}

		// Collect instance indices (cap at maxInstancesPerClass)
		if indices := instanceIndices[k]; len(indices) < maxInstancesPerClass {
			instanceIndices[k] = append(indices, i)
		}
	}

	// Convert to slice, attach instance details, and sort by total retained descending
	results := make([]GCRootSummaryResult, 0, len(groups))
	for k, r := range groups {
		// Assemble instance details for this class group
		indices := instanceIndices[k]
		if len(indices) > 0 {
			r.Roots = make([]GCRootInstance, 0, len(indices))
			for _, idx := range indices {
				info := e.assembler.AssembleByIndex(idx)
				r.Roots = append(r.Roots, GCRootInstance{
					ObjectID:     info.ObjectID,
					ShallowSize:  info.ShallowSize,
					RetainedSize: info.RetainedSize,
				})
			}
		}
		results = append(results, *r)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalRetained > results[j].TotalRetained
	})

	return results
}

// DominatorChildResult represents a child node in the dominator tree.
type DominatorChildResult struct {
	ObjectID     string `json:"object_id"`
	ClassName    string `json:"class_name"`
	ShallowSize  int64  `json:"shallow_size"`
	RetainedSize int64  `json:"retained_size"`
	HasChildren  bool   `json:"has_children"`
	IsGCRoot     bool   `json:"is_gc_root"`
}

// DominatorPathResult represents a node in the dominator path from root to object.
type DominatorPathResult struct {
	ObjectID     string `json:"object_id"`
	ClassName    string `json:"class_name"`
	ShallowSize  int64  `json:"shallow_size"`
	RetainedSize int64  `json:"retained_size"`
	Depth        int    `json:"depth"`
}

// TreemapNodeResult represents a node in the retained size treemap.
type TreemapNodeResult struct {
	Name     string              `json:"name"`
	Value    int64               `json:"value"`
	ObjectID string              `json:"object_id,omitempty"`
	Children []TreemapNodeResult `json:"children,omitempty"`
}

// QueryDominatorChildren returns direct dominated children of a given object,
// sorted by retained size descending. If idx == -1, returns the virtual root's
// children (objects whose dominator is -1, i.e., directly dominated by super root).
func (e *HeapQueryEngine) QueryDominatorChildren(objectID uint64, topN int, sortBy string) []DominatorChildResult {
	if topN <= 0 {
		topN = 50
	}

	parentIdx := int32(-1)
	if objectID != 0 {
		parentIdx = e.graph.GetObjectIndex(objectID)
		if parentIdx < 0 {
			return nil
		}
	}

	// Use a min-heap to find top N children by retained size
	h := &biggestObjectHeap{}
	heap.Init(h)

	objectCount := e.graph.ObjectCount()
	for i := int32(0); i < objectCount; i++ {
		if !e.graph.IsReachable(i) {
			continue
		}

		dominator := e.graph.GetDominator(i)
		if dominator != parentIdx {
			continue
		}

		sortValue := e.getSortValue(i, sortBy)

		if h.Len() < topN {
			heap.Push(h, biggestObjectEntry{idx: i, sortValue: sortValue})
		} else if sortValue > (*h)[0].sortValue {
			(*h)[0] = biggestObjectEntry{idx: i, sortValue: sortValue}
			heap.Fix(h, 0)
		}
	}

	// Extract results in descending order
	results := make([]DominatorChildResult, h.Len())
	for i := len(results) - 1; i >= 0; i-- {
		entry := heap.Pop(h).(biggestObjectEntry)
		info := e.assembler.AssembleByIndex(entry.idx)
		hasChildren := e.hasDominatedChildren(entry.idx)
		results[i] = DominatorChildResult{
			ObjectID:     info.ObjectID,
			ClassName:    info.ClassName,
			ShallowSize:  info.ShallowSize,
			RetainedSize: info.RetainedSize,
			HasChildren:  hasChildren,
			IsGCRoot:     e.graph.IsGCRoot(entry.idx),
		}
	}

	return results
}

// hasDominatedChildren checks if an object has at least one dominated child.
// Uses lazy-computed lookup map for O(1) per query after first build.
func (e *HeapQueryEngine) hasDominatedChildren(parentIdx int32) bool {
	e.domHasChildrenOnce.Do(func() {
		e.domHasChildren = make(map[int32]bool)
		objectCount := e.graph.ObjectCount()
		for i := int32(0); i < objectCount; i++ {
			if !e.graph.IsReachable(i) {
				continue
			}
			dominator := e.graph.GetDominator(i)
			if dominator >= 0 {
				e.domHasChildren[dominator] = true
			} else {
				// dominator == -1 means dominated by virtual root (-1)
				e.domHasChildren[-1] = true
			}
		}
	})
	return e.domHasChildren[parentIdx]
}

// QueryDominatorPath returns the dominator chain from virtual root to the given object.
func (e *HeapQueryEngine) QueryDominatorPath(objectID uint64) []DominatorPathResult {
	startIdx := e.graph.GetObjectIndex(objectID)
	if startIdx < 0 {
		return nil
	}

	// Walk up the dominator tree
	var path []DominatorPathResult
	current := startIdx
	depth := 0

	for current >= 0 {
		info := e.assembler.AssembleByIndex(current)
		path = append(path, DominatorPathResult{
			ObjectID:     info.ObjectID,
			ClassName:    info.ClassName,
			ShallowSize:  info.ShallowSize,
			RetainedSize: info.RetainedSize,
			Depth:        depth,
		})
		depth++

		dominator := e.graph.GetDominator(current)
		if dominator < 0 {
			break
		}
		current = dominator

		// Safety limit to prevent infinite loops
		if depth > 100 {
			break
		}
	}

	// Reverse to get root → object order
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	// Fix depth values after reversal
	for i := range path {
		path[i].Depth = i
	}

	return path
}

// QueryRetainedSizeTreemap returns treemap data for retained size visualization.
// It returns the top-level dominated children of the given root (or virtual root if objectID==0),
// grouped by class for a hierarchical treemap visualization.
func (e *HeapQueryEngine) QueryRetainedSizeTreemap(objectID uint64, maxNodes int) []TreemapNodeResult {
	if maxNodes <= 0 {
		maxNodes = 200
	}

	parentIdx := int32(-1)
	if objectID != 0 {
		parentIdx = e.graph.GetObjectIndex(objectID)
		if parentIdx < 0 {
			return nil
		}
	}

	// Collect children grouped by class
	type classGroup struct {
		classID      uint64
		className    string
		totalRetained int64
		objects      []int32
	}

	groups := make(map[uint64]*classGroup)
	objectCount := e.graph.ObjectCount()

	for i := int32(0); i < objectCount; i++ {
		if !e.graph.IsReachable(i) {
			continue
		}
		if e.graph.GetDominator(i) != parentIdx {
			continue
		}

		classID := e.graph.GetClassID(i)
		retained := e.graph.GetRetainedSize(i)

		if g, ok := groups[classID]; ok {
			g.totalRetained += retained
			g.objects = append(g.objects, i)
		} else {
			groups[classID] = &classGroup{
				classID:       classID,
				className:     e.graph.GetClassName(classID),
				totalRetained: retained,
				objects:       []int32{i},
			}
		}
	}

	// Sort groups by total retained size
	sortedGroups := make([]*classGroup, 0, len(groups))
	for _, g := range groups {
		sortedGroups = append(sortedGroups, g)
	}
	sort.Slice(sortedGroups, func(i, j int) bool {
		return sortedGroups[i].totalRetained > sortedGroups[j].totalRetained
	})

	// Limit total nodes
	totalNodes := 0
	var results []TreemapNodeResult
	for _, g := range sortedGroups {
		if totalNodes >= maxNodes {
			break
		}

		// Sort objects within group by retained size
		sort.Slice(g.objects, func(i, j int) bool {
			return e.graph.GetRetainedSize(g.objects[i]) > e.graph.GetRetainedSize(g.objects[j])
		})

		// Build children for this class group
		var children []TreemapNodeResult
		for _, idx := range g.objects {
			if totalNodes >= maxNodes {
				break
			}
			info := e.assembler.AssembleByIndex(idx)
			children = append(children, TreemapNodeResult{
				Name:     info.ClassName + " @" + info.ObjectID[2:], // Remove "0x" prefix for short display
				Value:    info.RetainedSize,
				ObjectID: info.ObjectID,
			})
			totalNodes++
		}

		node := TreemapNodeResult{
			Name:     g.className,
			Value:    g.totalRetained,
			Children: children,
		}
		results = append(results, node)
	}

	return results
}

// getSortValue returns the appropriate size value for sorting.
func (e *HeapQueryEngine) getSortValue(idx int32, sortBy string) int64 {
	switch sortBy {
	case "shallow":
		return e.graph.GetShallowSize(idx)
	default: // "retained" or empty
		return e.graph.GetRetainedSize(idx)
	}
}

// resolveClassID finds the classID for a given class name.
func (e *HeapQueryEngine) resolveClassID(className string) uint64 {
	e.classNameToIDOnce.Do(func() {
		e.classNameToID = make(map[string]uint64)
		objectCount := e.graph.ObjectCount()
		seen := make(map[uint64]bool)
		for i := int32(0); i < objectCount; i++ {
			classID := e.graph.GetClassID(i)
			if seen[classID] {
				continue
			}
			seen[classID] = true
			name := e.graph.GetClassName(classID)
			if name != "" {
				e.classNameToID[name] = classID
			}
		}
	})
	return e.classNameToID[className]
}

// biggestObjectEntry is used by the min-heap for top-N selection.
type biggestObjectEntry struct {
	idx       int32
	sortValue int64
}

// biggestObjectHeap implements heap.Interface for top-N selection (min-heap).
type biggestObjectHeap []biggestObjectEntry

func (h biggestObjectHeap) Len() int            { return len(h) }
func (h biggestObjectHeap) Less(i, j int) bool  { return h[i].sortValue < h[j].sortValue }
func (h biggestObjectHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *biggestObjectHeap) Push(x interface{}) { *h = append(*h, x.(biggestObjectEntry)) }
func (h *biggestObjectHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}
