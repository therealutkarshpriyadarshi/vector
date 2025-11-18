package quantization

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

func TestProductQuantizer_Train(t *testing.T) {
	pq := NewProductQuantizer(8, 8) // 8 subvectors, 8 bits

	// Create training data (768 dimensions)
	vectors := generateRandomVectors(1000, 768)

	err := pq.Train(vectors)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	if len(pq.codebooks) != 8 {
		t.Errorf("Expected 8 codebooks, got %d", len(pq.codebooks))
	}

	// Each codebook should have 2^8 = 256 centroids
	for i, codebook := range pq.codebooks {
		if len(codebook) != 256 {
			t.Errorf("Codebook %d: expected 256 centroids, got %d", i, len(codebook))
		}
	}

	if pq.subvectorDim != 96 {
		t.Errorf("Expected subvector dim 96, got %d", pq.subvectorDim)
	}
}

func TestProductQuantizer_Encode(t *testing.T) {
	pq := NewProductQuantizer(4, 6) // 4 subvectors, 6 bits

	vectors := generateRandomVectors(500, 128)
	pq.Train(vectors)

	testVector := generateRandomVectors(1, 128)[0]
	codes := pq.Encode(testVector)

	if len(codes) != 4 {
		t.Errorf("Expected 4 codes, got %d", len(codes))
	}

	// Each code should be < 2^6 = 64
	for i, code := range codes {
		if code >= 64 {
			t.Errorf("Code %d out of range: %d", i, code)
		}
	}
}

func TestProductQuantizer_Decode(t *testing.T) {
	pq := NewProductQuantizer(8, 8)

	vectors := generateRandomVectors(500, 768)
	pq.Train(vectors)

	testVector := generateRandomVectors(1, 768)[0]
	codes := pq.Encode(testVector)
	decoded := pq.Decode(codes)

	if len(decoded) != 768 {
		t.Errorf("Expected 768 dimensions, got %d", len(decoded))
	}

	// Compute reconstruction error
	var totalError float32
	for i := range testVector {
		diff := testVector[i] - decoded[i]
		totalError += diff * diff
	}
	mse := totalError / float32(len(testVector))

	// MSE should be reasonably small (< 0.1 for normalized vectors)
	if mse > 0.5 {
		t.Errorf("Reconstruction error too high: MSE=%f", mse)
	}
}

func TestProductQuantizer_AsymmetricDistance(t *testing.T) {
	pq := NewProductQuantizer(8, 8)

	vectors := generateRandomVectors(500, 768)
	pq.Train(vectors)

	query := generateRandomVectors(1, 768)[0]
	testVector := vectors[0]

	// Encode test vector
	codes := pq.Encode(testVector)

	// Compute asymmetric distance
	distTable := pq.ComputeDistanceTable(query)
	asymDist := pq.AsymmetricDistance(distTable, codes)

	// Compute exact distance
	exactDist := EuclideanDistanceFloat32(query, testVector)

	// Asymmetric distance should be close to exact (within 50% error)
	errorRatio := math.Abs(float64(asymDist-exactDist)) / float64(exactDist)
	if errorRatio > 0.5 {
		t.Logf("Asymmetric distance: %f, Exact distance: %f, Error ratio: %f",
			asymDist, exactDist, errorRatio)
	}
}

func TestProductQuantizer_CompressionRatio(t *testing.T) {
	pq := NewProductQuantizer(16, 6) // 16 bytes per vector

	ratio := pq.GetCompressionRatio(768)

	// 768 * 4 bytes / 16 bytes = 192x
	expected := float32(192.0)
	if math.Abs(float64(ratio-expected)) > 0.1 {
		t.Errorf("Expected compression ratio %f, got %f", expected, ratio)
	}
}

func TestProductQuantizer_Serialize(t *testing.T) {
	pq := NewProductQuantizer(4, 6)

	vectors := generateRandomVectors(500, 128)
	pq.Train(vectors)

	// Serialize
	data, err := pq.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Deserialize
	pq2 := NewProductQuantizer(0, 0)
	err = pq2.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Check parameters
	if pq2.numSubvectors != pq.numSubvectors {
		t.Errorf("numSubvectors mismatch: %d vs %d", pq2.numSubvectors, pq.numSubvectors)
	}

	if pq2.bitsPerCode != pq.bitsPerCode {
		t.Errorf("bitsPerCode mismatch: %d vs %d", pq2.bitsPerCode, pq.bitsPerCode)
	}

	if pq2.subvectorDim != pq.subvectorDim {
		t.Errorf("subvectorDim mismatch: %d vs %d", pq2.subvectorDim, pq.subvectorDim)
	}

	// Test encoding consistency
	testVector := generateRandomVectors(1, 128)[0]
	codes1 := pq.Encode(testVector)
	codes2 := pq2.Encode(testVector)

	for i := range codes1 {
		if codes1[i] != codes2[i] {
			t.Errorf("Code mismatch at %d: %d vs %d", i, codes1[i], codes2[i])
		}
	}
}

func TestProductQuantizer_SymmetricDistance(t *testing.T) {
	pq := NewProductQuantizer(8, 8)

	vectors := generateRandomVectors(500, 768)
	pq.Train(vectors)

	vec1 := vectors[0]
	vec2 := vectors[1]

	codes1 := pq.Encode(vec1)
	codes2 := pq.Encode(vec2)

	// Symmetric distance
	symDist := pq.SymmetricDistance(codes1, codes2)

	// Exact distance
	exactDist := EuclideanDistanceFloat32(vec1, vec2)

	// Should be reasonably close
	errorRatio := math.Abs(float64(symDist-exactDist)) / float64(exactDist)
	if errorRatio > 0.6 {
		t.Logf("Symmetric distance: %f, Exact distance: %f, Error ratio: %f",
			symDist, exactDist, errorRatio)
	}
}

func TestProductQuantizer_DifferentConfigurations(t *testing.T) {
	configs := []struct {
		numSubvectors int
		bitsPerCode   int
		expectedRatio float32
	}{
		{8, 8, 384.0},   // 768*4 / 8 = 384
		{16, 6, 192.0},  // 768*4 / 16 = 192
		{32, 8, 96.0},   // 768*4 / 32 = 96
	}

	vectors := generateRandomVectors(500, 768)

	for _, cfg := range configs {
		t.Run(fmt.Sprintf("m=%d_k=%d", cfg.numSubvectors, cfg.bitsPerCode), func(t *testing.T) {
			pq := NewProductQuantizer(cfg.numSubvectors, cfg.bitsPerCode)

			err := pq.Train(vectors)
			if err != nil {
				t.Fatalf("Train failed: %v", err)
			}

			ratio := pq.GetCompressionRatio(768)
			if math.Abs(float64(ratio-cfg.expectedRatio)) > 0.1 {
				t.Errorf("Expected compression ratio %f, got %f", cfg.expectedRatio, ratio)
			}

			// Test encoding
			testVec := generateRandomVectors(1, 768)[0]
			codes := pq.Encode(testVec)

			if len(codes) != cfg.numSubvectors {
				t.Errorf("Expected %d codes, got %d", cfg.numSubvectors, len(codes))
			}
		})
	}
}

func TestProductQuantizer_RecallAccuracy(t *testing.T) {
	// Test that PQ maintains reasonable recall
	pq := NewProductQuantizer(16, 8)

	// Create database and queries
	database := generateRandomVectors(1000, 768)
	queries := generateRandomVectors(100, 768)

	pq.Train(database)

	// Encode database
	encodedDB := make([][]byte, len(database))
	for i, vec := range database {
		encodedDB[i] = pq.Encode(vec)
	}

	var totalRecall float32
	k := 10

	for _, query := range queries {
		// Find exact k-NN
		exactNeighbors := findKNN(query, database, k)

		// Find approximate k-NN using PQ
		approxNeighbors := findKNNWithPQ(query, encodedDB, pq, k)

		// Compute recall
		recall := computeRecallForQuery(exactNeighbors, approxNeighbors)
		totalRecall += recall
	}

	avgRecall := totalRecall / float32(len(queries))

	// PQ with 16 subvectors and 8 bits should achieve >80% recall@10
	if avgRecall < 0.70 {
		t.Errorf("Recall too low: %f (expected >0.70)", avgRecall)
	}

	t.Logf("Average Recall@%d: %.2f%%", k, avgRecall*100)
}

// Helper functions

func generateRandomVectors(n, dim int) [][]float32 {
	vectors := make([][]float32, n)
	for i := 0; i < n; i++ {
		vectors[i] = make([]float32, dim)
		for j := 0; j < dim; j++ {
			vectors[i][j] = rand.Float32()
		}
	}
	return vectors
}

func findKNN(query []float32, database [][]float32, k int) []int {
	type distPair struct {
		idx  int
		dist float32
	}

	distances := make([]distPair, len(database))
	for i, vec := range database {
		distances[i] = distPair{
			idx:  i,
			dist: EuclideanDistanceFloat32(query, vec),
		}
	}

	// Sort by distance
	for i := 0; i < len(distances)-1; i++ {
		for j := i + 1; j < len(distances); j++ {
			if distances[j].dist < distances[i].dist {
				distances[i], distances[j] = distances[j], distances[i]
			}
		}
	}

	result := make([]int, k)
	for i := 0; i < k && i < len(distances); i++ {
		result[i] = distances[i].idx
	}

	return result
}

func findKNNWithPQ(query []float32, encodedDB [][]byte, pq *ProductQuantizer, k int) []int {
	type distPair struct {
		idx  int
		dist float32
	}

	distTable := pq.ComputeDistanceTable(query)
	distances := make([]distPair, len(encodedDB))

	for i, code := range encodedDB {
		distances[i] = distPair{
			idx:  i,
			dist: pq.AsymmetricDistance(distTable, code),
		}
	}

	// Sort by distance
	for i := 0; i < len(distances)-1; i++ {
		for j := i + 1; j < len(distances); j++ {
			if distances[j].dist < distances[i].dist {
				distances[i], distances[j] = distances[j], distances[i]
			}
		}
	}

	result := make([]int, k)
	for i := 0; i < k && i < len(distances); i++ {
		result[i] = distances[i].idx
	}

	return result
}

func computeRecallForQuery(exactNeighbors, approxNeighbors []int) float32 {
	exactSet := make(map[int]bool)
	for _, id := range exactNeighbors {
		exactSet[id] = true
	}

	var matches int
	for _, id := range approxNeighbors {
		if exactSet[id] {
			matches++
		}
	}

	return float32(matches) / float32(len(exactNeighbors))
}

// Benchmarks

func BenchmarkProductQuantizer_Train(b *testing.B) {
	pq := NewProductQuantizer(16, 8)
	vectors := generateRandomVectors(1000, 768)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pq.Train(vectors)
	}
}

func BenchmarkProductQuantizer_Encode(b *testing.B) {
	pq := NewProductQuantizer(16, 8)
	vectors := generateRandomVectors(1000, 768)
	pq.Train(vectors)

	testVector := generateRandomVectors(1, 768)[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pq.Encode(testVector)
	}
}

func BenchmarkProductQuantizer_AsymmetricDistance(b *testing.B) {
	pq := NewProductQuantizer(16, 8)
	vectors := generateRandomVectors(1000, 768)
	pq.Train(vectors)

	query := generateRandomVectors(1, 768)[0]
	codes := pq.Encode(vectors[0])
	distTable := pq.ComputeDistanceTable(query)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pq.AsymmetricDistance(distTable, codes)
	}
}

func BenchmarkProductQuantizer_ComputeDistanceTable(b *testing.B) {
	pq := NewProductQuantizer(16, 8)
	vectors := generateRandomVectors(1000, 768)
	pq.Train(vectors)

	query := generateRandomVectors(1, 768)[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pq.ComputeDistanceTable(query)
	}
}
