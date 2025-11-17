package hnsw

import (
	"math/rand"
	"testing"
	"time"
)

// TestInsertFirst tests inserting the first vector (entry point)
func TestInsertFirst(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	vector := []float32{1.0, 2.0, 3.0}
	id, err := idx.Insert(vector)

	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	if idx.Size() != 1 {
		t.Errorf("Expected size 1, got %d", idx.Size())
	}

	if idx.EntryPoint() == nil {
		t.Error("Entry point should be set")
	}

	if idx.EntryPoint().ID() != id {
		t.Error("Entry point should be the first inserted node")
	}

	if idx.Dimension() != 3 {
		t.Errorf("Expected dimension 3, got %d", idx.Dimension())
	}
}

// TestInsertMultiple tests inserting multiple vectors
func TestInsertMultiple(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	vectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{0.0, 0.0, 1.0},
		{1.0, 1.0, 0.0},
		{1.0, 0.0, 1.0},
		{0.0, 1.0, 1.0},
		{1.0, 1.0, 1.0},
		{0.5, 0.5, 0.5},
		{0.2, 0.3, 0.5},
		{0.8, 0.1, 0.1},
	}

	for i, vec := range vectors {
		id, err := idx.Insert(vec)
		if err != nil {
			t.Fatalf("Insert %d failed: %v", i, err)
		}

		if id != uint64(i) {
			t.Errorf("Expected ID %d, got %d", i, id)
		}
	}

	if idx.Size() != int64(len(vectors)) {
		t.Errorf("Expected size %d, got %d", len(vectors), idx.Size())
	}

	// Verify all nodes are accessible
	for i := 0; i < len(vectors); i++ {
		node := idx.GetNode(uint64(i))
		if node == nil {
			t.Errorf("Node %d not found", i)
		}
	}
}

// TestInsertDimensionMismatch tests that dimension mismatch is detected
func TestInsertDimensionMismatch(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	// Insert first vector (3D)
	_, err := idx.Insert([]float32{1.0, 2.0, 3.0})
	if err != nil {
		t.Fatalf("First insert failed: %v", err)
	}

	// Try to insert 2D vector
	_, err = idx.Insert([]float32{1.0, 2.0})
	if err == nil {
		t.Error("Expected error for dimension mismatch")
	}

	// Try to insert 4D vector
	_, err = idx.Insert([]float32{1.0, 2.0, 3.0, 4.0})
	if err == nil {
		t.Error("Expected error for dimension mismatch")
	}
}

// TestInsertEmpty tests that empty vectors are rejected
func TestInsertEmpty(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	_, err := idx.Insert([]float32{})
	if err == nil {
		t.Error("Expected error for empty vector")
	}
}

// TestInsert100 tests inserting 100 random vectors
func TestInsert100(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 128
	count := 100

	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}

		_, err := idx.Insert(vec)
		if err != nil {
			t.Fatalf("Insert %d failed: %v", i, err)
		}
	}

	if idx.Size() != int64(count) {
		t.Errorf("Expected size %d, got %d", count, idx.Size())
	}

	stats := idx.GetStats()
	t.Logf("Index stats after 100 insertions:")
	t.Logf("  Size: %d", stats.Size)
	t.Logf("  MaxLayer: %d", stats.MaxLayer)
	t.Logf("  Nodes per layer:")
	for layer := 0; layer <= stats.MaxLayer; layer++ {
		t.Logf("    Layer %d: %d nodes", layer, stats.NodesPerLayer[layer])
	}
}

// TestInsert1000 tests inserting 1000 vectors
func TestInsert1000(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 128
	count := 1000

	start := time.Now()

	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}

		_, err := idx.Insert(vec)
		if err != nil {
			t.Fatalf("Insert %d failed: %v", i, err)
		}
	}

	elapsed := time.Since(start)

	if idx.Size() != int64(count) {
		t.Errorf("Expected size %d, got %d", count, idx.Size())
	}

	avgTime := elapsed / time.Duration(count)
	t.Logf("Inserted %d vectors in %v (avg: %v per vector)", count, elapsed, avgTime)

	stats := idx.GetStats()
	t.Logf("Index stats after 1000 insertions:")
	t.Logf("  MaxLayer: %d", stats.MaxLayer)
	t.Logf("  Nodes per layer:")
	for layer := 0; layer <= stats.MaxLayer; layer++ {
		t.Logf("    Layer %d: %d nodes", layer, stats.NodesPerLayer[layer])
	}
}

// TestGraphConnectivity tests that the graph is connected
func TestGraphConnectivity(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	// Insert 50 vectors
	rng := rand.New(rand.NewSource(42))
	count := 50

	for i := 0; i < count; i++ {
		vec := make([]float32, 10)
		for j := 0; j < 10; j++ {
			vec[j] = rng.Float32()
		}
		idx.Insert(vec)
	}

	// Verify that all nodes at layer 0 have at least one neighbor
	// (except if there's only one node total)
	for i := 0; i < count; i++ {
		node := idx.GetNode(uint64(i))
		if node == nil {
			t.Errorf("Node %d not found", i)
			continue
		}

		neighborCount := node.NeighborCount(0)
		if count > 1 && neighborCount == 0 {
			t.Errorf("Node %d has no neighbors at layer 0", i)
		}
	}
}

// TestMaxConnections tests that M and M0 limits are respected
func TestMaxConnections(t *testing.T) {
	config := IndexConfig{
		M:              4,
		efConstruction: 20,
		DistanceFunc:   CosineSimilarity,
	}
	idx := New(config)

	// Insert 20 vectors to force pruning
	rng := rand.New(rand.NewSource(42))
	count := 20

	for i := 0; i < count; i++ {
		vec := make([]float32, 10)
		for j := 0; j < 10; j++ {
			vec[j] = rng.Float32()
		}
		idx.Insert(vec)
	}

	// Check that no node exceeds M connections at layer > 0
	// and M0 connections at layer 0
	for i := 0; i < count; i++ {
		node := idx.GetNode(uint64(i))
		if node == nil {
			continue
		}

		// Check layer 0
		neighbors0 := node.NeighborCount(0)
		if neighbors0 > idx.M0 {
			t.Errorf("Node %d has %d neighbors at layer 0 (max: %d)",
				i, neighbors0, idx.M0)
		}

		// Check higher layers
		for layer := 1; layer <= node.Level(); layer++ {
			neighbors := node.NeighborCount(layer)
			if neighbors > idx.M {
				t.Errorf("Node %d has %d neighbors at layer %d (max: %d)",
					i, neighbors, layer, idx.M)
			}
		}
	}
}

// TestBidirectionalLinks tests that all links are bidirectional
func TestBidirectionalLinks(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	// Insert some vectors
	rng := rand.New(rand.NewSource(42))
	count := 30

	for i := 0; i < count; i++ {
		vec := make([]float32, 10)
		for j := 0; j < 10; j++ {
			vec[j] = rng.Float32()
		}
		idx.Insert(vec)
	}

	// Verify bidirectional links
	for i := 0; i < count; i++ {
		node := idx.GetNode(uint64(i))
		if node == nil {
			continue
		}

		// Check all layers
		for layer := 0; layer <= node.Level(); layer++ {
			neighbors := node.GetNeighbors(layer)

			for _, neighborID := range neighbors {
				neighborNode := idx.GetNode(neighborID)
				if neighborNode == nil {
					t.Errorf("Neighbor %d not found", neighborID)
					continue
				}

				// Verify the link is bidirectional
				if !neighborNode.HasNeighbor(layer, uint64(i)) {
					t.Errorf("Link from %d to %d at layer %d is not bidirectional",
						i, neighborID, layer)
				}
			}
		}
	}
}

// BenchmarkInsert benchmarks insertion performance
func BenchmarkInsert(b *testing.B) {
	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 768

	// Pre-generate vectors
	vectors := make([][]float32, b.N)
	for i := 0; i < b.N; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		vectors[i] = vec
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx.Insert(vectors[i])
	}
}

// BenchmarkInsert100 benchmarks inserting 100 vectors
func BenchmarkInsert100(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	dim := 128

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		config := DefaultConfig()
		idx := New(config)

		for j := 0; j < 100; j++ {
			vec := make([]float32, dim)
			for k := 0; k < dim; k++ {
				vec[k] = rng.Float32()
			}
			idx.Insert(vec)
		}
	}
}
