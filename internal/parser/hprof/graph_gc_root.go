// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	libhprof "github.com/perf-analysis/perflib/parser/hprof"
)

// ============================================================================
// Type Aliases - delegating to perflib/parser/hprof
// ============================================================================

// GCRootType represents the type of GC root.
type GCRootType = libhprof.GCRootType

// GCRoot represents a garbage collection root.
type GCRoot = libhprof.GCRoot

// PathNode represents a node in a reference path from GC Root to object.
type PathNode = libhprof.PathNode

// GCRootPath represents a path from a GC Root to an object.
type GCRootPath = libhprof.GCRootPath

// GCRootInfo represents aggregated information about a GC root for display.
type GCRootInfo = libhprof.GCRootInfo

// GCRootSummary represents a summary of GC roots grouped by class.
type GCRootSummary = libhprof.GCRootSummary

// ============================================================================
// Constant Aliases
// ============================================================================

const (
	GCRootUnknown      = libhprof.GCRootUnknown
	GCRootJNIGlobal    = libhprof.GCRootJNIGlobal
	GCRootJNILocal     = libhprof.GCRootJNILocal
	GCRootJavaFrame    = libhprof.GCRootJavaFrame
	GCRootNativeStack  = libhprof.GCRootNativeStack
	GCRootStickyClass  = libhprof.GCRootStickyClass
	GCRootThreadBlock  = libhprof.GCRootThreadBlock
	GCRootMonitorUsed  = libhprof.GCRootMonitorUsed
	GCRootThreadObject = libhprof.GCRootThreadObject
)

// Note: All methods on ReferenceGraph (AddGCRoot, IsGCRoot, GetGCRootType,
// FindPathsToGCRoot, FindNonArrayRetainers, GetGCRootsList, GetGCRootsSummary,
// GetRetainedObjectsByGCRoot) are automatically available through the type alias.
