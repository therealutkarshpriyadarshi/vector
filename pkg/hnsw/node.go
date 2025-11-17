package hnsw

import (
	"sync"
)

// Node represents a vector in the HNSW graph with multi-layer connections
type Node struct {
	id     uint64      // Unique identifier for the node
	vector []float32   // The vector embedding
	level  int         // Maximum layer this node appears in

	// neighbors[layer] contains the neighbor IDs at each layer
	// Layer 0 is the base layer with all nodes
	neighbors [][]uint64

	// Mutex for thread-safe operations
	mu sync.RWMutex
}

// NewNode creates a new node with the given ID, vector, and level
func NewNode(id uint64, vector []float32, level int) *Node {
	// Initialize neighbors slice with empty slices for each layer
	neighbors := make([][]uint64, level+1)
	for i := range neighbors {
		neighbors[i] = make([]uint64, 0)
	}

	return &Node{
		id:        id,
		vector:    vector,
		level:     level,
		neighbors: neighbors,
	}
}

// ID returns the node's unique identifier
func (n *Node) ID() uint64 {
	return n.id
}

// Vector returns the node's vector embedding
func (n *Node) Vector() []float32 {
	return n.vector
}

// Level returns the maximum layer this node appears in
func (n *Node) Level() int {
	return n.level
}

// AddNeighbor adds a neighbor at the specified layer (thread-safe)
func (n *Node) AddNeighbor(layer int, neighborID uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if layer < 0 || layer > n.level {
		return
	}

	// Check if neighbor already exists to avoid duplicates
	for _, id := range n.neighbors[layer] {
		if id == neighborID {
			return
		}
	}

	n.neighbors[layer] = append(n.neighbors[layer], neighborID)
}

// RemoveNeighbor removes a neighbor from the specified layer (thread-safe)
func (n *Node) RemoveNeighbor(layer int, neighborID uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if layer < 0 || layer > n.level {
		return
	}

	// Find and remove the neighbor
	for i, id := range n.neighbors[layer] {
		if id == neighborID {
			// Remove by replacing with last element and truncating
			n.neighbors[layer][i] = n.neighbors[layer][len(n.neighbors[layer])-1]
			n.neighbors[layer] = n.neighbors[layer][:len(n.neighbors[layer])-1]
			return
		}
	}
}

// GetNeighbors returns a copy of neighbors at the specified layer (thread-safe)
func (n *Node) GetNeighbors(layer int) []uint64 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if layer < 0 || layer > n.level {
		return []uint64{}
	}

	// Return a copy to prevent external modification
	neighbors := make([]uint64, len(n.neighbors[layer]))
	copy(neighbors, n.neighbors[layer])
	return neighbors
}

// SetNeighbors replaces all neighbors at the specified layer (thread-safe)
func (n *Node) SetNeighbors(layer int, neighbors []uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if layer < 0 || layer > n.level {
		return
	}

	// Create a new slice to avoid external modification
	n.neighbors[layer] = make([]uint64, len(neighbors))
	copy(n.neighbors[layer], neighbors)
}

// NeighborCount returns the number of neighbors at the specified layer (thread-safe)
func (n *Node) NeighborCount(layer int) int {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if layer < 0 || layer > n.level {
		return 0
	}

	return len(n.neighbors[layer])
}

// HasNeighbor checks if a node is a neighbor at the specified layer (thread-safe)
func (n *Node) HasNeighbor(layer int, neighborID uint64) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if layer < 0 || layer > n.level {
		return false
	}

	for _, id := range n.neighbors[layer] {
		if id == neighborID {
			return true
		}
	}
	return false
}

// GetAllNeighbors returns all neighbors across all layers (thread-safe)
// Returns a map[layer][]neighborIDs
func (n *Node) GetAllNeighbors() map[int][]uint64 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	result := make(map[int][]uint64)
	for layer, neighbors := range n.neighbors {
		if len(neighbors) > 0 {
			result[layer] = make([]uint64, len(neighbors))
			copy(result[layer], neighbors)
		}
	}
	return result
}
