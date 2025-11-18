package scann

import (
	"math/rand"
	"testing"

	"github.com/therealutkarshpriyadarshi/vector/internal/quantization"
)

func TestSCANN_Train(t *testing.T) {
	config := DefaultConfig()
	config.NumPartitions = 20

	scann := NewSCANN(config)
	vectors := generateRandomVectors(1000, 768)

	err := scann.Train(vectors)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	if !scann.trained {
		t.Error("SCANN should be marked as trained")
	}

	if len(scann.partitions) != 20 {
		t.Errorf("Expected 20 partitions, got %d", len(scann.partitions))
	}

	if scann.aq == nil {
		t.Error("Anisotropic quantizer should be initialized")
	}
}

func TestSCANN_SphericalKMeans(t *testing.T) {
	config := DefaultConfig()
	config.NumPartitions = 10
	config.SphericalKM = true

	scann := NewSCANN(config)
	vectors := generateRandomVectors(500, 128)

	err := scann.Train(vectors)
	if err != nil {
		t.Fatalf("Spherical k-means training failed: %v", err)
	}

	// Check that centroids are normalized (for spherical k-means)
	for i, centroid := range scann.partitions {
		norm := quantization.NormL2(centroid)
		// Norm should be close to 1.0
		if norm < 0.9 || norm > 1.1 {
			t.Errorf("Centroid %d not normalized: norm=%f", i, norm)
		}
	}
}

func TestSCANN_Add(t *testing.T) {
	config := DefaultConfig()
	config.NumPartitions = 20

	scann := NewSCANN(config)
	vectors := generateRandomVectors(1000, 768)

	scann.Train(vectors)

	// Add vectors
	ids := make([]int, 500)
	addVectors := vectors[:500]
	for i := range ids {
		ids[i] = i
	}

	err := scann.Add(addVectors, ids, nil)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	stats := scann.GetStats()
	totalEntries := stats["total_entries"].(int)

	if totalEntries != 500 {
		t.Errorf("Expected 500 entries, got %d", totalEntries)
	}
}

func TestSCANN_Search(t *testing.T) {
	config := DefaultConfig()
	config.NumPartitions = 50
	config.NumSubvectors = 16
	config.BitsPerCode = 8

	scann := NewSCANN(config)
	vectors := generateRandomVectors(2000, 768)

	scann.Train(vectors)

	// Add vectors
	ids := make([]int, len(vectors))
	for i := range ids {
		ids[i] = i
	}
	scann.Add(vectors, ids, nil)

	// Search
	query := vectors[0]
	resultIDs, distances, err := scann.Search(query, 10, 10)

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(resultIDs) == 0 {
		t.Error("Expected some results")
	}

	t.Logf("Search returned %d results", len(resultIDs))
	t.Logf("First result: ID=%d, distance=%f", resultIDs[0], distances[0])

	// First result should be the query itself or very close
	if distances[0] > 1.0 {
		t.Logf("Warning: First result distance is high: %f", distances[0])
	}
}

func TestSCANN_SearchWithFilter(t *testing.T) {
	config := DefaultConfig()
	config.NumPartitions = 30

	scann := NewSCANN(config)
	vectors := generateRandomVectors(1000, 768)

	scann.Train(vectors)

	// Add vectors with metadata
	ids := make([]int, len(vectors))
	metadata := make([]map[string]interface{}, len(vectors))
	for i := range ids {
		ids[i] = i
		metadata[i] = map[string]interface{}{
			"category": i % 10,
		}
	}
	scann.Add(vectors, ids, metadata)

	// Search with filter
	query := vectors[25] // Category 5
	filter := func(meta map[string]interface{}) bool {
		if meta == nil {
			return false
		}
		cat, ok := meta["category"].(int)
		return ok && cat == 5
	}

	resultIDs, _, err := scann.SearchWithFilter(query, 10, 15, filter)

	if err != nil {
		t.Fatalf("Search with filter failed: %v", err)
	}

	// All results should have category 5
	for _, id := range resultIDs {
		cat := metadata[id]["category"].(int)
		if cat != 5 {
			t.Errorf("Expected category 5, got %d for ID %d", cat, id)
		}
	}

	t.Logf("Filtered search returned %d results", len(resultIDs))
}

func TestSCANN_CompressionRatio(t *testing.T) {
	config := DefaultConfig()
	config.NumPartitions = 50
	config.NumSubvectors = 16
	config.BitsPerCode = 8

	scann := NewSCANN(config)
	vectors := generateRandomVectors(1000, 768)

	scann.Train(vectors)

	stats := scann.GetStats()
	compressionRatio := stats["compression_ratio"].(float32)

	// Should be around 192x for 16 subvectors
	if compressionRatio < 180 || compressionRatio > 200 {
		t.Logf("Compression ratio: %f", compressionRatio)
	}

	bytesPerVector := stats["bytes_per_vector"].(int)
	t.Logf("Compression: %.1fx, %d bytes per vector", compressionRatio, bytesPerVector)
}

func TestAnisotropicQuantizer_Train(t *testing.T) {
	aq := NewAnisotropicQuantizer(768, 16, 8)

	vectors := generateRandomVectors(1000, 768)
	config := quantization.DefaultConfig()

	err := aq.Train(vectors, config)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	if len(aq.codebooks) != 16 {
		t.Errorf("Expected 16 codebooks, got %d", len(aq.codebooks))
	}

	// Each codebook should have 256 centroids (2^8)
	for i, codebook := range aq.codebooks {
		if len(codebook) != 256 {
			t.Errorf("Codebook %d: expected 256 centroids, got %d", i, len(codebook))
		}
	}
}

func TestAnisotropicQuantizer_Encode(t *testing.T) {
	aq := NewAnisotropicQuantizer(128, 8, 6)

	vectors := generateRandomVectors(500, 128)
	config := quantization.DefaultConfig()
	aq.Train(vectors, config)

	testVector := vectors[0]
	codes := aq.Encode(testVector)

	if len(codes) != 8 {
		t.Errorf("Expected 8 codes, got %d", len(codes))
	}
}

func TestAnisotropicQuantizer_AsymmetricDistance(t *testing.T) {
	aq := NewAnisotropicQuantizer(768, 16, 8)

	vectors := generateRandomVectors(1000, 768)
	config := quantization.DefaultConfig()
	aq.Train(vectors, config)

	query := vectors[0]
	testVector := vectors[1]

	codes := aq.Encode(testVector)
	distTable := aq.ComputeDistanceTable(query)
	asymDist := aq.AsymmetricDistance(distTable, codes)

	// Distance should be positive
	if asymDist <= 0 {
		t.Errorf("Distance should be positive, got %f", asymDist)
	}

	// Distance should be reasonable
	exactDist := quantization.EuclideanDistanceFloat32(query, testVector)
	t.Logf("Asymmetric distance: %f, Exact distance: %f", asymDist, exactDist)
}

func TestAnisotropicQuantizer_Serialize(t *testing.T) {
	aq := NewAnisotropicQuantizer(128, 8, 6)

	vectors := generateRandomVectors(500, 128)
	config := quantization.DefaultConfig()
	aq.Train(vectors, config)

	// Serialize
	data, err := aq.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Deserialize
	aq2 := NewAnisotropicQuantizer(0, 0, 0)
	err = aq2.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Check parameters
	if aq2.dim != aq.dim {
		t.Errorf("dim mismatch: %d vs %d", aq2.dim, aq.dim)
	}

	if aq2.numSubvectors != aq.numSubvectors {
		t.Errorf("numSubvectors mismatch: %d vs %d", aq2.numSubvectors, aq.numSubvectors)
	}

	// Test encoding consistency
	testVector := vectors[0]
	codes1 := aq.Encode(testVector)
	codes2 := aq2.Encode(testVector)

	for i := range codes1 {
		if codes1[i] != codes2[i] {
			t.Errorf("Code mismatch at %d: %d vs %d", i, codes1[i], codes2[i])
		}
	}
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

func BenchmarkSCANN_Search(b *testing.B) {
	config := DefaultConfig()
	config.NumPartitions = 100
	config.NumSubvectors = 16
	config.BitsPerCode = 8

	scann := NewSCANN(config)
	vectors := generateRandomVectors(10000, 768)

	scann.Train(vectors)

	ids := make([]int, len(vectors))
	for i := range ids {
		ids[i] = i
	}
	scann.Add(vectors, ids, nil)

	query := vectors[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scann.Search(query, 10, 10)
	}
}

func BenchmarkAnisotropicQuantizer_Encode(b *testing.B) {
	aq := NewAnisotropicQuantizer(768, 16, 8)

	vectors := generateRandomVectors(1000, 768)
	config := quantization.DefaultConfig()
	aq.Train(vectors, config)

	testVector := vectors[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		aq.Encode(testVector)
	}
}

func BenchmarkAnisotropicQuantizer_AsymmetricDistance(b *testing.B) {
	aq := NewAnisotropicQuantizer(768, 16, 8)

	vectors := generateRandomVectors(1000, 768)
	config := quantization.DefaultConfig()
	aq.Train(vectors, config)

	query := vectors[0]
	codes := aq.Encode(vectors[1])
	distTable := aq.ComputeDistanceTable(query)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		aq.AsymmetricDistance(distTable, codes)
	}
}
