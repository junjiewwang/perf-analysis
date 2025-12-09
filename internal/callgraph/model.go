// Package callgraph provides utilities for generating call graph data.
package callgraph

// Node represents a node in the call graph.
type Node struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Module    string  `json:"module,omitempty"`
	Label     string  `json:"label,omitempty"`
	SelfPct   float64 `json:"selfPct"`
	TotalPct  float64 `json:"totalPct"`
	SelfTime  int64   `json:"selfTime"`
	TotalTime int64   `json:"totalTime"`
}

// Edge represents an edge (call relationship) in the call graph.
type Edge struct {
	ID     string  `json:"id"`
	Source string  `json:"source"`
	Target string  `json:"target"`
	Weight float64 `json:"weight"`
	Count  int64   `json:"count"`
}

// CallGraph represents the complete call graph structure.
type CallGraph struct {
	Name         string  `json:"name,omitempty"`
	TotalSamples int64   `json:"totalSamples"`
	Nodes        []*Node `json:"nodes"`
	Edges        []*Edge `json:"edges"`

	// Internal maps for building
	nodeMap   map[string]*Node `json:"-"`
	edgeMap   map[string]*Edge `json:"-"`
	nodeIndex map[string]int   `json:"-"`
}

// NewCallGraph creates a new call graph.
func NewCallGraph() *CallGraph {
	return &CallGraph{
		Nodes:     make([]*Node, 0),
		Edges:     make([]*Edge, 0),
		nodeMap:   make(map[string]*Node),
		edgeMap:   make(map[string]*Edge),
		nodeIndex: make(map[string]int),
	}
}

// AddNode adds or updates a node in the call graph.
func (cg *CallGraph) AddNode(name, module string, selfTime, totalTime int64) *Node {
	nodeID := makeNodeID(name, module)

	if node, exists := cg.nodeMap[nodeID]; exists {
		node.SelfTime += selfTime
		node.TotalTime += totalTime
		return node
	}

	node := &Node{
		ID:        nodeID,
		Name:      name,
		Module:    module,
		Label:     name,
		SelfTime:  selfTime,
		TotalTime: totalTime,
	}

	cg.nodeMap[nodeID] = node
	cg.nodeIndex[nodeID] = len(cg.Nodes)
	cg.Nodes = append(cg.Nodes, node)

	return node
}

// AddEdge adds or updates an edge in the call graph.
func (cg *CallGraph) AddEdge(sourceName, sourceModule, targetName, targetModule string, count int64) *Edge {
	sourceID := makeNodeID(sourceName, sourceModule)
	targetID := makeNodeID(targetName, targetModule)
	edgeID := sourceID + "->" + targetID

	if edge, exists := cg.edgeMap[edgeID]; exists {
		edge.Count += count
		return edge
	}

	edge := &Edge{
		ID:     edgeID,
		Source: sourceID,
		Target: targetID,
		Count:  count,
	}

	cg.edgeMap[edgeID] = edge
	cg.Edges = append(cg.Edges, edge)

	return edge
}

// GetNode returns a node by name and module.
func (cg *CallGraph) GetNode(name, module string) *Node {
	nodeID := makeNodeID(name, module)
	return cg.nodeMap[nodeID]
}

// GetEdge returns an edge by source and target.
func (cg *CallGraph) GetEdge(sourceName, sourceModule, targetName, targetModule string) *Edge {
	sourceID := makeNodeID(sourceName, sourceModule)
	targetID := makeNodeID(targetName, targetModule)
	edgeID := sourceID + "->" + targetID
	return cg.edgeMap[edgeID]
}

// CalculatePercentages calculates percentage values for all nodes and edges.
func (cg *CallGraph) CalculatePercentages() {
	if cg.TotalSamples == 0 {
		return
	}

	total := float64(cg.TotalSamples)

	for _, node := range cg.Nodes {
		node.SelfPct = float64(node.SelfTime) / total * 100
		node.TotalPct = float64(node.TotalTime) / total * 100
	}

	for _, edge := range cg.Edges {
		edge.Weight = float64(edge.Count) / total * 100
	}
}

// Cleanup removes internal maps and filters nodes/edges below threshold.
func (cg *CallGraph) Cleanup(minNodePct, minEdgePct float64) {
	// Clear internal maps
	cg.nodeMap = nil
	cg.edgeMap = nil
	cg.nodeIndex = nil

	if minNodePct <= 0 && minEdgePct <= 0 {
		return
	}

	// Filter nodes
	if minNodePct > 0 {
		filteredNodes := make([]*Node, 0, len(cg.Nodes))
		keepNodes := make(map[string]bool)
		for _, node := range cg.Nodes {
			if node.TotalPct >= minNodePct {
				filteredNodes = append(filteredNodes, node)
				keepNodes[node.ID] = true
			}
		}
		cg.Nodes = filteredNodes

		// Filter edges that reference removed nodes
		filteredEdges := make([]*Edge, 0, len(cg.Edges))
		for _, edge := range cg.Edges {
			if keepNodes[edge.Source] && keepNodes[edge.Target] {
				if minEdgePct <= 0 || edge.Weight >= minEdgePct {
					filteredEdges = append(filteredEdges, edge)
				}
			}
		}
		cg.Edges = filteredEdges
	} else if minEdgePct > 0 {
		filteredEdges := make([]*Edge, 0, len(cg.Edges))
		for _, edge := range cg.Edges {
			if edge.Weight >= minEdgePct {
				filteredEdges = append(filteredEdges, edge)
			}
		}
		cg.Edges = filteredEdges
	}
}

// makeNodeID creates a unique ID for a node.
func makeNodeID(name, module string) string {
	if module == "" {
		return name
	}
	return name + "(" + module + ")"
}

// Stats returns statistics about the call graph.
type Stats struct {
	NodeCount   int
	EdgeCount   int
	MaxSelfPct  float64
	MaxTotalPct float64
}

// GetStats returns statistics about the call graph.
func (cg *CallGraph) GetStats() *Stats {
	stats := &Stats{
		NodeCount: len(cg.Nodes),
		EdgeCount: len(cg.Edges),
	}

	for _, node := range cg.Nodes {
		if node.SelfPct > stats.MaxSelfPct {
			stats.MaxSelfPct = node.SelfPct
		}
		if node.TotalPct > stats.MaxTotalPct {
			stats.MaxTotalPct = node.TotalPct
		}
	}

	return stats
}
