// Package webui provides the web UI server for performance analysis.
// This file implements HeapQueryEngine - the on-demand query engine for heap analysis.
// It operates on compact CSR-format data (IndexedReferenceGraph) and provides
// efficient runtime queries without requiring full pre-computation.
package webui

import (
	"container/heap"
	"fmt"
	"sort"
	"sync"

	"github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
)

// HeapQueryEngine provides on-demand heap analysis queries using
// pre-computed compact index data (CSR format). It replaces the
// need for full pre-computation of GC root paths, retainers, etc.
type HeapQueryEngine struct {
	graph *hprof.IndexedReferenceGraph

	// Lazy-computed lookup tables
	classNameToID     map[string]uint64
	classNameToIDOnce sync.Once
}

// NewHeapQueryEngine creates a new HeapQueryEngine from an IndexedReferenceGraph.
func NewHeapQueryEngine(graph *hprof.IndexedReferenceGraph) *HeapQueryEngine {
	return &HeapQueryEngine{
		graph: graph,
	}
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
	ClassName     string `json:"class_name"`
	RootType      string `json:"root_type"`
	InstanceCount int    `json:"instance_count"`
	TotalShallow  int64  `json:"total_shallow"`
	TotalRetained int64  `json:"total_retained"`
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

		var sortValue int64
		switch sortBy {
		case "shallow":
			sortValue = e.graph.GetShallowSize(i)
		default: // "retained" or empty
			sortValue = e.graph.GetRetainedSize(i)
		}

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
		classID := e.graph.GetClassID(entry.idx)
		results[i] = BiggestObjectResult{
			ObjectID:     fmt.Sprintf("0x%x", e.graph.GetObjectID(entry.idx)),
			ClassName:    e.graph.GetClassName(classID),
			ShallowSize:  e.graph.GetShallowSize(entry.idx),
			RetainedSize: e.graph.GetRetainedSize(entry.idx),
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

			fieldName := ""
			if e.graph.GetOutgoing() != nil && fieldIDs != nil {
				fieldName = e.graph.GetOutgoing().GetFieldName(fieldIDs[i])
			}

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
		classID := e.graph.GetClassID(current)
		node := GCRootPathNodeResult{
			ObjectID:  fmt.Sprintf("0x%x", e.graph.GetObjectID(current)),
			ClassName: e.graph.GetClassName(classID),
			Size:      e.graph.GetRetainedSize(current),
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
	classID := e.graph.GetClassID(startIdx)
	pathNodes = append(pathNodes, GCRootPathNodeResult{
		ObjectID:  fmt.Sprintf("0x%x", e.graph.GetObjectID(startIdx)),
		ClassName: e.graph.GetClassName(classID),
		Size:      e.graph.GetRetainedSize(startIdx),
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
		sourceIdx := sources[i]
		classID := e.graph.GetClassID(sourceIdx)

		fieldName := ""
		if e.graph.GetOutgoing() != nil && fieldIDs != nil {
			fieldName = e.graph.GetOutgoing().GetFieldName(fieldIDs[i])
		}

		results = append(results, RetainerResult{
			ObjectID:     fmt.Sprintf("0x%x", e.graph.GetObjectID(sourceIdx)),
			ClassName:    e.graph.GetClassName(classID),
			FieldName:    fieldName,
			ShallowSize:  e.graph.GetShallowSize(sourceIdx),
			RetainedSize: e.graph.GetRetainedSize(sourceIdx),
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

		classID := e.graph.GetClassID(targetIdx)
		className := e.graph.GetClassName(classID)

		fieldName := ""
		if e.graph.GetOutgoing() != nil && fieldIDs != nil {
			fieldName = e.graph.GetOutgoing().GetFieldName(fieldIDs[i])
		}

		// Check if target has outgoing edges (children)
		_, tgtFieldIDs, _ := e.graph.GetOutgoingEdges(targetIdx)
		hasChildren := len(tgtFieldIDs) > 0

		results = append(results, ObjectFieldResult{
			Name:         fieldName,
			Type:         className,
			RefID:        fmt.Sprintf("0x%x", e.graph.GetObjectID(targetIdx)),
			RefClass:     className,
			ShallowSize:  e.graph.GetShallowSize(targetIdx),
			RetainedSize: e.graph.GetRetainedSize(targetIdx),
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
		var sv int64
		switch sortBy {
		case "shallow":
			sv = e.graph.GetShallowSize(idx)
		default:
			sv = e.graph.GetRetainedSize(idx)
		}
		sorted = append(sorted, indexedObj{idx: idx, sortValue: sv})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].sortValue > sorted[j].sortValue
	})

	if len(sorted) > topN {
		sorted = sorted[:topN]
	}

	results := make([]BiggestObjectResult, len(sorted))
	for i, obj := range sorted {
		results[i] = BiggestObjectResult{
			ObjectID:     fmt.Sprintf("0x%x", e.graph.GetObjectID(obj.idx)),
			ClassName:    className,
			ShallowSize:  e.graph.GetShallowSize(obj.idx),
			RetainedSize: e.graph.GetRetainedSize(obj.idx),
		}
	}

	return results
}

// QueryGCRootsSummary returns GC roots grouped by class and type.
func (e *HeapQueryEngine) QueryGCRootsSummary() []GCRootSummaryResult {
	// Group GC roots by class
	type key struct {
		className string
		rootType  string
	}
	groups := make(map[key]*GCRootSummaryResult)

	objectCount := e.graph.ObjectCount()
	for i := int32(0); i < objectCount; i++ {
		if !e.graph.IsGCRoot(i) {
			continue
		}
		classID := e.graph.GetClassID(i)
		className := e.graph.GetClassName(classID)
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
	}

	// Convert to slice and sort by total retained descending
	results := make([]GCRootSummaryResult, 0, len(groups))
	for _, r := range groups {
		results = append(results, *r)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalRetained > results[j].TotalRetained
	})

	return results
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
