// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"sort"
	"sync"
)

// ============================================================================
// IndexedObjectStore - High-performance object storage using slice-based indexing
// ============================================================================

// IndexedObjectStore provides O(1) access to object data using continuous indices.
type IndexedObjectStore struct {
	objToIdx      map[uint64]int32
	idxToObj      []uint64
	classIDs      []uint64
	shallowSizes  []int64
	retainedSizes []int64
	dominators    []int32
	count         int32
	capacity      int32
	mu            sync.RWMutex
	finalized     bool
}

// NewIndexedObjectStore creates a new indexed object store with estimated capacity.
func NewIndexedObjectStore(estimatedObjects int) *IndexedObjectStore {
	if estimatedObjects <= 0 {
		estimatedObjects = 100000
	}
	cap := int32(estimatedObjects)
	return &IndexedObjectStore{
		objToIdx:      make(map[uint64]int32, estimatedObjects),
		idxToObj:      make([]uint64, 0, cap),
		classIDs:      make([]uint64, 0, cap),
		shallowSizes:  make([]int64, 0, cap),
		retainedSizes: make([]int64, 0, cap),
		dominators:    make([]int32, 0, cap),
		capacity:      cap,
	}
}

// AddObject adds an object to the store and returns its index.
func (s *IndexedObjectStore) AddObject(objectID uint64, classID uint64, size int64) int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finalized {
		if idx, ok := s.objToIdx[objectID]; ok {
			return idx
		}
		return -1
	}
	if idx, ok := s.objToIdx[objectID]; ok {
		return idx
	}
	idx := s.count
	s.objToIdx[objectID] = idx
	s.idxToObj = append(s.idxToObj, objectID)
	s.classIDs = append(s.classIDs, classID)
	s.shallowSizes = append(s.shallowSizes, size)
	s.retainedSizes = append(s.retainedSizes, size)
	s.dominators = append(s.dominators, -1)
	s.count++
	return idx
}

// Finalize finalizes the store, preventing further additions.
func (s *IndexedObjectStore) Finalize() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finalized = true
}

// GetIndex returns the index for an object ID, or -1 if not found.
func (s *IndexedObjectStore) GetIndex(objectID uint64) int32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if idx, ok := s.objToIdx[objectID]; ok {
		return idx
	}
	return -1
}

// GetObjectID returns the object ID for an index.
func (s *IndexedObjectStore) GetObjectID(idx int32) uint64 {
	if idx < 0 || idx >= s.count {
		return 0
	}
	return s.idxToObj[idx]
}

// GetClassID returns the class ID for an object index.
func (s *IndexedObjectStore) GetClassID(idx int32) uint64 {
	if idx < 0 || idx >= s.count {
		return 0
	}
	return s.classIDs[idx]
}

// GetShallowSize returns the shallow size for an object index.
func (s *IndexedObjectStore) GetShallowSize(idx int32) int64 {
	if idx < 0 || idx >= s.count {
		return 0
	}
	return s.shallowSizes[idx]
}

// GetRetainedSize returns the retained size for an object index.
func (s *IndexedObjectStore) GetRetainedSize(idx int32) int64 {
	if idx < 0 || idx >= s.count {
		return 0
	}
	return s.retainedSizes[idx]
}

// SetRetainedSize sets the retained size for an object index.
func (s *IndexedObjectStore) SetRetainedSize(idx int32, size int64) {
	if idx >= 0 && idx < s.count {
		s.retainedSizes[idx] = size
	}
}

// AddRetainedSize adds to the retained size for an object index.
func (s *IndexedObjectStore) AddRetainedSize(idx int32, delta int64) {
	if idx >= 0 && idx < s.count {
		s.retainedSizes[idx] += delta
	}
}

// GetDominator returns the dominator index for an object index.
func (s *IndexedObjectStore) GetDominator(idx int32) int32 {
	if idx < 0 || idx >= s.count {
		return -1
	}
	return s.dominators[idx]
}

// SetDominator sets the dominator index for an object index.
func (s *IndexedObjectStore) SetDominator(idx int32, domIdx int32) {
	if idx >= 0 && idx < s.count {
		s.dominators[idx] = domIdx
	}
}

// Count returns the number of objects in the store.
func (s *IndexedObjectStore) Count() int32 {
	return s.count
}

// IterateObjects iterates over all objects, calling fn for each.
func (s *IndexedObjectStore) IterateObjects(fn func(idx int32, objectID, classID uint64, size int64) bool) {
	for i := int32(0); i < s.count; i++ {
		if !fn(i, s.idxToObj[i], s.classIDs[i], s.shallowSizes[i]) {
			break
		}
	}
}

// GetObjectIDsByClass returns all object indices for a given class ID.
func (s *IndexedObjectStore) GetObjectIDsByClass(classID uint64) []int32 {
	result := make([]int32, 0, 100)
	for i := int32(0); i < s.count; i++ {
		if s.classIDs[i] == classID {
			result = append(result, i)
		}
	}
	return result
}

// ============================================================================
// CompactEdgeList - Memory-efficient edge storage
// ============================================================================

// CompactEdgeList stores edges in a compact format using CSR (Compressed Sparse Row).
type CompactEdgeList struct {
	offsets    []int32
	targets    []int32
	fieldIDs   []int32
	classIDs   []uint64
	fieldNames []string
	fieldToID  map[string]int32
	nodeCount  int32
	edgeCount  int32
}

// NewCompactEdgeList creates a new compact edge list.
func NewCompactEdgeList(nodeCount int, estimatedEdges int) *CompactEdgeList {
	return &CompactEdgeList{
		offsets:    make([]int32, nodeCount+1),
		targets:    make([]int32, 0, estimatedEdges),
		fieldIDs:   make([]int32, 0, estimatedEdges),
		classIDs:   make([]uint64, 0, estimatedEdges),
		fieldNames: make([]string, 0, 1000),
		fieldToID:  make(map[string]int32, 1000),
		nodeCount:  int32(nodeCount),
	}
}

func (e *CompactEdgeList) internFieldName(name string) int32 {
	if id, ok := e.fieldToID[name]; ok {
		return id
	}
	id := int32(len(e.fieldNames))
	e.fieldNames = append(e.fieldNames, name)
	e.fieldToID[name] = id
	return id
}

// GetFieldName returns the field name for a field ID.
func (e *CompactEdgeList) GetFieldName(fieldID int32) string {
	if fieldID < 0 || int(fieldID) >= len(e.fieldNames) {
		return ""
	}
	return e.fieldNames[fieldID]
}

// CompactEdgeListBuilder helps build a CompactEdgeList efficiently.
type CompactEdgeListBuilder struct {
	nodeCount  int32
	edges      []struct {
		from    int32
		to      int32
		fieldID int32
		classID uint64
	}
	fieldNames []string
	fieldToID  map[string]int32
}

// NewCompactEdgeListBuilder creates a new builder.
func NewCompactEdgeListBuilder(nodeCount int, estimatedEdges int) *CompactEdgeListBuilder {
	return &CompactEdgeListBuilder{
		nodeCount: int32(nodeCount),
		edges: make([]struct {
			from    int32
			to      int32
			fieldID int32
			classID uint64
		}, 0, estimatedEdges),
		fieldNames: make([]string, 0, 1000),
		fieldToID:  make(map[string]int32, 1000),
	}
}

// AddEdge adds an edge to the builder.
func (b *CompactEdgeListBuilder) AddEdge(from, to int32, fieldName string, classID uint64) {
	fieldID := b.internFieldName(fieldName)
	b.edges = append(b.edges, struct {
		from    int32
		to      int32
		fieldID int32
		classID uint64
	}{from, to, fieldID, classID})
}

func (b *CompactEdgeListBuilder) internFieldName(name string) int32 {
	if id, ok := b.fieldToID[name]; ok {
		return id
	}
	id := int32(len(b.fieldNames))
	b.fieldNames = append(b.fieldNames, name)
	b.fieldToID[name] = id
	return id
}

// Build creates the CompactEdgeList from the builder.
func (b *CompactEdgeListBuilder) Build() *CompactEdgeList {
	sort.Slice(b.edges, func(i, j int) bool {
		return b.edges[i].from < b.edges[j].from
	})
	result := &CompactEdgeList{
		offsets:    make([]int32, b.nodeCount+1),
		targets:    make([]int32, len(b.edges)),
		fieldIDs:   make([]int32, len(b.edges)),
		classIDs:   make([]uint64, len(b.edges)),
		fieldNames: b.fieldNames,
		fieldToID:  b.fieldToID,
		nodeCount:  b.nodeCount,
		edgeCount:  int32(len(b.edges)),
	}
	for i, edge := range b.edges {
		result.targets[i] = edge.to
		result.fieldIDs[i] = edge.fieldID
		result.classIDs[i] = edge.classID
		result.offsets[edge.from+1]++
	}
	for i := int32(1); i <= b.nodeCount; i++ {
		result.offsets[i] += result.offsets[i-1]
	}
	return result
}

// GetEdges returns all edges from a node.
func (e *CompactEdgeList) GetEdges(nodeIdx int32) (targets []int32, fieldIDs []int32, classIDs []uint64) {
	if nodeIdx < 0 || nodeIdx >= e.nodeCount {
		return nil, nil, nil
	}
	start := e.offsets[nodeIdx]
	end := e.offsets[nodeIdx+1]
	return e.targets[start:end], e.fieldIDs[start:end], e.classIDs[start:end]
}

// GetEdgeCount returns the number of edges from a node.
func (e *CompactEdgeList) GetEdgeCount(nodeIdx int32) int32 {
	if nodeIdx < 0 || nodeIdx >= e.nodeCount {
		return 0
	}
	return e.offsets[nodeIdx+1] - e.offsets[nodeIdx]
}

// TotalEdges returns the total number of edges.
func (e *CompactEdgeList) TotalEdges() int32 {
	return e.edgeCount
}

// ============================================================================
// IndexedReferenceGraph - Optimized reference graph using indexed storage
// ============================================================================

// IndexedReferenceGraph is an optimized version of ReferenceGraph using
// slice-based storage instead of maps for better memory efficiency and cache locality.
type IndexedReferenceGraph struct {
	objects         *IndexedObjectStore
	outgoing        *CompactEdgeList
	incoming        *CompactEdgeList
	classNames      map[uint64]string
	gcRoots         []GCRoot
	gcRootBits      *Bitset
	classObjectBits *Bitset
	reachableBits   *Bitset
	classToObjects     map[uint64][]int32
	classToObjectsOnce sync.Once
	dominatorComputed  bool
}

// NewIndexedReferenceGraph creates a new indexed reference graph.
func NewIndexedReferenceGraph(estimatedObjects int) *IndexedReferenceGraph {
	return &IndexedReferenceGraph{
		objects:    NewIndexedObjectStore(estimatedObjects),
		classNames: make(map[uint64]string, estimatedObjects/100),
	}
}

// AddObject adds an object to the graph.
func (g *IndexedReferenceGraph) AddObject(objectID uint64, classID uint64, size int64) int32 {
	return g.objects.AddObject(objectID, classID, size)
}

// SetClassName sets the name for a class ID.
func (g *IndexedReferenceGraph) SetClassName(classID uint64, name string) {
	g.classNames[classID] = name
}

// GetClassName returns the name for a class ID.
func (g *IndexedReferenceGraph) GetClassName(classID uint64) string {
	return g.classNames[classID]
}

// AddGCRoot adds a GC root.
func (g *IndexedReferenceGraph) AddGCRoot(root GCRoot) {
	g.gcRoots = append(g.gcRoots, root)
}

// MarkClassObject marks an object as a Class instance.
func (g *IndexedReferenceGraph) MarkClassObject(objectID uint64) {
	idx := g.objects.GetIndex(objectID)
	if idx >= 0 {
		if g.classObjectBits == nil {
			g.classObjectBits = NewBitset(int(g.objects.Count()))
		}
		g.classObjectBits.Set(int(idx))
	}
}

// IsClassObject returns true if the object is a Class instance.
func (g *IndexedReferenceGraph) IsClassObject(idx int32) bool {
	if g.classObjectBits == nil {
		return false
	}
	return g.classObjectBits.Test(int(idx))
}

// IsGCRoot returns true if the object is a GC root.
func (g *IndexedReferenceGraph) IsGCRoot(idx int32) bool {
	if g.gcRootBits == nil {
		return false
	}
	return g.gcRootBits.Test(int(idx))
}

// IsReachable returns true if the object is reachable from GC roots.
func (g *IndexedReferenceGraph) IsReachable(idx int32) bool {
	if g.reachableBits == nil {
		return false
	}
	return g.reachableBits.Test(int(idx))
}

// FinalizeObjects finalizes object additions and prepares for edge building.
func (g *IndexedReferenceGraph) FinalizeObjects() {
	g.objects.Finalize()
	count := int(g.objects.Count())
	g.gcRootBits = NewBitset(count)
	g.classObjectBits = NewBitset(count)
	g.reachableBits = NewBitset(count)
	for _, root := range g.gcRoots {
		idx := g.objects.GetIndex(root.ObjectID)
		if idx >= 0 {
			g.gcRootBits.Set(int(idx))
		}
	}
}

// BuildEdges builds the edge lists from a builder.
func (g *IndexedReferenceGraph) BuildEdges(outBuilder, inBuilder *CompactEdgeListBuilder) {
	g.outgoing = outBuilder.Build()
	g.incoming = inBuilder.Build()
}

// GetOutgoingEdges returns outgoing edges for an object.
func (g *IndexedReferenceGraph) GetOutgoingEdges(idx int32) (targets []int32, fieldIDs []int32, classIDs []uint64) {
	if g.outgoing == nil {
		return nil, nil, nil
	}
	return g.outgoing.GetEdges(idx)
}

// GetIncomingEdges returns incoming edges for an object.
func (g *IndexedReferenceGraph) GetIncomingEdges(idx int32) (sources []int32, fieldIDs []int32, classIDs []uint64) {
	if g.incoming == nil {
		return nil, nil, nil
	}
	return g.incoming.GetEdges(idx)
}

// ObjectCount returns the number of objects.
func (g *IndexedReferenceGraph) ObjectCount() int32 {
	return g.objects.Count()
}

// GetObjectIndex returns the index for an object ID.
func (g *IndexedReferenceGraph) GetObjectIndex(objectID uint64) int32 {
	return g.objects.GetIndex(objectID)
}

// GetObjectID returns the object ID for an index.
func (g *IndexedReferenceGraph) GetObjectID(idx int32) uint64 {
	return g.objects.GetObjectID(idx)
}

// GetClassID returns the class ID for an object index.
func (g *IndexedReferenceGraph) GetClassID(idx int32) uint64 {
	return g.objects.GetClassID(idx)
}

// GetShallowSize returns the shallow size for an object index.
func (g *IndexedReferenceGraph) GetShallowSize(idx int32) int64 {
	return g.objects.GetShallowSize(idx)
}

// GetRetainedSize returns the retained size for an object index.
func (g *IndexedReferenceGraph) GetRetainedSize(idx int32) int64 {
	return g.objects.GetRetainedSize(idx)
}

// SetRetainedSize sets the retained size for an object index.
func (g *IndexedReferenceGraph) SetRetainedSize(idx int32, size int64) {
	g.objects.SetRetainedSize(idx, size)
}

// GetDominator returns the dominator index for an object index.
func (g *IndexedReferenceGraph) GetDominator(idx int32) int32 {
	return g.objects.GetDominator(idx)
}

// SetDominator sets the dominator index for an object index.
func (g *IndexedReferenceGraph) SetDominator(idx int32, domIdx int32) {
	g.objects.SetDominator(idx, domIdx)
}

// MarkReachable marks an object as reachable from GC roots.
func (g *IndexedReferenceGraph) MarkReachable(idx int32) {
	if g.reachableBits != nil {
		g.reachableBits.Set(int(idx))
	}
}

// BuildClassToObjectsIndex builds the class-to-objects index.
func (g *IndexedReferenceGraph) BuildClassToObjectsIndex() {
	g.classToObjectsOnce.Do(func() {
		g.classToObjects = make(map[uint64][]int32)
		g.objects.IterateObjects(func(idx int32, objectID, classID uint64, size int64) bool {
			g.classToObjects[classID] = append(g.classToObjects[classID], idx)
			return true
		})
	})
}

// GetObjectsByClass returns all object indices for a class.
func (g *IndexedReferenceGraph) GetObjectsByClass(classID uint64) []int32 {
	g.BuildClassToObjectsIndex()
	return g.classToObjects[classID]
}

// GetOutgoing returns the outgoing edge list (CSR format).
func (g *IndexedReferenceGraph) GetOutgoing() *CompactEdgeList {
	return g.outgoing
}

// GetIncoming returns the incoming edge list (CSR format).
func (g *IndexedReferenceGraph) GetIncoming() *CompactEdgeList {
	return g.incoming
}

// GetGCRoots returns the GC root entries.
func (g *IndexedReferenceGraph) GetGCRoots() []GCRoot {
	return g.gcRoots
}

// GetClassNames returns the classID to class name mapping.
func (g *IndexedReferenceGraph) GetClassNames() map[uint64]string {
	return g.classNames
}

// GetObjects returns the underlying IndexedObjectStore.
func (g *IndexedReferenceGraph) GetObjects() *IndexedObjectStore {
	return g.objects
}
