package webui

import (
	"compress/gzip"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
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

// Server represents the web UI server
type Server struct {
	dataDir string
	port    int
	logger  utils.Logger
	server  *http.Server
}

// NewServer creates a new web UI server
func NewServer(dataDir string, port int, logger utils.Logger) *Server {
	return &Server{
		dataDir: dataDir,
		port:    port,
		logger:  logger,
	}
}

// Start starts the web server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/summary", s.handleSummary)
	mux.HandleFunc("/api/flamegraph", s.handleFlameGraph)
	mux.HandleFunc("/api/callgraph", s.handleCallGraph)
	mux.HandleFunc("/api/tasks", s.handleListTasks)

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
	tmpl, err := template.ParseFS(templatesFS, "templates/index.html")
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		s.logger.Error("Failed to parse template: %v", err)
		return
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

// handleFlameGraph returns flame graph data
func (s *Server) handleFlameGraph(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		taskID = s.getDefaultTask()
	}

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

	taskDir := filepath.Join(s.dataDir, taskID)
	if taskID == "" {
		taskDir = s.dataDir
	}

	// Find the call graph file (*.json but not .json.gz)
	files, err := os.ReadDir(taskDir)
	if err != nil {
		http.Error(w, "Task directory not found", http.StatusNotFound)
		return
	}

	var callGraphFile string
	for _, f := range files {
		name := f.Name()
		if strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".gz") && name != "summary.json" {
			callGraphFile = filepath.Join(taskDir, name)
			break
		}
	}

	if callGraphFile == "" {
		http.Error(w, "Call graph file not found", http.StatusNotFound)
		return
	}

	data, err := os.ReadFile(callGraphFile)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
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
