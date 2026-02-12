package analyzer

import (
	"fmt"
)

// AnalyzerConstructor is a function that creates an Analyzer from config.
type AnalyzerConstructor func(config *BaseAnalyzerConfig) Analyzer

// Factory creates analyzers based on analysis mode using a registry of constructors.
type Factory struct {
	config       *BaseAnalyzerConfig
	constructors map[AnalysisMode]AnalyzerConstructor
}

// NewFactory creates a new analyzer factory with built-in constructors.
func NewFactory(config *BaseAnalyzerConfig) *Factory {
	if config == nil {
		config = DefaultBaseAnalyzerConfig()
	}
	return &Factory{
		config:       config,
		constructors: defaultConstructors(),
	}
}

// CreateAnalyzerForMode creates an analyzer for the given analysis mode.
func (f *Factory) CreateAnalyzerForMode(mode AnalysisMode) (Analyzer, error) {
	ctor, ok := f.constructors[mode]
	if !ok {
		return nil, fmt.Errorf("%w: unknown mode %q", ErrUnsupportedMode, mode)
	}
	return ctor(f.config), nil
}

// RegisterConstructor registers a custom constructor for a mode.
// This can be used to override built-in constructors or add new modes at runtime.
func (f *Factory) RegisterConstructor(mode AnalysisMode, ctor AnalyzerConstructor) {
	f.constructors[mode] = ctor
}

// asConstructor adapts a typed constructor to the generic AnalyzerConstructor signature.
func asConstructor[T Analyzer](fn func(*BaseAnalyzerConfig) T) AnalyzerConstructor {
	return func(config *BaseAnalyzerConfig) Analyzer {
		return fn(config)
	}
}

// defaultConstructors returns the built-in mode → constructor mapping.
func defaultConstructors() map[AnalysisMode]AnalyzerConstructor {
	// Shared constructor — CPU/Wall/Lock/Perf all use the same collapsed-stack analyzer
	cpuCtor := asConstructor(NewJavaCPUAnalyzer)

	return map[AnalysisMode]AnalyzerConstructor{
		ModeJavaCPU:        cpuCtor,
		ModeJavaWall:       cpuCtor,
		ModeJavaLock:       cpuCtor,
		ModeCPU:            cpuCtor,
		ModeJavaAlloc:      asConstructor(NewJavaMemAnalyzer),
		ModeJavaHeap:       func(config *BaseAnalyzerConfig) Analyzer { return NewJavaHeapAnalyzer(config) },
		ModePProfCPU:       asConstructor(NewPProfCPUAnalyzer),
		ModePProfHeap:      asConstructor(NewPProfHeapAnalyzer),
		ModePProfGoroutine: asConstructor(NewPProfGoroutineAnalyzer),
		ModePProfBlock:     asConstructor(NewPProfBlockAnalyzer),
		ModePProfMutex:     asConstructor(NewPProfMutexAnalyzer),
		ModePProfAll:       asConstructor(NewPProfBatchAnalyzer),
	}
}
