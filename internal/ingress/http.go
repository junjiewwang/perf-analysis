package ingress

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/junjiewwang/perf-analysis/internal/repository"
	"github.com/junjiewwang/perf-analysis/pkg/config"
	"github.com/junjiewwang/perf-analysis/pkg/model"
	"github.com/junjiewwang/perf-analysis/pkg/utils"
)

// TaskSubmission is the inbound DTO for task submission via HTTP.
// It decouples the external API contract from the internal model.Task,
// exposing only the fields callers need to provide.
type TaskSubmission struct {
	TaskUUID      string            `json:"tid"`
	Profiler      string            `json:"profiler"`
	Event         string            `json:"event"`
	ResultFile    string            `json:"result_file"`
	UserName      string            `json:"user_name,omitempty"`
	MasterTaskTID *string           `json:"mastertask_tid,omitempty"`
	COSBucket     string            `json:"cos_bucket,omitempty"`
	CallbackURL   string            `json:"callback_url,omitempty"`
	RequestParams *model.RequestParams `json:"request_params,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// Validate validates the task submission fields.
func (s *TaskSubmission) Validate() error {
	if s.TaskUUID == "" {
		return fmt.Errorf("tid is required")
	}
	if s.Profiler == "" {
		return fmt.Errorf("profiler is required")
	}
	if s.Event == "" {
		return fmt.Errorf("event is required")
	}
	profiler := model.Profiler(s.Profiler)
	if !profiler.IsValid() {
		return fmt.Errorf("invalid profiler: %s", s.Profiler)
	}
	event := model.EventType(s.Event)
	if !event.IsValid() {
		return fmt.Errorf("invalid event: %s", s.Event)
	}
	return nil
}

// ToTask converts the DTO to an internal model.Task.
func (s *TaskSubmission) ToTask() *model.Task {
	profiler := model.Profiler(s.Profiler)
	event := model.EventType(s.Event)

	task := &model.Task{
		TaskUUID:      s.TaskUUID,
		Profiler:      profiler,
		Event:         event,
		Mode:          model.AnalysisMode(profiler, event),
		ResultFile:    s.ResultFile,
		UserName:      s.UserName,
		MasterTaskTID: s.MasterTaskTID,
		COSBucket:     s.COSBucket,
		CallbackURL:   s.CallbackURL,
		Metadata:      s.Metadata,
	}

	if s.RequestParams != nil {
		task.RequestParams = *s.RequestParams
	}

	return task
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

	var submission TaskSubmission
	if err := json.Unmarshal(body, &submission); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Validate the submission DTO
	if err := submission.Validate(); err != nil {
		h.sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Convert DTO to internal model
	task := submission.ToTask()

	// Callback URL downgrade-save: if task has no callback_url,
	// use the ingress-level configured callback_url.
	if task.CallbackURL == "" && h.cfg.CallbackURL != "" {
		task.CallbackURL = h.cfg.CallbackURL
	}

	// Set default status for new tasks
	task.Status = model.TaskStatusCompleted // ready for analysis
	task.AnalysisStatus = model.AnalysisStatusPending
	task.CreateTime = time.Now()

	// Persist to database
	if err := h.taskRepo.CreateTask(r.Context(), task); err != nil {
		h.logger.Error("Failed to create task %s: %v", task.TaskUUID, err)
		h.sendError(w, http.StatusInternalServerError, "failed to create task")
		return
	}

	h.logger.Info("HTTP ingress received and persisted task %s (id: %d, mode: %s)",
		task.TaskUUID, task.ID, task.Mode)
	h.sendSuccess(w, task.TaskUUID, "task created and queued for analysis")
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
