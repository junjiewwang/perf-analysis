// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"sort"
	"strings"
	"sync"

	"github.com/perf-analysis/pkg/utils"
)

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

// RetainerInfo describes what retains a class's instances.
type RetainerInfo struct {
	RetainerClass string  `json:"retainer_class"`
	FieldName     string  `json:"field_name,omitempty"`
	RetainedSize  int64   `json:"retained_size"`
	RetainedCount int64   `json:"retained_count"`
	Percentage    float64 `json:"percentage"`
	Depth         int     `json:"depth,omitempty"` // Distance from target (1 = direct, 2+ = indirect)
}

// ClassRetainers holds retainer information for a class.
type ClassRetainers struct {
	ClassName     string          `json:"class_name"`
	TotalSize     int64           `json:"total_size"`
	InstanceCount int64           `json:"instance_count"`
	Retainers     []*RetainerInfo `json:"retainers"`
	RetainedSize  int64           `json:"retained_size,omitempty"` // Dominator tree retained size
	GCRootPaths   []*GCRootPath   `json:"gc_root_paths,omitempty"` // Sample paths to GC roots
}

// ObjectReference represents a reference from one object to another.
type ObjectReference struct {
	FromObjectID uint64
	ToObjectID   uint64
	FieldName    string
	FromClassID  uint64
}

// ReferenceGraph holds the object reference graph with GC root tracking.
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
	// retainedSizes maps objectID -> retained size (computed via dominator tree)
	retainedSizes map[uint64]int64
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
		incomingRefs:                 make(map[uint64][]ObjectReference, estimatedRefs),
		outgoingRefs:                 make(map[uint64][]ObjectReference, estimatedRefs),
		objectClass:                  make(map[uint64]uint64, estimatedObjects),
		objectSize:                   make(map[uint64]int64, estimatedObjects),
		classNames:                   make(map[uint64]string, estimatedClasses),
		gcRoots:                      make([]*GCRoot, 0, 10000),
		gcRootSet:                    make(map[uint64]GCRootType, 10000),
		classObjectIDs:               make(map[uint64]bool, estimatedClasses),
		dominators:                   make(map[uint64]uint64, estimatedObjects),
		retainedSizes:                make(map[uint64]int64, estimatedObjects),
		classRetainedSizes:           make(map[uint64]int64, estimatedClasses),
		classRetainedSizesAttributed: make(map[uint64]int64, estimatedClasses),
		reachableObjects:             make(map[uint64]bool, estimatedObjects),
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
	for classID, name := range g.classNames {
		if name == className {
			return classID, true
		}
	}
	return 0, false
}

// FindPathsToGCRoot finds paths from an object to GC roots using BFS.
// maxPaths limits the number of paths returned.
// maxDepth limits the search depth.
func (g *ReferenceGraph) FindPathsToGCRoot(objectID uint64, maxPaths, maxDepth int) []*GCRootPath {
	if maxPaths <= 0 {
		maxPaths = 3
	}
	if maxDepth <= 0 {
		maxDepth = 15
	}

	var paths []*GCRootPath

	// BFS state: each entry is a path (list of nodes from target to current)
	type bfsState struct {
		path    []*PathNode
		visited map[uint64]bool
	}

	// Start from the target object
	startNode := &PathNode{
		ObjectID:  objectID,
		ClassID:   g.objectClass[objectID],
		ClassName: g.classNames[g.objectClass[objectID]],
		Size:      g.objectSize[objectID],
	}

	queue := []bfsState{{
		path:    []*PathNode{startNode},
		visited: map[uint64]bool{objectID: true},
	}}

	for len(queue) > 0 && len(paths) < maxPaths {
		current := queue[0]
		queue = queue[1:]

		lastNode := current.path[len(current.path)-1]

		// Check if we reached a GC root
		if rootType, isRoot := g.gcRootSet[lastNode.ObjectID]; isRoot {
			// Reverse the path (from GC root to target)
			reversedPath := make([]*PathNode, len(current.path))
			for i, node := range current.path {
				reversedPath[len(current.path)-1-i] = node
			}
			paths = append(paths, &GCRootPath{
				RootType: rootType,
				Path:     reversedPath,
				Depth:    len(current.path),
			})
			continue
		}

		// Check depth limit
		if len(current.path) >= maxDepth {
			continue
		}

		// Explore incoming references (who references this object)
		for _, ref := range g.incomingRefs[lastNode.ObjectID] {
			if current.visited[ref.FromObjectID] {
				continue
			}

			newNode := &PathNode{
				ObjectID:  ref.FromObjectID,
				ClassID:   ref.FromClassID,
				ClassName: g.classNames[ref.FromClassID],
				FieldName: ref.FieldName,
				Size:      g.objectSize[ref.FromObjectID],
			}

			newVisited := make(map[uint64]bool)
			for k, v := range current.visited {
				newVisited[k] = v
			}
			newVisited[ref.FromObjectID] = true

			newPath := make([]*PathNode, len(current.path)+1)
			copy(newPath, current.path)
			newPath[len(current.path)] = newNode

			queue = append(queue, bfsState{
				path:    newPath,
				visited: newVisited,
			})
		}
	}

	return paths
}

// MaxObjectsForRetainerAnalysis is the maximum number of target objects to analyze.
// For classes with more instances, we use stratified sampling to maintain accuracy.
const MaxObjectsForRetainerAnalysis = 1000

// SamplingConfig controls how sampling is performed for large datasets.
type SamplingConfig struct {
	MaxSamples       int     // Maximum number of samples
	MinSampleRatio   float64 // Minimum sampling ratio (e.g., 0.01 = 1%)
	SizeWeighted     bool    // Whether to weight by object size
	StratifiedBySize bool    // Whether to stratify by size buckets
}

// DefaultSamplingConfig returns the default sampling configuration.
func DefaultSamplingConfig() SamplingConfig {
	return SamplingConfig{
		MaxSamples:       1000,
		MinSampleRatio:   0.01,
		SizeWeighted:     true,
		StratifiedBySize: true,
	}
}

// stratifiedSample performs stratified sampling by object size to ensure
// both large and small objects are represented in the sample.
// This preserves the distribution of retainer patterns across different object sizes.
func (g *ReferenceGraph) stratifiedSample(objects []uint64, config SamplingConfig) []uint64 {
	if len(objects) <= config.MaxSamples {
		return objects
	}

	// Calculate actual sample size (at least MinSampleRatio of total)
	sampleSize := config.MaxSamples
	minSamples := int(float64(len(objects)) * config.MinSampleRatio)
	if minSamples > sampleSize {
		sampleSize = minSamples
	}
	if sampleSize > len(objects) {
		sampleSize = len(objects)
	}

	if !config.StratifiedBySize {
		// Simple uniform sampling with even distribution
		step := len(objects) / sampleSize
		if step < 1 {
			step = 1
		}
		result := make([]uint64, 0, sampleSize)
		for i := 0; i < len(objects) && len(result) < sampleSize; i += step {
			result = append(result, objects[i])
		}
		return result
	}

	// Stratified sampling by size buckets
	// Sort objects by size (descending) to ensure large objects are included
	type objWithSize struct {
		id   uint64
		size int64
	}
	objSizes := make([]objWithSize, len(objects))
	for i, objID := range objects {
		objSizes[i] = objWithSize{id: objID, size: g.objectSize[objID]}
	}
	sort.Slice(objSizes, func(i, j int) bool {
		return objSizes[i].size > objSizes[j].size
	})

	// Divide into 3 strata: top 10% (large), middle 40%, bottom 50% (small)
	// Sample more from large objects as they're more likely to be leak sources
	topCount := len(objects) / 10
	midCount := len(objects) * 4 / 10
	bottomCount := len(objects) - topCount - midCount

	// Allocate samples: 40% to top, 35% to middle, 25% to bottom
	topSamples := sampleSize * 40 / 100
	midSamples := sampleSize * 35 / 100
	bottomSamples := sampleSize - topSamples - midSamples

	result := make([]uint64, 0, sampleSize)

	// Sample from top stratum (large objects)
	step := 1
	if topCount > topSamples && topSamples > 0 {
		step = topCount / topSamples
	}
	for i := 0; i < topCount && len(result) < topSamples; i += step {
		result = append(result, objSizes[i].id)
	}

	// Sample from middle stratum
	step = 1
	if midCount > midSamples && midSamples > 0 {
		step = midCount / midSamples
	}
	for i := topCount; i < topCount+midCount && len(result) < topSamples+midSamples; i += step {
		result = append(result, objSizes[i].id)
	}

	// Sample from bottom stratum (small objects)
	step = 1
	if bottomCount > bottomSamples && bottomSamples > 0 {
		step = bottomCount / bottomSamples
	}
	for i := topCount + midCount; i < len(objSizes) && len(result) < sampleSize; i += step {
		result = append(result, objSizes[i].id)
	}

	return result
}

// ComputeMultiLevelRetainers computes retainers up to maxDepth levels.
// For classes with many instances, it uses stratified sampling to maintain
// statistical accuracy while ensuring performance.
//
// Key guarantees for root cause analysis:
// 1. Large objects are always included (more likely to be leak sources)
// 2. Retainer patterns are preserved through stratified sampling
// 3. Statistics are scaled to represent the full population
func (g *ReferenceGraph) ComputeMultiLevelRetainers(targetClassName string, maxDepth, topN int) *ClassRetainers {
	if maxDepth <= 0 {
		maxDepth = 5
	}

	// Find all objects of the target class
	var targetObjects []uint64
	var targetClassID uint64
	for classID, name := range g.classNames {
		if name == targetClassName {
			targetClassID = classID
			break
		}
	}

	var totalSize int64
	for objID, classID := range g.objectClass {
		if classID == targetClassID {
			targetObjects = append(targetObjects, objID)
			totalSize += g.objectSize[objID]
		}
	}

	if len(targetObjects) == 0 {
		return nil
	}

	// Use stratified sampling for large datasets
	config := DefaultSamplingConfig()
	sampleObjects := g.stratifiedSample(targetObjects, config)
	sampleRatio := float64(len(sampleObjects)) / float64(len(targetObjects))

	// Calculate sample total size for accurate percentage calculation
	var sampleTotalSize int64
	for _, objID := range sampleObjects {
		sampleTotalSize += g.objectSize[objID]
	}

	// Aggregate retainers by class and depth using BFS
	// Key insight: Each target object should only be counted once per retainer class+depth
	type retainerKey struct {
		className string
		fieldName string
		depth     int
	}
	retainerStats := make(map[retainerKey]*RetainerInfo)

	for _, objID := range sampleObjects {
		// BFS to find retainers at each depth level
		visited := map[uint64]bool{objID: true}
		currentLevel := []uint64{objID}
		objSize := g.objectSize[objID]
		
		// Track which retainer keys we've already counted for this target object
		countedRetainers := make(map[retainerKey]bool)

		for depth := 1; depth <= maxDepth && len(currentLevel) > 0; depth++ {
			nextLevel := []uint64{}

			for _, currentObjID := range currentLevel {
				for _, ref := range g.incomingRefs[currentObjID] {
					if visited[ref.FromObjectID] {
						continue
					}
					visited[ref.FromObjectID] = true

					retainerClassName := g.classNames[ref.FromClassID]
					if retainerClassName == "" {
						retainerClassName = "(unknown)"
					}

					key := retainerKey{
						className: retainerClassName,
						fieldName: ref.FieldName,
						depth:     depth,
					}

					if _, ok := retainerStats[key]; !ok {
						retainerStats[key] = &RetainerInfo{
							RetainerClass: retainerClassName,
							FieldName:     ref.FieldName,
							Depth:         depth,
						}
					}
					
					// Only count this target object once per retainer key
					if !countedRetainers[key] {
						countedRetainers[key] = true
						retainerStats[key].RetainedCount++
						retainerStats[key].RetainedSize += objSize
					}

					nextLevel = append(nextLevel, ref.FromObjectID)
				}
			}

			currentLevel = nextLevel
		}
	}

	// Convert to slice and calculate percentages
	var retainers []*RetainerInfo
	for _, r := range retainerStats {
		// Scale count and size to estimate full population values FIRST
		// This ensures percentage is calculated on the scaled values
		if sampleRatio < 1.0 {
			r.RetainedCount = int64(float64(r.RetainedCount) / sampleRatio)
			r.RetainedSize = int64(float64(r.RetainedSize) / sampleRatio)
		}
		
		// Calculate percentage based on TOTAL size (not sample size)
		// This gives accurate percentage representation
		if totalSize > 0 {
			r.Percentage = float64(r.RetainedSize) * 100.0 / float64(totalSize)
		}
		
		// Cap percentage at 100% and size at total size
		if r.Percentage > 100.0 {
			r.Percentage = 100.0
		}
		if r.RetainedSize > totalSize {
			r.RetainedSize = totalSize
		}
		
		retainers = append(retainers, r)
	}

	// Sort by depth first, then by retained size
	sort.Slice(retainers, func(i, j int) bool {
		if retainers[i].Depth != retainers[j].Depth {
			return retainers[i].Depth < retainers[j].Depth
		}
		return retainers[i].RetainedSize > retainers[j].RetainedSize
	})

	if len(retainers) > topN {
		retainers = retainers[:topN]
	}

	// Get sample GC root paths - prioritize large objects for more relevant paths
	var gcRootPaths []*GCRootPath
	sampleCount := min(5, len(sampleObjects)) // Increase sample for better coverage
	for i := 0; i < sampleCount; i++ {
		paths := g.FindPathsToGCRoot(sampleObjects[i], 1, 15)
		gcRootPaths = append(gcRootPaths, paths...)
	}

	return &ClassRetainers{
		ClassName:     targetClassName,
		TotalSize:     totalSize,
		InstanceCount: int64(len(targetObjects)),
		Retainers:     retainers,
		GCRootPaths:   gcRootPaths,
	}
}

// retainedSizeEstimated indicates if retained sizes are estimated (not exact).
var retainedSizeEstimated bool

// ComputeDominatorTree computes the dominator tree using Lengauer-Tarjan algorithm.
// This algorithm has near-linear time complexity O(E·α(E,V)) and always produces exact results.
func (g *ReferenceGraph) ComputeDominatorTree() {
	if g.dominatorComputed {
		return
	}

	// Always use exact Lengauer-Tarjan algorithm for accurate results
	g.computeLengauerTarjan()
	g.dominatorComputed = true
	retainedSizeEstimated = false
}

// superRootID is a special ID representing the super root that dominates all GC roots
const superRootID = ^uint64(0)

// dominatorState holds the state for dominator computation
type dominatorState struct {
	// Object ID to index mapping for array-based access
	objToIdx map[uint64]int
	idxToObj []uint64

	// Algorithm data structures (indexed by node index, 1-based)
	// Index 0 is reserved for "undefined"
	parent   []int   // parent in DFS spanning tree
	semi     []int   // semidominator (as DFS number)
	idom     []int   // immediate dominator
	ancestor []int   // ancestor in forest for path compression
	label    []int   // label for path compression (best semi on path)
	bucket   [][]int // bucket[w] = nodes whose semidominator is w

	// DFS data
	dfn    []int // dfn[v] = DFS number of node v (0 = not visited)
	vertex []int // vertex[i] = node with DFS number i (1-based)
	n      int   // number of nodes visited by DFS
}

// computeLengauerTarjan implements the Lengauer-Tarjan algorithm for computing dominators.
// This is the standard algorithm used by Eclipse MAT and other professional tools.
// Time complexity: O(E·α(E,V)) where α is the inverse Ackermann function (nearly linear).
//
// Reference: "A Fast Algorithm for Finding Dominators in a Flowgraph"
// by Thomas Lengauer and Robert Endre Tarjan, 1979
func (g *ReferenceGraph) computeLengauerTarjan() {
	numObjects := len(g.objectClass)
	if numObjects == 0 {
		return
	}

	// Total nodes = objects + 1 (for virtual super root at index 0)
	totalNodes := numObjects + 1

	// Initialize state
	state := &dominatorState{
		objToIdx: make(map[uint64]int, totalNodes),
		idxToObj: make([]uint64, totalNodes),
		parent:   make([]int, totalNodes),
		semi:     make([]int, totalNodes),
		idom:     make([]int, totalNodes),
		ancestor: make([]int, totalNodes),
		label:    make([]int, totalNodes),
		bucket:   make([][]int, totalNodes),
		dfn:      make([]int, totalNodes),
		vertex:   make([]int, totalNodes),
		n:        0,
	}

	// Index 0 = super root (virtual node that dominates all GC roots)
	state.objToIdx[superRootID] = 0
	state.idxToObj[0] = superRootID

	// Create indices for all objects (1-based)
	idx := 1
	for objID := range g.objectClass {
		state.objToIdx[objID] = idx
		state.idxToObj[idx] = objID
		idx++
	}

	// Initialize arrays
	for i := 0; i < totalNodes; i++ {
		state.semi[i] = 0    // 0 means undefined
		state.ancestor[i] = 0 // 0 means no ancestor
		state.label[i] = i    // initially, label[v] = v
		state.idom[i] = 0     // 0 means undefined
		state.dfn[i] = 0      // 0 means not visited
	}

	// Build successors list (outgoing edges in reference graph)
	successors := make([][]int, totalNodes)
	for i := range successors {
		successors[i] = make([]int, 0)
	}

	// Super root (index 0) has edges to all GC roots
	gcRootSet := make(map[uint64]bool)
	gcRootsFound := 0
	gcRootsNotFound := 0
	for _, root := range g.gcRoots {
		if rootIdx, ok := state.objToIdx[root.ObjectID]; ok {
			if !gcRootSet[root.ObjectID] {
				gcRootSet[root.ObjectID] = true
				successors[0] = append(successors[0], rootIdx)
				gcRootsFound++
			}
		} else {
			gcRootsNotFound++
		}
	}

	// Also treat all Class objects as implicit GC roots
	// Class objects are held by ClassLoaders and should be considered reachable
	classObjectsAdded := 0
	for classObjID := range g.classObjectIDs {
		if rootIdx, ok := state.objToIdx[classObjID]; ok {
			if !gcRootSet[classObjID] {
				gcRootSet[classObjID] = true
				successors[0] = append(successors[0], rootIdx)
				classObjectsAdded++
			}
		}
	}

	// Note: We intentionally do NOT treat objects with no incoming refs as roots.
	// These are likely unreachable garbage objects.
	// Only explicit GC roots and Class objects are treated as roots.
	// Objects not reachable from roots will have super root as dominator,
	// and their retained size will only be their shallow size.
	noIncomingCount := 0
	for objID := range g.objectClass {
		if len(g.incomingRefs[objID]) == 0 && !gcRootSet[objID] {
			noIncomingCount++
		}
	}
	g.debugf("Objects with no incoming refs (not added as roots): %d", noIncomingCount)

	// Add edges from each object to objects it references
	// Debug: track ArrayList -> Object[] references
	arrayListToObjectArrayRefs := 0
	arrayListClassID := uint64(0)
	objectArrayClassID := uint64(0)
	for classID, name := range g.classNames {
		if name == "java.util.ArrayList" {
			arrayListClassID = classID
		}
		if name == "java.lang.Object[]" {
			objectArrayClassID = classID
		}
	}
	
	for objID := range g.objectClass {
		fromIdx := state.objToIdx[objID]
		fromClassID := g.objectClass[objID]
		seen := make(map[int]bool)
		for _, ref := range g.outgoingRefs[objID] {
			if toIdx, ok := state.objToIdx[ref.ToObjectID]; ok {
				if !seen[toIdx] {
					seen[toIdx] = true
					successors[fromIdx] = append(successors[fromIdx], toIdx)
					
					// Debug: count ArrayList -> Object[] references
					if fromClassID == arrayListClassID {
						toClassID := g.objectClass[ref.ToObjectID]
						if toClassID == objectArrayClassID {
							arrayListToObjectArrayRefs++
						}
					}
				}
			}
		}
	}
	g.debugf("ArrayList -> Object[] references: %d (ArrayList classID=%d, Object[] classID=%d)", 
		arrayListToObjectArrayRefs, arrayListClassID, objectArrayClassID)

	// Build predecessors list (reverse edges)
	predecessors := make([][]int, totalNodes)
	for i := range predecessors {
		predecessors[i] = make([]int, 0)
	}
	for v := 0; v < totalNodes; v++ {
		for _, w := range successors[v] {
			predecessors[w] = append(predecessors[w], v)
		}
	}

	// Step 1: DFS to compute spanning tree and DFS numbering
	// Use iterative DFS to avoid stack overflow on large graphs
	type dfsFrame struct {
		v     int
		i     int // index into successors[v]
		first bool
	}

	stack := []dfsFrame{{v: 0, i: 0, first: true}}
	for len(stack) > 0 {
		frame := &stack[len(stack)-1]

		if frame.first {
			frame.first = false
			state.n++
			state.dfn[frame.v] = state.n
			state.vertex[state.n] = frame.v
			state.semi[frame.v] = state.n // Initialize semi to DFS number
		}

		// Find next unvisited successor
		found := false
		for frame.i < len(successors[frame.v]) {
			w := successors[frame.v][frame.i]
			frame.i++
			if state.dfn[w] == 0 { // Not visited
				state.parent[w] = frame.v
				stack = append(stack, dfsFrame{v: w, i: 0, first: true})
				found = true
				break
			}
		}

		if !found {
			stack = stack[:len(stack)-1]
		}
	}

	g.debugf("DFS visited %d nodes out of %d total (%.1f%%)", 
		state.n, totalNodes, float64(state.n)*100.0/float64(totalNodes))
	g.debugf("GC roots: %d explicit + %d class = %d total (found=%d, notFound=%d)", 
		len(g.gcRoots), classObjectsAdded, len(gcRootSet), gcRootsFound, gcRootsNotFound)
	
	// Debug: count objects with outgoing references
	objectsWithOutgoing := 0
	totalOutgoingRefs := 0
	objectsWithIncoming := 0
	for objID := range g.objectClass {
		if refs := g.outgoingRefs[objID]; len(refs) > 0 {
			objectsWithOutgoing++
			totalOutgoingRefs += len(refs)
		}
		if refs := g.incomingRefs[objID]; len(refs) > 0 {
			objectsWithIncoming++
		}
	}
	objectsWithNoIncoming := len(g.objectClass) - objectsWithIncoming
	g.debugf("Objects with outgoing refs: %d, total outgoing refs: %d",
		objectsWithOutgoing, totalOutgoingRefs)
	g.debugf("Objects with NO incoming refs (potential roots or unreachable): %d",
		objectsWithNoIncoming)
	
	// Debug: GC root types
	rootTypeCounts := make(map[GCRootType]int)
	for _, root := range g.gcRoots {
		rootTypeCounts[root.Type]++
	}
	g.debugf("GC root types: %v", rootTypeCounts)

	// LINK: add edge (v, w) to the forest
	link := func(v, w int) {
		state.ancestor[w] = v
	}

	// EVAL: find the node with minimum semi on the path from v to root of its tree
	var eval func(v int) int
	eval = func(v int) int {
		if state.ancestor[v] == 0 {
			return v
		}
		compressPath(state, v)
		return state.label[v]
	}

	// Steps 2 & 3: Compute semidominators and implicitly define idom
	// Process nodes in reverse DFS order (excluding root)
	for i := state.n; i >= 2; i-- {
		w := state.vertex[i]

		// Step 2: Compute semidominator of w
		// semi(w) = min { dfn(v) | v is a predecessor of w and dfn(v) < dfn(w) }
		//         ∪ min { semi(u) | u = eval(v) for predecessor v with dfn(v) > dfn(w) }
		for _, v := range predecessors[w] {
			if state.dfn[v] == 0 {
				continue // v not reachable from root
			}
			var u int
			if state.dfn[v] <= state.dfn[w] {
				u = v
			} else {
				u = eval(v)
			}
			if state.semi[u] < state.semi[w] {
				state.semi[w] = state.semi[u]
			}
		}

		// Add w to bucket of vertex[semi[w]]
		semiNode := state.vertex[state.semi[w]]
		state.bucket[semiNode] = append(state.bucket[semiNode], w)

		// Link w to its parent in DFS tree
		link(state.parent[w], w)

		// Step 3: Implicitly define idom for nodes in bucket of parent[w]
		for _, v := range state.bucket[state.parent[w]] {
			u := eval(v)
			if state.semi[u] < state.semi[v] {
				state.idom[v] = u
			} else {
				state.idom[v] = state.parent[w]
			}
		}
		state.bucket[state.parent[w]] = nil
	}

	// Step 4: Explicitly define idom
	for i := 2; i <= state.n; i++ {
		w := state.vertex[i]
		if state.idom[w] != state.vertex[state.semi[w]] {
			state.idom[w] = state.idom[state.idom[w]]
		}
	}

	// idom of root is 0 (undefined)
	state.idom[0] = 0

	// Convert results back to object IDs and mark reachable objects
	g.reachableObjects = make(map[uint64]bool, state.n)
	for i := 1; i <= state.n; i++ {
		v := state.vertex[i]
		if v == 0 {
			continue // skip super root
		}
		objID := state.idxToObj[v]
		g.reachableObjects[objID] = true // Mark as reachable
		idomNode := state.idom[v]
		if idomNode == 0 {
			g.dominators[objID] = superRootID
		} else {
			g.dominators[objID] = state.idxToObj[idomNode]
		}
	}

	// Handle unreachable objects (not visited by DFS)
	// These are garbage objects that should not be counted in statistics
	unreachableCount := 0
	for objID := range g.objectClass {
		if _, hasDom := g.dominators[objID]; !hasDom {
			g.dominators[objID] = superRootID
			unreachableCount++
		}
	}
	g.debugf("Unreachable objects (garbage): %d", unreachableCount)

	// Compute retained sizes
	g.computeRetainedSizes()
}

// compressPath performs path compression for EVAL using iterative approach.
// After compression, label[v] contains the node with minimum semi on path from v to tree root.
// Optimization: Uses iterative approach to avoid stack overflow on deep paths.
func compressPath(state *dominatorState, v int) {
	// First, collect the path from v to the root of its tree
	path := make([]int, 0, 32)
	current := v
	for state.ancestor[current] != 0 && state.ancestor[state.ancestor[current]] != 0 {
		path = append(path, current)
		current = state.ancestor[current]
	}
	
	// Now compress the path from root to v
	for i := len(path) - 1; i >= 0; i-- {
		node := path[i]
		anc := state.ancestor[node]
		if state.semi[state.label[anc]] < state.semi[state.label[node]] {
			state.label[node] = state.label[anc]
		}
		state.ancestor[node] = state.ancestor[anc]
	}
}

// IsRetainedSizeEstimated returns true if retained sizes are estimated rather than exact.
func (g *ReferenceGraph) IsRetainedSizeEstimated() bool {
	return retainedSizeEstimated
}

// computeRetainedSizes computes retained sizes based on dominator tree.
// Retained size of an object = its shallow size + sum of retained sizes of all objects it dominates
// Also pre-computes class-level retained sizes for fast lookup.
// 
// Optimization: Uses iterative post-order traversal instead of recursion to avoid stack overflow
// on large heaps and reduce function call overhead.
func (g *ReferenceGraph) computeRetainedSizes() {
	// Build dominator tree children map (reverse of dominators)
	// children[A] = list of objects whose immediate dominator is A
	// Pre-allocate with estimated capacity
	children := make(map[uint64][]uint64, len(g.objectClass)/10)

	// Count dominators for debugging
	dominatedBySuperRoot := 0
	dominatedByOther := 0

	// Only iterate over objects in our object graph
	for objID := range g.objectClass {
		domID := g.dominators[objID]
		if domID == superRootID {
			dominatedBySuperRoot++
		} else if domID != 0 && domID != objID {
			dominatedByOther++
			// Verify domID is a valid object
			if _, exists := g.objectClass[domID]; exists {
				children[domID] = append(children[domID], objID)
			}
		}
	}

	g.debugf("Dominator stats: dominatedBySuperRoot=%d, dominatedByOther=%d, objectsWithChildren=%d",
		dominatedBySuperRoot, dominatedByOther, len(children))

	// Initialize all retained sizes to shallow size
	for objID := range g.objectClass {
		g.retainedSizes[objID] = g.objectSize[objID]
	}

	// Compute retained sizes using iterative post-order traversal
	// This avoids stack overflow on large heaps and is faster than recursion
	
	// Find all leaf nodes (objects with no children in dominator tree)
	// These are the starting points for bottom-up computation
	leafNodes := make([]uint64, 0, len(g.objectClass)/2)
	for objID := range g.objectClass {
		if len(children[objID]) == 0 {
			leafNodes = append(leafNodes, objID)
		}
	}
	
	// Track how many children each node has remaining to process
	remainingChildren := make(map[uint64]int, len(children))
	for objID, childList := range children {
		remainingChildren[objID] = len(childList)
	}
	
	// Process nodes in bottom-up order using a work queue
	queue := make([]uint64, 0, len(g.objectClass))
	queue = append(queue, leafNodes...)
	processed := make(map[uint64]bool, len(g.objectClass))
	
	for len(queue) > 0 {
		objID := queue[0]
		queue = queue[1:]
		
		if processed[objID] {
			continue
		}
		processed[objID] = true
		
		// Retained size is already initialized to shallow size
		// Add retained sizes of all children (already computed since we process bottom-up)
		for _, childID := range children[objID] {
			g.retainedSizes[objID] += g.retainedSizes[childID]
		}
		
		// Notify parent that this child is done
		domID := g.dominators[objID]
		if domID != superRootID && domID != 0 {
			remainingChildren[domID]--
			// If all children of parent are processed, add parent to queue
			if remainingChildren[domID] == 0 {
				queue = append(queue, domID)
			}
		}
	}
	
	// Handle any remaining unprocessed objects (shouldn't happen in a valid tree)
	for objID := range g.objectClass {
		if !processed[objID] {
			// Process remaining objects - their retained size stays as shallow size
			processed[objID] = true
		}
	}

	// Debug: count objects with retained > shallow
	objectsWithRetained := 0
	for objID := range g.objectClass {
		if g.retainedSizes[objID] > g.objectSize[objID] {
			objectsWithRetained++
		}
	}
	g.debugf("Objects with retained > shallow: %d", objectsWithRetained)

	// Pre-compute class-level retained sizes for fast lookup
	g.computeClassRetainedSizes()
}

// computeClassRetainedSizes pre-computes two views of class retained size:
// 1) MAT top-level view (classRetainedSizes): retained of instances not dominated by same class
// 2) Attribution view (classRetainedSizesAttributed): non-overlapping, attribute each object's
//    retained size to the nearest dominator of a different class; if dominator is super root,
//    attribute to the object's own class. This better matches IDE "Retained by class"口径。
func (g *ReferenceGraph) computeClassRetainedSizes() {
	// Reset maps in case ComputeDominatorTree is called multiple times
	g.classRetainedSizes = make(map[uint64]int64)
	g.classRetainedSizesAttributed = make(map[uint64]int64)

	// --- View 1: MAT top-level (avoid intra-class double count, but allows cross-class overlap) ---
	for objID, classID := range g.objectClass {
		domID := g.dominators[objID]

		isDominatedBySameClass := false
		if domID != superRootID && domID != 0 {
			if domClassID, exists := g.objectClass[domID]; exists && domClassID == classID {
				isDominatedBySameClass = true
			}
		}

		if !isDominatedBySameClass {
			g.classRetainedSizes[classID] += g.retainedSizes[objID]
		}
	}

	// --- View 2: Attribution (non-overlapping) ---
	// Attribute each object's SHALLOW size to the nearest dominator whose class differs.
	// If dominator chain reaches super root, attribute to the object's own class.
	// This ensures every byte is counted exactly once and totals ~= heap size.
	for objID, classID := range g.objectClass {
		attribClassID := classID
		domID := g.dominators[objID]

		for domID != superRootID && domID != 0 {
			domClassID, ok := g.objectClass[domID]
			if !ok {
				break
			}
			if domClassID != classID {
				attribClassID = domClassID
				break
			}
			domID = g.dominators[domID]
		}

		g.classRetainedSizesAttributed[attribClassID] += g.objectSize[objID]
	}
}

// GetRetainedSize returns the retained size for an object.
func (g *ReferenceGraph) GetRetainedSize(objectID uint64) int64 {
	if !g.dominatorComputed {
		g.ComputeDominatorTree()
	}
	return g.retainedSizes[objectID]
}

// GetClassRetainedSize returns the MAT top-level retained size for a class (may overlap across classes).
// This matches Eclipse MAT "retained if delete all instances of this class" semantics.
func (g *ReferenceGraph) GetClassRetainedSize(className string) int64 {
	if !g.dominatorComputed {
		g.ComputeDominatorTree()
	}

	for classID, name := range g.classNames {
		if name == className {
			return g.classRetainedSizes[classID]
		}
	}
	return 0
}

// GetClassRetainedSizeAttributed returns the non-overlapping attribution size.
// Each object's shallow size is attributed to the nearest dominator of a different class
// (or itself if dominated only by same class / super root). Totals ~= heap size and
// are closer to IDEA 的"Retained by class"。
func (g *ReferenceGraph) GetClassRetainedSizeAttributed(className string) int64 {
	if !g.dominatorComputed {
		g.ComputeDominatorTree()
	}

	for classID, name := range g.classNames {
		if name == className {
			return g.classRetainedSizesAttributed[classID]
		}
	}
	return 0
}

// IsObjectReachable returns true if the object is reachable from GC roots.
// This is determined during dominator tree computation.
func (g *ReferenceGraph) IsObjectReachable(objectID uint64) bool {
	if !g.dominatorComputed {
		return true // Assume reachable if not computed yet
	}
	return g.reachableObjects[objectID]
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

// ComputeRetainersForClass computes what classes retain instances of the given class.
// This is the original single-level retainer analysis.
func (g *ReferenceGraph) ComputeRetainersForClass(targetClassName string, topN int) *ClassRetainers {
	// Find all objects of the target class
	var targetObjects []uint64
	var targetClassID uint64
	for classID, name := range g.classNames {
		if name == targetClassName {
			targetClassID = classID
			break
		}
	}

	var totalSize int64
	for objID, classID := range g.objectClass {
		if classID == targetClassID {
			targetObjects = append(targetObjects, objID)
			totalSize += g.objectSize[objID]
		}
	}

	if len(targetObjects) == 0 {
		return nil
	}

	// Aggregate retainers by class
	retainerStats := make(map[string]*RetainerInfo)

	for _, objID := range targetObjects {
		refs := g.incomingRefs[objID]
		for _, ref := range refs {
			retainerClassName := g.classNames[ref.FromClassID]
			if retainerClassName == "" {
				retainerClassName = "(unknown)"
			}

			key := retainerClassName
			if ref.FieldName != "" {
				key = retainerClassName + "." + ref.FieldName
			}

			if _, ok := retainerStats[key]; !ok {
				retainerStats[key] = &RetainerInfo{
					RetainerClass: retainerClassName,
					FieldName:     ref.FieldName,
					Depth:         1,
				}
			}
			retainerStats[key].RetainedCount++
			retainerStats[key].RetainedSize += g.objectSize[objID]
		}
	}

	// Convert to slice and sort
	var retainers []*RetainerInfo
	for _, r := range retainerStats {
		r.Percentage = float64(r.RetainedSize) * 100.0 / float64(totalSize)
		retainers = append(retainers, r)
	}

	sort.Slice(retainers, func(i, j int) bool {
		return retainers[i].RetainedSize > retainers[j].RetainedSize
	})

	if len(retainers) > topN {
		retainers = retainers[:topN]
	}

	// Get retained size from dominator tree if computed
	var retainedSize int64
	if g.dominatorComputed {
		retainedSize = g.GetClassRetainedSize(targetClassName)
	}

	return &ClassRetainers{
		ClassName:     targetClassName,
		TotalSize:     totalSize,
		InstanceCount: int64(len(targetObjects)),
		Retainers:     retainers,
		RetainedSize:  retainedSize,
	}
}

// ComputeTopRetainers computes retainer info for the top memory-consuming classes.
func (g *ReferenceGraph) ComputeTopRetainers(topClasses []*ClassStats, topN int) map[string]*ClassRetainers {
	result := make(map[string]*ClassRetainers)

	// Compute dominator tree first for retained sizes
	g.ComputeDominatorTree()

	for _, cls := range topClasses {
		// Use multi-level retainer analysis
		retainers := g.ComputeMultiLevelRetainers(cls.ClassName, 5, topN)
		if retainers != nil && len(retainers.Retainers) > 0 {
			// Add retained size from dominator tree (MAT top-level view)
			retainers.RetainedSize = g.GetClassRetainedSize(cls.ClassName)
			result[cls.ClassName] = retainers
		}
	}

	return result
}

// GetReferenceGraphData returns data for visualization.
type ReferenceGraphData struct {
	Nodes []ReferenceGraphNode `json:"nodes"`
	Edges []ReferenceGraphEdge `json:"edges"`
}

// ReferenceGraphNode represents a node in the reference graph visualization.
type ReferenceGraphNode struct {
	ID           string `json:"id"`
	ClassName    string `json:"class_name"`
	Size         int64  `json:"size"`
	RetainedSize int64  `json:"retained_size"`
	IsGCRoot     bool   `json:"is_gc_root"`
	GCRootType   string `json:"gc_root_type,omitempty"`
}

// ReferenceGraphEdge represents an edge in the reference graph visualization.
type ReferenceGraphEdge struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	FieldName string `json:"field_name,omitempty"`
}

// isJDKInternalClass checks if a class is a JDK internal class that should be skipped
// when looking for business-level retainers.
func isJDKInternalClass(className string) bool {
	jdkPrefixes := []string{
		"java.lang.",
		"java.util.",
		"java.io.",
		"java.nio.",
		"java.net.",
		"java.security.",
		"java.math.",
		"java.text.",
		"java.time.",
		"java.sql.",
		"java.reflect.",
		"javax.",
		"sun.",
		"com.sun.",
		"jdk.",
	}
	
	// Array types are internal
	if strings.HasSuffix(className, "[]") {
		return true
	}
	
	for _, prefix := range jdkPrefixes {
		if strings.HasPrefix(className, prefix) {
			return true
		}
	}
	return false
}

// isFrameworkClass checks if a class is a core framework internal class.
// Returns true only for the most internal framework classes that are rarely the root cause.
// Application-level framework usage (like Kafka consumers, Spring beans) are NOT filtered.
func isFrameworkClass(className string) bool {
	// Only filter the most internal framework classes
	// These are implementation details that are almost never the root cause
	coreInternals := []string{
		// Spring internals (not beans or components)
		"org.springframework.aop.framework.",
		"org.springframework.beans.factory.support.",
		"org.springframework.context.annotation.ConfigurationClassParser",
		"org.springframework.core.annotation.AnnotationUtils",
		"org.springframework.util.ConcurrentReferenceHashMap",
		// Netty buffer pool internals
		"io.netty.buffer.PoolArena",
		"io.netty.buffer.PoolChunk",
		"io.netty.buffer.PoolSubpage",
		"io.netty.buffer.PoolThreadCache",
		"io.netty.util.internal.",
		"io.netty.util.Recycler",
		// Guava internals
		"com.google.common.collect.",
		"com.google.common.cache.",
		// Logging internals
		"org.slf4j.impl.",
		"ch.qos.logback.core.",
		"ch.qos.logback.classic.spi.",
		// Jackson internals
		"com.fasterxml.jackson.core.json.",
		"com.fasterxml.jackson.databind.cfg.",
		"com.fasterxml.jackson.databind.introspect.",
		// ByteBuddy internals
		"net.bytebuddy.description.",
		"net.bytebuddy.pool.",
		"net.bytebuddy.dynamic.",
		// OpenTelemetry agent internals
		"io.opentelemetry.javaagent.tooling.",
		"io.opentelemetry.javaagent.shaded.",
		"io.opentelemetry.javaagent.bootstrap.",
		// Arthas internals
		"com.alibaba.arthas.deps.",
	}
	
	for _, prefix := range coreInternals {
		if strings.HasPrefix(className, prefix) {
			return true
		}
	}
	return false
}

// isApplicationLevelClass checks if a class is an application-level class
// that could be relevant for root cause analysis (includes framework beans, consumers, etc.)
// Returns true for classes that are likely to be the root cause of memory issues.
func isApplicationLevelClass(className string) bool {
	if isJDKInternalClass(className) {
		return false
	}
	if isFrameworkClass(className) {
		return false
	}
	return true
}

// isBusinessClass checks if a class is likely a business class or application-level class.
// This includes user code and application-level framework usage (like Spring beans, Kafka consumers, etc.)
func isBusinessClass(className string) bool {
	if isJDKInternalClass(className) {
		return false
	}
	if isFrameworkClass(className) {
		return false
	}
	return true
}

// BusinessRetainer represents a business-level retainer with full path information.
type BusinessRetainer struct {
	ClassName     string   `json:"class_name"`
	FieldPath     []string `json:"field_path"`      // Path from business class to target
	RetainedSize  int64    `json:"retained_size"`
	RetainedCount int64    `json:"retained_count"`
	Percentage    float64  `json:"percentage"`
	Depth         int      `json:"depth"`
	IsGCRoot      bool     `json:"is_gc_root"`
	GCRootType    string   `json:"gc_root_type,omitempty"`
}

// ComputeBusinessRetainers finds business-level classes that retain instances of the target class.
// This skips JDK internal classes and framework classes to find the actual business code
// that is holding references.
// 
// Optimizations applied:
// 1. Uses classToObjects index for O(1) lookup instead of O(n) scan
// 2. Shared visited set across samples to avoid redundant traversals
// 3. Early termination when enough retainers are found
// 4. Reduced path tracking overhead
func (g *ReferenceGraph) ComputeBusinessRetainers(targetClassName string, maxDepth, topN int) []*BusinessRetainer {
	if maxDepth <= 0 {
		maxDepth = 15
	}

	// Use index to find target class objects - O(1) lookup
	targetClassID, found := g.getClassIDByName(targetClassName)
	if !found {
		return nil
	}

	targetObjects := g.getObjectsByClass(targetClassID)
	if len(targetObjects) == 0 {
		return nil
	}

	// Calculate total size
	var totalSize int64
	for _, objID := range targetObjects {
		totalSize += g.objectSize[objID]
	}

	// Use stratified sampling for large datasets
	config := DefaultSamplingConfig()
	config.MaxSamples = 500
	sampleObjects := g.stratifiedSample(targetObjects, config)
	sampleRatio := float64(len(sampleObjects)) / float64(len(targetObjects))

	// Track business retainers - simplified structure for better performance
	type retainerStats struct {
		retainer      *BusinessRetainer
		targetObjects map[uint64]bool
	}
	
	businessRetainers := make(map[string]*retainerStats)
	appLevelRetainers := make(map[string]*retainerStats)
	
	// Shared global visited set for optimization - tracks (objID, depth) pairs
	// This prevents redundant deep traversals from different target objects
	globalVisited := make(map[uint64]int) // objID -> minimum depth reached

	// Process samples with shared state
	for _, objID := range sampleObjects {
		objSize := g.objectSize[objID]
		
		// BFS with optimized structure - no path tracking for performance
		type bfsNode struct {
			objID uint64
			depth int
		}
		
		// Local visited for this target object's BFS
		localVisited := map[uint64]bool{objID: true}
		queue := make([]bfsNode, 0, 256)
		queue = append(queue, bfsNode{objID: objID, depth: 0})
		
		// Track which retainer classes we've counted for this target
		countedBusiness := make(map[string]bool)
		countedAppLevel := make(map[string]bool)
		
		// Early termination: stop if we found enough business retainers for this target
		businessFoundForTarget := 0
		const maxBusinessPerTarget = 10
		
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			
			if current.depth >= maxDepth {
				continue
			}
			
			// Check global visited for pruning
			if prevDepth, seen := globalVisited[current.objID]; seen && prevDepth <= current.depth {
				// Already visited at same or shallower depth, skip deeper exploration
				// But still process for counting if not counted yet
			}
			globalVisited[current.objID] = current.depth
			
			for _, ref := range g.incomingRefs[current.objID] {
				if localVisited[ref.FromObjectID] {
					continue
				}
				localVisited[ref.FromObjectID] = true
				
				retainerClassName := g.classNames[ref.FromClassID]
				if retainerClassName == "" {
					continue // Skip unknown classes
				}
				
				// Check if this is a business class
				if isBusinessClass(retainerClassName) {
					if !countedBusiness[retainerClassName] {
						countedBusiness[retainerClassName] = true
						businessFoundForTarget++
						
						stats, ok := businessRetainers[retainerClassName]
						if !ok {
							stats = &retainerStats{
								retainer: &BusinessRetainer{
									ClassName:  retainerClassName,
									Depth:      current.depth + 1,
									IsGCRoot:   g.IsGCRoot(ref.FromObjectID),
									GCRootType: string(g.GetGCRootType(ref.FromObjectID)),
								},
								targetObjects: make(map[uint64]bool),
							}
							businessRetainers[retainerClassName] = stats
						}
						
						if !stats.targetObjects[objID] {
							stats.targetObjects[objID] = true
							stats.retainer.RetainedCount++
							stats.retainer.RetainedSize += objSize
						}
					}
				} else if !isJDKInternalClass(retainerClassName) && !isFrameworkClass(retainerClassName) {
					// Application-level fallback
					if !countedAppLevel[retainerClassName] {
						countedAppLevel[retainerClassName] = true
						
						stats, ok := appLevelRetainers[retainerClassName]
						if !ok {
							stats = &retainerStats{
								retainer: &BusinessRetainer{
									ClassName:  retainerClassName,
									Depth:      current.depth + 1,
									IsGCRoot:   g.IsGCRoot(ref.FromObjectID),
									GCRootType: string(g.GetGCRootType(ref.FromObjectID)),
								},
								targetObjects: make(map[uint64]bool),
							}
							appLevelRetainers[retainerClassName] = stats
						}
						
						if !stats.targetObjects[objID] {
							stats.targetObjects[objID] = true
							stats.retainer.RetainedCount++
							stats.retainer.RetainedSize += objSize
						}
					}
				}
				
				// Continue BFS if not at max depth and haven't found enough
				if current.depth+1 < maxDepth && businessFoundForTarget < maxBusinessPerTarget {
					queue = append(queue, bfsNode{
						objID: ref.FromObjectID,
						depth: current.depth + 1,
					})
				}
			}
			
			// Early termination for this target
			if businessFoundForTarget >= maxBusinessPerTarget {
				break
			}
		}
	}

	// Use business retainers if found, otherwise use app-level retainers
	retainerMap := businessRetainers
	if len(retainerMap) == 0 {
		retainerMap = appLevelRetainers
	}

	// Convert to slice and calculate percentages
	var retainers []*BusinessRetainer
	sampleCount := len(sampleObjects)
	
	var sampleTotalSize int64
	for _, objID := range sampleObjects {
		sampleTotalSize += g.objectSize[objID]
	}
	
	for _, stats := range retainerMap {
		r := stats.retainer
		
		// Calculate percentage based on retained size vs sample total size
		if sampleTotalSize > 0 {
			r.Percentage = float64(r.RetainedSize) * 100.0 / float64(sampleTotalSize)
		}
		
		if r.Percentage > 100.0 {
			r.Percentage = 100.0
		}
		
		// Scale to estimate full population values
		if sampleRatio < 1.0 && sampleCount > 0 {
			r.RetainedCount = int64(float64(r.RetainedCount) / sampleRatio)
			r.RetainedSize = int64(float64(r.RetainedSize) / sampleRatio)
		}
		retainers = append(retainers, r)
	}

	// Sort by retained size
	sort.Slice(retainers, func(i, j int) bool {
		return retainers[i].RetainedSize > retainers[j].RetainedSize
	})

	if len(retainers) > topN {
		retainers = retainers[:topN]
	}

	return retainers
}

// GetReferenceGraphForClass returns the reference graph data for visualization.
// It includes the target class instances and their retainers up to maxDepth levels.
// Enhanced to prioritize business classes and show meaningful paths.
func (g *ReferenceGraph) GetReferenceGraphForClass(targetClassName string, maxDepth, maxNodes int) *ReferenceGraphData {
	if maxDepth <= 0 {
		maxDepth = 10 // Increased from 3 to find business classes
	}
	if maxNodes <= 0 {
		maxNodes = 100
	}

	// Find target class ID
	var targetClassID uint64
	for classID, name := range g.classNames {
		if name == targetClassName {
			targetClassID = classID
			break
		}
	}

	// Find target objects that have incoming references (are retained by something)
	type objWithRefs struct {
		id       uint64
		size     int64
		refCount int
	}
	var allTargetObjects []objWithRefs
	for objID, classID := range g.objectClass {
		if classID == targetClassID {
			refCount := len(g.incomingRefs[objID])
			allTargetObjects = append(allTargetObjects, objWithRefs{
				id:       objID,
				size:     g.objectSize[objID],
				refCount: refCount,
			})
		}
	}

	if len(allTargetObjects) == 0 {
		return nil
	}

	// Sort by reference count (prefer objects with references), then by size
	sort.Slice(allTargetObjects, func(i, j int) bool {
		if allTargetObjects[i].refCount != allTargetObjects[j].refCount {
			return allTargetObjects[i].refCount > allTargetObjects[j].refCount
		}
		return allTargetObjects[i].size > allTargetObjects[j].size
	})
	
	var targetObjects []uint64
	for i := 0; i < len(allTargetObjects) && i < 10; i++ {
		targetObjects = append(targetObjects, allTargetObjects[i].id)
	}

	nodes := make(map[uint64]*ReferenceGraphNode)
	edges := make(map[string]*ReferenceGraphEdge)

	// BFS to collect nodes and edges, prioritizing paths to business classes
	visited := make(map[uint64]bool)
	currentLevel := targetObjects

	for _, objID := range targetObjects {
		visited[objID] = true
		nodes[objID] = &ReferenceGraphNode{
			ID:           formatObjectID(objID),
			ClassName:    g.classNames[g.objectClass[objID]],
			Size:         g.objectSize[objID],
			RetainedSize: g.retainedSizes[objID],
			IsGCRoot:     g.IsGCRoot(objID),
			GCRootType:   string(g.GetGCRootType(objID)),
		}
	}

	// Track if we've found business classes
	foundBusinessClass := false

	for depth := 1; depth <= maxDepth && len(nodes) < maxNodes; depth++ {
		nextLevel := []uint64{}

		for _, currentObjID := range currentLevel {
			for _, ref := range g.incomingRefs[currentObjID] {
				if visited[ref.FromObjectID] {
					// Still add edge if not exists
					edgeKey := formatObjectID(ref.FromObjectID) + "->" + formatObjectID(currentObjID)
					if _, exists := edges[edgeKey]; !exists {
						edges[edgeKey] = &ReferenceGraphEdge{
							Source:    formatObjectID(ref.FromObjectID),
							Target:    formatObjectID(currentObjID),
							FieldName: ref.FieldName,
						}
					}
					continue
				}

				visited[ref.FromObjectID] = true
				className := g.classNames[ref.FromClassID]
				
				node := &ReferenceGraphNode{
					ID:           formatObjectID(ref.FromObjectID),
					ClassName:    className,
					Size:         g.objectSize[ref.FromObjectID],
					RetainedSize: g.retainedSizes[ref.FromObjectID],
					IsGCRoot:     g.IsGCRoot(ref.FromObjectID),
					GCRootType:   string(g.GetGCRootType(ref.FromObjectID)),
				}
				nodes[ref.FromObjectID] = node

				edgeKey := formatObjectID(ref.FromObjectID) + "->" + formatObjectID(currentObjID)
				edges[edgeKey] = &ReferenceGraphEdge{
					Source:    formatObjectID(ref.FromObjectID),
					Target:    formatObjectID(currentObjID),
					FieldName: ref.FieldName,
				}

				// Check if this is a business class
				if isBusinessClass(className) {
					foundBusinessClass = true
				}

				nextLevel = append(nextLevel, ref.FromObjectID)

				if len(nodes) >= maxNodes {
					break
				}
			}
			if len(nodes) >= maxNodes {
				break
			}
		}

		currentLevel = nextLevel
		
		// If we found business classes and have enough nodes, we can stop
		if foundBusinessClass && len(nodes) >= 20 && depth >= 5 {
			break
		}
	}

	// Convert maps to slices
	nodeList := make([]ReferenceGraphNode, 0, len(nodes))
	for _, node := range nodes {
		nodeList = append(nodeList, *node)
	}

	edgeList := make([]ReferenceGraphEdge, 0, len(edges))
	for _, edge := range edges {
		edgeList = append(edgeList, *edge)
	}

	return &ReferenceGraphData{
		Nodes: nodeList,
		Edges: edgeList,
	}
}

func formatObjectID(id uint64) string {
	return "0x" + formatHex(id)
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
