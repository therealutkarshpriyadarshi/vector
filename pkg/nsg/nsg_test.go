package nsg

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

func TestNewIndex(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	if idx == nil {
		t.Fatal("Expected non-nil index")
	}

	if idx.R != 16 {
		t.Errorf("Expected R=16, got %d", idx.R)
	}

	if idx.L != 100 {
		t.Errorf("Expected L=100, got %d", idx.L)
	}

	if idx.C != 500 {
		t.Errorf("Expected C=500, got %d", idx.C)
	}

	if idx.isBuilt {
		t.Error("New index should not be built")
	}
}

func TestAddVector(t *testing.T) {
	idx := New(DefaultConfig())

	vec := []float32{0.1, 0.2, 0.3, 0.4}
	id, err := idx.AddVector(vec)

	if err != nil {
		t.Fatalf("AddVector failed: %v", err)
	}

	if id != 0 {
		t.Errorf("Expected first ID to be 0, got %d", id)
	}

	if idx.Size() != 1 {
		t.Errorf("Expected size 1, got %d", idx.Size())
	}
}

func TestAddVectorDimensionMismatch(t *testing.T) {
	idx := New(DefaultConfig())

	vec1 := []float32{0.1, 0.2, 0.3, 0.4}
	_, err := idx.AddVector(vec1)
	if err != nil {
		t.Fatalf("First AddVector failed: %v", err)
	}

	vec2 := []float32{0.1, 0.2, 0.3} // Wrong dimension
	_, err = idx.AddVector(vec2)
	if err == nil {
		t.Error("Expected error for dimension mismatch")
	}
}

func TestBuildIndex(t *testing.T) {
	idx := New(DefaultConfig())

	// Add some vectors
	dim := 4
	numVectors := 100

	rand.Seed(42)
	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rand.Float32()
		}
		_, err := idx.AddVector(vec)
		if err != nil {
			t.Fatalf("AddVector failed: %v", err)
		}
	}

	err := idx.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !idx.IsBuilt() {
		t.Error("Index should be built")
	}

	if idx.Size() != int64(numVectors) {
		t.Errorf("Expected size %d, got %d", numVectors, idx.Size())
	}

	// Check that navigating node was set
	navID := idx.GetNavigatingNode()
	if navID >= uint64(numVectors) {
		t.Errorf("Invalid navigating node ID: %d", navID)
	}

	// Check that nodes have neighbors
	for id := uint64(0); id < uint64(numVectors); id++ {
		node, exists := idx.GetNode(id)
		if !exists {
			t.Errorf("Node %d not found", id)
			continue
		}

		neighborCount := node.NeighborCount()
		if neighborCount == 0 {
			t.Errorf("Node %d has no neighbors", id)
		}

		if neighborCount > idx.R {
			t.Errorf("Node %d has %d neighbors, expected max %d", id, neighborCount, idx.R)
		}
	}
}

func TestSearchBasic(t *testing.T) {
	idx := New(DefaultConfig())

	dim := 4
	numVectors := 50

	rand.Seed(42)
	vectors := make([][]float32, numVectors)
	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rand.Float32()
		}
		vectors[i] = vec
		_, err := idx.AddVector(vec)
		if err != nil {
			t.Fatalf("AddVector failed: %v", err)
		}
	}

	err := idx.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Search for first vector (should find itself)
	results, err := idx.Search(vectors[0], 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected at least one result")
	}

	// First result should be the query vector itself
	if results[0].ID != 0 {
		t.Errorf("Expected first result ID to be 0, got %d", results[0].ID)
	}

	// Distance to itself should be very small
	if results[0].Distance > 0.001 {
		t.Errorf("Distance to self should be near 0, got %f", results[0].Distance)
	}
}

func TestSearchRecall(t *testing.T) {
	idx := New(DefaultConfig())

	dim := 16
	numVectors := 200

	rand.Seed(42)
	vectors := make([][]float32, numVectors)
	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rand.Float32()
		}
		vectors[i] = vec
		_, err := idx.AddVector(vec)
		if err != nil {
			t.Fatalf("AddVector failed: %v", err)
		}
	}

	err := idx.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Test search recall
	k := 10
	numQueries := 20
	totalRecall := 0.0

	for i := 0; i < numQueries; i++ {
		query := vectors[i]

		// Get ground truth (brute force)
		groundTruth := bruteForceSearch(query, vectors, k, CosineSimilarity)

		// Get NSG results
		results, err := idx.Search(query, k)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Calculate recall
		recall := calculateRecall(results, groundTruth, k)
		totalRecall += recall
	}

	avgRecall := totalRecall / float64(numQueries)

	// NSG should achieve >80% recall
	if avgRecall < 0.80 {
		t.Errorf("Average recall %.2f%% is too low, expected >80%%", avgRecall*100)
	}

	t.Logf("Average recall: %.2f%%", avgRecall*100)
}

func TestSearchWithFilter(t *testing.T) {
	idx := New(DefaultConfig())

	dim := 4
	numVectors := 50

	rand.Seed(42)
	vectors := make([][]float32, numVectors)
	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rand.Float32()
		}
		vectors[i] = vec
		_, err := idx.AddVector(vec)
		if err != nil {
			t.Fatalf("AddVector failed: %v", err)
		}
	}

	err := idx.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Filter: only even IDs
	filter := func(id uint64) bool {
		return id%2 == 0
	}

	results, err := idx.SearchWithFilter(vectors[0], 5, filter)
	if err != nil {
		t.Fatalf("SearchWithFilter failed: %v", err)
	}

	// All results should have even IDs
	for _, result := range results {
		if result.ID%2 != 0 {
			t.Errorf("Result ID %d is odd, expected only even IDs", result.ID)
		}
	}
}

func TestRangeSearch(t *testing.T) {
	idx := New(DefaultConfig())

	dim := 4
	numVectors := 50

	rand.Seed(42)
	vectors := make([][]float32, numVectors)
	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rand.Float32()
		}
		vectors[i] = vec
		_, err := idx.AddVector(vec)
		if err != nil {
			t.Fatalf("AddVector failed: %v", err)
		}
	}

	err := idx.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Range search around first vector
	radius := float32(0.5)
	results, err := idx.RangeSearch(vectors[0], radius)
	if err != nil {
		t.Fatalf("RangeSearch failed: %v", err)
	}

	// All results should be within radius
	for _, result := range results {
		if result.Distance > radius {
			t.Errorf("Result %d has distance %f, exceeds radius %f", result.ID, result.Distance, radius)
		}
	}

	// Should find at least the query vector itself
	if len(results) == 0 {
		t.Error("Expected at least one result (query itself)")
	}
}

func TestSearchBeforeBuild(t *testing.T) {
	idx := New(DefaultConfig())

	vec := []float32{0.1, 0.2, 0.3, 0.4}
	idx.AddVector(vec)

	// Try to search before building
	_, err := idx.Search(vec, 5)
	if err == nil {
		t.Error("Expected error when searching before build")
	}
}

func TestAddVectorAfterBuild(t *testing.T) {
	idx := New(DefaultConfig())

	vec := []float32{0.1, 0.2, 0.3, 0.4}
	idx.AddVector(vec)
	idx.Build()

	// Try to add vector after building
	_, err := idx.AddVector(vec)
	if err == nil {
		t.Error("Expected error when adding vector after build")
	}
}

func TestDistanceFunctions(t *testing.T) {
	vec1 := []float32{1.0, 0.0, 0.0, 0.0}
	vec2 := []float32{0.0, 1.0, 0.0, 0.0}
	vec3 := []float32{1.0, 0.0, 0.0, 0.0}

	// Cosine distance
	d1 := CosineSimilarity(vec1, vec2)
	if d1 < 0.99 || d1 > 1.01 {
		t.Errorf("Cosine distance between orthogonal vectors should be ~1.0, got %f", d1)
	}

	d2 := CosineSimilarity(vec1, vec3)
	if d2 > 0.001 {
		t.Errorf("Cosine distance between identical vectors should be ~0, got %f", d2)
	}

	// Euclidean distance
	d3 := EuclideanDistance(vec1, vec2)
	expected := float32(math.Sqrt(2.0))
	if math.Abs(float64(d3-expected)) > 0.001 {
		t.Errorf("Euclidean distance should be ~%f, got %f", expected, d3)
	}

	d4 := EuclideanDistance(vec1, vec3)
	if d4 > 0.001 {
		t.Errorf("Euclidean distance between identical vectors should be ~0, got %f", d4)
	}
}

func TestConcurrentSearch(t *testing.T) {
	idx := New(DefaultConfig())

	dim := 8
	numVectors := 100

	rand.Seed(42)
	vectors := make([][]float32, numVectors)
	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rand.Float32()
		}
		vectors[i] = vec
		idx.AddVector(vec)
	}

	idx.Build()

	// Concurrent searches
	done := make(chan bool)
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				query := vectors[rand.Intn(len(vectors))]
				_, err := idx.Search(query, 5)
				if err != nil {
					t.Errorf("Concurrent search failed: %v", err)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// BenchmarkBuild measures index construction time
func BenchmarkBuild(b *testing.B) {
	dim := 128
	numVectors := 1000

	rand.Seed(42)
	vectors := make([][]float32, numVectors)
	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rand.Float32()
		}
		vectors[i] = vec
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx := New(DefaultConfig())
		for _, vec := range vectors {
			idx.AddVector(vec)
		}
		idx.Build()
	}
}

// BenchmarkSearch measures search performance
func BenchmarkSearch(b *testing.B) {
	idx := New(DefaultConfig())

	dim := 128
	numVectors := 10000

	rand.Seed(42)
	vectors := make([][]float32, numVectors)
	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rand.Float32()
		}
		vectors[i] = vec
		idx.AddVector(vec)
	}

	idx.Build()

	// Random query
	query := make([]float32, dim)
	for j := 0; j < dim; j++ {
		query[j] = rand.Float32()
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx.Search(query, 10)
	}
}

// Helper functions

func bruteForceSearch(query []float32, vectors [][]float32, k int, distFunc DistanceFunc) []uint64 {
	type result struct {
		id   uint64
		dist float32
	}

	results := make([]result, len(vectors))
	for i, vec := range vectors {
		dist := distFunc(query, vec)
		results[i] = result{id: uint64(i), dist: dist}
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
	topK := make([]uint64, k)
	for i := 0; i < k && i < len(results); i++ {
		topK[i] = results[i].id
	}

	return topK
}

func calculateRecall(results []SearchResult, groundTruth []uint64, k int) float64 {
	if len(results) == 0 || len(groundTruth) == 0 {
		return 0.0
	}

	// Convert results to ID set
	resultSet := make(map[uint64]bool)
	for _, r := range results {
		resultSet[r.ID] = true
	}

	// Count how many ground truth IDs are in results
	matches := 0
	for i := 0; i < k && i < len(groundTruth); i++ {
		if resultSet[groundTruth[i]] {
			matches++
		}
	}

	return float64(matches) / float64(k)
}

// Test example from documentation
func TestExample(t *testing.T) {
	// Create index
	config := DefaultConfig()
	config.R = 16
	config.L = 100
	idx := New(config)

	// Add vectors
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 1000; i++ {
		vec := make([]float32, 128)
		for j := range vec {
			vec[j] = rand.Float32()
		}
		_, err := idx.AddVector(vec)
		if err != nil {
			t.Fatalf("Failed to add vector: %v", err)
		}
	}

	// Build index
	err := idx.Build()
	if err != nil {
		t.Fatalf("Failed to build index: %v", err)
	}

	// Search
	query := make([]float32, 128)
	for j := range query {
		query[j] = rand.Float32()
	}

	results, err := idx.Search(query, 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one result")
	}

	t.Logf("Found %d results", len(results))
	for i, result := range results {
		t.Logf("  %d. ID=%d, Distance=%.4f", i+1, result.ID, result.Distance)
	}
}
