package pprof

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HTTPMode implements HTTP-based profile collection.
// It exposes pprof endpoints for on-demand profile collection.
type HTTPMode struct {
	config    *HTTPConfig
	collector *Collector

	server *http.Server
	mux    *http.ServeMux

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewHTTPMode creates a new HTTPMode.
func NewHTTPMode(config *HTTPConfig) *HTTPMode {
	if config == nil {
		config = DefaultConfig().HTTPConfig
	}
	return &HTTPMode{
		config: config,
		mux:    http.NewServeMux(),
	}
}

// Name returns the mode name.
func (m *HTTPMode) Name() string {
	return "http"
}

// Start starts the HTTP mode.
func (m *HTTPMode) Start(ctx context.Context, collector *Collector) error {
	m.collector = collector
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Enable block and mutex profiling
	cfg := collector.Config()
	if cfg.HasProfile(ProfileBlock) {
		runtime.SetBlockProfileRate(1)
	}
	if cfg.HasProfile(ProfileMutex) {
		runtime.SetMutexProfileFraction(1)
	}

	// Register handlers
	m.registerHandlers()

	// Create HTTP server
	m.server = &http.Server{
		Addr:         m.config.Addr,
		Handler:      m.mux,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
	}

	// Start server in background
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("pprof HTTP server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the HTTP mode.
func (m *HTTPMode) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}

	// Shutdown HTTP server
	if m.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.server.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown HTTP server: %w", err)
		}
	}

	// Disable block and mutex profiling
	runtime.SetBlockProfileRate(0)
	runtime.SetMutexProfileFraction(0)

	m.wg.Wait()
	return nil
}

// Handler returns the HTTP handler for integration with existing servers.
func (m *HTTPMode) Handler() http.Handler {
	return m.mux
}

func (m *HTTPMode) registerHandlers() {
	path := strings.TrimSuffix(m.config.Path, "/")

	// Wrap handlers with auth if enabled
	wrap := func(h http.HandlerFunc) http.HandlerFunc {
		if m.config.Auth != nil && m.config.Auth.Enabled {
			return m.authMiddleware(h)
		}
		return h
	}

	// Standard pprof handlers
	if m.config.EnableUI {
		m.mux.HandleFunc(path+"/", wrap(pprof.Index))
		m.mux.HandleFunc(path+"/cmdline", wrap(pprof.Cmdline))
		m.mux.HandleFunc(path+"/symbol", wrap(pprof.Symbol))
		m.mux.HandleFunc(path+"/trace", wrap(pprof.Trace))
	}

	// Profile handlers
	m.mux.HandleFunc(path+"/profile", wrap(m.handleCPUProfile))
	m.mux.HandleFunc(path+"/heap", wrap(m.handleHeapProfile))
	m.mux.HandleFunc(path+"/goroutine", wrap(m.handleGoroutineProfile))
	m.mux.HandleFunc(path+"/block", wrap(m.handleBlockProfile))
	m.mux.HandleFunc(path+"/mutex", wrap(m.handleMutexProfile))
	m.mux.HandleFunc(path+"/allocs", wrap(m.handleAllocsProfile))
	m.mux.HandleFunc(path+"/threadcreate", wrap(m.handleThreadCreateProfile))

	// Extended endpoints
	m.mux.HandleFunc(path+"/status", wrap(m.handleStatus))
	m.mux.HandleFunc(path+"/snapshot", wrap(m.handleSnapshot))
}

func (m *HTTPMode) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := m.config.Auth

		// Check token auth
		if auth.Token != "" {
			token := r.Header.Get("Authorization")
			if token == "" {
				token = r.URL.Query().Get("token")
			}
			if token == "Bearer "+auth.Token || token == auth.Token {
				next(w, r)
				return
			}
		}

		// Check basic auth
		if auth.Username != "" && auth.Password != "" {
			user, pass, ok := r.BasicAuth()
			if ok && user == auth.Username && pass == auth.Password {
				next(w, r)
				return
			}
		}

		w.Header().Set("WWW-Authenticate", `Basic realm="pprof"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func (m *HTTPMode) handleCPUProfile(w http.ResponseWriter, r *http.Request) {
	seconds := m.config.DefaultSeconds
	if s := r.URL.Query().Get("seconds"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			seconds = n
		}
	}

	// Limit max duration
	if seconds > 300 {
		seconds = 300
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=cpu_%s.pprof", time.Now().Format("20060102_150405")))

	data, err := m.collector.SnapshotCPU(r.Context(), time.Duration(seconds)*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Save to file if configured
	if m.config.SaveToFile {
		go func() {
			_, _ = m.collector.WriteSnapshot(ProfileCPU, data)
		}()
	}

	w.Write(data)
}

func (m *HTTPMode) handleHeapProfile(w http.ResponseWriter, r *http.Request) {
	m.handleProfile(w, r, ProfileHeap)
}

func (m *HTTPMode) handleGoroutineProfile(w http.ResponseWriter, r *http.Request) {
	m.handleProfile(w, r, ProfileGoroutine)
}

func (m *HTTPMode) handleBlockProfile(w http.ResponseWriter, r *http.Request) {
	m.handleProfile(w, r, ProfileBlock)
}

func (m *HTTPMode) handleMutexProfile(w http.ResponseWriter, r *http.Request) {
	m.handleProfile(w, r, ProfileMutex)
}

func (m *HTTPMode) handleAllocsProfile(w http.ResponseWriter, r *http.Request) {
	m.handleProfile(w, r, ProfileAllocs)
}

func (m *HTTPMode) handleThreadCreateProfile(w http.ResponseWriter, r *http.Request) {
	pprof.Handler("threadcreate").ServeHTTP(w, r)
}

func (m *HTTPMode) handleProfile(w http.ResponseWriter, _ *http.Request, pt ProfileType) {
	data, err := m.collector.Snapshot(pt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_%s.pprof", pt, time.Now().Format("20060102_150405")))

	// Save to file if configured
	if m.config.SaveToFile {
		go func() {
			_, _ = m.collector.WriteSnapshot(pt, data)
		}()
	}

	w.Write(data)
}

func (m *HTTPMode) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := m.collector.Status()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (m *HTTPMode) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse profile types from request
	profilesStr := r.URL.Query().Get("profiles")
	var profiles []ProfileType
	if profilesStr != "" {
		var err error
		profiles, err = ParseProfileTypes(profilesStr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		profiles = m.collector.Config().Profiles
	}

	// Collect and save snapshots
	results := make(map[string]string)
	errors := make(map[string]string)

	for _, pt := range profiles {
		if pt == ProfileCPU {
			// Skip CPU in batch snapshot
			continue
		}

		data, err := m.collector.Snapshot(pt)
		if err != nil {
			errors[string(pt)] = err.Error()
			continue
		}

		filePath, err := m.collector.WriteSnapshot(pt, data)
		if err != nil {
			errors[string(pt)] = err.Error()
			continue
		}

		results[string(pt)] = filePath
	}

	response := map[string]interface{}{
		"files":  results,
		"errors": errors,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
