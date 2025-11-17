package hnsw

import (
	"math/rand"
	"testing"
)

// TestRecallEuclidean tests recall with Euclidean distance
func TestRecallEuclidean(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	config := IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   EuclideanDistance,
	}
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

	t.Logf("Inserted %d vectors with Euclidean distance", count)
	stats := idx.GetStats()
	t.Logf("Max layer: %d", stats.MaxLayer)
	for layer := 0; layer <= stats.MaxLayer; layer++ {
		t.Logf("  Layer %d: %d nodes", layer, stats.NodesPerLayer[layer])
	}

	// Test recall
	totalRecall := 0.0
	totalRecall1 := 0.0

	for q := 0; q < queries; q++ {
		query := make([]float32, dim)
		for j := 0; j < dim; j++ {
			query[j] = rng.Float32()
		}

		hnswResult, _ := idx.Search(query, k, 100)
		bruteForce := bruteForceKNN(query, vectors, k, EuclideanDistance)

		recall := calculateRecall(hnswResult.Results, bruteForce, k)
		totalRecall += recall

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

	t.Logf("Euclidean Distance Results:")
	t.Logf("  Average Recall@%d: %.2f%%", k, avgRecall*100)
	t.Logf("  Average Recall@1: %.2f%%", avgRecall1*100)

	if avgRecall < 0.90 {
		t.Logf("Warning: Recall is %.2f%% (target >90%%)", avgRecall*100)
	}
}

// TestRecallSmallDataset tests recall on a smaller dataset
func TestRecallSmallDataset(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 64
	count := 100
	k := 5

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

	// Test recall on every vector in the dataset
	totalRecall := 0.0

	for i := 0; i < count; i++ {
		query := vectors[i]

		hnswResult, err := idx.Search(query, k, 50)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		bruteForce := bruteForceKNN(query, vectors, k, idx.distanceFunc)
		recall := calculateRecall(hnswResult.Results, bruteForce, k)
		totalRecall += recall

		// For exact match (searching for same vector), first result should be itself
		if hnswResult.Results[0].ID != uint64(i) {
			t.Errorf("Query for vector %d: first result is %d (distance %.4f), expected %d",
				i, hnswResult.Results[0].ID, hnswResult.Results[0].Distance, i)
		}
	}

	avgRecall := totalRecall / float64(count)
	t.Logf("Small dataset (%d vectors) recall@%d: %.2f%%", count, k, avgRecall*100)

	if avgRecall < 0.95 {
		t.Errorf("Recall too low for small dataset: %.2f%%", avgRecall*100)
	}
}

// TestLayerDistribution checks that nodes are distributed across layers
func TestLayerDistribution(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 128
	count := 1000

	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		idx.Insert(vec)
	}

	stats := idx.GetStats()
	t.Logf("Layer distribution for %d vectors:", count)
	for layer := 0; layer <= stats.MaxLayer; layer++ {
		percentage := float64(stats.NodesPerLayer[layer]) / float64(count) * 100
		t.Logf("  Layer %d: %d nodes (%.2f%%)", layer, stats.NodesPerLayer[layer], percentage)
	}

	// Should have multiple layers
	if stats.MaxLayer < 1 {
		t.Error("Expected at least 2 layers for 1000 vectors")
	}

	// Layer 0 should have all nodes
	if stats.NodesPerLayer[0] != count {
		t.Errorf("Layer 0 should have all %d nodes, got %d", count, stats.NodesPerLayer[0])
	}

	// Higher layers should have fewer nodes (exponential decay)
	if stats.MaxLayer >= 1 {
		ratio := float64(stats.NodesPerLayer[1]) / float64(stats.NodesPerLayer[0])
		t.Logf("Layer 1/Layer 0 ratio: %.4f", ratio)

		// Should be roughly 1/M  (with M=16, expect ~6%)
		if ratio > 0.2 {
			t.Logf("Warning: Layer ratio seems high: %.4f", ratio)
		}
	}
}
