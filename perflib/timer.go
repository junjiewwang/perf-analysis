package perflib

import "time"

// Timer defines the performance timing interface for perflib.
// Consumers inject their own Timer implementation for phase-level profiling.
//
// This interface covers the methods used by perflib internals (hprof parser, etc.).
// If nil is provided where a Timer is expected, NullTimer should be used instead.
type Timer interface {
	// Start starts timing a new phase. Returns a PhaseTimer for stopping.
	Start(phaseName string) PhaseTimer

	// TimeFunc times the execution of a function and records it as a phase.
	TimeFunc(phaseName string, fn func()) time.Duration

	// TimeFuncWithError times a function that may return an error.
	TimeFuncWithError(phaseName string, fn func() error) (time.Duration, error)

	// PrintSummary outputs the timing summary using the configured output strategy.
	PrintSummary()
}

// PhaseTimer controls a single timing phase.
// It supports automatic completion via defer: defer pt.Stop()
type PhaseTimer interface {
	// Stop stops the phase timer and returns its duration.
	// Safe to call multiple times; only the first call has effect.
	Stop() time.Duration
}

// nullTimer is a no-op Timer implementation for zero overhead.
type nullTimer struct{}

// nullPhaseTimer is a no-op PhaseTimer implementation.
type nullPhaseTimer struct{}

// NullTimer is a no-op timer for when timing is disabled.
// All methods are safe to call but do nothing.
var NullTimer Timer = &nullTimer{}

func (t *nullTimer) Start(string) PhaseTimer                                   { return &nullPhaseTimer{} }
func (t *nullTimer) TimeFunc(_ string, fn func()) time.Duration                { fn(); return 0 }
func (t *nullTimer) TimeFuncWithError(_ string, fn func() error) (time.Duration, error) { return 0, fn() }
func (t *nullTimer) PrintSummary()                                             {}

func (pt *nullPhaseTimer) Stop() time.Duration { return 0 }
