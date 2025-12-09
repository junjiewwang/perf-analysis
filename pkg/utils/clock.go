// Package utils provides utility functions and types.
package utils

import "time"

// Clock provides an interface for time operations, making code testable.
type Clock interface {
	// Now returns the current time.
	Now() time.Time

	// Since returns the duration since the given time.
	Since(t time.Time) time.Duration

	// Until returns the duration until the given time.
	Until(t time.Time) time.Duration

	// Sleep pauses the current goroutine for the specified duration.
	Sleep(d time.Duration)

	// After waits for the duration to elapse and then returns the current time on a channel.
	After(d time.Duration) <-chan time.Time

	// NewTicker creates a new Ticker that sends the current time on a channel at intervals.
	NewTicker(d time.Duration) *time.Ticker
}

// RealClock implements Clock using the standard time package.
type RealClock struct{}

// NewRealClock creates a new RealClock instance.
func NewRealClock() *RealClock {
	return &RealClock{}
}

// Now returns the current time.
func (c *RealClock) Now() time.Time {
	return time.Now()
}

// Since returns the duration since the given time.
func (c *RealClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}

// Until returns the duration until the given time.
func (c *RealClock) Until(t time.Time) time.Duration {
	return time.Until(t)
}

// Sleep pauses the current goroutine for the specified duration.
func (c *RealClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

// After waits for the duration to elapse and then returns the current time on a channel.
func (c *RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// NewTicker creates a new Ticker that sends the current time on a channel at intervals.
func (c *RealClock) NewTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}

// MockClock implements Clock for testing purposes.
type MockClock struct {
	currentTime time.Time
	afterChan   chan time.Time
}

// NewMockClock creates a new MockClock instance with the given start time.
func NewMockClock(startTime time.Time) *MockClock {
	return &MockClock{
		currentTime: startTime,
		afterChan:   make(chan time.Time, 1),
	}
}

// Now returns the mock current time.
func (c *MockClock) Now() time.Time {
	return c.currentTime
}

// Since returns the duration since the given time using mock time.
func (c *MockClock) Since(t time.Time) time.Duration {
	return c.currentTime.Sub(t)
}

// Until returns the duration until the given time using mock time.
func (c *MockClock) Until(t time.Time) time.Duration {
	return t.Sub(c.currentTime)
}

// Sleep does nothing in mock clock (instant).
func (c *MockClock) Sleep(d time.Duration) {
	c.Advance(d)
}

// After returns a channel that receives the time after advancing.
func (c *MockClock) After(d time.Duration) <-chan time.Time {
	c.Advance(d)
	c.afterChan <- c.currentTime
	return c.afterChan
}

// NewTicker creates a new Ticker (for testing, this returns a real ticker).
func (c *MockClock) NewTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}

// Advance advances the mock clock by the given duration.
func (c *MockClock) Advance(d time.Duration) {
	c.currentTime = c.currentTime.Add(d)
}

// Set sets the mock clock to the given time.
func (c *MockClock) Set(t time.Time) {
	c.currentTime = t
}
