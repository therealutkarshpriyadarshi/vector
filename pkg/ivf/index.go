package ivf

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/therealutkarshpriyadarshi/vector/internal/quantization"
)

// IVFFlat implements Inverted File index with flat (uncompressed) vectors
// Good for: categorical/tag-based filtering, small-medium datasets
//
// The IVF index partitions the vector space into regions using k-means clustering.
// Each region (centroid) has an inverted list of vectors in that region.
// Search first finds the nearest centroids, then searches only those regions.
//
// Advantages:
// - Fast search with good recall
// - Excellent for filtered searches (each filter can have its own IVF)
// - Simple implementation, easy to understand
//
// Disadvantages:
// - Requires batch building (not dynamic like HNSW)
// - Memory usage same as brute force (use IVF-PQ for compression)
type IVFFlat struct {
	numCentroids int               // Number of clusters (nlist)
	centroids    [][]float32       // Cluster centroids
	invertedLists [][]IVFEntry     // invertedLists[centroid] = vectors in that cluster
	vectors      [][]float32       // Original vectors
	ids          []int             // Vector IDs
	dim          int               // Vector dimension
	metric       quantization.DistanceMetric
	mu           sync.RWMutex
	trained      bool
}

// IVFEntry represents an entry in an inverted list
type IVFEntry struct {
	ID       int       // Vector ID
	Vector   []float32 // Original vector
	Metadata map[string]interface{} // Metadata for filtering
}

// Config holds IVF configuration
type Config struct {
	NumCentroids int // Number of clusters (typical: sqrt(N) to N/100)
	Metric       quantization.DistanceMetric
	TrainConfig  *quantization.QuantizationConfig
}

// NewIVFFlat creates a new IVF-Flat index
func NewIVFFlat(config Config) *IVFFlat {
	if config.TrainConfig == nil {
		config.TrainConfig = quantization.DefaultConfig()
	}

	return &IVFFlat{
		numCentroids: config.NumCentroids,
		metric:       config.Metric,
		invertedLists: make([][]IVFEntry, config.NumCentroids),
		vectors:      make([][]float32, 0),
		ids:          make([]int, 0),
	}
}

// Train trains the IVF index by clustering vectors into regions
func (ivf *IVFFlat) Train(vectors [][]float32) error {
	ivf.mu.Lock()
	defer ivf.mu.Unlock()

	if len(vectors) == 0 {
		return fmt.Errorf("no training data provided")
	}

	if len(vectors) < ivf.numCentroids {
		return fmt.Errorf("need at least %d vectors for %d centroids, got %d",
			ivf.numCentroids, ivf.numCentroids, len(vectors))
	}

	ivf.dim = len(vectors[0])

	// Use k-means++ to find centroids
	trainConfig := quantization.DefaultConfig()
	trainConfig.DistanceMetric = ivf.metric
	trainConfig.Verbose = true

	centroids, err := quantization.KMeansPlusPlus(vectors, ivf.numCentroids, trainConfig)
	if err != nil {
		return fmt.Errorf("k-means clustering failed: %w", err)
	}

	ivf.centroids = centroids
	ivf.trained = true

	fmt.Printf("IVF-Flat trained with %d centroids\n", ivf.numCentroids)
	return nil
}

// Add adds vectors to the index
func (ivf *IVFFlat) Add(vectors [][]float32, ids []int, metadata []map[string]interface{}) error {
	ivf.mu.Lock()
	defer ivf.mu.Unlock()

	if !ivf.trained {
		return fmt.Errorf("index not trained, call Train() first")
	}

	if len(vectors) != len(ids) {
		return fmt.Errorf("vectors and ids length mismatch")
	}

	if metadata != nil && len(metadata) != len(vectors) {
		return fmt.Errorf("metadata length mismatch")
	}

	for i, vec := range vectors {
		if len(vec) != ivf.dim {
			return fmt.Errorf("vector dimension mismatch: expected %d, got %d", ivf.dim, len(vec))
		}

		// Find nearest centroid
		centroidIdx := ivf.findNearestCentroid(vec)

		// Add to inverted list
		entry := IVFEntry{
			ID:     ids[i],
			Vector: vec,
		}
		if metadata != nil {
			entry.Metadata = metadata[i]
		}

		ivf.invertedLists[centroidIdx] = append(ivf.invertedLists[centroidIdx], entry)
		ivf.vectors = append(ivf.vectors, vec)
		ivf.ids = append(ivf.ids, ids[i])
	}

	return nil
}

// Search performs nearest neighbor search
func (ivf *IVFFlat) Search(query []float32, k int, nprobe int) ([]int, []float32, error) {
	ivf.mu.RLock()
	defer ivf.mu.RUnlock()

	if !ivf.trained {
		return nil, nil, fmt.Errorf("index not trained")
	}

	if len(query) != ivf.dim {
		return nil, nil, fmt.Errorf("query dimension mismatch")
	}

	// Step 1: Find nprobe nearest centroids
	centroidIDs := ivf.findNearestCentroids(query, nprobe)

	// Step 2: Search in those regions
	type result struct {
		id   int
		dist float32
	}

	results := make([]result, 0, nprobe*100)

	for _, centroidID := range centroidIDs {
		// Search all vectors in this inverted list
		for _, entry := range ivf.invertedLists[centroidID] {
			dist := ivf.computeDistance(query, entry.Vector)
			results = append(results, result{id: entry.ID, dist: dist})
		}
	}

	// Step 3: Sort by distance and return top-k
	sort.Slice(results, func(i, j int) bool {
		return results[i].dist < results[j].dist
	})

	if len(results) > k {
		results = results[:k]
	}

	ids := make([]int, len(results))
	distances := make([]float32, len(results))
	for i, r := range results {
		ids[i] = r.id
		distances[i] = r.dist
	}

	return ids, distances, nil
}

// SearchWithFilter performs filtered nearest neighbor search
// This is where IVF-Flat shines - each filter can probe different centroids
func (ivf *IVFFlat) SearchWithFilter(query []float32, k int, nprobe int, filter func(map[string]interface{}) bool) ([]int, []float32, error) {
	ivf.mu.RLock()
	defer ivf.mu.RUnlock()

	if !ivf.trained {
		return nil, nil, fmt.Errorf("index not trained")
	}

	// Find nprobe nearest centroids
	centroidIDs := ivf.findNearestCentroids(query, nprobe)

	type result struct {
		id   int
		dist float32
	}

	results := make([]result, 0, nprobe*100)

	// Search with filter
	for _, centroidID := range centroidIDs {
		for _, entry := range ivf.invertedLists[centroidID] {
			// Apply filter
			if filter != nil && !filter(entry.Metadata) {
				continue
			}

			dist := ivf.computeDistance(query, entry.Vector)
			results = append(results, result{id: entry.ID, dist: dist})
		}
	}

	// Sort and return top-k
	sort.Slice(results, func(i, j int) bool {
		return results[i].dist < results[j].dist
	})

	if len(results) > k {
		results = results[:k]
	}

	ids := make([]int, len(results))
	distances := make([]float32, len(results))
	for i, r := range results {
		ids[i] = r.id
		distances[i] = r.dist
	}

	return ids, distances, nil
}

// findNearestCentroid finds the nearest centroid for a vector
func (ivf *IVFFlat) findNearestCentroid(vec []float32) int {
	minDist := float32(math.MaxFloat32)
	minIdx := 0

	for i, centroid := range ivf.centroids {
		dist := ivf.computeDistance(vec, centroid)
		if dist < minDist {
			minDist = dist
			minIdx = i
		}
	}

	return minIdx
}

// findNearestCentroids finds the nprobe nearest centroids
func (ivf *IVFFlat) findNearestCentroids(vec []float32, nprobe int) []int {
	type distPair struct {
		idx  int
		dist float32
	}

	distances := make([]distPair, len(ivf.centroids))
	for i, centroid := range ivf.centroids {
		distances[i] = distPair{
			idx:  i,
			dist: ivf.computeDistance(vec, centroid),
		}
	}

	sort.Slice(distances, func(i, j int) bool {
		return distances[i].dist < distances[j].dist
	})

	if nprobe > len(distances) {
		nprobe = len(distances)
	}

	result := make([]int, nprobe)
	for i := 0; i < nprobe; i++ {
		result[i] = distances[i].idx
	}

	return result
}

// computeDistance computes distance between two vectors
func (ivf *IVFFlat) computeDistance(a, b []float32) float32 {
	switch ivf.metric {
	case quantization.EuclideanDistance:
		return quantization.EuclideanDistanceFloat32(a, b)
	case quantization.CosineDistance:
		return quantization.CosineDistanceFloat32(a, b)
	case quantization.DotProductDistance:
		return -quantization.DotProductFloat32(a, b)
	default:
		return quantization.EuclideanDistanceFloat32(a, b)
	}
}

// GetStats returns index statistics
func (ivf *IVFFlat) GetStats() map[string]interface{} {
	ivf.mu.RLock()
	defer ivf.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["num_centroids"] = ivf.numCentroids
	stats["num_vectors"] = len(ivf.vectors)
	stats["dimension"] = ivf.dim
	stats["trained"] = ivf.trained

	// Compute inverted list sizes
	listSizes := make([]int, ivf.numCentroids)
	totalEntries := 0
	for i, list := range ivf.invertedLists {
		listSizes[i] = len(list)
		totalEntries += len(list)
	}

	stats["total_entries"] = totalEntries
	stats["avg_list_size"] = float32(totalEntries) / float32(ivf.numCentroids)

	// Find min/max list sizes
	minSize := len(ivf.invertedLists[0])
	maxSize := len(ivf.invertedLists[0])
	for _, size := range listSizes {
		if size < minSize {
			minSize = size
		}
		if size > maxSize {
			maxSize = size
		}
	}

	stats["min_list_size"] = minSize
	stats["max_list_size"] = maxSize

	return stats
}

// GetMemoryUsage returns memory usage in bytes
func (ivf *IVFFlat) GetMemoryUsage() int64 {
	ivf.mu.RLock()
	defer ivf.mu.RUnlock()

	var total int64

	// Centroids
	total += int64(ivf.numCentroids * ivf.dim * 4) // float32 = 4 bytes

	// Vectors in inverted lists
	total += int64(len(ivf.vectors) * ivf.dim * 4)

	// IDs
	total += int64(len(ivf.ids) * 8) // int = 8 bytes

	return total
}
