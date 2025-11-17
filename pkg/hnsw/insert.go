package hnsw

import (
	"container/heap"
	"fmt"
)

// Insert adds a vector to the HNSW index
// Returns the ID of the inserted node
func (idx *Index) Insert(vector []float32) (uint64, error) {
	if len(vector) == 0 {
		return 0, fmt.Errorf("cannot insert empty vector")
	}

	idx.mu.Lock()

	// Set dimension on first insert
	if idx.dimension == 0 {
		idx.dimension = len(vector)
	} else if len(vector) != idx.dimension {
		idx.mu.Unlock()
		return 0, fmt.Errorf("vector dimension mismatch: expected %d, got %d",
			idx.dimension, len(vector))
	}

	// Generate unique ID for the new node
	nodeID := idx.nodeCounter
	idx.nodeCounter++

	// Assign random level for the new node
	level := idx.randomLevel()

	// Create the new node
	newNode := NewNode(nodeID, vector, level)

	// Handle first insertion (entry point initialization)
	if idx.entryPoint == nil {
		idx.nodes[nodeID] = newNode
		idx.entryPoint = newNode
		idx.maxLayer = level
		idx.size++
		idx.mu.Unlock()
		return nodeID, nil
	}

	// For subsequent insertions, we need to find nearest neighbors
	entryPoint := idx.entryPoint
	currentMaxLayer := idx.maxLayer

	idx.mu.Unlock()

	// Phase 1: Search for nearest neighbors from top layer to target layer+1
	// We do greedy search without expanding candidates on upper layers
	ep := entryPoint
	currentDist := idx.distanceToNode(vector, ep)

	// Search from top layer down to level+1
	for lc := currentMaxLayer; lc > level; lc-- {
		changed := true
		for changed {
			changed = false

			// Check all neighbors at current layer
			neighbors := ep.GetNeighbors(lc)
			for _, neighborID := range neighbors {
				neighborNode := idx.GetNode(neighborID)
				if neighborNode == nil {
					continue
				}

				dist := idx.distanceToNode(vector, neighborNode)
				if dist < currentDist {
					currentDist = dist
					ep = neighborNode
					changed = true
				}
			}
		}
	}

	// Phase 2: For each layer from level down to 0, find M nearest neighbors
	// and insert bidirectional links
	for lc := min(level, currentMaxLayer); lc >= 0; lc-- {
		// Search for efConstruction nearest neighbors at layer lc
		candidates := idx.searchLayer(vector, ep, idx.efConstruction, lc)

		// Select M neighbors using heuristic
		M := idx.M
		if lc == 0 {
			M = idx.M0
		}

		neighbors := idx.selectNeighbors(candidates, M, lc)

		// Add bidirectional links
		for _, neighbor := range neighbors {
			neighborNode := idx.GetNode(neighbor)
			if neighborNode != nil {
				newNode.AddNeighbor(lc, neighbor)
				neighborNode.AddNeighbor(lc, nodeID)

				// Prune neighbors if needed
				idx.pruneNeighbors(neighborNode, lc)
			}
		}

		// Update entry point for next layer
		if len(candidates) > 0 {
			ep = idx.GetNode(candidates[0].id)
		}
	}

	// Add node to index
	idx.mu.Lock()
	idx.nodes[nodeID] = newNode

	// Update entry point if new node has higher level
	if level > idx.maxLayer {
		idx.maxLayer = level
		idx.entryPoint = newNode
	}

	idx.size++
	idx.mu.Unlock()

	return nodeID, nil
}

// searchLayer performs a greedy search for the ef nearest neighbors at a specific layer
// Returns a priority queue of candidates sorted by distance (closest first)
func (idx *Index) searchLayer(query []float32, entryPoint *Node, ef int, layer int) []heapItem {
	visited := make(map[uint64]bool)
	candidates := &minHeap{}
	results := &maxHeap{}

	// Start with entry point
	dist := idx.distanceToNode(query, entryPoint)
	heap.Push(candidates, heapItem{id: entryPoint.ID(), distance: dist})
	heap.Push(results, heapItem{id: entryPoint.ID(), distance: dist})
	visited[entryPoint.ID()] = true

	// Greedy search
	for candidates.Len() > 0 {
		// Get closest candidate
		current := heap.Pop(candidates).(heapItem)

		// If current is farther than the worst result, we're done
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
			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			neighborNode := idx.GetNode(neighborID)
			if neighborNode == nil {
				continue
			}

			neighborDist := idx.distanceToNode(query, neighborNode)

			// If neighbor is closer than worst result, or we haven't found ef results yet
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

// selectNeighbors selects M neighbors from candidates using a heuristic
// This implements the neighbor selection heuristic from the HNSW paper
func (idx *Index) selectNeighbors(candidates []heapItem, M int, layer int) []uint64 {
	if len(candidates) <= M {
		result := make([]uint64, len(candidates))
		for i, c := range candidates {
			result[i] = c.id
		}
		return result
	}

	// Simple heuristic: select M closest candidates
	// More sophisticated heuristic could consider diversity
	result := make([]uint64, M)
	for i := 0; i < M; i++ {
		result[i] = candidates[i].id
	}
	return result
}

// pruneNeighbors ensures a node doesn't have more than M connections at a layer
func (idx *Index) pruneNeighbors(node *Node, layer int) {
	M := idx.M
	if layer == 0 {
		M = idx.M0
	}

	neighbors := node.GetNeighbors(layer)
	if len(neighbors) <= M {
		return
	}

	// Calculate distances to all neighbors
	type neighborDist struct {
		id   uint64
		dist float32
	}

	distances := make([]neighborDist, len(neighbors))
	for i, neighborID := range neighbors {
		neighborNode := idx.GetNode(neighborID)
		if neighborNode != nil {
			dist := idx.distanceBetweenNodes(node, neighborNode)
			distances[i] = neighborDist{id: neighborID, dist: dist}
		}
	}

	// Sort by distance (keep M closest)
	// Simple selection: just keep first M by distance
	// More sophisticated: could use heuristic to maintain diversity
	selectedIDs := make([]uint64, 0, M)

	// Find M closest neighbors
	for len(selectedIDs) < M && len(distances) > 0 {
		minIdx := 0
		minDist := distances[0].dist

		for i := 1; i < len(distances); i++ {
			if distances[i].dist < minDist {
				minDist = distances[i].dist
				minIdx = i
			}
		}

		selectedIDs = append(selectedIDs, distances[minIdx].id)

		// Remove selected element
		distances = append(distances[:minIdx], distances[minIdx+1:]...)
	}

	// Update neighbors
	node.SetNeighbors(layer, selectedIDs)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// heapItem represents an item in the priority queue
type heapItem struct {
	id       uint64
	distance float32
}

// minHeap is a min-heap of heapItem (smallest distance at top)
type minHeap []heapItem

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i].distance < h[j].distance }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *minHeap) Push(x interface{}) {
	*h = append(*h, x.(heapItem))
}

func (h *minHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (h *minHeap) Peek() interface{} {
	if len(*h) == 0 {
		return heapItem{distance: 1e9}
	}
	return (*h)[0]
}

// maxHeap is a max-heap of heapItem (largest distance at top)
type maxHeap []heapItem

func (h maxHeap) Len() int           { return len(h) }
func (h maxHeap) Less(i, j int) bool { return h[i].distance > h[j].distance }
func (h maxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *maxHeap) Push(x interface{}) {
	*h = append(*h, x.(heapItem))
}

func (h *maxHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (h *maxHeap) Peek() interface{} {
	if len(*h) == 0 {
		return heapItem{distance: 1e9}
	}
	return (*h)[0]
}
