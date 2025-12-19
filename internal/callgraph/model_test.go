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

func TestCallGraph_GetCallers(t *testing.T) {
	cg := NewCallGraph()
	cg.TotalSamples = 1000

	// Add nodes
	cg.AddNode("caller1", "", 0, 500)
	cg.AddNode("caller2", "", 0, 300)
	cg.AddNode("target", "", 200, 800)

	// Add edges (caller -> target)
	cg.AddEdge("caller1", "", "target", "", 500)
	cg.AddEdge("caller2", "", "target", "", 300)

	cg.CalculatePercentages()

	callers := cg.GetCallers("target")
	require.Len(t, callers, 2)

	// Should be sorted by call count descending
	assert.Equal(t, "caller1", callers[0].Name)
	assert.Equal(t, int64(500), callers[0].CallCount)
	assert.InDelta(t, 62.5, callers[0].Percentage, 0.1) // 500/(500+300) * 100

	assert.Equal(t, "caller2", callers[1].Name)
	assert.Equal(t, int64(300), callers[1].CallCount)
}

func TestCallGraph_GetCallees(t *testing.T) {
	cg := NewCallGraph()
	cg.TotalSamples = 1000

	// Add nodes
	cg.AddNode("source", "", 0, 800)
	cg.AddNode("callee1", "", 100, 500)
	cg.AddNode("callee2", "", 50, 300)

	// Add edges (source -> callees)
	cg.AddEdge("source", "", "callee1", "", 500)
	cg.AddEdge("source", "", "callee2", "", 300)

	cg.CalculatePercentages()

	callees := cg.GetCallees("source")
	require.Len(t, callees, 2)

	// Should be sorted by call count descending
	assert.Equal(t, "callee1", callees[0].Name)
	assert.Equal(t, int64(500), callees[0].CallCount)

	assert.Equal(t, "callee2", callees[1].Name)
	assert.Equal(t, int64(300), callees[1].CallCount)
}

func TestCallGraph_GetTopFunctionsBySelf(t *testing.T) {
	cg := NewCallGraph()
	cg.TotalSamples = 1000

	cg.AddNode("hot1", "", 300, 500)
	cg.AddNode("hot2", "", 200, 400)
	cg.AddNode("cold", "", 10, 100)

	cg.CalculatePercentages()

	// Get top 2
	top := cg.GetTopFunctionsBySelf(2)
	require.Len(t, top, 2)

	assert.Equal(t, "hot1", top[0].Name)
	assert.Equal(t, int64(300), top[0].SelfTime)

	assert.Equal(t, "hot2", top[1].Name)
	assert.Equal(t, int64(200), top[1].SelfTime)
}

func TestCallGraph_GetTopFunctionsByTotal(t *testing.T) {
	cg := NewCallGraph()
	cg.TotalSamples = 1000

	cg.AddNode("func1", "", 100, 800)
	cg.AddNode("func2", "", 200, 600)
	cg.AddNode("func3", "", 50, 200)

	cg.CalculatePercentages()

	// Get top 2
	top := cg.GetTopFunctionsByTotal(2)
	require.Len(t, top, 2)

	assert.Equal(t, "func1", top[0].Name)
	assert.Equal(t, int64(800), top[0].TotalTime)

	assert.Equal(t, "func2", top[1].Name)
	assert.Equal(t, int64(600), top[1].TotalTime)
}

func TestNewThreadCallGraph(t *testing.T) {
	tcg := NewThreadCallGraph(123, "worker-thread")

	assert.Equal(t, 123, tcg.TID)
	assert.Equal(t, "worker-thread", tcg.ThreadName)
	assert.NotNil(t, tcg.Nodes)
	assert.NotNil(t, tcg.Edges)
	assert.NotNil(t, tcg.nodeMap)
}

func TestThreadCallGraph_AddNode(t *testing.T) {
	tcg := NewThreadCallGraph(1, "main")

	node := tcg.AddNode("func1", "mod", 100, 200)

	assert.Len(t, tcg.Nodes, 1)
	assert.Equal(t, "func1", node.Name)
	assert.Equal(t, "mod", node.Module)
}

func TestThreadCallGraph_CalculatePercentages(t *testing.T) {
	tcg := NewThreadCallGraph(1, "main")
	tcg.TotalSamples = 1000

	tcg.AddNode("func1", "", 200, 500)
	tcg.AddEdge("func1", "", "func2", "", 200)

	tcg.CalculatePercentages()

	node := tcg.nodeMap["func1"]
	assert.InDelta(t, 20.0, node.SelfPct, 0.01)
	assert.InDelta(t, 50.0, node.TotalPct, 0.01)
}
