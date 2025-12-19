package utils

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MockOutput captures output for testing.
type MockOutput struct {
	Messages []string
}

func (m *MockOutput) Output(format string, args ...interface{}) {
	m.Messages = append(m.Messages, fmt.Sprintf(format, args...))
}

func TestNewTimer(t *testing.T) {
	timer := NewTimer("test")
	assert.NotNil(t, timer)
	assert.Equal(t, "test", timer.name)
	assert.True(t, timer.enabled)
}

func TestTimerWithOptions(t *testing.T) {
	output := &MockOutput{}
	timer := NewTimer("test",
		WithOutput(output),
		WithEnabled(true),
	)

	assert.NotNil(t, timer)
	assert.Equal(t, output, timer.output)
	assert.True(t, timer.enabled)
}

func TestTimerWithLogger(t *testing.T) {
	logger := NewDefaultLogger(LevelInfo, nil)
	timer := NewTimer("test", WithLogger(logger))

	assert.NotNil(t, timer.output)
	loggerOutput, ok := timer.output.(*LoggerOutput)
	assert.True(t, ok)
	assert.Equal(t, logger, loggerOutput.Logger)
}

func TestTimerDisabled(t *testing.T) {
	timer := NewTimer("test", WithEnabled(false))

	// All operations should be no-ops
	pt := timer.Start("phase1")
	assert.NotNil(t, pt)

	duration := pt.Stop()
	assert.Equal(t, time.Duration(0), duration)

	assert.Equal(t, "", timer.Summary())
}

func TestTimerPhases(t *testing.T) {
	mockClock := NewMockClock(time.Now())
	timer := NewTimer("test", WithClock(mockClock))

	// Start phase 1
	pt1 := timer.Start("phase1")
	mockClock.Advance(100 * time.Millisecond)
	pt1.Stop()

	// Start phase 2
	pt2 := timer.Start("phase2")
	mockClock.Advance(200 * time.Millisecond)
	pt2.Stop()

	// Verify durations
	assert.Equal(t, 100*time.Millisecond, timer.GetDuration("phase1"))
	assert.Equal(t, 200*time.Millisecond, timer.GetDuration("phase2"))
}

func TestTimerChildPhases(t *testing.T) {
	mockClock := NewMockClock(time.Now())
	timer := NewTimer("test", WithClock(mockClock))

	// Start parent phase
	ptParent := timer.Start("parent")
	mockClock.Advance(50 * time.Millisecond)

	// Start child phase
	ptChild := timer.StartChild("parent", "child")
	mockClock.Advance(100 * time.Millisecond)
	ptChild.Stop()

	mockClock.Advance(50 * time.Millisecond)
	ptParent.Stop()

	// Verify durations
	assert.Equal(t, 200*time.Millisecond, timer.GetDuration("parent"))
	assert.Equal(t, 100*time.Millisecond, timer.GetDuration("child"))

	// Verify hierarchy
	phases := timer.GetPhases()
	assert.Len(t, phases, 2)
	assert.Equal(t, 0, phases[0].Level)
	assert.Equal(t, 1, phases[1].Level)
	assert.Equal(t, "parent", phases[1].Parent)
}

func TestTimerDeferPattern(t *testing.T) {
	mockClock := NewMockClock(time.Now())
	timer := NewTimer("test", WithClock(mockClock))

	func() {
		defer timer.Start("deferred").Stop()
		mockClock.Advance(150 * time.Millisecond)
	}()

	assert.Equal(t, 150*time.Millisecond, timer.GetDuration("deferred"))
}

func TestTimerSummary(t *testing.T) {
	mockClock := NewMockClock(time.Now())
	timer := NewTimer("TestOp", WithClock(mockClock))

	timer.Start("phase1")
	mockClock.Advance(100 * time.Millisecond)
	timer.StopPhase("phase1")

	timer.Start("phase2")
	mockClock.Advance(200 * time.Millisecond)
	timer.StopPhase("phase2")

	summary := timer.Summary()
	assert.Contains(t, summary, "TestOp Timing Summary")
	assert.Contains(t, summary, "phase1")
	assert.Contains(t, summary, "phase2")
	assert.Contains(t, summary, "Total:")
}

func TestTimerPrintSummary(t *testing.T) {
	mockClock := NewMockClock(time.Now())
	output := &MockOutput{}
	timer := NewTimer("TestOp", WithClock(mockClock), WithOutput(output))

	timer.Start("phase1")
	mockClock.Advance(100 * time.Millisecond)
	timer.StopPhase("phase1")

	timer.PrintSummary()

	assert.True(t, len(output.Messages) > 0)
	assert.Contains(t, output.Messages[0], "TestOp Timing Summary")
}

func TestTimerToMap(t *testing.T) {
	mockClock := NewMockClock(time.Now())
	timer := NewTimer("test", WithClock(mockClock))

	timer.Start("phase1")
	mockClock.Advance(100 * time.Millisecond)
	timer.StopPhase("phase1")

	m := timer.ToMap()
	assert.Equal(t, "test", m["name"])
	assert.NotNil(t, m["phases"])

	phases := m["phases"].([]map[string]interface{})
	assert.Len(t, phases, 1)
	assert.Equal(t, "phase1", phases[0]["name"])
	assert.Equal(t, int64(100), phases[0]["ms"])
}

func TestTimerTopN(t *testing.T) {
	mockClock := NewMockClock(time.Now())
	timer := NewTimer("test", WithClock(mockClock))

	// Create phases with different durations
	timer.Start("short")
	mockClock.Advance(50 * time.Millisecond)
	timer.StopPhase("short")

	timer.Start("medium")
	mockClock.Advance(100 * time.Millisecond)
	timer.StopPhase("medium")

	timer.Start("long")
	mockClock.Advance(200 * time.Millisecond)
	timer.StopPhase("long")

	top2 := timer.TopN(2)
	assert.Len(t, top2, 2)
	assert.Equal(t, "long", top2[0].Name)
	assert.Equal(t, "medium", top2[1].Name)
}

func TestTimerReset(t *testing.T) {
	mockClock := NewMockClock(time.Now())
	timer := NewTimer("test", WithClock(mockClock))

	timer.Start("phase1")
	mockClock.Advance(100 * time.Millisecond)
	timer.StopPhase("phase1")

	timer.Reset()

	phases := timer.GetPhases()
	assert.Len(t, phases, 0)
}

func TestTimerTimeFunc(t *testing.T) {
	mockClock := NewMockClock(time.Now())
	timer := NewTimer("test", WithClock(mockClock))

	executed := false
	timer.TimeFunc("func_phase", func() {
		mockClock.Advance(150 * time.Millisecond)
		executed = true
	})

	assert.True(t, executed)
	assert.Equal(t, 150*time.Millisecond, timer.GetDuration("func_phase"))
}

func TestTimerTimeFuncWithError(t *testing.T) {
	mockClock := NewMockClock(time.Now())
	timer := NewTimer("test", WithClock(mockClock))

	duration, err := timer.TimeFuncWithError("func_phase", func() error {
		mockClock.Advance(150 * time.Millisecond)
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 150*time.Millisecond, duration)
}

func TestTimerConcurrency(t *testing.T) {
	timer := NewTimer("concurrent")
	done := make(chan bool)

	// Start multiple goroutines that use the timer
	for i := 0; i < 10; i++ {
		go func(id int) {
			phaseName := strings.Repeat("x", id+1)
			pt := timer.Start(phaseName)
			time.Sleep(time.Millisecond)
			pt.Stop()
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	phases := timer.GetPhases()
	assert.Len(t, phases, 10)
}

func TestTimerStopIdempotent(t *testing.T) {
	mockClock := NewMockClock(time.Now())
	timer := NewTimer("test", WithClock(mockClock))

	pt := timer.Start("phase1")
	mockClock.Advance(100 * time.Millisecond)
	d1 := pt.Stop()

	mockClock.Advance(100 * time.Millisecond)
	d2 := pt.Stop() // Second stop should return same duration

	assert.Equal(t, d1, d2)
	assert.Equal(t, 100*time.Millisecond, d1)
}

func TestNullTimer(t *testing.T) {
	// NullTimer should be safe to use without panics
	pt := NullTimer.Start("phase")
	pt.Stop()

	NullTimer.StartChild("parent", "child")
	NullTimer.StopPhase("phase")
	NullTimer.GetDuration("phase")
	NullTimer.TotalDuration()
	NullTimer.GetPhases()
	NullTimer.Summary()
	NullTimer.PrintSummary()
	NullTimer.ToMap()
	NullTimer.TopN(5)
	NullTimer.Reset()
	NullTimer.TimeFunc("test", func() {})
	NullTimer.TimeFuncWithError("test", func() error { return nil })
}

func TestLoggerOutputNilLogger(t *testing.T) {
	output := &LoggerOutput{Logger: nil}
	// Should not panic
	output.Output("test %s", "message")
}
