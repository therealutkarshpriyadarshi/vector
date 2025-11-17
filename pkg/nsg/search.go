package nsg

import (
	"container/heap"
	"fmt"
	"sort"
)

// SearchResult represents a search result with ID, distance, and vector
type SearchResult struct {
	ID       uint64
	Distance float32
	Vector   []float32
}

// Search finds the K nearest neighbors to the query vector
// NSG uses greedy best-first search starting from the navigating node
func (idx *Index) Search(query []float32, k int) ([]SearchResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if !idx.isBuilt {
		return nil, fmt.Errorf("index not built yet")
	}

	if len(query) != idx.dimension {
		return nil, fmt.Errorf("query dimension mismatch: expected %d, got %d", idx.dimension, len(query))
	}

	if k <= 0 {
		return nil, fmt.Errorf("k must be positive")
	}

	if len(idx.nodes) == 0 {
		return []SearchResult{}, nil
	}

	// Use C (max candidate pool size) for search expansion
	// Increase search pool for better recall
	searchL := idx.C
	if searchL < k*20 {
		searchL = k * 20
	}
	if searchL > len(idx.nodes) {
		searchL = len(idx.nodes)
	}

	// Priority queues for search
	candidates := &priorityQueue{}   // Min-heap: candidates to explore
	visited := make(map[uint64]bool) // Track visited nodes
	results := &priorityQueue{}      // Max-heap: track best k results

	heap.Init(candidates)
	heap.Init(results)

	// Start from navigating node
	navNode, exists := idx.nodes[idx.navigatingID]
	if !exists {
		return nil, fmt.Errorf("navigating node not found")
	}

	navDist := idx.distanceFunc(query, navNode.vector)
	heap.Push(candidates, &item{id: idx.navigatingID, distance: navDist, maxHeap: false})
	visited[idx.navigatingID] = true

	// Best-first search
	for candidates.Len() > 0 {
		// Get closest unvisited candidate
		current := heap.Pop(candidates).(*item)

		// If this candidate is farther than our k-th best result, we can stop
		if results.Len() >= k && current.distance > (*results)[0].distance {
			break
		}

		// Add to results if within top k
		if results.Len() < k {
			heap.Push(results, &item{id: current.id, distance: current.distance, maxHeap: true})
		} else if current.distance < (*results)[0].distance {
			heap.Pop(results)
			heap.Push(results, &item{id: current.id, distance: current.distance, maxHeap: true})
		}

		// Explore neighbors
		currentNode, exists := idx.nodes[current.id]
		if !exists {
			continue
		}

		neighbors := currentNode.GetNeighbors()
		for _, neighborID := range neighbors {
			if visited[neighborID] {
				continue
			}

			visited[neighborID] = true

			neighborNode, exists := idx.nodes[neighborID]
			if !exists {
				continue
			}

			dist := idx.distanceFunc(query, neighborNode.vector)

			// Add to candidates if promising
			if results.Len() < k || dist < (*results)[0].distance {
				heap.Push(candidates, &item{id: neighborID, distance: dist, maxHeap: false})
			}
		}

		// Limit number of visited nodes
		if len(visited) > searchL {
			break
		}
	}

	// Extract results from heap and sort by distance
	resultList := make([]SearchResult, 0, results.Len())
	for results.Len() > 0 {
		item := heap.Pop(results).(*item)
		node, exists := idx.nodes[item.id]
		if !exists {
			continue
		}

		resultList = append(resultList, SearchResult{
			ID:       item.id,
			Distance: item.distance,
			Vector:   node.vector,
		})
	}

	// Sort by distance (ascending)
	sort.Slice(resultList, func(i, j int) bool {
		return resultList[i].Distance < resultList[j].Distance
	})

	// Return top k
	if len(resultList) > k {
		resultList = resultList[:k]
	}

	return resultList, nil
}

// SearchWithFilter finds K nearest neighbors with a custom filter function
// Filter should return true if the node should be included in results
func (idx *Index) SearchWithFilter(query []float32, k int, filter func(uint64) bool) ([]SearchResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if !idx.isBuilt {
		return nil, fmt.Errorf("index not built yet")
	}

	if len(query) != idx.dimension {
		return nil, fmt.Errorf("query dimension mismatch: expected %d, got %d", idx.dimension, len(query))
	}

	// Similar to Search but with filter
	searchL := idx.C
	if searchL < k*10 {
		searchL = k * 10
	}

	candidates := &priorityQueue{}
	visited := make(map[uint64]bool)
	results := &priorityQueue{}

	heap.Init(candidates)
	heap.Init(results)

	// Start from navigating node
	navNode, exists := idx.nodes[idx.navigatingID]
	if !exists {
		return nil, fmt.Errorf("navigating node not found")
	}

	navDist := idx.distanceFunc(query, navNode.vector)
	heap.Push(candidates, &item{id: idx.navigatingID, distance: navDist, maxHeap: false})
	visited[idx.navigatingID] = true

	// Best-first search with filter
	for candidates.Len() > 0 {
		current := heap.Pop(candidates).(*item)

		// Check filter
		if filter != nil && !filter(current.id) {
			// Don't add to results, but continue exploration
		} else {
			// Add to results if passes filter
			if results.Len() < k {
				heap.Push(results, &item{id: current.id, distance: current.distance, maxHeap: true})
			} else if current.distance < (*results)[0].distance {
				heap.Pop(results)
				heap.Push(results, &item{id: current.id, distance: current.distance, maxHeap: true})
			}
		}

		// Explore neighbors
		currentNode, exists := idx.nodes[current.id]
		if !exists {
			continue
		}

		neighbors := currentNode.GetNeighbors()
		for _, neighborID := range neighbors {
			if visited[neighborID] {
				continue
			}

			visited[neighborID] = true

			neighborNode, exists := idx.nodes[neighborID]
			if !exists {
				continue
			}

			dist := idx.distanceFunc(query, neighborNode.vector)

			// Always explore promising neighbors, regardless of filter
			if results.Len() < k || dist < (*results)[0].distance || (filter != nil && filter(neighborID)) {
				heap.Push(candidates, &item{id: neighborID, distance: dist, maxHeap: false})
			}
		}

		if len(visited) > searchL {
			break
		}
	}

	// Extract and sort results
	resultList := make([]SearchResult, 0, results.Len())
	for results.Len() > 0 {
		item := heap.Pop(results).(*item)
		node, exists := idx.nodes[item.id]
		if !exists {
			continue
		}

		resultList = append(resultList, SearchResult{
			ID:       item.id,
			Distance: item.distance,
			Vector:   node.vector,
		})
	}

	sort.Slice(resultList, func(i, j int) bool {
		return resultList[i].Distance < resultList[j].Distance
	})

	if len(resultList) > k {
		resultList = resultList[:k]
	}

	return resultList, nil
}

// RangeSearch finds all neighbors within a given radius
func (idx *Index) RangeSearch(query []float32, radius float32) ([]SearchResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if !idx.isBuilt {
		return nil, fmt.Errorf("index not built yet")
	}

	if len(query) != idx.dimension {
		return nil, fmt.Errorf("query dimension mismatch: expected %d, got %d", idx.dimension, len(query))
	}

	candidates := &priorityQueue{}
	visited := make(map[uint64]bool)
	results := make([]SearchResult, 0)

	heap.Init(candidates)

	// Start from navigating node
	navNode, exists := idx.nodes[idx.navigatingID]
	if !exists {
		return nil, fmt.Errorf("navigating node not found")
	}

	navDist := idx.distanceFunc(query, navNode.vector)
	heap.Push(candidates, &item{id: idx.navigatingID, distance: navDist, maxHeap: false})
	visited[idx.navigatingID] = true

	// Search for all nodes within radius
	for candidates.Len() > 0 {
		current := heap.Pop(candidates).(*item)

		// Add to results if within radius
		if current.distance <= radius {
			node, exists := idx.nodes[current.id]
			if exists {
				results = append(results, SearchResult{
					ID:       current.id,
					Distance: current.distance,
					Vector:   node.vector,
				})
			}
		}

		// Explore neighbors
		currentNode, exists := idx.nodes[current.id]
		if !exists {
			continue
		}

		neighbors := currentNode.GetNeighbors()
		for _, neighborID := range neighbors {
			if visited[neighborID] {
				continue
			}

			visited[neighborID] = true

			neighborNode, exists := idx.nodes[neighborID]
			if !exists {
				continue
			}

			dist := idx.distanceFunc(query, neighborNode.vector)

			// Explore if within radius or might lead to nodes within radius
			if dist <= radius*2.0 { // Use 2x radius as exploration threshold
				heap.Push(candidates, &item{id: neighborID, distance: dist, maxHeap: false})
			}
		}
	}

	// Sort by distance
	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	return results, nil
}
