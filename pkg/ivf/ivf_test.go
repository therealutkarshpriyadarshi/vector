package ivf

import (
	"math/rand"
	"testing"

	"github.com/therealutkarshpriyadarshi/vector/internal/quantization"
)

func TestIVFFlat_Train(t *testing.T) {
	config := Config{
		NumCentroids: 10,
		Metric:       quantization.EuclideanDistance,
	}

	ivf := NewIVFFlat(config)
	vectors := generateRandomVectors(500, 128)

	err := ivf.Train(vectors)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	if !ivf.trained {
		t.Error("Index should be marked as trained")
	}

	if len(ivf.centroids) != 10 {
		t.Errorf("Expected 10 centroids, got %d", len(ivf.centroids))
	}
}

func TestIVFFlat_Add(t *testing.T) {
	config := Config{
		NumCentroids: 10,
		Metric:       quantization.EuclideanDistance,
	}

	ivf := NewIVFFlat(config)
	vectors := generateRandomVectors(500, 128)

	ivf.Train(vectors)

	// Add vectors
	ids := make([]int, 100)
	addVectors := vectors[:100]
	for i := range ids {
		ids[i] = i
	}

	err := ivf.Add(addVectors, ids, nil)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if len(ivf.vectors) != 100 {
		t.Errorf("Expected 100 vectors, got %d", len(ivf.vectors))
	}
}

func TestIVFFlat_Search(t *testing.T) {
	config := Config{
		NumCentroids: 20,
		Metric:       quantization.EuclideanDistance,
	}

	ivf := NewIVFFlat(config)
	vectors := generateRandomVectors(1000, 128)

	ivf.Train(vectors)

	// Add vectors
	ids := make([]int, len(vectors))
	for i := range ids {
		ids[i] = i
	}
	ivf.Add(vectors, ids, nil)

	// Search
	query := vectors[0]
	resultIDs, distances, err := ivf.Search(query, 10, 5)

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(resultIDs) == 0 {
		t.Error("Expected some results")
	}

	// First result should be the query itself (ID=0)
	if resultIDs[0] != 0 {
		t.Errorf("Expected first result to be ID 0, got %d", resultIDs[0])
	}

	// Distance should be 0 (or very close)
	if distances[0] > 0.01 {
		t.Errorf("Expected distance ~0, got %f", distances[0])
	}
}

func TestIVFFlat_SearchWithFilter(t *testing.T) {
	config := Config{
		NumCentroids: 20,
		Metric:       quantization.EuclideanDistance,
	}

	ivf := NewIVFFlat(config)
	vectors := generateRandomVectors(1000, 128)

	ivf.Train(vectors)

	// Add vectors with metadata
	ids := make([]int, len(vectors))
	metadata := make([]map[string]interface{}, len(vectors))
	for i := range ids {
		ids[i] = i
		metadata[i] = map[string]interface{}{
			"category": i % 5, // 5 categories
		}
	}
	ivf.Add(vectors, ids, metadata)

	// Search with filter (only category 2)
	query := vectors[12] // This has category 2
	filter := func(meta map[string]interface{}) bool {
		if meta == nil {
			return false
		}
		cat, ok := meta["category"].(int)
		return ok && cat == 2
	}

	resultIDs, _, err := ivf.SearchWithFilter(query, 10, 10, filter)

	if err != nil {
		t.Fatalf("Search with filter failed: %v", err)
	}

	// All results should have category 2
	for _, id := range resultIDs {
		cat := metadata[id]["category"].(int)
		if cat != 2 {
			t.Errorf("Expected category 2, got %d for ID %d", cat, id)
		}
	}
}

func TestIVFPQ_Train(t *testing.T) {
	config := ConfigPQ{
		NumCentroids:  20,
		NumSubvectors: 8,
		BitsPerCode:   8,
		Metric:        quantization.EuclideanDistance,
	}

	ivfpq := NewIVFPQ(config)
	vectors := generateRandomVectors(1000, 768)

	err := ivfpq.Train(vectors)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	if !ivfpq.trained {
		t.Error("IVF should be trained")
	}

	if !ivfpq.pqTrained {
		t.Error("PQ should be trained")
	}
}

func TestIVFPQ_Add(t *testing.T) {
	config := ConfigPQ{
		NumCentroids:  20,
		NumSubvectors: 8,
		BitsPerCode:   8,
		Metric:        quantization.EuclideanDistance,
	}

	ivfpq := NewIVFPQ(config)
	vectors := generateRandomVectors(1000, 768)

	ivfpq.Train(vectors)

	// Add vectors
	ids := make([]int, 500)
	addVectors := vectors[:500]
	for i := range ids {
		ids[i] = i
	}

	err := ivfpq.Add(addVectors, ids, nil)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	stats := ivfpq.GetStats()
	totalEntries := stats["total_entries"].(int)

	if totalEntries != 500 {
		t.Errorf("Expected 500 entries, got %d", totalEntries)
	}
}

func TestIVFPQ_Search(t *testing.T) {
	config := ConfigPQ{
		NumCentroids:  20,
		NumSubvectors: 16,
		BitsPerCode:   8,
		Metric:        quantization.EuclideanDistance,
	}

	ivfpq := NewIVFPQ(config)
	vectors := generateRandomVectors(1000, 768)

	ivfpq.Train(vectors)

	// Add vectors
	ids := make([]int, len(vectors))
	for i := range ids {
		ids[i] = i
	}
	ivfpq.Add(vectors, ids, nil)

	// Search
	query := vectors[0]
	resultIDs, distances, err := ivfpq.Search(query, 10, 5)

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(resultIDs) == 0 {
		t.Error("Expected some results")
	}

	// First result should be the query itself or very close
	if resultIDs[0] != 0 {
		t.Logf("First result is not exact match (ID=%d), distance=%f", resultIDs[0], distances[0])
	}

	t.Logf("Search returned %d results, first distance: %f", len(resultIDs), distances[0])
}

func TestIVFPQ_CompressionRatio(t *testing.T) {
	config := ConfigPQ{
		NumCentroids:  50,
		NumSubvectors: 16,
		BitsPerCode:   8,
		Metric:        quantization.EuclideanDistance,
	}

	ivfpq := NewIVFPQ(config)
	vectors := generateRandomVectors(1000, 768)

	ivfpq.Train(vectors)

	stats := ivfpq.GetStats()
	compressionRatio := stats["compression_ratio"].(float32)

	// Should be 768*4 / 16 = 192x
	if compressionRatio < 180 || compressionRatio > 200 {
		t.Errorf("Unexpected compression ratio: %f", compressionRatio)
	}

	t.Logf("Compression ratio: %.1fx", compressionRatio)
}

func TestIVFPQ_MemoryUsage(t *testing.T) {
	config := ConfigPQ{
		NumCentroids:  50,
		NumSubvectors: 16,
		BitsPerCode:   8,
		Metric:        quantization.EuclideanDistance,
	}

	ivfpq := NewIVFPQ(config)
	vectors := generateRandomVectors(1000, 768)

	ivfpq.Train(vectors)

	ids := make([]int, len(vectors))
	for i := range ids {
		ids[i] = i
	}
	ivfpq.Add(vectors, ids, nil)

	memoryUsage := ivfpq.GetMemoryUsage()

	// Original: 1000 vectors * 768 dims * 4 bytes = 3,072,000 bytes
	// Compressed: should be much smaller
	originalSize := int64(1000 * 768 * 4)

	if memoryUsage >= originalSize {
		t.Errorf("Compressed size (%d) should be < original (%d)", memoryUsage, originalSize)
	}

	compressionRatio := float64(originalSize) / float64(memoryUsage)
	t.Logf("Memory usage: %d bytes, compression: %.1fx", memoryUsage, compressionRatio)
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

// Benchmarks

func BenchmarkIVFFlat_Search(b *testing.B) {
	config := Config{
		NumCentroids: 100,
		Metric:       quantization.EuclideanDistance,
	}

	ivf := NewIVFFlat(config)
	vectors := generateRandomVectors(10000, 768)

	ivf.Train(vectors)

	ids := make([]int, len(vectors))
	for i := range ids {
		ids[i] = i
	}
	ivf.Add(vectors, ids, nil)

	query := vectors[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ivf.Search(query, 10, 10)
	}
}

func BenchmarkIVFPQ_Search(b *testing.B) {
	config := ConfigPQ{
		NumCentroids:  100,
		NumSubvectors: 16,
		BitsPerCode:   8,
		Metric:        quantization.EuclideanDistance,
	}

	ivfpq := NewIVFPQ(config)
	vectors := generateRandomVectors(10000, 768)

	ivfpq.Train(vectors)

	ids := make([]int, len(vectors))
	for i := range ids {
		ids[i] = i
	}
	ivfpq.Add(vectors, ids, nil)

	query := vectors[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ivfpq.Search(query, 10, 10)
	}
}
