package quantization

import (
	"fmt"
	"math"
	"math/rand"
)

// EuclideanDistanceFloat32 computes Euclidean distance between two float32 vectors
func EuclideanDistanceFloat32(a, b []float32) float32 {
	var sum float32
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return float32(math.Sqrt(float64(sum)))
}

// CosineDistanceFloat32 computes cosine distance (1 - cosine similarity)
func CosineDistanceFloat32(a, b []float32) float32 {
	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	normA = float32(math.Sqrt(float64(normA)))
	normB = float32(math.Sqrt(float64(normB)))

	if normA == 0 || normB == 0 {
		return 1.0 // Maximum distance
	}

	cosineSim := dotProduct / (normA * normB)
	return 1.0 - cosineSim
}

// DotProductFloat32 computes dot product between two vectors
func DotProductFloat32(a, b []float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// NormL2 computes L2 norm of a vector
func NormL2(v []float32) float32 {
	var sum float32
	for _, x := range v {
		sum += x * x
	}
	return float32(math.Sqrt(float64(sum)))
}

// Normalize normalizes a vector to unit length
func Normalize(v []float32) []float32 {
	norm := NormL2(v)
	if norm == 0 {
		return v
	}
	result := make([]float32, len(v))
	for i, x := range v {
		result[i] = x / norm
	}
	return result
}

// KMeansPlusPlus performs k-means clustering with k-means++ initialization
// This provides better initialization than random selection
func KMeansPlusPlus(vectors [][]float32, k int, config *QuantizationConfig) ([][]float32, error) {
	if len(vectors) < k {
		return nil, fmt.Errorf("not enough vectors (%d) for %d clusters", len(vectors), k)
	}

	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return nil, fmt.Errorf("empty vectors")
	}

	dim := len(vectors[0])
	centroids := make([][]float32, k)

	// Use provided random seed for reproducibility
	r := rand.New(rand.NewSource(config.RandomSeed))

	// Step 1: Choose first centroid randomly
	firstIdx := r.Intn(len(vectors))
	centroids[0] = make([]float32, dim)
	copy(centroids[0], vectors[firstIdx])

	// Step 2: Choose remaining centroids using k-means++ algorithm
	for c := 1; c < k; c++ {
		// Compute distance to nearest centroid for each vector
		distances := make([]float32, len(vectors))
		var totalDist float32

		for i, vec := range vectors {
			minDist := float32(math.MaxFloat32)

			// Find distance to nearest existing centroid
			for j := 0; j < c; j++ {
				var dist float32
				switch config.DistanceMetric {
				case EuclideanDistance:
					dist = EuclideanDistanceFloat32(vec, centroids[j])
				case CosineDistance:
					dist = CosineDistanceFloat32(vec, centroids[j])
				case DotProductDistance:
					dist = -DotProductFloat32(vec, centroids[j])
				}

				if dist < minDist {
					minDist = dist
				}
			}

			distances[i] = minDist * minDist // Square for probability weighting
			totalDist += distances[i]
		}

		// Choose next centroid with probability proportional to squared distance
		if totalDist > 0 {
			target := r.Float32() * totalDist
			var cumulative float32

			for i, dist := range distances {
				cumulative += dist
				if cumulative >= target {
					centroids[c] = make([]float32, dim)
					copy(centroids[c], vectors[i])
					break
				}
			}
		} else {
			// Fallback: choose random vector
			idx := r.Intn(len(vectors))
			centroids[c] = make([]float32, dim)
			copy(centroids[c], vectors[idx])
		}
	}

	// Step 3: Run standard k-means iterations
	for iter := 0; iter < config.NumIterations; iter++ {
		// Assign vectors to clusters
		clusters := make([][][]float32, k)

		for _, vec := range vectors {
			minDist := float32(math.MaxFloat32)
			minCluster := 0

			for c, centroid := range centroids {
				var dist float32
				switch config.DistanceMetric {
				case EuclideanDistance:
					dist = EuclideanDistanceFloat32(vec, centroid)
				case CosineDistance:
					dist = CosineDistanceFloat32(vec, centroid)
				case DotProductDistance:
					dist = -DotProductFloat32(vec, centroid)
				}

				if dist < minDist {
					minDist = dist
					minCluster = c
				}
			}

			clusters[minCluster] = append(clusters[minCluster], vec)
		}

		// Update centroids
		converged := true
		for c := range centroids {
			if len(clusters[c]) == 0 {
				continue // Keep old centroid if cluster is empty
			}

			// Compute new centroid as mean of cluster
			newCentroid := make([]float32, dim)
			for _, vec := range clusters[c] {
				for d := 0; d < dim; d++ {
					newCentroid[d] += vec[d]
				}
			}

			for d := 0; d < dim; d++ {
				newCentroid[d] /= float32(len(clusters[c]))
			}

			// Check for convergence
			if EuclideanDistanceFloat32(centroids[c], newCentroid) > 1e-6 {
				converged = false
			}

			centroids[c] = newCentroid
		}

		if converged && config.Verbose {
			fmt.Printf("K-means converged at iteration %d\n", iter)
			break
		}
	}

	return centroids, nil
}

// ComputeRecall computes recall@k for approximate search results
func ComputeRecall(groundTruth [][]int, results [][]int, k int) float32 {
	if len(groundTruth) != len(results) {
		return 0
	}

	var totalRecall float32
	for i := range groundTruth {
		gt := groundTruth[i]
		res := results[i]

		if len(gt) == 0 {
			continue
		}

		// Limit to top-k
		if len(gt) > k {
			gt = gt[:k]
		}
		if len(res) > k {
			res = res[:k]
		}

		// Count matches
		gtSet := make(map[int]bool)
		for _, id := range gt {
			gtSet[id] = true
		}

		var matches int
		for _, id := range res {
			if gtSet[id] {
				matches++
			}
		}

		recall := float32(matches) / float32(len(gt))
		totalRecall += recall
	}

	return totalRecall / float32(len(groundTruth))
}

// VectorStats computes statistics for a set of vectors
type VectorStats struct {
	Mean   []float32
	StdDev []float32
	Min    []float32
	Max    []float32
}

// ComputeVectorStats computes statistics for training data
func ComputeVectorStats(vectors [][]float32) *VectorStats {
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return nil
	}

	dim := len(vectors[0])
	stats := &VectorStats{
		Mean:   make([]float32, dim),
		StdDev: make([]float32, dim),
		Min:    make([]float32, dim),
		Max:    make([]float32, dim),
	}

	// Initialize min/max
	for d := 0; d < dim; d++ {
		stats.Min[d] = float32(math.MaxFloat32)
		stats.Max[d] = float32(-math.MaxFloat32)
	}

	// Compute mean, min, max
	for _, vec := range vectors {
		for d := 0; d < dim; d++ {
			stats.Mean[d] += vec[d]
			if vec[d] < stats.Min[d] {
				stats.Min[d] = vec[d]
			}
			if vec[d] > stats.Max[d] {
				stats.Max[d] = vec[d]
			}
		}
	}

	for d := 0; d < dim; d++ {
		stats.Mean[d] /= float32(len(vectors))
	}

	// Compute standard deviation
	for _, vec := range vectors {
		for d := 0; d < dim; d++ {
			diff := vec[d] - stats.Mean[d]
			stats.StdDev[d] += diff * diff
		}
	}

	for d := 0; d < dim; d++ {
		stats.StdDev[d] = float32(math.Sqrt(float64(stats.StdDev[d] / float32(len(vectors)))))
	}

	return stats
}
