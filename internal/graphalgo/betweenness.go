// Package graphalgo provides small, dependency-free graph algorithms used by
// the Analysis section (Key Papers ranking).
package graphalgo

// Betweenness computes betweenness centrality for every node in an
// undirected, unweighted graph using Brandes' algorithm (Ulrik Brandes, "A
// Faster Algorithm for Betweenness Centrality", 2001), O(V·E) time.
//
// nodes is the full node set (isolated nodes get centrality 0); adj is an
// adjacency list where adj[v] lists v's neighbors. The graph is treated as
// undirected regardless of how the caller populated adj — for a citation
// graph this identifies papers that bridge otherwise-separate clusters of
// the corpus (a paper cited by, and citing into, different sub-fields),
// which is what "who connects sub-fields" (Bridge ranking) means; direction
// of citation mostly reflects publication chronology, not community
// structure, so undirected betweenness is the standard choice here.
//
// Returns raw (unnormalized) centrality scores — comparable to each other
// within this graph, not across graphs of different sizes.
func Betweenness(nodes []int64, adj map[int64][]int64) map[int64]float64 {
	centrality := make(map[int64]float64, len(nodes))
	for _, v := range nodes {
		centrality[v] = 0
	}

	for _, s := range nodes {
		stack := make([]int64, 0, len(nodes))
		pred := make(map[int64][]int64, len(nodes))
		sigma := make(map[int64]float64, len(nodes))
		dist := make(map[int64]int, len(nodes))
		for _, v := range nodes {
			sigma[v] = 0
			dist[v] = -1
		}
		sigma[s] = 1
		dist[s] = 0

		queue := []int64{s}
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			stack = append(stack, v)
			for _, w := range adj[v] {
				if dist[w] < 0 {
					dist[w] = dist[v] + 1
					queue = append(queue, w)
				}
				if dist[w] == dist[v]+1 {
					sigma[w] += sigma[v]
					pred[w] = append(pred[w], v)
				}
			}
		}

		delta := make(map[int64]float64, len(nodes))
		for i := len(stack) - 1; i >= 0; i-- {
			w := stack[i]
			for _, v := range pred[w] {
				if sigma[w] != 0 {
					delta[v] += (sigma[v] / sigma[w]) * (1 + delta[w])
				}
			}
			if w != s {
				centrality[w] += delta[w]
			}
		}
	}

	// Each shortest path between a pair is discovered from both endpoints'
	// BFS in an undirected graph, double-counting every pair's contribution.
	// Halve to get the standard betweenness definition.
	for k := range centrality {
		centrality[k] /= 2
	}
	return centrality
}
