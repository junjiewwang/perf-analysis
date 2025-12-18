// Package hprof provides parsing functionality for Java HPROF heap dump files.
package hprof

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// AsyncSerializationResult holds the result of an async serialization operation.
type AsyncSerializationResult struct {
	Stats    *SerializationStats
	Error    error
	Filename string
	Done     bool
}

// AsyncSerializer provides asynchronous serialization capabilities.
// It allows the main analysis to continue while serialization happens in the background.
type AsyncSerializer struct {
	mu       sync.Mutex
	results  map[string]*asyncJob
	wg       sync.WaitGroup
	maxJobs  int
	jobCount int
}

// asyncJob represents a background serialization job.
type asyncJob struct {
	result   *AsyncSerializationResult
	doneChan chan struct{}
	cancel   context.CancelFunc
}

// NewAsyncSerializer creates a new async serializer.
// maxConcurrentJobs limits the number of concurrent serialization jobs (0 = no limit).
func NewAsyncSerializer(maxConcurrentJobs int) *AsyncSerializer {
	if maxConcurrentJobs <= 0 {
		maxConcurrentJobs = 2 // Default to 2 concurrent jobs
	}
	return &AsyncSerializer{
		results: make(map[string]*asyncJob),
		maxJobs: maxConcurrentJobs,
	}
}

// SerializeAsync starts an asynchronous serialization job.
// Returns a job ID that can be used to check status or wait for completion.
// If the serializer is at capacity, this will block until a slot is available.
func (s *AsyncSerializer) SerializeAsync(ctx context.Context, g *ReferenceGraph, filename string, opts SerializeOptions) (string, error) {
	s.mu.Lock()
	
	// Wait for a slot if at capacity
	for s.jobCount >= s.maxJobs {
		s.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		s.mu.Lock()
	}
	
	jobID := filename
	if _, exists := s.results[jobID]; exists {
		s.mu.Unlock()
		return "", fmt.Errorf("job already exists for file: %s", filename)
	}
	
	jobCtx, cancel := context.WithCancel(ctx)
	job := &asyncJob{
		result: &AsyncSerializationResult{
			Filename: filename,
		},
		doneChan: make(chan struct{}),
		cancel:   cancel,
	}
	s.results[jobID] = job
	s.jobCount++
	s.wg.Add(1)
	s.mu.Unlock()
	
	// Start background serialization
	go s.runSerializationJob(jobCtx, g, filename, opts, job)
	
	return jobID, nil
}

// runSerializationJob performs the actual serialization in the background.
func (s *AsyncSerializer) runSerializationJob(ctx context.Context, g *ReferenceGraph, filename string, opts SerializeOptions, job *asyncJob) {
	defer func() {
		s.wg.Done()
		s.mu.Lock()
		s.jobCount--
		s.mu.Unlock()
		close(job.doneChan)
	}()
	
	// Check for cancellation before starting
	select {
	case <-ctx.Done():
		job.result.Error = ctx.Err()
		job.result.Done = true
		return
	default:
	}
	
	// Perform serialization
	stats, err := g.SerializeToFile(filename, opts)
	
	job.result.Stats = stats
	job.result.Error = err
	job.result.Done = true
}

// GetResult returns the result for a job, or nil if the job doesn't exist.
// This is non-blocking and returns the current state.
func (s *AsyncSerializer) GetResult(jobID string) *AsyncSerializationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if job, exists := s.results[jobID]; exists {
		return job.result
	}
	return nil
}

// Wait blocks until the specified job completes.
// Returns the final result or an error if the job doesn't exist.
func (s *AsyncSerializer) Wait(jobID string) (*AsyncSerializationResult, error) {
	s.mu.Lock()
	job, exists := s.results[jobID]
	s.mu.Unlock()
	
	if !exists {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	
	<-job.doneChan
	return job.result, nil
}

// WaitWithTimeout blocks until the job completes or timeout is reached.
func (s *AsyncSerializer) WaitWithTimeout(jobID string, timeout time.Duration) (*AsyncSerializationResult, error) {
	s.mu.Lock()
	job, exists := s.results[jobID]
	s.mu.Unlock()
	
	if !exists {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	
	select {
	case <-job.doneChan:
		return job.result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for job: %s", jobID)
	}
}

// Cancel cancels a running job.
func (s *AsyncSerializer) Cancel(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if job, exists := s.results[jobID]; exists {
		job.cancel()
	}
}

// CancelAll cancels all running jobs.
func (s *AsyncSerializer) CancelAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	for _, job := range s.results {
		job.cancel()
	}
}

// WaitAll waits for all jobs to complete.
func (s *AsyncSerializer) WaitAll() {
	s.wg.Wait()
}

// Cleanup removes completed job results from memory.
// Returns the number of jobs cleaned up.
func (s *AsyncSerializer) Cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	count := 0
	for id, job := range s.results {
		if job.result.Done {
			delete(s.results, id)
			count++
		}
	}
	return count
}

// PendingJobs returns the number of jobs that are still running.
func (s *AsyncSerializer) PendingJobs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobCount
}

// SerializeToFileAsync is a convenience function that serializes asynchronously
// and returns immediately. The caller can optionally wait for completion.
func SerializeToFileAsync(ctx context.Context, g *ReferenceGraph, filename string, opts SerializeOptions) (<-chan *AsyncSerializationResult, error) {
	resultChan := make(chan *AsyncSerializationResult, 1)
	
	go func() {
		defer close(resultChan)
		
		result := &AsyncSerializationResult{
			Filename: filename,
		}
		
		// Check for cancellation
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			result.Done = true
			resultChan <- result
			return
		default:
		}
		
		// Perform serialization
		stats, err := g.SerializeToFile(filename, opts)
		result.Stats = stats
		result.Error = err
		result.Done = true
		resultChan <- result
	}()
	
	return resultChan, nil
}

// SerializeToFileWithCallback serializes asynchronously and calls the callback when done.
// This is useful for fire-and-forget scenarios where you want to be notified of completion.
func SerializeToFileWithCallback(ctx context.Context, g *ReferenceGraph, filename string, opts SerializeOptions, callback func(*AsyncSerializationResult)) {
	go func() {
		result := &AsyncSerializationResult{
			Filename: filename,
		}
		
		// Check for cancellation
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			result.Done = true
			if callback != nil {
				callback(result)
			}
			return
		default:
		}
		
		// Perform serialization
		stats, err := g.SerializeToFile(filename, opts)
		result.Stats = stats
		result.Error = err
		result.Done = true
		
		if callback != nil {
			callback(result)
		}
	}()
}

// BackgroundSerializer is a singleton-style serializer for simple use cases.
var backgroundSerializer *AsyncSerializer
var backgroundSerializerOnce sync.Once

// GetBackgroundSerializer returns the global background serializer instance.
func GetBackgroundSerializer() *AsyncSerializer {
	backgroundSerializerOnce.Do(func() {
		backgroundSerializer = NewAsyncSerializer(2)
	})
	return backgroundSerializer
}

// SerializeInBackground is a convenience function that uses the global background serializer.
// It returns immediately and serialization happens in the background.
// Use GetBackgroundSerializer().Wait(filename) to wait for completion if needed.
func (g *ReferenceGraph) SerializeInBackground(ctx context.Context, filename string, opts SerializeOptions) (string, error) {
	return GetBackgroundSerializer().SerializeAsync(ctx, g, filename, opts)
}

// SerializeToFileOrBackground serializes to file, either synchronously or asynchronously
// based on the async parameter. This provides a unified API for both modes.
func (g *ReferenceGraph) SerializeToFileOrBackground(ctx context.Context, filename string, opts SerializeOptions, async bool) (*SerializationStats, error) {
	if !async {
		return g.SerializeToFile(filename, opts)
	}
	
	// Async mode - start background job and return immediately
	jobID, err := g.SerializeInBackground(ctx, filename, opts)
	if err != nil {
		return nil, err
	}
	
	// Return placeholder stats indicating async operation
	return &SerializationStats{
		Duration: 0, // Will be filled when complete
	}, fmt.Errorf("async serialization started with job ID: %s (use GetBackgroundSerializer().Wait() to get final result)", jobID)
}

// EnsureSerializationComplete waits for any pending background serialization to complete
// for the given file. Returns the final stats or error.
func EnsureSerializationComplete(filename string, timeout time.Duration) (*SerializationStats, error) {
	serializer := GetBackgroundSerializer()
	result := serializer.GetResult(filename)
	
	if result == nil {
		// No async job for this file, check if file exists
		if _, err := os.Stat(filename); err == nil {
			return nil, nil // File exists, no async job needed
		}
		return nil, fmt.Errorf("no serialization job found for: %s", filename)
	}
	
	if result.Done {
		return result.Stats, result.Error
	}
	
	// Wait for completion
	finalResult, err := serializer.WaitWithTimeout(filename, timeout)
	if err != nil {
		return nil, err
	}
	return finalResult.Stats, finalResult.Error
}
