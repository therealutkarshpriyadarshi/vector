package diskann

// Node represents a node in the DiskANN graph
type Node struct {
	ID         uint64                 // Unique identifier
	Vector     []float32              // Original vector (may be compressed)
	PQCode     []byte                 // Product quantization code
	Neighbors  []uint64               // Neighbor IDs in the graph
	Metadata   map[string]interface{} // User-defined metadata
	OnDisk     bool                   // Whether this node is primarily stored on disk
}

// NewNode creates a new node
func NewNode(id uint64, vector []float32, metadata map[string]interface{}) *Node {
	return &Node{
		ID:        id,
		Vector:    vector,
		Neighbors: make([]uint64, 0),
		Metadata:  metadata,
		OnDisk:    false,
	}
}

// AddNeighbor adds a neighbor to this node
func (n *Node) AddNeighbor(neighborID uint64) {
	n.Neighbors = append(n.Neighbors, neighborID)
}

// SetNeighbors sets the neighbors for this node
func (n *Node) SetNeighbors(neighbors []uint64) {
	n.Neighbors = neighbors
}

// GetNeighbors returns the neighbors of this node
func (n *Node) GetNeighbors() []uint64 {
	return n.Neighbors
}
