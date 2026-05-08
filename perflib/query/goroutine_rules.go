// Package query provides reusable query utilities for analysis data.
// This file implements the Rule Engine for goroutine issue detection.
// It follows the Strategy Pattern to make rules independently testable and extensible.
package query

import (
	"fmt"
	"strings"

	"github.com/junjiewwang/perf-analysis/perflib/model"
)

// GoroutineRule defines the interface for a goroutine issue detection rule.
// Each rule encapsulates a specific detection heuristic and produces zero or more issues.
type GoroutineRule interface {
	// Name returns a unique identifier for this rule.
	Name() string
	// Evaluate checks the goroutine data and returns any detected issues.
	Evaluate(data *model.PProfGoroutineData) []GoroutineIssue
}

// GoroutineRuleEngine executes a collection of rules against goroutine data.
// It provides a central registry for rules and handles execution ordering.
type GoroutineRuleEngine struct {
	rules []GoroutineRule
}

// NewGoroutineRuleEngine creates a rule engine with the default set of rules.
func NewGoroutineRuleEngine() *GoroutineRuleEngine {
	return &GoroutineRuleEngine{
		rules: defaultRules(),
	}
}

// AddRule appends a custom rule to the engine.
func (e *GoroutineRuleEngine) AddRule(rule GoroutineRule) {
	e.rules = append(e.rules, rule)
}

// Evaluate runs all rules against the data and returns collected issues.
// Issues are returned in rule-registration order, with higher severity rules first.
func (e *GoroutineRuleEngine) Evaluate(data *model.PProfGoroutineData) []GoroutineIssue {
	if data == nil {
		return nil
	}

	var issues []GoroutineIssue
	for _, rule := range e.rules {
		ruleIssues := rule.Evaluate(data)
		issues = append(issues, ruleIssues...)
	}
	return issues
}

// defaultRules returns the standard set of goroutine detection rules.
// Rules are ordered by severity: critical checks first, then warnings, then info.
func defaultRules() []GoroutineRule {
	return []GoroutineRule{
		// Critical/Warning rules
		&ExcessiveCountRule{},
		&DominantGroupRule{},
		&BlockingFunctionRule{},
		&IOWaitRule{},
		&MutexContentionRule{},
		// Info rules
		&FragmentationRule{},
		&SyscallBlockingRule{},
		&ChannelLeakRule{},
		&SleepAccumulationRule{},
	}
}

// ============================================================
// Rule 1: Excessive Goroutine Count
// Detects when total goroutine count exceeds safe thresholds.
// ============================================================

// ExcessiveCountRule checks for abnormally high goroutine counts.
type ExcessiveCountRule struct{}

// Name returns the rule identifier.
func (r *ExcessiveCountRule) Name() string { return "excessive_count" }

// Evaluate checks goroutine count against thresholds.
func (r *ExcessiveCountRule) Evaluate(data *model.PProfGoroutineData) []GoroutineIssue {
	if data.TotalCount > 100000 {
		return []GoroutineIssue{{
			Severity:    "critical",
			Type:        "excessive",
			Title:       "Extremely high goroutine count",
			Description: fmt.Sprintf("Total goroutine count (%d) exceeds 100,000. This is likely a goroutine leak.", data.TotalCount),
			Suggestion:  "Review goroutine lifecycle management. Look for goroutines spawned in loops without proper exit conditions or context cancellation.",
		}}
	}
	if data.TotalCount > 10000 {
		return []GoroutineIssue{{
			Severity:    "warning",
			Type:        "excessive",
			Title:       "High goroutine count",
			Description: fmt.Sprintf("Total goroutine count (%d) exceeds 10,000. This may indicate a goroutine leak or insufficient backpressure.", data.TotalCount),
			Suggestion:  "Consider implementing worker pools, rate limiting, or backpressure mechanisms to control goroutine creation.",
		}}
	}
	return nil
}

// ============================================================
// Rule 2: Dominant Single Group
// Detects when >50% of goroutines share the same stack.
// ============================================================

// DominantGroupRule checks for a single goroutine group dominating all others.
type DominantGroupRule struct{}

// Name returns the rule identifier.
func (r *DominantGroupRule) Name() string { return "dominant_group" }

// Evaluate checks for dominant groups.
func (r *DominantGroupRule) Evaluate(data *model.PProfGoroutineData) []GoroutineIssue {
	var issues []GoroutineIssue
	for i := range data.Distribution {
		g := &data.Distribution[i]
		if g.Percentage > 50 && g.Count > 100 {
			issue := GoroutineIssue{
				Severity:    "warning",
				Type:        "blocking",
				Title:       fmt.Sprintf("Dominant goroutine group: %s", truncateFunc(g.TopFunc, 60)),
				Description: fmt.Sprintf("%.1f%% (%d) goroutines share the same call stack topped at %s. This may indicate a blocking bottleneck.", g.Percentage, g.Count, g.TopFunc),
				GroupIndex:  i,
				Suggestion:  "Investigate why so many goroutines are blocked at the same point. Consider reducing concurrency or increasing resource capacity.",
			}
			// Extract related functions from stack
			if len(g.Stack) > 0 {
				maxFuncs := 5
				if len(g.Stack) < maxFuncs {
					maxFuncs = len(g.Stack)
				}
				issue.RelatedFuncs = g.Stack[:maxFuncs]
			}
			issues = append(issues, issue)
		}
	}
	return issues
}

// ============================================================
// Rule 3: Blocking Function Detection
// Detects blocking primitives (select, chan, Lock) in top functions.
// ============================================================

// BlockingFunctionRule detects high blocking at synchronization primitives.
type BlockingFunctionRule struct{}

// Name returns the rule identifier.
func (r *BlockingFunctionRule) Name() string { return "blocking_function" }

// Evaluate checks top functions for blocking patterns.
func (r *BlockingFunctionRule) Evaluate(data *model.PProfGoroutineData) []GoroutineIssue {
	blockingPatterns := []string{"select", "chan", "Lock", "Wait", "Cond", "Semaphore"}

	var issues []GoroutineIssue
	for _, tf := range data.TopFuncs {
		for _, pattern := range blockingPatterns {
			if strings.Contains(tf.Name, pattern) && tf.FlatPct > 20 {
				issues = append(issues, GoroutineIssue{
					Severity:     "warning",
					Type:         "blocking",
					Title:        fmt.Sprintf("High blocking at %s", truncateFunc(tf.Name, 50)),
					Description:  fmt.Sprintf("%.1f%% of goroutines are blocked at %s. Consider reviewing synchronization.", tf.FlatPct, tf.Name),
					Suggestion:   "Reduce lock contention by using finer-grained locks, lock-free data structures, or redesigning the concurrent access pattern.",
					RelatedFuncs: []string{tf.Name},
				})
				break
			}
		}
	}
	return issues
}

// ============================================================
// Rule 4: I/O Wait Detection (NEW)
// Detects goroutines waiting on I/O operations (network, disk).
// ============================================================

// IOWaitRule detects high I/O wait patterns in goroutine stacks.
type IOWaitRule struct{}

// Name returns the rule identifier.
func (r *IOWaitRule) Name() string { return "io_wait" }

// Evaluate checks for I/O blocking patterns.
func (r *IOWaitRule) Evaluate(data *model.PProfGoroutineData) []GoroutineIssue {
	ioPatterns := []string{
		"net/http.(*conn).serve",
		"net.(*netFD).Read",
		"net.(*netFD).Write",
		"net.(*netFD).Accept",
		"internal/poll.(*FD).Read",
		"internal/poll.(*FD).Write",
		"internal/poll.(*pollDesc).waitRead",
		"internal/poll.(*pollDesc).waitWrite",
		"os.(*File).Read",
		"os.(*File).Write",
		"io.ReadFull",
		"bufio.(*Reader).Read",
	}

	var ioWaitCount int64
	var relatedFuncs []string
	for i := range data.Distribution {
		g := &data.Distribution[i]
		for _, pattern := range ioPatterns {
			if containsInStack(g, pattern) {
				ioWaitCount += g.Count
				if len(relatedFuncs) < 5 {
					relatedFuncs = appendUnique(relatedFuncs, g.TopFunc)
				}
				break
			}
		}
	}

	if ioWaitCount == 0 {
		return nil
	}

	pct := float64(ioWaitCount) * 100.0 / float64(data.TotalCount)
	if pct < 30 {
		return nil
	}

	severity := "info"
	if pct > 70 {
		severity = "warning"
	}

	return []GoroutineIssue{{
		Severity:     severity,
		Type:         "io_wait",
		Title:        "High I/O wait ratio",
		Description:  fmt.Sprintf("%.1f%% (%d) goroutines are waiting on I/O operations. This may indicate slow upstream services or resource exhaustion.", pct, ioWaitCount),
		Suggestion:   "Check network latency, connection pool sizes, and upstream service health. Consider adding timeouts and circuit breakers.",
		RelatedFuncs: relatedFuncs,
	}}
}

// ============================================================
// Rule 5: Mutex Contention Detection (NEW)
// Detects goroutines stuck on mutex/RWMutex operations.
// ============================================================

// MutexContentionRule detects mutex contention hotspots.
type MutexContentionRule struct{}

// Name returns the rule identifier.
func (r *MutexContentionRule) Name() string { return "mutex_contention" }

// Evaluate checks for mutex contention patterns.
func (r *MutexContentionRule) Evaluate(data *model.PProfGoroutineData) []GoroutineIssue {
	mutexPatterns := []string{
		"sync.(*Mutex).Lock",
		"sync.(*RWMutex).Lock",
		"sync.(*RWMutex).RLock",
		"sync.runtime_SemacquireMutex",
	}

	var contentionCount int64
	var relatedFuncs []string
	for i := range data.Distribution {
		g := &data.Distribution[i]
		for _, pattern := range mutexPatterns {
			if containsInStack(g, pattern) {
				contentionCount += g.Count
				// Find the caller (one above the mutex call) for context
				if len(relatedFuncs) < 5 {
					relatedFuncs = appendUnique(relatedFuncs, g.TopFunc)
				}
				break
			}
		}
	}

	if contentionCount == 0 {
		return nil
	}

	pct := float64(contentionCount) * 100.0 / float64(data.TotalCount)
	if pct < 10 {
		return nil
	}

	severity := "info"
	if pct > 30 {
		severity = "warning"
	}
	if pct > 60 {
		severity = "critical"
	}

	return []GoroutineIssue{{
		Severity:     severity,
		Type:         "mutex_contention",
		Title:        "Mutex contention detected",
		Description:  fmt.Sprintf("%.1f%% (%d) goroutines are blocked on mutex operations. High contention degrades throughput.", pct, contentionCount),
		Suggestion:   "Consider using sync.RWMutex for read-heavy workloads, sharding data with per-shard locks, or redesigning to use channels for coordination.",
		RelatedFuncs: relatedFuncs,
	}}
}

// ============================================================
// Rule 6: Stack Fragmentation (existing, refactored)
// Detects high diversity of goroutine stacks (many single-count groups).
// ============================================================

// FragmentationRule detects high goroutine stack diversity.
type FragmentationRule struct{}

// Name returns the rule identifier.
func (r *FragmentationRule) Name() string { return "fragmentation" }

// Evaluate checks for stack fragmentation.
func (r *FragmentationRule) Evaluate(data *model.PProfGoroutineData) []GoroutineIssue {
	if len(data.Distribution) == 0 {
		return nil
	}

	singleCount := 0
	for _, g := range data.Distribution {
		if g.Count == 1 {
			singleCount++
		}
	}

	ratio := float64(singleCount) / float64(len(data.Distribution))
	if singleCount > 50 && ratio > 0.7 {
		return []GoroutineIssue{{
			Severity:    "info",
			Type:        "fragmentation",
			Title:       "High goroutine stack diversity",
			Description: fmt.Sprintf("%d out of %d groups have only 1 goroutine (%.0f%%). This suggests high concurrency diversity, which may make debugging harder.", singleCount, len(data.Distribution), ratio*100),
			Suggestion:  "High diversity is normal for servers handling many unique requests. If unexpected, check for per-request goroutine spawning without pooling.",
		}}
	}
	return nil
}

// ============================================================
// Rule 7: Syscall Blocking Detection (NEW)
// Detects goroutines blocked in syscalls (may consume OS threads).
// ============================================================

// SyscallBlockingRule detects goroutines blocked in system calls.
type SyscallBlockingRule struct{}

// Name returns the rule identifier.
func (r *SyscallBlockingRule) Name() string { return "syscall_blocking" }

// Evaluate checks for syscall blocking patterns.
func (r *SyscallBlockingRule) Evaluate(data *model.PProfGoroutineData) []GoroutineIssue {
	syscallPatterns := []string{
		"syscall.Syscall",
		"syscall.Syscall6",
		"syscall.RawSyscall",
		"runtime.entersyscall",
	}

	var syscallCount int64
	var relatedFuncs []string
	for i := range data.Distribution {
		g := &data.Distribution[i]
		if g.State == "syscall" {
			syscallCount += g.Count
			if len(relatedFuncs) < 5 {
				relatedFuncs = appendUnique(relatedFuncs, g.TopFunc)
			}
			continue
		}
		for _, pattern := range syscallPatterns {
			if containsInStack(g, pattern) {
				syscallCount += g.Count
				if len(relatedFuncs) < 5 {
					relatedFuncs = appendUnique(relatedFuncs, g.TopFunc)
				}
				break
			}
		}
	}

	if syscallCount == 0 {
		return nil
	}

	pct := float64(syscallCount) * 100.0 / float64(data.TotalCount)
	if pct < 20 {
		return nil
	}

	return []GoroutineIssue{{
		Severity:     "warning",
		Type:         "syscall_blocking",
		Title:        "Goroutines blocked in syscalls",
		Description:  fmt.Sprintf("%.1f%% (%d) goroutines are in syscall state. Each syscall-blocked goroutine consumes an OS thread, potentially exhausting GOMAXPROCS.", pct, syscallCount),
		Suggestion:   "Use non-blocking I/O where possible. For CGo or blocking syscalls, set GOMAXPROCS higher or use goroutine pools to limit concurrent blocking calls.",
		RelatedFuncs: relatedFuncs,
	}}
}

// ============================================================
// Rule 8: Channel Leak Detection (NEW)
// Detects goroutines stuck on channel send/receive (potential leak).
// ============================================================

// ChannelLeakRule detects goroutines stuck on channel operations.
type ChannelLeakRule struct{}

// Name returns the rule identifier.
func (r *ChannelLeakRule) Name() string { return "channel_leak" }

// Evaluate checks for channel blocking patterns.
func (r *ChannelLeakRule) Evaluate(data *model.PProfGoroutineData) []GoroutineIssue {
	channelPatterns := []string{
		"runtime.chanrecv",
		"runtime.chansend",
		"runtime.selectgo",
	}

	var chanBlockedCount int64
	var relatedFuncs []string
	for i := range data.Distribution {
		g := &data.Distribution[i]
		for _, pattern := range channelPatterns {
			if containsInStack(g, pattern) {
				chanBlockedCount += g.Count
				if len(relatedFuncs) < 5 {
					relatedFuncs = appendUnique(relatedFuncs, g.TopFunc)
				}
				break
			}
		}
	}

	if chanBlockedCount == 0 {
		return nil
	}

	pct := float64(chanBlockedCount) * 100.0 / float64(data.TotalCount)

	// Only report if significant and total count is high (indicating leak)
	if pct < 40 || data.TotalCount < 1000 {
		return nil
	}

	severity := "warning"
	if pct > 70 && data.TotalCount > 10000 {
		severity = "critical"
	}

	return []GoroutineIssue{{
		Severity:     severity,
		Type:         "channel_leak",
		Title:        "Potential channel-based goroutine leak",
		Description:  fmt.Sprintf("%.1f%% (%d) goroutines are blocked on channel operations. With %d total goroutines, this strongly suggests a goroutine leak via unbuffered or unread channels.", pct, chanBlockedCount, data.TotalCount),
		Suggestion:   "Ensure all channel senders have corresponding receivers. Use context cancellation to unblock goroutines. Consider buffered channels or select with default/timeout.",
		RelatedFuncs: relatedFuncs,
	}}
}

// ============================================================
// Rule 9: Sleep/Timer Accumulation (NEW)
// Detects goroutines stuck in time.Sleep or timer waits.
// ============================================================

// SleepAccumulationRule detects accumulated sleeping goroutines.
type SleepAccumulationRule struct{}

// Name returns the rule identifier.
func (r *SleepAccumulationRule) Name() string { return "sleep_accumulation" }

// Evaluate checks for sleep/timer accumulation patterns.
func (r *SleepAccumulationRule) Evaluate(data *model.PProfGoroutineData) []GoroutineIssue {
	sleepPatterns := []string{
		"time.Sleep",
		"runtime.timeSleep",
		"time.(*Timer).Reset",
		"time.After",
		"time.(*Ticker).Stop",
	}

	var sleepCount int64
	var relatedFuncs []string
	for i := range data.Distribution {
		g := &data.Distribution[i]
		for _, pattern := range sleepPatterns {
			if containsInStack(g, pattern) {
				sleepCount += g.Count
				if len(relatedFuncs) < 5 {
					relatedFuncs = appendUnique(relatedFuncs, g.TopFunc)
				}
				break
			}
		}
	}

	if sleepCount == 0 {
		return nil
	}

	// Only report if count is significant
	if sleepCount < 500 {
		return nil
	}

	pct := float64(sleepCount) * 100.0 / float64(data.TotalCount)

	return []GoroutineIssue{{
		Severity:     "info",
		Type:         "sleep_accumulation",
		Title:        "Accumulated sleeping goroutines",
		Description:  fmt.Sprintf("%d goroutines (%.1f%%) are in sleep/timer wait state. Large numbers may indicate retry loops or polling patterns that could be replaced with event-driven approaches.", sleepCount, pct),
		Suggestion:   "Consider using event-driven patterns (channels, watchers) instead of polling. For retry logic, use exponential backoff with jitter.",
		RelatedFuncs: relatedFuncs,
	}}
}

// ============================================================
// Helper functions
// ============================================================

// containsInStack checks if any function in the group's stack contains the pattern.
// It checks both the top function and the full stack.
func containsInStack(g *model.GoroutineGroup, pattern string) bool {
	if strings.Contains(g.TopFunc, pattern) {
		return true
	}
	for _, frame := range g.Stack {
		if strings.Contains(frame, pattern) {
			return true
		}
	}
	return false
}

// appendUnique appends s to slice only if not already present.
func appendUnique(slice []string, s string) []string {
	for _, existing := range slice {
		if existing == s {
			return slice
		}
	}
	return append(slice, s)
}
