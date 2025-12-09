package collapsed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitFuncAndModule(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFunc   string
		wantModule string
	}{
		{
			name:       "function with module",
			input:      "doSomething(libfoo.so)",
			wantFunc:   "doSomething",
			wantModule: "libfoo.so",
		},
		{
			name:       "function without module",
			input:      "doSomething",
			wantFunc:   "doSomething",
			wantModule: "",
		},
		{
			name:       "java method",
			input:      "java.lang.Thread.run(Thread.java)",
			wantFunc:   "java.lang.Thread.run",
			wantModule: "Thread.java",
		},
		{
			name:       "kernel symbol with module",
			input:      "tcp_sendmsg([kernel.kallsyms])",
			wantFunc:   "tcp_sendmsg",
			wantModule: "[kernel.kallsyms]",
		},
		{
			name:       "nested parentheses",
			input:      "operator()(mystuff.so)",
			wantFunc:   "operator()",
			wantModule: "mystuff.so",
		},
		{
			name:       "empty input",
			input:      "",
			wantFunc:   "",
			wantModule: "",
		},
		{
			name:       "only opening paren",
			input:      "func(",
			wantFunc:   "func(",
			wantModule: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFunc, gotModule := SplitFuncAndModule(tt.input)
			assert.Equal(t, tt.wantFunc, gotFunc)
			assert.Equal(t, tt.wantModule, gotModule)
		})
	}
}

func TestExtractThreadInfo(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantThreadName string
		wantTID        int
	}{
		{
			name:           "standard perf format",
			input:          "sap1009-?/1088670",
			wantThreadName: "sap1009",
			wantTID:        1088670,
		},
		{
			name:           "APM format",
			input:          "[Thread-7 tid=1060369]",
			wantThreadName: "Thread-7",
			wantTID:        1060369,
		},
		{
			name:           "process with pid",
			input:          "java-12345/67890",
			wantThreadName: "java",
			wantTID:        67890,
		},
		{
			name:           "swapper thread",
			input:          "swapper-?/0",
			wantThreadName: "swapper",
			wantTID:        0,
		},
		{
			name:           "complex thread name",
			input:          "pool-1-thread-1-12345/67890",
			wantThreadName: "pool-1-thread-1",
			wantTID:        67890,
		},
		{
			name:           "APM format with spaces",
			input:          "[main thread tid=12345]",
			wantThreadName: "main thread",
			wantTID:        12345,
		},
		{
			name:           "no tid info",
			input:          "process_name",
			wantThreadName: "process_name",
			wantTID:        -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ExtractThreadInfo(tt.input)
			assert.Equal(t, tt.wantThreadName, info.ThreadName)
			assert.Equal(t, tt.wantTID, info.TID)
		})
	}
}

func TestIsSwapperThread(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"swapper-?/0", true},
		{"swapper-0/0", true},
		{"swapper", true},
		{"java-12345/67890", false},
		{"", false},
		{"swap", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsSwapperThread(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsInvalidData(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"5_2175795_[002]_83367.826506:-?/10101010", true},
		{"123_456_something", true},
		{"java-12345/67890", false},
		{"swapper-?/0", false},
		{"1_notdigit_test", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsInvalidData(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseStackFrame(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFunc   string
		wantModule string
	}{
		{
			name:       "with module",
			input:      "runtime.schedule(go)",
			wantFunc:   "runtime.schedule",
			wantModule: "go",
		},
		{
			name:       "without module",
			input:      "main.main",
			wantFunc:   "main.main",
			wantModule: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := ParseStackFrame(tt.input)
			require.NotNil(t, frame)
			assert.Equal(t, tt.wantFunc, frame.Function)
			assert.Equal(t, tt.wantModule, frame.Module)
		})
	}
}

func TestParseCallStack(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantThreadName string
		wantTID        int
		wantFrameCount int
		wantTopFunc    string
	}{
		{
			name:           "standard stack",
			input:          "java-12345/67890;Thread.run;App.main;App.process",
			wantThreadName: "java",
			wantTID:        67890,
			wantFrameCount: 3,
			wantTopFunc:    "App.process",
		},
		{
			name:           "APM format stack",
			input:          "[Thread-7 tid=1060369];java.lang.Thread.run;com.example.App.main",
			wantThreadName: "Thread-7",
			wantTID:        1060369,
			wantFrameCount: 2,
			wantTopFunc:    "com.example.App.main",
		},
		{
			name:           "single frame",
			input:          "process-?/123;single_func",
			wantThreadName: "process",
			wantTID:        123,
			wantFrameCount: 1,
			wantTopFunc:    "single_func",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threadInfo, frames := ParseCallStack(tt.input)
			require.NotNil(t, threadInfo)
			assert.Equal(t, tt.wantThreadName, threadInfo.ThreadName)
			assert.Equal(t, tt.wantTID, threadInfo.TID)
			assert.Len(t, frames, tt.wantFrameCount)
			if tt.wantFrameCount > 0 {
				assert.Equal(t, tt.wantTopFunc, GetStackTopFunction(frames))
			}
		})
	}
}

func TestFramesToCallStackString(t *testing.T) {
	tests := []struct {
		name   string
		frames []*StackFrame
		want   string
	}{
		{
			name: "multiple frames",
			frames: []*StackFrame{
				{Function: "func1"},
				{Function: "func2"},
				{Function: "func3"},
			},
			want: "func1;func2;func3",
		},
		{
			name:   "empty frames",
			frames: []*StackFrame{},
			want:   "",
		},
		{
			name: "single frame",
			frames: []*StackFrame{
				{Function: "single"},
			},
			want: "single",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FramesToCallStackString(tt.frames)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetStackTopFunction(t *testing.T) {
	tests := []struct {
		name   string
		frames []*StackFrame
		want   string
	}{
		{
			name: "multiple frames",
			frames: []*StackFrame{
				{Function: "func1"},
				{Function: "func2"},
				{Function: "top_func"},
			},
			want: "top_func",
		},
		{
			name:   "empty frames",
			frames: []*StackFrame{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetStackTopFunction(tt.frames)
			assert.Equal(t, tt.want, got)
		})
	}
}
