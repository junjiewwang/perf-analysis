// Package flamegraph provides unified flame graph data structures and utilities.
// This package consolidates flame graph representation for both visualization and analysis.
package flamegraph

import (
	"sort"
	"strings"
	"sync"

	"github.com/perf-analysis/pkg/profiling"
)

// Node represents a node in the flame graph tree.
// This is the unified structure used for both visualization and analysis.
type Node struct {
	// Core fields for flame graph visualization
	Name     string  `json:"name"`              // Function name (unified from Func)
	Value    int64   `json:"value"`             // Total samples including children
	Self     int64   `json:"self,omitempty"`    // Self samples (leaf value)
	Children []*Node `json:"children,omitempty"`

	// Optional metadata for detailed analysis
	Module  string `json:"module,omitempty"`  // Module/library name
	TID     int    `json:"tid,omitempty"`     // Thread ID (0 means aggregated)
	Process string `json:"process,omitempty"` // Process/thread name

	// Internal use only, not serialized
	childrenMap map[string]int `json:"-"`
}

// NewNode creates a new flame graph node.
func NewNode(name string, value int64) *Node {
	return &Node{
		Name:        name,
		Value:       value,
		Children:    make([]*Node, 0),
		childrenMap: make(map[string]int),
	}
}

// NewNodeWithMetadata creates a node with full metadata.
func NewNodeWithMetadata(name, module, process string, tid int, value int64) *Node {
	return &Node{
		Name:        name,
		Module:      module,
		Process:     process,
		TID:         tid,
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

// GetChild returns a child node by name, or nil if not found.
func (n *Node) GetChild(name string) *Node {
	if idx, exists := n.childrenMap[name]; exists {
		return n.Children[idx]
	}
	return nil
}

// GetChildWithMetadata returns a child node by full key, or nil if not found.
func (n *Node) GetChildWithMetadata(name, module, process string, tid int) *Node {
	key := makeChildKey(name, module, process, tid)
	if idx, exists := n.childrenMap[key]; exists {
		return n.Children[idx]
	}
	return nil
}

// FindOrCreateChild finds an existing child or creates a new one.
func (n *Node) FindOrCreateChild(name string) *Node {
	if child := n.GetChild(name); child != nil {
		return child
	}
	child := NewNode(name, 0)
	n.AddChild(child)
	return child
}

// childKey generates a unique key for a child node.
func (n *Node) childKey(child *Node) string {
	if child.Module != "" || child.Process != "" || child.TID != 0 {
		return makeChildKey(child.Name, child.Module, child.Process, child.TID)
	}
	return child.Name
}

// makeChildKey creates a unique key for node identification.
func makeChildKey(name, module, process string, tid int) string {
	if module == "" && process == "" && tid == 0 {
		return name
	}
	// Use record separator (\x1E) to avoid collision
	return name + "\x1E" + module + "\x1E" + process + "\x1E" + itoa(tid)
}

// Cleanup removes internal maps and optionally filters nodes below threshold.
func (n *Node) Cleanup(threshold int64) {
	n.childrenMap = nil

	if len(n.Children) == 0 {
		n.Children = nil
		return
	}

	// Filter children below threshold
	if threshold > 0 {
		filtered := make([]*Node, 0, len(n.Children))
		for _, child := range n.Children {
			if child.Value >= threshold {
				child.Cleanup(threshold)
				filtered = append(filtered, child)
			}
		}
		if len(filtered) == 0 {
			n.Children = nil
		} else {
			n.Children = filtered
		}
	} else {
		for _, child := range n.Children {
			child.Cleanup(threshold)
		}
	}
}

// Clone creates a deep copy of the node.
func (n *Node) Clone() *Node {
	if n == nil {
		return nil
	}
	clone := &Node{
		Name:    n.Name,
		Value:   n.Value,
		Self:    n.Self,
		Module:  n.Module,
		TID:     n.TID,
		Process: n.Process,
	}
	if len(n.Children) > 0 {
		clone.Children = make([]*Node, len(n.Children))
		clone.childrenMap = make(map[string]int, len(n.Children))
		for i, child := range n.Children {
			clone.Children[i] = child.Clone()
			clone.childrenMap[clone.childKey(clone.Children[i])] = i
		}
	}
	return clone
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

// FlameGraph represents the complete flame graph structure with optional analysis data.
type FlameGraph struct {
	// Core flame graph data
	Root         *Node `json:"root"`
	TotalSamples int64 `json:"total_samples"`
	MaxDepth     int   `json:"max_depth,omitempty"`

	// Thread-level analysis (optional, for detailed analysis)
	ThreadAnalysis *ThreadAnalysisData `json:"thread_analysis,omitempty"`
}

// ThreadAnalysisData holds thread-level CPU analysis data.
// This replaces the separate cpu_analysis.json file.
type ThreadAnalysisData struct {
	// Summary statistics
	TotalThreads    int   `json:"total_threads"`
	ActiveThreads   int   `json:"active_threads"`
	UniqueFunctions int   `json:"unique_functions"`
	AnalysisDurationMs int64 `json:"analysis_duration_ms,omitempty"`

	// Thread details
	Threads []*ThreadInfo `json:"threads"`

	// Global top functions
	TopFunctions []*TopFunction `json:"top_functions"`

	// Thread groups
	ThreadGroups []*ThreadGroupInfo `json:"thread_groups,omitempty"`
}

// ThreadInfo represents detailed CPU analysis for a single thread.
type ThreadInfo struct {
	TID         int     `json:"tid"`
	Name        string  `json:"name"`
	Group       string  `json:"group,omitempty"`
	Samples     int64   `json:"samples"`
	Percentage  float64 `json:"percentage"`
	IsSwapper   bool    `json:"is_swapper,omitempty"`

	// Top functions within this thread
	TopFunctions []*ThreadTopFunction `json:"top_functions,omitempty"`

	// Top call stacks within this thread
	TopCallStacks []*CallStackEntry `json:"top_call_stacks,omitempty"`

	// Thread-specific flame graph (optional, for per-thread visualization)
	FlameRoot *Node `json:"flame_root,omitempty"`
}

// ThreadTopFunction represents a hot function within a thread.
type ThreadTopFunction struct {
	Name       string  `json:"name"`
	Module     string  `json:"module,omitempty"`
	Samples    int64   `json:"samples"`
	Percentage float64 `json:"percentage"`
}

// CallStackEntry represents a unique call stack.
type CallStackEntry struct {
	Stack      []string `json:"stack"`
	Samples    int64    `json:"samples"`
	Percentage float64  `json:"percentage"`
}

// TopFunction represents a globally hot function across all threads.
type TopFunction struct {
	Name        string  `json:"name"`
	Module      string  `json:"module,omitempty"`
	Samples     int64   `json:"samples"`
	Percentage  float64 `json:"percentage"`
	ThreadCount int     `json:"thread_count"`

	// Per-thread breakdown
	Threads []*ThreadFunctionInfo `json:"threads,omitempty"`

	// Top call stacks for this function
	TopCallStacks []string `json:"top_call_stacks,omitempty"`
}

// ThreadFunctionInfo shows function statistics per thread.
type ThreadFunctionInfo struct {
	TID        int     `json:"tid"`
	ThreadName string  `json:"thread_name"`
	Samples    int64   `json:"samples"`
	Percentage float64 `json:"percentage"`
}

// ThreadGroupInfo provides aggregated statistics for a thread group.
type ThreadGroupInfo struct {
	Name         string  `json:"name"`
	ThreadCount  int     `json:"thread_count"`
	TotalSamples int64   `json:"total_samples"`
	Percentage   float64 `json:"percentage"`
	TopThread    string  `json:"top_thread,omitempty"`
}

// NewFlameGraph creates a new flame graph with a root node.
func NewFlameGraph() *FlameGraph {
	return &FlameGraph{
		Root: NewNode("root", 0),
	}
}

// NewFlameGraphWithAnalysis creates a new flame graph with thread analysis support.
func NewFlameGraphWithAnalysis() *FlameGraph {
	return &FlameGraph{
		Root: NewNode("root", 0),
		ThreadAnalysis: &ThreadAnalysisData{
			Threads:      make([]*ThreadInfo, 0),
			TopFunctions: make([]*TopFunction, 0),
			ThreadGroups: make([]*ThreadGroupInfo, 0),
		},
	}
}

// Cleanup removes internal maps and filters nodes below threshold.
func (fg *FlameGraph) Cleanup(minPercent float64) {
	if fg.Root == nil {
		return
	}

	threshold := int64(0)
	if minPercent > 0 && fg.TotalSamples > 0 {
		threshold = int64(float64(fg.TotalSamples) * minPercent / 100.0)
	}
	fg.Root.Cleanup(threshold)
}

// CalculateMaxDepth calculates the maximum depth of the flame graph.
func (fg *FlameGraph) CalculateMaxDepth() int {
	if fg.Root == nil {
		return 0
	}
	fg.MaxDepth = calculateDepth(fg.Root, 0)
	return fg.MaxDepth
}

func calculateDepth(node *Node, currentDepth int) int {
	if node.Children == nil || len(node.Children) == 0 {
		return currentDepth
	}

	maxChildDepth := currentDepth
	for _, child := range node.Children {
		childDepth := calculateDepth(child, currentDepth+1)
		if childDepth > maxChildDepth {
			maxChildDepth = childDepth
		}
	}
	return maxChildDepth
}

// GetThread retrieves a thread by TID.
func (fg *FlameGraph) GetThread(tid int) *ThreadInfo {
	if fg.ThreadAnalysis == nil {
		return nil
	}
	for _, t := range fg.ThreadAnalysis.Threads {
		if t.TID == tid {
			return t
		}
	}
	return nil
}

// GetThreadGroups returns thread group summaries.
func (fg *FlameGraph) GetThreadGroups() []*ThreadGroupInfo {
	if fg.ThreadAnalysis == nil {
		return nil
	}
	return fg.ThreadAnalysis.ThreadGroups
}

// SortThreads sorts threads by sample count descending.
func (fg *FlameGraph) SortThreads() {
	if fg.ThreadAnalysis == nil || len(fg.ThreadAnalysis.Threads) == 0 {
		return
	}
	sort.Slice(fg.ThreadAnalysis.Threads, func(i, j int) bool {
		return fg.ThreadAnalysis.Threads[i].Samples > fg.ThreadAnalysis.Threads[j].Samples
	})
}

// NodeBuilder helps construct flame graph trees efficiently.
type NodeBuilder struct {
	root     *Node
	nodePool sync.Pool
}

// NewNodeBuilder creates a new NodeBuilder.
func NewNodeBuilder(rootName string) *NodeBuilder {
	return &NodeBuilder{
		root: NewNode(rootName, 0),
		nodePool: sync.Pool{
			New: func() interface{} {
				return &Node{Children: make([]*Node, 0, 4), childrenMap: make(map[string]int, 4)}
			},
		},
	}
}

// AddStack adds a call stack to the flame graph.
func (b *NodeBuilder) AddStack(stack []string, value int64) {
	if len(stack) == 0 || value <= 0 {
		return
	}

	current := b.root
	current.Value += value

	for _, frame := range stack {
		// Find or create child
		var child *Node
		if current.childrenMap == nil {
			current.childrenMap = make(map[string]int)
		}
		if idx, exists := current.childrenMap[frame]; exists {
			child = current.Children[idx]
		}

		if child == nil {
			child = b.nodePool.Get().(*Node)
			child.Name = frame
			child.Value = 0
			child.Self = 0
			child.Module = ""
			child.TID = 0
			child.Process = ""
			if child.Children == nil {
				child.Children = make([]*Node, 0, 4)
			} else {
				child.Children = child.Children[:0]
			}
			if child.childrenMap == nil {
				child.childrenMap = make(map[string]int, 4)
			} else {
				for k := range child.childrenMap {
					delete(child.childrenMap, k)
				}
			}
			current.childrenMap[frame] = len(current.Children)
			current.Children = append(current.Children, child)
		}

		child.Value += value
		current = child
	}

	// The leaf node gets the self value
	current.Self += value
}

// Build returns the constructed flame graph node.
func (b *NodeBuilder) Build() *Node {
	return b.root
}

// MergeNodes merges multiple flame nodes into one.
func MergeNodes(nodes []*Node) *Node {
	if len(nodes) == 0 {
		return nil
	}
	if len(nodes) == 1 {
		return nodes[0]
	}

	root := NewNode("all", 0)
	for _, node := range nodes {
		if node == nil {
			continue
		}
		root.Value += node.Value
		root.Children = append(root.Children, node)
	}

	return root
}

// CPUAnalysisResult holds the complete CPU profiling analysis result.
// This is the unified result type for both analysis and API responses.
type CPUAnalysisResult struct {
	// Summary statistics
	TotalSamples       int64 `json:"total_samples"`
	TotalThreads       int   `json:"total_threads"`
	ActiveThreads      int   `json:"active_threads"`
	MaxStackDepth      int   `json:"max_stack_depth"`
	UniqueFunctions    int   `json:"unique_functions"`
	AnalysisDurationMs int64 `json:"analysis_duration_ms,omitempty"`

	// Thread-level analysis
	Threads      []*ThreadInfo          `json:"threads"`
	ThreadsByTID map[int]*ThreadInfo    `json:"-"`
	ThreadGroups map[string][]*ThreadInfo `json:"thread_groups,omitempty"`

	// Global top functions
	TopFuncs           []*TopFunction           `json:"top_funcs"`
	TopFuncsCallstacks map[string]*CallStackInfo `json:"top_funcs_callstacks,omitempty"`

	// Flame graph data (for visualization)
	FlameGraph   *Node          `json:"flame_graph,omitempty"`
	ThreadFlames map[int]*Node  `json:"-"`

	// Internal use
	mu sync.RWMutex `json:"-"`
}

// CallStackInfo holds call stack information for a top function.
type CallStackInfo struct {
	FunctionName string   `json:"func"`
	CallStacks   []string `json:"callstacks"`
	Count        int      `json:"count"`
}

// NewCPUAnalysisResult creates a new CPUAnalysisResult with initialized maps.
func NewCPUAnalysisResult() *CPUAnalysisResult {
	return &CPUAnalysisResult{
		Threads:            make([]*ThreadInfo, 0),
		ThreadsByTID:       make(map[int]*ThreadInfo),
		ThreadGroups:       make(map[string][]*ThreadInfo),
		TopFuncs:           make([]*TopFunction, 0),
		TopFuncsCallstacks: make(map[string]*CallStackInfo),
		ThreadFlames:       make(map[int]*Node),
	}
}

// AddThread adds a thread analysis result (thread-safe).
func (r *CPUAnalysisResult) AddThread(thread *ThreadInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Threads = append(r.Threads, thread)
	r.ThreadsByTID[thread.TID] = thread

	// Group threads by name pattern
	group := profiling.ExtractThreadGroup(thread.Name)
	thread.Group = group
	r.ThreadGroups[group] = append(r.ThreadGroups[group], thread)
}

// GetThread retrieves a thread by TID (thread-safe).
func (r *CPUAnalysisResult) GetThread(tid int) *ThreadInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ThreadsByTID[tid]
}

// SortThreads sorts threads by sample count descending.
func (r *CPUAnalysisResult) SortThreads() {
	r.mu.Lock()
	defer r.mu.Unlock()

	sort.Slice(r.Threads, func(i, j int) bool {
		return r.Threads[i].Samples > r.Threads[j].Samples
	})
}

// FilterThreads returns threads matching the filter criteria.
func (r *CPUAnalysisResult) FilterThreads(filter ThreadFilter) []*ThreadInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ThreadInfo, 0)
	for _, t := range r.Threads {
		if filter.Match(t) {
			result = append(result, t)
		}
	}
	return result
}

// ThreadFilter defines criteria for filtering threads.
type ThreadFilter struct {
	NamePattern    string  `json:"name_pattern,omitempty"`
	MinSamples     int64   `json:"min_samples,omitempty"`
	MinPercentage  float64 `json:"min_percentage,omitempty"`
	IncludeSwapper bool    `json:"include_swapper,omitempty"`
	ThreadGroup    string  `json:"thread_group,omitempty"`
	TIDs           []int   `json:"tids,omitempty"`
}

// Match checks if a thread matches the filter criteria.
func (f *ThreadFilter) Match(t *ThreadInfo) bool {
	// Exclude swapper by default
	if t.IsSwapper && !f.IncludeSwapper {
		return false
	}

	// Filter by minimum samples
	if f.MinSamples > 0 && t.Samples < f.MinSamples {
		return false
	}

	// Filter by minimum percentage
	if f.MinPercentage > 0 && t.Percentage < f.MinPercentage {
		return false
	}

	// Filter by thread group
	if f.ThreadGroup != "" && t.Group != f.ThreadGroup {
		return false
	}

	// Filter by specific TIDs
	if len(f.TIDs) > 0 {
		found := false
		for _, tid := range f.TIDs {
			if tid == t.TID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by name pattern (simple contains match)
	if f.NamePattern != "" {
		if !containsIgnoreCase(t.Name, f.NamePattern) {
			return false
		}
	}

	return true
}

// ThreadGroupSummary provides aggregated statistics for a thread group.
type ThreadGroupSummary struct {
	GroupName    string  `json:"group_name"`
	ThreadCount  int     `json:"thread_count"`
	TotalSamples int64   `json:"total_samples"`
	Percentage   float64 `json:"percentage"`
	TopThread    string  `json:"top_thread"`
}

// GetThreadGroupSummaries returns summaries for all thread groups.
func (r *CPUAnalysisResult) GetThreadGroupSummaries() []ThreadGroupSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	summaries := make([]ThreadGroupSummary, 0, len(r.ThreadGroups))
	for group, threads := range r.ThreadGroups {
		var totalSamples int64
		var topThread *ThreadInfo
		for _, t := range threads {
			totalSamples += t.Samples
			if topThread == nil || t.Samples > topThread.Samples {
				topThread = t
			}
		}

		topThreadName := ""
		if topThread != nil {
			topThreadName = topThread.Name
		}

		summaries = append(summaries, ThreadGroupSummary{
			GroupName:    group,
			ThreadCount:  len(threads),
			TotalSamples: totalSamples,
			Percentage:   float64(totalSamples) / float64(r.TotalSamples) * 100,
			TopThread:    topThreadName,
		})
	}

	// Sort by total samples descending
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].TotalSamples > summaries[j].TotalSamples
	})

	return summaries
}

// SearchResult represents a search result for functions or threads.
type SearchResult struct {
	Type       string      `json:"type"` // "function" or "thread"
	Name       string      `json:"name"`
	Samples    int64       `json:"samples"`
	Percentage float64     `json:"percentage"`
	Context    interface{} `json:"context,omitempty"`
}

// Search searches for functions or threads matching the query.
func (r *CPUAnalysisResult) Search(query string, searchType string, limit int) []SearchResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]SearchResult, 0)

	if searchType == "" || searchType == "function" {
		for _, f := range r.TopFuncs {
			if containsIgnoreCase(f.Name, query) {
				results = append(results, SearchResult{
					Type:       "function",
					Name:       f.Name,
					Samples:    f.Samples,
					Percentage: f.Percentage,
					Context:    f.Threads,
				})
			}
		}
	}

	if searchType == "" || searchType == "thread" {
		for _, t := range r.Threads {
			if containsIgnoreCase(t.Name, query) {
				results = append(results, SearchResult{
					Type:       "thread",
					Name:       t.Name,
					Samples:    t.Samples,
					Percentage: t.Percentage,
					Context:    t.TID,
				})
			}
		}
	}

	// Sort by samples descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Samples > results[j].Samples
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
