package nsg

import (
	"sync"
)

// Node represents a vector in the NSG graph (single-layer)
type Node struct {
	id     uint64      // Unique identifier for the node
	vector []float32   // The vector embedding

	// neighbors contains the outgoing edge IDs
	// NSG uses a single-layer graph with optimized connectivity
	neighbors []uint64

	// Mutex for thread-safe operations
	mu sync.RWMutex
}

// NewNode creates a new node with the given ID and vector
func NewNode(id uint64, vector []float32) *Node {
	return &Node{
		id:        id,
		vector:    vector,
		neighbors: make([]uint64, 0),
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

// AddNeighbor adds a neighbor (thread-safe)
func (n *Node) AddNeighbor(neighborID uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Check if neighbor already exists to avoid duplicates
	for _, id := range n.neighbors {
		if id == neighborID {
			return
		}
	}

	n.neighbors = append(n.neighbors, neighborID)
}

// RemoveNeighbor removes a neighbor (thread-safe)
func (n *Node) RemoveNeighbor(neighborID uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Find and remove the neighbor
	for i, id := range n.neighbors {
		if id == neighborID {
			// Remove by replacing with last element and truncating
			n.neighbors[i] = n.neighbors[len(n.neighbors)-1]
			n.neighbors = n.neighbors[:len(n.neighbors)-1]
			return
		}
	}
}

// GetNeighbors returns a copy of neighbors (thread-safe)
func (n *Node) GetNeighbors() []uint64 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Return a copy to prevent external modification
	neighbors := make([]uint64, len(n.neighbors))
	copy(neighbors, n.neighbors)
	return neighbors
}

// SetNeighbors replaces all neighbors (thread-safe)
func (n *Node) SetNeighbors(neighbors []uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Create a new slice to avoid external modification
	n.neighbors = make([]uint64, len(neighbors))
	copy(n.neighbors, neighbors)
}

// NeighborCount returns the number of neighbors (thread-safe)
func (n *Node) NeighborCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return len(n.neighbors)
}

// HasNeighbor checks if a node is a neighbor (thread-safe)
func (n *Node) HasNeighbor(neighborID uint64) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for _, id := range n.neighbors {
		if id == neighborID {
			return true
		}
	}
	return false
}
