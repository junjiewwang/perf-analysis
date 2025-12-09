package collapsed

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestDataPath returns the path to test data files.
func getTestDataPath(filename string) string {
	_, currentFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(currentFile)
	return filepath.Join(dir, "testdata", filename)
}

func TestParser_Parse_JavaCPUFile(t *testing.T) {
	filePath := getTestDataPath("java_cpu.folded")

	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	parser := NewParser(&ParserOptions{TopN: 10})
	result, err := parser.Parse(context.Background(), file)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Total samples: 100+50+80+30+60+20 = 340 (excluding swapper 500)
	assert.Equal(t, int64(340), result.TotalSamples)

	// Should have 7 samples total
	assert.Len(t, result.Samples, 7)

	// Check top functions
	// com.example.App.process and com.example.Worker.process should be high
	assert.Contains(t, result.TopFuncs, "com.example.App.process")
	assert.Contains(t, result.TopFuncs, "com.example.Worker.process")

	// Check thread stats
	// main-thread should have 150 samples (100+50)
	mainThreadFound := false
	for _, info := range result.ThreadStats {
		if info.ThreadName == "main-thread" {
			mainThreadFound = true
			assert.Equal(t, int64(150), info.Samples)
			break
		}
	}
	assert.True(t, mainThreadFound, "main-thread should be in thread stats")
}

func TestParser_Parse_APMFormatFile(t *testing.T) {
	filePath := getTestDataPath("apm_format.folded")

	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	parser := NewParser(nil)
	result, err := parser.Parse(context.Background(), file)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Total samples: 150+100+80+30 = 360
	assert.Equal(t, int64(360), result.TotalSamples)

	// Check thread names are correctly parsed from APM format
	threadNames := make(map[string]bool)
	for _, sample := range result.Samples {
		threadNames[sample.ThreadName] = true
	}

	assert.True(t, threadNames["Thread-7"], "Thread-7 should be parsed")
	assert.True(t, threadNames["worker-pool-1"], "worker-pool-1 should be parsed")
	assert.True(t, threadNames["main thread"], "main thread should be parsed")

	// Check TIDs are correctly parsed
	for _, sample := range result.Samples {
		if sample.ThreadName == "Thread-7" {
			assert.Equal(t, 1060369, sample.TID)
		}
	}
}

func TestParser_Parse_FileNotExists(t *testing.T) {
	filePath := getTestDataPath("nonexistent.folded")

	file, err := os.Open(filePath)
	assert.Error(t, err)
	assert.Nil(t, file)
}

func TestIntegration_FullParseWorkflow(t *testing.T) {
	filePath := getTestDataPath("java_cpu.folded")

	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	// Parse with default options
	parser := NewParser(DefaultParserOptions())
	result, err := parser.Parse(context.Background(), file)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the parsing results can be used for further analysis
	// 1. Top functions should be non-empty
	assert.NotEmpty(t, result.TopFuncs)

	// 2. Thread stats should be non-empty
	assert.NotEmpty(t, result.ThreadStats)

	// 3. Samples should be properly structured
	for _, sample := range result.Samples {
		assert.NotEmpty(t, sample.ThreadName)
		assert.NotEmpty(t, sample.CallStack)
		assert.Greater(t, sample.Value, int64(0))
	}

	// 4. Total samples should match sum of individual samples (excluding swapper)
	var totalFromSamples int64
	for _, sample := range result.Samples {
		if !IsSwapperThread(sample.ThreadName) {
			totalFromSamples += sample.Value
		}
	}
	assert.Equal(t, result.TotalSamples, totalFromSamples)
}

// TestIntegration_TopFuncsCallstacks verifies call stack tracking.
func TestIntegration_TopFuncsCallstacks(t *testing.T) {
	filePath := getTestDataPath("java_cpu.folded")

	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	parser := NewParser(&ParserOptions{TopN: 20})
	result, err := parser.Parse(context.Background(), file)

	require.NoError(t, err)

	// Check that call stacks are tracked for top functions
	// com.example.Worker.process appears twice with different call stacks
	if callstackInfo, ok := result.TopFuncsCallstacks["com.example.Worker.process"]; ok {
		assert.Greater(t, callstackInfo.Count, 0)
		assert.NotEmpty(t, callstackInfo.CallStacks)
	}
}
