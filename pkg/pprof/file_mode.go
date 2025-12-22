package pprof

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// FileMode implements file-based profile collection.
// It periodically collects profiles and writes them to files.
type FileMode struct {
	config    *FileConfig
	collector *Collector

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// For logging/notification
	onSnapshot func(pt ProfileType, filePath string, err error)
}

// NewFileMode creates a new FileMode.
func NewFileMode(config *FileConfig) *FileMode {
	if config == nil {
		config = DefaultConfig().FileConfig
	}
	return &FileMode{
		config: config,
	}
}

// Name returns the mode name.
func (m *FileMode) Name() string {
	return "file"
}

// SetSnapshotCallback sets a callback for snapshot events.
func (m *FileMode) SetSnapshotCallback(fn func(pt ProfileType, filePath string, err error)) {
	m.onSnapshot = fn
}

// Start starts the file mode collection.
func (m *FileMode) Start(ctx context.Context, collector *Collector) error {
	m.collector = collector
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Enable block and mutex profiling if requested
	cfg := collector.Config()
	if cfg.HasProfile(ProfileBlock) {
		runtime.SetBlockProfileRate(1)
	}
	if cfg.HasProfile(ProfileMutex) {
		runtime.SetMutexProfileFraction(1)
	}

	// Start collection goroutine
	m.wg.Add(1)
	go m.collectLoop()

	return nil
}

// Stop stops the file mode collection.
func (m *FileMode) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}

	// Wait for collection goroutine to finish
	m.wg.Wait()

	// Collect final snapshots
	m.collectFinalSnapshots()

	// Disable block and mutex profiling
	runtime.SetBlockProfileRate(0)
	runtime.SetMutexProfileFraction(0)

	return nil
}

func (m *FileMode) collectLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.Interval)
	defer ticker.Stop()

	// Collect initial snapshot
	m.collectSnapshots()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.collectSnapshots()
		}
	}
}

func (m *FileMode) collectSnapshots() {
	cfg := m.collector.Config()

	for _, pt := range cfg.Profiles {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		var data []byte
		var err error

		if pt == ProfileCPU {
			// CPU profile needs special handling with duration
			data, err = m.collector.SnapshotCPU(m.ctx, m.config.CPUDuration)
		} else {
			data, err = m.collector.Snapshot(pt)
		}

		if err != nil {
			m.notifySnapshot(pt, "", err)
			continue
		}

		filePath, err := m.collector.WriteSnapshot(pt, data)
		m.notifySnapshot(pt, filePath, err)
	}
}

func (m *FileMode) collectFinalSnapshots() {
	cfg := m.collector.Config()

	// Collect non-CPU profiles one more time
	for _, pt := range cfg.Profiles {
		if pt == ProfileCPU {
			continue // Skip CPU for final snapshot
		}

		data, err := m.collector.Snapshot(pt)
		if err != nil {
			m.notifySnapshot(pt, "", fmt.Errorf("final snapshot: %w", err))
			continue
		}

		filePath, err := m.collector.WriteSnapshot(pt, data)
		m.notifySnapshot(pt, filePath, err)
	}
}

func (m *FileMode) notifySnapshot(pt ProfileType, filePath string, err error) {
	if m.onSnapshot != nil {
		m.onSnapshot(pt, filePath, err)
	}
}
