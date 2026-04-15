package callgraph

import (
	"testing"
)

func TestMakeNodeID(t *testing.T) {
	tests := []struct {
		name   string
		module string
		want   string
	}{
		{"func", "", "func"},
		{"func", "mod", "func(mod)"},
		{"java.lang.Thread.run", "Thread.java", "java.lang.Thread.run(Thread.java)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := makeNodeID(tt.name, tt.module)
			if got != tt.want {
				t.Errorf("makeNodeID(%q, %q) = %q, want %q", tt.name, tt.module, got, tt.want)
			}
		})
	}
}

func TestInternalMaps(t *testing.T) {
	// Test that internal maps are properly initialized
	cg := NewCallGraph()
	if cg.nodeMap == nil {
		t.Error("nodeMap should not be nil after NewCallGraph()")
	}
	if cg.edgeMap == nil {
		t.Error("edgeMap should not be nil after NewCallGraph()")
	}

	tcg := NewThreadCallGraph(1, "main")
	if tcg.nodeMap == nil {
		t.Error("nodeMap should not be nil after NewThreadCallGraph()")
	}

	// Test cleanup clears internal maps
	cg.AddNode("func1", "", 100, 200)
	cg.CalculatePercentages()
	cg.Cleanup(0, 0)
	if cg.nodeMap != nil {
		t.Error("nodeMap should be nil after Cleanup()")
	}
	if cg.edgeMap != nil {
		t.Error("edgeMap should be nil after Cleanup()")
	}
}

func TestThreadCallGraph_InternalAccess(t *testing.T) {
	tcg := NewThreadCallGraph(1, "main")
	tcg.TotalSamples = 1000

	tcg.AddNode("func1", "", 200, 500)
	tcg.AddEdge("func1", "", "func2", "", 200)

	tcg.CalculatePercentages()

	// Direct access to internal nodeMap
	node := tcg.nodeMap["func1"]
	if node == nil {
		t.Fatal("func1 should exist in nodeMap")
	}

	if delta := node.SelfPct - 20.0; delta > 0.01 || delta < -0.01 {
		t.Errorf("SelfPct = %f, want ~20.0", node.SelfPct)
	}
	if delta := node.TotalPct - 50.0; delta > 0.01 || delta < -0.01 {
		t.Errorf("TotalPct = %f, want ~50.0", node.TotalPct)
	}
}
