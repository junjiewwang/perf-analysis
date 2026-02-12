package analyzer

import (
	"testing"

	"github.com/perf-analysis/pkg/model"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    AnalysisMode
		wantErr bool
	}{
		{"async-profiler-cpu", "async-profiler-cpu", ModeJavaCPU, false},
		{"uppercase", "ASYNC-PROFILER-CPU", ModeJavaCPU, false},
		{"with spaces", "  async-profiler-cpu  ", ModeJavaCPU, false},
		{"async-profiler-alloc", "async-profiler-alloc", ModeJavaAlloc, false},
		{"async-profiler-wall", "async-profiler-wall", ModeJavaWall, false},
		{"async-profiler-lock", "async-profiler-lock", ModeJavaLock, false},
		{"heapdump-heap", "heapdump-heap", ModeJavaHeap, false},
		{"perf-cpu", "perf-cpu", ModeCPU, false},
		{"pprof-cpu", "pprof-cpu", ModePProfCPU, false},
		{"pprof-heap", "pprof-heap", ModePProfHeap, false},
		{"pprof-goroutine", "pprof-goroutine", ModePProfGoroutine, false},
		{"pprof-block", "pprof-block", ModePProfBlock, false},
		{"pprof-mutex", "pprof-mutex", ModePProfMutex, false},
		{"pprof-all", "pprof-all", ModePProfAll, false},
		{"jeprof-heap", "jeprof-heap", ModeJeprof, false},
		{"invalid", "invalid-mode", "", true},
		{"empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModeInfo_ProfilerAndEvent(t *testing.T) {
	tests := []struct {
		mode     AnalysisMode
		profiler model.Profiler
		event    model.EventType
		resource model.ResourceType
	}{
		{ModeJavaCPU, model.ProfilerAsyncProfiler, model.EventCPU, model.ResourceCPU},
		{ModeJavaAlloc, model.ProfilerAsyncProfiler, model.EventAlloc, model.ResourceMemory},
		{ModeJavaWall, model.ProfilerAsyncProfiler, model.EventWall, model.ResourceApp},
		{ModeJavaLock, model.ProfilerAsyncProfiler, model.EventLock, model.ResourceConcurrency},
		{ModeJavaHeap, model.ProfilerHeapDump, model.EventHeap, model.ResourceMemory},
		{ModeCPU, model.ProfilerPerf, model.EventCPU, model.ResourceCPU},
		{ModePProfCPU, model.ProfilerPProf, model.EventCPU, model.ResourceCPU},
		{ModePProfHeap, model.ProfilerPProf, model.EventHeap, model.ResourceMemory},
		{ModePProfGoroutine, model.ProfilerPProf, model.EventGoroutine, model.ResourceGoroutine},
		{ModePProfBlock, model.ProfilerPProf, model.EventBlock, model.ResourceConcurrency},
		{ModePProfMutex, model.ProfilerPProf, model.EventMutex, model.ResourceConcurrency},
		{ModePProfAll, model.ProfilerPProf, model.EventCPU, model.ResourceCPU},
		{ModeJeprof, model.ProfilerJeprof, model.EventHeap, model.ResourceMemory},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			info, ok := GetModeInfo(tt.mode)
			if !ok {
				t.Fatalf("GetModeInfo(%q) not found", tt.mode)
			}
			if info.Profiler != tt.profiler {
				t.Errorf("Profiler = %v, want %v", info.Profiler, tt.profiler)
			}
			if info.Event != tt.event {
				t.Errorf("Event = %v, want %v", info.Event, tt.event)
			}
			if info.Resource != tt.resource {
				t.Errorf("Resource = %v, want %v", info.Resource, tt.resource)
			}
		})
	}
}

func TestResourceType(t *testing.T) {
	tests := []struct {
		mode     AnalysisMode
		resource model.ResourceType
	}{
		{ModeJavaCPU, model.ResourceCPU},
		{ModeJavaAlloc, model.ResourceMemory},
		{ModeJavaWall, model.ResourceApp},
		{ModeJavaLock, model.ResourceConcurrency},
		{ModePProfGoroutine, model.ResourceGoroutine},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if got := tt.mode.ResourceType(); got != tt.resource {
				t.Errorf("ResourceType() = %v, want %v", got, tt.resource)
			}
		})
	}
}

func TestGetModeInfo(t *testing.T) {
	tests := []struct {
		mode    AnalysisMode
		wantOk  bool
		wantNil bool
	}{
		{ModeJavaCPU, true, false},
		{ModeJavaAlloc, true, false},
		{ModeJavaWall, true, false},
		{ModeJavaLock, true, false},
		{ModeJavaHeap, true, false},
		{ModeCPU, true, false},
		{ModePProfCPU, true, false},
		{ModePProfHeap, true, false},
		{ModePProfGoroutine, true, false},
		{ModePProfBlock, true, false},
		{ModePProfMutex, true, false},
		{ModePProfAll, true, false},
		{ModeJeprof, true, false},
		{"invalid", false, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			info, ok := GetModeInfo(tt.mode)
			if ok != tt.wantOk {
				t.Errorf("GetModeInfo() ok = %v, want %v", ok, tt.wantOk)
			}
			if (info == nil) != tt.wantNil {
				t.Errorf("GetModeInfo() info nil = %v, want nil = %v", info == nil, tt.wantNil)
			}
		})
	}
}

func TestAllModes(t *testing.T) {
	modes := AllModes()
	if len(modes) != 13 {
		t.Errorf("AllModes() returned %d modes, want 13", len(modes))
	}

	expectedOrder := []AnalysisMode{
		ModeJavaCPU, ModeJavaAlloc, ModeJavaWall, ModeJavaLock, ModeJavaHeap, ModeCPU,
		ModePProfCPU, ModePProfHeap, ModePProfGoroutine, ModePProfBlock, ModePProfMutex, ModePProfAll,
		ModeJeprof,
	}
	for i, info := range modes {
		if info.Mode != expectedOrder[i] {
			t.Errorf("AllModes()[%d] = %v, want %v", i, info.Mode, expectedOrder[i])
		}
	}
}

func TestValidModes(t *testing.T) {
	valid := ValidModes()
	expectedModes := []string{
		"async-profiler-cpu", "async-profiler-alloc", "async-profiler-wall", "async-profiler-lock",
		"heapdump-heap", "perf-cpu",
		"pprof-cpu", "pprof-heap", "pprof-goroutine", "pprof-block", "pprof-mutex", "pprof-all",
		"jeprof-heap",
	}
	for _, mode := range expectedModes {
		if !contains(valid, mode) {
			t.Errorf("ValidModes() should contain %q", mode)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFactory_CreateAnalyzerForMode(t *testing.T) {
	factory := NewFactory(nil)

	tests := []struct {
		mode     AnalysisMode
		wantName string
		wantErr  bool
	}{
		{ModeJavaCPU, "java_cpu_analyzer", false},
		{ModeJavaAlloc, "java_mem_analyzer", false},
		{ModeJavaWall, "java_cpu_analyzer", false},
		{ModeJavaLock, "java_cpu_analyzer", false},
		{ModeJavaHeap, "java_heap_analyzer", false},
		{ModeCPU, "java_cpu_analyzer", false},
		{ModePProfCPU, "pprof_cpu_analyzer", false},
		{ModePProfHeap, "pprof_heap_analyzer", false},
		{ModePProfGoroutine, "pprof_goroutine_analyzer", false},
		{ModePProfBlock, "pprof_block_analyzer", false},
		{ModePProfMutex, "pprof_mutex_analyzer", false},
		{ModePProfAll, "pprof_batch_analyzer", false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			ana, err := factory.CreateAnalyzerForMode(tt.mode)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateAnalyzerForMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && ana.Name() != tt.wantName {
				t.Errorf("CreateAnalyzerForMode() analyzer name = %v, want %v", ana.Name(), tt.wantName)
			}
		})
	}
}
