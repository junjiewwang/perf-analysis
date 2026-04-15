// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"fmt"
	"sort"
	"strings"
)

// =============================================================================
// Core Types and Interfaces
// =============================================================================

// RetainedSizeAnalyzer analyzes retained size discrepancies between different calculation methods.
// It follows the Strategy pattern for extensibility and supports pluggable analysis strategies.
type RetainedSizeAnalyzer struct {
	refGraph   *ReferenceGraph
	config     *AnalyzerConfig
	strategies []AnalysisStrategy
}

// AnalyzerConfig holds configuration for the analyzer.
type AnalyzerConfig struct {
	MaxInstances       int  // Maximum instances to analyze per class
	MaxFieldsToShow    int  // Maximum fields to display in output
	MaxIncomingRefs    int  // Maximum incoming refs to show per field
	MaxArraysToShow    int  // Maximum Object[] arrays to show
	EnableDebugOutput  bool // Whether to output debug information
}

// DefaultAnalyzerConfig returns the default configuration.
func DefaultAnalyzerConfig() *AnalyzerConfig {
	return &AnalyzerConfig{
		MaxInstances:      5,
		MaxFieldsToShow:   15,
		MaxIncomingRefs:   5,
		MaxArraysToShow:   10,
		EnableDebugOutput: true,
	}
}

// AnalysisStrategy defines the interface for pluggable analysis strategies.
// This allows extending the analyzer with new analysis types without modifying existing code.
type AnalysisStrategy interface {
	// Name returns the strategy name for identification.
	Name() string
	// Analyze performs the analysis on the given instance.
	Analyze(ctx *InstanceAnalysisContext) *StrategyResult
}

// StrategyResult holds the result of a strategy analysis.
type StrategyResult struct {
	StrategyName string
	Findings     []Finding
	Metrics      map[string]int64
}

// Finding represents a single finding from the analysis.
type Finding struct {
	Level   FindingLevel
	Message string
	Details map[string]interface{}
}

// FindingLevel indicates the severity of a finding.
type FindingLevel int

const (
	FindingInfo FindingLevel = iota
	FindingWarning
	FindingCritical
)

// =============================================================================
// Analysis Context - Holds all data needed for analysis
// =============================================================================

// InstanceAnalysisContext holds all the analysis data for a single instance.
// This is the central data structure passed to all analysis strategies.
type InstanceAnalysisContext struct {
	// Basic instance info
	ObjectID    uint64
	ClassName   string
	ShallowSize int64
	RetainedSize int64
	Dominator   uint64
	IsReachable bool
	IsGCRoot    bool

	// Field analysis results
	Fields              []FieldAnalysisResult
	ChildrenRetainedSum int64
	DominatedRetained   int64
	NotDominatedRetained int64

	// Object[] reference analysis
	ObjectArrayRefs     map[uint64]*ObjectArrayRefInfo
	TotalViaObjectArray int64
	ChildrenWithArrayRef int

	// Holder type statistics
	HolderStats []HolderTypeStats

	// Reference to the graph for additional queries
	refGraph *ReferenceGraph
}

// FieldAnalysisResult holds analysis result for a single field.
type FieldAnalysisResult struct {
	FieldName      string
	RefID          uint64
	RefClassName   string
	ShallowSize    int64
	RetainedSize   int64
	DominatorID    uint64
	DominatorClass string
	IsDominated    bool
}

// ObjectArrayRefInfo holds information about Object[] references.
type ObjectArrayRefInfo struct {
	ArrayID        uint64
	ArrayRetained  int64
	Children       []uint64
	ChildrenRetained int64
	HolderClass    string
}

// HolderTypeStats holds statistics grouped by holder type.
type HolderTypeStats struct {
	HolderClass   string
	ArrayCount    int
	TotalChildren int
	TotalRetained int64
}

// =============================================================================
// Analysis Result - Final output structure
// =============================================================================

// RetainedSizeAnalysisResult holds the complete analysis result.
type RetainedSizeAnalysisResult struct {
	ClassName        string
	InstanceCount    int
	InstanceResults  []*InstanceAnalysisResult
	StrategyResults  map[string][]*StrategyResult
}

// InstanceAnalysisResult holds analysis result for a single instance.
type InstanceAnalysisResult struct {
	ObjectID         uint64
	ShallowSize      int64
	RetainedSize     int64
	DominatedRetained int64
	ViaObjectArray   int64
	PotentialRetained int64
	HolderBreakdown  map[string]int64
	ScenarioResults  map[string]int64
}

// =============================================================================
// Constructor and Main Entry Point
// =============================================================================

// NewRetainedSizeAnalyzer creates a new analyzer with default configuration.
func NewRetainedSizeAnalyzer(refGraph *ReferenceGraph) *RetainedSizeAnalyzer {
	return NewRetainedSizeAnalyzerWithConfig(refGraph, DefaultAnalyzerConfig())
}

// NewRetainedSizeAnalyzerWithConfig creates a new analyzer with custom configuration.
func NewRetainedSizeAnalyzerWithConfig(refGraph *ReferenceGraph, config *AnalyzerConfig) *RetainedSizeAnalyzer {
	analyzer := &RetainedSizeAnalyzer{
		refGraph: refGraph,
		config:   config,
	}
	// Register default strategies
	analyzer.RegisterStrategy(&ObjectArrayAnalysisStrategy{})
	analyzer.RegisterStrategy(&HolderTypeAnalysisStrategy{})
	analyzer.RegisterStrategy(&ScenarioComparisonStrategy{})
	return analyzer
}

// RegisterStrategy adds a new analysis strategy.
func (a *RetainedSizeAnalyzer) RegisterStrategy(strategy AnalysisStrategy) {
	a.strategies = append(a.strategies, strategy)
}

// =============================================================================
// Main Analysis Methods
// =============================================================================

// AnalyzeClass analyzes all instances of a class and returns structured results.
func (a *RetainedSizeAnalyzer) AnalyzeClass(className string) *RetainedSizeAnalysisResult {
	if a.refGraph == nil {
		return nil
	}

	// Ensure dominator tree is computed
	a.refGraph.ComputeDominatorTree()
	a.refGraph.buildClassToObjectsIndex()

	// Find all instances
	instances := a.findInstances(className)
	if len(instances) == 0 {
		a.debugf("Class '%s' not found", className)
		return nil
	}

	// Sort by retained size
	sort.Slice(instances, func(i, j int) bool {
		return a.refGraph.retainedSizes[instances[i]] > a.refGraph.retainedSizes[instances[j]]
	})

	// Limit instances
	if len(instances) > a.config.MaxInstances {
		instances = instances[:a.config.MaxInstances]
	}

	result := &RetainedSizeAnalysisResult{
		ClassName:       className,
		InstanceCount:   len(instances),
		InstanceResults: make([]*InstanceAnalysisResult, 0, len(instances)),
		StrategyResults: make(map[string][]*StrategyResult),
	}

	// Analyze each instance
	for idx, objID := range instances {
		ctx := a.buildAnalysisContext(objID, className)
		instanceResult := a.analyzeInstance(ctx, idx+1)
		result.InstanceResults = append(result.InstanceResults, instanceResult)

		// Run all strategies
		for _, strategy := range a.strategies {
			strategyResult := strategy.Analyze(ctx)
			result.StrategyResults[strategy.Name()] = append(
				result.StrategyResults[strategy.Name()],
				strategyResult,
			)
		}
	}

	return result
}

// AnalyzeClassWithDebug analyzes a class and outputs debug information.
// This is the main entry point for debugging retained size discrepancies.
func (a *RetainedSizeAnalyzer) AnalyzeClassWithDebug(className string) {
	if a.refGraph == nil {
		return
	}

	a.debugf("\n=== DEBUG: Analyzing %s ===", className)
	a.debugf("Total classNames in refGraph: %d", len(a.refGraph.classNames))
	a.debugf("Total objectClass in refGraph: %d", len(a.refGraph.objectClass))

	// Ensure dominator tree is computed
	a.refGraph.ComputeDominatorTree()
	a.refGraph.buildClassToObjectsIndex()
	a.debugf("classToObjects has %d entries", len(a.refGraph.classToObjects))

	// Find instances and log similar classes
	instances := a.findInstancesWithDebug(className)
	if len(instances) == 0 {
		a.debugf("DEBUG: Class '%s' not found in classNames map", className)
		return
	}

	a.debugf("Found %d instances of '%s'", len(instances), className)

	// Sort by retained size
	sort.Slice(instances, func(i, j int) bool {
		return a.refGraph.retainedSizes[instances[i]] > a.refGraph.retainedSizes[instances[j]]
	})

	// Limit instances
	if len(instances) > a.config.MaxInstances {
		instances = instances[:a.config.MaxInstances]
	}

	// Analyze each instance with detailed output
	for idx, objID := range instances {
		ctx := a.buildAnalysisContext(objID, className)
		a.outputDetailedInstanceDebug(ctx, idx+1)
	}

	// Output IDEA comparison for first instance
	if len(instances) > 0 {
		a.outputIDEAComparisonDetailed(instances[0])
	}

	// Output IDEA-style retained size comparison
	a.PrintIDEAStyleComparison(className)
}

// findInstancesWithDebug finds instances and logs debug info about similar classes.
func (a *RetainedSizeAnalyzer) findInstancesWithDebug(className string) []uint64 {
	var instances []uint64
	var matchingClassIDs []uint64

	for classID, name := range a.refGraph.classNames {
		if name == className {
			matchingClassIDs = append(matchingClassIDs, classID)
			classInstances := a.refGraph.classToObjects[classID]
			a.debugf("  ClassID=0x%x has %d instances", classID, len(classInstances))
			instances = append(instances, classInstances...)
		}
		// Log similar class names for debugging (configurable pattern)
		if a.containsAny(name, []string{"ArthasClassloader", "arthas.agent", "ClassLoader"}) {
			classInstances := a.refGraph.classToObjects[classID]
			if len(classInstances) > 0 {
				a.debugf("  Similar class: classID=0x%x, name=%s, instances=%d", classID, name, len(classInstances))
			}
		}
	}

	a.debugf("Found %d matching classIDs for '%s', total instances: %d", 
		len(matchingClassIDs), className, len(instances))

	return instances
}

// containsAny checks if s contains any of the patterns.
func (a *RetainedSizeAnalyzer) containsAny(s string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(s, pattern) {
			return true
		}
	}
	return false
}

// outputDetailedInstanceDebug outputs detailed debug information for an instance.
func (a *RetainedSizeAnalyzer) outputDetailedInstanceDebug(ctx *InstanceAnalysisContext, index int) {
	a.debugf("\n--- Instance #%d: objID=0x%x ---", index, ctx.ObjectID)
	a.debugf("  Shallow Size: %d bytes", ctx.ShallowSize)
	a.debugf("  Retained Size: %d bytes (%.2f KB)", ctx.RetainedSize, float64(ctx.RetainedSize)/1024)
	a.debugf("  Is Reachable: %v", ctx.IsReachable)
	a.debugf("  Is GC Root: %v", ctx.IsGCRoot)

	// Output dominator info
	if ctx.Dominator == superRootID {
		a.debugf("  Dominator: <super root>")
	} else {
		domClass := a.refGraph.objectClass[ctx.Dominator]
		domClassName := a.refGraph.GetClassName(domClass)
		a.debugf("  Dominator: 0x%x (%s)", ctx.Dominator, domClassName)
	}

	// Output fields
	a.debugf("  Outgoing References (%d):", len(ctx.Fields))
	for i, field := range ctx.Fields {
		if i >= a.config.MaxFieldsToShow {
			a.debugf("    ... and %d more fields", len(ctx.Fields)-a.config.MaxFieldsToShow)
			break
		}
		dominatedMark := ""
		if field.IsDominated {
			dominatedMark = " [DOMINATED BY THIS]"
		}
		a.debugf("    [%d] %s -> %s (shallow=%d, retained=%d, dom=%s)%s",
			i+1, field.FieldName, field.RefClassName, field.ShallowSize, field.RetainedSize, field.DominatorClass, dominatedMark)
	}

	// Output summary
	a.debugf("\n  Summary:")
	a.debugf("    Children total retained: %d bytes (%.2f KB)", ctx.ChildrenRetainedSum, float64(ctx.ChildrenRetainedSum)/1024)
	a.debugf("    Children dominated by this: %d bytes (%.2f KB)", ctx.DominatedRetained, float64(ctx.DominatedRetained)/1024)
	a.debugf("    Children NOT dominated by this: %d bytes (%.2f KB)", ctx.NotDominatedRetained, float64(ctx.NotDominatedRetained)/1024)
	expectedRetained := ctx.ShallowSize + ctx.DominatedRetained
	a.debugf("    Expected retained (shallow + dominated children): %d bytes (%.2f KB)",
		expectedRetained, float64(expectedRetained)/1024)
	a.debugf("    Actual retained: %d bytes (%.2f KB)", ctx.RetainedSize, float64(ctx.RetainedSize)/1024)

	// Output why children are not dominated
	a.outputNotDominatedAnalysis(ctx)

	// Output Object[] analysis
	a.outputObjectArrayAnalysis(ctx)

	// Output holder type statistics
	a.outputHolderTypeStats(ctx)

	// Output scenario comparisons
	a.outputScenarioComparisons(ctx)

	// Output potential IDEA-style calculation
	a.outputPotentialIDEACalculation(ctx)
}

// outputNotDominatedAnalysis outputs analysis of why children are not dominated.
func (a *RetainedSizeAnalyzer) outputNotDominatedAnalysis(ctx *InstanceAnalysisContext) {
	a.debugf("\n  Analyzing why children are not dominated by this ClassLoader:")
	
	count := 0
	for _, field := range ctx.Fields {
		if count >= 10 {
			break
		}
		if field.IsDominated {
			continue
		}
		count++

		inRefs := a.refGraph.incomingRefs[field.RefID]
		a.debugf("    Field '%s' (%s) has %d incoming refs:", field.FieldName, field.RefClassName, len(inRefs))
		
		for j, inRef := range inRefs {
			if j >= a.config.MaxIncomingRefs {
				a.debugf("      ... and %d more incoming refs", len(inRefs)-a.config.MaxIncomingRefs)
				break
			}
			inRefClassID := a.refGraph.objectClass[inRef.FromObjectID]
			inRefClassName := a.refGraph.GetClassName(inRefClassID)
			
			fromUs := ""
			if inRef.FromObjectID == ctx.ObjectID {
				fromUs = " [FROM THIS CLASSLOADER]"
			}
			isClassObj := ""
			if a.refGraph.classObjectIDs[inRef.FromObjectID] {
				isClassObj = " [IS CLASS OBJECT]"
			}
			a.debugf("      <- 0x%x (%s) via '%s'%s%s",
				inRef.FromObjectID, inRefClassName, inRef.FieldName, fromUs, isClassObj)
		}
	}
}

// outputObjectArrayAnalysis outputs Object[] reference analysis.
func (a *RetainedSizeAnalyzer) outputObjectArrayAnalysis(ctx *InstanceAnalysisContext) {
	a.debugf("\n  === Analyzing java.lang.Object[] incoming references ===")
	a.debugf("  Children with Object[] incoming refs: %d", ctx.ChildrenWithArrayRef)
	a.debugf("  Total retained size of these children: %d bytes (%.2f KB, %.2f MB)",
		ctx.TotalViaObjectArray, float64(ctx.TotalViaObjectArray)/1024, float64(ctx.TotalViaObjectArray)/(1024*1024))

	// Output Object[] arrays details
	a.debugf("\n  Object[] arrays holding references to ClassLoader's children:")

	// Sort arrays by children retained
	type arrayInfo struct {
		info    *ObjectArrayRefInfo
		holders []string
	}
	var sortedArrays []arrayInfo
	for _, info := range ctx.ObjectArrayRefs {
		// Get holders
		arrayInRefs := a.refGraph.incomingRefs[info.ArrayID]
		var holders []string
		for j, inRef := range arrayInRefs {
			if j >= 3 {
				holders = append(holders, "...")
				break
			}
			holderClassID := a.refGraph.objectClass[inRef.FromObjectID]
			holders = append(holders, a.refGraph.GetClassName(holderClassID))
		}
		sortedArrays = append(sortedArrays, arrayInfo{info: info, holders: holders})
	}
	sort.Slice(sortedArrays, func(i, j int) bool {
		return sortedArrays[i].info.ChildrenRetained > sortedArrays[j].info.ChildrenRetained
	})

	for i, arr := range sortedArrays {
		if i >= a.config.MaxArraysToShow {
			a.debugf("    ... and %d more Object[] arrays", len(sortedArrays)-a.config.MaxArraysToShow)
			break
		}
		a.debugf("    Object[] 0x%x: holds %d children (retained=%d bytes, %.2f KB), held by: %v",
			arr.info.ArrayID, len(arr.info.Children), arr.info.ChildrenRetained, 
			float64(arr.info.ChildrenRetained)/1024, arr.holders)
	}
}

// outputHolderTypeStats outputs statistics grouped by holder type.
func (a *RetainedSizeAnalyzer) outputHolderTypeStats(ctx *InstanceAnalysisContext) {
	a.debugf("\n  === Statistics grouped by Object[] holder type ===")
	a.debugf("  %-50s | %8s | %10s | %15s | %10s", "Holder Class", "Arrays", "Children", "Retained (bytes)", "Retained")
	a.debugf("  %s", strings.Repeat("-", 100))

	for _, stats := range ctx.HolderStats {
		retainedStr := FormatBytes(stats.TotalRetained)
		a.debugf("  %-50s | %8d | %10d | %15d | %10s",
			stats.HolderClass, stats.ArrayCount, stats.TotalChildren, stats.TotalRetained, retainedStr)
	}
}

// outputScenarioComparisons outputs scenario comparison results.
func (a *RetainedSizeAnalyzer) outputScenarioComparisons(ctx *InstanceAnalysisContext) {
	a.debugf("\n  === Impact analysis by holder type ===")

	// Calculate breakdown
	var arrayListRetained, identityHashMapRetained, otherRetained int64
	for _, stats := range ctx.HolderStats {
		if strings.Contains(stats.HolderClass, "ArrayList") {
			arrayListRetained += stats.TotalRetained
		} else if strings.Contains(stats.HolderClass, "IdentityHashMap") {
			identityHashMapRetained += stats.TotalRetained
		} else {
			otherRetained += stats.TotalRetained
		}
	}

	a.debugf("  Breakdown by holder type:")
	a.debugf("    Via ArrayList:       %d bytes (%.2f KB, %.2f MB)",
		arrayListRetained, float64(arrayListRetained)/1024, float64(arrayListRetained)/(1024*1024))
	a.debugf("    Via IdentityHashMap: %d bytes (%.2f KB, %.2f MB)",
		identityHashMapRetained, float64(identityHashMapRetained)/1024, float64(identityHashMapRetained)/(1024*1024))
	a.debugf("    Via Other:           %d bytes (%.2f KB, %.2f MB)",
		otherRetained, float64(otherRetained)/1024, float64(otherRetained)/(1024*1024))

	// Scenario comparisons
	a.debugf("\n  Scenario comparisons:")
	baseRetained := ctx.ShallowSize + ctx.DominatedRetained

	scenario1 := baseRetained + arrayListRetained
	a.debugf("    Scenario 1 (ignore only ArrayList Object[]): %d bytes (%.2f MB)",
		scenario1, float64(scenario1)/(1024*1024))

	scenario2 := baseRetained + arrayListRetained + identityHashMapRetained
	a.debugf("    Scenario 2 (ignore ArrayList + IdentityHashMap): %d bytes (%.2f MB)",
		scenario2, float64(scenario2)/(1024*1024))

	scenario3 := baseRetained + ctx.TotalViaObjectArray
	a.debugf("    Scenario 3 (ignore ALL Object[] refs): %d bytes (%.2f MB)",
		scenario3, float64(scenario3)/(1024*1024))

	// Difference analysis
	a.debugf("\n  Difference analysis:")
	if ctx.TotalViaObjectArray > 0 {
		identityPct := float64(identityHashMapRetained) / float64(ctx.TotalViaObjectArray) * 100
		arrayListPct := float64(arrayListRetained) / float64(ctx.TotalViaObjectArray) * 100
		a.debugf("    IdentityHashMap contribution: %d bytes (%.2f KB) = %.4f%% of total",
			identityHashMapRetained, float64(identityHashMapRetained)/1024, identityPct)
		a.debugf("    ArrayList contribution: %d bytes (%.2f MB) = %.2f%% of total",
			arrayListRetained, float64(arrayListRetained)/(1024*1024), arrayListPct)
	}
}

// outputPotentialIDEACalculation outputs potential IDEA-style calculation.
func (a *RetainedSizeAnalyzer) outputPotentialIDEACalculation(ctx *InstanceAnalysisContext) {
	a.debugf("\n  === Potential IDEA-style calculation ===")
	a.debugf("  If we ignore Object[] refs and count children as dominated by ClassLoader:")
	
	potentialRetained := ctx.ShallowSize + ctx.DominatedRetained + ctx.TotalViaObjectArray
	a.debugf("    Shallow: %d", ctx.ShallowSize)
	a.debugf("    + Already dominated children: %d", ctx.DominatedRetained)
	a.debugf("    + Children via Object[] (would be dominated): %d", ctx.TotalViaObjectArray)
	a.debugf("    = Potential retained: %d bytes (%.2f KB, %.2f MB)",
		potentialRetained, float64(potentialRetained)/1024, float64(potentialRetained)/(1024*1024))
}

// outputIDEAComparisonDetailed outputs detailed IDEA comparison.
func (a *RetainedSizeAnalyzer) outputIDEAComparisonDetailed(objID uint64) {
	outRefs := a.refGraph.outgoingRefs[objID]
	var totalChildrenRetained int64
	for _, ref := range outRefs {
		totalChildrenRetained += a.refGraph.retainedSizes[ref.ToObjectID]
	}

	a.debugf("\n=== IDEA Comparison ===")
	a.debugf("If IDEA shows retained = sum of children's retained + shallow:")
	shallowSize := a.refGraph.objectSize[objID]
	a.debugf("  Expected: %d + %d = %d bytes (%.2f MB)",
		shallowSize, totalChildrenRetained,
		shallowSize+totalChildrenRetained,
		float64(shallowSize+totalChildrenRetained)/(1024*1024))
	a.debugf("  Actual (our calculation): %d bytes (%.2f KB)",
		a.refGraph.retainedSizes[objID], float64(a.refGraph.retainedSizes[objID])/1024)
}

// =============================================================================
// Instance Analysis
// =============================================================================

// buildAnalysisContext builds the analysis context for an instance.
func (a *RetainedSizeAnalyzer) buildAnalysisContext(objID uint64, className string) *InstanceAnalysisContext {
	ctx := &InstanceAnalysisContext{
		ObjectID:        objID,
		ClassName:       className,
		ShallowSize:     a.refGraph.objectSize[objID],
		RetainedSize:    a.refGraph.retainedSizes[objID],
		Dominator:       a.refGraph.dominators[objID],
		IsReachable:     a.refGraph.IsObjectReachable(objID),
		IsGCRoot:        a.refGraph.IsGCRoot(objID),
		ObjectArrayRefs: make(map[uint64]*ObjectArrayRefInfo),
		refGraph:        a.refGraph,
	}

	// Analyze fields
	a.analyzeFields(ctx)

	// Analyze Object[] references
	a.analyzeObjectArrayRefs(ctx)

	// Calculate holder statistics
	a.calculateHolderStats(ctx)

	return ctx
}

// analyzeFields analyzes all outgoing references (fields) of an instance.
func (a *RetainedSizeAnalyzer) analyzeFields(ctx *InstanceAnalysisContext) {
	outRefs := a.refGraph.outgoingRefs[ctx.ObjectID]
	ctx.Fields = make([]FieldAnalysisResult, 0, len(outRefs))

	for _, ref := range outRefs {
		refClassID := a.refGraph.objectClass[ref.ToObjectID]
		refClassName := a.refGraph.GetClassName(refClassID)
		refRetained := a.refGraph.retainedSizes[ref.ToObjectID]
		refDom := a.refGraph.dominators[ref.ToObjectID]

		isDominated := refDom == ctx.ObjectID
		domClass := a.getDominatorClassName(refDom, ctx.ObjectID)

		field := FieldAnalysisResult{
			FieldName:      ref.FieldName,
			RefID:          ref.ToObjectID,
			RefClassName:   refClassName,
			ShallowSize:    a.refGraph.objectSize[ref.ToObjectID],
			RetainedSize:   refRetained,
			DominatorID:    refDom,
			DominatorClass: domClass,
			IsDominated:    isDominated,
		}

		ctx.Fields = append(ctx.Fields, field)
		ctx.ChildrenRetainedSum += refRetained

		if isDominated {
			ctx.DominatedRetained += refRetained
		} else {
			ctx.NotDominatedRetained += refRetained
		}
	}

	// Sort by retained size
	sort.Slice(ctx.Fields, func(i, j int) bool {
		return ctx.Fields[i].RetainedSize > ctx.Fields[j].RetainedSize
	})
}

// analyzeObjectArrayRefs analyzes Object[] references for non-dominated children.
func (a *RetainedSizeAnalyzer) analyzeObjectArrayRefs(ctx *InstanceAnalysisContext) {
	for _, field := range ctx.Fields {
		if field.IsDominated {
			continue
		}

		// Check incoming refs for Object[]
		inRefs := a.refGraph.incomingRefs[field.RefID]
		for _, inRef := range inRefs {
			if inRef.FromObjectID == ctx.ObjectID {
				continue
			}

			inRefClassID := a.refGraph.objectClass[inRef.FromObjectID]
			inRefClassName := a.refGraph.GetClassName(inRefClassID)

			if inRefClassName == "java.lang.Object[]" {
				ctx.ChildrenWithArrayRef++
				ctx.TotalViaObjectArray += field.RetainedSize

				arrayID := inRef.FromObjectID
				if _, exists := ctx.ObjectArrayRefs[arrayID]; !exists {
					holderClass := a.getArrayHolderClass(arrayID)
					ctx.ObjectArrayRefs[arrayID] = &ObjectArrayRefInfo{
						ArrayID:       arrayID,
						ArrayRetained: a.refGraph.retainedSizes[arrayID],
						HolderClass:   holderClass,
					}
				}
				ctx.ObjectArrayRefs[arrayID].Children = append(
					ctx.ObjectArrayRefs[arrayID].Children,
					field.RefID,
				)
				ctx.ObjectArrayRefs[arrayID].ChildrenRetained += field.RetainedSize
				break // Only count once per child
			}
		}
	}
}

// calculateHolderStats calculates statistics grouped by holder type.
func (a *RetainedSizeAnalyzer) calculateHolderStats(ctx *InstanceAnalysisContext) {
	statsMap := make(map[string]*HolderTypeStats)

	for _, info := range ctx.ObjectArrayRefs {
		stats, exists := statsMap[info.HolderClass]
		if !exists {
			stats = &HolderTypeStats{HolderClass: info.HolderClass}
			statsMap[info.HolderClass] = stats
		}
		stats.ArrayCount++
		stats.TotalChildren += len(info.Children)
		stats.TotalRetained += info.ChildrenRetained
	}

	// Convert to sorted slice
	ctx.HolderStats = make([]HolderTypeStats, 0, len(statsMap))
	for _, stats := range statsMap {
		ctx.HolderStats = append(ctx.HolderStats, *stats)
	}
	sort.Slice(ctx.HolderStats, func(i, j int) bool {
		return ctx.HolderStats[i].TotalRetained > ctx.HolderStats[j].TotalRetained
	})
}

// analyzeInstance creates the instance result from context.
func (a *RetainedSizeAnalyzer) analyzeInstance(ctx *InstanceAnalysisContext, index int) *InstanceAnalysisResult {
	result := &InstanceAnalysisResult{
		ObjectID:          ctx.ObjectID,
		ShallowSize:       ctx.ShallowSize,
		RetainedSize:      ctx.RetainedSize,
		DominatedRetained: ctx.DominatedRetained,
		ViaObjectArray:    ctx.TotalViaObjectArray,
		PotentialRetained: ctx.ShallowSize + ctx.DominatedRetained + ctx.TotalViaObjectArray,
		HolderBreakdown:   make(map[string]int64),
		ScenarioResults:   make(map[string]int64),
	}

	// Calculate holder breakdown
	for _, stats := range ctx.HolderStats {
		result.HolderBreakdown[stats.HolderClass] = stats.TotalRetained
	}

	// Calculate scenario results
	baseRetained := ctx.ShallowSize + ctx.DominatedRetained
	var arrayListRetained, identityHashMapRetained, otherRetained int64

	for _, stats := range ctx.HolderStats {
		if strings.Contains(stats.HolderClass, "ArrayList") {
			arrayListRetained += stats.TotalRetained
		} else if strings.Contains(stats.HolderClass, "IdentityHashMap") {
			identityHashMapRetained += stats.TotalRetained
		} else {
			otherRetained += stats.TotalRetained
		}
	}

	result.ScenarioResults["base"] = baseRetained
	result.ScenarioResults["ignore_arraylist"] = baseRetained + arrayListRetained
	result.ScenarioResults["ignore_arraylist_identityhashmap"] = baseRetained + arrayListRetained + identityHashMapRetained
	result.ScenarioResults["ignore_all_object_array"] = baseRetained + ctx.TotalViaObjectArray

	return result
}

// =============================================================================
// Helper Methods
// =============================================================================

// findInstances finds all instances of a class by name.
func (a *RetainedSizeAnalyzer) findInstances(className string) []uint64 {
	var instances []uint64
	for classID, name := range a.refGraph.classNames {
		if name == className {
			instances = append(instances, a.refGraph.classToObjects[classID]...)
		}
	}
	return instances
}

// getDominatorClassName returns the class name of a dominator.
func (a *RetainedSizeAnalyzer) getDominatorClassName(domID, selfID uint64) string {
	if domID == selfID {
		return "(this object)"
	}
	if domID == superRootID {
		return "<super root>"
	}
	domClassID := a.refGraph.objectClass[domID]
	return a.refGraph.GetClassName(domClassID)
}

// getArrayHolderClass returns the class that holds an Object[] array.
func (a *RetainedSizeAnalyzer) getArrayHolderClass(arrayID uint64) string {
	inRefs := a.refGraph.incomingRefs[arrayID]
	if len(inRefs) > 0 {
		holderClassID := a.refGraph.objectClass[inRefs[0].FromObjectID]
		return a.refGraph.GetClassName(holderClassID)
	}
	return "(unknown)"
}

// debugf outputs debug information if enabled.
func (a *RetainedSizeAnalyzer) debugf(format string, args ...interface{}) {
	if a.config.EnableDebugOutput && a.refGraph != nil {
		a.refGraph.debugf(format, args...)
	}
}

// =============================================================================
// Built-in Analysis Strategies
// =============================================================================

// ObjectArrayAnalysisStrategy analyzes Object[] references.
type ObjectArrayAnalysisStrategy struct{}

func (s *ObjectArrayAnalysisStrategy) Name() string {
	return "object_array_analysis"
}

func (s *ObjectArrayAnalysisStrategy) Analyze(ctx *InstanceAnalysisContext) *StrategyResult {
	result := &StrategyResult{
		StrategyName: s.Name(),
		Metrics: map[string]int64{
			"children_with_array_ref": int64(ctx.ChildrenWithArrayRef),
			"total_via_object_array":  ctx.TotalViaObjectArray,
			"unique_arrays":           int64(len(ctx.ObjectArrayRefs)),
		},
	}

	if ctx.TotalViaObjectArray > 0 {
		result.Findings = append(result.Findings, Finding{
			Level:   FindingInfo,
			Message: fmt.Sprintf("Found %d children referenced via Object[] arrays", ctx.ChildrenWithArrayRef),
			Details: map[string]interface{}{
				"total_retained": ctx.TotalViaObjectArray,
			},
		})
	}

	return result
}

// HolderTypeAnalysisStrategy analyzes holder types.
type HolderTypeAnalysisStrategy struct{}

func (s *HolderTypeAnalysisStrategy) Name() string {
	return "holder_type_analysis"
}

func (s *HolderTypeAnalysisStrategy) Analyze(ctx *InstanceAnalysisContext) *StrategyResult {
	result := &StrategyResult{
		StrategyName: s.Name(),
		Metrics:      make(map[string]int64),
	}

	for _, stats := range ctx.HolderStats {
		result.Metrics[stats.HolderClass] = stats.TotalRetained

		// Add findings for significant holders
		if stats.TotalRetained > 1024*1024 { // > 1MB
			result.Findings = append(result.Findings, Finding{
				Level:   FindingWarning,
				Message: fmt.Sprintf("%s holds significant retained size", stats.HolderClass),
				Details: map[string]interface{}{
					"retained":  stats.TotalRetained,
					"children":  stats.TotalChildren,
					"arrays":    stats.ArrayCount,
				},
			})
		}
	}

	return result
}

// ScenarioComparisonStrategy compares different retained size calculation scenarios.
type ScenarioComparisonStrategy struct{}

func (s *ScenarioComparisonStrategy) Name() string {
	return "scenario_comparison"
}

func (s *ScenarioComparisonStrategy) Analyze(ctx *InstanceAnalysisContext) *StrategyResult {
	baseRetained := ctx.ShallowSize + ctx.DominatedRetained
	var arrayListRetained, identityHashMapRetained int64

	for _, stats := range ctx.HolderStats {
		if strings.Contains(stats.HolderClass, "ArrayList") {
			arrayListRetained += stats.TotalRetained
		} else if strings.Contains(stats.HolderClass, "IdentityHashMap") {
			identityHashMapRetained += stats.TotalRetained
		}
	}

	result := &StrategyResult{
		StrategyName: s.Name(),
		Metrics: map[string]int64{
			"base_retained":                 baseRetained,
			"arraylist_contribution":        arrayListRetained,
			"identityhashmap_contribution":  identityHashMapRetained,
			"scenario_ignore_arraylist":     baseRetained + arrayListRetained,
			"scenario_ignore_all":           baseRetained + ctx.TotalViaObjectArray,
		},
	}

	// Calculate contribution percentages
	if ctx.TotalViaObjectArray > 0 {
		arrayListPct := float64(arrayListRetained) / float64(ctx.TotalViaObjectArray) * 100
		identityHashMapPct := float64(identityHashMapRetained) / float64(ctx.TotalViaObjectArray) * 100

		result.Findings = append(result.Findings, Finding{
			Level:   FindingInfo,
			Message: fmt.Sprintf("ArrayList contributes %.2f%%, IdentityHashMap contributes %.4f%%", arrayListPct, identityHashMapPct),
			Details: map[string]interface{}{
				"arraylist_pct":       arrayListPct,
				"identityhashmap_pct": identityHashMapPct,
			},
		})

		// Recommend ignoring IdentityHashMap if contribution is negligible
		if identityHashMapPct < 1.0 {
			result.Findings = append(result.Findings, Finding{
				Level:   FindingInfo,
				Message: "IdentityHashMap contribution is negligible and can be ignored",
			})
		}
	}

	return result
}

// =============================================================================
// IDEA-Style Retained Size Calculation
// =============================================================================

// IDEAStyleRetainedResult holds the result of IDEA-style retained size calculation.
type IDEAStyleRetainedResult struct {
	ObjectID              uint64
	ClassName             string
	ShallowSize           int64
	StandardRetainedSize  int64 // Standard dominator tree calculation
	IDEAStyleRetainedSize int64 // IDEA-style calculation (includes logically owned objects)
	DominatedRetained     int64 // Objects directly dominated
	ViaObjectArrayRetained int64 // Objects referenced via Object[] but logically owned
	LoadedClassesRetained int64 // For ClassLoaders: retained size of loaded classes
}

// CalculateIDEAStyleRetainedSize calculates retained size in a way that matches IDEA's behavior.
// 
// Key differences from standard dominator tree:
// 1. For ClassLoader objects: includes loaded classes even if they're also referenced by ArrayList
// 2. Ignores "shared" references through collection internal arrays (Object[])
// 3. Treats logical ownership (e.g., ClassLoader -> loaded classes) as domination
//
// This is useful for understanding memory impact from a logical/business perspective.
func (a *RetainedSizeAnalyzer) CalculateIDEAStyleRetainedSize(objID uint64) *IDEAStyleRetainedResult {
	if a.refGraph == nil {
		return nil
	}

	a.refGraph.ComputeDominatorTree()

	classID := a.refGraph.objectClass[objID]
	className := a.refGraph.GetClassName(classID)

	result := &IDEAStyleRetainedResult{
		ObjectID:             objID,
		ClassName:            className,
		ShallowSize:          a.refGraph.objectSize[objID],
		StandardRetainedSize: a.refGraph.retainedSizes[objID],
	}

	// Start with standard retained size
	result.IDEAStyleRetainedSize = result.StandardRetainedSize
	result.DominatedRetained = result.StandardRetainedSize - result.ShallowSize

	// Check if this is a ClassLoader
	isClassLoader := strings.Contains(strings.ToLower(className), "classloader")

	// Analyze outgoing references
	outRefs := a.refGraph.outgoingRefs[objID]
	
	// Track objects that should be "logically" owned by this object
	logicallyOwned := make(map[uint64]bool)
	
	for _, ref := range outRefs {
		refClassID := a.refGraph.objectClass[ref.ToObjectID]
		refClassName := a.refGraph.GetClassName(refClassID)
		refDom := a.refGraph.dominators[ref.ToObjectID]
		
		// Skip if already dominated by this object
		if refDom == objID {
			continue
		}
		
		// For ClassLoaders: loaded classes should be counted
		if isClassLoader && ref.FieldName == "<loaded_class>" {
			// Check if this class is not dominated by us due to Object[] references
			if a.isNotDominatedDueToObjectArray(ref.ToObjectID, objID) {
				logicallyOwned[ref.ToObjectID] = true
				result.LoadedClassesRetained += a.refGraph.retainedSizes[ref.ToObjectID]
			}
		}
		
		// For any object: check if child is only "shared" via collection internals
		if refClassName == "java.lang.Class" || a.isLogicallyOwned(ref.ToObjectID, objID) {
			if !logicallyOwned[ref.ToObjectID] && a.isNotDominatedDueToObjectArray(ref.ToObjectID, objID) {
				logicallyOwned[ref.ToObjectID] = true
				result.ViaObjectArrayRetained += a.refGraph.retainedSizes[ref.ToObjectID]
			}
		}
	}
	
	// Add logically owned objects to IDEA-style retained size
	for childID := range logicallyOwned {
		result.IDEAStyleRetainedSize += a.refGraph.retainedSizes[childID]
	}
	
	return result
}

// isNotDominatedDueToObjectArray checks if an object is not dominated by parent
// because it's also referenced through an Object[] array (typically from ArrayList/HashMap).
func (a *RetainedSizeAnalyzer) isNotDominatedDueToObjectArray(childID, parentID uint64) bool {
	inRefs := a.refGraph.incomingRefs[childID]
	
	hasParentRef := false
	hasObjectArrayRef := false
	
	for _, ref := range inRefs {
		if ref.FromObjectID == parentID {
			hasParentRef = true
			continue
		}
		
		refClassID := a.refGraph.objectClass[ref.FromObjectID]
		refClassName := a.refGraph.GetClassName(refClassID)
		
		if refClassName == "java.lang.Object[]" {
			// Check if the Object[] is held by a collection (ArrayList, HashMap, etc.)
			arrayHolderClass := a.getArrayHolderClass(ref.FromObjectID)
			if a.isCollectionClass(arrayHolderClass) {
				hasObjectArrayRef = true
			}
		}
	}
	
	return hasParentRef && hasObjectArrayRef
}

// isLogicallyOwned checks if a child object is logically owned by the parent.
// This is a heuristic based on field names and object types.
func (a *RetainedSizeAnalyzer) isLogicallyOwned(childID, parentID uint64) bool {
	// Check the reference from parent to child
	outRefs := a.refGraph.outgoingRefs[parentID]
	for _, ref := range outRefs {
		if ref.ToObjectID == childID {
			// Ownership-indicating field names
			ownershipFields := []string{
				"<loaded_class>", "classes", "loadedClasses",
				"children", "elements", "entries", "items",
				"value", "data", "content",
			}
			for _, field := range ownershipFields {
				if ref.FieldName == field {
					return true
				}
			}
		}
	}
	return false
}

// isCollectionClass checks if a class is a Java collection class.
// Delegates to the shared CollectionClasses map.
func (a *RetainedSizeAnalyzer) isCollectionClass(className string) bool {
	return IsCollectionClass(className)
}

// CalculateIDEAStyleForClass calculates IDEA-style retained size for all instances of a class.
func (a *RetainedSizeAnalyzer) CalculateIDEAStyleForClass(className string) []*IDEAStyleRetainedResult {
	if a.refGraph == nil {
		return nil
	}

	a.refGraph.ComputeDominatorTree()
	a.refGraph.buildClassToObjectsIndex()

	instances := a.findInstances(className)
	if len(instances) == 0 {
		return nil
	}

	// Sort by standard retained size
	sort.Slice(instances, func(i, j int) bool {
		return a.refGraph.retainedSizes[instances[i]] > a.refGraph.retainedSizes[instances[j]]
	})

	// Limit to top instances
	maxInstances := a.config.MaxInstances
	if len(instances) > maxInstances {
		instances = instances[:maxInstances]
	}

	results := make([]*IDEAStyleRetainedResult, 0, len(instances))
	for _, objID := range instances {
		result := a.CalculateIDEAStyleRetainedSize(objID)
		if result != nil {
			results = append(results, result)
		}
	}

	return results
}

// PrintIDEAStyleComparison prints a comparison between standard and IDEA-style retained sizes.
func (a *RetainedSizeAnalyzer) PrintIDEAStyleComparison(className string) {
	results := a.CalculateIDEAStyleForClass(className)
	if len(results) == 0 {
		a.debugf("No instances found for class: %s", className)
		return
	}

	a.debugf("\n=== IDEA-Style Retained Size Comparison for %s ===", className)
	a.debugf("%-20s | %-15s | %-15s | %-15s | %-10s", 
		"Object ID", "Standard", "IDEA-Style", "Difference", "Ratio")
	a.debugf("%s", strings.Repeat("-", 85))

	for _, r := range results {
		diff := r.IDEAStyleRetainedSize - r.StandardRetainedSize
		ratio := float64(r.IDEAStyleRetainedSize) / float64(r.StandardRetainedSize)
		
		a.debugf("0x%x | %s | %s | %s | %.2fx",
			r.ObjectID,
			FormatBytes(r.StandardRetainedSize),
			FormatBytes(r.IDEAStyleRetainedSize),
			FormatBytes(diff),
			ratio)
	}

	// Summary
	if len(results) > 0 {
		r := results[0]
		a.debugf("\n=== Breakdown for largest instance (0x%x) ===", r.ObjectID)
		a.debugf("  Shallow Size:           %s", FormatBytes(r.ShallowSize))
		a.debugf("  Dominated Retained:     %s", FormatBytes(r.DominatedRetained))
		a.debugf("  Via Object[] (added):   %s", FormatBytes(r.ViaObjectArrayRetained))
		if r.LoadedClassesRetained > 0 {
			a.debugf("  Loaded Classes (added): %s", FormatBytes(r.LoadedClassesRetained))
		}
		a.debugf("  ---")
		a.debugf("  Standard Retained:      %s", FormatBytes(r.StandardRetainedSize))
		a.debugf("  IDEA-Style Retained:    %s", FormatBytes(r.IDEAStyleRetainedSize))
	}
}

// =============================================================================
// Utility Functions
// =============================================================================

// FormatBytes formats bytes to human-readable string.
// Delegates to the shared FormatBytesSize function.
func FormatBytes(bytes int64) string {
	return FormatBytesSize(bytes)
}
