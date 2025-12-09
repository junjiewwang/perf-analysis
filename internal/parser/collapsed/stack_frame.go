// Package collapsed implements parsing of collapsed stack format data.
// Collapsed format example: thread_name-pid/tid;func1;func2;func3 count
package collapsed

import (
	"regexp"
	"strconv"
	"strings"
)

// StackFrame represents a single frame in a call stack.
type StackFrame struct {
	Function string `json:"func"`
	Module   string `json:"module,omitempty"`
}

// ThreadInfo represents extracted thread information from a stack trace.
type ThreadInfo struct {
	ThreadName string `json:"thread_name"`
	TID        int    `json:"tid"`
	PID        int    `json:"pid,omitempty"`
}

// APM format regex: [Thread-7 tid=1060369]
var apmFormatRegex = regexp.MustCompile(`^\[(.+)\s+tid=(\d+)\]$`)

// Invalid data pattern: 5_2175795_[002]_83367.826506:-?/10101010
var invalidDataRegex = regexp.MustCompile(`^\d+_\d+_`)

// SplitFuncAndModule splits a function name with module information.
// e.g., "funcName(module)" => ("funcName", "module")
// e.g., "funcName" => ("funcName", "")
func SplitFuncAndModule(funcModule string) (function, module string) {
	// Find the last occurrence of '('
	lastParen := strings.LastIndex(funcModule, "(")
	if lastParen == -1 {
		return funcModule, ""
	}

	// Check if it ends with ')'
	if !strings.HasSuffix(funcModule, ")") {
		return funcModule, ""
	}

	function = funcModule[:lastParen]
	module = funcModule[lastParen+1 : len(funcModule)-1]
	return function, module
}

// ParseStackFrame parses a raw frame string into a StackFrame.
func ParseStackFrame(raw string) *StackFrame {
	function, module := SplitFuncAndModule(raw)
	return &StackFrame{
		Function: function,
		Module:   module,
	}
}

// ExtractThreadInfo extracts thread name and TID from the first frame.
// Supports two formats:
// 1. Standard perf format: "process_name-pid/tid" e.g., "sap1009-?/1088670"
// 2. APM format: "[Thread-7 tid=1060369]"
func ExtractThreadInfo(threadFrame string) *ThreadInfo {
	info := &ThreadInfo{
		ThreadName: threadFrame,
		TID:        -1,
		PID:        -1,
	}

	// Check APM format first: [Thread-7 tid=1060369]
	if strings.HasPrefix(threadFrame, "[") && strings.HasSuffix(threadFrame, "]") {
		matches := apmFormatRegex.FindStringSubmatch(threadFrame)
		if len(matches) == 3 {
			info.ThreadName = matches[1]
			if tid, err := strconv.Atoi(matches[2]); err == nil {
				info.TID = tid
			}
			return info
		}
	}

	// Standard perf format: process_name-pid/tid
	// Extract thread name (before the last '-')
	lastDash := strings.LastIndex(threadFrame, "-")
	if lastDash > 0 {
		info.ThreadName = threadFrame[:lastDash]
	}

	// Extract TID (after the last '/')
	lastSlash := strings.LastIndex(threadFrame, "/")
	if lastSlash > 0 && lastSlash < len(threadFrame)-1 {
		tidStr := threadFrame[lastSlash+1:]
		if tid, err := strconv.Atoi(tidStr); err == nil {
			info.TID = tid
		}
	}

	return info
}

// IsSwapperThread checks if the thread is the swapper (idle) thread.
func IsSwapperThread(threadFrame string) bool {
	return strings.HasPrefix(threadFrame, "swapper-") || threadFrame == "swapper"
}

// IsInvalidData checks if the line matches invalid data pattern.
// e.g., "5_2175795_[002]_83367.826506:-?/10101010"
func IsInvalidData(firstFrame string) bool {
	return invalidDataRegex.MatchString(firstFrame)
}

// ParseCallStack parses a semicolon-separated stack string into frames.
// Returns the thread info and the remaining call stack frames.
func ParseCallStack(stack string) (*ThreadInfo, []*StackFrame) {
	parts := strings.Split(stack, ";")
	if len(parts) == 0 {
		return &ThreadInfo{TID: -1}, nil
	}

	// First part is thread info
	threadInfo := ExtractThreadInfo(parts[0])

	// Skip APM format prefix if present (it appears as first frame after thread)
	startIdx := 1
	if startIdx < len(parts) && apmFormatRegex.MatchString(parts[startIdx]) {
		startIdx++
	}

	// Parse remaining frames
	frames := make([]*StackFrame, 0, len(parts)-startIdx)
	for i := startIdx; i < len(parts); i++ {
		if parts[i] == "" || parts[i] == "[]" {
			continue
		}
		frames = append(frames, ParseStackFrame(parts[i]))
	}

	return threadInfo, frames
}

// GetStackTopFunction returns the top function (leaf) from the call stack.
func GetStackTopFunction(frames []*StackFrame) string {
	if len(frames) == 0 {
		return ""
	}
	return frames[len(frames)-1].Function
}

// FramesToCallStackString converts frames to a semicolon-separated string.
// Used for call stack deduplication and grouping.
func FramesToCallStackString(frames []*StackFrame) string {
	if len(frames) == 0 {
		return ""
	}

	parts := make([]string, len(frames))
	for i, frame := range frames {
		parts[i] = frame.Function
	}
	return strings.Join(parts, ";")
}

// NormalizeThreadName cleans up thread name by removing TID suffix if present.
func NormalizeThreadName(threadFrame string) string {
	info := ExtractThreadInfo(threadFrame)
	return info.ThreadName
}
