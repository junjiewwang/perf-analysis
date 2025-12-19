package flamegraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNode(t *testing.T) {
	node := NewNode("func", 100)

	assert.Equal(t, "func", node.Name)
	assert.Equal(t, int64(100), node.Value)
	assert.NotNil(t, node.Children)
	assert.NotNil(t, node.childrenMap)
}

func TestNewNodeWithMetadata(t *testing.T) {
	node := NewNodeWithMetadata("func", "module", "process", 123, 100)

	assert.Equal(t, "func", node.Name)
	assert.Equal(t, "module", node.Module)
	assert.Equal(t, "process", node.Process)
	assert.Equal(t, 123, node.TID)
	assert.Equal(t, int64(100), node.Value)
}

func TestNode_AddChild(t *testing.T) {
	parent := NewNode("root", 0)
	child1 := NewNode("func1", 10)
	child2 := NewNode("func2", 20)
	child1Dup := NewNode("func1", 5) // Same name as child1

	idx1 := parent.AddChild(child1)
	idx2 := parent.AddChild(child2)
	idx1Dup := parent.AddChild(child1Dup) // Should return same index

	assert.Equal(t, 0, idx1)
	assert.Equal(t, 1, idx2)
	assert.Equal(t, 0, idx1Dup) // Duplicate returns same index
	assert.Len(t, parent.Children, 2)
}

func TestNode_GetChild(t *testing.T) {
	parent := NewNode("root", 0)
	child := NewNode("func1", 10)
	parent.AddChild(child)

	// Found
	found := parent.GetChild("func1")
	require.NotNil(t, found)
	assert.Equal(t, "func1", found.Name)

	// Not found
	notFound := parent.GetChild("func2")
	assert.Nil(t, notFound)
}

func TestNode_GetChildWithMetadata(t *testing.T) {
	parent := NewNode("root", 0)
	child := NewNodeWithMetadata("func1", "mod1", "proc", 1, 10)
	parent.AddChild(child)

	// Found
	found := parent.GetChildWithMetadata("func1", "mod1", "proc", 1)
	require.NotNil(t, found)
	assert.Equal(t, "func1", found.Name)

	// Not found
	notFound := parent.GetChildWithMetadata("func1", "mod2", "proc", 1)
	assert.Nil(t, notFound)
}

func TestNode_FindOrCreateChild(t *testing.T) {
	parent := NewNode("root", 0)

	// Create new child
	child1 := parent.FindOrCreateChild("func1")
	require.NotNil(t, child1)
	assert.Equal(t, "func1", child1.Name)

	// Find existing child
	child1Again := parent.FindOrCreateChild("func1")
	assert.Equal(t, child1, child1Again) // Same pointer

	assert.Len(t, parent.Children, 1)
}

func TestNode_Clone(t *testing.T) {
	original := NewNodeWithMetadata("func", "mod", "proc", 1, 100)
	original.Self = 50
	child := NewNode("child", 30)
	original.AddChild(child)

	clone := original.Clone()

	// Verify clone is independent
	assert.Equal(t, original.Name, clone.Name)
	assert.Equal(t, original.Value, clone.Value)
	assert.Equal(t, original.Self, clone.Self)
	assert.Len(t, clone.Children, 1)

	// Modify clone shouldn't affect original
	clone.Value = 200
	assert.Equal(t, int64(100), original.Value)
}

func TestNewFlameGraph(t *testing.T) {
	fg := NewFlameGraph()

	require.NotNil(t, fg.Root)
	assert.Equal(t, "root", fg.Root.Name)
	assert.Equal(t, int64(0), fg.Root.Value)
}

func TestNewFlameGraphWithAnalysis(t *testing.T) {
	fg := NewFlameGraphWithAnalysis()

	require.NotNil(t, fg.Root)
	require.NotNil(t, fg.ThreadAnalysis)
	assert.NotNil(t, fg.ThreadAnalysis.Threads)
	assert.NotNil(t, fg.ThreadAnalysis.TopFunctions)
}

func TestFlameGraph_Cleanup(t *testing.T) {
	fg := NewFlameGraph()
	fg.TotalSamples = 1000

	// Add children
	child1 := NewNode("hot_func", 500) // 50%
	child2 := NewNode("cold_func", 5)  // 0.5%
	child3 := NewNode("tiny_func", 1)  // 0.1%

	fg.Root.AddChild(child1)
	fg.Root.AddChild(child2)
	fg.Root.AddChild(child3)
	fg.Root.Value = 1000

	// Cleanup with 1% threshold
	fg.Cleanup(1.0)

	// Only hot_func should remain
	assert.Len(t, fg.Root.Children, 1)
	assert.Equal(t, "hot_func", fg.Root.Children[0].Name)

	// Internal map should be cleared
	assert.Nil(t, fg.Root.childrenMap)
}

func TestFlameGraph_CalculateMaxDepth(t *testing.T) {
	fg := NewFlameGraph()

	// Build: root -> child1 -> grandchild -> great_grandchild
	child1 := NewNode("func1", 100)
	grandchild := NewNode("func2", 100)
	greatGrandchild := NewNode("func3", 100)

	grandchild.AddChild(greatGrandchild)
	child1.AddChild(grandchild)
	fg.Root.AddChild(child1)

	depth := fg.CalculateMaxDepth()
	assert.Equal(t, 3, depth)
	assert.Equal(t, 3, fg.MaxDepth)
}

func TestFlameGraph_EmptyGraph(t *testing.T) {
	fg := NewFlameGraph()

	fg.Cleanup(0.01)
	depth := fg.CalculateMaxDepth()

	assert.Equal(t, 0, depth)
	assert.Nil(t, fg.Root.Children)
}

func TestFlameGraph_GetThread(t *testing.T) {
	fg := NewFlameGraphWithAnalysis()
	fg.ThreadAnalysis.Threads = []*ThreadInfo{
		{TID: 1, Name: "thread-1"},
		{TID: 2, Name: "thread-2"},
	}

	thread := fg.GetThread(1)
	require.NotNil(t, thread)
	assert.Equal(t, "thread-1", thread.Name)

	notFound := fg.GetThread(999)
	assert.Nil(t, notFound)
}

func TestFlameGraph_SortThreads(t *testing.T) {
	fg := NewFlameGraphWithAnalysis()
	fg.ThreadAnalysis.Threads = []*ThreadInfo{
		{TID: 1, Name: "low", Samples: 10},
		{TID: 2, Name: "high", Samples: 100},
		{TID: 3, Name: "mid", Samples: 50},
	}

	fg.SortThreads()

	assert.Equal(t, "high", fg.ThreadAnalysis.Threads[0].Name)
	assert.Equal(t, "mid", fg.ThreadAnalysis.Threads[1].Name)
	assert.Equal(t, "low", fg.ThreadAnalysis.Threads[2].Name)
}

func TestNodeBuilder_AddStack(t *testing.T) {
	builder := NewNodeBuilder("root")

	builder.AddStack([]string{"a", "b", "c"}, 100)
	builder.AddStack([]string{"a", "b", "d"}, 50)
	builder.AddStack([]string{"a", "e"}, 30)

	root := builder.Build()

	assert.Equal(t, int64(180), root.Value)
	assert.Len(t, root.Children, 1) // Only "a"

	a := root.Children[0]
	assert.Equal(t, "a", a.Name)
	assert.Equal(t, int64(180), a.Value)
	assert.Len(t, a.Children, 2) // "b" and "e"
}

func TestNodeBuilder_EmptyStack(t *testing.T) {
	builder := NewNodeBuilder("root")

	builder.AddStack([]string{}, 100)
	builder.AddStack(nil, 50)

	root := builder.Build()
	assert.Equal(t, int64(0), root.Value)
	assert.Empty(t, root.Children)
}

func TestMergeNodes(t *testing.T) {
	node1 := NewNode("thread1", 100)
	node2 := NewNode("thread2", 50)

	merged := MergeNodes([]*Node{node1, node2})

	require.NotNil(t, merged)
	assert.Equal(t, "all", merged.Name)
	assert.Equal(t, int64(150), merged.Value)
	assert.Len(t, merged.Children, 2)
}

func TestMergeNodes_Empty(t *testing.T) {
	merged := MergeNodes([]*Node{})
	assert.Nil(t, merged)
}

func TestMergeNodes_Single(t *testing.T) {
	node := NewNode("single", 100)
	merged := MergeNodes([]*Node{node})
	assert.Equal(t, node, merged)
}

func TestMakeChildKey(t *testing.T) {
	// Simple key (no metadata)
	key1 := makeChildKey("func", "", "", 0)
	assert.Equal(t, "func", key1)

	// Key with metadata
	key2 := makeChildKey("func", "mod", "proc", 123)
	assert.Contains(t, key2, "\x1E")
	assert.Contains(t, key2, "func")
	assert.Contains(t, key2, "mod")
	assert.Contains(t, key2, "proc")
	assert.Contains(t, key2, "123")
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{123456, "123456"},
		{-123456, "-123456"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := itoa(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
