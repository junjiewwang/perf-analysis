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

func BenchmarkExtractThreadInfo(b *testing.B) {
	testCases := []string{
		"java-12345/67890",
		"[Thread-7 tid=1060369]",
		"pool-1-thread-1-12345/67890",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			ExtractThreadInfo(tc)
		}
	}
}

func BenchmarkSplitFuncAndModule(b *testing.B) {
	testCases := []string{
		"doSomething(libfoo.so)",
		"java.lang.Thread.run",
		"tcp_sendmsg([kernel.kallsyms])",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			SplitFuncAndModule(tc)
		}
	}
}
