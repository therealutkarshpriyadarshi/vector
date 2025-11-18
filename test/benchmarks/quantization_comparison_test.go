package benchmarks

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/vector/internal/quantization"
	"github.com/therealutkarshpriyadarshi/vector/pkg/ivf"
	"github.com/therealutkarshpriyadarshi/vector/pkg/scann"
)

// This file contains comprehensive benchmarks comparing different
// quantization methods and index types for vector search.
//
// Metrics compared:
// - Compression ratio
// - Search speed (QPS)
// - Recall@k
// - Memory usage
// - Training time

const (
	benchVectorDim   = 768  // Typical embedding dimension
	benchNumVectors  = 10000
	benchNumQueries  = 100
	benchK           = 10
)

// Test Configuration Matrix
var quantizationConfigs = []struct {
	name          string
	numSubvectors int
	bitsPerCode   int
}{
	{"PQ-8x6", 8, 6},   // 8 bytes, 64 clusters/subvector
	{"PQ-16x8", 16, 8}, // 16 bytes, 256 clusters/subvector
	{"PQ-32x8", 32, 8}, // 32 bytes, 256 clusters/subvector
}

func TestQuantizationComparison(t *testing.T) {
	fmt.Println("\n=== QUANTIZATION METHODS COMPARISON ===\n")

	// Generate test data
	database := generateRandomVectors(benchNumVectors, benchVectorDim)
	queries := generateRandomVectors(benchNumQueries, benchVectorDim)

	// Compute ground truth (exact k-NN)
	groundTruth := computeGroundTruth(queries, database, benchK)

	fmt.Printf("Dataset: %d vectors x %d dimensions\n", benchNumVectors, benchVectorDim)
	fmt.Printf("Queries: %d\n", benchNumQueries)
	fmt.Printf("k: %d\n\n", benchK)

	// Test each configuration
	for _, config := range quantizationConfigs {
		t.Run(config.name, func(t *testing.T) {
			testProductQuantization(t, config.name, config.numSubvectors, config.bitsPerCode, database, queries, groundTruth)
		})
	}

	// Compare with scalar quantization
	t.Run("Scalar", func(t *testing.T) {
		testScalarQuantization(t, database, queries, groundTruth)
	})
}

func testProductQuantization(t *testing.T, name string, numSubvectors, bitsPerCode int, database, queries [][]float32, groundTruth [][]int) {
	pq := quantization.NewProductQuantizer(numSubvectors, bitsPerCode)

	// Training
	trainStart := time.Now()
	err := pq.Train(database)
	trainTime := time.Since(trainStart)

	if err != nil {
		t.Fatalf("Training failed: %v", err)
	}

	// Encoding
	encodeStart := time.Now()
	encodedDB := make([][]byte, len(database))
	for i, vec := range database {
		encodedDB[i] = pq.Encode(vec)
	}
	encodeTime := time.Since(encodeStart)

	// Compression metrics
	compressionRatio := pq.GetCompressionRatio(benchVectorDim)
	originalSize := benchNumVectors * benchVectorDim * 4 // float32 = 4 bytes
	compressedSize := benchNumVectors * numSubvectors    // 1 byte per subvector
	actualRatio := float64(originalSize) / float64(compressedSize)

	// Search and measure recall
	searchStart := time.Now()
	var totalRecall float32

	for qi, query := range queries {
		// Precompute distance table
		distTable := pq.ComputeDistanceTable(query)

		// Find k-NN using asymmetric distance
		type candidate struct {
			id   int
			dist float32
		}

		candidates := make([]candidate, len(encodedDB))
		for i, code := range encodedDB {
			candidates[i] = candidate{
				id:   i,
				dist: pq.AsymmetricDistance(distTable, code),
			}
		}

		// Sort and get top-k
		quickSelect(candidates, benchK)
		results := make([]int, benchK)
		for i := 0; i < benchK; i++ {
			results[i] = candidates[i].id
		}

		// Compute recall
		recall := computeRecall(groundTruth[qi], results)
		totalRecall += recall
	}

	searchTime := time.Since(searchStart)
	avgRecall := totalRecall / float32(benchNumQueries)
	qps := float64(benchNumQueries) / searchTime.Seconds()

	// Print results
	fmt.Printf("\n%s Results:\n", name)
	fmt.Printf("  Compression: %.1fx (theoretical: %.1fx)\n", actualRatio, compressionRatio)
	fmt.Printf("  Bytes per vector: %d (original: %d)\n", numSubvectors, benchVectorDim*4)
	fmt.Printf("  Training time: %v\n", trainTime)
	fmt.Printf("  Encoding time: %v (%.2f vec/sec)\n", encodeTime, float64(benchNumVectors)/encodeTime.Seconds())
	fmt.Printf("  Recall@%d: %.2f%%\n", benchK, avgRecall*100)
	fmt.Printf("  Search QPS: %.0f\n", qps)
	fmt.Printf("  Avg latency: %.2f ms\n", 1000.0/qps)
}

func testScalarQuantization(t *testing.T, database, queries [][]float32, groundTruth [][]int) {
	sq := quantization.NewScalarQuantizer()

	// Training
	trainStart := time.Now()
	err := sq.Train(database)
	trainTime := time.Since(trainStart)

	if err != nil {
		t.Fatalf("Training failed: %v", err)
	}

	// Encoding
	encodeStart := time.Now()
	encodedDB := make([][]int8, len(database))
	for i, vec := range database {
		encodedDB[i] = sq.Quantize(vec)
	}
	encodeTime := time.Since(encodeStart)

	// Search and measure recall
	searchStart := time.Now()
	var totalRecall float32

	for qi, query := range queries {
		quantizedQuery := sq.Quantize(query)

		// Find k-NN using int8 distance
		type candidate struct {
			id   int
			dist float32
		}

		candidates := make([]candidate, len(encodedDB))
		for i, code := range encodedDB {
			candidates[i] = candidate{
				id:   i,
				dist: quantization.DistanceInt8(quantizedQuery, code),
			}
		}

		// Sort and get top-k
		quickSelect(candidates, benchK)
		results := make([]int, benchK)
		for i := 0; i < benchK; i++ {
			results[i] = candidates[i].id
		}

		// Compute recall
		recall := computeRecall(groundTruth[qi], results)
		totalRecall += recall
	}

	searchTime := time.Since(searchStart)
	avgRecall := totalRecall / float32(benchNumQueries)
	qps := float64(benchNumQueries) / searchTime.Seconds()

	compressionRatio := sq.GetMemoryReduction()
	bytesPerVector := benchVectorDim // 1 byte per dimension

	fmt.Printf("\nScalar Quantization Results:\n")
	fmt.Printf("  Compression: %.1fx\n", compressionRatio)
	fmt.Printf("  Bytes per vector: %d (original: %d)\n", bytesPerVector, benchVectorDim*4)
	fmt.Printf("  Training time: %v\n", trainTime)
	fmt.Printf("  Encoding time: %v (%.2f vec/sec)\n", encodeTime, float64(benchNumVectors)/encodeTime.Seconds())
	fmt.Printf("  Recall@%d: %.2f%%\n", benchK, avgRecall*100)
	fmt.Printf("  Search QPS: %.0f\n", qps)
	fmt.Printf("  Avg latency: %.2f ms\n", 1000.0/qps)
}

func TestIndexComparison(t *testing.T) {
	fmt.Println("\n=== INDEX METHODS COMPARISON ===\n")

	database := generateRandomVectors(benchNumVectors, benchVectorDim)
	queries := generateRandomVectors(benchNumQueries, benchVectorDim)
	groundTruth := computeGroundTruth(queries, database, benchK)

	// Test IVF-Flat
	t.Run("IVF-Flat", func(t *testing.T) {
		testIVFFlat(t, database, queries, groundTruth)
	})

	// Test IVF-PQ
	t.Run("IVF-PQ", func(t *testing.T) {
		testIVFPQ(t, database, queries, groundTruth)
	})

	// Test SCANN
	t.Run("SCANN", func(t *testing.T) {
		testSCANN(t, database, queries, groundTruth)
	})
}

func testIVFFlat(t *testing.T, database, queries [][]float32, groundTruth [][]int) {
	config := ivf.Config{
		NumCentroids: 100, // sqrt(N) is typical
		Metric:       quantization.EuclideanDistance,
	}

	index := ivf.NewIVFFlat(config)

	// Training
	trainStart := time.Now()
	err := index.Train(database)
	trainTime := time.Since(trainStart)

	if err != nil {
		t.Fatalf("Training failed: %v", err)
	}

	// Adding vectors
	addStart := time.Now()
	ids := make([]int, len(database))
	for i := range ids {
		ids[i] = i
	}
	index.Add(database, ids, nil)
	addTime := time.Since(addStart)

	// Search with different nprobe values
	nprobeValues := []int{1, 5, 10, 20}

	for _, nprobe := range nprobeValues {
		searchStart := time.Now()
		var totalRecall float32

		for qi, query := range queries {
			resultIDs, _, err := index.Search(query, benchK, nprobe)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			recall := computeRecall(groundTruth[qi], resultIDs)
			totalRecall += recall
		}

		searchTime := time.Since(searchStart)
		avgRecall := totalRecall / float32(benchNumQueries)
		qps := float64(benchNumQueries) / searchTime.Seconds()

		fmt.Printf("\nIVF-Flat (nprobe=%d):\n", nprobe)
		fmt.Printf("  Training time: %v\n", trainTime)
		fmt.Printf("  Adding time: %v\n", addTime)
		fmt.Printf("  Recall@%d: %.2f%%\n", benchK, avgRecall*100)
		fmt.Printf("  Search QPS: %.0f\n", qps)
		fmt.Printf("  Avg latency: %.2f ms\n", 1000.0/qps)
		fmt.Printf("  Memory: No compression (baseline)\n")
	}
}

func testIVFPQ(t *testing.T, database, queries [][]float32, groundTruth [][]int) {
	config := ivf.ConfigPQ{
		NumCentroids:  100,
		NumSubvectors: 16,
		BitsPerCode:   8,
		Metric:        quantization.EuclideanDistance,
	}

	index := ivf.NewIVFPQ(config)

	// Training
	trainStart := time.Now()
	err := index.Train(database)
	trainTime := time.Since(trainStart)

	if err != nil {
		t.Fatalf("Training failed: %v", err)
	}

	// Adding vectors
	addStart := time.Now()
	ids := make([]int, len(database))
	for i := range ids {
		ids[i] = i
	}
	index.Add(database, ids, nil)
	addTime := time.Since(addStart)

	// Get stats
	stats := index.GetStats()
	compressionRatio := stats["compression_ratio"].(float32)

	nprobeValues := []int{1, 5, 10, 20}

	for _, nprobe := range nprobeValues {
		searchStart := time.Now()
		var totalRecall float32

		for qi, query := range queries {
			resultIDs, _, err := index.Search(query, benchK, nprobe)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			recall := computeRecall(groundTruth[qi], resultIDs)
			totalRecall += recall
		}

		searchTime := time.Since(searchStart)
		avgRecall := totalRecall / float32(benchNumQueries)
		qps := float64(benchNumQueries) / searchTime.Seconds()

		fmt.Printf("\nIVF-PQ (nprobe=%d):\n", nprobe)
		fmt.Printf("  Compression: %.1fx\n", compressionRatio)
		fmt.Printf("  Training time: %v\n", trainTime)
		fmt.Printf("  Adding time: %v\n", addTime)
		fmt.Printf("  Recall@%d: %.2f%%\n", benchK, avgRecall*100)
		fmt.Printf("  Search QPS: %.0f\n", qps)
		fmt.Printf("  Avg latency: %.2f ms\n", 1000.0/qps)
	}
}

func testSCANN(t *testing.T, database, queries [][]float32, groundTruth [][]int) {
	config := scann.DefaultConfig()
	config.NumPartitions = 100
	config.NumSubvectors = 16
	config.BitsPerCode = 8

	index := scann.NewSCANN(config)

	// Training
	trainStart := time.Now()
	err := index.Train(database)
	trainTime := time.Since(trainStart)

	if err != nil {
		t.Fatalf("Training failed: %v", err)
	}

	// Adding vectors
	addStart := time.Now()
	ids := make([]int, len(database))
	for i := range ids {
		ids[i] = i
	}
	index.Add(database, ids, nil)
	addTime := time.Since(addStart)

	// Get stats
	stats := index.GetStats()
	compressionRatio := stats["compression_ratio"].(float32)

	nprobeValues := []int{1, 5, 10, 20}

	for _, nprobe := range nprobeValues {
		searchStart := time.Now()
		var totalRecall float32

		for qi, query := range queries {
			resultIDs, _, err := index.Search(query, benchK, nprobe)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			recall := computeRecall(groundTruth[qi], resultIDs)
			totalRecall += recall
		}

		searchTime := time.Since(searchStart)
		avgRecall := totalRecall / float32(benchNumQueries)
		qps := float64(benchNumQueries) / searchTime.Seconds()

		fmt.Printf("\nSCANN (nprobe=%d):\n", nprobe)
		fmt.Printf("  Compression: %.1fx\n", compressionRatio)
		fmt.Printf("  Spherical k-means: %v\n", config.SphericalKM)
		fmt.Printf("  Training time: %v\n", trainTime)
		fmt.Printf("  Adding time: %v\n", addTime)
		fmt.Printf("  Recall@%d: %.2f%%\n", benchK, avgRecall*100)
		fmt.Printf("  Search QPS: %.0f\n", qps)
		fmt.Printf("  Avg latency: %.2f ms\n", 1000.0/qps)
	}
}

// Helper functions

func generateRandomVectors(n, dim int) [][]float32 {
	vectors := make([][]float32, n)
	for i := 0; i < n; i++ {
		vectors[i] = make([]float32, dim)
		for j := 0; j < dim; j++ {
			vectors[i][j] = rand.Float32()
		}
	}
	return vectors
}

func computeGroundTruth(queries, database [][]float32, k int) [][]int {
	groundTruth := make([][]int, len(queries))

	for qi, query := range queries {
		type candidate struct {
			id   int
			dist float32
		}

		candidates := make([]candidate, len(database))
		for i, vec := range database {
			candidates[i] = candidate{
				id:   i,
				dist: quantization.EuclideanDistanceFloat32(query, vec),
			}
		}

		// Sort by distance
		quickSelect(candidates, k)

		groundTruth[qi] = make([]int, k)
		for i := 0; i < k; i++ {
			groundTruth[qi][i] = candidates[i].id
		}
	}

	return groundTruth
}

func computeRecall(groundTruth, results []int) float32 {
	gtSet := make(map[int]bool)
	for _, id := range groundTruth {
		gtSet[id] = true
	}

	var matches int
	for _, id := range results {
		if gtSet[id] {
			matches++
		}
	}

	return float32(matches) / float32(len(groundTruth))
}

// Quick select for partial sorting (faster than full sort)
func quickSelect(candidates []struct{ id int; dist float32 }, k int) {
	if k >= len(candidates) {
		// Full sort
		for i := 0; i < len(candidates)-1; i++ {
			for j := i + 1; j < len(candidates); j++ {
				if candidates[j].dist < candidates[i].dist {
					candidates[i], candidates[j] = candidates[j], candidates[i]
				}
			}
		}
		return
	}

	// Partial sort (only sort top-k)
	for i := 0; i < k; i++ {
		minIdx := i
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].dist < candidates[minIdx].dist {
				minIdx = j
			}
		}
		if minIdx != i {
			candidates[i], candidates[minIdx] = candidates[minIdx], candidates[i]
		}
	}
}
