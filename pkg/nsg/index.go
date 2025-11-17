package nsg

import (
	"fmt"
	"sync"
)

// Index represents an NSG (Navigating Spreading-out Graph) index
// NSG is a single-layer graph-based ANN algorithm with optimized connectivity
type Index struct {
	// Configuration parameters
	R             int          // Number of outgoing edges per node
	L             int          // Candidate pool size for graph construction
	C             int          // Maximum candidate pool size
	distanceFunc  DistanceFunc // Distance metric function

	// Index state
	nodes        map[uint64]*Node // All nodes in the index
	navigatingID uint64           // ID of the navigating node (approximate centroid)
	nodeCounter  uint64           // Counter for generating unique node IDs
	dimension    int              // Vector dimension (set on first insert)

	// Build state (only used during construction)
	isBuilt     bool   // Whether the index has been built
	buildVectors [][]float32 // Temporary storage during batch build
	buildIDs     []uint64    // Temporary ID storage during batch build

	// Concurrency control
	mu sync.RWMutex // Protects index-level operations

	// Statistics
	size int64 // Number of vectors in the index
}

// IndexConfig holds configuration for creating a new NSG Index
type IndexConfig struct {
	R            int          // Outgoing edges per node (typical: 16-32)
	L            int          // Candidate pool size for construction (typical: 100)
	C            int          // Max candidate pool size (typical: 500)
	DistanceFunc DistanceFunc // Distance metric (default: CosineSimilarity)
}

// DefaultConfig returns a configuration with recommended default values
func DefaultConfig() IndexConfig {
	return IndexConfig{
		R:            16,
		L:            100,
		C:            500,
		DistanceFunc: CosineSimilarity,
	}
}

// New creates a new NSG index with the given configuration
func New(config IndexConfig) *Index {
	// Apply defaults if not set
	if config.R == 0 {
		config.R = 16
	}
	if config.L == 0 {
		config.L = 100
	}
	if config.C == 0 {
		config.C = 500
	}
	if config.DistanceFunc == nil {
		config.DistanceFunc = CosineSimilarity
	}

	return &Index{
		R:            config.R,
		L:            config.L,
		C:            config.C,
		distanceFunc: config.DistanceFunc,
		nodes:        make(map[uint64]*Node),
		nodeCounter:  0,
		isBuilt:      false,
		buildVectors: make([][]float32, 0),
		buildIDs:     make([]uint64, 0),
	}
}

// AddVector adds a vector to the build queue (must call Build() after adding all vectors)
// NSG requires batch construction - cannot insert into built index
func (idx *Index) AddVector(vector []float32) (uint64, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.isBuilt {
		return 0, fmt.Errorf("cannot add vectors to built NSG index")
	}

	// Set dimension on first vector
	if idx.dimension == 0 {
		idx.dimension = len(vector)
	} else if len(vector) != idx.dimension {
		return 0, fmt.Errorf("vector dimension mismatch: expected %d, got %d", idx.dimension, len(vector))
	}

	// Generate ID and add to build queue
	id := idx.nodeCounter
	idx.nodeCounter++

	// Make a copy of the vector
	vecCopy := make([]float32, len(vector))
	copy(vecCopy, vector)

	idx.buildVectors = append(idx.buildVectors, vecCopy)
	idx.buildIDs = append(idx.buildIDs, id)
	idx.size++

	return id, nil
}

// Build constructs the NSG graph from all added vectors
// This is the main construction algorithm
func (idx *Index) Build() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.isBuilt {
		return fmt.Errorf("index already built")
	}

	if len(idx.buildVectors) == 0 {
		return fmt.Errorf("no vectors to build index")
	}

	// Create nodes
	for i, vec := range idx.buildVectors {
		node := NewNode(idx.buildIDs[i], vec)
		idx.nodes[idx.buildIDs[i]] = node
	}

	// Find navigating node (approximate centroid)
	idx.navigatingID = idx.findNavigatingNode()

	// Build initial KNN graph
	knnGraph := idx.buildKNNGraph()

	// Refine to NSG
	idx.refineToNSG(knnGraph)

	// Clear build buffers
	idx.buildVectors = nil
	idx.buildIDs = nil
	idx.isBuilt = true

	return nil
}

// Size returns the number of vectors in the index
func (idx *Index) Size() int64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.size
}

// IsBuilt returns whether the index has been built
func (idx *Index) IsBuilt() bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.isBuilt
}

// GetNavigatingNode returns the ID of the navigating node
func (idx *Index) GetNavigatingNode() uint64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.navigatingID
}

// GetNode returns a node by ID (thread-safe, returns copy)
func (idx *Index) GetNode(id uint64) (*Node, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	node, exists := idx.nodes[id]
	return node, exists
}
