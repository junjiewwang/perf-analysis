package hprof

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/junjiewwang/perf-analysis/perflib"
)

// TestBuildPassLargeFile compares parallel vs sequential Build Pass on a large hprof file.
// This test is skipped if the large test file doesn't exist or in short mode.
// Run with: go test -v -run TestBuildPassLargeFile -timeout 300s
func TestBuildPassLargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large file benchmark in short mode")
	}

	const filePath = "../../../test/heap.hprof"

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Skipf("large test file not found: %s", filePath)
	}

	ctx := context.Background()
	logger := &benchLogger{t: t}
	parser := NewParser(&ParserOptions{Logger: logger})

	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer file.Close()

	fi, _ := file.Stat()
	t.Logf("File size: %.2f GB", float64(fi.Size())/(1024*1024*1024))

	// Phase 1: Scan Pass
	t.Log("=== Starting Scan Pass ===")
	scanStart := time.Now()
	scanResult, err := parser.ScanPass(ctx, file)
	if err != nil {
		t.Fatalf("scan pass failed: %v", err)
	}
	scanDuration := time.Since(scanStart)
	t.Logf("Scan Pass: %v (objects: %d, edges: %d, segments: %d)",
		scanDuration, scanResult.ObjectCount, scanResult.EdgeCount, len(scanResult.SegmentOffsets))

	// Phase 2a: Parallel Build Pass
	t.Log("=== Starting Parallel Build Pass ===")
	parallelStart := time.Now()
	parallelGraph, err := parser.BuildPassParallel(ctx, file, scanResult)
	if err != nil {
		t.Fatalf("parallel build pass failed: %v", err)
	}
	parallelDuration := time.Since(parallelStart)
	t.Logf("Parallel Build Pass: %v (outgoing edges: %d, incoming edges: %d)",
		parallelDuration, parallelGraph.GetOutgoing().TotalEdges(), parallelGraph.GetIncoming().TotalEdges())

	// Phase 2b: Sequential Build Pass (need to re-scan since degree counts were consumed)
	t.Log("=== Starting Sequential Build Pass (comparison) ===")
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatalf("seek failed: %v", err)
	}
	scanResult2, err := parser.ScanPass(ctx, file)
	if err != nil {
		t.Fatalf("scan pass 2 failed: %v", err)
	}
	seqStart := time.Now()
	seqGraph, err := parser.BuildPass(ctx, file, scanResult2)
	if err != nil {
		t.Fatalf("sequential build pass failed: %v", err)
	}
	seqDuration := time.Since(seqStart)
	t.Logf("Sequential Build Pass: %v (outgoing edges: %d, incoming edges: %d)",
		seqDuration, seqGraph.GetOutgoing().TotalEdges(), seqGraph.GetIncoming().TotalEdges())

	// Compare results
	t.Log("=== Results Comparison ===")
	t.Logf("Parallel: %v", parallelDuration)
	t.Logf("Sequential: %v", seqDuration)
	t.Logf("Speedup: %.2fx", float64(seqDuration)/float64(parallelDuration))
	t.Logf("Object count match: %v", parallelGraph.ObjectCount() == seqGraph.ObjectCount())
	t.Logf("Outgoing edge count match: %v (par=%d, seq=%d)",
		parallelGraph.GetOutgoing().TotalEdges() == seqGraph.GetOutgoing().TotalEdges(),
		parallelGraph.GetOutgoing().TotalEdges(), seqGraph.GetOutgoing().TotalEdges())
	t.Logf("Incoming edge count match: %v (par=%d, seq=%d)",
		parallelGraph.GetIncoming().TotalEdges() == seqGraph.GetIncoming().TotalEdges(),
		parallelGraph.GetIncoming().TotalEdges(), seqGraph.GetIncoming().TotalEdges())
}

// benchLogger implements perflib.Logger for test output.
type benchLogger struct {
	t *testing.T
}

var _ perflib.Logger = (*benchLogger)(nil)

func (l *benchLogger) Info(format string, args ...interface{}) {
	l.t.Logf("[INFO] "+format, args...)
}

func (l *benchLogger) Debug(format string, args ...interface{}) {
	// skip debug in benchmarks to reduce noise
}

func (l *benchLogger) Error(format string, args ...interface{}) {
	l.t.Logf("[ERROR] "+format, args...)
}

func (l *benchLogger) Warn(format string, args ...interface{}) {
	l.t.Logf("[WARN] "+format, args...)
}
