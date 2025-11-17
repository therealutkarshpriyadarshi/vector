package hnsw

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// Index represents an HNSW (Hierarchical Navigable Small World) index
type Index struct {
	// Configuration parameters
	M              int          // Maximum number of connections per layer (except layer 0)
	M0             int          // Maximum number of connections for layer 0
	efConstruction int          // Size of dynamic candidate list during construction
	ml             float64      // Normalization factor for level generation
	distanceFunc   DistanceFunc // Distance metric function

	// Index state
	nodes       map[uint64]*Node // All nodes in the index
	entryPoint  *Node            // Entry point for search (highest level node)
	maxLayer    int              // Maximum layer in the index
	nodeCounter uint64           // Counter for generating unique node IDs
	dimension   int              // Vector dimension (set on first insert)

	// Concurrency control
	mu   sync.RWMutex // Protects index-level operations
	rand *rand.Rand   // Random number generator for level assignment

	// Statistics
	size int64 // Number of vectors in the index
}

// IndexConfig holds configuration for creating a new Index
type IndexConfig struct {
	M              int          // Bi-directional links per node (typical: 16-32)
	efConstruction int          // Size of candidate list during insertion (typical: 200)
	DistanceFunc   DistanceFunc // Distance metric (default: CosineSimilarity)
}

// DefaultConfig returns a configuration with recommended default values
func DefaultConfig() IndexConfig {
	return IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	}
}

// New creates a new HNSW index with the given configuration
func New(config IndexConfig) *Index {
	// Apply defaults if not set
	if config.M == 0 {
		config.M = 16
	}
	if config.efConstruction == 0 {
		config.efConstruction = 200
	}
	if config.DistanceFunc == nil {
		config.DistanceFunc = CosineSimilarity
	}

	// M0 is typically 2*M for the base layer
	M0 := config.M * 2

	// Normalization factor for level generation
	// ml = 1/ln(M) ensures exponential decay of layer probabilities
	ml := 1.0 / math.Log(float64(config.M))

	return &Index{
		M:              config.M,
		M0:             M0,
		efConstruction: config.efConstruction,
		ml:             ml,
		distanceFunc:   config.DistanceFunc,
		nodes:          make(map[uint64]*Node),
		maxLayer:       -1,
		nodeCounter:    0,
		rand:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// randomLevel generates a random layer for a new node
// Uses exponential decay: P(level=l) = e^(-l/ml)
// This ensures most nodes are on lower layers, with fewer on higher layers
func (idx *Index) randomLevel() int {
	// Generate random float between 0 and 1
	r := idx.rand.Float64()

	// Apply exponential distribution
	// floor(-ln(r) * ml) gives us the layer
	level := int(math.Floor(-math.Log(r) * idx.ml))

	return level
}

// Size returns the number of vectors in the index
func (idx *Index) Size() int64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.size
}

// Dimension returns the vector dimension of the index
func (idx *Index) Dimension() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.dimension
}

// MaxLayer returns the highest layer in the index
func (idx *Index) MaxLayer() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.maxLayer
}

// GetNode retrieves a node by ID (thread-safe)
func (idx *Index) GetNode(id uint64) *Node {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.nodes[id]
}

// EntryPoint returns the current entry point node
func (idx *Index) EntryPoint() *Node {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.entryPoint
}

// Stats returns statistics about the index
type IndexStats struct {
	Size          int64
	Dimension     int
	MaxLayer      int
	M             int
	M0            int
	EfConstruction int
	NodesPerLayer map[int]int // Number of nodes at each layer
}

// GetStats returns current index statistics
func (idx *Index) GetStats() IndexStats {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Count nodes per layer
	nodesPerLayer := make(map[int]int)
	for _, node := range idx.nodes {
		for layer := 0; layer <= node.level; layer++ {
			nodesPerLayer[layer]++
		}
	}

	return IndexStats{
		Size:           idx.size,
		Dimension:      idx.dimension,
		MaxLayer:       idx.maxLayer,
		M:              idx.M,
		M0:             idx.M0,
		EfConstruction: idx.efConstruction,
		NodesPerLayer:  nodesPerLayer,
	}
}

// distance calculates the distance between two vectors
func (idx *Index) distance(a, b []float32) float32 {
	return idx.distanceFunc(a, b)
}

// distanceToNode calculates the distance from a vector to a node
func (idx *Index) distanceToNode(vector []float32, node *Node) float32 {
	return idx.distanceFunc(vector, node.vector)
}

// distanceBetweenNodes calculates the distance between two nodes
func (idx *Index) distanceBetweenNodes(a, b *Node) float32 {
	return idx.distanceFunc(a.vector, b.vector)
}
