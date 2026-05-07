package hprof

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Test Helpers - Graph Construction
// ============================================================================

// buildDomTestGraph creates a graph for dominator tests using the given topology.
// topology defines edges as from→to pairs.
// gcRoots defines which objectIDs are GC roots.
// objects defines objectID, classID, shallowSize triples.
func buildDomTestGraph(t *testing.T, objects []struct {
	objID   uint64
	classID uint64
	size    int64
}, edges []struct {
	from int32
	to   int32
}, gcRootIndices []int32,
) *IndexedReferenceGraph {
	t.Helper()

	n := len(objects)
	graph := NewIndexedReferenceGraph(n)

	for _, obj := range objects {
		graph.AddObject(obj.objID, obj.classID, obj.size)
	}

	// Add GC roots
	for _, idx := range gcRootIndices {
		graph.AddGCRoot(GCRoot{
			ObjectID: objects[idx].objID,
			Type:     GCRootJNIGlobal,
		})
	}

	graph.FinalizeObjects()

	// Build edges
	outBuilder := NewCompactEdgeListBuilder(n, len(edges))
	inBuilder := NewCompactEdgeListBuilder(n, len(edges))

	for _, e := range edges {
		outBuilder.AddEdge(e.from, e.to, "ref", 100)
		inBuilder.AddEdge(e.to, e.from, "ref", 100)
	}

	graph.BuildEdges(outBuilder, inBuilder)
	return graph
}

// ============================================================================
// Test: Empty Graph (0 objects)
// ============================================================================

func TestComputeDominatorForIndexedGraph_EmptyGraph(t *testing.T) {
	graph := NewIndexedReferenceGraph(0)
	graph.FinalizeObjects()

	// Build empty edges
	outBuilder := NewCompactEdgeListBuilder(0, 0)
	inBuilder := NewCompactEdgeListBuilder(0, 0)
	graph.BuildEdges(outBuilder, inBuilder)

	// Should not panic
	ComputeDominatorForIndexedGraph(context.Background(), graph)

	assert.Equal(t, int32(0), graph.ObjectCount())
}

// ============================================================================
// Test: Single GC Root Object
// ============================================================================

func TestComputeDominatorForIndexedGraph_SingleObject(t *testing.T) {
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 64},
	}

	graph := buildDomTestGraph(t, objects, nil, []int32{0})

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	// Single GC root's dominator should be -1 (dominated by super root)
	assert.Equal(t, int32(-1), graph.GetDominator(0))
	// Retained size = shallow size (no children)
	assert.Equal(t, int64(64), graph.GetRetainedSize(0))
}

// ============================================================================
// Test: Linear Chain (A→B→C→D→E)
// ============================================================================

func TestComputeDominatorForIndexedGraph_LinearChain(t *testing.T) {
	// A(root) → B → C → D → E
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 10}, // idx=0: A (GC root)
		{0x1002, 100, 20}, // idx=1: B
		{0x1003, 100, 30}, // idx=2: C
		{0x1004, 100, 40}, // idx=3: D
		{0x1005, 100, 50}, // idx=4: E
	}

	edges := []struct {
		from int32
		to   int32
	}{
		{0, 1}, // A→B
		{1, 2}, // B→C
		{2, 3}, // C→D
		{3, 4}, // D→E
	}

	graph := buildDomTestGraph(t, objects, edges, []int32{0})

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	// A is dominated by super root
	assert.Equal(t, int32(-1), graph.GetDominator(0), "A's dominator should be super root")
	// B is dominated by A
	assert.Equal(t, int32(0), graph.GetDominator(1), "B's dominator should be A")
	// C is dominated by B
	assert.Equal(t, int32(1), graph.GetDominator(2), "C's dominator should be B")
	// D is dominated by C
	assert.Equal(t, int32(2), graph.GetDominator(3), "D's dominator should be C")
	// E is dominated by D
	assert.Equal(t, int32(3), graph.GetDominator(4), "E's dominator should be D")

	// Retained sizes: bottom-up accumulation
	// E retained = 50 (leaf)
	assert.Equal(t, int64(50), graph.GetRetainedSize(4), "E retained size")
	// D retained = 40 + 50 = 90
	assert.Equal(t, int64(90), graph.GetRetainedSize(3), "D retained size")
	// C retained = 30 + 90 = 120
	assert.Equal(t, int64(120), graph.GetRetainedSize(2), "C retained size")
	// B retained = 20 + 120 = 140
	assert.Equal(t, int64(140), graph.GetRetainedSize(1), "B retained size")
	// A retained = 10 + 140 = 150
	assert.Equal(t, int64(150), graph.GetRetainedSize(0), "A retained size")
}

// ============================================================================
// Test: Diamond / Convergence Pattern
// ============================================================================

func TestComputeDominatorForIndexedGraph_Diamond(t *testing.T) {
	// A(root) → B, A → C, B → D, C → D
	// D's only common dominator is A (not B or C)
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 10}, // idx=0: A (GC root)
		{0x1002, 100, 20}, // idx=1: B
		{0x1003, 100, 30}, // idx=2: C
		{0x1004, 100, 40}, // idx=3: D
	}

	edges := []struct {
		from int32
		to   int32
	}{
		{0, 1}, // A→B
		{0, 2}, // A→C
		{1, 3}, // B→D
		{2, 3}, // C→D
	}

	graph := buildDomTestGraph(t, objects, edges, []int32{0})

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	// A → super root
	assert.Equal(t, int32(-1), graph.GetDominator(0), "A's dominator should be super root")
	// B → A
	assert.Equal(t, int32(0), graph.GetDominator(1), "B's dominator should be A")
	// C → A
	assert.Equal(t, int32(0), graph.GetDominator(2), "C's dominator should be A")
	// D → A (both B and C point to D, so D's idom is their LCA = A)
	assert.Equal(t, int32(0), graph.GetDominator(3), "D's dominator should be A (LCA of B and C)")

	// Retained sizes:
	// D is dominated by A directly, so D's retained adds to A
	// B and C have no children in dominator tree (D is child of A)
	assert.Equal(t, int64(20), graph.GetRetainedSize(1), "B retained = shallow only")
	assert.Equal(t, int64(30), graph.GetRetainedSize(2), "C retained = shallow only")
	assert.Equal(t, int64(40), graph.GetRetainedSize(3), "D retained = shallow only")
	// A retained = 10 + 20 + 30 + 40 = 100
	assert.Equal(t, int64(100), graph.GetRetainedSize(0), "A retained = all objects")
}

// ============================================================================
// Test: Multiple GC Roots
// ============================================================================

func TestComputeDominatorForIndexedGraph_MultipleRoots(t *testing.T) {
	// R1(root) → A → C
	// R2(root) → B → C
	// C has two paths from roots, dominator = super root (i.e. -1)
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 10}, // idx=0: R1 (GC root)
		{0x1002, 100, 20}, // idx=1: R2 (GC root)
		{0x1003, 100, 30}, // idx=2: A
		{0x1004, 100, 40}, // idx=3: B
		{0x1005, 100, 50}, // idx=4: C
	}

	edges := []struct {
		from int32
		to   int32
	}{
		{0, 2}, // R1→A
		{1, 3}, // R2→B
		{2, 4}, // A→C
		{3, 4}, // B→C
	}

	graph := buildDomTestGraph(t, objects, edges, []int32{0, 1})

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	// Both roots dominated by super root
	assert.Equal(t, int32(-1), graph.GetDominator(0), "R1 idom = super root")
	assert.Equal(t, int32(-1), graph.GetDominator(1), "R2 idom = super root")
	// A dominated by R1
	assert.Equal(t, int32(0), graph.GetDominator(2), "A idom = R1")
	// B dominated by R2
	assert.Equal(t, int32(1), graph.GetDominator(3), "B idom = R2")
	// C: two paths R1→A→C and R2→B→C, LCA in dominator tree is super root
	assert.Equal(t, int32(-1), graph.GetDominator(4), "C idom = super root (multiple root paths)")

	// Retained sizes
	assert.Equal(t, int64(50), graph.GetRetainedSize(4), "C retained = shallow (child of super root)")
	assert.Equal(t, int64(30), graph.GetRetainedSize(2), "A retained = shallow (C not under A)")
	assert.Equal(t, int64(40), graph.GetRetainedSize(3), "B retained = shallow (C not under B)")
	// R1 retained = 10 + 30 = 40
	assert.Equal(t, int64(40), graph.GetRetainedSize(0), "R1 retained = R1 + A")
	// R2 retained = 20 + 40 = 60
	assert.Equal(t, int64(60), graph.GetRetainedSize(1), "R2 retained = R2 + B")
}

// ============================================================================
// Test: Unreachable Objects (disconnected subgraph)
// ============================================================================

func TestComputeDominatorForIndexedGraph_UnreachableObjects(t *testing.T) {
	// A(root) → B
	// C, D are not reachable from any GC root
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 10}, // idx=0: A (GC root)
		{0x1002, 100, 20}, // idx=1: B (reachable via A)
		{0x1003, 100, 30}, // idx=2: C (unreachable)
		{0x1004, 100, 40}, // idx=3: D (unreachable)
	}

	edges := []struct {
		from int32
		to   int32
	}{
		{0, 1}, // A→B
		{2, 3}, // C→D (disconnected subgraph)
	}

	graph := buildDomTestGraph(t, objects, edges, []int32{0})

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	// A: GC root, dominated by super root
	assert.Equal(t, int32(-1), graph.GetDominator(0))
	// B: dominated by A
	assert.Equal(t, int32(0), graph.GetDominator(1))
	// C: unreachable, dominator = -1
	assert.Equal(t, int32(-1), graph.GetDominator(2), "unreachable C dominator = -1")
	// D: unreachable, dominator = -1
	assert.Equal(t, int32(-1), graph.GetDominator(3), "unreachable D dominator = -1")

	// Retained sizes: unreachable objects should keep initial shallow size (no accumulation)
	assert.Equal(t, int64(30), graph.GetRetainedSize(2), "unreachable C retains initial value")
	assert.Equal(t, int64(40), graph.GetRetainedSize(3), "unreachable D retains initial value")

	// Reachable objects: correct retained sizes
	assert.Equal(t, int64(20), graph.GetRetainedSize(1), "B retained = shallow (leaf)")
	assert.Equal(t, int64(30), graph.GetRetainedSize(0), "A retained = 10 + 20 = 30")
}

// ============================================================================
// Test: Tree with Branching
// ============================================================================

func TestComputeDominatorForIndexedGraph_TreeBranching(t *testing.T) {
	// A(root) → B, A → C
	// B → D, B → E
	// C → F
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 10},  // idx=0: A (GC root)
		{0x1002, 100, 20},  // idx=1: B
		{0x1003, 100, 30},  // idx=2: C
		{0x1004, 100, 40},  // idx=3: D
		{0x1005, 100, 50},  // idx=4: E
		{0x1006, 100, 100}, // idx=5: F
	}

	edges := []struct {
		from int32
		to   int32
	}{
		{0, 1}, // A→B
		{0, 2}, // A→C
		{1, 3}, // B→D
		{1, 4}, // B→E
		{2, 5}, // C→F
	}

	graph := buildDomTestGraph(t, objects, edges, []int32{0})

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	// Dominator tree matches reference graph (since it's already a tree)
	assert.Equal(t, int32(-1), graph.GetDominator(0), "A idom = super root")
	assert.Equal(t, int32(0), graph.GetDominator(1), "B idom = A")
	assert.Equal(t, int32(0), graph.GetDominator(2), "C idom = A")
	assert.Equal(t, int32(1), graph.GetDominator(3), "D idom = B")
	assert.Equal(t, int32(1), graph.GetDominator(4), "E idom = B")
	assert.Equal(t, int32(2), graph.GetDominator(5), "F idom = C")

	// Retained sizes
	assert.Equal(t, int64(40), graph.GetRetainedSize(3), "D retained = 40")
	assert.Equal(t, int64(50), graph.GetRetainedSize(4), "E retained = 50")
	assert.Equal(t, int64(100), graph.GetRetainedSize(5), "F retained = 100")
	// B retained = 20 + 40 + 50 = 110
	assert.Equal(t, int64(110), graph.GetRetainedSize(1), "B retained = 20+40+50")
	// C retained = 30 + 100 = 130
	assert.Equal(t, int64(130), graph.GetRetainedSize(2), "C retained = 30+100")
	// A retained = 10 + 110 + 130 = 250
	assert.Equal(t, int64(250), graph.GetRetainedSize(0), "A retained = all")
}

// ============================================================================
// Test: Cycle (Back Edge)
// ============================================================================

func TestComputeDominatorForIndexedGraph_Cycle(t *testing.T) {
	// A(root) → B → C → B (cycle)
	// Dominator tree should still be well-defined
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 10}, // idx=0: A (GC root)
		{0x1002, 100, 20}, // idx=1: B
		{0x1003, 100, 30}, // idx=2: C
	}

	edges := []struct {
		from int32
		to   int32
	}{
		{0, 1}, // A→B
		{1, 2}, // B→C
		{2, 1}, // C→B (back edge, creates cycle)
	}

	graph := buildDomTestGraph(t, objects, edges, []int32{0})

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	// A dominated by super root
	assert.Equal(t, int32(-1), graph.GetDominator(0))
	// B dominated by A (first reached from A, cycle doesn't change dominator)
	assert.Equal(t, int32(0), graph.GetDominator(1), "B idom = A")
	// C dominated by B
	assert.Equal(t, int32(1), graph.GetDominator(2), "C idom = B")

	// Retained sizes
	assert.Equal(t, int64(30), graph.GetRetainedSize(2), "C retained = 30")
	assert.Equal(t, int64(50), graph.GetRetainedSize(1), "B retained = 20 + 30 = 50")
	assert.Equal(t, int64(60), graph.GetRetainedSize(0), "A retained = 10 + 50 = 60")
}

// ============================================================================
// Test: Class Objects as Roots
// ============================================================================

func TestComputeDominatorForIndexedGraph_ClassObjectsAsRoots(t *testing.T) {
	// classObj(classObject) → instance
	// classObj is treated as root (via classObjectBits)
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 16}, // idx=0: classObj
		{0x1002, 100, 32}, // idx=1: instance
	}

	edges := []struct {
		from int32
		to   int32
	}{
		{0, 1}, // classObj → instance
	}

	n := len(objects)
	graph := NewIndexedReferenceGraph(n)

	for _, obj := range objects {
		graph.AddObject(obj.objID, obj.classID, obj.size)
	}

	// No GC roots, but mark classObj as class object
	graph.FinalizeObjects()
	graph.classObjectBits.Set(0) // Mark idx=0 as class object

	// Build edges
	outBuilder := NewCompactEdgeListBuilder(n, len(edges))
	inBuilder := NewCompactEdgeListBuilder(n, len(edges))
	for _, e := range edges {
		outBuilder.AddEdge(e.from, e.to, "ref", 100)
		inBuilder.AddEdge(e.to, e.from, "ref", 100)
	}
	graph.BuildEdges(outBuilder, inBuilder)

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	// classObj is treated as root (via classObjectBits), dominated by super root
	assert.Equal(t, int32(-1), graph.GetDominator(0), "classObj idom = super root")
	// instance dominated by classObj
	assert.Equal(t, int32(0), graph.GetDominator(1), "instance idom = classObj")

	// Retained sizes
	assert.Equal(t, int64(32), graph.GetRetainedSize(1), "instance retained = 32")
	assert.Equal(t, int64(48), graph.GetRetainedSize(0), "classObj retained = 16 + 32 = 48")
}

// ============================================================================
// Test: nil Context (should use Background)
// ============================================================================

func TestComputeDominatorForIndexedGraph_NilContext(t *testing.T) {
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 10},
		{0x1002, 100, 20},
	}

	edges := []struct {
		from int32
		to   int32
	}{
		{0, 1},
	}

	graph := buildDomTestGraph(t, objects, edges, []int32{0})

	// Should not panic with nil context
	ComputeDominatorForIndexedGraph(nil, graph)

	assert.Equal(t, int32(-1), graph.GetDominator(0))
	assert.Equal(t, int32(0), graph.GetDominator(1))
}

// ============================================================================
// Test: No Edges Graph (all isolated roots)
// ============================================================================

func TestComputeDominatorForIndexedGraph_NoEdges(t *testing.T) {
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 10},
		{0x1002, 100, 20},
		{0x1003, 100, 30},
	}

	// All are GC roots, no edges between them
	graph := buildDomTestGraph(t, objects, nil, []int32{0, 1, 2})

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	// All dominated by super root
	for i := int32(0); i < 3; i++ {
		assert.Equal(t, int32(-1), graph.GetDominator(i), "object %d idom = super root", i)
	}

	// Retained = shallow (no children)
	assert.Equal(t, int64(10), graph.GetRetainedSize(0))
	assert.Equal(t, int64(20), graph.GetRetainedSize(1))
	assert.Equal(t, int64(30), graph.GetRetainedSize(2))
}

// ============================================================================
// Test: Complex DAG (multiple convergence points)
// ============================================================================

func TestComputeDominatorForIndexedGraph_ComplexDAG(t *testing.T) {
	// A(root) → B → D → F
	// A → C → D → F
	// A → C → E → F
	// D's dominator: both B and C lead to D, so D's idom = A
	// F's dominator: B→D→F, C→D→F, C→E→F. All paths from A, so F's idom = A
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 10}, // idx=0: A (root)
		{0x1002, 100, 20}, // idx=1: B
		{0x1003, 100, 30}, // idx=2: C
		{0x1004, 100, 40}, // idx=3: D
		{0x1005, 100, 50}, // idx=4: E
		{0x1006, 100, 60}, // idx=5: F
	}

	edges := []struct {
		from int32
		to   int32
	}{
		{0, 1}, // A→B
		{0, 2}, // A→C
		{1, 3}, // B→D
		{2, 3}, // C→D
		{2, 4}, // C→E
		{3, 5}, // D→F
		{4, 5}, // E→F
	}

	graph := buildDomTestGraph(t, objects, edges, []int32{0})

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	assert.Equal(t, int32(-1), graph.GetDominator(0), "A idom = super root")
	assert.Equal(t, int32(0), graph.GetDominator(1), "B idom = A")
	assert.Equal(t, int32(0), graph.GetDominator(2), "C idom = A")
	// D: reached from B and C, both children of A → D's idom = A
	assert.Equal(t, int32(0), graph.GetDominator(3), "D idom = A")
	// E: only reached from C → E's idom = C
	assert.Equal(t, int32(2), graph.GetDominator(4), "E idom = C")
	// F: reached from D and E, D's idom=A, E's idom=C (which is child of A)
	// LCA(D,E) in dominator tree: D is child of A, E is child of C which is child of A → LCA = A
	assert.Equal(t, int32(0), graph.GetDominator(5), "F idom = A")
}

// ============================================================================
// Test: dominatorComputed flag
// ============================================================================

func TestComputeDominatorForIndexedGraph_SetsComputedFlag(t *testing.T) {
	objects := []struct {
		objID   uint64
		classID uint64
		size    int64
	}{
		{0x1001, 100, 10},
	}

	graph := buildDomTestGraph(t, objects, nil, []int32{0})
	require.False(t, graph.dominatorComputed, "should be false before compute")

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	assert.True(t, graph.dominatorComputed, "should be true after compute")
}

// ============================================================================
// Test: Large Graph (stress test for correctness and no panics)
// ============================================================================

func TestComputeDominatorForIndexedGraph_LargeLinearChain(t *testing.T) {
	const n = 1000
	objects := make([]struct {
		objID   uint64
		classID uint64
		size    int64
	}, n)

	for i := 0; i < n; i++ {
		objects[i] = struct {
			objID   uint64
			classID uint64
			size    int64
		}{uint64(0x1000 + i), 100, int64(i + 1)}
	}

	edges := make([]struct {
		from int32
		to   int32
	}, n-1)
	for i := 0; i < n-1; i++ {
		edges[i] = struct {
			from int32
			to   int32
		}{int32(i), int32(i + 1)}
	}

	graph := buildDomTestGraph(t, objects, edges, []int32{0})

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	// Verify linear dominator chain: each i is dominated by i-1
	assert.Equal(t, int32(-1), graph.GetDominator(0))
	for i := int32(1); i < n; i++ {
		assert.Equal(t, i-1, graph.GetDominator(i), "object %d idom should be %d", i, i-1)
	}

	// Verify retained sizes: object at index i retains sum of shallow sizes from i to n-1
	// shallow[i] = i+1, so retained[i] = sum(j+1 for j in [i, n-1]) = sum(i+1..n)
	for i := int32(0); i < n; i++ {
		expected := int64(0)
		for j := i; j < n; j++ {
			expected += int64(j + 1)
		}
		assert.Equal(t, expected, graph.GetRetainedSize(i), "retained size at idx %d", i)
	}
}

// ============================================================================
// Test: Wide Fan-out (one root → many children)
// ============================================================================

func TestComputeDominatorForIndexedGraph_WideFanout(t *testing.T) {
	const childCount = 100
	n := 1 + childCount // root + children
	objects := make([]struct {
		objID   uint64
		classID uint64
		size    int64
	}, n)

	objects[0] = struct {
		objID   uint64
		classID uint64
		size    int64
	}{0x1000, 100, 10}

	for i := 1; i < n; i++ {
		objects[i] = struct {
			objID   uint64
			classID uint64
			size    int64
		}{uint64(0x1000 + i), 100, int64(i * 10)}
	}

	edges := make([]struct {
		from int32
		to   int32
	}, childCount)
	for i := 0; i < childCount; i++ {
		edges[i] = struct {
			from int32
			to   int32
		}{0, int32(i + 1)}
	}

	graph := buildDomTestGraph(t, objects, edges, []int32{0})

	ComputeDominatorForIndexedGraph(context.Background(), graph)

	// All children dominated by root
	for i := int32(1); i < int32(n); i++ {
		assert.Equal(t, int32(0), graph.GetDominator(i), "child %d idom = root", i)
	}

	// Root retained = sum of all shallow sizes
	totalRetained := int64(10)
	for i := 1; i < n; i++ {
		totalRetained += int64(i * 10)
		// Each child retained = its own shallow size (leaf)
		assert.Equal(t, int64(i*10), graph.GetRetainedSize(int32(i)))
	}
	assert.Equal(t, totalRetained, graph.GetRetainedSize(0))
}
