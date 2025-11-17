package hnsw

import (
	"math/rand"
	"testing"
)

// TestDebugGraphStructure helps debug graph construction
func TestDebugGraphStructure(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 10
	count := 20

	// Insert vectors
	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		idx.Insert(vec)
	}

	t.Logf("Graph structure after %d insertions:", count)
	t.Logf("  Max layer: %d", idx.MaxLayer())
	t.Logf("  Entry point: %d (level %d)", idx.EntryPoint().ID(), idx.EntryPoint().Level())

	// Check connectivity at layer 0
	totalNeighbors := 0
	nodesWithNoNeighbors := 0

	for i := 0; i < count; i++ {
		node := idx.GetNode(uint64(i))
		if node == nil {
			continue
		}

		neighbors := node.GetNeighbors(0)
		totalNeighbors += len(neighbors)

		if len(neighbors) == 0 {
			nodesWithNoNeighbors++
			t.Logf("  Node %d has NO neighbors at layer 0!", i)
		}
	}

	avgNeighbors := float64(totalNeighbors) / float64(count)
	t.Logf("  Average neighbors at layer 0: %.2f", avgNeighbors)
	t.Logf("  Nodes with no neighbors: %d", nodesWithNoNeighbors)

	if nodesWithNoNeighbors > 1 {
		t.Errorf("Too many nodes without neighbors: %d", nodesWithNoNeighbors)
	}

	// Test a simple search
	query := make([]float32, dim)
	for j := 0; j < dim; j++ {
		query[j] = rng.Float32()
	}

	result, err := idx.Search(query, 5, 20)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	t.Logf("  Search visited %d nodes", result.Visited)
	t.Logf("  Search returned %d results", len(result.Results))

	if result.Visited < 5 {
		t.Errorf("Search visited too few nodes: %d (index has %d nodes)", result.Visited, count)
	}
}

// TestDebugSimpleInsert tests insertion step by step
func TestDebugSimpleInsert(t *testing.T) {
	config := IndexConfig{
		M:              4,
		efConstruction: 10,
		DistanceFunc:   EuclideanDistance,
	}
	idx := New(config)

	// Insert first 5 vectors
	vectors := [][]float32{
		{1.0, 0.0},
		{0.9, 0.1},
		{0.0, 1.0},
		{0.1, 0.9},
		{0.5, 0.5},
	}

	for i, vec := range vectors {
		id, err := idx.Insert(vec)
		if err != nil {
			t.Fatalf("Insert %d failed: %v", i, err)
		}

		t.Logf("Inserted vector %d (ID=%d)", i, id)

		// Check graph after each insertion
		for j := 0; j <= i; j++ {
			node := idx.GetNode(uint64(j))
			if node != nil {
				neighbors := node.GetNeighbors(0)
				t.Logf("  Node %d: %d neighbors at layer 0", j, len(neighbors))
				if len(neighbors) > 0 {
					t.Logf("    Neighbors: %v", neighbors)
				}
			}
		}
		t.Logf("")
	}

	// Now test search
	query := vectors[0] // Search for first vector
	result, err := idx.Search(query, 3, 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	t.Logf("Search for vector 0:")
	t.Logf("  Visited: %d nodes", result.Visited)
	for i, r := range result.Results {
		t.Logf("  Result %d: ID=%d, distance=%.4f", i, r.ID, r.Distance)
	}

	// First result should be ID 0 with distance ~0
	if len(result.Results) == 0 {
		t.Fatal("No results returned")
	}

	if result.Results[0].ID != 0 {
		t.Errorf("Expected first result to be ID 0, got %d", result.Results[0].ID)
	}
}

// TestSearchLayerDebug tests the searchLayer function directly
func TestSearchLayerDebug(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	// Insert a few vectors
	vectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{0.0, 0.0, 1.0},
	}

	for _, vec := range vectors {
		idx.Insert(vec)
	}

	// Search for a query similar to first vector
	query := []float32{0.95, 0.05, 0.0}
	entryPoint := idx.EntryPoint()

	t.Logf("Entry point: ID=%d, level=%d", entryPoint.ID(), entryPoint.Level())
	t.Logf("Entry point neighbors at layer 0: %v", entryPoint.GetNeighbors(0))

	candidates := idx.searchLayer(query, entryPoint, 10, 0)

	t.Logf("searchLayer returned %d candidates:", len(candidates))
	for i, c := range candidates {
		t.Logf("  %d: ID=%d, distance=%.4f", i, c.id, c.distance)
	}

	if len(candidates) == 0 {
		t.Error("searchLayer returned no candidates")
	}
}
