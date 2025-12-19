// Package profiling provides common utilities for profiling data analysis.
package profiling

import "strings"

// ExtractThreadGroup extracts the thread group name by removing trailing numbers and separators.
// For example: "grpc-nio-worker-1" -> "grpc-nio-worker"
func ExtractThreadGroup(threadName string) string {
	name := threadName
	for len(name) > 0 {
		lastChar := name[len(name)-1]
		if lastChar >= '0' && lastChar <= '9' {
			name = name[:len(name)-1]
		} else if lastChar == '-' || lastChar == '_' || lastChar == '#' {
			name = name[:len(name)-1]
		} else {
			break
		}
	}
	if name == "" {
		return threadName
	}
	return name
}

// IsSwapperThread checks if the thread name indicates a swapper (idle) thread.
func IsSwapperThread(name string) bool {
	return name == "swapper" || strings.HasPrefix(name, "swapper/") ||
		name == "[swapper]" || strings.HasPrefix(name, "[swapper/")
}

// SplitFuncAndModule splits a frame into function name and module.
// Frame format: "funcName(moduleName)" or just "funcName"
func SplitFuncAndModule(frame string) (function, module string) {
	lastParen := strings.LastIndex(frame, "(")
	if lastParen == -1 || !strings.HasSuffix(frame, ")") {
		return frame, ""
	}
	function = frame[:lastParen]
	module = frame[lastParen+1 : len(frame)-1]
	return function, module
}

// StackToString converts a call stack to a semicolon-separated string.
func StackToString(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	return strings.Join(stack, ";")
}

// StringToStack converts a semicolon-separated string back to a call stack.
func StringToStack(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ";")
}
