// Package webui provides flame graph analysis services for the web UI.
package webui

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/perf-analysis/internal/flamegraph"
	"github.com/perf-analysis/internal/parser/collapsed"
)

// FlameGraphType represents the type of flame graph analysis.
type FlameGraphType string

const (
	// FlameGraphTypeCPU represents CPU profiling flame graph.
	FlameGraphTypeCPU FlameGraphType = "cpu"
	// FlameGraphTypeMemory represents memory allocation flame graph.
	FlameGraphTypeMemory FlameGraphType = "memory"
	// FlameGraphTypeTracing represents tracing/latency flame graph.
	FlameGraphTypeTracing FlameGraphType = "tracing"
)

// FlameGraphLoader defines the interface for loading flame graph data.
type FlameGraphLoader interface {
	// Load loads flame graph data for a task.
	Load(ctx context.Context, taskDir string) (*flamegraph.FlameGraph, error)
	// SupportedType returns the flame graph type this loader supports.
	SupportedType() FlameGraphType
}

// FlameGraphService provides unified flame graph data loading and caching.
// It supports multiple flame graph types (CPU, Memory, Tracing) through a common interface.
type FlameGraphService struct {
	dataDir string
	loaders map[FlameGraphType]FlameGraphLoader
	cache   sync.Map // key: "taskID:type" -> *flamegraph.FlameGraph
}

// NewFlameGraphService creates a new FlameGraphService.
func NewFlameGraphService(dataDir string) *FlameGraphService {
	svc := &FlameGraphService{
		dataDir: dataDir,
		loaders: make(map[FlameGraphType]FlameGraphLoader),
	}

	// Register default loaders
	svc.RegisterLoader(NewCPUFlameGraphLoader())

	return svc
}

// RegisterLoader registers a flame graph loader for a specific type.
func (s *FlameGraphService) RegisterLoader(loader FlameGraphLoader) {
	s.loaders[loader.SupportedType()] = loader
}

// GetFlameGraph returns the flame graph for a task and type.
func (s *FlameGraphService) GetFlameGraph(ctx context.Context, taskID string, fgType FlameGraphType) (*flamegraph.FlameGraph, error) {
	cacheKey := fmt.Sprintf("%s:%s", taskID, fgType)

	// Check cache first
	if cached, ok := s.cache.Load(cacheKey); ok {
		return cached.(*flamegraph.FlameGraph), nil
	}

	// Get the appropriate loader
	loader, ok := s.loaders[fgType]
	if !ok {
		return nil, fmt.Errorf("no loader registered for flame graph type: %s", fgType)
	}

	// Load the flame graph
	taskDir := filepath.Join(s.dataDir, taskID)
	fg, err := loader.Load(ctx, taskDir)
	if err != nil {
		return nil, err
	}

	// Cache the result
	s.cache.Store(cacheKey, fg)
	return fg, nil
}

// InvalidateCache invalidates the cache for a task.
func (s *FlameGraphService) InvalidateCache(taskID string) {
	// Delete all type caches for this task
	for fgType := range s.loaders {
		cacheKey := fmt.Sprintf("%s:%s", taskID, fgType)
		s.cache.Delete(cacheKey)
	}
}

// ClearCache clears all cached data.
func (s *FlameGraphService) ClearCache() {
	s.cache = sync.Map{}
}

// CPUFlameGraphLoader loads CPU profiling flame graphs.
type CPUFlameGraphLoader struct{}

// NewCPUFlameGraphLoader creates a new CPUFlameGraphLoader.
func NewCPUFlameGraphLoader() *CPUFlameGraphLoader {
	return &CPUFlameGraphLoader{}
}

// SupportedType returns the flame graph type this loader supports.
func (l *CPUFlameGraphLoader) SupportedType() FlameGraphType {
	return FlameGraphTypeCPU
}

// Load loads CPU flame graph data for a task.
func (l *CPUFlameGraphLoader) Load(ctx context.Context, taskDir string) (*flamegraph.FlameGraph, error) {
	// Try to load from pre-computed flame graph file first
	fg, err := l.loadFromFlameGraphFile(taskDir)
	if err == nil {
		return fg, nil
	}

	// Fall back to analyzing from collapsed file
	return l.loadFromCollapsedFile(ctx, taskDir)
}

// loadFromFlameGraphFile loads flame graph from pre-computed JSON file.
func (l *CPUFlameGraphLoader) loadFromFlameGraphFile(taskDir string) (*flamegraph.FlameGraph, error) {
	flameGraphFile := filepath.Join(taskDir, "collapsed_data.json.gz")

	f, err := os.Open(flameGraphFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	var fg flamegraph.FlameGraph
	decoder := json.NewDecoder(gzReader)
	if err := decoder.Decode(&fg); err != nil {
		return nil, fmt.Errorf("failed to decode flame graph: %w", err)
	}

	return &fg, nil
}

// loadFromCollapsedFile loads flame graph by parsing collapsed file.
func (l *CPUFlameGraphLoader) loadFromCollapsedFile(ctx context.Context, taskDir string) (*flamegraph.FlameGraph, error) {
	collapsedFile := l.findCollapsedFile(taskDir)
	if collapsedFile == "" {
		return nil, fmt.Errorf("no collapsed file found in %s", taskDir)
	}

	f, err := os.Open(collapsedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open collapsed file: %w", err)
	}
	defer f.Close()

	// Parse the collapsed file
	parser := collapsed.NewParser(nil)
	parseCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	parseResult, err := parser.Parse(parseCtx, f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collapsed file: %w", err)
	}

	// Generate flame graph with thread analysis
	generator := flamegraph.NewGenerator(nil)
	fg, err := generator.Generate(parseCtx, parseResult.Samples)
	if err != nil {
		return nil, fmt.Errorf("failed to generate flame graph: %w", err)
	}

	return fg, nil
}

// findCollapsedFile finds a collapsed format file in the task directory.
func (l *CPUFlameGraphLoader) findCollapsedFile(taskDir string) string {
	patterns := []string{"*.collapsed", "*.folded", "cpu.collapsed", "cpu.folded"}

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(taskDir, pattern))
		if len(matches) > 0 {
			return matches[0]
		}
	}

	return ""
}

// MemoryFlameGraphLoader loads memory allocation flame graphs.
type MemoryFlameGraphLoader struct{}

// NewMemoryFlameGraphLoader creates a new MemoryFlameGraphLoader.
func NewMemoryFlameGraphLoader() *MemoryFlameGraphLoader {
	return &MemoryFlameGraphLoader{}
}

// SupportedType returns the flame graph type this loader supports.
func (l *MemoryFlameGraphLoader) SupportedType() FlameGraphType {
	return FlameGraphTypeMemory
}

// Load loads memory flame graph data for a task.
func (l *MemoryFlameGraphLoader) Load(ctx context.Context, taskDir string) (*flamegraph.FlameGraph, error) {
	// Try to load from pre-computed flame graph file
	// Priority order: alloc_data.json.gz (new format), memory_flamegraph.json.gz, alloc_flamegraph.json.gz
	fileNames := []string{
		"alloc_data.json.gz",        // New format from JavaMemAnalyzer
		"memory_flamegraph.json.gz", // Legacy format
		"alloc_flamegraph.json.gz",  // Legacy format
		"heap_flamegraph.json.gz",   // Legacy format
	}

	var f *os.File
	var err error
	for _, name := range fileNames {
		filePath := filepath.Join(taskDir, name)
		if f, err = os.Open(filePath); err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("no memory flame graph file found in %s", taskDir)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	var fg flamegraph.FlameGraph
	decoder := json.NewDecoder(gzReader)
	if err := decoder.Decode(&fg); err != nil {
		return nil, fmt.Errorf("failed to decode memory flame graph: %w", err)
	}

	return &fg, nil
}

// TracingFlameGraphLoader loads tracing/latency flame graphs.
type TracingFlameGraphLoader struct{}

// NewTracingFlameGraphLoader creates a new TracingFlameGraphLoader.
func NewTracingFlameGraphLoader() *TracingFlameGraphLoader {
	return &TracingFlameGraphLoader{}
}

// SupportedType returns the flame graph type this loader supports.
func (l *TracingFlameGraphLoader) SupportedType() FlameGraphType {
	return FlameGraphTypeTracing
}

// Load loads tracing flame graph data for a task.
func (l *TracingFlameGraphLoader) Load(ctx context.Context, taskDir string) (*flamegraph.FlameGraph, error) {
	// Try to load from pre-computed flame graph file
	flameGraphFile := filepath.Join(taskDir, "tracing_flamegraph.json.gz")

	f, err := os.Open(flameGraphFile)
	if err != nil {
		// Try alternative file names
		altFiles := []string{"latency_flamegraph.json.gz", "wall_flamegraph.json.gz"}
		for _, alt := range altFiles {
			altPath := filepath.Join(taskDir, alt)
			if f, err = os.Open(altPath); err == nil {
				break
			}
		}
		if err != nil {
			return nil, fmt.Errorf("no tracing flame graph file found in %s", taskDir)
		}
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	var fg flamegraph.FlameGraph
	decoder := json.NewDecoder(gzReader)
	if err := decoder.Decode(&fg); err != nil {
		return nil, fmt.Errorf("failed to decode tracing flame graph: %w", err)
	}

	return &fg, nil
}

// ConvertFlameGraphToAnalysisResult converts a FlameGraph to CPUAnalysisResult.
// This is a shared utility function used by various services.
func ConvertFlameGraphToAnalysisResult(fg *flamegraph.FlameGraph) *flamegraph.CPUAnalysisResult {
	result := flamegraph.NewCPUAnalysisResult()

	if fg == nil {
		return result
	}

	// Set basic info from flame graph
	result.TotalSamples = fg.TotalSamples
	result.MaxStackDepth = fg.MaxDepth
	result.FlameGraph = fg.Root

	if fg.ThreadAnalysis == nil {
		return result
	}

	ta := fg.ThreadAnalysis

	// Set summary statistics
	result.TotalThreads = ta.TotalThreads
	result.ActiveThreads = ta.ActiveThreads
	result.UniqueFunctions = ta.UniqueFunctions
	result.AnalysisDurationMs = ta.AnalysisDurationMs

	// Convert threads
	for _, t := range ta.Threads {
		result.AddThread(t)
	}

	// Sort threads
	result.SortThreads()

	// Convert global top functions
	result.TopFuncs = ta.TopFunctions

	// Build call stacks map from top functions
	for _, tf := range ta.TopFunctions {
		if len(tf.TopCallStacks) > 0 {
			result.TopFuncsCallstacks[tf.Name] = &flamegraph.CallStackInfo{
				FunctionName: tf.Name,
				CallStacks:   tf.TopCallStacks,
				Count:        len(tf.TopCallStacks),
			}
		}
	}

	return result
}
