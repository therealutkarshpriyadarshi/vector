package diskann

import (
	"sync"
)

// MemoryGraph represents a small in-memory graph for fast initial search
// This is the key innovation of DiskANN: keep a small graph in memory for routing,
// while the full graph resides on disk
type MemoryGraph struct {
	maxSize    int                      // Maximum number of nodes to keep in memory
	R          int                      // Number of neighbors per node
	nodes      map[uint64]*MemoryNode   // In-memory nodes (subset of full graph)
	entryPoint uint64                   // Entry point for search (medoid)
	mu         sync.RWMutex             // Protects concurrent access
}

// MemoryNode represents a node in the memory graph
type MemoryNode struct {
	ID        uint64    // Node ID
	Vector    []float32 // Full precision vector (for accurate distance computation)
	Neighbors []uint64  // Neighbor IDs
	PQCode    []byte    // Product quantization code (optional, for memory savings)
}

// NewMemoryGraph creates a new memory graph
func NewMemoryGraph(maxSize, R int) *MemoryGraph {
	return &MemoryGraph{
		maxSize: maxSize,
		R:       R,
		nodes:   make(map[uint64]*MemoryNode),
	}
}

// AddNode adds a node to the memory graph
func (mg *MemoryGraph) AddNode(id uint64, vector []float32, neighbors []uint64) error {
	mg.mu.Lock()
	defer mg.mu.Unlock()

	// If we've reached capacity, we would need to evict nodes
	// For simplicity, we'll just store the first maxSize nodes
	// A more sophisticated implementation would keep the "most important" nodes
	if len(mg.nodes) >= mg.maxSize && mg.nodes[id] == nil {
		return nil // Silently skip if full (not ideal, but simple)
	}

	vectorCopy := make([]float32, len(vector))
	copy(vectorCopy, vector)

	neighborsCopy := make([]uint64, len(neighbors))
	copy(neighborsCopy, neighbors)

	mg.nodes[id] = &MemoryNode{
		ID:        id,
		Vector:    vectorCopy,
		Neighbors: neighborsCopy,
	}

	return nil
}

// GetNode retrieves a node from the memory graph
func (mg *MemoryGraph) GetNode(id uint64) (*MemoryNode, bool) {
	mg.mu.RLock()
	defer mg.mu.RUnlock()

	node, exists := mg.nodes[id]
	return node, exists
}

// SetEntryPoint sets the entry point for search
func (mg *MemoryGraph) SetEntryPoint(id uint64) {
	mg.mu.Lock()
	defer mg.mu.Unlock()

	mg.entryPoint = id
}

// GetEntryPoint returns the entry point for search
func (mg *MemoryGraph) GetEntryPoint() uint64 {
	mg.mu.RLock()
	defer mg.mu.RUnlock()

	return mg.entryPoint
}

// Size returns the number of nodes in the memory graph
func (mg *MemoryGraph) Size() int {
	mg.mu.RLock()
	defer mg.mu.RUnlock()

	return len(mg.nodes)
}

// UpdateNeighbors updates the neighbors of a node
func (mg *MemoryGraph) UpdateNeighbors(id uint64, neighbors []uint64) {
	mg.mu.Lock()
	defer mg.mu.Unlock()

	if node, exists := mg.nodes[id]; exists {
		node.Neighbors = make([]uint64, len(neighbors))
		copy(node.Neighbors, neighbors)
	}
}

// Contains checks if a node is in the memory graph
func (mg *MemoryGraph) Contains(id uint64) bool {
	mg.mu.RLock()
	defer mg.mu.RUnlock()

	_, exists := mg.nodes[id]
	return exists
}

// GetAllNodes returns all node IDs in the memory graph
func (mg *MemoryGraph) GetAllNodes() []uint64 {
	mg.mu.RLock()
	defer mg.mu.RUnlock()

	ids := make([]uint64, 0, len(mg.nodes))
	for id := range mg.nodes {
		ids = append(ids, id)
	}
	return ids
}
