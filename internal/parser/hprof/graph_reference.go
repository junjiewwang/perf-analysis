// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"sync"

	"github.com/perf-analysis/pkg/utils"
)

// ReferenceGraph holds the object reference graph with GC root tracking.
// This is the core data structure for heap analysis, containing:
// - Object references (incoming and outgoing)
// - Object metadata (class, size)
// - GC roots
// - Dominator tree data
// - Retained size calculations
type ReferenceGraph struct {
	// incomingRefs maps objectID -> list of objects that reference it
	incomingRefs map[uint64][]ObjectReference
	// outgoingRefs maps objectID -> list of objects it references
	outgoingRefs map[uint64][]ObjectReference
	// objectClass maps objectID -> classID
	objectClass map[uint64]uint64
	// objectSize maps objectID -> size
	objectSize map[uint64]int64
	// classNames maps classID -> className
	classNames map[uint64]string
	// gcRoots holds all GC roots
	gcRoots []*GCRoot
	// gcRootSet for fast lookup
	gcRootSet map[uint64]GCRootType
	// classObjectIDs tracks all Class object IDs (from CLASS_DUMP)
	classObjectIDs map[uint64]bool
	// dominators maps objectID -> immediate dominator objectID
	dominators map[uint64]uint64
	// retainedSizes maps objectID -> retained size (computed via dominator tree, standard calculation)
	retainedSizes map[uint64]int64
	// computedRetainedSizes maps objectID -> retained size computed by the active strategy
	computedRetainedSizes map[uint64]int64
	// classRetainedSizes maps classID -> total retained size for all instances (MAT top-level style)
	classRetainedSizes map[uint64]int64
	// classRetainedSizesAttributed maps classID -> attributed retained size (non-overlapping, by nearest dominator class)
	classRetainedSizesAttributed map[uint64]int64
	// dominatorComputed indicates if dominator tree has been computed
	dominatorComputed bool
	// reachableObjects tracks objects reachable from GC roots (populated during dominator computation)
	reachableObjects map[uint64]bool
	// classToObjects maps classID -> list of objectIDs (lazy built for optimization)
	classToObjects map[uint64][]uint64
	// classToObjectsBuilt indicates if classToObjects index has been built
	classToObjectsBuilt bool
	// classToObjectsOnce ensures classToObjects is built only once (thread-safe)
	classToObjectsOnce sync.Once
	// logger is used for debug logging. If nil, debug logs are suppressed.
	logger utils.Logger

	// Retained size calculation strategy (pluggable)
	retainedSizeCalculatorRegistry *RetainedSizeCalculatorRegistry
	activeRetainedSizeStrategy     RetainedSizeStrategy

	// Field name interning for optimized map key operations
	// fieldNameToID maps field name string -> interned ID (uint32)
	fieldNameToID map[string]uint32
	// fieldNames maps interned ID -> field name string
	fieldNames []string
	// fieldNameMu protects field name interning (for concurrent access)
	fieldNameMu sync.RWMutex
	// fieldNamesBuilt indicates if field name index has been built
	fieldNamesBuilt bool

	// classNameToID maps className -> classID for reverse lookup (lazy built)
	classNameToID map[string]uint64
	// classNameToIDBuilt indicates if classNameToID index has been built
	classNameToIDBuilt bool
	// classNameToIDOnce ensures classNameToID is built only once
	classNameToIDOnce sync.Once

	// Object ID indexing for Bitset-based visited tracking (O(1) reset)
	// objectIDToIndex maps objectID -> compact index (for Bitset operations)
	// Note: We use int (not uint64) as index because:
	// 1. VersionedBitset uses int for indexing
	// 2. int on 64-bit systems can hold ~9.2 × 10^18 values, far exceeding any realistic heap object count
	// 3. The index is sequentially assigned (0, 1, 2, ...), not converted from objectID
	objectIDToIndex map[uint64]int
	// indexToObjectID maps compact index -> objectID
	indexToObjectID []uint64
	// objectIndexBuilt indicates if object index has been built
	objectIndexBuilt bool
	// objectIndexOnce ensures object index is built only once
	objectIndexOnce sync.Once

	// Index-based incoming references for optimized BFS traversal
	// This eliminates GetObjectIndex map lookups during BFS (saves ~20% CPU)
	// indexedIncomingRefs maps object index -> list of indexed references
	indexedIncomingRefs [][]IndexedReference
	// indexedRefsBuilt indicates if indexed refs have been built
	indexedRefsBuilt bool
	// indexedRefsOnce ensures indexed refs are built only once
	indexedRefsOnce sync.Once
}

// IndexedReference represents a reference using compact indices instead of object IDs.
// This eliminates map lookups during BFS traversal.
type IndexedReference struct {
	FromIndex   int    // Compact index of the source object
	ClassID     uint64 // Class ID of the source object
	FieldNameID uint32 // Interned field name ID (0 = empty)
}

// ObjectReference represents a reference from one object to another.
type ObjectReference struct {
	FromObjectID uint64
	ToObjectID   uint64
	FieldName    string
	FromClassID  uint64
}

// SetLogger sets the logger for debug output.
func (g *ReferenceGraph) SetLogger(logger utils.Logger) {
	g.logger = logger
}

// debugf logs a debug message if logger is configured.
func (g *ReferenceGraph) debugf(format string, args ...interface{}) {
	if g.logger != nil {
		g.logger.Debug(format, args...)
	}
}

// NewReferenceGraph creates a new reference graph.
func NewReferenceGraph() *ReferenceGraph {
	return NewReferenceGraphWithCapacity(0)
}

// NewReferenceGraphWithCapacity creates a new reference graph with pre-allocated capacity.
// estimatedObjects is the expected number of objects (use 0 for default sizing).
func NewReferenceGraphWithCapacity(estimatedObjects int) *ReferenceGraph {
	if estimatedObjects <= 0 {
		estimatedObjects = 100000 // Default capacity
	}

	// Estimate: average 3 references per object
	estimatedRefs := estimatedObjects
	estimatedClasses := estimatedObjects / 100 // Rough estimate: 1 class per 100 objects
	if estimatedClasses < 1000 {
		estimatedClasses = 1000
	}

	return &ReferenceGraph{
		incomingRefs:                   make(map[uint64][]ObjectReference, estimatedRefs),
		outgoingRefs:                   make(map[uint64][]ObjectReference, estimatedRefs),
		objectClass:                    make(map[uint64]uint64, estimatedObjects),
		objectSize:                     make(map[uint64]int64, estimatedObjects),
		classNames:                     make(map[uint64]string, estimatedClasses),
		gcRoots:                        make([]*GCRoot, 0, 10000),
		gcRootSet:                      make(map[uint64]GCRootType, 10000),
		classObjectIDs:                 make(map[uint64]bool, estimatedClasses),
		dominators:                     make(map[uint64]uint64, estimatedObjects),
		retainedSizes:                  make(map[uint64]int64, estimatedObjects),
		computedRetainedSizes:          make(map[uint64]int64, estimatedObjects),
		classRetainedSizes:             make(map[uint64]int64, estimatedClasses),
		classRetainedSizesAttributed:   make(map[uint64]int64, estimatedClasses),
		reachableObjects:               make(map[uint64]bool, estimatedObjects),
		retainedSizeCalculatorRegistry: NewRetainedSizeCalculatorRegistry(),
		activeRetainedSizeStrategy:     RetainedSizeStrategyIDEA, // Default to IDEA style
		// Field name interning initialization
		fieldNameToID: make(map[string]uint32, 10000),
		fieldNames:    make([]string, 1, 10000), // Index 0 reserved for empty string
	}
}

// AddReference adds a reference to the graph.
func (g *ReferenceGraph) AddReference(ref ObjectReference) {
	g.incomingRefs[ref.ToObjectID] = append(g.incomingRefs[ref.ToObjectID], ref)
	g.outgoingRefs[ref.FromObjectID] = append(g.outgoingRefs[ref.FromObjectID], ObjectReference{
		FromObjectID: ref.FromObjectID,
		ToObjectID:   ref.ToObjectID,
		FieldName:    ref.FieldName,
		FromClassID:  ref.FromClassID,
	})
}

// AddReferences adds multiple references in batch for better performance.
// This reduces map lookup overhead when adding many references from the same object.
func (g *ReferenceGraph) AddReferences(refs []ObjectReference) {
	for i := range refs {
		ref := &refs[i]
		g.incomingRefs[ref.ToObjectID] = append(g.incomingRefs[ref.ToObjectID], *ref)
		g.outgoingRefs[ref.FromObjectID] = append(g.outgoingRefs[ref.FromObjectID], ObjectReference{
			FromObjectID: ref.FromObjectID,
			ToObjectID:   ref.ToObjectID,
			FieldName:    ref.FieldName,
			FromClassID:  ref.FromClassID,
		})
	}
}

// GetStats returns statistics about the reference graph.
func (g *ReferenceGraph) GetStats() (objects int, refs int, gcRoots int, objectsWithIncoming int) {
	objects = len(g.objectClass)
	gcRoots = len(g.gcRoots)

	totalRefs := 0
	for _, refs := range g.incomingRefs {
		totalRefs += len(refs)
	}
	refs = totalRefs
	objectsWithIncoming = len(g.incomingRefs)
	return
}

// SetObjectInfo sets object class and size.
func (g *ReferenceGraph) SetObjectInfo(objectID, classID uint64, size int64) {
	g.objectClass[objectID] = classID
	g.objectSize[objectID] = size
}

// RegisterClassObject registers a Class object ID.
// Class objects are treated as implicit GC roots since they are held by ClassLoaders.
func (g *ReferenceGraph) RegisterClassObject(classID uint64) {
	g.classObjectIDs[classID] = true
}

// FixClassObjectClassIDs updates all Class objects to have java.lang.Class as their classID.
// Returns the number of Class objects fixed.
func (g *ReferenceGraph) FixClassObjectClassIDs(javaLangClassID uint64) int {
	count := 0
	for classObjID := range g.classObjectIDs {
		// Only fix if the classID is currently set to itself (the temporary value)
		if currentClassID, exists := g.objectClass[classObjID]; exists && currentClassID == classObjID {
			g.objectClass[classObjID] = javaLangClassID
			count++
		}
	}
	return count
}

// SetClassName sets the class name for a class ID.
func (g *ReferenceGraph) SetClassName(classID uint64, name string) {
	g.classNames[classID] = name
}

// GetClassName returns the class name for a class ID.
func (g *ReferenceGraph) GetClassName(classID uint64) string {
	return g.classNames[classID]
}

// GetIncomingRefs returns the incoming references for an object (who references this object).
func (g *ReferenceGraph) GetIncomingRefs(objectID uint64) []ObjectReference {
	return g.incomingRefs[objectID]
}

// GetOutgoingRefs returns the outgoing references for an object (what this object references).
func (g *ReferenceGraph) GetOutgoingRefs(objectID uint64) []ObjectReference {
	return g.outgoingRefs[objectID]
}

// GetObjectSize returns the shallow size of an object.
func (g *ReferenceGraph) GetObjectSize(objectID uint64) int64 {
	return g.objectSize[objectID]
}

// GetObjectClassID returns the class ID of an object.
func (g *ReferenceGraph) GetObjectClassID(objectID uint64) (uint64, bool) {
	classID, ok := g.objectClass[objectID]
	return classID, ok
}

// buildClassToObjectsIndex builds the classID -> []objectID index for fast lookup.
// This is called lazily when needed and cached for subsequent calls.
// Thread-safe: uses sync.Once to ensure index is built only once.
func (g *ReferenceGraph) buildClassToObjectsIndex() {
	g.classToObjectsOnce.Do(func() {
		// Pre-allocate with estimated size
		g.classToObjects = make(map[uint64][]uint64, len(g.classNames))

		// Count objects per class first for pre-allocation
		classCounts := make(map[uint64]int, len(g.classNames))
		for _, classID := range g.objectClass {
			classCounts[classID]++
		}

		// Pre-allocate slices
		for classID, count := range classCounts {
			g.classToObjects[classID] = make([]uint64, 0, count)
		}

		// Populate index
		for objID, classID := range g.objectClass {
			g.classToObjects[classID] = append(g.classToObjects[classID], objID)
		}

		g.classToObjectsBuilt = true
	})
}

// getObjectsByClass returns all objects of a given class using the cached index.
func (g *ReferenceGraph) getObjectsByClass(classID uint64) []uint64 {
	g.buildClassToObjectsIndex()
	return g.classToObjects[classID]
}

// getClassIDByName returns the classID for a given class name.
func (g *ReferenceGraph) getClassIDByName(className string) (uint64, bool) {
	g.buildClassNameToIDIndex()
	classID, ok := g.classNameToID[className]
	return classID, ok
}

// buildClassNameToIDIndex builds the className -> classID index for fast lookup.
// Thread-safe: uses sync.Once to ensure index is built only once.
func (g *ReferenceGraph) buildClassNameToIDIndex() {
	g.classNameToIDOnce.Do(func() {
		g.classNameToID = make(map[string]uint64, len(g.classNames))
		for classID, name := range g.classNames {
			g.classNameToID[name] = classID
		}
		g.classNameToIDBuilt = true
	})
}

// buildObjectIndex builds the objectID <-> index mapping for Bitset-based visited tracking.
// This enables O(1) reset for visited tracking instead of O(V) map clearing.
// Thread-safe: uses sync.Once to ensure index is built only once.
func (g *ReferenceGraph) buildObjectIndex() {
	g.objectIndexOnce.Do(func() {
		objectCount := len(g.objectClass)
		g.objectIDToIndex = make(map[uint64]int, objectCount)
		g.indexToObjectID = make([]uint64, 0, objectCount)

		// Assign sequential indices to all objects
		idx := 0
		for objID := range g.objectClass {
			g.objectIDToIndex[objID] = idx
			g.indexToObjectID = append(g.indexToObjectID, objID)
			idx++
		}
		g.objectIndexBuilt = true
	})
}

// GetObjectIndex returns the compact index for an objectID.
// Returns -1 if the objectID is not found.
// Thread-safe after buildObjectIndex is called.
func (g *ReferenceGraph) GetObjectIndex(objID uint64) int {
	if !g.objectIndexBuilt {
		g.buildObjectIndex()
	}
	if idx, ok := g.objectIDToIndex[objID]; ok {
		return idx
	}
	return -1
}

// GetObjectIDByIndex returns the objectID for a compact index.
// Returns 0 if the index is out of range.
func (g *ReferenceGraph) GetObjectIDByIndex(idx int) uint64 {
	if !g.objectIndexBuilt {
		g.buildObjectIndex()
	}
	if idx < 0 || idx >= len(g.indexToObjectID) {
		return 0
	}
	return g.indexToObjectID[idx]
}

// GetObjectCount returns the total number of objects in the graph.
func (g *ReferenceGraph) GetObjectCount() int {
	return len(g.objectClass)
}

// buildIndexedIncomingRefs builds the index-based incoming references structure.
// This pre-computes all the index lookups and field name interning to eliminate
// map lookups during BFS traversal.
// Thread-safe: uses sync.Once to ensure it's built only once.
func (g *ReferenceGraph) buildIndexedIncomingRefs() {
	g.indexedRefsOnce.Do(func() {
		// Ensure object index is built first
		g.buildObjectIndex()
		// Ensure field names are interned
		g.BuildFieldNameIndex()

		objectCount := len(g.indexToObjectID)
		g.indexedIncomingRefs = make([][]IndexedReference, objectCount)

		// Convert each object's incoming refs to indexed format
		for objID, refs := range g.incomingRefs {
			toIdx, ok := g.objectIDToIndex[objID]
			if !ok {
				continue
			}

			if len(refs) == 0 {
				continue
			}

			indexedRefs := make([]IndexedReference, 0, len(refs))
			for _, ref := range refs {
				fromIdx, ok := g.objectIDToIndex[ref.FromObjectID]
				if !ok {
					continue
				}

				// Pre-intern field name (already done in BuildFieldNameIndex, just lookup)
				var fieldNameID uint32
				if ref.FieldName != "" {
					if id, exists := g.fieldNameToID[ref.FieldName]; exists {
						fieldNameID = id
					}
				}

				indexedRefs = append(indexedRefs, IndexedReference{
					FromIndex:   fromIdx,
					ClassID:     ref.FromClassID,
					FieldNameID: fieldNameID,
				})
			}

			g.indexedIncomingRefs[toIdx] = indexedRefs
		}

		g.indexedRefsBuilt = true
	})
}

// GetIndexedIncomingRefs returns the indexed incoming references for an object.
// This is optimized for BFS traversal - no map lookups needed.
// Must call buildIndexedIncomingRefs first.
func (g *ReferenceGraph) GetIndexedIncomingRefs(objIdx int) []IndexedReference {
	if !g.indexedRefsBuilt {
		g.buildIndexedIncomingRefs()
	}
	if objIdx < 0 || objIdx >= len(g.indexedIncomingRefs) {
		return nil
	}
	return g.indexedIncomingRefs[objIdx]
}

// GetObjectSizeByIndex returns the object size by index.
// This avoids objectID -> size map lookup.
func (g *ReferenceGraph) GetObjectSizeByIndex(idx int) int64 {
	if idx < 0 || idx >= len(g.indexToObjectID) {
		return 0
	}
	return g.objectSize[g.indexToObjectID[idx]]
}

// InternFieldName returns the interned ID for a field name.
// Thread-safe for concurrent access during analysis.
func (g *ReferenceGraph) InternFieldName(name string) uint32 {
	if name == "" {
		return 0 // Index 0 is reserved for empty string
	}

	// Fast path: read lock for existing names
	g.fieldNameMu.RLock()
	if id, ok := g.fieldNameToID[name]; ok {
		g.fieldNameMu.RUnlock()
		return id
	}
	g.fieldNameMu.RUnlock()

	// Slow path: write lock for new names
	g.fieldNameMu.Lock()
	defer g.fieldNameMu.Unlock()

	// Double-check after acquiring write lock
	if id, ok := g.fieldNameToID[name]; ok {
		return id
	}

	id := uint32(len(g.fieldNames))
	g.fieldNames = append(g.fieldNames, name)
	g.fieldNameToID[name] = id
	return id
}

// GetFieldNameByID returns the field name for an interned ID.
func (g *ReferenceGraph) GetFieldNameByID(id uint32) string {
	g.fieldNameMu.RLock()
	defer g.fieldNameMu.RUnlock()

	if int(id) >= len(g.fieldNames) {
		return ""
	}
	return g.fieldNames[id]
}

// BuildFieldNameIndex builds the field name index from all references.
// This should be called once after parsing is complete for optimal performance.
func (g *ReferenceGraph) BuildFieldNameIndex() {
	g.fieldNameMu.Lock()
	defer g.fieldNameMu.Unlock()

	if g.fieldNamesBuilt {
		return
	}

	// Collect all unique field names from references
	for _, refs := range g.incomingRefs {
		for _, ref := range refs {
			if ref.FieldName != "" {
				if _, ok := g.fieldNameToID[ref.FieldName]; !ok {
					id := uint32(len(g.fieldNames))
					g.fieldNames = append(g.fieldNames, ref.FieldName)
					g.fieldNameToID[ref.FieldName] = id
				}
			}
		}
	}

	g.fieldNamesBuilt = true
}

// IsObjectReachable returns true if the object is reachable from GC roots.
// This is determined during dominator tree computation.
func (g *ReferenceGraph) IsObjectReachable(objectID uint64) bool {
	if !g.dominatorComputed {
		return true // Assume reachable if not computed yet
	}
	return g.reachableObjects[objectID]
}

// GetReachableObjectCount returns the count of reachable objects.
func (g *ReferenceGraph) GetReachableObjectCount() int {
	if !g.dominatorComputed {
		g.ComputeDominatorTree()
	}
	return len(g.reachableObjects)
}

// GetTotalReachableHeapSize returns the total heap size of reachable objects.
func (g *ReferenceGraph) GetTotalReachableHeapSize() int64 {
	if !g.dominatorComputed {
		g.ComputeDominatorTree()
	}

	var total int64
	for objID := range g.reachableObjects {
		total += g.objectSize[objID]
	}
	return total
}

// GetReachableClassStats returns class statistics for only reachable objects.
// This matches MAT's behavior of only counting live objects.
func (g *ReferenceGraph) GetReachableClassStats() map[uint64]struct {
	InstanceCount int64
	TotalSize     int64
} {
	if !g.dominatorComputed {
		g.ComputeDominatorTree()
	}

	stats := make(map[uint64]struct {
		InstanceCount int64
		TotalSize     int64
	})

	for objID, classID := range g.objectClass {
		if g.reachableObjects[objID] {
			s := stats[classID]
			s.InstanceCount++
			s.TotalSize += g.objectSize[objID]
			stats[classID] = s
		}
	}

	return stats
}

// GetAllClassStats returns class statistics for ALL objects (including unreachable).
// This matches IDEA's behavior of showing all objects in the heap.
func (g *ReferenceGraph) GetAllClassStats() map[uint64]struct {
	InstanceCount int64
	TotalSize     int64
} {
	stats := make(map[uint64]struct {
		InstanceCount int64
		TotalSize     int64
	})

	for objID, classID := range g.objectClass {
		s := stats[classID]
		s.InstanceCount++
		s.TotalSize += g.objectSize[objID]
		stats[classID] = s
	}

	return stats
}

// formatObjectID formats an object ID as a hex string.
func formatObjectID(id uint64) string {
	return "0x" + formatHex(id)
}

// formatHex formats a uint64 as a hex string without leading zeros.
func formatHex(n uint64) string {
	const hexDigits = "0123456789abcdef"
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = hexDigits[n&0xf]
		n >>= 4
	}
	return string(buf[i:])
}
