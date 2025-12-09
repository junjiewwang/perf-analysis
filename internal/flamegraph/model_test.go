package flamegraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNode(t *testing.T) {
	node := NewNode("process", 123, "func", "module", 100)

	assert.Equal(t, "process", node.Process)
	assert.Equal(t, 123, node.TID)
	assert.Equal(t, "func", node.Func)
	assert.Equal(t, "module", node.Module)
	assert.Equal(t, int64(100), node.Value)
	assert.NotNil(t, node.Children)
	assert.NotNil(t, node.childrenMap)
}

func TestNode_AddChild(t *testing.T) {
	parent := NewNode("", -1, "root", "", 0)
	child1 := NewNode("proc", 1, "func1", "mod1", 10)
	child2 := NewNode("proc", 1, "func2", "mod2", 20)
	child1Dup := NewNode("proc", 1, "func1", "mod1", 5) // Same as child1

	idx1 := parent.AddChild(child1)
	idx2 := parent.AddChild(child2)
	idx1Dup := parent.AddChild(child1Dup) // Should return same index

	assert.Equal(t, 0, idx1)
	assert.Equal(t, 1, idx2)
	assert.Equal(t, 0, idx1Dup) // Duplicate returns same index
	assert.Len(t, parent.Children, 2)
}

func TestNode_GetChild(t *testing.T) {
	parent := NewNode("", -1, "root", "", 0)
	child := NewNode("proc", 1, "func1", "mod1", 10)
	parent.AddChild(child)

	// Found
	found := parent.GetChild("proc", 1, "func1", "mod1")
	require.NotNil(t, found)
	assert.Equal(t, "func1", found.Func)

	// Not found
	notFound := parent.GetChild("proc", 1, "func2", "mod2")
	assert.Nil(t, notFound)
}

func TestNode_HasChild(t *testing.T) {
	parent := NewNode("", -1, "root", "", 0)
	child := NewNode("proc", 1, "func1", "mod1", 10)
	parent.AddChild(child)

	assert.True(t, parent.HasChild("proc", 1, "func1", "mod1"))
	assert.False(t, parent.HasChild("proc", 1, "func2", "mod2"))
}

func TestNewFlameGraph(t *testing.T) {
	fg := NewFlameGraph()

	require.NotNil(t, fg.Root)
	assert.Equal(t, "root", fg.Root.Func)
	assert.Equal(t, int64(0), fg.Root.Value)
}

func TestFlameGraph_Cleanup(t *testing.T) {
	fg := NewFlameGraph()
	fg.TotalSamples = 1000

	// Add children
	child1 := NewNode("proc", 1, "hot_func", "", 500) // 50%
	child2 := NewNode("proc", 1, "cold_func", "", 5)  // 0.5%
	child3 := NewNode("proc", 1, "tiny_func", "", 1)  // 0.1%

	fg.Root.AddChild(child1)
	fg.Root.AddChild(child2)
	fg.Root.AddChild(child3)
	fg.Root.Value = 1000

	// Cleanup with 1% threshold
	fg.Cleanup(1.0)

	// Only hot_func should remain
	assert.Len(t, fg.Root.Children, 1)
	assert.Equal(t, "hot_func", fg.Root.Children[0].Func)

	// Internal map should be cleared
	assert.Nil(t, fg.Root.childrenMap)
}

func TestFlameGraph_CalculateMaxDepth(t *testing.T) {
	fg := NewFlameGraph()

	// Build: root -> child1 -> grandchild -> great_grandchild
	child1 := NewNode("proc", 1, "func1", "", 100)
	grandchild := NewNode("proc", 1, "func2", "", 100)
	greatGrandchild := NewNode("proc", 1, "func3", "", 100)

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

func TestMakeChildKey(t *testing.T) {
	key := makeChildKey("process", 123, "function", "module")
	// Should contain separator character
	assert.Contains(t, key, "\x1E")
	assert.Contains(t, key, "process")
	assert.Contains(t, key, "123")
	assert.Contains(t, key, "function")
	assert.Contains(t, key, "module")
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
