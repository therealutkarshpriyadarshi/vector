package diskann

import (
	"fmt"
	"sync"

	"github.com/therealutkarshpriyadarshi/vector/internal/quantization"
)

// Index represents a DiskANN (Disk-based Approximate Nearest Neighbor) index
// DiskANN is Microsoft's billion-scale, SSD-resident indexing algorithm with:
// - 10-100x memory reduction through disk storage
// - In-memory graph for fast initial search
// - Disk-resident graph for billion-scale datasets
// - Product quantization for compression
type Index struct {
	// Configuration parameters
	R              int          // Number of outgoing edges per node in graph
	L              int          // Search list size
	beamWidth      int          // Beam width for beam search
	alpha          float64      // Distance threshold multiplier
	distanceFunc   DistanceFunc // Distance metric function

	// Memory-resident components (small)
	memoryGraph    *MemoryGraph        // Small in-memory graph for fast routing
	pqCodebook     *quantization.ProductQuantizer // Product quantization for compression

	// Disk-resident components (large)
	diskGraph      *DiskGraph   // Large SSD-resident graph

	// Index state
	nodes          map[uint64]*Node // Metadata for all nodes
	nodeCounter    uint64           // Counter for generating unique node IDs
	dimension      int              // Vector dimension (set on first insert)

	// Build state
	isBuilt        bool          // Whether the index has been built
	buildVectors   [][]float32   // Temporary storage during batch build
	buildIDs       []uint64      // Temporary ID storage during batch build
	buildMetadata  []map[string]interface{} // Metadata during build

	// Concurrency control
	mu             sync.RWMutex  // Protects index-level operations

	// Storage paths
	dataPath       string        // Path to disk storage

	// Statistics
	size           int64         // Number of vectors in the index
}

// IndexConfig holds configuration for creating a new DiskANN Index
type IndexConfig struct {
	R              int          // Outgoing edges per node (typical: 32-64 for DiskANN)
	L              int          // Search list size (typical: 100-200)
	BeamWidth      int          // Beam width for beam search (typical: 4-8)
	Alpha          float64      // Distance threshold multiplier (typical: 1.2)
	DistanceFunc   DistanceFunc // Distance metric (default: CosineSimilarity)
	DataPath       string       // Path to disk storage (required)

	// Product Quantization config
	NumSubvectors  int          // Number of subvectors for PQ (typical: 8-32)
	BitsPerCode    int          // Bits per PQ code (typical: 8)

	// Memory budget
	MemoryGraphSize int         // Max nodes in memory graph (typical: 100k-1M)
}

// DefaultConfig returns a configuration with recommended default values
func DefaultConfig() IndexConfig {
	return IndexConfig{
		R:               64,
		L:               100,
		BeamWidth:       4,
		Alpha:           1.2,
		DistanceFunc:    CosineSimilarity,
		DataPath:        "./diskann_data",
		NumSubvectors:   16,
		BitsPerCode:     8,
		MemoryGraphSize: 100000, // 100k nodes in memory
	}
}

// New creates a new DiskANN index with the given configuration
func New(config IndexConfig) (*Index, error) {
	// Apply defaults if not set
	if config.R == 0 {
		config.R = 64
	}
	if config.L == 0 {
		config.L = 100
	}
	if config.BeamWidth == 0 {
		config.BeamWidth = 4
	}
	if config.Alpha == 0 {
		config.Alpha = 1.2
	}
	if config.DistanceFunc == nil {
		config.DistanceFunc = CosineSimilarity
	}
	if config.DataPath == "" {
		return nil, fmt.Errorf("DataPath is required for DiskANN")
	}
	if config.NumSubvectors == 0 {
		config.NumSubvectors = 16
	}
	if config.BitsPerCode == 0 {
		config.BitsPerCode = 8
	}
	if config.MemoryGraphSize == 0 {
		config.MemoryGraphSize = 100000
	}

	// Create disk graph
	diskGraph, err := NewDiskGraph(config.DataPath, config.R)
	if err != nil {
		return nil, fmt.Errorf("failed to create disk graph: %w", err)
	}

	// Create memory graph
	memoryGraph := NewMemoryGraph(config.MemoryGraphSize, config.R)

	// Create product quantizer (will be trained during Build)
	pq := quantization.NewProductQuantizer(config.NumSubvectors, config.BitsPerCode)

	return &Index{
		R:             config.R,
		L:             config.L,
		beamWidth:     config.BeamWidth,
		alpha:         config.Alpha,
		distanceFunc:  config.DistanceFunc,
		memoryGraph:   memoryGraph,
		diskGraph:     diskGraph,
		pqCodebook:    pq,
		nodes:         make(map[uint64]*Node),
		nodeCounter:   0,
		isBuilt:       false,
		buildVectors:  make([][]float32, 0),
		buildIDs:      make([]uint64, 0),
		buildMetadata: make([]map[string]interface{}, 0),
		dataPath:      config.DataPath,
	}, nil
}

// AddVector adds a vector to the build queue (must call Build() after adding all vectors)
// DiskANN requires batch construction - cannot insert into built index
func (idx *Index) AddVector(vector []float32, metadata map[string]interface{}) (uint64, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.isBuilt {
		return 0, fmt.Errorf("cannot add vectors to built DiskANN index - use incremental update mode")
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

	// Store vector for building
	vectorCopy := make([]float32, len(vector))
	copy(vectorCopy, vector)
	idx.buildVectors = append(idx.buildVectors, vectorCopy)
	idx.buildIDs = append(idx.buildIDs, id)
	idx.buildMetadata = append(idx.buildMetadata, metadata)

	return id, nil
}

// Size returns the number of vectors in the index
func (idx *Index) Size() int64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.size
}

// Dimension returns the vector dimension
func (idx *Index) Dimension() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.dimension
}

// IsBuilt returns whether the index has been built
func (idx *Index) IsBuilt() bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.isBuilt
}

// Close closes the index and releases resources
func (idx *Index) Close() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.diskGraph != nil {
		if err := idx.diskGraph.Close(); err != nil {
			return fmt.Errorf("failed to close disk graph: %w", err)
		}
	}

	return nil
}
