package analyzer

import (
	"fmt"
	"strings"

	"github.com/perf-analysis/pkg/model"
)

// AnalysisMode represents a user-friendly analysis mode.
// It abstracts away the complexity of TaskType + ProfilerType combinations.
type AnalysisMode string

const (
	// ModeJavaCPU analyzes Java CPU hotspots from async-profiler/perf data.
	ModeJavaCPU AnalysisMode = "java-cpu"

	// ModeJavaAlloc analyzes Java memory allocation from async-profiler alloc data.
	ModeJavaAlloc AnalysisMode = "java-alloc"

	// ModeJavaHeap analyzes Java heap dump (HPROF format).
	ModeJavaHeap AnalysisMode = "java-heap"

	// ModeCPU analyzes generic CPU profiling data (collapsed format).
	ModeCPU AnalysisMode = "cpu"

	// ModePProfCPU analyzes Go pprof CPU profile.
	ModePProfCPU AnalysisMode = "pprof-cpu"

	// ModePProfHeap analyzes Go pprof Heap profile.
	ModePProfHeap AnalysisMode = "pprof-heap"

	// ModePProfGoroutine analyzes Go pprof Goroutine profile.
	ModePProfGoroutine AnalysisMode = "pprof-goroutine"

	// ModePProfBlock analyzes Go pprof Block profile.
	ModePProfBlock AnalysisMode = "pprof-block"

	// ModePProfMutex analyzes Go pprof Mutex profile.
	ModePProfMutex AnalysisMode = "pprof-mutex"

	// ModePProfAll analyzes all pprof profiles in a directory.
	ModePProfAll AnalysisMode = "pprof-all"
)

// ModeInfo describes an analysis mode for help and validation.
type ModeInfo struct {
	Mode        AnalysisMode
	Description string
	InputFormat string
	TaskType    model.TaskType
	Profiler    model.ProfilerType
}

// modeRegistry maps mode names to their metadata.
var modeRegistry = map[AnalysisMode]*ModeInfo{
	ModeJavaCPU: {
		Mode:        ModeJavaCPU,
		Description: "Java CPU hotspot analysis (async-profiler/perf)",
		InputFormat: "Collapsed stack format (.collapsed, .data, .txt)",
		TaskType:    model.TaskTypeJava,
		Profiler:    model.ProfilerTypePerf,
	},
	ModeJavaAlloc: {
		Mode:        ModeJavaAlloc,
		Description: "Java memory allocation analysis (async-profiler alloc)",
		InputFormat: "Collapsed stack format (.collapsed, .data, .txt)",
		TaskType:    model.TaskTypeJava,
		Profiler:    model.ProfilerTypeAsyncAlloc,
	},
	ModeJavaHeap: {
		Mode:        ModeJavaHeap,
		Description: "Java heap dump analysis (HPROF)",
		InputFormat: "HPROF binary format (.hprof)",
		TaskType:    model.TaskTypeJavaHeap,
		Profiler:    model.ProfilerTypePerf, // Not used for heap
	},
	ModeCPU: {
		Mode:        ModeCPU,
		Description: "Generic CPU profiling analysis",
		InputFormat: "Collapsed stack format (.collapsed, .data, .txt)",
		TaskType:    model.TaskTypeGeneric,
		Profiler:    model.ProfilerTypePerf,
	},
	ModePProfCPU: {
		Mode:        ModePProfCPU,
		Description: "Go pprof CPU profile analysis",
		InputFormat: "Go pprof format (.pprof, .pb.gz)",
		TaskType:    model.TaskTypePProfCPU,
		Profiler:    model.ProfilerTypePProf,
	},
	ModePProfHeap: {
		Mode:        ModePProfHeap,
		Description: "Go pprof Heap profile analysis",
		InputFormat: "Go pprof format (.pprof, .pb.gz)",
		TaskType:    model.TaskTypePProfHeap,
		Profiler:    model.ProfilerTypePProf,
	},
	ModePProfGoroutine: {
		Mode:        ModePProfGoroutine,
		Description: "Go pprof Goroutine profile analysis",
		InputFormat: "Go pprof format (.pprof, .pb.gz)",
		TaskType:    model.TaskTypePProfGoroutine,
		Profiler:    model.ProfilerTypePProf,
	},
	ModePProfBlock: {
		Mode:        ModePProfBlock,
		Description: "Go pprof Block profile analysis",
		InputFormat: "Go pprof format (.pprof, .pb.gz)",
		TaskType:    model.TaskTypePProfBlock,
		Profiler:    model.ProfilerTypePProf,
	},
	ModePProfMutex: {
		Mode:        ModePProfMutex,
		Description: "Go pprof Mutex profile analysis",
		InputFormat: "Go pprof format (.pprof, .pb.gz)",
		TaskType:    model.TaskTypePProfMutex,
		Profiler:    model.ProfilerTypePProf,
	},
	ModePProfAll: {
		Mode:        ModePProfAll,
		Description: "Batch analysis of all pprof profiles in a directory",
		InputFormat: "Directory containing pprof subdirectories (cpu/, heap/, goroutine/, etc.)",
		TaskType:    model.TaskTypePProfCPU, // Primary type
		Profiler:    model.ProfilerTypePProf,
	},
}

// ParseMode parses a mode string into AnalysisMode.
func ParseMode(s string) (AnalysisMode, error) {
	mode := AnalysisMode(strings.ToLower(strings.TrimSpace(s)))
	if _, ok := modeRegistry[mode]; ok {
		return mode, nil
	}
	return "", fmt.Errorf("unknown analysis mode: %q (valid: %s)", s, ValidModes())
}

// GetModeInfo returns the metadata for a mode.
func GetModeInfo(mode AnalysisMode) (*ModeInfo, bool) {
	info, ok := modeRegistry[mode]
	return info, ok
}

// ValidModes returns a comma-separated list of valid mode names.
func ValidModes() string {
	modes := make([]string, 0, len(modeRegistry))
	for mode := range modeRegistry {
		modes = append(modes, string(mode))
	}
	return strings.Join(modes, ", ")
}

// AllModes returns all registered mode information.
func AllModes() []*ModeInfo {
	result := make([]*ModeInfo, 0, len(modeRegistry))
	// Return in a consistent order
	order := []AnalysisMode{
		ModeJavaCPU, ModeJavaAlloc, ModeJavaHeap, ModeCPU,
		ModePProfCPU, ModePProfHeap, ModePProfGoroutine, ModePProfBlock, ModePProfMutex, ModePProfAll,
	}
	for _, mode := range order {
		if info, ok := modeRegistry[mode]; ok {
			result = append(result, info)
		}
	}
	return result
}

// String returns the string representation of the mode.
func (m AnalysisMode) String() string {
	return string(m)
}

// Info returns the metadata for this mode.
func (m AnalysisMode) Info() *ModeInfo {
	info, _ := GetModeInfo(m)
	return info
}

// ToTaskType converts the mode to TaskType.
func (m AnalysisMode) ToTaskType() model.TaskType {
	if info := m.Info(); info != nil {
		return info.TaskType
	}
	return model.TaskTypeGeneric
}

// ToProfilerType converts the mode to ProfilerType.
func (m AnalysisMode) ToProfilerType() model.ProfilerType {
	if info := m.Info(); info != nil {
		return info.Profiler
	}
	return model.ProfilerTypePerf
}
