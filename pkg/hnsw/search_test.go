package hnsw

import (
	"math/rand"
	"sort"
	"testing"
	"time"
)

// TestSearchEmpty tests searching in an empty index
func TestSearchEmpty(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	query := []float32{1.0, 2.0, 3.0}
	_, err := idx.Search(query, 5, 50)

	if err == nil {
		t.Error("Expected error when searching empty index")
	}
}

// TestSearchSingle tests searching with a single vector
func TestSearchSingle(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	vector := []float32{1.0, 2.0, 3.0}
	id, _ := idx.Insert(vector)

	// Search for identical vector
	result, err := idx.Search(vector, 1, 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(result.Results))
	}

	if result.Results[0].ID != id {
		t.Errorf("Expected ID %d, got %d", id, result.Results[0].ID)
	}

	if !almostEqual(result.Results[0].Distance, 0.0) {
		t.Errorf("Expected distance ~0, got %f", result.Results[0].Distance)
	}
}

// TestSearchMultiple tests searching with multiple vectors
func TestSearchMultiple(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	// Insert unit vectors
	vectors := [][]float32{
		{1.0, 0.0, 0.0}, // ID 0
		{0.0, 1.0, 0.0}, // ID 1
		{0.0, 0.0, 1.0}, // ID 2
		{1.0, 1.0, 0.0}, // ID 3
		{1.0, 0.0, 1.0}, // ID 4
	}

	for _, vec := range vectors {
		idx.Insert(vec)
	}

	// Search for vector close to first one
	query := []float32{0.9, 0.1, 0.0}
	result, err := idx.Search(query, 3, 20)

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Results) < 1 {
		t.Fatal("Expected at least 1 result")
	}

	// First result should be ID 0 (closest)
	if result.Results[0].ID != 0 {
		t.Errorf("Expected ID 0 as closest, got %d", result.Results[0].ID)
	}

	// Results should be sorted by distance (ascending)
	for i := 1; i < len(result.Results); i++ {
		if result.Results[i].Distance < result.Results[i-1].Distance {
			t.Error("Results not sorted by distance")
			break
		}
	}
}

// TestKNNSearch tests the convenience KNNSearch method
func TestKNNSearch(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	// Insert some vectors
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 100; i++ {
		vec := make([]float32, 10)
		for j := 0; j < 10; j++ {
			vec[j] = rng.Float32()
		}
		idx.Insert(vec)
	}

	// Search
	query := make([]float32, 10)
	for j := 0; j < 10; j++ {
		query[j] = rng.Float32()
	}

	result, err := idx.KNNSearch(query, 10)
	if err != nil {
		t.Fatalf("KNNSearch failed: %v", err)
	}

	if len(result.Results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(result.Results))
	}
}

// TestSearchDimensionMismatch tests dimension validation
func TestSearchDimensionMismatch(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	// Insert 3D vector
	idx.Insert([]float32{1.0, 2.0, 3.0})

	// Try to search with 2D vector
	_, err := idx.Search([]float32{1.0, 2.0}, 1, 10)
	if err == nil {
		t.Error("Expected error for dimension mismatch")
	}
}

// TestRecall tests recall accuracy against brute force
func TestRecall(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping recall test in short mode")
	}

	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 128
	count := 1000
	queries := 100
	k := 10

	// Insert vectors
	vectors := make([][]float32, count)
	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		vectors[i] = vec
		idx.Insert(vec)
	}

	t.Logf("Inserted %d vectors, index size: %d", count, idx.Size())

	// Generate random queries and test recall
	totalRecall := 0.0
	totalRecall1 := 0.0

	for q := 0; q < queries; q++ {
		query := make([]float32, dim)
		for j := 0; j < dim; j++ {
			query[j] = rng.Float32()
		}

		// HNSW search
		hnswResult, err := idx.Search(query, k, 100)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Brute force search (ground truth)
		bruteForce := bruteForceKNN(query, vectors, k, idx.distanceFunc)

		// Calculate recall
		recall := calculateRecall(hnswResult.Results, bruteForce, k)
		totalRecall += recall

		// Calculate recall@1 (top-1 accuracy)
		recall1 := 0.0
		if len(hnswResult.Results) > 0 && len(bruteForce) > 0 {
			if hnswResult.Results[0].ID == bruteForce[0].ID {
				recall1 = 1.0
			}
		}
		totalRecall1 += recall1
	}

	avgRecall := totalRecall / float64(queries)
	avgRecall1 := totalRecall1 / float64(queries)

	t.Logf("Average Recall@%d: %.2f%%", k, avgRecall*100)
	t.Logf("Average Recall@1: %.2f%%", avgRecall1*100)

	// Expect >90% recall for k=10
	if avgRecall < 0.90 {
		t.Errorf("Recall too low: %.2f%% (expected >90%%)", avgRecall*100)
	}

	// Expect >85% recall@1
	if avgRecall1 < 0.85 {
		t.Errorf("Recall@1 too low: %.2f%% (expected >85%%)", avgRecall1*100)
	}
}

// TestRecallWithDifferentEf tests how efSearch affects recall
func TestRecallWithDifferentEf(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 64
	count := 500

	// Insert vectors
	vectors := make([][]float32, count)
	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		vectors[i] = vec
		idx.Insert(vec)
	}

	// Test different efSearch values
	efValues := []int{10, 20, 50, 100, 200}
	k := 10
	numQueries := 50

	t.Logf("Testing recall with different efSearch values (k=%d):", k)

	for _, ef := range efValues {
		totalRecall := 0.0

		for q := 0; q < numQueries; q++ {
			query := make([]float32, dim)
			for j := 0; j < dim; j++ {
				query[j] = rng.Float32()
			}

			hnswResult, _ := idx.Search(query, k, ef)
			bruteForce := bruteForceKNN(query, vectors, k, idx.distanceFunc)
			recall := calculateRecall(hnswResult.Results, bruteForce, k)
			totalRecall += recall
		}

		avgRecall := totalRecall / float64(numQueries)
		t.Logf("  ef=%3d: Recall = %.2f%%", ef, avgRecall*100)
	}
}

// TestDelete tests vector deletion
func TestDelete(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	// Insert some vectors
	for i := 0; i < 10; i++ {
		vec := []float32{float32(i), float32(i * 2), float32(i * 3)}
		idx.Insert(vec)
	}

	initialSize := idx.Size()

	// Delete a vector
	err := idx.Delete(5)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if idx.Size() != initialSize-1 {
		t.Errorf("Expected size %d after delete, got %d", initialSize-1, idx.Size())
	}

	// Try to get deleted vector
	_, err = idx.GetVector(5)
	if err == nil {
		t.Error("Expected error when getting deleted vector")
	}

	// Delete non-existent vector
	err = idx.Delete(999)
	if err == nil {
		t.Error("Expected error when deleting non-existent vector")
	}
}

// TestGetVector tests vector retrieval
func TestGetVector(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	vector := []float32{1.0, 2.0, 3.0}
	id, _ := idx.Insert(vector)

	retrieved, err := idx.GetVector(id)
	if err != nil {
		t.Fatalf("GetVector failed: %v", err)
	}

	if len(retrieved) != len(vector) {
		t.Errorf("Retrieved vector has wrong length")
	}

	for i := range vector {
		if retrieved[i] != vector[i] {
			t.Errorf("Retrieved vector mismatch at index %d", i)
		}
	}
}

// bruteForceKNN performs brute force k-NN search
func bruteForceKNN(query []float32, vectors [][]float32, k int, distFunc DistanceFunc) []Result {
	type dist struct {
		id   uint64
		dist float32
	}

	distances := make([]dist, len(vectors))
	for i, vec := range vectors {
		distances[i] = dist{
			id:   uint64(i),
			dist: distFunc(query, vec),
		}
	}

	// Sort by distance
	sort.Slice(distances, func(i, j int) bool {
		return distances[i].dist < distances[j].dist
	})

	// Return top k
	results := make([]Result, 0, k)
	for i := 0; i < k && i < len(distances); i++ {
		results = append(results, Result{
			ID:       distances[i].id,
			Distance: distances[i].dist,
		})
	}

	return results
}

// calculateRecall calculates the recall between HNSW and brute force results
func calculateRecall(hnswResults []Result, bruteForce []Result, k int) float64 {
	if len(hnswResults) == 0 || len(bruteForce) == 0 {
		return 0.0
	}

	// Create set of IDs from brute force
	bruteForceIDs := make(map[uint64]bool)
	for _, r := range bruteForce {
		bruteForceIDs[r.ID] = true
	}

	// Count how many HNSW results are in brute force top-k
	matches := 0
	for _, r := range hnswResults {
		if bruteForceIDs[r.ID] {
			matches++
		}
	}

	return float64(matches) / float64(k)
}

// BenchmarkSearch benchmarks search performance
func BenchmarkSearch(b *testing.B) {
	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 768

	// Insert 1000 vectors
	for i := 0; i < 1000; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		idx.Insert(vec)
	}

	// Generate queries
	queries := make([][]float32, b.N)
	for i := 0; i < b.N; i++ {
		query := make([]float32, dim)
		for j := 0; j < dim; j++ {
			query[j] = rng.Float32()
		}
		queries[i] = query
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx.Search(queries[i], 10, 50)
	}
}

// BenchmarkSearchVaryingEf benchmarks search with different efSearch values
func BenchmarkSearchEf50(b *testing.B)   { benchmarkSearchWithEf(b, 50) }
func BenchmarkSearchEf100(b *testing.B)  { benchmarkSearchWithEf(b, 100) }
func BenchmarkSearchEf200(b *testing.B)  { benchmarkSearchWithEf(b, 200) }

func benchmarkSearchWithEf(b *testing.B, ef int) {
	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 128

	// Insert 1000 vectors
	for i := 0; i < 1000; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		idx.Insert(vec)
	}

	query := make([]float32, dim)
	for j := 0; j < dim; j++ {
		query[j] = rng.Float32()
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx.Search(query, 10, ef)
	}
}

// BenchmarkBruteForce benchmarks brute force search
func BenchmarkBruteForce(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	dim := 128
	count := 1000

	vectors := make([][]float32, count)
	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		vectors[i] = vec
	}

	query := make([]float32, dim)
	for j := 0; j < dim; j++ {
		query[j] = rng.Float32()
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bruteForceKNN(query, vectors, 10, CosineSimilarity)
	}
}

// TestSearchPerformance tests search latency
func TestSearchPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 128
	count := 10000

	// Insert 10K vectors
	t.Logf("Inserting %d vectors...", count)
	start := time.Now()

	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		idx.Insert(vec)
	}

	insertTime := time.Since(start)
	t.Logf("Insertion completed in %v (avg: %v per vector)",
		insertTime, insertTime/time.Duration(count))

	// Perform searches and measure latency
	numQueries := 1000
	latencies := make([]time.Duration, numQueries)

	t.Logf("Performing %d searches...", numQueries)

	for i := 0; i < numQueries; i++ {
		query := make([]float32, dim)
		for j := 0; j < dim; j++ {
			query[j] = rng.Float32()
		}

		start := time.Now()
		_, err := idx.Search(query, 10, 50)
		latencies[i] = time.Since(start)

		if err != nil {
			t.Fatalf("Search %d failed: %v", i, err)
		}
	}

	// Calculate percentiles
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	p50 := latencies[int(float64(numQueries)*0.50)]
	p95 := latencies[int(float64(numQueries)*0.95)]
	p99 := latencies[int(float64(numQueries)*0.99)]

	t.Logf("Search latency (n=%d):", numQueries)
	t.Logf("  p50: %v", p50)
	t.Logf("  p95: %v", p95)
	t.Logf("  p99: %v", p99)

	// Expect p95 < 10ms for 10K vectors
	if p95 > 10*time.Millisecond {
		t.Logf("Warning: p95 latency (%v) exceeds 10ms target", p95)
	}
}
