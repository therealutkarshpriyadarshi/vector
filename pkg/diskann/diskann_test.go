package diskann

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
)

// generateRandomVectors generates random vectors for testing
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

// normalizeVector normalizes a vector to unit length
func normalizeVector(v []float32) {
	var norm float32
	for _, val := range v {
		norm += val * val
	}
	norm = float32(1.0 / (norm + 1e-10))
	for i := range v {
		v[i] *= norm
	}
}

// TestDiskANN_BuildAndSearch tests basic build and search functionality
func TestDiskANN_BuildAndSearch(t *testing.T) {
	// Create temporary directory for test data
	tmpDir := "/tmp/diskann_test"
	os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	// Create index
	config := IndexConfig{
		R:               32,
		L:               50,
		BeamWidth:       4,
		Alpha:           1.2,
		DistanceFunc:    CosineSimilarity,
		DataPath:        tmpDir,
		NumSubvectors:   8,
		BitsPerCode:     8,
		MemoryGraphSize: 500, // Keep half in memory
	}

	idx, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	defer idx.Close()

	// Generate test data
	numVectors := 1000
	dim := 128
	vectors := generateRandomVectors(numVectors, dim)

	// Normalize vectors for cosine similarity
	for i := range vectors {
		normalizeVector(vectors[i])
	}

	// Add vectors
	t.Logf("Adding %d vectors...", numVectors)
	for i, vec := range vectors {
		metadata := map[string]interface{}{
			"id":   i,
			"data": fmt.Sprintf("vector_%d", i),
		}
		_, err := idx.AddVector(vec, metadata)
		if err != nil {
			t.Fatalf("Failed to add vector %d: %v", i, err)
		}
	}

	// Build index
	t.Logf("Building index...")
	if err := idx.Build(); err != nil {
		t.Fatalf("Failed to build index: %v", err)
	}

	// Verify index state
	if !idx.IsBuilt() {
		t.Fatal("Index should be built")
	}

	if idx.Size() != int64(numVectors) {
		t.Fatalf("Expected size %d, got %d", numVectors, idx.Size())
	}

	// Test search
	t.Logf("Testing search...")
	k := 10
	query := vectors[0] // Use first vector as query

	results, err := idx.Search(query, k)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected non-empty results")
	}

	// First result should be the query vector itself (distance ~0)
	if results[0].Distance > 0.01 {
		t.Errorf("Expected first result to be query itself (distance ~0), got %f", results[0].Distance)
	}

	t.Logf("Search returned %d results", len(results))
	t.Logf("Top result: ID=%d, Distance=%f", results[0].ID, results[0].Distance)
}

// TestDiskANN_Recall tests recall accuracy
func TestDiskANN_Recall(t *testing.T) {
	// Create temporary directory for test data
	tmpDir := "/tmp/diskann_recall_test"
	os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	// Create index
	config := IndexConfig{
		R:               64,
		L:               100,
		BeamWidth:       8,
		Alpha:           1.2,
		DistanceFunc:    EuclideanDistance,
		DataPath:        tmpDir,
		NumSubvectors:   16,
		BitsPerCode:     8,
		MemoryGraphSize: 200,
	}

	idx, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	defer idx.Close()

	// Generate test data
	numVectors := 500
	dim := 128
	vectors := generateRandomVectors(numVectors, dim)

	// Add vectors
	for i, vec := range vectors {
		_, err := idx.AddVector(vec, map[string]interface{}{"id": i})
		if err != nil {
			t.Fatalf("Failed to add vector %d: %v", i, err)
		}
	}

	// Build index
	if err := idx.Build(); err != nil {
		t.Fatalf("Failed to build index: %v", err)
	}

	// Test recall on random queries
	numQueries := 10
	k := 10
	totalRecall := 0.0

	for q := 0; q < numQueries; q++ {
		query := vectors[rand.Intn(numVectors)]

		// Get DiskANN results
		results, err := idx.Search(query, k)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Get ground truth (brute force)
		groundTruth := bruteForceSearch(query, vectors, k, EuclideanDistance)

		// Calculate recall
		recall := calculateRecall(results, groundTruth)
		totalRecall += recall
	}

	avgRecall := totalRecall / float64(numQueries)
	t.Logf("Average recall@%d: %.2f%%", k, avgRecall*100)

	// DiskANN should achieve at least 70% recall (lower than HNSW due to disk storage and PQ compression)
	if avgRecall < 0.70 {
		t.Errorf("Expected recall >= 70%%, got %.2f%%", avgRecall*100)
	}
}

// TestDiskANN_EmptyIndex tests operations on empty index
func TestDiskANN_EmptyIndex(t *testing.T) {
	tmpDir := "/tmp/diskann_empty_test"
	os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	config := DefaultConfig()
	config.DataPath = tmpDir

	idx, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	defer idx.Close()

	// Try to build empty index
	err = idx.Build()
	if err == nil {
		t.Error("Expected error when building empty index")
	}

	// Try to search empty index
	query := []float32{1.0, 2.0, 3.0}
	_, err = idx.Search(query, 10)
	if err == nil {
		t.Error("Expected error when searching unbuilt index")
	}
}

// TestDiskANN_DimensionMismatch tests dimension mismatch handling
func TestDiskANN_DimensionMismatch(t *testing.T) {
	tmpDir := "/tmp/diskann_dim_test"
	os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	config := DefaultConfig()
	config.DataPath = tmpDir
	config.NumSubvectors = 8

	idx, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	defer idx.Close()

	// Add vector with dimension 128
	vec1 := generateRandomVectors(1, 128)[0]
	_, err = idx.AddVector(vec1, nil)
	if err != nil {
		t.Fatalf("Failed to add first vector: %v", err)
	}

	// Try to add vector with different dimension
	vec2 := generateRandomVectors(1, 256)[0]
	_, err = idx.AddVector(vec2, nil)
	if err == nil {
		t.Error("Expected error when adding vector with different dimension")
	}
}

// Helper functions

// bruteForceSearch performs brute force search for ground truth
func bruteForceSearch(query []float32, vectors [][]float32, k int, distFunc DistanceFunc) []uint64 {
	type result struct {
		id   uint64
		dist float32
	}

	results := make([]result, len(vectors))
	for i, vec := range vectors {
		results[i] = result{
			id:   uint64(i),
			dist: distFunc(query, vec),
		}
	}

	// Sort by distance
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].dist < results[i].dist {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Return top k IDs
	topK := make([]uint64, min(k, len(results)))
	for i := range topK {
		topK[i] = results[i].id
	}

	return topK
}

// calculateRecall calculates recall between search results and ground truth
func calculateRecall(results []SearchResult, groundTruth []uint64) float64 {
	if len(groundTruth) == 0 {
		return 0.0
	}

	// Convert results to set
	resultSet := make(map[uint64]bool)
	for _, r := range results {
		resultSet[r.ID] = true
	}

	// Count matches
	matches := 0
	for _, id := range groundTruth {
		if resultSet[id] {
			matches++
		}
	}

	return float64(matches) / float64(len(groundTruth))
}

// TestDiskANN_LargeScale tests DiskANN on larger dataset
func TestDiskANN_LargeScale(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large-scale test in short mode")
	}

	tmpDir := "/tmp/diskann_large_test"
	os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	config := IndexConfig{
		R:               64,
		L:               100,
		BeamWidth:       8,
		Alpha:           1.2,
		DistanceFunc:    CosineSimilarity,
		DataPath:        tmpDir,
		NumSubvectors:   16,
		BitsPerCode:     8,
		MemoryGraphSize: 5000, // 5k nodes in memory
	}

	idx, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	defer idx.Close()

	// Generate larger dataset
	numVectors := 10000
	dim := 128
	t.Logf("Generating %d vectors with dimension %d...", numVectors, dim)
	vectors := generateRandomVectors(numVectors, dim)

	// Normalize for cosine similarity
	for i := range vectors {
		normalizeVector(vectors[i])
	}

	// Add vectors
	t.Logf("Adding vectors...")
	for i, vec := range vectors {
		if i%1000 == 0 {
			t.Logf("  Added %d/%d vectors", i, numVectors)
		}
		_, err := idx.AddVector(vec, map[string]interface{}{"id": i})
		if err != nil {
			t.Fatalf("Failed to add vector %d: %v", i, err)
		}
	}

	// Build index
	t.Logf("Building index...")
	if err := idx.Build(); err != nil {
		t.Fatalf("Failed to build index: %v", err)
	}

	t.Logf("Index built: %d total nodes, %d in memory, %d on disk",
		idx.Size(), idx.memoryGraph.Size(), idx.diskGraph.Size())

	// Test search performance
	numQueries := 100
	k := 10
	t.Logf("Running %d queries...", numQueries)

	for q := 0; q < numQueries; q++ {
		query := vectors[rand.Intn(numVectors)]
		results, err := idx.Search(query, k)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) == 0 {
			t.Error("Expected non-empty results")
		}
	}

	t.Logf("Large-scale test completed successfully")
}
