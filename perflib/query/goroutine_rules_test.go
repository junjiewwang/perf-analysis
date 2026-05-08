package query

import (
	"testing"

	"github.com/junjiewwang/perf-analysis/perflib/model"
)

// ============================================================
// RuleEngine Tests
// ============================================================

func TestGoroutineRuleEngine_NilData(t *testing.T) {
	engine := NewGoroutineRuleEngine()
	issues := engine.Evaluate(nil)
	if issues != nil {
		t.Error("expected nil issues for nil data")
	}
}

func TestGoroutineRuleEngine_NoIssues(t *testing.T) {
	data := &model.PProfGoroutineData{
		TotalCount: 10,
		Distribution: []model.GoroutineGroup{
			{Count: 5, Percentage: 50.0, TopFunc: "main.doWork"},
			{Count: 5, Percentage: 50.0, TopFunc: "main.listen"},
		},
	}
	engine := NewGoroutineRuleEngine()
	issues := engine.Evaluate(data)
	if len(issues) != 0 {
		t.Errorf("expected no issues for normal data, got %d: %v", len(issues), issues)
	}
}

func TestGoroutineRuleEngine_AddCustomRule(t *testing.T) {
	engine := NewGoroutineRuleEngine()
	engine.AddRule(&mockRule{})

	data := &model.PProfGoroutineData{TotalCount: 1}
	issues := engine.Evaluate(data)

	found := false
	for _, issue := range issues {
		if issue.Type == "mock" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected custom rule issue to be present")
	}
}

type mockRule struct{}

func (r *mockRule) Name() string { return "mock" }
func (r *mockRule) Evaluate(data *model.PProfGoroutineData) []GoroutineIssue {
	return []GoroutineIssue{{Severity: "info", Type: "mock", Title: "Mock"}}
}

// ============================================================
// IOWaitRule Tests
// ============================================================

func TestIOWaitRule_Triggered(t *testing.T) {
	// 80% goroutines in I/O wait → should trigger
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 800, Percentage: 80.0, TopFunc: "net/http.(*conn).serve",
				Stack: []string{"net/http.(*conn).serve", "internal/poll.(*FD).Read"}},
			{Count: 200, Percentage: 20.0, TopFunc: "main.doWork",
				Stack: []string{"main.doWork"}},
		},
	}

	rule := &IOWaitRule{}
	issues := rule.Evaluate(data)
	if len(issues) == 0 {
		t.Fatal("expected IO wait issue to be detected")
	}
	if issues[0].Type != "io_wait" {
		t.Errorf("expected type=io_wait, got %s", issues[0].Type)
	}
	if issues[0].Severity != "warning" {
		t.Errorf("expected severity=warning for >70%%, got %s", issues[0].Severity)
	}
	if issues[0].Suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
	if len(issues[0].RelatedFuncs) == 0 {
		t.Error("expected non-empty RelatedFuncs")
	}
}

func TestIOWaitRule_InfoSeverity(t *testing.T) {
	// 40% goroutines in I/O wait → info severity
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 400, Percentage: 40.0, TopFunc: "net.(*netFD).Read",
				Stack: []string{"net.(*netFD).Read"}},
			{Count: 600, Percentage: 60.0, TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &IOWaitRule{}
	issues := rule.Evaluate(data)
	if len(issues) == 0 {
		t.Fatal("expected IO wait issue for 40%")
	}
	if issues[0].Severity != "info" {
		t.Errorf("expected severity=info for 30-70%%, got %s", issues[0].Severity)
	}
}

func TestIOWaitRule_NotTriggered_LowPercentage(t *testing.T) {
	// 20% goroutines in I/O wait → should NOT trigger (threshold is 30%)
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 200, Percentage: 20.0, TopFunc: "net/http.(*conn).serve",
				Stack: []string{"net/http.(*conn).serve"}},
			{Count: 800, Percentage: 80.0, TopFunc: "main.compute",
				Stack: []string{"main.compute"}},
		},
	}

	rule := &IOWaitRule{}
	issues := rule.Evaluate(data)
	if len(issues) != 0 {
		t.Errorf("expected no issues for 20%% IO wait, got %d", len(issues))
	}
}

func TestIOWaitRule_NotTriggered_NoIOFunctions(t *testing.T) {
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 1000, Percentage: 100.0, TopFunc: "main.compute",
				Stack: []string{"main.compute", "math.Sqrt"}},
		},
	}

	rule := &IOWaitRule{}
	issues := rule.Evaluate(data)
	if len(issues) != 0 {
		t.Errorf("expected no issues when no IO functions, got %d", len(issues))
	}
}

// ============================================================
// MutexContentionRule Tests
// ============================================================

func TestMutexContentionRule_Triggered_Warning(t *testing.T) {
	// 40% goroutines on mutex → warning
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 400, Percentage: 40.0, TopFunc: "myapp.handleRequest",
				Stack: []string{"myapp.handleRequest", "sync.(*Mutex).Lock", "myapp.cache.Get"}},
			{Count: 600, Percentage: 60.0, TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &MutexContentionRule{}
	issues := rule.Evaluate(data)
	if len(issues) == 0 {
		t.Fatal("expected mutex contention issue")
	}
	if issues[0].Type != "mutex_contention" {
		t.Errorf("expected type=mutex_contention, got %s", issues[0].Type)
	}
	if issues[0].Severity != "warning" {
		t.Errorf("expected severity=warning for 30-60%%, got %s", issues[0].Severity)
	}
	if issues[0].Suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
}

func TestMutexContentionRule_Triggered_Critical(t *testing.T) {
	// 70% goroutines on mutex → critical
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 700, Percentage: 70.0, TopFunc: "sync.(*Mutex).Lock",
				Stack: []string{"sync.(*Mutex).Lock", "myapp.globalLock"}},
			{Count: 300, Percentage: 30.0, TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &MutexContentionRule{}
	issues := rule.Evaluate(data)
	if len(issues) == 0 {
		t.Fatal("expected critical mutex contention issue")
	}
	if issues[0].Severity != "critical" {
		t.Errorf("expected severity=critical for >60%%, got %s", issues[0].Severity)
	}
}

func TestMutexContentionRule_NotTriggered_LowContention(t *testing.T) {
	// 5% goroutines on mutex → should NOT trigger (threshold is 10%)
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 50, Percentage: 5.0, TopFunc: "sync.(*Mutex).Lock",
				Stack: []string{"sync.(*Mutex).Lock"}},
			{Count: 950, Percentage: 95.0, TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &MutexContentionRule{}
	issues := rule.Evaluate(data)
	if len(issues) != 0 {
		t.Errorf("expected no issues for 5%% mutex contention, got %d", len(issues))
	}
}

func TestMutexContentionRule_NotTriggered_NoMutex(t *testing.T) {
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 1000, Percentage: 100.0, TopFunc: "main.worker",
				Stack: []string{"main.worker", "runtime.gopark"}},
		},
	}

	rule := &MutexContentionRule{}
	issues := rule.Evaluate(data)
	if len(issues) != 0 {
		t.Errorf("expected no issues without mutex functions, got %d", len(issues))
	}
}

// ============================================================
// SyscallBlockingRule Tests
// ============================================================

func TestSyscallBlockingRule_Triggered_ByState(t *testing.T) {
	// 30% goroutines in "syscall" state → should trigger
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 300, Percentage: 30.0, State: "syscall", TopFunc: "cgo_wait",
				Stack: []string{"cgo_wait", "runtime.cgocall"}},
			{Count: 700, Percentage: 70.0, State: "running", TopFunc: "main.compute",
				Stack: []string{"main.compute"}},
		},
	}

	rule := &SyscallBlockingRule{}
	issues := rule.Evaluate(data)
	if len(issues) == 0 {
		t.Fatal("expected syscall blocking issue")
	}
	if issues[0].Type != "syscall_blocking" {
		t.Errorf("expected type=syscall_blocking, got %s", issues[0].Type)
	}
	if issues[0].Severity != "warning" {
		t.Errorf("expected severity=warning, got %s", issues[0].Severity)
	}
}

func TestSyscallBlockingRule_Triggered_ByStackPattern(t *testing.T) {
	// Goroutines with syscall.Syscall in stack → should trigger
	data := &model.PProfGoroutineData{
		TotalCount: 100,
		Distribution: []model.GoroutineGroup{
			{Count: 25, Percentage: 25.0, State: "running", TopFunc: "myapp.callFFI",
				Stack: []string{"myapp.callFFI", "syscall.Syscall", "runtime.cgocall"}},
			{Count: 75, Percentage: 75.0, State: "running", TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &SyscallBlockingRule{}
	issues := rule.Evaluate(data)
	if len(issues) == 0 {
		t.Fatal("expected syscall blocking issue from stack pattern")
	}
}

func TestSyscallBlockingRule_NotTriggered_LowPercentage(t *testing.T) {
	// 10% in syscall → should NOT trigger (threshold is 20%)
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 100, Percentage: 10.0, State: "syscall", TopFunc: "cgo_wait",
				Stack: []string{"cgo_wait"}},
			{Count: 900, Percentage: 90.0, State: "running", TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &SyscallBlockingRule{}
	issues := rule.Evaluate(data)
	if len(issues) != 0 {
		t.Errorf("expected no issues for 10%% syscall, got %d", len(issues))
	}
}

func TestSyscallBlockingRule_NotTriggered_NoSyscalls(t *testing.T) {
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 1000, Percentage: 100.0, State: "running", TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &SyscallBlockingRule{}
	issues := rule.Evaluate(data)
	if len(issues) != 0 {
		t.Errorf("expected no issues without syscalls, got %d", len(issues))
	}
}

// ============================================================
// ChannelLeakRule Tests
// ============================================================

func TestChannelLeakRule_Triggered_Warning(t *testing.T) {
	// 50% of 5000 goroutines blocked on channels → warning
	data := &model.PProfGoroutineData{
		TotalCount: 5000,
		Distribution: []model.GoroutineGroup{
			{Count: 2500, Percentage: 50.0, TopFunc: "myapp.producer",
				Stack: []string{"myapp.producer", "runtime.chansend", "runtime.gopark"}},
			{Count: 2500, Percentage: 50.0, TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &ChannelLeakRule{}
	issues := rule.Evaluate(data)
	if len(issues) == 0 {
		t.Fatal("expected channel leak issue")
	}
	if issues[0].Type != "channel_leak" {
		t.Errorf("expected type=channel_leak, got %s", issues[0].Type)
	}
	if issues[0].Severity != "warning" {
		t.Errorf("expected severity=warning, got %s", issues[0].Severity)
	}
	if issues[0].Suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
}

func TestChannelLeakRule_Triggered_Critical(t *testing.T) {
	// 80% of 20000 goroutines → critical
	data := &model.PProfGoroutineData{
		TotalCount: 20000,
		Distribution: []model.GoroutineGroup{
			{Count: 16000, Percentage: 80.0, TopFunc: "myapp.leakyConsumer",
				Stack: []string{"myapp.leakyConsumer", "runtime.chanrecv", "runtime.gopark"}},
			{Count: 4000, Percentage: 20.0, TopFunc: "main.healthy",
				Stack: []string{"main.healthy"}},
		},
	}

	rule := &ChannelLeakRule{}
	issues := rule.Evaluate(data)
	if len(issues) == 0 {
		t.Fatal("expected critical channel leak issue")
	}
	if issues[0].Severity != "critical" {
		t.Errorf("expected severity=critical for >70%% with >10k goroutines, got %s", issues[0].Severity)
	}
}

func TestChannelLeakRule_NotTriggered_LowPercentage(t *testing.T) {
	// 20% on channels → should NOT trigger (threshold is 40%)
	data := &model.PProfGoroutineData{
		TotalCount: 5000,
		Distribution: []model.GoroutineGroup{
			{Count: 1000, Percentage: 20.0, TopFunc: "myapp.listener",
				Stack: []string{"myapp.listener", "runtime.chanrecv"}},
			{Count: 4000, Percentage: 80.0, TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &ChannelLeakRule{}
	issues := rule.Evaluate(data)
	if len(issues) != 0 {
		t.Errorf("expected no issues for 20%% channel blocking, got %d", len(issues))
	}
}

func TestChannelLeakRule_NotTriggered_LowTotalCount(t *testing.T) {
	// 80% on channels but only 100 total goroutines → not a leak indicator
	data := &model.PProfGoroutineData{
		TotalCount: 100,
		Distribution: []model.GoroutineGroup{
			{Count: 80, Percentage: 80.0, TopFunc: "myapp.listener",
				Stack: []string{"myapp.listener", "runtime.chanrecv"}},
			{Count: 20, Percentage: 20.0, TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &ChannelLeakRule{}
	issues := rule.Evaluate(data)
	if len(issues) != 0 {
		t.Errorf("expected no issues for low total count (100), got %d", len(issues))
	}
}

func TestChannelLeakRule_SelectGo(t *testing.T) {
	// Test that runtime.selectgo is also detected
	data := &model.PProfGoroutineData{
		TotalCount: 5000,
		Distribution: []model.GoroutineGroup{
			{Count: 3000, Percentage: 60.0, TopFunc: "myapp.selectLoop",
				Stack: []string{"myapp.selectLoop", "runtime.selectgo", "runtime.gopark"}},
			{Count: 2000, Percentage: 40.0, TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &ChannelLeakRule{}
	issues := rule.Evaluate(data)
	if len(issues) == 0 {
		t.Fatal("expected channel leak issue from runtime.selectgo")
	}
}

// ============================================================
// SleepAccumulationRule Tests
// ============================================================

func TestSleepAccumulationRule_Triggered(t *testing.T) {
	// 600 goroutines sleeping → should trigger (threshold is 500)
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 600, Percentage: 60.0, TopFunc: "myapp.retryLoop",
				Stack: []string{"myapp.retryLoop", "time.Sleep", "runtime.timeSleep"}},
			{Count: 400, Percentage: 40.0, TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &SleepAccumulationRule{}
	issues := rule.Evaluate(data)
	if len(issues) == 0 {
		t.Fatal("expected sleep accumulation issue")
	}
	if issues[0].Type != "sleep_accumulation" {
		t.Errorf("expected type=sleep_accumulation, got %s", issues[0].Type)
	}
	if issues[0].Severity != "info" {
		t.Errorf("expected severity=info, got %s", issues[0].Severity)
	}
	if issues[0].Suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
	if len(issues[0].RelatedFuncs) == 0 {
		t.Error("expected non-empty RelatedFuncs")
	}
}

func TestSleepAccumulationRule_Triggered_TimerPattern(t *testing.T) {
	// time.After pattern detection
	data := &model.PProfGoroutineData{
		TotalCount: 2000,
		Distribution: []model.GoroutineGroup{
			{Count: 800, Percentage: 40.0, TopFunc: "myapp.pollWithTimeout",
				Stack: []string{"myapp.pollWithTimeout", "time.After", "runtime.gopark"}},
			{Count: 1200, Percentage: 60.0, TopFunc: "main.handler",
				Stack: []string{"main.handler"}},
		},
	}

	rule := &SleepAccumulationRule{}
	issues := rule.Evaluate(data)
	if len(issues) == 0 {
		t.Fatal("expected sleep accumulation issue from time.After")
	}
}

func TestSleepAccumulationRule_NotTriggered_LowCount(t *testing.T) {
	// Only 100 sleeping goroutines → should NOT trigger (threshold is 500)
	data := &model.PProfGoroutineData{
		TotalCount: 1000,
		Distribution: []model.GoroutineGroup{
			{Count: 100, Percentage: 10.0, TopFunc: "myapp.retry",
				Stack: []string{"myapp.retry", "time.Sleep"}},
			{Count: 900, Percentage: 90.0, TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
	}

	rule := &SleepAccumulationRule{}
	issues := rule.Evaluate(data)
	if len(issues) != 0 {
		t.Errorf("expected no issues for 100 sleeping goroutines, got %d", len(issues))
	}
}

func TestSleepAccumulationRule_NotTriggered_NoSleepFunctions(t *testing.T) {
	data := &model.PProfGoroutineData{
		TotalCount: 5000,
		Distribution: []model.GoroutineGroup{
			{Count: 5000, Percentage: 100.0, TopFunc: "main.compute",
				Stack: []string{"main.compute", "math.Sqrt"}},
		},
	}

	rule := &SleepAccumulationRule{}
	issues := rule.Evaluate(data)
	if len(issues) != 0 {
		t.Errorf("expected no issues without sleep functions, got %d", len(issues))
	}
}

// ============================================================
// Helper function tests
// ============================================================

func TestContainsInStack(t *testing.T) {
	g := &model.GoroutineGroup{
		TopFunc: "myapp.handler",
		Stack:   []string{"myapp.handler", "sync.(*Mutex).Lock", "runtime.gopark"},
	}

	if !containsInStack(g, "sync.(*Mutex).Lock") {
		t.Error("expected to find sync.(*Mutex).Lock in stack")
	}
	if !containsInStack(g, "myapp.handler") {
		t.Error("expected to find myapp.handler in TopFunc")
	}
	if containsInStack(g, "nonexistent.Function") {
		t.Error("should not find nonexistent function")
	}
}

func TestAppendUnique(t *testing.T) {
	slice := []string{"a", "b"}
	slice = appendUnique(slice, "c")
	if len(slice) != 3 {
		t.Errorf("expected length 3, got %d", len(slice))
	}

	// Adding duplicate should not increase length
	slice = appendUnique(slice, "b")
	if len(slice) != 3 {
		t.Errorf("expected length 3 after duplicate, got %d", len(slice))
	}
}

// ============================================================
// Integration test: all rules together
// ============================================================

func TestRuleEngine_MultipleIssuesDetected(t *testing.T) {
	// Data that triggers multiple rules simultaneously
	data := &model.PProfGoroutineData{
		TotalCount: 50000,
		Distribution: []model.GoroutineGroup{
			// Dominant group with mutex contention
			{Count: 30000, Percentage: 60.0, TopFunc: "myapp.cache.Get",
				Stack: []string{"myapp.cache.Get", "sync.(*Mutex).Lock", "sync.runtime_SemacquireMutex"}},
			// I/O wait group
			{Count: 15000, Percentage: 30.0, TopFunc: "net/http.(*conn).serve",
				Stack: []string{"net/http.(*conn).serve", "internal/poll.(*FD).Read"}},
			// Normal workers
			{Count: 5000, Percentage: 10.0, TopFunc: "main.worker",
				Stack: []string{"main.worker"}},
		},
		TopFuncs: []model.PProfTopFunc{
			{Name: "sync.(*Mutex).Lock", Flat: 30000, FlatPct: 60.0},
			{Name: "net/http.(*conn).serve", Flat: 15000, FlatPct: 30.0},
		},
	}

	engine := NewGoroutineRuleEngine()
	issues := engine.Evaluate(data)

	if len(issues) == 0 {
		t.Fatal("expected multiple issues for complex data")
	}

	// Should detect at least: excessive count, dominant group, blocking, mutex contention, io wait
	typeSet := make(map[string]bool)
	for _, issue := range issues {
		typeSet[issue.Type] = true
	}

	expectedTypes := []string{"excessive", "blocking", "mutex_contention"}
	for _, expected := range expectedTypes {
		if !typeSet[expected] {
			t.Errorf("expected issue type %q to be detected, found types: %v", expected, typeSet)
		}
	}
}
