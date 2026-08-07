package topics

import (
	"math"
	"math/rand"
)

// kmeans performs spherical K-Means (cosine similarity on L2-normalized
// vectors) with K-Means++ initialization. Deterministic given the same seed.
//
// Centroids are tracked as a dense []float64 (k*dim) during iteration for
// fast dot products against the sparse document vectors, then converted to
// sparse maps on return (most of a TF-IDF centroid is zero).
func kmeans(vectors []map[int]float64, dim, k int, seed int64) (assign []int, centroids []map[int]float64) {
	n := len(vectors)
	if n == 0 || k <= 0 || dim <= 0 {
		return nil, nil
	}
	rng := rand.New(rand.NewSource(seed))

	dense := make([]float64, k*dim)
	initKMeansPlusPlus(vectors, dim, k, rng, dense)

	assign = make([]int, n)
	for i := range assign {
		assign[i] = -1
	}

	const maxIter = 50
	for iter := 0; iter < maxIter; iter++ {
		changed := false
		counts := make([]int, k)
		sums := make([]float64, k*dim)

		for i, vec := range vectors {
			best, bestSim := 0, math.Inf(-1)
			for c := 0; c < k; c++ {
				sim := dotSparseDense(vec, dense[c*dim:(c+1)*dim])
				if sim > bestSim {
					bestSim = sim
					best = c
				}
			}
			if assign[i] != best {
				changed = true
			}
			assign[i] = best
			counts[best]++
			for idx, w := range vec {
				sums[best*dim+idx] += w
			}
		}

		for c := 0; c < k; c++ {
			if counts[c] == 0 {
				reseedEmptyCluster(vectors, dense, c, dim, rng)
				changed = true
				continue
			}
			var norm float64
			for idx := 0; idx < dim; idx++ {
				v := sums[c*dim+idx] / float64(counts[c])
				dense[c*dim+idx] = v
				norm += v * v
			}
			norm = math.Sqrt(norm)
			if norm > 0 {
				for idx := 0; idx < dim; idx++ {
					dense[c*dim+idx] /= norm
				}
			}
		}

		if !changed && iter > 0 {
			break
		}
	}

	centroids = make([]map[int]float64, k)
	for c := 0; c < k; c++ {
		m := make(map[int]float64)
		for idx := 0; idx < dim; idx++ {
			if v := dense[c*dim+idx]; v != 0 {
				m[idx] = v
			}
		}
		centroids[c] = m
	}
	return assign, centroids
}

// initKMeansPlusPlus seeds k centroids using the K-Means++ scheme: each new
// center is picked with probability proportional to its squared cosine
// distance from the nearest already-chosen center, which spreads the
// initial centroids out and gives far more stable clusters than picking k
// random documents.
func initKMeansPlusPlus(vectors []map[int]float64, dim, k int, rng *rand.Rand, dense []float64) {
	n := len(vectors)
	first := rng.Intn(n)
	setDenseFromSparse(dense[0:dim], vectors[first])

	minDist := make([]float64, n)
	for i := range minDist {
		minDist[i] = math.Inf(1)
	}

	for c := 1; c < k; c++ {
		last := dense[(c-1)*dim : c*dim]
		for i, vec := range vectors {
			d := 1 - dotSparseDense(vec, last)
			if d < minDist[i] {
				minDist[i] = d
			}
		}

		var total float64
		for _, d := range minDist {
			if d > 0 {
				total += d * d
			}
		}

		next := n - 1
		if total <= 0 {
			next = rng.Intn(n)
		} else {
			r := rng.Float64() * total
			var cum float64
			for i, d := range minDist {
				if d > 0 {
					cum += d * d
				}
				if cum >= r {
					next = i
					break
				}
			}
		}
		setDenseFromSparse(dense[c*dim:(c+1)*dim], vectors[next])
	}
}

// reseedEmptyCluster replaces a centroid that ended up with no assigned
// documents (can happen with k close to n, or unlucky initialization) with a
// random document's vector, so the caller always gets k non-empty topics.
func reseedEmptyCluster(vectors []map[int]float64, dense []float64, c, dim int, rng *rand.Rand) {
	if len(vectors) == 0 {
		return
	}
	for idx := c * dim; idx < (c+1)*dim; idx++ {
		dense[idx] = 0
	}
	setDenseFromSparse(dense[c*dim:(c+1)*dim], vectors[rng.Intn(len(vectors))])
}

func setDenseFromSparse(dst []float64, sparse map[int]float64) {
	for idx, w := range sparse {
		dst[idx] = w
	}
}

func dotSparseDense(sparse map[int]float64, dense []float64) float64 {
	var sum float64
	for idx, w := range sparse {
		sum += w * dense[idx]
	}
	return sum
}

func dotSparse(a, b map[int]float64) float64 {
	if len(a) > len(b) {
		a, b = b, a
	}
	var sum float64
	for idx, w := range a {
		sum += w * b[idx]
	}
	return sum
}
