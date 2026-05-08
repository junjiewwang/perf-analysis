// Package webui provides the web UI server for performance analysis.
package webui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/junjiewwang/perf-analysis/perflib/output"
	perflibHprof "github.com/junjiewwang/perf-analysis/perflib/parser/hprof"
	"github.com/junjiewwang/perf-analysis/perflib/query"
)

// HeapDataProvider abstracts different data sources for heap queries.
// This enables the WebUI to work with both legacy refgraph.bin and new heap_index.bin.
type HeapDataProvider interface {
	GetObjectFields(objectIDStr string) ([]*ObjectFieldResponse, error)
	GetObjectInfo(objectIDStr string) (*ObjectInfoResponse, error)
	GetBiggestObjects(topN int, sortBy string, classFilter string) ([]map[string]interface{}, error)
	GetGCRootPaths(objectIDStr string, maxPaths int, maxDepth int) (interface{}, error)
	GetRetainers(objectIDStr string, maxRetainers int) ([]*ObjectRetainerInfo, error)
	GetGCRootsSummary() (interface{}, error)
	GetGCRootsList() (interface{}, error)
	GetRetainedObjectsByGCRoot(objectIDStr string, maxObjects int) (interface{}, error)
	GetDominatorChildren(objectIDStr string, topN int, sortBy string) (interface{}, error)
	GetDominatorPath(objectIDStr string) (interface{}, error)
	GetRetainedSizeTreemap(objectIDStr string, maxNodes int) (interface{}, error)
}

// ObjectFieldResponse is the unified field response used by all providers.
type ObjectFieldResponse struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	Value        interface{} `json:"value,omitempty"`
	RefID        string      `json:"ref_id,omitempty"`
	RefClass     string      `json:"ref_class,omitempty"`
	ShallowSize  int64       `json:"shallow_size,omitempty"`
	RetainedSize int64       `json:"retained_size,omitempty"`
	HasChildren  bool        `json:"has_children"`
}

// ObjectInfoResponse is the unified object info response.
type ObjectInfoResponse struct {
	ObjectID     string `json:"object_id"`
	ClassName    string `json:"class_name"`
	ShallowSize  int64  `json:"shallow_size"`
	RetainedSize int64  `json:"retained_size"`
}

// RefGraphService manages ReferenceGraph/HeapIndex loading, caching, and queries.
// It provides a high-level API for the web UI to interact with heap data.
type RefGraphService struct {
	dataDir string

	// Cache for loaded data providers (keyed by task ID)
	mu           sync.RWMutex
	cache        map[string]*heapCacheEntry
	maxCacheSize int
}

// heapCacheEntry holds a cached data provider.
type heapCacheEntry struct {
	provider HeapDataProvider
	source   string // "heap_index" or "refgraph"
}

// NewRefGraphService creates a new RefGraphService.
func NewRefGraphService(dataDir string) *RefGraphService {
	return &RefGraphService{
		dataDir:      dataDir,
		cache:        make(map[string]*heapCacheEntry),
		maxCacheSize: 3,
	}
}

// GetObjectFields returns the fields of a specific object for tree expansion.
func (s *RefGraphService) GetObjectFields(taskID string, objectIDStr string) ([]*ObjectFieldResponse, error) {
	entry, err := s.getOrLoadProvider(taskID)
	if err != nil {
		return nil, err
	}
	return entry.provider.GetObjectFields(objectIDStr)
}

// GetObjectInfo returns basic information about an object.
func (s *RefGraphService) GetObjectInfo(taskID string, objectIDStr string) (*ObjectInfoResponse, error) {
	entry, err := s.getOrLoadProvider(taskID)
	if err != nil {
		return nil, err
	}
	return entry.provider.GetObjectInfo(objectIDStr)
}

// GetBiggestObjects returns the biggest objects for a specific class.
func (s *RefGraphService) GetBiggestObjects(taskID string, topN int, sortBy string, classFilter string) ([]map[string]interface{}, error) {
	entry, err := s.getOrLoadProvider(taskID)
	if err != nil {
		return nil, err
	}
	return entry.provider.GetBiggestObjects(topN, sortBy, classFilter)
}

// GetGCRootPaths returns the GC root paths for a specific object.
func (s *RefGraphService) GetGCRootPaths(taskID string, objectIDStr string, maxPaths int, maxDepth int) (interface{}, error) {
	entry, err := s.getOrLoadProvider(taskID)
	if err != nil {
		return nil, err
	}
	return entry.provider.GetGCRootPaths(objectIDStr, maxPaths, maxDepth)
}

// GetRetainers returns the retainers for a specific object.
func (s *RefGraphService) GetRetainers(taskID string, objectIDStr string, maxRetainers int) ([]*ObjectRetainerInfo, error) {
	entry, err := s.getOrLoadProvider(taskID)
	if err != nil {
		return nil, err
	}
	return entry.provider.GetRetainers(objectIDStr, maxRetainers)
}

// GetGCRootsSummary returns GC roots grouped by class (like IDEA).
func (s *RefGraphService) GetGCRootsSummary(taskID string) (interface{}, error) {
	entry, err := s.getOrLoadProvider(taskID)
	if err != nil {
		return nil, err
	}
	return entry.provider.GetGCRootsSummary()
}

// GetGCRootsList returns all GC roots with their information.
func (s *RefGraphService) GetGCRootsList(taskID string) (interface{}, error) {
	entry, err := s.getOrLoadProvider(taskID)
	if err != nil {
		return nil, err
	}
	return entry.provider.GetGCRootsList()
}

// GetRetainedObjectsByGCRoot returns objects retained by a specific GC root.
func (s *RefGraphService) GetRetainedObjectsByGCRoot(taskID string, objectIDStr string, maxObjects int) (interface{}, error) {
	entry, err := s.getOrLoadProvider(taskID)
	if err != nil {
		return nil, err
	}
	return entry.provider.GetRetainedObjectsByGCRoot(objectIDStr, maxObjects)
}

// GetDominatorChildren returns dominated children of a given object in the dominator tree.
func (s *RefGraphService) GetDominatorChildren(taskID string, objectIDStr string, topN int, sortBy string) (interface{}, error) {
	entry, err := s.getOrLoadProvider(taskID)
	if err != nil {
		return nil, err
	}
	return entry.provider.GetDominatorChildren(objectIDStr, topN, sortBy)
}

// GetDominatorPath returns the dominator chain from virtual root to the given object.
func (s *RefGraphService) GetDominatorPath(taskID string, objectIDStr string) (interface{}, error) {
	entry, err := s.getOrLoadProvider(taskID)
	if err != nil {
		return nil, err
	}
	return entry.provider.GetDominatorPath(objectIDStr)
}

// GetRetainedSizeTreemap returns treemap data for retained size visualization.
func (s *RefGraphService) GetRetainedSizeTreemap(taskID string, objectIDStr string, maxNodes int) (interface{}, error) {
	entry, err := s.getOrLoadProvider(taskID)
	if err != nil {
		return nil, err
	}
	return entry.provider.GetRetainedSizeTreemap(objectIDStr, maxNodes)
}

// GetHeapQueryHelper returns a HeapQueryHelper for class histogram and heap stats queries.
// This bridges the webui layer to the perflib/query layer.
func (s *RefGraphService) GetHeapQueryHelper(taskID string) (*query.HeapQueryHelper, error) {
	entry, err := s.getOrLoadProvider(taskID)
	if err != nil {
		return nil, err
	}

	// The indexedProvider has access to the HeapGraph via its engine
	indexed, ok := entry.provider.(*indexedProvider)
	if !ok {
		return nil, fmt.Errorf("heap query helper requires heap_index.bin data source")
	}

	return query.NewHeapQueryHelper(indexed.engine.GetGraph()), nil
}

// HasRefGraph checks if any heap data source exists for the given task.
func (s *RefGraphService) HasRefGraph(taskID string) bool {
	taskDir := s.getTaskDir(taskID)
	// Check heap_index.bin first (preferred)
	if _, err := os.Stat(filepath.Join(taskDir, output.FileHeapIndex)); err == nil {
		return true
	}
	// Fall back to legacy refgraph.bin
	if _, err := os.Stat(filepath.Join(taskDir, "refgraph.bin")); err == nil {
		return true
	}
	return false
}

// ClearCache clears the heap data cache.
func (s *RefGraphService) ClearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[string]*heapCacheEntry)
}

// ObjectRetainerInfo represents information about an object that retains another object.
type ObjectRetainerInfo struct {
	ObjectID     string `json:"object_id"`
	ClassName    string `json:"class_name"`
	FieldName    string `json:"field_name"`
	ShallowSize  int64  `json:"shallow_size"`
	RetainedSize int64  `json:"retained_size"`
}

// getOrLoadProvider loads a data provider from cache or disk.
func (s *RefGraphService) getOrLoadProvider(taskID string) (*heapCacheEntry, error) {
	s.mu.RLock()
	entry, ok := s.cache[taskID]
	s.mu.RUnlock()

	if ok {
		return entry, nil
	}

	return s.loadProvider(taskID)
}

// loadProvider loads a data provider from disk and caches it.
func (s *RefGraphService) loadProvider(taskID string) (*heapCacheEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check cache after acquiring write lock
	if entry, ok := s.cache[taskID]; ok {
		return entry, nil
	}

	taskDir := s.getTaskDir(taskID)

	// Try heap_index.bin first (preferred - much faster to load)
	indexFile := filepath.Join(taskDir, output.FileHeapIndex)
	if _, err := os.Stat(indexFile); err == nil {
		provider, err := newIndexedProvider(indexFile)
		if err == nil {
			entry := &heapCacheEntry{provider: provider, source: "heap_index"}
			s.evictIfNeeded()
			s.cache[taskID] = entry
			return entry, nil
		}
		return nil, fmt.Errorf("failed to load heap index for task %s: %w", taskID, err)
	}

	return nil, fmt.Errorf("no heap data found for task %s (heap_index.bin not found)", taskID)
}

// evictIfNeeded removes oldest cache entry if cache is full.
func (s *RefGraphService) evictIfNeeded() {
	if len(s.cache) >= s.maxCacheSize {
		for k := range s.cache {
			delete(s.cache, k)
			break
		}
	}
}

// getTaskDir returns the task directory path.
func (s *RefGraphService) getTaskDir(taskID string) string {
	if taskID == "" {
		return s.dataDir
	}
	return filepath.Join(s.dataDir, taskID)
}

// ============================================================================
// indexedProvider - HeapDataProvider backed by heap_index.bin + HeapQueryEngine
// ============================================================================

// indexedProvider implements HeapDataProvider using the new IndexedReferenceGraph.
type indexedProvider struct {
	engine *HeapQueryEngine
}

// newIndexedProvider creates a provider from a heap_index.bin file.
func newIndexedProvider(indexFile string) (*indexedProvider, error) {
	graph, err := perflibHprof.ReadHeapIndex(indexFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load heap index: %w", err)
	}
	return &indexedProvider{engine: NewHeapQueryEngine(graph)}, nil
}

func (p *indexedProvider) GetObjectFields(objectIDStr string) ([]*ObjectFieldResponse, error) {
	objectID, err := parseObjectID(objectIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid object ID: %w", err)
	}

	fields := p.engine.QueryObjectFields(objectID)
	result := make([]*ObjectFieldResponse, 0, len(fields))
	for _, f := range fields {
		result = append(result, &ObjectFieldResponse{
			Name:         f.Name,
			Type:         f.Type,
			RefID:        f.RefID,
			RefClass:     f.RefClass,
			ShallowSize:  f.ShallowSize,
			RetainedSize: f.RetainedSize,
			HasChildren:  f.HasChildren,
		})
	}
	return result, nil
}

func (p *indexedProvider) GetObjectInfo(objectIDStr string) (*ObjectInfoResponse, error) {
	objectID, err := parseObjectID(objectIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid object ID: %w", err)
	}

	info := p.engine.QueryObjectInfo(objectID)
	if info == nil {
		return nil, fmt.Errorf("object not found: %s", objectIDStr)
	}

	return &ObjectInfoResponse{
		ObjectID:     info.ObjectID,
		ClassName:    info.ClassName,
		ShallowSize:  info.ShallowSize,
		RetainedSize: info.RetainedSize,
	}, nil
}

func (p *indexedProvider) GetBiggestObjects(topN int, sortBy string, classFilter string) ([]map[string]interface{}, error) {
	objects := p.engine.QueryBiggestObjects(topN, sortBy, classFilter)
	result := make([]map[string]interface{}, 0, len(objects))
	for _, obj := range objects {
		result = append(result, map[string]interface{}{
			"object_id":     obj.ObjectID,
			"class_name":    obj.ClassName,
			"shallow_size":  obj.ShallowSize,
			"retained_size": obj.RetainedSize,
		})
	}
	return result, nil
}

func (p *indexedProvider) GetGCRootPaths(objectIDStr string, maxPaths int, maxDepth int) (interface{}, error) {
	objectID, err := parseObjectID(objectIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid object ID: %w", err)
	}
	return p.engine.QueryGCRootPath(objectID, maxPaths, maxDepth), nil
}

func (p *indexedProvider) GetRetainers(objectIDStr string, maxRetainers int) ([]*ObjectRetainerInfo, error) {
	objectID, err := parseObjectID(objectIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid object ID: %w", err)
	}

	retainers := p.engine.QueryRetainers(objectID, maxRetainers)
	result := make([]*ObjectRetainerInfo, 0, len(retainers))
	for _, r := range retainers {
		result = append(result, &ObjectRetainerInfo{
			ObjectID:     r.ObjectID,
			ClassName:    r.ClassName,
			FieldName:    r.FieldName,
			ShallowSize:  r.ShallowSize,
			RetainedSize: r.RetainedSize,
		})
	}
	return result, nil
}

func (p *indexedProvider) GetGCRootsSummary() (interface{}, error) {
	return p.engine.QueryGCRootsSummary(), nil
}

func (p *indexedProvider) GetGCRootsList() (interface{}, error) {
	// Return GC roots as list using the summary entries
	return p.engine.QueryGCRootsSummary(), nil
}

func (p *indexedProvider) GetRetainedObjectsByGCRoot(objectIDStr string, maxObjects int) (interface{}, error) {
	objectID, err := parseObjectID(objectIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid object ID: %w", err)
	}

	// Use QueryObjectFields to get outgoing references from the GC root
	fields := p.engine.QueryObjectFields(objectID)
	return fields, nil
}

func (p *indexedProvider) GetDominatorChildren(objectIDStr string, topN int, sortBy string) (interface{}, error) {
	var objectID uint64
	if objectIDStr != "" && objectIDStr != "0" {
		var err error
		objectID, err = parseObjectID(objectIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid object ID: %w", err)
		}
	}
	return p.engine.QueryDominatorChildren(objectID, topN, sortBy), nil
}

func (p *indexedProvider) GetDominatorPath(objectIDStr string) (interface{}, error) {
	objectID, err := parseObjectID(objectIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid object ID: %w", err)
	}
	return p.engine.QueryDominatorPath(objectID), nil
}

func (p *indexedProvider) GetRetainedSizeTreemap(objectIDStr string, maxNodes int) (interface{}, error) {
	var objectID uint64
	if objectIDStr != "" && objectIDStr != "0" {
		var err error
		objectID, err = parseObjectID(objectIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid object ID: %w", err)
		}
	}
	return p.engine.QueryRetainedSizeTreemap(objectID, maxNodes), nil
}

// ============================================================================
// Helpers
// ============================================================================

// parseObjectID parses an object ID from string (supports hex format like "0x12345").
func parseObjectID(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return strconv.ParseUint(s[2:], 16, 64)
	}
	return strconv.ParseUint(s, 10, 64)
}

// formatObjectID formats an object ID as hex string.
func formatObjectID(id uint64) string {
	return fmt.Sprintf("0x%x", id)
}

// parseInt parses a string as int.
func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}
