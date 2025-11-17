package hnsw

import (
	"sync"
	"testing"
)

// TestNewNode tests node creation
func TestNewNode(t *testing.T) {
	vector := []float32{1.0, 2.0, 3.0}
	node := NewNode(123, vector, 2)

	if node.ID() != 123 {
		t.Errorf("Expected ID 123, got %d", node.ID())
	}

	if node.Level() != 2 {
		t.Errorf("Expected level 2, got %d", node.Level())
	}

	if len(node.Vector()) != 3 {
		t.Errorf("Expected vector length 3, got %d", len(node.Vector()))
	}

	// Check that neighbors are initialized for all layers
	for layer := 0; layer <= 2; layer++ {
		neighbors := node.GetNeighbors(layer)
		if neighbors == nil {
			t.Errorf("Neighbors at layer %d should be initialized", layer)
		}
		if len(neighbors) != 0 {
			t.Errorf("Layer %d should start with 0 neighbors, got %d", layer, len(neighbors))
		}
	}
}

// TestNodeAddNeighbor tests adding neighbors
func TestNodeAddNeighbor(t *testing.T) {
	node := NewNode(1, []float32{1, 2, 3}, 2)

	// Add neighbor at layer 0
	node.AddNeighbor(0, 2)
	neighbors := node.GetNeighbors(0)
	if len(neighbors) != 1 || neighbors[0] != 2 {
		t.Errorf("Expected neighbor 2 at layer 0")
	}

	// Add another neighbor
	node.AddNeighbor(0, 3)
	neighbors = node.GetNeighbors(0)
	if len(neighbors) != 2 {
		t.Errorf("Expected 2 neighbors at layer 0, got %d", len(neighbors))
	}

	// Try to add duplicate - should be ignored
	node.AddNeighbor(0, 2)
	neighbors = node.GetNeighbors(0)
	if len(neighbors) != 2 {
		t.Errorf("Duplicate neighbor should be ignored, got %d neighbors", len(neighbors))
	}
}

// TestNodeRemoveNeighbor tests removing neighbors
func TestNodeRemoveNeighbor(t *testing.T) {
	node := NewNode(1, []float32{1, 2, 3}, 1)

	// Add some neighbors
	node.AddNeighbor(0, 2)
	node.AddNeighbor(0, 3)
	node.AddNeighbor(0, 4)

	// Remove middle neighbor
	node.RemoveNeighbor(0, 3)
	neighbors := node.GetNeighbors(0)
	if len(neighbors) != 2 {
		t.Errorf("Expected 2 neighbors after removal, got %d", len(neighbors))
	}

	// Verify 3 is not in neighbors
	if node.HasNeighbor(0, 3) {
		t.Error("Neighbor 3 should have been removed")
	}

	// Verify others still exist
	if !node.HasNeighbor(0, 2) || !node.HasNeighbor(0, 4) {
		t.Error("Other neighbors should still exist")
	}
}

// TestNodeSetNeighbors tests setting neighbors
func TestNodeSetNeighbors(t *testing.T) {
	node := NewNode(1, []float32{1, 2, 3}, 1)

	// Set neighbors
	newNeighbors := []uint64{10, 20, 30}
	node.SetNeighbors(0, newNeighbors)

	neighbors := node.GetNeighbors(0)
	if len(neighbors) != 3 {
		t.Errorf("Expected 3 neighbors, got %d", len(neighbors))
	}

	// Modify original slice - should not affect node
	newNeighbors[0] = 999
	neighbors = node.GetNeighbors(0)
	if neighbors[0] == 999 {
		t.Error("Node neighbors should not be affected by external modification")
	}
}

// TestNodeHasNeighbor tests neighbor existence check
func TestNodeHasNeighbor(t *testing.T) {
	node := NewNode(1, []float32{1, 2, 3}, 2)

	node.AddNeighbor(0, 5)
	node.AddNeighbor(1, 6)

	if !node.HasNeighbor(0, 5) {
		t.Error("Should have neighbor 5 at layer 0")
	}

	if !node.HasNeighbor(1, 6) {
		t.Error("Should have neighbor 6 at layer 1")
	}

	if node.HasNeighbor(0, 6) {
		t.Error("Should not have neighbor 6 at layer 0")
	}

	if node.HasNeighbor(2, 5) {
		t.Error("Should not have neighbor 5 at layer 2")
	}
}

// TestNodeConcurrency tests thread-safe operations
func TestNodeConcurrency(t *testing.T) {
	node := NewNode(1, []float32{1, 2, 3}, 0)
	var wg sync.WaitGroup

	// Concurrently add neighbors
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id uint64) {
			defer wg.Done()
			node.AddNeighbor(0, id)
		}(uint64(i))
	}

	wg.Wait()

	neighbors := node.GetNeighbors(0)
	if len(neighbors) != 100 {
		t.Errorf("Expected 100 neighbors, got %d", len(neighbors))
	}
}

// TestNewIndex tests index creation
func TestNewIndex(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	if idx.M != 16 {
		t.Errorf("Expected M=16, got %d", idx.M)
	}

	if idx.M0 != 32 {
		t.Errorf("Expected M0=32, got %d", idx.M0)
	}

	if idx.efConstruction != 200 {
		t.Errorf("Expected efConstruction=200, got %d", idx.efConstruction)
	}

	if idx.Size() != 0 {
		t.Errorf("New index should have size 0, got %d", idx.Size())
	}

	if idx.MaxLayer() != -1 {
		t.Errorf("New index should have maxLayer=-1, got %d", idx.MaxLayer())
	}
}

// TestRandomLevel tests level distribution
func TestRandomLevel(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	// Generate many levels and check distribution
	levelCounts := make(map[int]int)
	iterations := 10000

	for i := 0; i < iterations; i++ {
		level := idx.randomLevel()
		levelCounts[level]++
	}

	// Most nodes should be at lower levels
	// At least 50% should be at level 0
	if levelCounts[0] < iterations/2 {
		t.Errorf("Expected at least 50%% of nodes at level 0, got %.2f%%",
			float64(levelCounts[0])/float64(iterations)*100)
	}

	// Distribution should follow exponential decay
	// Each level should have fewer nodes than the previous
	for level := 1; level <= 3; level++ {
		if levelCounts[level] >= levelCounts[level-1] {
			// Allow some variance due to randomness
			if float64(levelCounts[level]) > float64(levelCounts[level-1])*1.2 {
				t.Errorf("Level %d has more nodes than level %d (not exponential decay)",
					level, level-1)
			}
		}
	}

	// Should have some nodes at higher levels
	totalHigherLevels := 0
	for level, count := range levelCounts {
		if level > 0 {
			totalHigherLevels += count
		}
	}

	if totalHigherLevels == 0 {
		t.Error("Should have some nodes at levels > 0")
	}

	t.Logf("Level distribution (n=%d):", iterations)
	for level := 0; level <= 5; level++ {
		if count, ok := levelCounts[level]; ok {
			t.Logf("  Level %d: %d (%.2f%%)", level, count,
				float64(count)/float64(iterations)*100)
		}
	}
}

// TestIndexCustomConfig tests index with custom configuration
func TestIndexCustomConfig(t *testing.T) {
	config := IndexConfig{
		M:              32,
		efConstruction: 400,
		DistanceFunc:   EuclideanDistance,
	}
	idx := New(config)

	if idx.M != 32 {
		t.Errorf("Expected M=32, got %d", idx.M)
	}

	if idx.M0 != 64 {
		t.Errorf("Expected M0=64, got %d", idx.M0)
	}

	if idx.efConstruction != 400 {
		t.Errorf("Expected efConstruction=400, got %d", idx.efConstruction)
	}

	// Test distance function
	vec1 := []float32{0, 0}
	vec2 := []float32{3, 4}
	dist := idx.distance(vec1, vec2)
	if !almostEqual(dist, 5.0) {
		t.Errorf("Expected Euclidean distance 5.0, got %f", dist)
	}
}

// TestIndexStats tests statistics gathering
func TestIndexStats(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	stats := idx.GetStats()
	if stats.Size != 0 {
		t.Errorf("Expected size 0, got %d", stats.Size)
	}

	if stats.MaxLayer != -1 {
		t.Errorf("Expected maxLayer -1, got %d", stats.MaxLayer)
	}

	if len(stats.NodesPerLayer) != 0 {
		t.Errorf("Expected 0 layers, got %d", len(stats.NodesPerLayer))
	}
}

// BenchmarkRandomLevel benchmarks level generation
func BenchmarkRandomLevel(b *testing.B) {
	config := DefaultConfig()
	idx := New(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.randomLevel()
	}
}

// BenchmarkNodeAddNeighbor benchmarks neighbor addition
func BenchmarkNodeAddNeighbor(b *testing.B) {
	node := NewNode(1, []float32{1, 2, 3}, 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.AddNeighbor(0, uint64(i%1000))
	}
}

// BenchmarkNodeGetNeighbors benchmarks neighbor retrieval
func BenchmarkNodeGetNeighbors(b *testing.B) {
	node := NewNode(1, []float32{1, 2, 3}, 3)

	// Add some neighbors
	for i := 0; i < 100; i++ {
		node.AddNeighbor(0, uint64(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.GetNeighbors(0)
	}
}
