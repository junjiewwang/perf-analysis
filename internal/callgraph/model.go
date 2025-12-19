// Package callgraph provides utilities for generating call graph data.
package callgraph

import "sort"

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

	// Enhanced analysis fields
	CallCount   int64 `json:"callCount,omitempty"`   // Number of times this function was called
	CallerCount int   `json:"callerCount,omitempty"` // Number of unique callers
	CalleeCount int   `json:"calleeCount,omitempty"` // Number of unique callees
	MaxDepth    int   `json:"maxDepth,omitempty"`    // Maximum call stack depth where this function appears
	IsRecursive bool  `json:"isRecursive,omitempty"` // Whether this function has recursive calls
}

// Edge represents an edge (call relationship) in the call graph.
type Edge struct {
	ID     string  `json:"id"`
	Source string  `json:"source"`
	Target string  `json:"target"`
	Weight float64 `json:"weight"`
	Count  int64   `json:"count"`
}

// CallerInfo represents information about a caller of a function.
type CallerInfo struct {
	NodeID     string  `json:"nodeId"`
	Name       string  `json:"name"`
	Module     string  `json:"module,omitempty"`
	CallCount  int64   `json:"callCount"`
	Percentage float64 `json:"percentage"` // Percentage of total calls to this function
}

// CalleeInfo represents information about a callee of a function.
type CalleeInfo struct {
	NodeID     string  `json:"nodeId"`
	Name       string  `json:"name"`
	Module     string  `json:"module,omitempty"`
	CallCount  int64   `json:"callCount"`
	Percentage float64 `json:"percentage"` // Percentage of calls from this function
}

// FunctionAnalysis provides detailed analysis for a single function.
type FunctionAnalysis struct {
	NodeID      string        `json:"nodeId"`
	Name        string        `json:"name"`
	Module      string        `json:"module,omitempty"`
	SelfTime    int64         `json:"selfTime"`
	TotalTime   int64         `json:"totalTime"`
	SelfPct     float64       `json:"selfPct"`
	TotalPct    float64       `json:"totalPct"`
	CallCount   int64         `json:"callCount"`
	Callers     []*CallerInfo `json:"callers,omitempty"`
	Callees     []*CalleeInfo `json:"callees,omitempty"`
	IsRecursive bool          `json:"isRecursive,omitempty"`
	ThreadCount int           `json:"threadCount,omitempty"` // Number of threads where this function appears
	ThreadTIDs  []int         `json:"threadTids,omitempty"`  // TIDs of threads where this function appears
}

// HotPath represents a critical/hot execution path.
type HotPath struct {
	Path       []string `json:"path"`       // Function names in the path
	Samples    int64    `json:"samples"`    // Total samples for this path
	Percentage float64  `json:"percentage"` // Percentage of total samples
	Depth      int      `json:"depth"`      // Depth of the path
}

// ThreadCallGraph represents a call graph for a single thread.
type ThreadCallGraph struct {
	TID          int     `json:"tid"`
	ThreadName   string  `json:"threadName"`
	ThreadGroup  string  `json:"threadGroup,omitempty"`
	TotalSamples int64   `json:"totalSamples"`
	Percentage   float64 `json:"percentage"` // Percentage of global samples
	Nodes        []*Node `json:"nodes"`
	Edges        []*Edge `json:"edges"`

	// Internal maps for building
	nodeMap   map[string]*Node `json:"-"`
	edgeMap   map[string]*Edge `json:"-"`
	nodeIndex map[string]int   `json:"-"`
}

// ModuleAnalysis provides aggregated analysis by module/package.
type ModuleAnalysis struct {
	Module       string   `json:"module"`
	FunctionCount int     `json:"functionCount"`
	TotalSamples int64    `json:"totalSamples"`
	SelfSamples  int64    `json:"selfSamples"`
	TotalPct     float64  `json:"totalPct"`
	SelfPct      float64  `json:"selfPct"`
	TopFunctions []string `json:"topFunctions,omitempty"` // Top functions in this module
}

// ThreadGroupAnalysis provides aggregated analysis by thread group.
type ThreadGroupAnalysis struct {
	GroupName    string  `json:"groupName"`
	ThreadCount  int     `json:"threadCount"`
	TotalSamples int64   `json:"totalSamples"`
	Percentage   float64 `json:"percentage"`
	TopFunctions []string `json:"topFunctions,omitempty"`
}

// CallGraphAnalysis holds the complete enhanced analysis data.
type CallGraphAnalysis struct {
	// Summary statistics
	TotalSamples       int64 `json:"totalSamples"`
	TotalThreads       int   `json:"totalThreads"`
	TotalFunctions     int   `json:"totalFunctions"`
	TotalEdges         int   `json:"totalEdges"`
	MaxCallDepth       int   `json:"maxCallDepth"`
	RecursiveFunctions int   `json:"recursiveFunctions"`

	// Hot paths (critical execution paths)
	HotPaths []*HotPath `json:"hotPaths,omitempty"`

	// Top functions by self time
	TopFunctionsBySelf []*FunctionAnalysis `json:"topFunctionsBySelf,omitempty"`

	// Top functions by total time
	TopFunctionsByTotal []*FunctionAnalysis `json:"topFunctionsByTotal,omitempty"`

	// Module-level aggregation
	ModuleAnalysis []*ModuleAnalysis `json:"moduleAnalysis,omitempty"`

	// Thread group aggregation
	ThreadGroupAnalysis []*ThreadGroupAnalysis `json:"threadGroupAnalysis,omitempty"`

	// Per-thread call graphs
	ThreadCallGraphs []*ThreadCallGraph `json:"threadCallGraphs,omitempty"`
}

// CallGraph represents the complete call graph structure.
type CallGraph struct {
	Name         string  `json:"name,omitempty"`
	TotalSamples int64   `json:"totalSamples"`
	Nodes        []*Node `json:"nodes"`
	Edges        []*Edge `json:"edges"`

	// Enhanced analysis data (optional)
	Analysis *CallGraphAnalysis `json:"analysis,omitempty"`

	// Internal maps for building
	nodeMap   map[string]*Node `json:"-"`
	edgeMap   map[string]*Edge `json:"-"`
	nodeIndex map[string]int   `json:"-"`

	// Internal: caller/callee tracking
	callers map[string]map[string]int64 `json:"-"` // nodeID -> callerID -> count
	callees map[string]map[string]int64 `json:"-"` // nodeID -> calleeID -> count
}

// NewCallGraph creates a new call graph.
func NewCallGraph() *CallGraph {
	return &CallGraph{
		Nodes:     make([]*Node, 0),
		Edges:     make([]*Edge, 0),
		nodeMap:   make(map[string]*Node),
		edgeMap:   make(map[string]*Edge),
		nodeIndex: make(map[string]int),
		callers:   make(map[string]map[string]int64),
		callees:   make(map[string]map[string]int64),
	}
}

// NewThreadCallGraph creates a new thread-specific call graph.
func NewThreadCallGraph(tid int, threadName string) *ThreadCallGraph {
	return &ThreadCallGraph{
		TID:        tid,
		ThreadName: threadName,
		Nodes:      make([]*Node, 0),
		Edges:      make([]*Edge, 0),
		nodeMap:    make(map[string]*Node),
		edgeMap:    make(map[string]*Edge),
		nodeIndex:  make(map[string]int),
	}
}

// AddNode adds or updates a node in the call graph.
func (cg *CallGraph) AddNode(name, module string, selfTime, totalTime int64) *Node {
	nodeID := makeNodeID(name, module)

	if node, exists := cg.nodeMap[nodeID]; exists {
		node.SelfTime += selfTime
		node.TotalTime += totalTime
		node.CallCount++
		return node
	}

	node := &Node{
		ID:        nodeID,
		Name:      name,
		Module:    module,
		Label:     name,
		SelfTime:  selfTime,
		TotalTime: totalTime,
		CallCount: 1,
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

	// Track callers and callees
	if cg.callers[targetID] == nil {
		cg.callers[targetID] = make(map[string]int64)
	}
	cg.callers[targetID][sourceID] += count

	if cg.callees[sourceID] == nil {
		cg.callees[sourceID] = make(map[string]int64)
	}
	cg.callees[sourceID][targetID] += count

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
		node.CallerCount = len(cg.callers[node.ID])
		node.CalleeCount = len(cg.callees[node.ID])
	}

	for _, edge := range cg.Edges {
		edge.Weight = float64(edge.Count) / total * 100
	}
}

// GetCallers returns the callers of a function.
func (cg *CallGraph) GetCallers(nodeID string) []*CallerInfo {
	callerMap := cg.callers[nodeID]
	if len(callerMap) == 0 {
		return nil
	}

	var totalCalls int64
	for _, count := range callerMap {
		totalCalls += count
	}

	callers := make([]*CallerInfo, 0, len(callerMap))
	for callerID, count := range callerMap {
		node := cg.nodeMap[callerID]
		if node == nil {
			continue
		}
		pct := float64(0)
		if totalCalls > 0 {
			pct = float64(count) / float64(totalCalls) * 100
		}
		callers = append(callers, &CallerInfo{
			NodeID:     callerID,
			Name:       node.Name,
			Module:     node.Module,
			CallCount:  count,
			Percentage: pct,
		})
	}

	// Sort by call count descending
	sort.Slice(callers, func(i, j int) bool {
		return callers[i].CallCount > callers[j].CallCount
	})

	return callers
}

// GetCallees returns the callees of a function.
func (cg *CallGraph) GetCallees(nodeID string) []*CalleeInfo {
	calleeMap := cg.callees[nodeID]
	if len(calleeMap) == 0 {
		return nil
	}

	var totalCalls int64
	for _, count := range calleeMap {
		totalCalls += count
	}

	callees := make([]*CalleeInfo, 0, len(calleeMap))
	for calleeID, count := range calleeMap {
		node := cg.nodeMap[calleeID]
		if node == nil {
			continue
		}
		pct := float64(0)
		if totalCalls > 0 {
			pct = float64(count) / float64(totalCalls) * 100
		}
		callees = append(callees, &CalleeInfo{
			NodeID:     calleeID,
			Name:       node.Name,
			Module:     node.Module,
			CallCount:  count,
			Percentage: pct,
		})
	}

	// Sort by call count descending
	sort.Slice(callees, func(i, j int) bool {
		return callees[i].CallCount > callees[j].CallCount
	})

	return callees
}

// Cleanup removes internal maps and filters nodes/edges below threshold.
func (cg *CallGraph) Cleanup(minNodePct, minEdgePct float64) {
	if minNodePct <= 0 && minEdgePct <= 0 {
		// Clear internal maps but keep data
		cg.nodeMap = nil
		cg.edgeMap = nil
		cg.nodeIndex = nil
		cg.callers = nil
		cg.callees = nil
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

	// Clear internal maps
	cg.nodeMap = nil
	cg.edgeMap = nil
	cg.nodeIndex = nil
	cg.callers = nil
	cg.callees = nil
}

// CleanupKeepMaps removes only filtered data but keeps internal maps for further analysis.
func (cg *CallGraph) CleanupKeepMaps(minNodePct, minEdgePct float64) {
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

		// Update nodeMap and nodeIndex
		newNodeMap := make(map[string]*Node)
		newNodeIndex := make(map[string]int)
		for i, node := range cg.Nodes {
			newNodeMap[node.ID] = node
			newNodeIndex[node.ID] = i
		}
		cg.nodeMap = newNodeMap
		cg.nodeIndex = newNodeIndex

		// Filter edges that reference removed nodes
		filteredEdges := make([]*Edge, 0, len(cg.Edges))
		newEdgeMap := make(map[string]*Edge)
		for _, edge := range cg.Edges {
			if keepNodes[edge.Source] && keepNodes[edge.Target] {
				if minEdgePct <= 0 || edge.Weight >= minEdgePct {
					filteredEdges = append(filteredEdges, edge)
					newEdgeMap[edge.ID] = edge
				}
			}
		}
		cg.Edges = filteredEdges
		cg.edgeMap = newEdgeMap
	} else if minEdgePct > 0 {
		filteredEdges := make([]*Edge, 0, len(cg.Edges))
		newEdgeMap := make(map[string]*Edge)
		for _, edge := range cg.Edges {
			if edge.Weight >= minEdgePct {
				filteredEdges = append(filteredEdges, edge)
				newEdgeMap[edge.ID] = edge
			}
		}
		cg.Edges = filteredEdges
		cg.edgeMap = newEdgeMap
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

// GetTopFunctionsBySelf returns top N functions sorted by self time.
func (cg *CallGraph) GetTopFunctionsBySelf(n int) []*FunctionAnalysis {
	if len(cg.Nodes) == 0 {
		return nil
	}

	// Sort nodes by self time
	sorted := make([]*Node, len(cg.Nodes))
	copy(sorted, cg.Nodes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].SelfTime > sorted[j].SelfTime
	})

	if n > len(sorted) {
		n = len(sorted)
	}

	result := make([]*FunctionAnalysis, n)
	for i := 0; i < n; i++ {
		node := sorted[i]
		result[i] = &FunctionAnalysis{
			NodeID:      node.ID,
			Name:        node.Name,
			Module:      node.Module,
			SelfTime:    node.SelfTime,
			TotalTime:   node.TotalTime,
			SelfPct:     node.SelfPct,
			TotalPct:    node.TotalPct,
			CallCount:   node.CallCount,
			Callers:     cg.GetCallers(node.ID),
			Callees:     cg.GetCallees(node.ID),
			IsRecursive: node.IsRecursive,
		}
	}

	return result
}

// GetTopFunctionsByTotal returns top N functions sorted by total time.
func (cg *CallGraph) GetTopFunctionsByTotal(n int) []*FunctionAnalysis {
	if len(cg.Nodes) == 0 {
		return nil
	}

	// Sort nodes by total time
	sorted := make([]*Node, len(cg.Nodes))
	copy(sorted, cg.Nodes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TotalTime > sorted[j].TotalTime
	})

	if n > len(sorted) {
		n = len(sorted)
	}

	result := make([]*FunctionAnalysis, n)
	for i := 0; i < n; i++ {
		node := sorted[i]
		result[i] = &FunctionAnalysis{
			NodeID:      node.ID,
			Name:        node.Name,
			Module:      node.Module,
			SelfTime:    node.SelfTime,
			TotalTime:   node.TotalTime,
			SelfPct:     node.SelfPct,
			TotalPct:    node.TotalPct,
			CallCount:   node.CallCount,
			Callers:     cg.GetCallers(node.ID),
			Callees:     cg.GetCallees(node.ID),
			IsRecursive: node.IsRecursive,
		}
	}

	return result
}

// AddNode adds or updates a node in the thread call graph.
func (tcg *ThreadCallGraph) AddNode(name, module string, selfTime, totalTime int64) *Node {
	nodeID := makeNodeID(name, module)

	if node, exists := tcg.nodeMap[nodeID]; exists {
		node.SelfTime += selfTime
		node.TotalTime += totalTime
		node.CallCount++
		return node
	}

	node := &Node{
		ID:        nodeID,
		Name:      name,
		Module:    module,
		Label:     name,
		SelfTime:  selfTime,
		TotalTime: totalTime,
		CallCount: 1,
	}

	tcg.nodeMap[nodeID] = node
	tcg.nodeIndex[nodeID] = len(tcg.Nodes)
	tcg.Nodes = append(tcg.Nodes, node)

	return node
}

// AddEdge adds or updates an edge in the thread call graph.
func (tcg *ThreadCallGraph) AddEdge(sourceName, sourceModule, targetName, targetModule string, count int64) *Edge {
	sourceID := makeNodeID(sourceName, sourceModule)
	targetID := makeNodeID(targetName, targetModule)
	edgeID := sourceID + "->" + targetID

	if edge, exists := tcg.edgeMap[edgeID]; exists {
		edge.Count += count
		return edge
	}

	edge := &Edge{
		ID:     edgeID,
		Source: sourceID,
		Target: targetID,
		Count:  count,
	}

	tcg.edgeMap[edgeID] = edge
	tcg.Edges = append(tcg.Edges, edge)

	return edge
}

// CalculatePercentages calculates percentage values for nodes and edges.
func (tcg *ThreadCallGraph) CalculatePercentages() {
	if tcg.TotalSamples == 0 {
		return
	}

	total := float64(tcg.TotalSamples)

	for _, node := range tcg.Nodes {
		node.SelfPct = float64(node.SelfTime) / total * 100
		node.TotalPct = float64(node.TotalTime) / total * 100
	}

	for _, edge := range tcg.Edges {
		edge.Weight = float64(edge.Count) / total * 100
	}
}

// Cleanup removes internal maps.
func (tcg *ThreadCallGraph) Cleanup() {
	tcg.nodeMap = nil
	tcg.edgeMap = nil
	tcg.nodeIndex = nil
}
