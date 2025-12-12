package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// SourceTypeHTTP is the source type constant for HTTP source.
const SourceTypeHTTP SourceType = "http"

func init() {
	// Register the HTTP source strategy
	Register(SourceTypeHTTP, NewHTTPSource)
}

// HTTPOptions holds HTTP source specific configuration.
type HTTPOptions struct {
	// ListenAddr is the address to listen on (e.g., ":8080").
	ListenAddr string

	// Path is the HTTP path for receiving tasks.
	Path string

	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration

	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration

	// MaxBodySize is the maximum allowed request body size in bytes.
	MaxBodySize int64
}

// DefaultHTTPOptions returns the default options.
func DefaultHTTPOptions() *HTTPOptions {
	return &HTTPOptions{
		ListenAddr:   ":8080",
		Path:         "/tasks",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		MaxBodySize:  1 << 20, // 1MB
	}
}

// HTTPTaskRequest represents an incoming task request.
type HTTPTaskRequest struct {
	Task     *model.Task       `json:"task"`
	Priority int               `json:"priority,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// HTTPTaskResponse represents the response for a task submission.
type HTTPTaskResponse struct {
	Success bool   `json:"success"`
	TaskID  string `json:"task_id,omitempty"`
	Message string `json:"message,omitempty"`
}

// HTTPSource implements TaskSource for HTTP webhook-based task submission.
type HTTPSource struct {
	name    string
	options *HTTPOptions
	logger  utils.Logger

	server   *http.Server
	taskChan chan *TaskEvent
	stopCh   chan struct{}

	mu      sync.RWMutex
	running bool
}

// NewHTTPSource creates a new HTTP source from configuration.
func NewHTTPSource(cfg *SourceConfig) (TaskSource, error) {
	opts := &HTTPOptions{
		ListenAddr:   cfg.GetString("listen_addr", ":8080"),
		Path:         cfg.GetString("path", "/tasks"),
		ReadTimeout:  cfg.GetDuration("read_timeout", 30*time.Second),
		WriteTimeout: cfg.GetDuration("write_timeout", 30*time.Second),
		MaxBodySize:  int64(cfg.GetInt("max_body_size", 1<<20)),
	}

	return &HTTPSource{
		name:     cfg.Name,
		options:  opts,
		taskChan: make(chan *TaskEvent, 100),
		stopCh:   make(chan struct{}),
	}, nil
}

// NewHTTPSourceWithOptions creates a new HTTP source with explicit options.
func NewHTTPSourceWithOptions(name string, opts *HTTPOptions, logger utils.Logger) *HTTPSource {
	if opts == nil {
		opts = DefaultHTTPOptions()
	}
	if logger == nil {
		logger = utils.NewDefaultLogger(utils.LevelInfo, nil)
	}

	return &HTTPSource{
		name:     name,
		options:  opts,
		logger:   logger,
		taskChan: make(chan *TaskEvent, 100),
		stopCh:   make(chan struct{}),
	}
}

// SetLogger sets the logger.
func (s *HTTPSource) SetLogger(logger utils.Logger) {
	s.logger = logger
}

// Type returns the source type.
func (s *HTTPSource) Type() SourceType {
	return SourceTypeHTTP
}

// Name returns the source instance name.
func (s *HTTPSource) Name() string {
	return s.name
}

// Start starts the HTTP server.
func (s *HTTPSource) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc(s.options.Path, s.handleTask)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:         s.options.ListenAddr,
		Handler:      mux,
		ReadTimeout:  s.options.ReadTimeout,
		WriteTimeout: s.options.WriteTimeout,
	}

	if s.logger != nil {
		s.logger.Info("HTTP source %s starting on %s%s", s.name, s.options.ListenAddr, s.options.Path)
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if s.logger != nil {
				s.logger.Error("HTTP source %s server error: %v", s.name, err)
			}
		}
	}()

	return nil
}

// Stop stops the HTTP server.
func (s *HTTPSource) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}

	return nil
}

// Tasks returns the task event channel.
func (s *HTTPSource) Tasks() <-chan *TaskEvent {
	return s.taskChan
}

// Ack acknowledges a task has been processed successfully.
// For HTTP source, this is typically a no-op as the response was already sent.
func (s *HTTPSource) Ack(ctx context.Context, event *TaskEvent) error {
	// HTTP is synchronous, ack is handled in the response
	if s.logger != nil {
		s.logger.Debug("HTTP source %s acked task %s", s.name, event.ID)
	}
	return nil
}

// Nack indicates a task processing failed.
// For HTTP source, this could trigger a callback if configured.
func (s *HTTPSource) Nack(ctx context.Context, event *TaskEvent, reason string) error {
	// TODO: Optionally call a callback URL if provided in metadata
	// callbackURL := event.GetMetadata("callback_url")
	// if callbackURL != "" {
	//     // POST failure notification to callback URL
	// }

	if s.logger != nil {
		s.logger.Warn("HTTP source %s nacked task %s: %s", s.name, event.ID, reason)
	}
	return nil
}

// HealthCheck checks if the HTTP server is running.
func (s *HTTPSource) HealthCheck(ctx context.Context) error {
	s.mu.RLock()
	running := s.running
	s.mu.RUnlock()

	if !running {
		return fmt.Errorf("HTTP source %s is not running", s.name)
	}
	return nil
}

// handleTask handles incoming task submissions.
func (s *HTTPSource) handleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, http.StatusMethodNotAllowed, "only POST method is allowed")
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, s.options.MaxBodySize)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.sendError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var req HTTPTaskRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.sendError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Task == nil {
		s.sendError(w, http.StatusBadRequest, "task is required")
		return
	}

	// Create task event
	event := NewTaskEvent(req.Task, SourceTypeHTTP, s.name)
	if req.Priority > 0 {
		event.Priority = req.Priority
	}
	for k, v := range req.Metadata {
		event.WithMetadata(k, v)
	}

	// Try to send to channel
	select {
	case s.taskChan <- event:
		s.sendSuccess(w, req.Task.TaskUUID, "task accepted")
		if s.logger != nil {
			s.logger.Debug("HTTP source %s received task %s", s.name, req.Task.TaskUUID)
		}
	default:
		s.sendError(w, http.StatusServiceUnavailable, "task queue is full")
	}
}

// handleHealth handles health check requests.
func (s *HTTPSource) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"source": s.name,
		"type":   string(SourceTypeHTTP),
	})
}

// sendError sends an error response.
func (s *HTTPSource) sendError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(HTTPTaskResponse{
		Success: false,
		Message: message,
	})
}

// sendSuccess sends a success response.
func (s *HTTPSource) sendSuccess(w http.ResponseWriter, taskID, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(HTTPTaskResponse{
		Success: true,
		TaskID:  taskID,
		Message: message,
	})
}
