package topics

import "math"

// layoutCentroids assigns 2D scatter coordinates to topics via PCA on the
// cluster centroids. Because the number of clusters (rows, ≤ MaxK) is far
// smaller than the vocabulary size (columns, possibly thousands), PCA is
// computed through the small K×K Gram-matrix trick instead of a D×D
// covariance matrix: for a centered matrix M (K×D), the eigenvectors of
// M·Mᵀ (K×K) give the same principal-axis scores as a full PCA on M, at a
// fraction of the cost.
func layoutCentroids(topics []Topic, centroids []map[int]float64) {
	k := len(centroids)
	if k == 0 {
		return
	}
	if k == 1 {
		topics[0].X, topics[0].Y = 0, 0
		return
	}

	mean := make(map[int]float64)
	for _, c := range centroids {
		for idx, w := range c {
			mean[idx] += w / float64(k)
		}
	}
	centered := make([]map[int]float64, k)
	for i, c := range centroids {
		cm := make(map[int]float64, len(c))
		for idx, w := range c {
			cm[idx] = w - mean[idx]
		}
		for idx, m := range mean {
			if _, ok := c[idx]; !ok {
				cm[idx] = -m
			}
		}
		centered[i] = cm
	}

	gram := make([][]float64, k)
	for i := range gram {
		gram[i] = make([]float64, k)
	}
	for i := 0; i < k; i++ {
		for j := i; j < k; j++ {
			v := dotSparse(centered[i], centered[j])
			gram[i][j] = v
			gram[j][i] = v
		}
	}

	pc1, val1 := powerIteration(gram)
	gram2 := deflate(gram, pc1, val1)
	pc2, val2 := powerIteration(gram2)

	scale1 := math.Sqrt(math.Max(val1, 0))
	scale2 := math.Sqrt(math.Max(val2, 0))
	for i := range topics {
		topics[i].X = pc1[i] * scale1
		topics[i].Y = pc2[i] * scale2
	}
}

// powerIteration finds the dominant eigenvector/eigenvalue of a symmetric
// matrix. 200 iterations is far more than needed for K ≤ 15.
func powerIteration(m [][]float64) (vec []float64, eigenvalue float64) {
	k := len(m)
	v := make([]float64, k)
	// A uniform start ([1,1,...,1]) is exactly the null vector of any
	// mean-centered Gram matrix (centered rows sum to zero, so Gram·1 = 0),
	// which would make power iteration converge to nothing on the very
	// first step. A linearly increasing seed is never parallel to that (or
	// any other single) eigenvector, so it always has a component along the
	// dominant one.
	for i := range v {
		v[i] = float64(i + 1)
	}
	normalizeVec(v)

	for iter := 0; iter < 200; iter++ {
		nv := matVec(m, v)
		norm := vecNorm(nv)
		if norm < 1e-12 {
			return v, 0
		}
		for i := range nv {
			nv[i] /= norm
		}
		eigenvalue = norm
		v = nv
	}
	return v, eigenvalue
}

// deflate removes the contribution of eigenpair (v, eigenvalue) from m
// (Hotelling's deflation) so a subsequent power iteration finds the next
// dominant eigenvector.
func deflate(m [][]float64, v []float64, eigenvalue float64) [][]float64 {
	k := len(m)
	out := make([][]float64, k)
	for i := range out {
		out[i] = make([]float64, k)
		for j := range out[i] {
			out[i][j] = m[i][j] - eigenvalue*v[i]*v[j]
		}
	}
	return out
}

func matVec(m [][]float64, v []float64) []float64 {
	out := make([]float64, len(m))
	for i, row := range m {
		var sum float64
		for j, x := range row {
			sum += x * v[j]
		}
		out[i] = sum
	}
	return out
}

func vecNorm(v []float64) float64 {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	return math.Sqrt(sum)
}

func normalizeVec(v []float64) {
	norm := vecNorm(v)
	if norm == 0 {
		return
	}
	for i := range v {
		v[i] /= norm
	}
}
