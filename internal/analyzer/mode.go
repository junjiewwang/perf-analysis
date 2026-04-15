package analyzer

import (
	libanalyzer "github.com/junjiewwang/perf-analysis/perflib/analyzer"
	"github.com/junjiewwang/perf-analysis/pkg/model"
)

// AnalysisMode represents a user-friendly analysis mode.
// It maps directly to the model.AnalysisMode("{profiler}-{event}") composite key.
type AnalysisMode = libanalyzer.AnalysisMode

// Mode constants — aliases to perflib/analyzer.
const (
	ModeJavaCPU        = libanalyzer.ModeJavaCPU
	ModeJavaAlloc      = libanalyzer.ModeJavaAlloc
	ModeJavaWall       = libanalyzer.ModeJavaWall
	ModeJavaLock       = libanalyzer.ModeJavaLock
	ModeJavaHeap       = libanalyzer.ModeJavaHeap
	ModeCPU            = libanalyzer.ModeCPU
	ModePProfCPU       = libanalyzer.ModePProfCPU
	ModePProfHeap      = libanalyzer.ModePProfHeap
	ModePProfGoroutine = libanalyzer.ModePProfGoroutine
	ModePProfBlock     = libanalyzer.ModePProfBlock
	ModePProfMutex     = libanalyzer.ModePProfMutex
	ModePProfAll       = libanalyzer.ModePProfAll
	ModeJeprof         = libanalyzer.ModeJeprof
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

// ParseMode parses a mode string into AnalysisMode.
func ParseMode(s string) (AnalysisMode, error) {
	return libanalyzer.ParseMode(s)
}

// GetModeInfo returns the metadata for a mode.
// It delegates to perflib and converts the result to the local ModeInfo type.
func GetModeInfo(mode AnalysisMode) (*ModeInfo, bool) {
	info, ok := libanalyzer.GetModeInfo(mode)
	if !ok {
		return nil, false
	}
	return convertModeInfo(info), true
}

// ValidModes returns a comma-separated list of valid mode names.
func ValidModes() string {
	return libanalyzer.ValidModes()
}

// AllModes returns all registered mode information.
func AllModes() []*ModeInfo {
	libModes := libanalyzer.AllModes()
	result := make([]*ModeInfo, 0, len(libModes))
	for _, lm := range libModes {
		result = append(result, convertModeInfo(lm))
	}
	return result
}

// convertModeInfo converts a perflib ModeInfo to the local ModeInfo type.
// Since model.Profiler/EventType/ResourceType are type aliases of perflib/model types,
// the field values are directly compatible.
func convertModeInfo(info *libanalyzer.ModeInfo) *ModeInfo {
	return &ModeInfo{
		Mode:        info.Mode,
		Description: info.Description,
		InputFormat: info.InputFormat,
		Profiler:    info.Profiler,
		Event:       info.Event,
		Resource:    info.Resource,
	}
}
