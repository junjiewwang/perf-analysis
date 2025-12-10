package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAnalysisContext(t *testing.T) {
	ctx := NewAnalysisContext()

	assert.NotNil(t, ctx)
	assert.NotNil(t, ctx.Suggestions)
	assert.Empty(t, ctx.Suggestions)
	assert.NotNil(t, ctx.APMConfig)
	assert.Equal(t, AnalysisStatusPending, ctx.AnalysisStatus)
}

func TestAnalysisContext_SetFromNamespaceResult(t *testing.T) {
	ctx := NewAnalysisContext()

	ns := &NamespaceResult{
		TopFuncs:               `{"func1": 100}`,
		ActiveThreadsJSON:      `[{"tid": 1}]`,
		FlameGraphFile:         "fg.json.gz",
		ExtendedFlameGraphFile: "extended_fg.json.gz",
		CallGraphFile:          "cg.json",
		TotalRecords:           1000,
		Suggestions: []Suggestion{
			{Suggestion: "test suggestion"},
		},
	}

	ctx.SetFromNamespaceResult(ns)

	assert.Equal(t, ns.TopFuncs, ctx.TopFuncs)
	assert.Equal(t, ns.TopFuncs, ctx.TopFuncsWithSwapper)
	assert.Equal(t, ns.ActiveThreadsJSON, ctx.ActiveThreadJSON)
	assert.Equal(t, ns.FlameGraphFile, ctx.FlameGraphFile)
	assert.Equal(t, ns.ExtendedFlameGraphFile, ctx.ExtendedFlameGraphFile)
	assert.Equal(t, ns.CallGraphFile, ctx.CallGraphFile)
	assert.Equal(t, ns.TotalRecords, ctx.TotalRecords)
	assert.Equal(t, ns.TotalRecords, ctx.TotalRecordsWithSwapper)
	assert.Len(t, ctx.Suggestions, 1)
}

func TestParseResult(t *testing.T) {
	result := &ParseResult{
		Samples: []*Sample{
			{ThreadName: "main", Value: 100},
			{ThreadName: "worker", Value: 50},
		},
		TotalSamples: 150,
		ThreadStats: map[string]*ThreadInfo{
			"main":   {TID: 1, ThreadName: "main", Samples: 100},
			"worker": {TID: 2, ThreadName: "worker", Samples: 50},
		},
		TopFuncs: TopFuncsMap{
			"func1": TopFuncValue{Self: 50.0},
			"func2": TopFuncValue{Self: 30.0},
		},
	}

	assert.Equal(t, int64(150), result.TotalSamples)
	assert.Len(t, result.Samples, 2)
	assert.Len(t, result.ThreadStats, 2)
	assert.Len(t, result.TopFuncs, 2)
}

func TestSample(t *testing.T) {
	sample := &Sample{
		ThreadName: "main-thread",
		TID:        12345,
		CallStack:  []string{"func1", "func2", "func3"},
		Value:      100,
	}

	assert.Equal(t, "main-thread", sample.ThreadName)
	assert.Equal(t, 12345, sample.TID)
	assert.Len(t, sample.CallStack, 3)
	assert.Equal(t, int64(100), sample.Value)
}

func TestTopFuncsMap(t *testing.T) {
	funcs := TopFuncsMap{
		"com.example.App.process": TopFuncValue{Self: 45.5, Total: 60.0},
		"com.example.App.load":    TopFuncValue{Self: 25.0, Total: 40.0},
	}

	assert.Equal(t, 45.5, funcs["com.example.App.process"].Self)
	assert.Equal(t, 60.0, funcs["com.example.App.process"].Total)
}

func TestThreadInfo(t *testing.T) {
	info := &ThreadInfo{
		TID:        12345,
		ThreadName: "worker-1",
		Samples:    500,
		Percentage: 25.5,
	}

	assert.Equal(t, 12345, info.TID)
	assert.Equal(t, "worker-1", info.ThreadName)
	assert.Equal(t, int64(500), info.Samples)
	assert.Equal(t, 25.5, info.Percentage)
}

func TestAnalysisRequest(t *testing.T) {
	masterTID := "master-123"
	req := &AnalysisRequest{
		TaskID:        1,
		TaskUUID:      "uuid-123",
		TaskType:      TaskTypeJava,
		ProfilerType:  ProfilerTypePerf,
		ResultFile:    "result.collapsed",
		UserName:      "testuser",
		MasterTaskTID: &masterTID,
		COSBucket:     "bucket-1",
		RequestParams: RequestParams{
			Duration: 60,
		},
	}

	assert.Equal(t, int64(1), req.TaskID)
	assert.Equal(t, "uuid-123", req.TaskUUID)
	assert.Equal(t, TaskTypeJava, req.TaskType)
	assert.NotNil(t, req.MasterTaskTID)
	assert.Equal(t, "master-123", *req.MasterTaskTID)
}
