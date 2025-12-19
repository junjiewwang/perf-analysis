// Package analyzer defines the core analyzer interfaces.
package analyzer

import (
	"context"
	"io"

	"github.com/perf-analysis/pkg/model"
)

// Analyzer is the interface for all profiling data analyzers.
type Analyzer interface {
	// Analyze performs the analysis on the given request.
	Analyze(ctx context.Context, req *model.AnalysisRequest) (*model.AnalysisResponse, error)

	// AnalyzeFromReader performs the analysis using a reader.
	AnalyzeFromReader(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error)

	// SupportedTypes returns the task types supported by this analyzer.
	SupportedTypes() []model.TaskType

	// Name returns the name of this analyzer.
	Name() string
}

// CanHandleAnalyzer is an optional interface for analyzers that need
// fine-grained control over which requests they can handle.
type CanHandleAnalyzer interface {
	Analyzer
	// CanHandle checks if this analyzer can handle the given request.
	CanHandle(req *model.AnalysisRequest) bool
}

// AnalyzerFactory is a function that creates a new Analyzer instance.
type AnalyzerFactory func(opts ...Option) (Analyzer, error)

// Option is a function that configures an Analyzer.
type Option func(interface{})

// AnalyzerKey uniquely identifies an analyzer by task type and profiler type.
type AnalyzerKey struct {
	TaskType     model.TaskType
	ProfilerType model.ProfilerType
}

// Manager manages analyzer instances and routes tasks to appropriate analyzers.
type Manager struct {
	// analyzers maps task type to analyzers (for backward compatibility)
	analyzers map[model.TaskType][]Analyzer
	// analyzersByKey maps (TaskType, ProfilerType) to specific analyzer
	analyzersByKey map[AnalyzerKey]Analyzer
}

// NewManager creates a new analyzer Manager.
func NewManager() *Manager {
	return &Manager{
		analyzers:      make(map[model.TaskType][]Analyzer),
		analyzersByKey: make(map[AnalyzerKey]Analyzer),
	}
}

// Register registers an analyzer for specific task types.
func (m *Manager) Register(analyzer Analyzer) {
	for _, taskType := range analyzer.SupportedTypes() {
		m.analyzers[taskType] = append(m.analyzers[taskType], analyzer)
	}
}

// RegisterWithKey registers an analyzer with a specific key (TaskType + ProfilerType).
func (m *Manager) RegisterWithKey(analyzer Analyzer, taskType model.TaskType, profilerType model.ProfilerType) {
	key := AnalyzerKey{TaskType: taskType, ProfilerType: profilerType}
	m.analyzersByKey[key] = analyzer
	// Also register in the general map for backward compatibility
	m.analyzers[taskType] = append(m.analyzers[taskType], analyzer)
}

// GetAnalyzer returns the appropriate analyzer for a task type.
// Deprecated: Use GetAnalyzerForRequest for more precise matching.
func (m *Manager) GetAnalyzer(taskType model.TaskType) (Analyzer, bool) {
	analyzers, ok := m.analyzers[taskType]
	if !ok || len(analyzers) == 0 {
		return nil, false
	}
	return analyzers[0], true
}

// GetAnalyzerForRequest returns the appropriate analyzer for a request.
// It first tries to find an exact match by (TaskType, ProfilerType),
// then falls back to CanHandle check, and finally returns the first registered analyzer.
func (m *Manager) GetAnalyzerForRequest(req *model.AnalysisRequest) (Analyzer, bool) {
	// Priority 1: Exact match by key
	key := AnalyzerKey{TaskType: req.TaskType, ProfilerType: req.ProfilerType}
	if analyzer, ok := m.analyzersByKey[key]; ok {
		return analyzer, true
	}

	// Priority 2: Check CanHandle for analyzers registered for this task type
	analyzers, ok := m.analyzers[req.TaskType]
	if !ok || len(analyzers) == 0 {
		return nil, false
	}

	for _, analyzer := range analyzers {
		if canHandle, ok := analyzer.(CanHandleAnalyzer); ok {
			if canHandle.CanHandle(req) {
				return analyzer, true
			}
		}
	}

	// Priority 3: Return first registered analyzer for this task type
	return analyzers[0], true
}

// AnalyzeTask routes a task to the appropriate analyzer and performs analysis.
func (m *Manager) AnalyzeTask(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	analyzer, ok := m.GetAnalyzerForRequest(req)
	if !ok {
		return nil, ErrUnsupportedTaskType
	}
	return analyzer.AnalyzeFromReader(ctx, req, dataReader)
}

// ListAnalyzers returns all registered analyzers.
func (m *Manager) ListAnalyzers() []Analyzer {
	seen := make(map[string]bool)
	var result []Analyzer
	for _, analyzers := range m.analyzers {
		for _, analyzer := range analyzers {
			if !seen[analyzer.Name()] {
				seen[analyzer.Name()] = true
				result = append(result, analyzer)
			}
		}
	}
	return result
}
