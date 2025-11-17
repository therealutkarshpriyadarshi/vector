package hnsw

import (
	"math"
)

// DistanceFunc is a function type for calculating distance between two vectors
type DistanceFunc func(a, b []float32) float32

// CosineSimilarity calculates the cosine similarity between two vectors
// Returns 1 - cosine similarity to make it a distance metric (0 = identical, 2 = opposite)
// Formula: 1 - (a·b) / (||a|| * ||b||)
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		panic("vectors must have the same dimension")
	}

	var dotProduct, normA, normB float32

	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	// Avoid division by zero
	if normA == 0 || normB == 0 {
		return 1.0
	}

	normA = float32(math.Sqrt(float64(normA)))
	normB = float32(math.Sqrt(float64(normB)))

	similarity := dotProduct / (normA * normB)

	// Convert similarity to distance: higher distance = less similar
	return 1.0 - similarity
}

// EuclideanDistance calculates the Euclidean (L2) distance between two vectors
// Formula: sqrt(Σ(a[i] - b[i])²)
func EuclideanDistance(a, b []float32) float32 {
	if len(a) != len(b) {
		panic("vectors must have the same dimension")
	}

	var sum float32

	for i := 0; i < len(a); i++ {
		diff := a[i] - b[i]
		sum += diff * diff
	}

	return float32(math.Sqrt(float64(sum)))
}

// DotProduct calculates the negative dot product between two vectors
// Returns negative value because HNSW minimizes distance (lower = better)
// Formula: -(a·b)
func DotProduct(a, b []float32) float32 {
	if len(a) != len(b) {
		panic("vectors must have the same dimension")
	}

	var sum float32

	for i := 0; i < len(a); i++ {
		sum += a[i] * b[i]
	}

	// Negate so that higher similarity = lower distance
	return -sum
}

// SquaredEuclideanDistance calculates the squared Euclidean distance
// Faster than EuclideanDistance since it skips the sqrt operation
// Formula: Σ(a[i] - b[i])²
func SquaredEuclideanDistance(a, b []float32) float32 {
	if len(a) != len(b) {
		panic("vectors must have the same dimension")
	}

	var sum float32

	for i := 0; i < len(a); i++ {
		diff := a[i] - b[i]
		sum += diff * diff
	}

	return sum
}
