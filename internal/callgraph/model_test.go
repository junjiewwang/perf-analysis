package callgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCallGraph(t *testing.T) {
	cg := NewCallGraph()

	assert.NotNil(t, cg.Nodes)
	assert.NotNil(t, cg.Edges)
	assert.NotNil(t, cg.nodeMap)
	assert.NotNil(t, cg.edgeMap)
	assert.Empty(t, cg.Nodes)
	assert.Empty(t, cg.Edges)
}

func TestCallGraph_AddNode(t *testing.T) {
	cg := NewCallGraph()

	node1 := cg.AddNode("func1", "module1", 100, 200)
	node2 := cg.AddNode("func2", "", 50, 100)

	assert.Len(t, cg.Nodes, 2)

	assert.Equal(t, "func1", node1.Name)
	assert.Equal(t, "module1", node1.Module)
	assert.Equal(t, int64(100), node1.SelfTime)
	assert.Equal(t, int64(200), node1.TotalTime)

	assert.Equal(t, "func2", node2.Name)
	assert.Equal(t, "", node2.Module)
}

func TestCallGraph_AddNode_Duplicate(t *testing.T) {
	cg := NewCallGraph()

	node1 := cg.AddNode("func1", "mod", 100, 200)
	node2 := cg.AddNode("func1", "mod", 50, 100) // Duplicate

	assert.Len(t, cg.Nodes, 1)
	assert.Same(t, node1, node2)

	// Values should be accumulated
	assert.Equal(t, int64(150), node1.SelfTime)
	assert.Equal(t, int64(300), node1.TotalTime)
}

func TestCallGraph_AddEdge(t *testing.T) {
	cg := NewCallGraph()

	edge := cg.AddEdge("func1", "mod1", "func2", "mod2", 100)

	assert.Len(t, cg.Edges, 1)
	assert.Equal(t, int64(100), edge.Count)
	assert.Contains(t, edge.ID, "->")
}

func TestCallGraph_AddEdge_Duplicate(t *testing.T) {
	cg := NewCallGraph()

	edge1 := cg.AddEdge("func1", "", "func2", "", 100)
	edge2 := cg.AddEdge("func1", "", "func2", "", 50) // Duplicate

	assert.Len(t, cg.Edges, 1)
	assert.Same(t, edge1, edge2)

	// Count should be accumulated
	assert.Equal(t, int64(150), edge1.Count)
}

func TestCallGraph_GetNode(t *testing.T) {
	cg := NewCallGraph()
	cg.AddNode("func1", "mod", 100, 200)

	// Found
	node := cg.GetNode("func1", "mod")
	require.NotNil(t, node)
	assert.Equal(t, "func1", node.Name)

	// Not found
	notFound := cg.GetNode("func2", "mod")
	assert.Nil(t, notFound)
}

func TestCallGraph_GetEdge(t *testing.T) {
	cg := NewCallGraph()
	cg.AddEdge("func1", "", "func2", "", 100)

	// Found
	edge := cg.GetEdge("func1", "", "func2", "")
	require.NotNil(t, edge)
	assert.Equal(t, int64(100), edge.Count)

	// Not found
	notFound := cg.GetEdge("func1", "", "func3", "")
	assert.Nil(t, notFound)
}

func TestCallGraph_CalculatePercentages(t *testing.T) {
	cg := NewCallGraph()
	cg.TotalSamples = 1000

	cg.AddNode("func1", "", 200, 500)         // 20% self, 50% total
	cg.AddNode("func2", "", 100, 300)         // 10% self, 30% total
	cg.AddEdge("func1", "", "func2", "", 200) // 20% weight

	cg.CalculatePercentages()

	node1 := cg.GetNode("func1", "")
	assert.InDelta(t, 20.0, node1.SelfPct, 0.01)
	assert.InDelta(t, 50.0, node1.TotalPct, 0.01)

	node2 := cg.GetNode("func2", "")
	assert.InDelta(t, 10.0, node2.SelfPct, 0.01)
	assert.InDelta(t, 30.0, node2.TotalPct, 0.01)

	edge := cg.GetEdge("func1", "", "func2", "")
	assert.InDelta(t, 20.0, edge.Weight, 0.01)
}

func TestCallGraph_Cleanup(t *testing.T) {
	cg := NewCallGraph()
	cg.TotalSamples = 1000

	cg.AddNode("hot_func", "", 500, 800) // 80% total
	cg.AddNode("cold_func", "", 10, 30)  // 3% total
	cg.AddNode("tiny_func", "", 1, 2)    // 0.2% total

	cg.AddEdge("hot_func", "", "cold_func", "", 100)
	cg.AddEdge("hot_func", "", "tiny_func", "", 5)

	cg.CalculatePercentages()
	cg.Cleanup(5.0, 1.0) // Min 5% nodes, 1% edges

	// Only hot_func should remain (80% > 5%)
	// cold_func is 3% < 5%, should be removed
	assert.Len(t, cg.Nodes, 1)
	assert.Equal(t, "hot_func", cg.Nodes[0].Name)

	// Edges should be removed (reference removed nodes)
	assert.Empty(t, cg.Edges)

	// Internal maps should be cleared
	assert.Nil(t, cg.nodeMap)
	assert.Nil(t, cg.edgeMap)
}

func TestCallGraph_GetStats(t *testing.T) {
	cg := NewCallGraph()
	cg.TotalSamples = 1000

	cg.AddNode("func1", "", 200, 500)
	cg.AddNode("func2", "", 100, 300)
	cg.AddEdge("func1", "", "func2", "", 200)

	cg.CalculatePercentages()
	stats := cg.GetStats()

	assert.Equal(t, 2, stats.NodeCount)
	assert.Equal(t, 1, stats.EdgeCount)
	assert.InDelta(t, 20.0, stats.MaxSelfPct, 0.01)
	assert.InDelta(t, 50.0, stats.MaxTotalPct, 0.01)
}

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
			assert.Equal(t, tt.want, got)
		})
	}
}
