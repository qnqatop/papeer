package graphalgo

import (
	"math"
	"testing"
)

func undirected(adj map[int64][]int64, a, b int64) {
	adj[a] = append(adj[a], b)
	adj[b] = append(adj[b], a)
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// Star graph: center 0 connected to leaves 1..4. Every shortest path between
// two leaves passes through the center, so the center's betweenness equals
// C(4,2)=6 and every leaf's betweenness is 0.
func TestBetweenness_Star(t *testing.T) {
	nodes := []int64{0, 1, 2, 3, 4}
	adj := make(map[int64][]int64)
	for _, leaf := range []int64{1, 2, 3, 4} {
		undirected(adj, 0, leaf)
	}

	centrality := Betweenness(nodes, adj)

	if !almostEqual(centrality[0], 6) {
		t.Errorf("center betweenness = %v, want 6", centrality[0])
	}
	for _, leaf := range []int64{1, 2, 3, 4} {
		if !almostEqual(centrality[leaf], 0) {
			t.Errorf("leaf %d betweenness = %v, want 0", leaf, centrality[leaf])
		}
	}
}

// Path A-B-C: the only pair whose shortest path has an intermediate node is
// (A,C), which passes through B. So B=1, A=C=0.
func TestBetweenness_PathOfThree(t *testing.T) {
	nodes := []int64{1, 2, 3} // A=1 (endpoint), B=2 (middle), C=3 (endpoint)
	adj := make(map[int64][]int64)
	undirected(adj, 1, 2)
	undirected(adj, 2, 3)

	centrality := Betweenness(nodes, adj)

	if !almostEqual(centrality[2], 1) {
		t.Errorf("middle node betweenness = %v, want 1", centrality[2])
	}
	if !almostEqual(centrality[1], 0) || !almostEqual(centrality[3], 0) {
		t.Errorf("endpoint betweenness = %v/%v, want 0/0", centrality[1], centrality[3])
	}
}

// Longer path A-B-C-D-E: node i (0-indexed from one end) has betweenness
// i*(n-1-i) for a path of n nodes. For n=5: B(i=1)=1*3=3, C(i=2)=2*2=4.
func TestBetweenness_PathOfFive(t *testing.T) {
	nodes := []int64{0, 1, 2, 3, 4}
	adj := make(map[int64][]int64)
	undirected(adj, 0, 1)
	undirected(adj, 1, 2)
	undirected(adj, 2, 3)
	undirected(adj, 3, 4)

	centrality := Betweenness(nodes, adj)

	want := map[int64]float64{0: 0, 1: 3, 2: 4, 3: 3, 4: 0}
	for node, w := range want {
		if !almostEqual(centrality[node], w) {
			t.Errorf("node %d betweenness = %v, want %v", node, centrality[node], w)
		}
	}
}

// 4-cycle 0-1-2-3-0: by symmetry, every node has the same betweenness. Each
// pair of opposite nodes has two equally-short paths through the two other
// nodes, splitting credit 0.5/0.5, so every node ends up with 0.5.
func TestBetweenness_Cycle(t *testing.T) {
	nodes := []int64{0, 1, 2, 3}
	adj := make(map[int64][]int64)
	undirected(adj, 0, 1)
	undirected(adj, 1, 2)
	undirected(adj, 2, 3)
	undirected(adj, 3, 0)

	centrality := Betweenness(nodes, adj)

	for _, n := range nodes {
		if !almostEqual(centrality[n], 0.5) {
			t.Errorf("cycle node %d betweenness = %v, want 0.5 (symmetric)", n, centrality[n])
		}
	}
}

func TestBetweenness_IsolatedNodeIsZero(t *testing.T) {
	nodes := []int64{1, 2, 3, 99}
	adj := make(map[int64][]int64)
	undirected(adj, 1, 2)
	undirected(adj, 2, 3)
	// 99 has no edges at all.

	centrality := Betweenness(nodes, adj)
	if !almostEqual(centrality[99], 0) {
		t.Errorf("isolated node betweenness = %v, want 0", centrality[99])
	}
}

func TestBetweenness_DisconnectedComponents(t *testing.T) {
	// Two separate triangles-ish components: {1,2,3} path and {10,11} edge.
	nodes := []int64{1, 2, 3, 10, 11}
	adj := make(map[int64][]int64)
	undirected(adj, 1, 2)
	undirected(adj, 2, 3)
	undirected(adj, 10, 11)

	centrality := Betweenness(nodes, adj)
	if !almostEqual(centrality[2], 1) {
		t.Errorf("component 1 middle node betweenness = %v, want 1", centrality[2])
	}
	if !almostEqual(centrality[10], 0) || !almostEqual(centrality[11], 0) {
		t.Errorf("component 2 nodes betweenness = %v/%v, want 0/0", centrality[10], centrality[11])
	}
}

func TestBetweenness_EmptyGraph(t *testing.T) {
	centrality := Betweenness(nil, nil)
	if len(centrality) != 0 {
		t.Errorf("Betweenness(nil,nil) = %v, want empty map", centrality)
	}
}
