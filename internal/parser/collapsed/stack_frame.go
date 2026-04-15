// Package collapsed implements parsing of collapsed stack format data.
// This is a thin wrapper around github.com/junjiewwang/perf-analysis/perflib/parser/collapsed.
package collapsed

import (
	libcollapsed "github.com/junjiewwang/perf-analysis/perflib/parser/collapsed"
)

// Type aliases - delegate all types to perflib/parser/collapsed.
type (
	// StackFrame represents a single frame in a call stack.
	StackFrame = libcollapsed.StackFrame

	// ThreadInfo represents extracted thread information from a stack trace.
	ThreadInfo = libcollapsed.ThreadInfo
)

// SplitFuncAndModule splits a function name with module information.
var SplitFuncAndModule = libcollapsed.SplitFuncAndModule

// ParseStackFrame parses a raw frame string into a StackFrame.
var ParseStackFrame = libcollapsed.ParseStackFrame

// ExtractThreadInfo extracts thread name and TID from the first frame.
var ExtractThreadInfo = libcollapsed.ExtractThreadInfo

// IsSwapperThread checks if the thread is the swapper (idle) thread.
var IsSwapperThread = libcollapsed.IsSwapperThread

// IsInvalidData checks if the line matches invalid data pattern.
var IsInvalidData = libcollapsed.IsInvalidData

// ParseCallStack parses a semicolon-separated stack string into frames.
var ParseCallStack = libcollapsed.ParseCallStack

// GetStackTopFunction returns the top function (leaf) from the call stack.
var GetStackTopFunction = libcollapsed.GetStackTopFunction

// FramesToCallStackString converts frames to a semicolon-separated string.
var FramesToCallStackString = libcollapsed.FramesToCallStackString

// NormalizeThreadName cleans up thread name by removing TID suffix if present.
var NormalizeThreadName = libcollapsed.NormalizeThreadName
