// Package hprof provides parsing functionality for Java HPROF heap dump files.
//
// This file previously contained the dominator tree computation logic.
// All functionality has been moved to perflib/parser/hprof.
// Methods on ReferenceGraph (ComputeDominatorTree, SetRetainedSizeStrategy, etc.)
// are automatically available through the ReferenceGraph type alias.
package hprof
