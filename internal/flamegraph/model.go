// Package flamegraph provides utilities for generating flame graph data.
package flamegraph

// Node represents a node in the flame graph tree.
type Node struct {
	Process  string  `json:"process"`
	TID      int     `json:"tid"`
	Module   string  `json:"module"`
	Func     string  `json:"func"`
	Value    int64   `json:"value"`
	Children []*Node `json:"children,omitempty"`

	// Internal use only, not serialized
	childrenMap map[string]int `json:"-"`
}

// NewNode creates a new flame graph node.
func NewNode(process string, tid int, function, module string, value int64) *Node {
	return &Node{
		Process:     process,
		TID:         tid,
		Module:      module,
		Func:        function,
		Value:       value,
		Children:    make([]*Node, 0),
		childrenMap: make(map[string]int),
	}
}

// AddChild adds a child node and returns its index.
func (n *Node) AddChild(child *Node) int {
	key := n.childKey(child)
	if idx, exists := n.childrenMap[key]; exists {
		return idx
	}
	idx := len(n.Children)
	n.childrenMap[key] = idx
	n.Children = append(n.Children, child)
	return idx
}

// GetChild returns a child node by key, or nil if not found.
func (n *Node) GetChild(process string, tid int, function, module string) *Node {
	key := makeChildKey(process, tid, function, module)
	if idx, exists := n.childrenMap[key]; exists {
		return n.Children[idx]
	}
	return nil
}

// HasChild checks if a child with the given key exists.
func (n *Node) HasChild(process string, tid int, function, module string) bool {
	key := makeChildKey(process, tid, function, module)
	_, exists := n.childrenMap[key]
	return exists
}

// childKey generates a unique key for a child node.
func (n *Node) childKey(child *Node) string {
	return makeChildKey(child.Process, child.TID, child.Func, child.Module)
}

// makeChildKey creates a unique key for node identification.
// Uses record separator (\x1E) to avoid collision with visible characters.
func makeChildKey(process string, tid int, function, module string) string {
	// Use format: process\x1Etid\x1Emodule\x1Efunc
	return process + "\x1E" + itoa(tid) + "\x1E" + module + "\x1E" + function
}

// itoa converts int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + uitoa(uint(-i))
	}
	return uitoa(uint(i))
}

func uitoa(u uint) string {
	var buf [20]byte
	i := len(buf)
	for u >= 10 {
		i--
		q := u / 10
		buf[i] = byte('0' + u - q*10)
		u = q
	}
	i--
	buf[i] = byte('0' + u)
	return string(buf[i:])
}

// FlameGraph represents the complete flame graph structure.
type FlameGraph struct {
	Root         *Node `json:"root"`
	TotalSamples int64 `json:"totalSamples"`
	MaxDepth     int   `json:"maxDepth,omitempty"`
}

// NewFlameGraph creates a new flame graph with a root node.
func NewFlameGraph() *FlameGraph {
	return &FlameGraph{
		Root: NewNode("", -1, "root", "", 0),
	}
}

// Cleanup removes internal maps and filters nodes below threshold.
// minPercent is the minimum percentage (0-100) for a node to be kept.
func (fg *FlameGraph) Cleanup(minPercent float64) {
	if fg.Root == nil {
		return
	}

	threshold := int64(float64(fg.TotalSamples) * minPercent / 100.0)
	fg.cleanupNode(fg.Root, threshold)
}

// cleanupNode recursively cleans up a node and its children.
func (fg *FlameGraph) cleanupNode(node *Node, threshold int64) {
	// Clear internal map
	node.childrenMap = nil

	if len(node.Children) == 0 {
		node.Children = nil
		return
	}

	// Filter children below threshold
	filtered := make([]*Node, 0, len(node.Children))
	for _, child := range node.Children {
		if child.Value >= threshold {
			fg.cleanupNode(child, threshold)
			filtered = append(filtered, child)
		}
	}

	if len(filtered) == 0 {
		node.Children = nil
	} else {
		node.Children = filtered
	}
}

// CalculateMaxDepth calculates the maximum depth of the flame graph.
func (fg *FlameGraph) CalculateMaxDepth() int {
	if fg.Root == nil {
		return 0
	}
	fg.MaxDepth = fg.calculateDepth(fg.Root, 0)
	return fg.MaxDepth
}

func (fg *FlameGraph) calculateDepth(node *Node, currentDepth int) int {
	if node.Children == nil || len(node.Children) == 0 {
		return currentDepth
	}

	maxChildDepth := currentDepth
	for _, child := range node.Children {
		childDepth := fg.calculateDepth(child, currentDepth+1)
		if childDepth > maxChildDepth {
			maxChildDepth = childDepth
		}
	}
	return maxChildDepth
}
