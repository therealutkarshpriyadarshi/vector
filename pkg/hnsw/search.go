package hnsw

import (
	"container/heap"
	"fmt"
)

// Result represents a search result with ID and distance
type Result struct {
	ID       uint64  // Node ID
	Distance float32 // Distance to query vector
}

// SearchResult holds the results of a search operation
type SearchResult struct {
	Results []Result // Sorted results (closest first)
	Visited int      // Number of nodes visited during search
}

// Search performs k-NN search for the nearest neighbors of a query vector
// k: number of nearest neighbors to return
// efSearch: size of the dynamic candidate list (controls accuracy vs speed)
//           Higher values give better recall but slower search
//           Typical values: 50-200
func (idx *Index) Search(query []float32, k int, efSearch int) (*SearchResult, error) {
	if len(query) == 0 {
		return nil, fmt.Errorf("query vector cannot be empty")
	}

	idx.mu.RLock()

	if idx.dimension == 0 {
		idx.mu.RUnlock()
		return nil, fmt.Errorf("index is empty")
	}

	if len(query) != idx.dimension {
		idx.mu.RUnlock()
		return nil, fmt.Errorf("query dimension mismatch: expected %d, got %d",
			idx.dimension, len(query))
	}

	if idx.entryPoint == nil {
		idx.mu.RUnlock()
		return nil, fmt.Errorf("index has no entry point")
	}

	// Ensure efSearch is at least k
	if efSearch < k {
		efSearch = k
	}

	entryPoint := idx.entryPoint
	maxLayer := idx.maxLayer

	idx.mu.RUnlock()

	// Phase 1: Greedy search from top layer to layer 1
	// Find the closest node by greedily traversing down the layers
	ep := entryPoint
	currentDist := idx.distance(query, ep.vector)
	visited := 1

	// Traverse from top layer down to layer 1
	for lc := maxLayer; lc > 0; lc-- {
		changed := true
		for changed {
			changed = false

			neighbors := ep.GetNeighbors(lc)
			for _, neighborID := range neighbors {
				visited++
				neighborNode := idx.GetNode(neighborID)
				if neighborNode == nil {
					continue
				}

				dist := idx.distance(query, neighborNode.vector)
				if dist < currentDist {
					currentDist = dist
					ep = neighborNode
					changed = true
				}
			}
		}
	}

	// Phase 2: Search layer 0 with efSearch candidates
	candidates := idx.searchLayerForQuery(query, ep, efSearch, 0, &visited)

	// Select top-k results
	results := make([]Result, 0, k)
	for i := 0; i < len(candidates) && i < k; i++ {
		results = append(results, Result{
			ID:       candidates[i].id,
			Distance: candidates[i].distance,
		})
	}

	return &SearchResult{
		Results: results,
		Visited: visited,
	}, nil
}

// searchLayerForQuery is similar to searchLayer but used for querying
// It returns sorted results (closest first) and tracks visited nodes
func (idx *Index) searchLayerForQuery(query []float32, entryPoint *Node, ef int, layer int, visited *int) []heapItem {
	visitedSet := make(map[uint64]bool)
	candidates := &minHeap{}
	results := &maxHeap{}

	// Start with entry point
	dist := idx.distance(query, entryPoint.vector)
	heap.Push(candidates, heapItem{id: entryPoint.ID(), distance: dist})
	heap.Push(results, heapItem{id: entryPoint.ID(), distance: dist})
	visitedSet[entryPoint.ID()] = true
	*visited++

	// Greedy search with ef candidates
	for candidates.Len() > 0 {
		// Get closest candidate
		current := heap.Pop(candidates).(heapItem)

		// If current is farther than worst result, we can stop
		if current.distance > results.Peek().(heapItem).distance {
			break
		}

		// Explore neighbors
		currentNode := idx.GetNode(current.id)
		if currentNode == nil {
			continue
		}

		neighbors := currentNode.GetNeighbors(layer)
		for _, neighborID := range neighbors {
			if visitedSet[neighborID] {
				continue
			}
			visitedSet[neighborID] = true
			*visited++

			neighborNode := idx.GetNode(neighborID)
			if neighborNode == nil {
				continue
			}

			neighborDist := idx.distance(query, neighborNode.vector)

			// If neighbor is closer than worst result, or we need more results
			if neighborDist < results.Peek().(heapItem).distance || results.Len() < ef {
				heap.Push(candidates, heapItem{id: neighborID, distance: neighborDist})
				heap.Push(results, heapItem{id: neighborID, distance: neighborDist})

				// Keep only ef closest results
				if results.Len() > ef {
					heap.Pop(results)
				}
			}
		}
	}

	// Convert max heap to sorted slice (closest first)
	resultSlice := make([]heapItem, results.Len())
	for i := len(resultSlice) - 1; i >= 0; i-- {
		resultSlice[i] = heap.Pop(results).(heapItem)
	}

	return resultSlice
}

// KNNSearch is a convenience method for k-NN search with default efSearch
// Uses efSearch = max(k*2, 50) for good accuracy
func (idx *Index) KNNSearch(query []float32, k int) (*SearchResult, error) {
	efSearch := k * 2
	if efSearch < 50 {
		efSearch = 50
	}
	return idx.Search(query, k, efSearch)
}

// GetVector retrieves a vector by its ID
func (idx *Index) GetVector(id uint64) ([]float32, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	node := idx.nodes[id]
	if node == nil {
		return nil, fmt.Errorf("node with ID %d not found", id)
	}

	// Return a copy to prevent external modification
	vector := make([]float32, len(node.vector))
	copy(vector, node.vector)
	return vector, nil
}

// Delete removes a vector from the index by ID
// This is a simplified implementation that removes the node and updates neighbor links
func (idx *Index) Delete(id uint64) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	node := idx.nodes[id]
	if node == nil {
		return fmt.Errorf("node with ID %d not found", id)
	}

	// Remove all bidirectional links
	for layer := 0; layer <= node.level; layer++ {
		neighbors := node.GetNeighbors(layer)
		for _, neighborID := range neighbors {
			neighborNode := idx.nodes[neighborID]
			if neighborNode != nil {
				neighborNode.RemoveNeighbor(layer, id)
			}
		}
	}

	// If this was the entry point, find a new one
	if idx.entryPoint != nil && idx.entryPoint.ID() == id {
		// Find a node with the highest level as new entry point
		var newEntry *Node
		maxLevel := -1

		for _, n := range idx.nodes {
			if n.ID() != id && n.level > maxLevel {
				maxLevel = n.level
				newEntry = n
			}
		}

		idx.entryPoint = newEntry
		idx.maxLayer = maxLevel
	}

	// Remove the node
	delete(idx.nodes, id)
	idx.size--

	return nil
}

// Update updates a vector in the index
// This is implemented as delete + insert
func (idx *Index) Update(id uint64, newVector []float32) error {
	// Check if node exists
	idx.mu.RLock()
	_, exists := idx.nodes[id]
	idx.mu.RUnlock()

	if !exists {
		return fmt.Errorf("node with ID %d not found", id)
	}

	// Delete old vector
	if err := idx.Delete(id); err != nil {
		return fmt.Errorf("failed to delete old vector: %w", err)
	}

	// Insert new vector (will get a new ID)
	_, err := idx.Insert(newVector)
	if err != nil {
		return fmt.Errorf("failed to insert new vector: %w", err)
	}

	return nil
}
