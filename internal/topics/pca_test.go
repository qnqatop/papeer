package topics

import (
	"math"
	"testing"
)

// TestPowerIteration_MeanCenteredNullSpace is a regression test for a
// degenerate-PCA bug: for ANY mean-centered matrix M (rows summing to zero
// per column), the all-ones vector is exactly a null vector of the Gram
// matrix M·Mᵀ, since Gram·1 = M·(Mᵀ·1) = M·0 = 0. Seeding power iteration
// with a uniform starting vector [1,1,...,1] therefore starts (and stays)
// exactly in the null space, converging to a zero eigenvalue/vector on the
// very first step instead of finding the dominant eigenvector. The fix uses
// a linearly-increasing seed vector instead, which is never parallel to the
// null space. This test builds exactly such a mean-centered Gram matrix and
// asserts a non-degenerate result.
func TestPowerIteration_MeanCenteredNullSpace(t *testing.T) {
	// 3 points in 2D whose centroid is the origin (already mean-centered).
	centered := [][]float64{
		{2, 0},
		{-1, 1},
		{-1, -1},
	}
	k := len(centered)
	gram := make([][]float64, k)
	for i := range gram {
		gram[i] = make([]float64, k)
	}
	for i := 0; i < k; i++ {
		for j := 0; j < k; j++ {
			var s float64
			for d := range centered[i] {
				s += centered[i][d] * centered[j][d]
			}
			gram[i][j] = s
		}
	}

	// Sanity-check the premise: Gram·1 must be (numerically) the zero vector.
	ones := []float64{1, 1, 1}
	nullCheck := matVec(gram, ones)
	for _, v := range nullCheck {
		if math.Abs(v) > 1e-9 {
			t.Fatalf("test setup invalid: Gram·1 = %v, want ~0", nullCheck)
		}
	}

	vec, eigenvalue := powerIteration(gram)
	if eigenvalue <= 1e-9 {
		t.Fatalf("powerIteration returned degenerate eigenvalue %v (bug: stuck in null space)", eigenvalue)
	}
	for i, v := range vec {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("powerIteration eigenvector[%d] = %v, want finite", i, v)
		}
	}
}

func TestLayoutCentroids_NoClusters(t *testing.T) {
	topics := []Topic{}
	layoutCentroids(topics, nil)
	// Should not panic; nothing to assert on empty input.
}

func TestLayoutCentroids_SingleClusterAtOrigin(t *testing.T) {
	topics := make([]Topic, 1)
	centroids := []map[int]float64{{0: 1, 1: 0.5}}
	layoutCentroids(topics, centroids)
	if topics[0].X != 0 || topics[0].Y != 0 {
		t.Errorf("single cluster layout = (%v,%v), want (0,0)", topics[0].X, topics[0].Y)
	}
}

func TestLayoutCentroids_MultipleClustersFinite(t *testing.T) {
	topics := make([]Topic, 4)
	centroids := []map[int]float64{
		{0: 1, 1: 0},
		{0: 0, 1: 1},
		{0: -1, 1: 0},
		{0: 0, 1: -1},
	}
	layoutCentroids(topics, centroids)
	for i, topic := range topics {
		if math.IsNaN(topic.X) || math.IsInf(topic.X, 0) {
			t.Errorf("topic[%d].X = %v, want finite", i, topic.X)
		}
		if math.IsNaN(topic.Y) || math.IsInf(topic.Y, 0) {
			t.Errorf("topic[%d].Y = %v, want finite", i, topic.Y)
		}
	}
	// Not all points should collapse to the exact same coordinate — some
	// meaningful spread is expected given 4 well-separated input centroids.
	allSame := true
	for i := 1; i < len(topics); i++ {
		if topics[i].X != topics[0].X || topics[i].Y != topics[0].Y {
			allSame = false
		}
	}
	if allSame {
		t.Errorf("all topics collapsed to the same coordinate: %+v", topics)
	}
}

func TestDeflate_RemovesDominantEigenpair(t *testing.T) {
	m := [][]float64{
		{2, 0},
		{0, 0},
	}
	v := []float64{1, 0}
	deflated := deflate(m, v, 2)
	for i := range deflated {
		for j := range deflated[i] {
			if math.Abs(deflated[i][j]) > 1e-9 {
				t.Errorf("deflate result[%d][%d] = %v, want ~0", i, j, deflated[i][j])
			}
		}
	}
}

func TestNormalizeVec(t *testing.T) {
	v := []float64{3, 4}
	normalizeVec(v)
	if math.Abs(vecNorm(v)-1) > 1e-9 {
		t.Errorf("normalizeVec: norm = %v, want 1", vecNorm(v))
	}

	// Zero vector: normalizeVec must not divide by zero / produce NaN.
	z := []float64{0, 0}
	normalizeVec(z)
	for _, x := range z {
		if math.IsNaN(x) {
			t.Errorf("normalizeVec(zero vector) produced NaN")
		}
	}
}
