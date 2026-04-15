// Package timer provides a concrete Timer implementation that satisfies perflib.Timer.
package timer

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/junjiewwang/perf-analysis/perflib"
)

// TimerOutput defines the interface for outputting timer results.
// This enables dependency injection for different output strategies.
type TimerOutput interface {
	// Output writes the timing information.
	Output(format string, args ...interface{})
}

// LoggerOutput adapts perflib.Logger to TimerOutput.
type LoggerOutput struct {
	Logger perflib.Logger
}

// Output implements TimerOutput using Logger.Info.
func (o *LoggerOutput) Output(format string, args ...interface{}) {
	if o.Logger != nil {
		o.Logger.Info(format, args...)
	}
}

// Phase represents a single timing phase with hierarchical support.
type Phase struct {
	Name      string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Parent    string // Parent phase name for hierarchical display
	Level     int    // Nesting level (0 = root)
	completed bool
}

// phaseTimer provides a fluent API for timing a single phase.
type phaseTimer struct {
	timer     *Timer
	phaseName string
}

// Stop stops the phase timer and records the duration.
func (pt *phaseTimer) Stop() time.Duration {
	return pt.timer.StopPhase(pt.phaseName)
}

// Clock provides an interface for time operations, making code testable.
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
}

// RealClock implements Clock using the standard time package.
type RealClock struct{}

// Now returns the current time.
func (c *RealClock) Now() time.Time { return time.Now() }

// Since returns the duration since the given time.
func (c *RealClock) Since(t time.Time) time.Duration { return time.Since(t) }

// Timer is a high-cohesion, low-coupling timing utility.
// It implements perflib.Timer interface.
type Timer struct {
	mu         sync.RWMutex
	name       string
	startTime  time.Time
	phases     map[string]*Phase
	phaseOrder []string
	output     TimerOutput
	enabled    bool
	clock      Clock
}

// Option configures a Timer instance.
type Option func(*Timer)

// WithOutput sets the output strategy for the timer.
func WithOutput(output TimerOutput) Option {
	return func(t *Timer) {
		t.output = output
	}
}

// WithLogger sets a perflib.Logger as the output strategy.
func WithLogger(logger perflib.Logger) Option {
	return func(t *Timer) {
		if logger != nil {
			t.output = &LoggerOutput{Logger: logger}
		}
	}
}

// WithEnabled sets whether the timer is enabled.
func WithEnabled(enabled bool) Option {
	return func(t *Timer) {
		t.enabled = enabled
	}
}

// WithClock sets a custom clock for testability.
func WithClock(clock Clock) Option {
	return func(t *Timer) {
		t.clock = clock
	}
}

// New creates a new Timer with the given name and options.
// The returned Timer implements perflib.Timer.
func New(name string, opts ...Option) *Timer {
	t := &Timer{
		name:       name,
		phases:     make(map[string]*Phase),
		phaseOrder: make([]string, 0),
		enabled:    true,
		clock:      &RealClock{},
	}

	for _, opt := range opts {
		opt(t)
	}

	t.startTime = t.clock.Now()
	return t
}

// Start starts timing a new phase.
func (t *Timer) Start(phaseName string) perflib.PhaseTimer {
	if !t.enabled {
		return &phaseTimer{timer: t, phaseName: phaseName}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.phases[phaseName] = &Phase{
		Name:      phaseName,
		StartTime: t.clock.Now(),
		Level:     0,
	}
	t.phaseOrder = append(t.phaseOrder, phaseName)

	return &phaseTimer{timer: t, phaseName: phaseName}
}

// StartChild starts timing a child phase under a parent phase.
func (t *Timer) StartChild(parentName, childName string) perflib.PhaseTimer {
	if !t.enabled {
		return &phaseTimer{timer: t, phaseName: childName}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	parentLevel := 0
	if parent, ok := t.phases[parentName]; ok {
		parentLevel = parent.Level
	}

	t.phases[childName] = &Phase{
		Name:      childName,
		StartTime: t.clock.Now(),
		Parent:    parentName,
		Level:     parentLevel + 1,
	}
	t.phaseOrder = append(t.phaseOrder, childName)

	return &phaseTimer{timer: t, phaseName: childName}
}

// StopPhase stops timing a phase and returns its duration.
func (t *Timer) StopPhase(phaseName string) time.Duration {
	if !t.enabled {
		return 0
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	phase, ok := t.phases[phaseName]
	if !ok || phase.completed {
		if phase != nil {
			return phase.Duration
		}
		return 0
	}

	phase.EndTime = t.clock.Now()
	phase.Duration = phase.EndTime.Sub(phase.StartTime)
	phase.completed = true

	return phase.Duration
}

// GetDuration returns the duration of a completed phase.
func (t *Timer) GetDuration(phaseName string) time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if phase, ok := t.phases[phaseName]; ok {
		return phase.Duration
	}
	return 0
}

// TotalDuration returns the total duration since the timer was created.
func (t *Timer) TotalDuration() time.Duration {
	return t.clock.Since(t.startTime)
}

// GetPhases returns all phases in insertion order.
func (t *Timer) GetPhases() []*Phase {
	t.mu.RLock()
	defer t.mu.RUnlock()

	phases := make([]*Phase, 0, len(t.phaseOrder))
	for _, name := range t.phaseOrder {
		if phase, ok := t.phases[name]; ok {
			phaseCopy := *phase
			phases = append(phases, &phaseCopy)
		}
	}
	return phases
}

// Summary returns a formatted summary of all timing phases.
func (t *Timer) Summary() string {
	if !t.enabled {
		return ""
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== %s Timing Summary ===\n", t.name))

	for _, name := range t.phaseOrder {
		phase := t.phases[name]
		indent := strings.Repeat("  ", phase.Level)
		prefix := ""
		if phase.Level > 0 {
			prefix = fmt.Sprintf("%d.%d ", phase.Level, t.getChildIndex(name))
		} else {
			prefix = fmt.Sprintf("Phase %d - ", t.getRootIndex(name)+1)
		}
		sb.WriteString(fmt.Sprintf("%s%s%s: %v\n", indent, prefix, phase.Name, phase.Duration))
	}

	sb.WriteString(fmt.Sprintf("Total: %v\n", t.TotalDuration()))
	return sb.String()
}

// PrintSummary outputs the timing summary using the configured output strategy.
func (t *Timer) PrintSummary() {
	if !t.enabled || t.output == nil {
		return
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	t.output.Output("=== %s Timing Summary ===", t.name)

	for _, name := range t.phaseOrder {
		phase := t.phases[name]
		indent := strings.Repeat("  ", phase.Level)
		prefix := ""
		if phase.Level > 0 {
			prefix = fmt.Sprintf("%d.%d ", phase.Level, t.getChildIndex(name))
		} else {
			prefix = fmt.Sprintf("Phase %d - ", t.getRootIndex(name)+1)
		}
		t.output.Output("%s%s%s: %v", indent, prefix, phase.Name, phase.Duration)
	}

	t.output.Output("Total: %v", t.TotalDuration())
}

// getRootIndex returns the index of a root-level phase (0-based).
func (t *Timer) getRootIndex(phaseName string) int {
	index := 0
	for _, name := range t.phaseOrder {
		if name == phaseName {
			return index
		}
		if t.phases[name].Level == 0 {
			index++
		}
	}
	return index
}

// getChildIndex returns the index of a child phase under its parent (1-based).
func (t *Timer) getChildIndex(phaseName string) int {
	phase := t.phases[phaseName]
	index := 1
	for _, name := range t.phaseOrder {
		if name == phaseName {
			return index
		}
		p := t.phases[name]
		if p.Parent == phase.Parent && p.Level == phase.Level {
			index++
		}
	}
	return index
}

// ToMap returns the timing data as a map for serialization.
func (t *Timer) ToMap() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	phases := make([]map[string]interface{}, 0, len(t.phaseOrder))
	for _, name := range t.phaseOrder {
		phase := t.phases[name]
		phaseMap := map[string]interface{}{
			"name":     phase.Name,
			"duration": phase.Duration.String(),
			"ms":       phase.Duration.Milliseconds(),
			"level":    phase.Level,
		}
		if phase.Parent != "" {
			phaseMap["parent"] = phase.Parent
		}
		phases = append(phases, phaseMap)
	}

	return map[string]interface{}{
		"name":           t.name,
		"total_duration": t.TotalDuration().String(),
		"total_ms":       t.TotalDuration().Milliseconds(),
		"phases":         phases,
	}
}

// TopN returns the top N phases by duration.
func (t *Timer) TopN(n int) []*Phase {
	t.mu.RLock()
	defer t.mu.RUnlock()

	phases := make([]*Phase, 0, len(t.phases))
	for _, phase := range t.phases {
		phaseCopy := *phase
		phases = append(phases, &phaseCopy)
	}

	sort.Slice(phases, func(i, j int) bool {
		return phases[i].Duration > phases[j].Duration
	})

	if n > len(phases) {
		n = len(phases)
	}
	return phases[:n]
}

// Reset clears all phases and resets the start time.
func (t *Timer) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.phases = make(map[string]*Phase)
	t.phaseOrder = make([]string, 0)
	t.startTime = t.clock.Now()
}

// TimeFunc times the execution of a function and records it as a phase.
func (t *Timer) TimeFunc(phaseName string, fn func()) time.Duration {
	pt := t.Start(phaseName)
	fn()
	return pt.Stop()
}

// TimeFuncWithError times the execution of a function that returns an error.
func (t *Timer) TimeFuncWithError(phaseName string, fn func() error) (time.Duration, error) {
	pt := t.Start(phaseName)
	err := fn()
	return pt.Stop(), err
}
