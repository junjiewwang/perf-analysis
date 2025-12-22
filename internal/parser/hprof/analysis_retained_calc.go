// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"fmt"
	"sort"
)

// =============================================================================
// Retained Size Strategy Types
// =============================================================================

// RetainedSizeStrategy defines the strategy for retained size calculation.
type RetainedSizeStrategy string

const (
	// RetainedSizeStrategyStandard uses strict dominator-tree based calculation (Eclipse MAT style).
	// An object's retained size = shallow size + sum of retained sizes of all dominated objects.
	RetainedSizeStrategyStandard RetainedSizeStrategy = "standard"

	// RetainedSizeStrategyIDEA uses IDEA-style calculation that includes logically owned objects.
	// This accounts for objects that are not strictly dominated but are logically owned through
	// collection internals (ArrayList, HashMap, etc.).
	RetainedSizeStrategyIDEA RetainedSizeStrategy = "idea"
)

// =============================================================================
// Shared Constants and Utilities
// =============================================================================

// CollectionClasses defines Java collection classes that use Object[] internally.
// This is shared between retained size calculators and analyzers.
var CollectionClasses = map[string]bool{
	"java.util.ArrayList":                    true,
	"java.util.LinkedList":                   true,
	"java.util.HashMap":                      true,
	"java.util.LinkedHashMap":                true,
	"java.util.HashSet":                      true,
	"java.util.LinkedHashSet":                true,
	"java.util.TreeMap":                      true,
	"java.util.TreeSet":                      true,
	"java.util.concurrent.ConcurrentHashMap": true,
	"java.util.IdentityHashMap":              true,
	"java.util.WeakHashMap":                  true,
	"java.util.Vector":                       true,
	"java.util.Stack":                        true,
}

// IsCollectionClass checks if a class is a Java collection class.
func IsCollectionClass(className string) bool {
	return CollectionClasses[className]
}

// FormatBytesSize formats bytes to human-readable string.
// This is the canonical implementation used throughout the package.
func FormatBytesSize(bytes int64) string {
	if bytes >= 1024*1024*1024 {
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	}
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
	}
	if bytes >= 1024 {
		return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%d bytes", bytes)
}

// RetainedSizeCalculator defines the interface for retained size calculation strategies.
// Implementations should be stateless and thread-safe.
type RetainedSizeCalculator interface {
	// Name returns the unique identifier for this calculator.
	Name() RetainedSizeStrategy

	// Description returns a human-readable description of the calculation method.
	Description() string

	// ComputeRetainedSizes computes retained sizes for all objects in the graph.
	// It takes the base retained sizes (from dominator tree) and the graph context,
	// and returns the computed retained sizes.
	//
	// Parameters:
	//   - baseRetainedSizes: map of objectID -> retained size from dominator tree
	//   - ctx: context providing access to graph data
	//
	// Returns:
	//   - map of objectID -> computed retained size
	ComputeRetainedSizes(baseRetainedSizes map[uint64]int64, ctx *RetainedSizeContext) map[uint64]int64
}

// RetainedSizeContext provides read-only access to graph data needed for retained size calculation.
// This abstraction decouples calculators from the ReferenceGraph implementation.
type RetainedSizeContext struct {
	// Object data accessors
	GetObjectSize     func(objectID uint64) int64
	GetObjectClassID  func(objectID uint64) (uint64, bool)
	GetClassName      func(classID uint64) string
	GetDominator      func(objectID uint64) uint64
	GetOutgoingRefs   func(objectID uint64) []ObjectReference
	GetIncomingRefs   func(objectID uint64) []ObjectReference

	// Iteration support
	ForEachObject func(fn func(objectID uint64))

	// Constants
	SuperRootID uint64

	// Logger for debug output (optional)
	Debugf func(format string, args ...interface{})
}

// RetainedSizeCalculatorRegistry manages available retained size calculators.
type RetainedSizeCalculatorRegistry struct {
	calculators map[RetainedSizeStrategy]RetainedSizeCalculator
	defaultCalc RetainedSizeStrategy
}

// NewRetainedSizeCalculatorRegistry creates a new registry with default calculators.
func NewRetainedSizeCalculatorRegistry() *RetainedSizeCalculatorRegistry {
	registry := &RetainedSizeCalculatorRegistry{
		calculators: make(map[RetainedSizeStrategy]RetainedSizeCalculator),
		defaultCalc: RetainedSizeStrategyIDEA, // Default to IDEA style
	}

	// Register built-in calculators
	registry.Register(&StandardRetainedSizeCalculator{})
	registry.Register(&IDEAStyleRetainedSizeCalculator{})

	return registry
}

// Register adds a calculator to the registry.
func (r *RetainedSizeCalculatorRegistry) Register(calc RetainedSizeCalculator) {
	r.calculators[calc.Name()] = calc
}

// Get returns the calculator for the given strategy.
func (r *RetainedSizeCalculatorRegistry) Get(strategy RetainedSizeStrategy) (RetainedSizeCalculator, bool) {
	calc, ok := r.calculators[strategy]
	return calc, ok
}

// GetDefault returns the default calculator.
func (r *RetainedSizeCalculatorRegistry) GetDefault() RetainedSizeCalculator {
	calc, _ := r.calculators[r.defaultCalc]
	return calc
}

// SetDefault sets the default calculation strategy.
func (r *RetainedSizeCalculatorRegistry) SetDefault(strategy RetainedSizeStrategy) {
	r.defaultCalc = strategy
}

// ListStrategies returns all available strategies.
func (r *RetainedSizeCalculatorRegistry) ListStrategies() []RetainedSizeStrategy {
	strategies := make([]RetainedSizeStrategy, 0, len(r.calculators))
	for s := range r.calculators {
		strategies = append(strategies, s)
	}
	return strategies
}

// =============================================================================
// Standard Retained Size Calculator (Eclipse MAT style)
// =============================================================================

// StandardRetainedSizeCalculator implements strict dominator-tree based calculation.
// This is the traditional retained size calculation used by Eclipse MAT.
type StandardRetainedSizeCalculator struct{}

// Name returns the strategy identifier.
func (c *StandardRetainedSizeCalculator) Name() RetainedSizeStrategy {
	return RetainedSizeStrategyStandard
}

// Description returns a human-readable description.
func (c *StandardRetainedSizeCalculator) Description() string {
	return "Standard dominator-tree based calculation (Eclipse MAT style). " +
		"Retained size = shallow size + sum of retained sizes of all strictly dominated objects."
}

// ComputeRetainedSizes returns the base retained sizes unchanged.
// The dominator tree calculation already provides the standard retained sizes.
func (c *StandardRetainedSizeCalculator) ComputeRetainedSizes(
	baseRetainedSizes map[uint64]int64,
	ctx *RetainedSizeContext,
) map[uint64]int64 {
	// Standard calculation is already done by dominator tree
	// Just return a copy to avoid mutation
	result := make(map[uint64]int64, len(baseRetainedSizes))
	for k, v := range baseRetainedSizes {
		result[k] = v
	}
	return result
}

// =============================================================================
// IDEA-Style Retained Size Calculator
// =============================================================================

// IDEAStyleRetainedSizeCalculator implements IDEA-style retained size calculation.
// This includes objects that are logically owned but not dominated due to collection internals.
type IDEAStyleRetainedSizeCalculator struct{}

// Name returns the strategy identifier.
func (c *IDEAStyleRetainedSizeCalculator) Name() RetainedSizeStrategy {
	return RetainedSizeStrategyIDEA
}

// Description returns a human-readable description.
func (c *IDEAStyleRetainedSizeCalculator) Description() string {
	return "IDEA-style calculation that includes logically owned objects. " +
		"Accounts for objects not strictly dominated but owned through collection internals " +
		"(ArrayList, HashMap, etc.). More intuitive for analyzing ClassLoader retained sizes."
}

// ideaStylePrecomputedData holds precomputed data for IDEA-style retained size calculation.
// This eliminates redundant lookups during the main computation loop.
type ideaStylePrecomputedData struct {
	// objectArrayClassID is the class ID for java.lang.Object[]
	objectArrayClassID uint64
	// collectionOwnedObjectArrays contains Object[] IDs that are held by Collection classes
	// Key: Object[] objectID, Value: true
	// This is precomputed once to avoid repeated Collection detection in isChildNotDominatedDueToObjectArray
	collectionOwnedObjectArrays map[uint64]bool
	// objectsReferencedByCollectionObjectArray contains objects that are referenced by Collection-owned Object[]
	// Key: child objectID, Value: true
	// Only these objects need to be checked in isChildNotDominatedDueToObjectArray
	objectsReferencedByCollectionObjectArray map[uint64]bool
}

// precomputeIDEAStyleData precomputes data needed for IDEA-style retained size calculation.
// This implements:
// - Plan A: Precompute Collection-owned Object[] set (eliminates repeated Collection detection)
// - Plan E: Precompute objects referenced by Collection-owned Object[] (skip unnecessary checks)
func (c *IDEAStyleRetainedSizeCalculator) precomputeIDEAStyleData(ctx *RetainedSizeContext) *ideaStylePrecomputedData {
	data := &ideaStylePrecomputedData{
		collectionOwnedObjectArrays:              make(map[uint64]bool),
		objectsReferencedByCollectionObjectArray: make(map[uint64]bool),
	}

	// Step 1: Find Object[] class ID
	objectArrayFound := false
	ctx.ForEachObject(func(objID uint64) {
		if objectArrayFound {
			return
		}
		if classID, ok := ctx.GetObjectClassID(objID); ok {
			if ctx.GetClassName(classID) == "java.lang.Object[]" {
				data.objectArrayClassID = classID
				objectArrayFound = true
			}
		}
	})

	if !objectArrayFound {
		return data
	}

	// Step 2: Find all Object[] instances and check if they are held by Collection classes
	// This is Plan A: precompute collectionOwnedObjectArrays
	ctx.ForEachObject(func(objID uint64) {
		classID, ok := ctx.GetObjectClassID(objID)
		if !ok || classID != data.objectArrayClassID {
			return
		}

		// This is an Object[] - check if it's held by a Collection
		inRefs := ctx.GetIncomingRefs(objID)
		for _, ref := range inRefs {
			holderClassID, ok := ctx.GetObjectClassID(ref.FromObjectID)
			if !ok {
				continue
			}
			holderClassName := ctx.GetClassName(holderClassID)
			if CollectionClasses[holderClassName] {
				data.collectionOwnedObjectArrays[objID] = true
				break
			}
		}
	})

	// Step 3: Find all objects referenced by Collection-owned Object[]
	// This is Plan E: precompute objectsReferencedByCollectionObjectArray
	for objectArrayID := range data.collectionOwnedObjectArrays {
		outRefs := ctx.GetOutgoingRefs(objectArrayID)
		for _, ref := range outRefs {
			if ref.ToObjectID != 0 {
				data.objectsReferencedByCollectionObjectArray[ref.ToObjectID] = true
			}
		}
	}

	return data
}

// ComputeRetainedSizes computes IDEA-style retained sizes.
// Performance optimizations:
// - Plan A: Precomputes Collection-owned Object[] set (eliminates O(E²) repeated Collection detection)
// - Plan E: Precomputes objects referenced by Collection-owned Object[] (skips ~90% of unnecessary checks)
func (c *IDEAStyleRetainedSizeCalculator) ComputeRetainedSizes(
	baseRetainedSizes map[uint64]int64,
	ctx *RetainedSizeContext,
) map[uint64]int64 {
	// Initialize result with base retained sizes
	result := make(map[uint64]int64, len(baseRetainedSizes))
	for k, v := range baseRetainedSizes {
		result[k] = v
	}

	// Precompute data for optimization (Plan A + E)
	precomputed := c.precomputeIDEAStyleData(ctx)

	if precomputed.objectArrayClassID == 0 {
		// No Object[] found, return base sizes
		return result
	}

	// Early exit if no Collection-owned Object[] found
	if len(precomputed.collectionOwnedObjectArrays) == 0 {
		return result
	}

	// Compute additional retained sizes for logically owned objects
	additionalRetained := make(map[uint64]int64)
	processedPairs := make(map[uint64]map[uint64]bool) // parent -> set of children already counted

	ctx.ForEachObject(func(parentID uint64) {
		outRefs := ctx.GetOutgoingRefs(parentID)
		if len(outRefs) == 0 {
			return
		}

		var localProcessed map[uint64]bool // Lazy initialization

		for _, ref := range outRefs {
			childID := ref.ToObjectID
			if _, exists := ctx.GetObjectClassID(childID); !exists {
				continue
			}

			// Skip if already dominated by parent
			if ctx.GetDominator(childID) == parentID {
				continue
			}

			// Plan E optimization: Skip if child is NOT referenced by any Collection-owned Object[]
			// This eliminates ~90% of unnecessary isChildNotDominatedDueToObjectArray calls
			if !precomputed.objectsReferencedByCollectionObjectArray[childID] {
				continue
			}

			// Lazy init processedPairs for this parent
			if localProcessed == nil {
				localProcessed = processedPairs[parentID]
				if localProcessed == nil {
					localProcessed = make(map[uint64]bool)
					processedPairs[parentID] = localProcessed
				}
			}

			// Skip if already processed
			if localProcessed[childID] {
				continue
			}

			// Check if child is not dominated due to Object[] references
			// Plan A optimization: use precomputed collectionOwnedObjectArrays
			if c.isChildNotDominatedDueToObjectArrayOptimized(childID, parentID, precomputed, ctx) {
				localProcessed[childID] = true
				additionalRetained[parentID] += baseRetainedSizes[childID]
			}
		}
	})

	// Apply additional retained sizes (bottom-up to handle nested cases)
	type objDepth struct {
		objID uint64
		depth int
	}
	var objDepths []objDepth

	ctx.ForEachObject(func(objID uint64) {
		depth := 0
		current := objID
		for ctx.GetDominator(current) != ctx.SuperRootID && ctx.GetDominator(current) != 0 && depth < 100 {
			depth++
			current = ctx.GetDominator(current)
		}
		objDepths = append(objDepths, objDepth{objID: objID, depth: depth})
	})

	sort.Slice(objDepths, func(i, j int) bool {
		return objDepths[i].depth > objDepths[j].depth // Deepest first
	})

	// Apply additional retained
	for _, od := range objDepths {
		if additional, exists := additionalRetained[od.objID]; exists && additional > 0 {
			result[od.objID] += additional
		}
	}

	// Debug output
	if ctx.Debugf != nil {
		ideaStyleLarger := 0
		var totalDiff int64
		for objID, ideaSize := range result {
			if baseSize, ok := baseRetainedSizes[objID]; ok && ideaSize > baseSize {
				ideaStyleLarger++
				totalDiff += ideaSize - baseSize
			}
		}
		ctx.Debugf("IDEA-style: %d objects with larger retained size, total difference: %d bytes",
			ideaStyleLarger, totalDiff)
	}

	return result
}

// isChildNotDominatedDueToObjectArrayOptimized is an optimized version that uses precomputed data.
// Complexity reduced from O(inRefs × arrayInRefs) to O(inRefs) by using precomputed collectionOwnedObjectArrays.
func (c *IDEAStyleRetainedSizeCalculator) isChildNotDominatedDueToObjectArrayOptimized(
	childID, parentID uint64,
	precomputed *ideaStylePrecomputedData,
	ctx *RetainedSizeContext,
) bool {
	inRefs := ctx.GetIncomingRefs(childID)

	hasParentRef := false
	hasCollectionObjectArrayRef := false

	for _, ref := range inRefs {
		if ref.FromObjectID == parentID {
			hasParentRef = true
			continue
		}

		// Check if reference is from Object[]
		refClassID, ok := ctx.GetObjectClassID(ref.FromObjectID)
		if !ok || refClassID != precomputed.objectArrayClassID {
			continue
		}

		// Plan A optimization: Use precomputed collectionOwnedObjectArrays
		// This eliminates the inner loop that checks if Object[] is held by Collection
		if precomputed.collectionOwnedObjectArrays[ref.FromObjectID] {
			hasCollectionObjectArrayRef = true
			break
		}
	}

	return hasParentRef && hasCollectionObjectArrayRef
}


