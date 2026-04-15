package collapsed

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_Parse_BasicInput(t *testing.T) {
	input := `main-thread-?/1234;java.lang.Thread.run;com.example.App.main 100
worker-1-?/5678;java.lang.Thread.run;com.example.Worker.process 50
main-thread-?/1234;java.lang.Thread.run;com.example.App.init 30`

	parser := NewParser(nil)
	result, err := parser.Parse(context.Background(), strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)

	// Check total samples
	assert.Equal(t, int64(180), result.TotalSamples)

	// Check samples count
	assert.Len(t, result.Samples, 3)

	// Check first sample
	assert.Equal(t, "main-thread", result.Samples[0].ThreadName)
	assert.Equal(t, 1234, result.Samples[0].TID)
	assert.Equal(t, int64(100), result.Samples[0].Value)
}

func TestParser_Parse_EmptyInput(t *testing.T) {
	parser := NewParser(nil)
	result, err := parser.Parse(context.Background(), strings.NewReader(""))

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int64(0), result.TotalSamples)
	assert.Empty(t, result.Samples)
}

func TestParser_Parse_SwapperExclusion(t *testing.T) {
	input := `main-thread-?/1234;func1;func2 100
swapper-?/0;idle_func 50`

	parser := NewParser(nil)
	result, err := parser.Parse(context.Background(), strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)

	// Total samples should exclude swapper
	assert.Equal(t, int64(100), result.TotalSamples)

	// But we should have 2 samples stored
	assert.Len(t, result.Samples, 2)
}

func TestParser_Parse_TopFuncs(t *testing.T) {
	input := `thread-?/1;a;b;hot_func 100
thread-?/2;a;b;hot_func 80
thread-?/3;a;c;other_func 50
thread-?/4;a;d;rare_func 10`

	parser := NewParser(&ParserOptions{TopN: 2})
	result, err := parser.Parse(context.Background(), strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)

	// Check top functions
	assert.Len(t, result.TopFuncs, 2)

	// hot_func should be top (180 samples)
	hotFuncValue, ok := result.TopFuncs["hot_func"]
	assert.True(t, ok)
	assert.Greater(t, hotFuncValue.Self, 0.0)

	// other_func should be second
	otherFuncValue, ok := result.TopFuncs["other_func"]
	assert.True(t, ok)
	assert.Greater(t, otherFuncValue.Self, 0.0)

	// rare_func should not be in top 2
	_, ok = result.TopFuncs["rare_func"]
	assert.False(t, ok)
}

func TestParser_Parse_APMFormat(t *testing.T) {
	input := `[Thread-7 tid=1060369];java.lang.Thread.run;com.example.App.main 100
[worker-pool-1 tid=1060370];java.lang.Thread.run;com.example.Worker.process 50`

	parser := NewParser(nil)
	result, err := parser.Parse(context.Background(), strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int64(150), result.TotalSamples)
	assert.Len(t, result.Samples, 2)

	// Check APM thread parsing
	assert.Equal(t, "Thread-7", result.Samples[0].ThreadName)
	assert.Equal(t, 1060369, result.Samples[0].TID)
}

func TestParser_Parse_InvalidDataSkipped(t *testing.T) {
	input := `5_2175795_[002]_83367.826506:-?/10101010;[] 1
main-thread-?/1234;valid;stack 100`

	parser := NewParser(nil)
	result, err := parser.Parse(context.Background(), strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)

	// Only valid data should be counted
	assert.Equal(t, int64(100), result.TotalSamples)
	assert.Len(t, result.Samples, 1)
}

func TestParser_Parse_ThreadStats(t *testing.T) {
	input := `thread-A-?/1;func1 100
thread-A-?/1;func2 50
thread-B-?/2;func1 30`

	parser := NewParser(nil)
	result, err := parser.Parse(context.Background(), strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)

	// Check thread stats
	assert.Len(t, result.ThreadStats, 2)

	// Thread A should have 150 samples
	threadA, ok := result.ThreadStats["1"]
	assert.True(t, ok)
	assert.Equal(t, int64(150), threadA.Samples)

	// Thread B should have 30 samples
	threadB, ok := result.ThreadStats["2"]
	assert.True(t, ok)
	assert.Equal(t, int64(30), threadB.Samples)
}

func TestParser_Parse_StrictMode(t *testing.T) {
	input := `main-thread-?/1234;func1 100
invalid line without count
thread-?/5678;func2 50`

	// With strict mode disabled, should skip invalid lines
	parser := NewParser(&ParserOptions{StrictMode: false})
	result, err := parser.Parse(context.Background(), strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Samples, 2)

	// With strict mode enabled, should fail on invalid line
	parserStrict := NewParser(&ParserOptions{StrictMode: true})
	_, err = parserStrict.Parse(context.Background(), strings.NewReader(input))
	assert.Error(t, err)
}

func TestParser_Parse_ContextCancellation(t *testing.T) {
	input := strings.Repeat("thread-?/1;func 1\n", 1000)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	parser := NewParser(nil)
	_, err := parser.Parse(ctx, strings.NewReader(input))

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestParser_Parse_LargeSampleCount(t *testing.T) {
	input := `thread-?/1;func1;func2;func3 999999999`

	parser := NewParser(nil)
	result, err := parser.Parse(context.Background(), strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int64(999999999), result.TotalSamples)
}

func TestParser_Parse_ModuleExtraction(t *testing.T) {
	input := `thread-?/1;runtime.schedule(go);main.main 100`

	parser := NewParser(nil)
	result, err := parser.Parse(context.Background(), strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Samples, 1)

	// Call stack should have function names without modules
	assert.Contains(t, result.Samples[0].CallStack, "runtime.schedule")
	assert.Contains(t, result.Samples[0].CallStack, "main.main")
}

func TestParser_SupportedFormats(t *testing.T) {
	parser := NewParser(nil)
	formats := parser.SupportedFormats()

	assert.Contains(t, formats, "collapsed")
	assert.Contains(t, formats, "folded")
}

func TestParser_Name(t *testing.T) {
	parser := NewParser(nil)
	assert.Equal(t, "collapsed", parser.Name())
}

func TestIsCollapsedFormat(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"thread;func1;func2 100", true},
		{"a;b;c 1", true},
		{"single 42", true},
		{"no_count_here", false},
		{"", false},
		{"   ", false},
		{"thread;func abc", false}, // non-numeric count
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsCollapsedFormat(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParser_Parse_TopFuncsCallstacks(t *testing.T) {
	input := `thread-?/1;a;b;hot_func 50
thread-?/2;a;c;hot_func 30
thread-?/3;x;y;hot_func 20`

	parser := NewParser(&ParserOptions{TopN: 5})
	result, err := parser.Parse(context.Background(), strings.NewReader(input))

	require.NoError(t, err)
	require.NotNil(t, result)

	// Check call stacks for hot_func
	callstackInfo, ok := result.TopFuncsCallstacks["hot_func"]
	assert.True(t, ok)
	assert.Equal(t, "hot_func", callstackInfo.FunctionName)
	assert.Equal(t, 3, callstackInfo.Count) // 3 unique call stacks
	assert.LessOrEqual(t, len(callstackInfo.CallStacks), 5)
}

func TestSplitFuncAndModule(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFunc   string
		wantModule string
	}{
		{"function with module", "doSomething(libfoo.so)", "doSomething", "libfoo.so"},
		{"function without module", "doSomething", "doSomething", ""},
		{"java method", "java.lang.Thread.run(Thread.java)", "java.lang.Thread.run", "Thread.java"},
		{"kernel symbol with module", "tcp_sendmsg([kernel.kallsyms])", "tcp_sendmsg", "[kernel.kallsyms]"},
		{"nested parentheses", "operator()(mystuff.so)", "operator()", "mystuff.so"},
		{"empty input", "", "", ""},
		{"only opening paren", "func(", "func(", ""},
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
		{"standard perf format", "sap1009-?/1088670", "sap1009", 1088670},
		{"APM format", "[Thread-7 tid=1060369]", "Thread-7", 1060369},
		{"process with pid", "java-12345/67890", "java", 67890},
		{"swapper thread", "swapper-?/0", "swapper", 0},
		{"complex thread name", "pool-1-thread-1-12345/67890", "pool-1-thread-1", 67890},
		{"APM format with spaces", "[main thread tid=12345]", "main thread", 12345},
		{"no tid info", "process_name", "process_name", -1},
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
		{"with module", "runtime.schedule(go)", "runtime.schedule", "go"},
		{"without module", "main.main", "main.main", ""},
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

// Benchmark tests
func BenchmarkParser_Parse(b *testing.B) {
	// Generate test input
	var builder strings.Builder
	for i := 0; i < 10000; i++ {
		builder.WriteString("thread-?/1;func1;func2;func3;func4;func5 100\n")
	}
	input := builder.String()

	parser := NewParser(nil)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = parser.Parse(context.Background(), strings.NewReader(input))
	}
}
