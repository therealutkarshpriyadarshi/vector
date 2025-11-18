package ivf

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/therealutkarshpriyadarshi/vector/internal/quantization"
)

// IVFPQ implements Inverted File index with Product Quantization
// This combines the benefits of IVF (fast search) with PQ (high compression)
//
// Achieves:
// - 32-256x compression ratios
// - Fast search (only probe a few regions)
// - Good recall with asymmetric distance computation
//
// This is one of the most popular production vector index types!
type IVFPQ struct {
	numCentroids  int                        // Number of clusters (nlist)
	centroids     [][]float32                // Cluster centroids
	invertedLists [][]IVFPQEntry             // Compressed entries
	pq            *quantization.ProductQuantizer // Product quantizer
	dim           int                        // Vector dimension
	metric        quantization.DistanceMetric
	mu            sync.RWMutex
	trained       bool
	pqTrained     bool
}

// IVFPQEntry represents a compressed entry in an inverted list
type IVFPQEntry struct {
	ID       int                    // Vector ID
	Code     []byte                 // PQ code
	Metadata map[string]interface{} // Metadata for filtering
}

// ConfigPQ holds IVF-PQ configuration
type ConfigPQ struct {
	NumCentroids   int // Number of IVF clusters
	NumSubvectors  int // PQ parameter
	BitsPerCode    int // PQ parameter
	Metric         quantization.DistanceMetric
	TrainConfig    *quantization.QuantizationConfig
}

// NewIVFPQ creates a new IVF-PQ index
func NewIVFPQ(config ConfigPQ) *IVFPQ {
	if config.TrainConfig == nil {
		config.TrainConfig = quantization.DefaultConfig()
	}

	return &IVFPQ{
		numCentroids:  config.NumCentroids,
		metric:        config.Metric,
		invertedLists: make([][]IVFPQEntry, config.NumCentroids),
		pq:            quantization.NewProductQuantizerWithConfig(config.NumSubvectors, config.BitsPerCode, config.TrainConfig),
	}
}

// Train trains both the IVF clustering and the PQ quantizer
func (ivfpq *IVFPQ) Train(vectors [][]float32) error {
	ivfpq.mu.Lock()
	defer ivfpq.mu.Unlock()

	if len(vectors) == 0 {
		return fmt.Errorf("no training data provided")
	}

	ivfpq.dim = len(vectors[0])

	fmt.Printf("Training IVF-PQ index:\n")
	fmt.Printf("  Vectors: %d\n", len(vectors))
	fmt.Printf("  Dimension: %d\n", ivfpq.dim)
	fmt.Printf("  Centroids: %d\n", ivfpq.numCentroids)

	// Step 1: Train IVF clustering
	trainConfig := quantization.DefaultConfig()
	trainConfig.DistanceMetric = ivfpq.metric
	trainConfig.Verbose = true

	centroids, err := quantization.KMeansPlusPlus(vectors, ivfpq.numCentroids, trainConfig)
	if err != nil {
		return fmt.Errorf("IVF clustering failed: %w", err)
	}

	ivfpq.centroids = centroids
	ivfpq.trained = true

	fmt.Printf("IVF clustering complete\n")

	// Step 2: Compute residuals (vector - nearest centroid)
	// Product Quantization is trained on residuals for better accuracy
	fmt.Printf("Computing residuals for PQ training...\n")
	residuals := make([][]float32, len(vectors))

	for i, vec := range vectors {
		nearestCentroidIdx := ivfpq.findNearestCentroid(vec)
		nearestCentroid := ivfpq.centroids[nearestCentroidIdx]

		// Residual = vector - centroid
		residual := make([]float32, ivfpq.dim)
		for d := 0; d < ivfpq.dim; d++ {
			residual[d] = vec[d] - nearestCentroid[d]
		}
		residuals[i] = residual
	}

	// Step 3: Train Product Quantizer on residuals
	fmt.Printf("Training Product Quantizer on residuals...\n")
	if err := ivfpq.pq.Train(residuals); err != nil {
		return fmt.Errorf("PQ training failed: %w", err)
	}

	ivfpq.pqTrained = true

	// Print compression statistics
	codebookMB, perVectorBytes := ivfpq.pq.GetMemoryUsage()
	fmt.Printf("\nIVF-PQ Training Complete!\n")
	fmt.Printf("  Codebook size: %.2f MB\n", float64(codebookMB)/(1024*1024))
	fmt.Printf("  Per-vector size: %d bytes\n", perVectorBytes)
	fmt.Printf("  Compression ratio: %.1fx\n", ivfpq.pq.GetCompressionRatio(ivfpq.dim))

	return nil
}

// Add adds vectors to the index (compresses them with PQ)
func (ivfpq *IVFPQ) Add(vectors [][]float32, ids []int, metadata []map[string]interface{}) error {
	ivfpq.mu.Lock()
	defer ivfpq.mu.Unlock()

	if !ivfpq.trained || !ivfpq.pqTrained {
		return fmt.Errorf("index not trained, call Train() first")
	}

	if len(vectors) != len(ids) {
		return fmt.Errorf("vectors and ids length mismatch")
	}

	for i, vec := range vectors {
		if len(vec) != ivfpq.dim {
			return fmt.Errorf("vector dimension mismatch")
		}

		// Find nearest centroid
		centroidIdx := ivfpq.findNearestCentroid(vec)
		nearestCentroid := ivfpq.centroids[centroidIdx]

		// Compute residual
		residual := make([]float32, ivfpq.dim)
		for d := 0; d < ivfpq.dim; d++ {
			residual[d] = vec[d] - nearestCentroid[d]
		}

		// Encode residual with PQ
		code := ivfpq.pq.Encode(residual)

		// Add to inverted list
		entry := IVFPQEntry{
			ID:   ids[i],
			Code: code,
		}
		if metadata != nil && i < len(metadata) {
			entry.Metadata = metadata[i]
		}

		ivfpq.invertedLists[centroidIdx] = append(ivfpq.invertedLists[centroidIdx], entry)
	}

	return nil
}

// Search performs approximate nearest neighbor search
func (ivfpq *IVFPQ) Search(query []float32, k int, nprobe int) ([]int, []float32, error) {
	ivfpq.mu.RLock()
	defer ivfpq.mu.RUnlock()

	if !ivfpq.trained {
		return nil, nil, fmt.Errorf("index not trained")
	}

	if len(query) != ivfpq.dim {
		return nil, nil, fmt.Errorf("query dimension mismatch")
	}

	// Step 1: Find nprobe nearest centroids
	centroidIDs := ivfpq.findNearestCentroids(query, nprobe)

	type result struct {
		id   int
		dist float32
	}

	results := make([]result, 0, nprobe*100)

	// Step 2: Search in each probed region using asymmetric distance
	for _, centroidID := range centroidIDs {
		centroid := ivfpq.centroids[centroidID]

		// Compute query residual
		queryResidual := make([]float32, ivfpq.dim)
		for d := 0; d < ivfpq.dim; d++ {
			queryResidual[d] = query[d] - centroid[d]
		}

		// Precompute distance table for asymmetric distance
		distTable := ivfpq.pq.ComputeDistanceTable(queryResidual)

		// Compute distance to all vectors in this list
		for _, entry := range ivfpq.invertedLists[centroidID] {
			dist := ivfpq.pq.AsymmetricDistance(distTable, entry.Code)
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

// SearchWithFilter performs filtered search
func (ivfpq *IVFPQ) SearchWithFilter(query []float32, k int, nprobe int, filter func(map[string]interface{}) bool) ([]int, []float32, error) {
	ivfpq.mu.RLock()
	defer ivfpq.mu.RUnlock()

	if !ivfpq.trained {
		return nil, nil, fmt.Errorf("index not trained")
	}

	centroidIDs := ivfpq.findNearestCentroids(query, nprobe)

	type result struct {
		id   int
		dist float32
	}

	results := make([]result, 0, nprobe*100)

	for _, centroidID := range centroidIDs {
		centroid := ivfpq.centroids[centroidID]

		queryResidual := make([]float32, ivfpq.dim)
		for d := 0; d < ivfpq.dim; d++ {
			queryResidual[d] = query[d] - centroid[d]
		}

		distTable := ivfpq.pq.ComputeDistanceTable(queryResidual)

		for _, entry := range ivfpq.invertedLists[centroidID] {
			// Apply filter
			if filter != nil && !filter(entry.Metadata) {
				continue
			}

			dist := ivfpq.pq.AsymmetricDistance(distTable, entry.Code)
			results = append(results, result{id: entry.ID, dist: dist})
		}
	}

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
func (ivfpq *IVFPQ) findNearestCentroid(vec []float32) int {
	minDist := float32(math.MaxFloat32)
	minIdx := 0

	for i, centroid := range ivfpq.centroids {
		dist := ivfpq.computeDistance(vec, centroid)
		if dist < minDist {
			minDist = dist
			minIdx = i
		}
	}

	return minIdx
}

// findNearestCentroids finds the nprobe nearest centroids
func (ivfpq *IVFPQ) findNearestCentroids(vec []float32, nprobe int) []int {
	type distPair struct {
		idx  int
		dist float32
	}

	distances := make([]distPair, len(ivfpq.centroids))
	for i, centroid := range ivfpq.centroids {
		distances[i] = distPair{
			idx:  i,
			dist: ivfpq.computeDistance(vec, centroid),
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
func (ivfpq *IVFPQ) computeDistance(a, b []float32) float32 {
	switch ivfpq.metric {
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
func (ivfpq *IVFPQ) GetStats() map[string]interface{} {
	ivfpq.mu.RLock()
	defer ivfpq.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["num_centroids"] = ivfpq.numCentroids
	stats["dimension"] = ivfpq.dim
	stats["trained"] = ivfpq.trained

	// Count total entries
	totalEntries := 0
	for _, list := range ivfpq.invertedLists {
		totalEntries += len(list)
	}

	stats["total_entries"] = totalEntries
	stats["compression_ratio"] = ivfpq.pq.GetCompressionRatio(ivfpq.dim)

	codebookBytes, perVectorBytes := ivfpq.pq.GetMemoryUsage()
	stats["codebook_bytes"] = codebookBytes
	stats["per_vector_bytes"] = perVectorBytes

	return stats
}

// GetMemoryUsage returns memory usage in bytes
func (ivfpq *IVFPQ) GetMemoryUsage() int64 {
	ivfpq.mu.RLock()
	defer ivfpq.mu.RUnlock()

	var total int64

	// Centroids
	total += int64(ivfpq.numCentroids * ivfpq.dim * 4)

	// PQ codebooks
	codebookBytes, perVectorBytes := ivfpq.pq.GetMemoryUsage()
	total += int64(codebookBytes)

	// Compressed vectors
	totalEntries := 0
	for _, list := range ivfpq.invertedLists {
		totalEntries += len(list)
	}
	total += int64(totalEntries * perVectorBytes)

	return total
}
