// Package model provides type aliases for backward compatibility.
// All core analysis types have been migrated to the perflib/model package.
// This file ensures existing code that imports pkg/model continues to work.
package model

import (
	libmodel "github.com/junjiewwang/perf-analysis/perflib/model"
)

// ---- analysis.go types ----

// AnalysisDataType is an alias for the library type.
type AnalysisDataType = libmodel.AnalysisDataType

const (
	DataTypeCPUProfiling   = libmodel.DataTypeCPUProfiling
	DataTypeAllocation     = libmodel.DataTypeAllocation
	DataTypeHeapDump       = libmodel.DataTypeHeapDump
	DataTypeMemoryLeak     = libmodel.DataTypeMemoryLeak
	DataTypeTracing        = libmodel.DataTypeTracing
	DataTypePProfCPU       = libmodel.DataTypePProfCPU
	DataTypePProfHeap      = libmodel.DataTypePProfHeap
	DataTypePProfGoroutine = libmodel.DataTypePProfGoroutine
	DataTypePProfBlock     = libmodel.DataTypePProfBlock
	DataTypePProfMutex     = libmodel.DataTypePProfMutex
	DataTypePProfBatch     = libmodel.DataTypePProfBatch
)

// OutputFile is an alias for the library type.
type OutputFile = libmodel.OutputFile

// TopItem is an alias for the library type.
type TopItem = libmodel.TopItem

// AnalysisData is an alias for the library interface.
type AnalysisData = libmodel.AnalysisData

// ---- cpu.go types ----

// CPUProfilingData is an alias for the library type.
type CPUProfilingData = libmodel.CPUProfilingData

// AllocationData is an alias for the library type.
type AllocationData = libmodel.AllocationData

// MemoryLeakData is an alias for the library type.
type MemoryLeakData = libmodel.MemoryLeakData

// LeakInfo is an alias for the library type.
type LeakInfo = libmodel.LeakInfo

// TracingData is an alias for the library type.
type TracingData = libmodel.TracingData

// ---- heap.go types ----

// HeapClassStats is an alias for the library type.
type HeapClassStats = libmodel.HeapClassStats

// HeapRetainer is an alias for the library type.
type HeapRetainer = libmodel.HeapRetainer

// GCRootPathNode is an alias for the library type.
type GCRootPathNode = libmodel.GCRootPathNode

// GCRootPath is an alias for the library type.
type GCRootPath = libmodel.GCRootPath

// HeapReferenceNode is an alias for the library type.
type HeapReferenceNode = libmodel.HeapReferenceNode

// HeapReferenceEdge is an alias for the library type.
type HeapReferenceEdge = libmodel.HeapReferenceEdge

// HeapReferenceGraph is an alias for the library type.
type HeapReferenceGraph = libmodel.HeapReferenceGraph

// HeapBusinessRetainer is an alias for the library type.
type HeapBusinessRetainer = libmodel.HeapBusinessRetainer

// HeapBiggestObject is an alias for the library type.
type HeapBiggestObject = libmodel.HeapBiggestObject

// HeapObjectField is an alias for the library type.
type HeapObjectField = libmodel.HeapObjectField

// HeapGCRootPath is an alias for the library type.
type HeapGCRootPath = libmodel.HeapGCRootPath

// HeapGCRootPathNode is an alias for the library type.
type HeapGCRootPathNode = libmodel.HeapGCRootPathNode

// HeapGCRootsData is an alias for the library type.
type HeapGCRootsData = libmodel.HeapGCRootsData

// HeapGCRootsSummary is an alias for the library type.
type HeapGCRootsSummary = libmodel.HeapGCRootsSummary

// HeapGCRootClass is an alias for the library type.
type HeapGCRootClass = libmodel.HeapGCRootClass

// HeapGCRootInstance is an alias for the library type.
type HeapGCRootInstance = libmodel.HeapGCRootInstance

// HeapAnalysisData is an alias for the library type.
type HeapAnalysisData = libmodel.HeapAnalysisData

// ---- pprof.go types ----

// PProfTopFunc is an alias for the library type.
type PProfTopFunc = libmodel.PProfTopFunc

// PProfMemoryStats is an alias for the library type.
type PProfMemoryStats = libmodel.PProfMemoryStats

// PProfCPUData is an alias for the library type.
type PProfCPUData = libmodel.PProfCPUData

// PProfHeapSummary is an alias for the library type.
type PProfHeapSummary = libmodel.PProfHeapSummary

// PProfHeapData is an alias for the library type.
type PProfHeapData = libmodel.PProfHeapData

// GoroutineGroup is an alias for the library type.
type GoroutineGroup = libmodel.GoroutineGroup

// PProfGoroutineData is an alias for the library type.
type PProfGoroutineData = libmodel.PProfGoroutineData

// PProfBlockData is an alias for the library type.
type PProfBlockData = libmodel.PProfBlockData

// PProfBatchProfileSet is an alias for the library type.
type PProfBatchProfileSet = libmodel.PProfBatchProfileSet

// PProfLeakReportSummary is an alias for the library type.
type PProfLeakReportSummary = libmodel.PProfLeakReportSummary

// PProfLeakGrowthItem is an alias for the library type.
type PProfLeakGrowthItem = libmodel.PProfLeakGrowthItem

// PProfLeakReport is an alias for the library type.
type PProfLeakReport = libmodel.PProfLeakReport

// PProfBatchData is an alias for the library type.
type PProfBatchData = libmodel.PProfBatchData

// ---- sample.go types ----

// ThreadInfo is an alias for the library type.
type ThreadInfo = libmodel.ThreadInfo

// TopFuncsMap is an alias for the library type.
type TopFuncsMap = libmodel.TopFuncsMap

// TopFuncValue is an alias for the library type.
type TopFuncValue = libmodel.TopFuncValue

// TopFunction is an alias for the library type.
type TopFunction = libmodel.TopFunction

// CallStackInfo is an alias for the library type.
type CallStackInfo = libmodel.CallStackInfo

// Sample is an alias for the library type.
type Sample = libmodel.Sample

// ParseResult is an alias for the library type.
type ParseResult = libmodel.ParseResult

// SuggestionItem is an alias for the library type.
type SuggestionItem = libmodel.SuggestionItem

// ---- enum.go types ----

// Profiler is an alias for the library type.
type Profiler = libmodel.Profiler

const (
	ProfilerPerf          = libmodel.ProfilerPerf
	ProfilerAsyncProfiler = libmodel.ProfilerAsyncProfiler
	ProfilerPProf         = libmodel.ProfilerPProf
	ProfilerHeapDump      = libmodel.ProfilerHeapDump
	ProfilerJeprof        = libmodel.ProfilerJeprof
)

// EventType is an alias for the library type.
type EventType = libmodel.EventType

const (
	EventCPU       = libmodel.EventCPU
	EventAlloc     = libmodel.EventAlloc
	EventHeap      = libmodel.EventHeap
	EventWall      = libmodel.EventWall
	EventLock      = libmodel.EventLock
	EventGoroutine = libmodel.EventGoroutine
	EventBlock     = libmodel.EventBlock
	EventMutex     = libmodel.EventMutex
	EventIO        = libmodel.EventIO
)

// ResourceType is an alias for the library type.
type ResourceType = libmodel.ResourceType

const (
	ResourceCPU         = libmodel.ResourceCPU
	ResourceMemory      = libmodel.ResourceMemory
	ResourceIO          = libmodel.ResourceIO
	ResourceApp         = libmodel.ResourceApp
	ResourceGoroutine   = libmodel.ResourceGoroutine
	ResourceConcurrency = libmodel.ResourceConcurrency
)

// ---- function aliases ----

// MarshalAnalysisData is a wrapper for the library function.
var MarshalAnalysisData = libmodel.MarshalAnalysisData

// UnmarshalAnalysisData is a wrapper for the library function.
var UnmarshalAnalysisData = libmodel.UnmarshalAnalysisData

// AnalysisModeString is a wrapper for the library function.
var AnalysisModeString = libmodel.AnalysisModeString
