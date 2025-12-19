package analyzer

import (
	"fmt"

	"github.com/perf-analysis/pkg/model"
)

// Factory creates analyzers based on analysis mode.
type Factory struct {
	config *BaseAnalyzerConfig
}

// NewFactory creates a new analyzer factory.
func NewFactory(config *BaseAnalyzerConfig) *Factory {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}
	return &Factory{config: config}
}

// CreateAnalyzerForMode creates an analyzer for the given analysis mode.
// This is the preferred method for creating analyzers.
func (f *Factory) CreateAnalyzerForMode(mode AnalysisMode) (Analyzer, error) {
	switch mode {
	case ModeJavaCPU:
		return NewJavaCPUAnalyzer(f.config), nil
	case ModeJavaAlloc:
		return NewJavaMemAnalyzer(f.config), nil
	case ModeJavaHeap:
		return NewJavaHeapAnalyzer(f.config), nil
	case ModeCPU:
		// Generic CPU uses the same analyzer as Java CPU (collapsed format)
		return NewJavaCPUAnalyzer(f.config), nil
	default:
		return nil, fmt.Errorf("%w: unknown mode %q", ErrUnsupportedMode, mode)
	}
}

// CreateAnalyzer creates an analyzer for the given task type and profiler type.
// Deprecated: Use CreateAnalyzerForMode instead.
func (f *Factory) CreateAnalyzer(taskType model.TaskType, profilerType model.ProfilerType) (Analyzer, error) {
	switch taskType {
	case model.TaskTypeJava:
		return f.createJavaAnalyzer(profilerType)
	case model.TaskTypeJavaHeap:
		return NewJavaHeapAnalyzer(f.config), nil
	case model.TaskTypeGeneric:
		return f.createGenericAnalyzer(profilerType)
	default:
		return nil, ErrUnsupportedTaskType
	}
}

// createJavaAnalyzer creates a Java analyzer based on profiler type.
func (f *Factory) createJavaAnalyzer(profilerType model.ProfilerType) (Analyzer, error) {
	switch profilerType {
	case model.ProfilerTypePerf:
		return NewJavaCPUAnalyzer(f.config), nil
	case model.ProfilerTypeAsyncAlloc:
		return NewJavaMemAnalyzer(f.config), nil
	default:
		return nil, ErrUnsupportedTaskType
	}
}

// createGenericAnalyzer creates a generic analyzer based on profiler type.
func (f *Factory) createGenericAnalyzer(profilerType model.ProfilerType) (Analyzer, error) {
	switch profilerType {
	case model.ProfilerTypePerf:
		return NewJavaCPUAnalyzer(f.config), nil
	case model.ProfilerTypePProf:
		return nil, ErrUnsupportedTaskType
	default:
		return nil, ErrUnsupportedTaskType
	}
}

// CreateManager creates a new analyzer manager with all registered analyzers.
func (f *Factory) CreateManager() *Manager {
	manager := NewManager()

	// Register Java CPU analyzer with specific key
	javaCPUAnalyzer := NewJavaCPUAnalyzer(f.config)
	manager.RegisterWithKey(javaCPUAnalyzer, model.TaskTypeJava, model.ProfilerTypePerf)

	// Register Java memory analyzer with specific key
	javaMemAnalyzer := NewJavaMemAnalyzer(f.config)
	manager.RegisterWithKey(javaMemAnalyzer, model.TaskTypeJava, model.ProfilerTypeAsyncAlloc)

	// Register Java heap analyzer
	javaHeapAnalyzer := NewJavaHeapAnalyzer(f.config)
	manager.Register(javaHeapAnalyzer)

	return manager
}
