package ingress

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/perf-analysis/internal/repository"
	"github.com/perf-analysis/pkg/config"
	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// HTTPTaskRequest represents an incoming task request via HTTP.
type HTTPTaskRequest struct {
	Task     *model.Task       `json:"task"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// HTTPTaskResponse represents the response for a task submission.
type HTTPTaskResponse struct {
	Success bool   `json:"success"`
	TaskID  string `json:"task_id,omitempty"`
	Message string `json:"message,omitempty"`
}

// HTTPIngress implements TaskIngress for HTTP-based task submission.
// It receives tasks via HTTP POST, applies callback URL downgrade-save,
// and persists them to the database via TaskRepository.CreateTask().
type HTTPIngress struct {
	cfg      *config.HTTPIngressConfig
	taskRepo repository.TaskRepository
	logger   utils.Logger

	server *http.Server

	mu      sync.RWMutex
	running bool
}

// NewHTTPIngress creates a new HTTPIngress instance.
func NewHTTPIngress(cfg *config.HTTPIngressConfig, taskRepo repository.TaskRepository, logger utils.Logger) *HTTPIngress {
	return &HTTPIngress{
		cfg:      cfg,
		taskRepo: taskRepo,
		logger:   logger,
	}
}

// Start starts the HTTP ingress server.
func (h *HTTPIngress) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return nil
	}
	h.running = true
	h.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc(h.cfg.Path, h.handleTask)
	mux.HandleFunc("/health", h.handleHealth)

	h.server = &http.Server{
		Addr:         h.cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  h.cfg.GetReadTimeout(),
		WriteTimeout: h.cfg.GetWriteTimeout(),
	}

	h.logger.Info("HTTP ingress starting on %s%s", h.cfg.ListenAddr, h.cfg.Path)

	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			h.logger.Error("HTTP ingress server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the HTTP ingress server.
func (h *HTTPIngress) Stop(ctx context.Context) error {
	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return nil
	}
	h.running = false
	h.mu.Unlock()

	if h.server != nil {
		return h.server.Shutdown(ctx)
	}
	return nil
}

// handleTask handles incoming task submissions.
func (h *HTTPIngress) handleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, http.StatusMethodNotAllowed, "only POST method is allowed")
		return
	}

	// Limit request body size
	maxBody := h.cfg.MaxBodySize
	if maxBody <= 0 {
		maxBody = 1 << 20 // 1MB default
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var req HTTPTaskRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Task == nil {
		h.sendError(w, http.StatusBadRequest, "task is required")
		return
	}

	// Validate required fields
	if req.Task.TaskUUID == "" {
		h.sendError(w, http.StatusBadRequest, "task.tid is required")
		return
	}

	// Callback URL downgrade-save: if task has no callback_url,
	// use the ingress-level configured callback_url.
	if req.Task.CallbackURL == "" && h.cfg.CallbackURL != "" {
		req.Task.CallbackURL = h.cfg.CallbackURL
	}

	// Set default status for new tasks
	if req.Task.Status == 0 {
		req.Task.Status = model.TaskStatusCompleted // ready for analysis
	}
	if req.Task.AnalysisStatus == 0 {
		req.Task.AnalysisStatus = model.AnalysisStatusPending
	}
	if req.Task.CreateTime.IsZero() {
		req.Task.CreateTime = time.Now()
	}

	// Persist to database
	if err := h.taskRepo.CreateTask(r.Context(), req.Task); err != nil {
		h.logger.Error("Failed to create task %s: %v", req.Task.TaskUUID, err)
		h.sendError(w, http.StatusInternalServerError, "failed to create task")
		return
	}

	h.logger.Info("HTTP ingress received and persisted task %s (id: %d)", req.Task.TaskUUID, req.Task.ID)
	h.sendSuccess(w, req.Task.TaskUUID, "task created and queued for analysis")
}

// handleHealth handles health check requests.
func (h *HTTPIngress) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": "ingress",
	})
}

// sendError sends an error response.
func (h *HTTPIngress) sendError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(HTTPTaskResponse{
		Success: false,
		Message: message,
	})
}

// sendSuccess sends a success response.
func (h *HTTPIngress) sendSuccess(w http.ResponseWriter, taskID, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(HTTPTaskResponse{
		Success: true,
		TaskID:  taskID,
		Message: message,
	})
}

// Compile-time check: HTTPIngress implements TaskIngress.
var _ TaskIngress = (*HTTPIngress)(nil)
