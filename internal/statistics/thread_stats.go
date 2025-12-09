package statistics

import (
	"sort"

	"github.com/perf-analysis/pkg/model"
)

// ThreadStatsCalculator calculates thread statistics from samples.
type ThreadStatsCalculator struct {
	maxThreads int
}

// ThreadStatsOption configures the ThreadStatsCalculator.
type ThreadStatsOption func(*ThreadStatsCalculator)

// WithMaxThreads sets the maximum number of threads to return.
func WithMaxThreads(n int) ThreadStatsOption {
	return func(c *ThreadStatsCalculator) {
		c.maxThreads = n
	}
}

// NewThreadStatsCalculator creates a new ThreadStatsCalculator.
func NewThreadStatsCalculator(opts ...ThreadStatsOption) *ThreadStatsCalculator {
	c := &ThreadStatsCalculator{
		maxThreads: 0, // 0 means no limit
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ThreadEntry represents a thread with its statistics.
type ThreadEntry struct {
	TID        int     `json:"tid"`
	ThreadName string  `json:"comm"`
	Samples    int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// ThreadStatsResult holds the calculation result.
type ThreadStatsResult struct {
	Threads      []ThreadEntry
	ThreadsMap   map[string]*model.ThreadInfo
	TotalSamples int64
}

// Calculate calculates thread statistics from the given samples.
func (c *ThreadStatsCalculator) Calculate(samples []*model.Sample) *ThreadStatsResult {
	result := &ThreadStatsResult{
		Threads:      make([]ThreadEntry, 0),
		ThreadsMap:   make(map[string]*model.ThreadInfo),
		TotalSamples: 0,
	}

	if len(samples) == 0 {
		return result
	}

	// Aggregate samples by thread
	threadSamples := make(map[int]*ThreadEntry)

	for _, sample := range samples {
		result.TotalSamples += sample.Value

		if _, ok := threadSamples[sample.TID]; !ok {
			threadSamples[sample.TID] = &ThreadEntry{
				TID:        sample.TID,
				ThreadName: sample.ThreadName,
				Samples:    0,
			}
		}
		threadSamples[sample.TID].Samples += sample.Value
	}

	// Calculate percentages and build slice
	entries := make([]ThreadEntry, 0, len(threadSamples))
	for _, ts := range threadSamples {
		pct := 0.0
		if result.TotalSamples > 0 {
			pct = float64(ts.Samples) / float64(result.TotalSamples) * 100
		}
		entries = append(entries, ThreadEntry{
			TID:        ts.TID,
			ThreadName: ts.ThreadName,
			Samples:    ts.Samples,
			Percentage: pct,
		})
	}

	// Sort by samples descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Samples > entries[j].Samples
	})

	// Apply limit if set
	if c.maxThreads > 0 && len(entries) > c.maxThreads {
		entries = entries[:c.maxThreads]
	}

	result.Threads = entries

	// Build map
	for _, entry := range entries {
		result.ThreadsMap[entry.ThreadName] = &model.ThreadInfo{
			TID:        entry.TID,
			ThreadName: entry.ThreadName,
			Samples:    entry.Samples,
			Percentage: entry.Percentage,
		}
	}

	return result
}

// ActiveThreadInfo represents active thread information for JSON output.
type ActiveThreadInfo struct {
	TID   int    `json:"tid"`
	Comm  string `json:"comm"`
	Count int64  `json:"count"`
}

// ToActiveThreadsList converts thread entries to the active threads format.
func (r *ThreadStatsResult) ToActiveThreadsList() []ActiveThreadInfo {
	result := make([]ActiveThreadInfo, len(r.Threads))
	for i, t := range r.Threads {
		result[i] = ActiveThreadInfo{
			TID:   t.TID,
			Comm:  t.ThreadName,
			Count: t.Samples,
		}
	}
	return result
}

// GetThreadByTID returns thread info by TID.
func (r *ThreadStatsResult) GetThreadByTID(tid int) *ThreadEntry {
	for i := range r.Threads {
		if r.Threads[i].TID == tid {
			return &r.Threads[i]
		}
	}
	return nil
}

// GetThreadByName returns thread info by name.
func (r *ThreadStatsResult) GetThreadByName(name string) *ThreadEntry {
	for i := range r.Threads {
		if r.Threads[i].ThreadName == name {
			return &r.Threads[i]
		}
	}
	return nil
}
