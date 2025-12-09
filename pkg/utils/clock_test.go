package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRealClock_Now(t *testing.T) {
	clock := NewRealClock()

	before := time.Now()
	actual := clock.Now()
	after := time.Now()

	assert.True(t, actual.After(before) || actual.Equal(before))
	assert.True(t, actual.Before(after) || actual.Equal(after))
}

func TestRealClock_Since(t *testing.T) {
	clock := NewRealClock()

	past := time.Now().Add(-1 * time.Second)
	duration := clock.Since(past)

	assert.True(t, duration >= 1*time.Second)
}

func TestRealClock_Until(t *testing.T) {
	clock := NewRealClock()

	future := time.Now().Add(1 * time.Hour)
	duration := clock.Until(future)

	assert.True(t, duration > 0)
	assert.True(t, duration <= 1*time.Hour)
}

func TestMockClock_Now(t *testing.T) {
	startTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewMockClock(startTime)

	assert.Equal(t, startTime, clock.Now())
}

func TestMockClock_Advance(t *testing.T) {
	startTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewMockClock(startTime)

	clock.Advance(1 * time.Hour)

	expected := startTime.Add(1 * time.Hour)
	assert.Equal(t, expected, clock.Now())
}

func TestMockClock_Set(t *testing.T) {
	startTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewMockClock(startTime)

	newTime := time.Date(2024, 6, 15, 8, 30, 0, 0, time.UTC)
	clock.Set(newTime)

	assert.Equal(t, newTime, clock.Now())
}

func TestMockClock_Since(t *testing.T) {
	startTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewMockClock(startTime)

	past := startTime.Add(-1 * time.Hour)
	duration := clock.Since(past)

	assert.Equal(t, 1*time.Hour, duration)
}

func TestMockClock_Until(t *testing.T) {
	startTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewMockClock(startTime)

	future := startTime.Add(2 * time.Hour)
	duration := clock.Until(future)

	assert.Equal(t, 2*time.Hour, duration)
}

func TestMockClock_Sleep(t *testing.T) {
	startTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewMockClock(startTime)

	clock.Sleep(30 * time.Minute)

	expected := startTime.Add(30 * time.Minute)
	assert.Equal(t, expected, clock.Now())
}

func TestClockInterface(t *testing.T) {
	// Verify both implementations satisfy the Clock interface
	var _ Clock = &RealClock{}
	var _ Clock = &MockClock{}
}

func TestMockClock_UsageInTest(t *testing.T) {
	// Example of how MockClock can be used in tests
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewMockClock(startTime)

	// Simulate time-based logic
	recordedTimes := make([]time.Time, 0)

	// Record time at intervals
	for i := 0; i < 3; i++ {
		recordedTimes = append(recordedTimes, clock.Now())
		clock.Advance(1 * time.Hour)
	}

	assert.Len(t, recordedTimes, 3)
	assert.Equal(t, startTime, recordedTimes[0])
	assert.Equal(t, startTime.Add(1*time.Hour), recordedTimes[1])
	assert.Equal(t, startTime.Add(2*time.Hour), recordedTimes[2])
}
