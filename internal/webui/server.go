package webui

import (
	"compress/gzip"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/perf-analysis/pkg/utils"
)

//go:embed templates/*
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server represents the web UI server
type Server struct {
	dataDir         string
	port            int
	logger          utils.Logger
	server          *http.Server
	refGraphService *RefGraphService
	fgService       *FlameGraphService
}

// NewServer creates a new web UI server
func NewServer(dataDir string, port int, logger utils.Logger) *Server {
	fgService := NewFlameGraphService(dataDir)
	// Register additional loaders for memory and tracing
	fgService.RegisterLoader(NewMemoryFlameGraphLoader())
	fgService.RegisterLoader(NewTracingFlameGraphLoader())
	// Register pprof loaders
	fgService.RegisterLoader(NewPProfGoroutineFlameGraphLoader())
	fgService.RegisterLoader(NewPProfHeapInuseFlameGraphLoader())
	fgService.RegisterLoader(NewPProfHeapAllocFlameGraphLoader())
	fgService.RegisterLoader(NewPProfBlockFlameGraphLoader())
	fgService.RegisterLoader(NewPProfMutexFlameGraphLoader())

	return &Server{
		dataDir:         dataDir,
		port:            port,
		logger:          logger,
		refGraphService: NewRefGraphService(dataDir),
		fgService:       fgService,
	}
}

// Start starts the web server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Static file server (CSS, JS)
	// Use fs.Sub to strip the "static" prefix from the embedded filesystem
	staticSubFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("failed to create static sub-filesystem: %w", err)
	}
	staticHandler := http.FileServer(http.FS(staticSubFS))
	mux.Handle("/static/", http.StripPrefix("/static/", staticHandler))

	// API routes
	mux.HandleFunc("/api/summary", s.handleSummary)
	mux.HandleFunc("/api/flamegraph", s.handleFlameGraph)
	mux.HandleFunc("/api/callgraph", s.handleCallGraph)
	mux.HandleFunc("/api/tasks", s.handleListTasks)
	mux.HandleFunc("/api/retainers", s.handleRetainers)
	mux.HandleFunc("/api/biggest-objects", s.handleBiggestObjects)
	mux.HandleFunc("/api/object-fields", s.handleObjectFields)
	
	// Enhanced heap analysis APIs (using ReferenceGraph)
	mux.HandleFunc("/api/refgraph/fields", s.handleRefGraphFields)
	mux.HandleFunc("/api/refgraph/info", s.handleRefGraphObjectInfo)
	mux.HandleFunc("/api/refgraph/gc-roots", s.handleRefGraphGCRoots)
	mux.HandleFunc("/api/refgraph/gc-roots-summary", s.handleRefGraphGCRootsSummary)
	mux.HandleFunc("/api/refgraph/gc-roots-list", s.handleRefGraphGCRootsList)
	mux.HandleFunc("/api/refgraph/gc-root-retained", s.handleRefGraphGCRootRetained)
	mux.HandleFunc("/api/refgraph/retainers", s.handleRefGraphRetainers)
	mux.HandleFunc("/api/refgraph/biggest-by-class", s.handleRefGraphBiggestByClass)

	// pprof analysis APIs
	mux.HandleFunc("/api/pprof/leak-report", s.handlePProfLeakReport)
	mux.HandleFunc("/api/pprof/batch-analysis", s.handlePProfBatchAnalysis)

	// Page routes
	mux.HandleFunc("/", s.handleIndex)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	s.logger.Info("Starting web server at http://localhost:%d", s.port)
	s.logger.Info("Serving data from: %s", s.dataDir)
	s.logger.Info("Press Ctrl+C to stop")

	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// handleIndex serves the main HTML page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Use modular template if available, fallback to original
	tmpl, err := template.ParseFS(templatesFS, "templates/index_modular.html")
	if err != nil {
		// Fallback to original index.html
		tmpl, err = template.ParseFS(templatesFS, "templates/index.html")
		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
			s.logger.Error("Failed to parse template: %v", err)
			return
		}
	}

	data := map[string]interface{}{
		"DataDir": s.dataDir,
		"Port":    s.port,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		s.logger.Error("Failed to execute template: %v", err)
	}
}

// handleSummary returns the analysis summary
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	summaryFile := filepath.Join(s.dataDir, taskID, "summary.json")
	if taskID != "" && !strings.Contains(taskID, "/") {
		// Task ID provided, look in task subdirectory
		summaryFile = filepath.Join(s.dataDir, taskID, "summary.json")
	} else {
		// Direct data directory
		summaryFile = filepath.Join(s.dataDir, "summary.json")
	}

	data, err := os.ReadFile(summaryFile)
	if err != nil {
		http.Error(w, "Summary not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(data)
}

// handleFlameGraph returns flame graph data.
// Supports multiple flame graph types via the "type" query parameter:
// - cpu (default): CPU profiling flame graph
// - memory/alloc: Memory allocation flame graph
// - tracing/latency: Tracing/latency flame graph
// - pprof-goroutine: Go pprof goroutine flame graph
// - pprof-heap-inuse: Go pprof heap inuse flame graph
// - pprof-heap-alloc: Go pprof heap alloc flame graph
// - pprof-block: Go pprof block flame graph
// - pprof-mutex: Go pprof mutex flame graph
func (s *Server) handleFlameGraph(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	// Determine flame graph type
	fgTypeStr := r.URL.Query().Get("type")
	fgType := FlameGraphTypeCPU // default
	switch strings.ToLower(fgTypeStr) {
	case "memory", "alloc", "heap":
		fgType = FlameGraphTypeMemory
	case "tracing", "latency", "wall":
		fgType = FlameGraphTypeTracing
	case "cpu", "":
		fgType = FlameGraphTypeCPU
	case "pprof-goroutine", "goroutine":
		fgType = FlameGraphTypePProfGoroutine
	case "pprof-heap-inuse", "heap-inuse", "inuse":
		fgType = FlameGraphTypePProfHeapInuse
	case "pprof-heap-alloc", "heap-alloc":
		fgType = FlameGraphTypePProfHeapAlloc
	case "pprof-block", "block":
		fgType = FlameGraphTypePProfBlock
	case "pprof-mutex", "mutex":
		fgType = FlameGraphTypePProfMutex
	default:
		// Unknown type, try to find any .json.gz file (legacy behavior)
		s.handleFlameGraphLegacy(w, r, taskID)
		return
	}

	// Use FlameGraphService to load the flame graph
	ctx := r.Context()
	fg, err := s.fgService.GetFlameGraph(ctx, taskID, fgType)
	if err != nil {
		// Fall back to legacy behavior for backward compatibility
		s.handleFlameGraphLegacy(w, r, taskID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(fg)
}

// handleFlameGraphLegacy provides backward compatible flame graph loading.
// It directly reads .json.gz files without type-specific processing.
func (s *Server) handleFlameGraphLegacy(w http.ResponseWriter, r *http.Request, taskID string) {
	taskDir := filepath.Join(s.dataDir, taskID)
	if taskID == "" {
		taskDir = s.dataDir
	}

	// Find the flame graph file (*.json.gz)
	files, err := os.ReadDir(taskDir)
	if err != nil {
		http.Error(w, "Task directory not found", http.StatusNotFound)
		return
	}

	var flameFile string
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json.gz") {
			flameFile = filepath.Join(taskDir, f.Name())
			break
		}
	}

	if flameFile == "" {
		http.Error(w, "Flame graph file not found", http.StatusNotFound)
		return
	}

	// Read and decompress
	file, err := os.Open(flameFile)
	if err != nil {
		http.Error(w, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		http.Error(w, "Failed to decompress", http.StatusInternalServerError)
		return
	}
	defer gzReader.Close()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	io.Copy(w, gzReader)
}

// handleCallGraph returns call graph data
func (s *Server) handleCallGraph(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	// Determine call graph type from query parameter
	cgType := r.URL.Query().Get("type")

	taskDir := filepath.Join(s.dataDir, taskID)
	if taskID == "" {
		taskDir = s.dataDir
	}

	// Try to find call graph file in order of priority based on type
	callGraphFile := ""
	isGzipped := false

	// Build priority list based on type
	var priorityFiles []string
	var subDirs []string
	switch strings.ToLower(cgType) {
	case "memory", "alloc":
		subDirs = []string{"heap", "."}
		priorityFiles = []string{
			"alloc_callgraph_data.json.gz", // New format for memory
			"alloc_callgraph.json.gz",      // Alternative
			"memory_callgraph.json.gz",     // Legacy
		}
	default: // cpu or empty
		subDirs = []string{"cpu", "."}
		priorityFiles = []string{
			"callgraph_data.json.gz", // New format for CPU
			"callgraph.json",         // Legacy format
		}
	}

	// Try priority files first in each subdirectory
	for _, subDir := range subDirs {
		dir := filepath.Join(taskDir, subDir)
		for _, fileName := range priorityFiles {
			filePath := filepath.Join(dir, fileName)
			if _, err := os.Stat(filePath); err == nil {
				callGraphFile = filePath
				isGzipped = strings.HasSuffix(fileName, ".gz")
				break
			}
		}
		if callGraphFile != "" {
			break
		}
	}

	// Fallback: search for any callgraph file
	if callGraphFile == "" {
		files, err := os.ReadDir(taskDir)
		if err != nil {
			http.Error(w, "Task directory not found", http.StatusNotFound)
			return
		}

		for _, f := range files {
			name := f.Name()
			// Match callgraph files
			if strings.Contains(name, "callgraph") {
				if strings.HasSuffix(name, ".json.gz") {
					callGraphFile = filepath.Join(taskDir, name)
					isGzipped = true
					break
				} else if strings.HasSuffix(name, ".json") && name != "summary.json" {
					callGraphFile = filepath.Join(taskDir, name)
					break
				}
			}
		}
	}

	if callGraphFile == "" {
		http.Error(w, "Call graph file not found", http.StatusNotFound)
		return
	}

	// Read file (handle gzip if needed)
	var data []byte
	var err error

	if isGzipped {
		file, err := os.Open(callGraphFile)
		if err != nil {
			http.Error(w, "Failed to open file", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		gzReader, err := gzip.NewReader(file)
		if err != nil {
			http.Error(w, "Failed to decompress", http.StatusInternalServerError)
			return
		}
		defer gzReader.Close()

		data, err = io.ReadAll(gzReader)
		if err != nil {
			http.Error(w, "Failed to read compressed file", http.StatusInternalServerError)
			return
		}
	} else {
		data, err = os.ReadFile(callGraphFile)
		if err != nil {
			http.Error(w, "Failed to read file", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(data)
}

// handleListTasks lists all available tasks
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		http.Error(w, "Failed to list tasks", http.StatusInternalServerError)
		return
	}

	type TaskInfo struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
		HasData   bool   `json:"has_data"`
	}

	var tasks []TaskInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		taskDir := filepath.Join(s.dataDir, entry.Name())
		summaryFile := filepath.Join(taskDir, "summary.json")

		info, _ := entry.Info()
		createdAt := ""
		if info != nil {
			createdAt = info.ModTime().Format(time.RFC3339)
		}

		_, hasData := os.Stat(summaryFile)
		tasks = append(tasks, TaskInfo{
			ID:        entry.Name(),
			CreatedAt: createdAt,
			HasData:   hasData == nil,
		})
	}

	// Sort by creation time (newest first)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt > tasks[j].CreatedAt
	})

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(tasks)
}

// getDefaultTask returns the most recent task ID
func (s *Server) getDefaultTask() string {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return ""
	}

	var latest string
	var latestTime time.Time

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = entry.Name()
		}
	}

	return latest
}

// handleRetainers returns detailed retainer analysis data for heap analysis
func (s *Server) handleRetainers(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	taskDir := filepath.Join(s.dataDir, taskID)
	if taskID == "" {
		taskDir = s.dataDir
	}

	// Try multiple sources for retainer data
	var data []byte
	var err error

	// 1. Try retainer_analysis.json first
	retainerFile := filepath.Join(taskDir, "retainer_analysis.json")
	data, err = os.ReadFile(retainerFile)
	if err != nil {
		// 2. Try heap_analysis.json
		heapFile := filepath.Join(taskDir, "heap_analysis.json")
		data, err = os.ReadFile(heapFile)
		if err != nil {
			// 3. Fall back to extracting from summary.json
			summaryFile := filepath.Join(taskDir, "summary.json")
			summaryData, summaryErr := os.ReadFile(summaryFile)
			if summaryErr != nil {
				http.Error(w, "Retainer data not found", http.StatusNotFound)
				return
			}

			// Parse summary and extract retainer-related data
			var summary map[string]interface{}
			if jsonErr := json.Unmarshal(summaryData, &summary); jsonErr != nil {
				http.Error(w, "Failed to parse summary", http.StatusInternalServerError)
				return
			}

			// Extract data section which contains retainer info
			retainerData := make(map[string]interface{})
			if dataSection, ok := summary["data"].(map[string]interface{}); ok {
				if businessRetainers, ok := dataSection["business_retainers"]; ok {
					retainerData["business_retainers"] = businessRetainers
				}
				if referenceGraphs, ok := dataSection["reference_graphs"]; ok {
					retainerData["reference_graphs"] = referenceGraphs
				}
				if topClasses, ok := dataSection["top_classes"]; ok {
					retainerData["top_classes"] = topClasses
				}
			}

			data, err = json.Marshal(retainerData)
			if err != nil {
				http.Error(w, "Failed to marshal retainer data", http.StatusInternalServerError)
				return
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(data)
}

// handleBiggestObjects returns the biggest objects data for heap analysis
func (s *Server) handleBiggestObjects(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	taskDir := filepath.Join(s.dataDir, taskID)
	if taskID == "" {
		taskDir = s.dataDir
	}

	// Query parameters
	className := r.URL.Query().Get("class")
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "retained"
	}

	var data []byte
	var err error

	// Try to read from biggest_objects.json file first
	biggestObjectsFile := filepath.Join(taskDir, "biggest_objects.json")
	data, err = os.ReadFile(biggestObjectsFile)
	if err != nil {
		// Fall back to extracting from summary.json
		summaryFile := filepath.Join(taskDir, "summary.json")
		summaryData, summaryErr := os.ReadFile(summaryFile)
		if summaryErr != nil {
			http.Error(w, "Biggest objects data not found", http.StatusNotFound)
			return
		}

		// Parse summary and extract biggest_objects
		var summary map[string]interface{}
		if jsonErr := json.Unmarshal(summaryData, &summary); jsonErr != nil {
			http.Error(w, "Failed to parse summary", http.StatusInternalServerError)
			return
		}

		// Extract biggest_objects from data section
		if dataSection, ok := summary["data"].(map[string]interface{}); ok {
			if biggestObjects, ok := dataSection["biggest_objects"]; ok {
				// Filter by class if specified
				if className != "" {
					if objects, ok := biggestObjects.([]interface{}); ok {
						var filtered []interface{}
						for _, obj := range objects {
							if objMap, ok := obj.(map[string]interface{}); ok {
								if objClassName, ok := objMap["class_name"].(string); ok {
									if objClassName == className {
										filtered = append(filtered, obj)
									}
								}
							}
						}
						biggestObjects = filtered
					}
				}

				data, err = json.Marshal(biggestObjects)
				if err != nil {
					http.Error(w, "Failed to marshal biggest objects", http.StatusInternalServerError)
					return
				}
			} else {
				http.Error(w, "Biggest objects not found in summary", http.StatusNotFound)
				return
			}
		} else {
			http.Error(w, "Data section not found in summary", http.StatusNotFound)
			return
		}
	} else if className != "" {
		// Filter the loaded data by class name
		var objects []interface{}
		if jsonErr := json.Unmarshal(data, &objects); jsonErr == nil {
			var filtered []interface{}
			for _, obj := range objects {
				if objMap, ok := obj.(map[string]interface{}); ok {
					if objClassName, ok := objMap["class_name"].(string); ok {
						if objClassName == className {
							filtered = append(filtered, obj)
						}
					}
				}
			}
			data, _ = json.Marshal(filtered)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(data)
}

// handleObjectFields returns the fields of a specific object for tree expansion
func (s *Server) handleObjectFields(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	objectIDStr := r.URL.Query().Get("id")
	if objectIDStr == "" {
		http.Error(w, "Object ID is required", http.StatusBadRequest)
		return
	}

	taskDir := filepath.Join(s.dataDir, taskID)
	if taskID == "" {
		taskDir = s.dataDir
	}

	// Try to read from object_fields cache file
	// Format: object_fields_<objectID>.json
	cacheFile := filepath.Join(taskDir, "object_fields_"+objectIDStr+".json")
	data, err := os.ReadFile(cacheFile)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(data)
		return
	}

	// If cache doesn't exist, try to get from summary.json biggest_objects
	summaryFile := filepath.Join(taskDir, "summary.json")
	summaryData, err := os.ReadFile(summaryFile)
	if err != nil {
		http.Error(w, "Summary data not found", http.StatusNotFound)
		return
	}

	var summary map[string]interface{}
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		http.Error(w, "Failed to parse summary", http.StatusInternalServerError)
		return
	}

	// Look for the object in biggest_objects
	if dataSection, ok := summary["data"].(map[string]interface{}); ok {
		if biggestObjects, ok := dataSection["biggest_objects"].([]interface{}); ok {
			for _, obj := range biggestObjects {
				if objMap, ok := obj.(map[string]interface{}); ok {
					if objID, ok := objMap["object_id"].(string); ok && objID == objectIDStr {
						// Found the object, return its fields
						if fields, ok := objMap["fields"]; ok {
							// Enhance fields with has_children info
							if fieldsList, ok := fields.([]interface{}); ok {
								for _, f := range fieldsList {
									if fieldMap, ok := f.(map[string]interface{}); ok {
										// If it has ref_id, it potentially has children
										if _, hasRef := fieldMap["ref_id"]; hasRef {
											fieldMap["has_children"] = true
										}
									}
								}
							}
							data, _ = json.Marshal(fields)
							w.Header().Set("Content-Type", "application/json")
							w.Header().Set("Access-Control-Allow-Origin", "*")
							w.Write(data)
							return
						}
					}
					// Also check nested fields for the object
					if fields, ok := objMap["fields"].([]interface{}); ok {
						for _, f := range fields {
							if fieldMap, ok := f.(map[string]interface{}); ok {
								if refID, ok := fieldMap["ref_id"].(string); ok && refID == objectIDStr {
									// This is a child object, return empty fields for now
									// In production, we'd need to look up this object's fields
									data = []byte("[]")
									w.Header().Set("Content-Type", "application/json")
									w.Header().Set("Access-Control-Allow-Origin", "*")
									w.Write(data)
									return
								}
							}
						}
					}
				}
			}
		}
	}

	// Object not found - return empty array instead of error for better UX
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write([]byte("[]"))
}

// handleRefGraphFields returns the fields of a specific object using ReferenceGraph.
// This enables deep object exploration beyond the initial biggest_objects.json data.
func (s *Server) handleRefGraphFields(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	objectIDStr := r.URL.Query().Get("id")
	if objectIDStr == "" {
		http.Error(w, "Object ID is required", http.StatusBadRequest)
		return
	}

	fields, err := s.refGraphService.GetObjectFields(taskID, objectIDStr)
	if err != nil {
		// Fall back to legacy method if refgraph not available
		s.handleObjectFields(w, r)
		return
	}

	// Convert to JSON-friendly format with string object IDs
	type FieldResponse struct {
		Name         string      `json:"name"`
		Type         string      `json:"type"`
		Value        interface{} `json:"value,omitempty"`
		RefID        string      `json:"ref_id,omitempty"`
		RefClass     string      `json:"ref_class,omitempty"`
		ShallowSize  int64       `json:"shallow_size,omitempty"`
		RetainedSize int64       `json:"retained_size,omitempty"`
		HasChildren  bool        `json:"has_children"`
	}

	response := make([]FieldResponse, 0, len(fields))
	for _, f := range fields {
		fr := FieldResponse{
			Name:         f.Name,
			Type:         f.Type,
			Value:        f.Value,
			RefClass:     f.RefClass,
			ShallowSize:  f.ShallowSize,
			RetainedSize: f.RetainedSize,
			HasChildren:  f.HasChildren,
		}
		if f.RefID != 0 {
			fr.RefID = formatObjectID(f.RefID)
		}
		response = append(response, fr)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(response)
}

// handleRefGraphObjectInfo returns basic information about an object.
func (s *Server) handleRefGraphObjectInfo(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	objectIDStr := r.URL.Query().Get("id")
	if objectIDStr == "" {
		http.Error(w, "Object ID is required", http.StatusBadRequest)
		return
	}

	info, err := s.refGraphService.GetObjectInfo(taskID, objectIDStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Convert to JSON-friendly format
	response := map[string]interface{}{
		"object_id":     formatObjectID(info.RefID),
		"class_name":    info.RefClass,
		"shallow_size":  info.ShallowSize,
		"retained_size": info.RetainedSize,
		"has_children":  info.HasChildren,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(response)
}

// handleRefGraphGCRoots returns the GC root paths for a specific object.
func (s *Server) handleRefGraphGCRoots(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	objectIDStr := r.URL.Query().Get("id")
	if objectIDStr == "" {
		http.Error(w, "Object ID is required", http.StatusBadRequest)
		return
	}

	maxPaths := 3
	if mp := r.URL.Query().Get("max_paths"); mp != "" {
		if n, err := parseInt(mp); err == nil && n > 0 {
			maxPaths = n
		}
	}

	maxDepth := 15
	if md := r.URL.Query().Get("max_depth"); md != "" {
		if n, err := parseInt(md); err == nil && n > 0 {
			maxDepth = n
		}
	}

	paths, err := s.refGraphService.GetGCRootPaths(taskID, objectIDStr, maxPaths, maxDepth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(paths)
}

// handleRefGraphGCRootsSummary returns GC roots grouped by class (like IDEA).
// First tries to read from gc_roots.json, falls back to refgraph if not available.
func (s *Server) handleRefGraphGCRootsSummary(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	// Try to read from gc_roots.json first (fast path)
	taskDir := filepath.Join(s.dataDir, taskID)
	gcRootsFile := filepath.Join(taskDir, "gc_roots.json")
	if data, err := os.ReadFile(gcRootsFile); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(data)
		return
	}

	// Fall back to refgraph (slow path - requires loading refgraph.bin)
	summary, err := s.refGraphService.GetGCRootsSummary(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(summary)
}

// handleRefGraphGCRootsList returns all GC roots with their information.
func (s *Server) handleRefGraphGCRootsList(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	roots, err := s.refGraphService.GetGCRootsList(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(roots)
}

// handleRefGraphGCRootRetained returns objects retained by a specific GC root.
func (s *Server) handleRefGraphGCRootRetained(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	objectIDStr := r.URL.Query().Get("id")
	if objectIDStr == "" {
		http.Error(w, "Object ID is required", http.StatusBadRequest)
		return
	}

	maxObjects := 50
	if mo := r.URL.Query().Get("max"); mo != "" {
		if n, err := parseInt(mo); err == nil && n > 0 {
			maxObjects = n
		}
	}

	objects, err := s.refGraphService.GetRetainedObjectsByGCRoot(taskID, objectIDStr, maxObjects)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(objects)
}

// handleRefGraphRetainers returns the objects that retain a specific object.
func (s *Server) handleRefGraphRetainers(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	objectIDStr := r.URL.Query().Get("id")
	if objectIDStr == "" {
		http.Error(w, "Object ID is required", http.StatusBadRequest)
		return
	}

	maxRetainers := 20
	if mr := r.URL.Query().Get("max"); mr != "" {
		if n, err := parseInt(mr); err == nil && n > 0 {
			maxRetainers = n
		}
	}

	retainers, err := s.refGraphService.GetRetainers(taskID, objectIDStr, maxRetainers)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(retainers)
}

// handleRefGraphBiggestByClass returns the biggest objects for a specific class.
func (s *Server) handleRefGraphBiggestByClass(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

	className := r.URL.Query().Get("class")
	if className == "" {
		http.Error(w, "Class name is required", http.StatusBadRequest)
		return
	}

	topN := 50
	if tn := r.URL.Query().Get("top"); tn != "" {
		if n, err := parseInt(tn); err == nil && n > 0 {
			topN = n
		}
	}

	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "retained"
	}

	objects, err := s.refGraphService.GetBiggestObjectsByClass(taskID, className, topN, sortBy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Convert to JSON-friendly format
	type ObjectResponse struct {
		ObjectID     string `json:"object_id"`
		ClassName    string `json:"class_name"`
		ShallowSize  int64  `json:"shallow_size"`
		RetainedSize int64  `json:"retained_size"`
	}

	response := make([]ObjectResponse, 0, len(objects))
	for _, obj := range objects {
		response = append(response, ObjectResponse{
			ObjectID:     formatObjectID(obj.ObjectID),
			ClassName:    obj.ClassName,
			ShallowSize:  obj.ShallowSize,
			RetainedSize: obj.RetainedSize,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(response)
}

// parseInt parses an integer from a string.
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// handlePProfLeakReport returns the pprof leak detection report.
func (s *Server) handlePProfLeakReport(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")

	leakType := r.URL.Query().Get("type")
	if leakType == "" {
		leakType = "all" // Return all leak reports
	}

	// Determine task directory
	var taskDir string
	if taskID != "" {
		taskDir = filepath.Join(s.dataDir, taskID)
	} else {
		taskDir = s.dataDir
	}

	// Try to read batch_analysis.json which contains leak reports
	// First try in the task directory, then in subdirectories
	batchFile := filepath.Join(taskDir, "batch_analysis.json")
	data, err := os.ReadFile(batchFile)
	if err != nil {
		// No batch analysis file, return empty
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write([]byte(`{"leak_reports":{}}`))
		return
	}

	var batchResult map[string]interface{}
	if err := json.Unmarshal(data, &batchResult); err != nil {
		http.Error(w, "Failed to parse batch analysis", http.StatusInternalServerError)
		return
	}

	// Extract leak reports
	leakReports, ok := batchResult["leak_reports"].(map[string]interface{})
	if !ok {
		leakReports = make(map[string]interface{})
	}

	// Filter by type if specified
	if leakType != "all" {
		if report, exists := leakReports[leakType]; exists {
			leakReports = map[string]interface{}{leakType: report}
		} else {
			leakReports = make(map[string]interface{})
		}
	}

	response := map[string]interface{}{
		"leak_reports": leakReports,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(response)
}

// handlePProfBatchAnalysis returns the complete pprof batch analysis result.
func (s *Server) handlePProfBatchAnalysis(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")

	// Determine task directory
	var taskDir string
	if taskID != "" {
		taskDir = filepath.Join(s.dataDir, taskID)
	} else {
		taskDir = s.dataDir
	}

	// Try to read batch_analysis.json
	batchFile := filepath.Join(taskDir, "batch_analysis.json")
	data, err := os.ReadFile(batchFile)
	if err != nil {
		http.Error(w, "Batch analysis not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(data)
}
