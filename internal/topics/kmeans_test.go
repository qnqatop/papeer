package topics

import (
	"math"
	"testing"
)

// makeSparse builds a sparse L2-normalized vector from a dense slice, for
// convenient synthetic test data.
func makeSparse(dense []float64) map[int]float64 {
	var norm float64
	for _, v := range dense {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	out := make(map[int]float64, len(dense))
	for i, v := range dense {
		if v == 0 {
			continue
		}
		if norm > 0 {
			v /= norm
		}
		out[i] = v
	}
	return out
}

func TestKmeans_EmptyInputs(t *testing.T) {
	if assign, centroids := kmeans(nil, 3, 2, 1); assign != nil || centroids != nil {
		t.Errorf("kmeans(nil vectors) = %v, %v, want nil, nil", assign, centroids)
	}
	vectors := []map[int]float64{{0: 1}}
	if assign, centroids := kmeans(vectors, 3, 0, 1); assign != nil || centroids != nil {
		t.Errorf("kmeans(k=0) = %v, %v, want nil, nil", assign, centroids)
	}
	if assign, centroids := kmeans(vectors, 0, 1, 1); assign != nil || centroids != nil {
		t.Errorf("kmeans(dim=0) = %v, %v, want nil, nil", assign, centroids)
	}
}

func TestKmeans_SeparatesTwoObviousClusters(t *testing.T) {
	dim := 4
	// Two tight, well-separated groups in a 4-dim space: {e0,e0+noise} vs {e2,e2+noise}.
	vectors := []map[int]float64{
		makeSparse([]float64{1, 0.05, 0, 0}),
		makeSparse([]float64{0.95, 0.1, 0, 0}),
		makeSparse([]float64{1, 0, 0.05, 0}),
		makeSparse([]float64{0, 0, 1, 0.05}),
		makeSparse([]float64{0, 0, 0.95, 0.1}),
		makeSparse([]float64{0.05, 0, 1, 0}),
	}
	assign, centroids := kmeans(vectors, dim, 2, 42)
	if len(assign) != len(vectors) {
		t.Fatalf("len(assign) = %d, want %d", len(assign), len(vectors))
	}
	if len(centroids) != 2 {
		t.Fatalf("len(centroids) = %d, want 2", len(centroids))
	}

	firstGroup := assign[0]
	for i := 0; i < 3; i++ {
		if assign[i] != firstGroup {
			t.Errorf("assign[%d] = %d, want %d (first group)", i, assign[i], firstGroup)
		}
	}
	secondGroup := assign[3]
	if secondGroup == firstGroup {
		t.Fatalf("both groups assigned to the same cluster: %v", assign)
	}
	for i := 3; i < 6; i++ {
		if assign[i] != secondGroup {
			t.Errorf("assign[%d] = %d, want %d (second group)", i, assign[i], secondGroup)
		}
	}
}

func TestKmeans_DeterministicWithFixedSeed(t *testing.T) {
	dim := 4
	vectors := []map[int]float64{
		makeSparse([]float64{1, 0, 0, 0}),
		makeSparse([]float64{0.9, 0.1, 0, 0}),
		makeSparse([]float64{0, 1, 0, 0}),
		makeSparse([]float64{0, 0.9, 0.1, 0}),
		makeSparse([]float64{0, 0, 1, 0}),
	}
	assign1, _ := kmeans(vectors, dim, 3, 42)
	assign2, _ := kmeans(vectors, dim, 3, 42)
	for i := range assign1 {
		if assign1[i] != assign2[i] {
			t.Errorf("assign differs between runs with same seed at index %d: %d vs %d", i, assign1[i], assign2[i])
		}
	}
}

// TestKmeans_ReseedsEmptyClusters exercises k > n (more clusters requested
// than documents), which forces at least one cluster to start (or end up)
// empty; kmeans must reseed it rather than leave a nil/degenerate centroid.
func TestKmeans_ReseedsEmptyClusters(t *testing.T) {
	dim := 3
	vectors := []map[int]float64{
		makeSparse([]float64{1, 0, 0}),
		makeSparse([]float64{0, 1, 0}),
	}
	assign, centroids := kmeans(vectors, dim, 5, 7)
	if len(centroids) != 5 {
		t.Fatalf("len(centroids) = %d, want 5", len(centroids))
	}
	for i, c := range assign {
		if c < 0 || c >= 5 {
			t.Errorf("assign[%d] = %d out of range [0,5)", i, c)
		}
	}
	// No panics and no centroid map is nil.
	for i, c := range centroids {
		if c == nil {
			t.Errorf("centroid %d is nil", i)
		}
	}
}

func TestDotSparse(t *testing.T) {
	a := map[int]float64{0: 1, 1: 2}
	b := map[int]float64{1: 3, 2: 4}
	got := dotSparse(a, b)
	want := 2.0 * 3.0 // only overlapping index 1
	if got != want {
		t.Errorf("dotSparse = %v, want %v", got, want)
	}
	// Symmetric regardless of argument order (implementation swaps to iterate
	// over the shorter map).
	got2 := dotSparse(b, a)
	if got2 != want {
		t.Errorf("dotSparse(b,a) = %v, want %v", got2, want)
	}
}
