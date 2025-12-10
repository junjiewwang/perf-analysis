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

// AnalyzerFactory is a function that creates a new Analyzer instance.
type AnalyzerFactory func(opts ...Option) (Analyzer, error)

// Option is a function that configures an Analyzer.
type Option func(interface{})

// Manager manages analyzer instances and routes tasks to appropriate analyzers.
type Manager struct {
	analyzers map[model.TaskType]Analyzer
}

// NewManager creates a new analyzer Manager.
func NewManager() *Manager {
	return &Manager{
		analyzers: make(map[model.TaskType]Analyzer),
	}
}

// Register registers an analyzer for specific task types.
func (m *Manager) Register(analyzer Analyzer) {
	for _, taskType := range analyzer.SupportedTypes() {
		m.analyzers[taskType] = analyzer
	}
}

// GetAnalyzer returns the appropriate analyzer for a task type.
func (m *Manager) GetAnalyzer(taskType model.TaskType) (Analyzer, bool) {
	analyzer, ok := m.analyzers[taskType]
	return analyzer, ok
}

// AnalyzeTask routes a task to the appropriate analyzer and performs analysis.
func (m *Manager) AnalyzeTask(ctx context.Context, req *model.AnalysisRequest, dataReader io.Reader) (*model.AnalysisResponse, error) {
	analyzer, ok := m.GetAnalyzer(req.TaskType)
	if !ok {
		return nil, ErrUnsupportedTaskType
	}
	return analyzer.AnalyzeFromReader(ctx, req, dataReader)
}

// ListAnalyzers returns all registered analyzers.
func (m *Manager) ListAnalyzers() []Analyzer {
	seen := make(map[string]bool)
	var result []Analyzer
	for _, analyzer := range m.analyzers {
		if !seen[analyzer.Name()] {
			seen[analyzer.Name()] = true
			result = append(result, analyzer)
		}
	}
	return result
}
