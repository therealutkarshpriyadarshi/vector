package nsg

import (
	"container/heap"
	"math"
)

// findNavigatingNode finds the approximate centroid of all vectors
// This node serves as the entry point for all searches
func (idx *Index) findNavigatingNode() uint64 {
	if len(idx.nodes) == 0 {
		return 0
	}

	// Calculate centroid vector
	centroid := make([]float32, idx.dimension)
	count := float32(len(idx.nodes))

	for _, node := range idx.nodes {
		for i, val := range node.vector {
			centroid[i] += val / count
		}
	}

	// Find node closest to centroid
	var closestID uint64
	minDist := float32(math.MaxFloat32)

	for id, node := range idx.nodes {
		dist := idx.distanceFunc(node.vector, centroid)
		if dist < minDist {
			minDist = dist
			closestID = id
		}
	}

	return closestID
}

// buildKNNGraph builds an initial K-nearest neighbor graph
func (idx *Index) buildKNNGraph() map[uint64][]uint64 {
	knnGraph := make(map[uint64][]uint64)

	// For each node, find its K nearest neighbors
	for id, node := range idx.nodes {
		neighbors := idx.findKNN(node.vector, id, idx.L)
		knnGraph[id] = neighbors
	}

	return knnGraph
}

// findKNN finds K nearest neighbors for a vector (excluding excludeID)
func (idx *Index) findKNN(query []float32, excludeID uint64, k int) []uint64 {
	// Use a max-heap to track top K candidates
	pq := &priorityQueue{}
	heap.Init(pq)

	// Calculate distances to all nodes
	for id, node := range idx.nodes {
		if id == excludeID {
			continue
		}

		dist := idx.distanceFunc(query, node.vector)

		if pq.Len() < k {
			heap.Push(pq, &item{id: id, distance: dist, maxHeap: true})
		} else if dist < (*pq)[0].distance {
			heap.Pop(pq)
			heap.Push(pq, &item{id: id, distance: dist, maxHeap: true})
		}
	}

	// Extract IDs from heap
	result := make([]uint64, pq.Len())
	for i := pq.Len() - 1; i >= 0; i-- {
		result[i] = heap.Pop(pq).(*item).id
	}

	return result
}

// refineToNSG refines the KNN graph to an NSG graph with monotonic search paths
func (idx *Index) refineToNSG(knnGraph map[uint64][]uint64) {
	// For each node, select R best neighbors that form monotonic paths
	for id, node := range idx.nodes {
		// Find path from navigating node to current node
		pathNodes := idx.findPath(idx.navigatingID, id, knnGraph)

		// Select best R neighbors that maintain monotonicity
		candidates := knnGraph[id]
		selectedNeighbors := idx.selectMonotonicNeighbors(id, node.vector, candidates, pathNodes)

		// Set neighbors in NSG
		node.SetNeighbors(selectedNeighbors)
	}
}

// findPath finds a path from source to target using the KNN graph
func (idx *Index) findPath(sourceID, targetID uint64, knnGraph map[uint64][]uint64) []uint64 {
	if sourceID == targetID {
		return []uint64{sourceID}
	}

	// Use greedy search to find path
	visited := make(map[uint64]bool)
	path := []uint64{sourceID}
	current := sourceID

	targetNode, exists := idx.nodes[targetID]
	if !exists {
		return path
	}

	for len(path) < 100 { // Limit path length
		visited[current] = true

		// Find closest unvisited neighbor to target
		neighbors := knnGraph[current]
		if len(neighbors) == 0 {
			break
		}

		var nextID uint64
		minDist := float32(math.MaxFloat32)
		found := false

		currentNode, _ := idx.nodes[current]

		for _, neighborID := range neighbors {
			if visited[neighborID] {
				continue
			}

			neighborNode, exists := idx.nodes[neighborID]
			if !exists {
				continue
			}

			dist := idx.distanceFunc(neighborNode.vector, targetNode.vector)
			if dist < minDist {
				minDist = dist
				nextID = neighborID
				found = true
			}
		}

		if !found {
			// No unvisited neighbors, try to reach target directly
			if contains(neighbors, targetID) && !visited[targetID] {
				path = append(path, targetID)
				break
			}
			break
		}

		path = append(path, nextID)
		current = nextID

		// Check if we reached target
		if current == targetID {
			break
		}

		// If we're getting closer to target, continue
		currentDist := idx.distanceFunc(currentNode.vector, targetNode.vector)
		nextNode, _ := idx.nodes[nextID]
		nextDist := idx.distanceFunc(nextNode.vector, targetNode.vector)

		// If not getting closer, stop to avoid infinite loops
		if nextDist >= currentDist {
			break
		}
	}

	return path
}

// selectMonotonicNeighbors selects R neighbors that form monotonic paths
// Simplified version: just select R closest neighbors from KNN graph for better connectivity
func (idx *Index) selectMonotonicNeighbors(nodeID uint64, nodeVec []float32, candidates []uint64, pathNodes []uint64) []uint64 {
	// Score each candidate by distance to current node
	type scoredCandidate struct {
		id       uint64
		distance float32
	}

	scored := make([]scoredCandidate, 0, len(candidates))

	for _, candidateID := range candidates {
		if candidateID == nodeID {
			continue
		}

		candidateNode, exists := idx.nodes[candidateID]
		if !exists {
			continue
		}

		// Simple distance-based selection for better connectivity
		dist := idx.distanceFunc(nodeVec, candidateNode.vector)
		scored = append(scored, scoredCandidate{id: candidateID, distance: dist})
	}

	// Sort by distance (closest first)
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].distance < scored[i].distance {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Take top R candidates
	maxNeighbors := idx.R
	if len(scored) < maxNeighbors {
		maxNeighbors = len(scored)
	}

	result := make([]uint64, maxNeighbors)
	for i := 0; i < maxNeighbors; i++ {
		result[i] = scored[i].id
	}

	return result
}

// contains checks if a slice contains a value
func contains(slice []uint64, val uint64) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// Priority queue implementation for nearest neighbor search
type item struct {
	id       uint64
	distance float32
	maxHeap  bool // true for max-heap, false for min-heap
	index    int  // index in the heap
}

type priorityQueue []*item

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// For max-heap: reverse comparison
	if pq[i].maxHeap {
		return pq[i].distance > pq[j].distance
	}
	// For min-heap: normal comparison
	return pq[i].distance < pq[j].distance
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*item)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}
