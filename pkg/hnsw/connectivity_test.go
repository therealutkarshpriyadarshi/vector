package hnsw

import (
	"math/rand"
	"testing"
)

// TestGraphReachability checks if all nodes are reachable from entry point
func TestGraphReachability(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 10
	count := 100

	// Insert vectors
	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		idx.Insert(vec)
	}

	// BFS from entry point at layer 0
	visited := make(map[uint64]bool)
	queue := []uint64{idx.EntryPoint().ID()}
	visited[idx.EntryPoint().ID()] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		node := idx.GetNode(current)
		if node == nil {
			continue
		}

		neighbors := node.GetNeighbors(0)
		for _, neighborID := range neighbors {
			if !visited[neighborID] {
				visited[neighborID] = true
				queue = append(queue, neighborID)
			}
		}
	}

	unreachable := []uint64{}
	for i := 0; i < count; i++ {
		if !visited[uint64(i)] {
			unreachable = append(unreachable, uint64(i))
		}
	}

	t.Logf("Reachable nodes: %d/%d", len(visited), count)
	t.Logf("Unreachable nodes: %d", len(unreachable))

	if len(unreachable) > 0 {
		t.Logf("Unreachable node IDs: %v", unreachable)

		// Check neighbors of unreachable nodes
		for _, id := range unreachable[:min(5, len(unreachable))] {
			node := idx.GetNode(id)
			if node != nil {
				neighbors := node.GetNeighbors(0)
				t.Logf("  Node %d has %d neighbors: %v", id, len(neighbors), neighbors)
			}
		}
	}

	if len(unreachable) > count/10 {
		t.Errorf("Too many unreachable nodes: %d/%d", len(unreachable), count)
	}
}

// TestBidirectionalConnections checks that all connections are bidirectional
func TestBidirectionalConnections(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 10
	count := 50

	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		idx.Insert(vec)
	}

	brokenLinks := 0

	for i := 0; i < count; i++ {
		node := idx.GetNode(uint64(i))
		if node == nil {
			continue
		}

		neighbors := node.GetNeighbors(0)
		for _, neighborID := range neighbors {
			neighborNode := idx.GetNode(neighborID)
			if neighborNode == nil {
				t.Errorf("Node %d has neighbor %d which doesn't exist", i, neighborID)
				brokenLinks++
				continue
			}

			// Check if the link is bidirectional
			if !neighborNode.HasNeighbor(0, uint64(i)) {
				t.Errorf("Node %d -> %d is not bidirectional", i, neighborID)
				brokenLinks++
			}
		}
	}

	t.Logf("Broken or unidirectional links: %d", brokenLinks)

	if brokenLinks > 0 {
		t.Error("Found broken bidirectional links")
	}
}
