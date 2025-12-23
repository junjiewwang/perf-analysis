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
		{"java-cpu", "java-cpu", ModeJavaCPU, false},
		{"java-cpu uppercase", "JAVA-CPU", ModeJavaCPU, false},
		{"java-cpu with spaces", "  java-cpu  ", ModeJavaCPU, false},
		{"java-alloc", "java-alloc", ModeJavaAlloc, false},
		{"java-heap", "java-heap", ModeJavaHeap, false},
		{"cpu", "cpu", ModeCPU, false},
		{"pprof-cpu", "pprof-cpu", ModePProfCPU, false},
		{"pprof-heap", "pprof-heap", ModePProfHeap, false},
		{"pprof-goroutine", "pprof-goroutine", ModePProfGoroutine, false},
		{"pprof-block", "pprof-block", ModePProfBlock, false},
		{"pprof-mutex", "pprof-mutex", ModePProfMutex, false},
		{"pprof-all", "pprof-all", ModePProfAll, false},
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

func TestAnalysisMode_ToTaskType(t *testing.T) {
	tests := []struct {
		mode AnalysisMode
		want model.TaskType
	}{
		{ModeJavaCPU, model.TaskTypeJava},
		{ModeJavaAlloc, model.TaskTypeJava},
		{ModeJavaHeap, model.TaskTypeJavaHeap},
		{ModeCPU, model.TaskTypeGeneric},
		{ModePProfCPU, model.TaskTypePProfCPU},
		{ModePProfHeap, model.TaskTypePProfHeap},
		{ModePProfGoroutine, model.TaskTypePProfGoroutine},
		{ModePProfBlock, model.TaskTypePProfBlock},
		{ModePProfMutex, model.TaskTypePProfMutex},
		{ModePProfAll, model.TaskTypePProfCPU}, // Primary type
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if got := tt.mode.ToTaskType(); got != tt.want {
				t.Errorf("ToTaskType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnalysisMode_ToProfilerType(t *testing.T) {
	tests := []struct {
		mode AnalysisMode
		want model.ProfilerType
	}{
		{ModeJavaCPU, model.ProfilerTypePerf},
		{ModeJavaAlloc, model.ProfilerTypeAsyncAlloc},
		{ModeJavaHeap, model.ProfilerTypePerf},
		{ModeCPU, model.ProfilerTypePerf},
		{ModePProfCPU, model.ProfilerTypePProf},
		{ModePProfHeap, model.ProfilerTypePProf},
		{ModePProfGoroutine, model.ProfilerTypePProf},
		{ModePProfBlock, model.ProfilerTypePProf},
		{ModePProfMutex, model.ProfilerTypePProf},
		{ModePProfAll, model.ProfilerTypePProf},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if got := tt.mode.ToProfilerType(); got != tt.want {
				t.Errorf("ToProfilerType() = %v, want %v", got, tt.want)
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
		{ModeJavaHeap, true, false},
		{ModeCPU, true, false},
		{ModePProfCPU, true, false},
		{ModePProfHeap, true, false},
		{ModePProfGoroutine, true, false},
		{ModePProfBlock, true, false},
		{ModePProfMutex, true, false},
		{ModePProfAll, true, false},
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
	if len(modes) != 10 {
		t.Errorf("AllModes() returned %d modes, want 10", len(modes))
	}

	// Verify order
	expectedOrder := []AnalysisMode{
		ModeJavaCPU, ModeJavaAlloc, ModeJavaHeap, ModeCPU,
		ModePProfCPU, ModePProfHeap, ModePProfGoroutine, ModePProfBlock, ModePProfMutex, ModePProfAll,
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
		"java-cpu", "java-alloc", "java-heap", "cpu",
		"pprof-cpu", "pprof-heap", "pprof-goroutine", "pprof-block", "pprof-mutex", "pprof-all",
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
		{ModeJavaHeap, "java_heap_analyzer", false},
		{ModeCPU, "java_cpu_analyzer", false}, // Generic uses same analyzer
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
