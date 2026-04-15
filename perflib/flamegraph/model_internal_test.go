package flamegraph

import (
	"testing"
)

func TestInternalChildrenMap(t *testing.T) {
	node := NewNode("func", 100)
	if node.childrenMap == nil {
		t.Error("childrenMap should not be nil after NewNode()")
	}
}

func TestMakeChildKey(t *testing.T) {
	// Simple key (no metadata)
	key1 := makeChildKey("func", "", "", 0)
	if key1 != "func" {
		t.Errorf("makeChildKey(func, '', '', 0) = %q, want %q", key1, "func")
	}

	// Key with metadata
	key2 := makeChildKey("func", "mod", "proc", 123)
	if len(key2) == 0 {
		t.Error("makeChildKey with metadata should return non-empty string")
	}
	// Should contain the record separator and all parts
	contains := func(s, substr string) bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}
	if !contains(key2, "func") {
		t.Errorf("key should contain 'func', got %q", key2)
	}
	if !contains(key2, "mod") {
		t.Errorf("key should contain 'mod', got %q", key2)
	}
	if !contains(key2, "proc") {
		t.Errorf("key should contain 'proc', got %q", key2)
	}
	if !contains(key2, "123") {
		t.Errorf("key should contain '123', got %q", key2)
	}
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
			if got != tt.want {
				t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCleanupClearsChildrenMap(t *testing.T) {
	fg := NewFlameGraph()
	fg.TotalSamples = 1000

	child1 := NewNode("hot_func", 500)
	child2 := NewNode("cold_func", 5)
	fg.Root.AddChild(child1)
	fg.Root.AddChild(child2)
	fg.Root.Value = 1000

	fg.Cleanup(1.0)
	if fg.Root.childrenMap != nil {
		t.Error("childrenMap should be nil after Cleanup()")
	}
}
