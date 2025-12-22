package pprof

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
)

// Collector is the main pprof data collector.
type Collector struct {
	config *Config
	mode   Mode
	writer *Writer

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.RWMutex
	status *Status

	// cpuMu ensures only one CPU profile can run at a time
	cpuMu sync.Mutex
}

// Status represents the collector's current status.
type Status struct {
	Running       bool                       `json:"running"`
	Mode          ModeType                   `json:"mode"`
	StartTime     time.Time                  `json:"start_time"`
	SnapshotCount map[ProfileType]int64      `json:"snapshot_count"`
	LastSnapshot  map[ProfileType]time.Time  `json:"last_snapshot"`
	Errors        []string                   `json:"errors"`
}

// Mode defines the interface for pprof collection modes.
type Mode interface {
	// Name returns the mode name.
	Name() string
	// Start starts the mode.
	Start(ctx context.Context, collector *Collector) error
	// Stop stops the mode.
	Stop() error
}

// NewCollector creates a new Collector.
func NewCollector(cfg *Config) (*Collector, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	fileConfig := cfg.FileConfig
	if fileConfig == nil {
		fileConfig = DefaultConfig().FileConfig
	}

	writer := NewWriter(
		cfg.OutputDir,
		fileConfig.MaxFileSize,
		fileConfig.MaxFiles,
		fileConfig.AutoRotate,
	)

	c := &Collector{
		config: cfg,
		writer: writer,
		status: &Status{
			SnapshotCount: make(map[ProfileType]int64),
			LastSnapshot:  make(map[ProfileType]time.Time),
			Errors:        make([]string, 0),
		},
	}

	// Create the appropriate mode
	var mode Mode
	switch cfg.Mode {
	case ModeFile:
		mode = NewFileMode(cfg.FileConfig)
	case ModeHTTP:
		mode = NewHTTPMode(cfg.HTTPConfig)
	default:
		return nil, fmt.Errorf("unknown mode: %s", cfg.Mode)
	}
	c.mode = mode

	return c, nil
}

// Start starts the collector.
func (c *Collector) Start() error {
	c.mu.Lock()
	if c.status.Running {
		c.mu.Unlock()
		return fmt.Errorf("collector is already running")
	}

	// Ensure output directory exists
	if err := c.writer.EnsureDir(c.config.Profiles); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.status.Running = true
	c.status.Mode = c.config.Mode
	c.status.StartTime = time.Now()
	c.mu.Unlock()

	// Start the mode
	if err := c.mode.Start(c.ctx, c); err != nil {
		c.mu.Lock()
		c.status.Running = false
		c.mu.Unlock()
		return fmt.Errorf("failed to start mode: %w", err)
	}

	return nil
}

// Stop stops the collector gracefully.
func (c *Collector) Stop() error {
	c.mu.Lock()
	if !c.status.Running {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	// Cancel context to signal shutdown
	if c.cancel != nil {
		c.cancel()
	}

	// Stop the mode
	if err := c.mode.Stop(); err != nil {
		c.addError(fmt.Sprintf("mode stop error: %v", err))
	}

	// Wait for all goroutines to finish
	c.wg.Wait()

	c.mu.Lock()
	c.status.Running = false
	c.mu.Unlock()

	return nil
}

// Status returns the current collector status.
func (c *Collector) Status() *Status {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy
	status := &Status{
		Running:       c.status.Running,
		Mode:          c.status.Mode,
		StartTime:     c.status.StartTime,
		SnapshotCount: make(map[ProfileType]int64),
		LastSnapshot:  make(map[ProfileType]time.Time),
		Errors:        make([]string, len(c.status.Errors)),
	}
	for k, v := range c.status.SnapshotCount {
		status.SnapshotCount[k] = v
	}
	for k, v := range c.status.LastSnapshot {
		status.LastSnapshot[k] = v
	}
	copy(status.Errors, c.status.Errors)

	return status
}

// Snapshot collects a snapshot of the specified profile type.
func (c *Collector) Snapshot(pt ProfileType) ([]byte, error) {
	var buf bytes.Buffer

	switch pt {
	case ProfileCPU:
		return nil, fmt.Errorf("use SnapshotCPU for CPU profiles")
	case ProfileHeap:
		runtime.GC() // Run GC before heap snapshot for accuracy
		if err := pprof.WriteHeapProfile(&buf); err != nil {
			return nil, fmt.Errorf("failed to write heap profile: %w", err)
		}
	case ProfileGoroutine:
		p := pprof.Lookup("goroutine")
		if p == nil {
			return nil, fmt.Errorf("goroutine profile not found")
		}
		if err := p.WriteTo(&buf, 0); err != nil {
			return nil, fmt.Errorf("failed to write goroutine profile: %w", err)
		}
	case ProfileBlock:
		p := pprof.Lookup("block")
		if p == nil {
			return nil, fmt.Errorf("block profile not found")
		}
		if err := p.WriteTo(&buf, 0); err != nil {
			return nil, fmt.Errorf("failed to write block profile: %w", err)
		}
	case ProfileMutex:
		p := pprof.Lookup("mutex")
		if p == nil {
			return nil, fmt.Errorf("mutex profile not found")
		}
		if err := p.WriteTo(&buf, 0); err != nil {
			return nil, fmt.Errorf("failed to write mutex profile: %w", err)
		}
	case ProfileAllocs:
		p := pprof.Lookup("allocs")
		if p == nil {
			return nil, fmt.Errorf("allocs profile not found")
		}
		if err := p.WriteTo(&buf, 0); err != nil {
			return nil, fmt.Errorf("failed to write allocs profile: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown profile type: %s", pt)
	}

	return buf.Bytes(), nil
}

// SnapshotCPU collects a CPU profile for the specified duration.
func (c *Collector) SnapshotCPU(ctx context.Context, duration time.Duration) ([]byte, error) {
	// Ensure only one CPU profile runs at a time
	c.cpuMu.Lock()
	defer c.cpuMu.Unlock()

	var buf bytes.Buffer

	if err := pprof.StartCPUProfile(&buf); err != nil {
		return nil, fmt.Errorf("failed to start CPU profile: %w", err)
	}

	select {
	case <-time.After(duration):
	case <-ctx.Done():
		pprof.StopCPUProfile()
		return nil, ctx.Err()
	}

	pprof.StopCPUProfile()
	return buf.Bytes(), nil
}

// WriteSnapshot writes a profile snapshot to file.
func (c *Collector) WriteSnapshot(pt ProfileType, data []byte) (string, error) {
	filePath, err := c.writer.Write(pt, data)
	if err != nil {
		c.addError(fmt.Sprintf("write %s error: %v", pt, err))
		return "", err
	}

	c.mu.Lock()
	c.status.SnapshotCount[pt]++
	c.status.LastSnapshot[pt] = time.Now()
	c.mu.Unlock()

	return filePath, nil
}

// Config returns the collector configuration.
func (c *Collector) Config() *Config {
	return c.config
}

// Writer returns the file writer.
func (c *Collector) Writer() *Writer {
	return c.writer
}

// Context returns the collector's context.
func (c *Collector) Context() context.Context {
	return c.ctx
}

// WaitGroup returns the collector's wait group for goroutine management.
func (c *Collector) WaitGroup() *sync.WaitGroup {
	return &c.wg
}

func (c *Collector) addError(err string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Keep only last 100 errors
	if len(c.status.Errors) >= 100 {
		c.status.Errors = c.status.Errors[1:]
	}
	c.status.Errors = append(c.status.Errors, fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339), err))
}
