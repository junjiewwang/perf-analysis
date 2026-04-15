package analyzer

import (
	"fmt"
	"strings"

	"github.com/perf-analysis/perflib/model"
)

// AnalysisMode represents a user-friendly analysis mode.
// It maps directly to the model.AnalysisMode("{profiler}-{event}") composite key.
type AnalysisMode string

const (
	// ModeJavaCPU analyzes Java CPU hotspots from async-profiler/perf data.
	ModeJavaCPU AnalysisMode = "async-profiler-cpu"

	// ModeJavaAlloc analyzes Java memory allocation from async-profiler alloc data.
	ModeJavaAlloc AnalysisMode = "async-profiler-alloc"

	// ModeJavaWall analyzes Java wall-clock time from async-profiler wall data.
	ModeJavaWall AnalysisMode = "async-profiler-wall"

	// ModeJavaLock analyzes Java lock contention from async-profiler lock data.
	ModeJavaLock AnalysisMode = "async-profiler-lock"

	// ModeJavaHeap analyzes Java heap dump (HPROF format).
	ModeJavaHeap AnalysisMode = "heapdump-heap"

	// ModeCPU analyzes generic CPU profiling data (collapsed format).
	ModeCPU AnalysisMode = "perf-cpu"

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

	// ModeJeprof analyzes jemalloc heap profile.
	ModeJeprof AnalysisMode = "jeprof-heap"
)

// ModeInfo describes an analysis mode for help, validation, and dispatch.
type ModeInfo struct {
	Mode        AnalysisMode
	Description string
	InputFormat string
	Profiler    model.Profiler
	Event       model.EventType
	Resource    model.ResourceType
}

// modeRegistry maps mode names to their metadata.
var modeRegistry = map[AnalysisMode]*ModeInfo{
	ModeJavaCPU: {
		Mode:        ModeJavaCPU,
		Description: "Java CPU hotspot analysis (async-profiler)",
		InputFormat: "Collapsed stack format (.collapsed, .data, .txt)",
		Profiler:    model.ProfilerAsyncProfiler,
		Event:       model.EventCPU,
		Resource:    model.ResourceCPU,
	},
	ModeJavaAlloc: {
		Mode:        ModeJavaAlloc,
		Description: "Java memory allocation analysis (async-profiler alloc)",
		InputFormat: "Collapsed stack format (.collapsed, .data, .txt)",
		Profiler:    model.ProfilerAsyncProfiler,
		Event:       model.EventAlloc,
		Resource:    model.ResourceMemory,
	},
	ModeJavaWall: {
		Mode:        ModeJavaWall,
		Description: "Java wall-clock time analysis (async-profiler wall)",
		InputFormat: "Collapsed stack format (.collapsed, .data, .txt)",
		Profiler:    model.ProfilerAsyncProfiler,
		Event:       model.EventWall,
		Resource:    model.ResourceApp,
	},
	ModeJavaLock: {
		Mode:        ModeJavaLock,
		Description: "Java lock contention analysis (async-profiler lock)",
		InputFormat: "Collapsed stack format (.collapsed, .data, .txt)",
		Profiler:    model.ProfilerAsyncProfiler,
		Event:       model.EventLock,
		Resource:    model.ResourceConcurrency,
	},
	ModeJavaHeap: {
		Mode:        ModeJavaHeap,
		Description: "Java heap dump analysis (HPROF)",
		InputFormat: "HPROF binary format (.hprof)",
		Profiler:    model.ProfilerHeapDump,
		Event:       model.EventHeap,
		Resource:    model.ResourceMemory,
	},
	ModeCPU: {
		Mode:        ModeCPU,
		Description: "Generic CPU profiling analysis (perf)",
		InputFormat: "Collapsed stack format (.collapsed, .data, .txt)",
		Profiler:    model.ProfilerPerf,
		Event:       model.EventCPU,
		Resource:    model.ResourceCPU,
	},
	ModePProfCPU: {
		Mode:        ModePProfCPU,
		Description: "Go pprof CPU profile analysis",
		InputFormat: "Go pprof format (.pprof, .pb.gz)",
		Profiler:    model.ProfilerPProf,
		Event:       model.EventCPU,
		Resource:    model.ResourceCPU,
	},
	ModePProfHeap: {
		Mode:        ModePProfHeap,
		Description: "Go pprof Heap profile analysis",
		InputFormat: "Go pprof format (.pprof, .pb.gz)",
		Profiler:    model.ProfilerPProf,
		Event:       model.EventHeap,
		Resource:    model.ResourceMemory,
	},
	ModePProfGoroutine: {
		Mode:        ModePProfGoroutine,
		Description: "Go pprof Goroutine profile analysis",
		InputFormat: "Go pprof format (.pprof, .pb.gz)",
		Profiler:    model.ProfilerPProf,
		Event:       model.EventGoroutine,
		Resource:    model.ResourceGoroutine,
	},
	ModePProfBlock: {
		Mode:        ModePProfBlock,
		Description: "Go pprof Block profile analysis",
		InputFormat: "Go pprof format (.pprof, .pb.gz)",
		Profiler:    model.ProfilerPProf,
		Event:       model.EventBlock,
		Resource:    model.ResourceConcurrency,
	},
	ModePProfMutex: {
		Mode:        ModePProfMutex,
		Description: "Go pprof Mutex profile analysis",
		InputFormat: "Go pprof format (.pprof, .pb.gz)",
		Profiler:    model.ProfilerPProf,
		Event:       model.EventMutex,
		Resource:    model.ResourceConcurrency,
	},
	ModePProfAll: {
		Mode:        ModePProfAll,
		Description: "Batch analysis of all pprof profiles in a directory",
		InputFormat: "Directory containing pprof subdirectories (cpu/, heap/, goroutine/, etc.)",
		Profiler:    model.ProfilerPProf,
		Event:       model.EventCPU, // Primary event
		Resource:    model.ResourceCPU,
	},
	ModeJeprof: {
		Mode:        ModeJeprof,
		Description: "Jemalloc heap profile analysis",
		InputFormat: "Jeprof heap format",
		Profiler:    model.ProfilerJeprof,
		Event:       model.EventHeap,
		Resource:    model.ResourceMemory,
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
	order := []AnalysisMode{
		ModeJavaCPU, ModeJavaAlloc, ModeJavaWall, ModeJavaLock, ModeJavaHeap, ModeCPU,
		ModePProfCPU, ModePProfHeap, ModePProfGoroutine, ModePProfBlock, ModePProfMutex, ModePProfAll,
		ModeJeprof,
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

// ResourceType returns the resource type for this mode.
func (m AnalysisMode) ResourceType() model.ResourceType {
	if info := m.Info(); info != nil {
		return info.Resource
	}
	return ""
}
