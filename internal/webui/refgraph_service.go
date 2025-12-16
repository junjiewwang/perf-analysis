// Package webui provides the web UI server for performance analysis.
package webui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/perf-analysis/internal/parser/hprof"
)

// RefGraphService manages ReferenceGraph loading, caching, and queries.
// It provides a high-level API for the web UI to interact with heap data.
type RefGraphService struct {
	dataDir string

	// Cache for loaded reference graphs (keyed by task ID)
	mu     sync.RWMutex
	cache  map[string]*refGraphCacheEntry
	maxCacheSize int
}

// refGraphCacheEntry holds a cached reference graph and its builder.
type refGraphCacheEntry struct {
	refGraph *hprof.ReferenceGraph
	builder  *hprof.BiggestObjectsBuilder
}

// NewRefGraphService creates a new RefGraphService.
func NewRefGraphService(dataDir string) *RefGraphService {
	return &RefGraphService{
		dataDir:      dataDir,
		cache:        make(map[string]*refGraphCacheEntry),
		maxCacheSize: 3, // Keep at most 3 graphs in memory
	}
}

// GetObjectFields returns the fields of a specific object for tree expansion.
// This is the main API for lazy loading child objects in the Biggest Objects view.
func (s *RefGraphService) GetObjectFields(taskID string, objectIDStr string) ([]*hprof.ObjectFieldDetail, error) {
	entry, err := s.getOrLoadGraph(taskID)
	if err != nil {
		return nil, err
	}

	objectID, err := parseObjectID(objectIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid object ID: %w", err)
	}

	fields := entry.builder.GetObjectFields(objectID)
	return fields, nil
}

// GetObjectInfo returns basic information about an object.
func (s *RefGraphService) GetObjectInfo(taskID string, objectIDStr string) (*hprof.ObjectFieldDetail, error) {
	entry, err := s.getOrLoadGraph(taskID)
	if err != nil {
		return nil, err
	}

	objectID, err := parseObjectID(objectIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid object ID: %w", err)
	}

	info := entry.builder.GetObjectInfo(objectID)
	if info == nil {
		return nil, fmt.Errorf("object not found: %s", objectIDStr)
	}
	return info, nil
}

// GetBiggestObjectsByClass returns the biggest objects for a specific class.
func (s *RefGraphService) GetBiggestObjectsByClass(taskID string, className string, topN int, sortBy string) ([]*hprof.BiggestObject, error) {
	entry, err := s.getOrLoadGraph(taskID)
	if err != nil {
		return nil, err
	}

	if topN <= 0 {
		topN = 50
	}
	if sortBy == "" {
		sortBy = "retained"
	}

	objects := entry.builder.BuildBiggestObjectsByClass(className, topN, sortBy)
	return objects, nil
}

// GetGCRootPaths returns the GC root paths for a specific object.
func (s *RefGraphService) GetGCRootPaths(taskID string, objectIDStr string, maxPaths int, maxDepth int) ([]hprof.GCRootPath, error) {
	entry, err := s.getOrLoadGraph(taskID)
	if err != nil {
		return nil, err
	}

	objectID, err := parseObjectID(objectIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid object ID: %w", err)
	}

	if maxPaths <= 0 {
		maxPaths = 3
	}
	if maxDepth <= 0 {
		maxDepth = 15
	}

	paths := entry.refGraph.FindPathsToGCRoot(objectID, maxPaths, maxDepth)
	
	// Convert to value slice
	result := make([]hprof.GCRootPath, 0, len(paths))
	for _, p := range paths {
		if p != nil {
			result = append(result, *p)
		}
	}
	return result, nil
}

// GetRetainers returns the retainers for a specific object.
func (s *RefGraphService) GetRetainers(taskID string, objectIDStr string, maxRetainers int) ([]*ObjectRetainerInfo, error) {
	entry, err := s.getOrLoadGraph(taskID)
	if err != nil {
		return nil, err
	}

	objectID, err := parseObjectID(objectIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid object ID: %w", err)
	}

	if maxRetainers <= 0 {
		maxRetainers = 20
	}

	// Get incoming references (who holds this object)
	incomingRefs := entry.refGraph.GetIncomingRefs(objectID)
	
	result := make([]*ObjectRetainerInfo, 0, len(incomingRefs))
	for i, ref := range incomingRefs {
		if i >= maxRetainers {
			break
		}
		
		info := &ObjectRetainerInfo{
			ObjectID:     formatObjectID(ref.FromObjectID),
			ClassName:    entry.refGraph.GetClassName(ref.FromClassID),
			FieldName:    ref.FieldName,
			ShallowSize:  entry.refGraph.GetObjectSize(ref.FromObjectID),
			RetainedSize: entry.refGraph.GetRetainedSize(ref.FromObjectID),
		}
		result = append(result, info)
	}
	
	return result, nil
}

// HasRefGraph checks if a reference graph file exists for the given task.
func (s *RefGraphService) HasRefGraph(taskID string) bool {
	taskDir := s.getTaskDir(taskID)
	refGraphFile := filepath.Join(taskDir, "refgraph.bin")
	_, err := os.Stat(refGraphFile)
	return err == nil
}

// ClearCache clears the reference graph cache.
func (s *RefGraphService) ClearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[string]*refGraphCacheEntry)
}

// ObjectRetainerInfo represents information about an object that retains another object.
type ObjectRetainerInfo struct {
	ObjectID     string `json:"object_id"`
	ClassName    string `json:"class_name"`
	FieldName    string `json:"field_name"`
	ShallowSize  int64  `json:"shallow_size"`
	RetainedSize int64  `json:"retained_size"`
}

// getOrLoadGraph loads a reference graph from cache or disk.
func (s *RefGraphService) getOrLoadGraph(taskID string) (*refGraphCacheEntry, error) {
	// Check cache first
	s.mu.RLock()
	entry, ok := s.cache[taskID]
	s.mu.RUnlock()
	
	if ok {
		return entry, nil
	}

	// Load from disk
	return s.loadGraph(taskID)
}

// loadGraph loads a reference graph from disk and caches it.
func (s *RefGraphService) loadGraph(taskID string) (*refGraphCacheEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check cache after acquiring write lock
	if entry, ok := s.cache[taskID]; ok {
		return entry, nil
	}

	taskDir := s.getTaskDir(taskID)
	refGraphFile := filepath.Join(taskDir, "refgraph.bin")

	// Check if file exists
	if _, err := os.Stat(refGraphFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("reference graph not found for task %s", taskID)
	}

	// Deserialize the reference graph
	refGraph, err := hprof.DeserializeReferenceGraphFromFile(refGraphFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load reference graph: %w", err)
	}

	// Load class layouts if available
	var classLayouts map[uint64]*hprof.ClassFieldLayout
	classLayoutsFile := filepath.Join(taskDir, "class_layouts.json")
	if data, err := os.ReadFile(classLayoutsFile); err == nil {
		json.Unmarshal(data, &classLayouts)
	}

	// Create builder
	builder := hprof.NewBiggestObjectsBuilder(refGraph, classLayouts, nil)

	entry := &refGraphCacheEntry{
		refGraph: refGraph,
		builder:  builder,
	}

	// Evict oldest entry if cache is full
	if len(s.cache) >= s.maxCacheSize {
		for k := range s.cache {
			delete(s.cache, k)
			break // Just delete one
		}
	}

	s.cache[taskID] = entry
	return entry, nil
}

// getTaskDir returns the task directory path.
func (s *RefGraphService) getTaskDir(taskID string) string {
	if taskID == "" {
		return s.dataDir
	}
	return filepath.Join(s.dataDir, taskID)
}

// parseObjectID parses an object ID from string (supports hex format like "0x12345").
func parseObjectID(s string) (uint64, error) {
	// Remove "0x" prefix if present
	if len(s) > 2 && s[:2] == "0x" {
		return strconv.ParseUint(s[2:], 16, 64)
	}
	// Try hex first, then decimal
	if id, err := strconv.ParseUint(s, 16, 64); err == nil {
		return id, nil
	}
	return strconv.ParseUint(s, 10, 64)
}

// formatObjectID formats an object ID as a hex string.
func formatObjectID(id uint64) string {
	return fmt.Sprintf("0x%x", id)
}
