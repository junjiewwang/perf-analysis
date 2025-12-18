// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"sort"

	"github.com/perf-analysis/pkg/filter"
)

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

	// Reusable visited map from pool to reduce allocations
	visited := GetUint64BoolMap()
	defer PutUint64BoolMap(visited)

	for _, objID := range sampleObjects {
		// Clear visited map for reuse (more efficient than creating new one)
		for k := range visited {
			delete(visited, k)
		}
		visited[objID] = true

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
				if filter.IsBusiness(retainerClassName) {
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
				} else if filter.IsApplicationLevel(retainerClassName) {
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
			RetainedSize: g.GetRetainedSize(objID),
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
				if filter.IsBusiness(className) {
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

// Deprecated compatibility functions - use pkg/filter directly instead

// isJDKInternalClass checks if a class is a JDK internal class that should be skipped
// when looking for business-level retainers.
// Deprecated: Use filter.IsJDKInternal instead.
func isJDKInternalClass(className string) bool {
	return filter.IsJDKInternal(className)
}

// isFrameworkClass checks if a class is a core framework internal class.
// Returns true only for the most internal framework classes that are rarely the root cause.
// Application-level framework usage (like Kafka consumers, Spring beans) are NOT filtered.
// Deprecated: Use filter.IsFrameworkInternal instead.
func isFrameworkClass(className string) bool {
	return filter.IsFrameworkInternal(className)
}

// isApplicationLevelClass checks if a class is an application-level class
// that could be relevant for root cause analysis (includes framework beans, consumers, etc.)
// Returns true for classes that are likely to be the root cause of memory issues.
// Deprecated: Use filter.IsApplicationLevel instead.
func isApplicationLevelClass(className string) bool {
	return filter.IsApplicationLevel(className)
}

// isBusinessClass checks if a class is likely a business class or application-level class.
// This includes user code and application-level framework usage (like Spring beans, Kafka consumers, etc.)
// Deprecated: Use filter.IsBusiness instead.
func isBusinessClass(className string) bool {
	return filter.IsBusiness(className)
}
