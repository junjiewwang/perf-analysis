package analyzer

import (
	"github.com/perf-analysis/pkg/model"
)

// Factory creates analyzers based on task type and profiler type.
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

// CreateAnalyzer creates an analyzer for the given task type and profiler type.
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
		// For now, use Java CPU analyzer as it handles collapsed format
		return NewJavaCPUAnalyzer(f.config), nil
	case model.ProfilerTypePProf:
		// TODO: Implement pprof analyzer
		return nil, ErrUnsupportedTaskType
	default:
		return nil, ErrUnsupportedTaskType
	}
}

// CreateManager creates a new analyzer manager with all registered analyzers.
func (f *Factory) CreateManager() *Manager {
	manager := NewManager()

	// Register Java CPU analyzer
	javaCPUAnalyzer := NewJavaCPUAnalyzer(f.config)
	manager.Register(javaCPUAnalyzer)

	// Register Java memory analyzer (shares TaskTypeJava but different profiler type)
	// Note: Manager routes by TaskType, so we need special handling for profiler type

	// Register Java heap analyzer
	javaHeapAnalyzer := NewJavaHeapAnalyzer(f.config)
	manager.Register(javaHeapAnalyzer)

	return manager
}

// AnalyzerSelector selects the appropriate analyzer based on request.
type AnalyzerSelector struct {
	factory *Factory
}

// NewAnalyzerSelector creates a new analyzer selector.
func NewAnalyzerSelector(factory *Factory) *AnalyzerSelector {
	return &AnalyzerSelector{factory: factory}
}

// Select selects the appropriate analyzer for the given request.
func (s *AnalyzerSelector) Select(req *model.AnalysisRequest) (Analyzer, error) {
	return s.factory.CreateAnalyzer(req.TaskType, req.ProfilerType)
}
