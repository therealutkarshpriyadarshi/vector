package diskann

import (
	"fmt"
	"os"
	"testing"

	"github.com/therealutkarshpriyadarshi/vector/pkg/hnsw"
)

// BenchmarkDiskANN_Build benchmarks DiskANN index build time
func BenchmarkDiskANN_Build(b *testing.B) {
	dimensions := 128
	numVectors := 10000

	vectors := generateRandomVectors(numVectors, dimensions)
	for i := range vectors {
		normalizeVector(vectors[i])
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tmpDir := fmt.Sprintf("/tmp/diskann_bench_%d", i)
		os.RemoveAll(tmpDir)

		config := IndexConfig{
			R:               64,
			L:               100,
			BeamWidth:       8,
			Alpha:           1.2,
			DistanceFunc:    CosineSimilarity,
			DataPath:        tmpDir,
			NumSubvectors:   16,
			BitsPerCode:     8,
			MemoryGraphSize: 5000,
		}

		idx, err := New(config)
		if err != nil {
			b.Fatal(err)
		}

		for _, vec := range vectors {
			idx.AddVector(vec, nil)
		}

		b.StartTimer()
		if err := idx.Build(); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()

		idx.Close()
		os.RemoveAll(tmpDir)
	}
}

// BenchmarkDiskANN_Search benchmarks DiskANN search performance
func BenchmarkDiskANN_Search(b *testing.B) {
	dimensions := 128
	numVectors := 10000
	k := 10

	tmpDir := "/tmp/diskann_search_bench"
	os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	vectors := generateRandomVectors(numVectors, dimensions)
	for i := range vectors {
		normalizeVector(vectors[i])
	}

	config := IndexConfig{
		R:               64,
		L:               100,
		BeamWidth:       8,
		Alpha:           1.2,
		DistanceFunc:    CosineSimilarity,
		DataPath:        tmpDir,
		NumSubvectors:   16,
		BitsPerCode:     8,
		MemoryGraphSize: 5000,
	}

	idx, err := New(config)
	if err != nil {
		b.Fatal(err)
	}
	defer idx.Close()

	for _, vec := range vectors {
		idx.AddVector(vec, nil)
	}

	if err := idx.Build(); err != nil {
		b.Fatal(err)
	}

	queries := generateRandomVectors(100, dimensions)
	for i := range queries {
		normalizeVector(queries[i])
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		query := queries[i%len(queries)]
		_, err := idx.Search(query, k)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHNSW_Build benchmarks HNSW index build time for comparison
func BenchmarkHNSW_Build(b *testing.B) {
	dimensions := 128
	numVectors := 10000

	vectors := generateRandomVectors(numVectors, dimensions)
	for i := range vectors {
		normalizeVector(vectors[i])
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		config := hnsw.DefaultConfig()

		idx := hnsw.New(config)

		b.StartTimer()
		for _, vec := range vectors {
			idx.Insert(vec)
		}
		b.StopTimer()
	}
}

// BenchmarkHNSW_Search benchmarks HNSW search performance for comparison
func BenchmarkHNSW_Search(b *testing.B) {
	dimensions := 128
	numVectors := 10000
	k := 10

	vectors := generateRandomVectors(numVectors, dimensions)
	for i := range vectors {
		normalizeVector(vectors[i])
	}

	config := hnsw.DefaultConfig()

	idx := hnsw.New(config)

	for _, vec := range vectors {
		idx.Insert(vec)
	}

	queries := generateRandomVectors(100, dimensions)
	for i := range queries {
		normalizeVector(queries[i])
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		query := queries[i%len(queries)]
		idx.Search(query, k, 50)
	}
}

// BenchmarkComparison runs a comprehensive comparison between DiskANN and HNSW
func BenchmarkComparison(b *testing.B) {
	dimensions := 128
	numVectors := 10000

	vectors := generateRandomVectors(numVectors, dimensions)
	for i := range vectors {
		normalizeVector(vectors[i])
	}

	queries := generateRandomVectors(100, dimensions)
	for i := range queries {
		normalizeVector(queries[i])
	}

	b.Run("DiskANN_10k", func(b *testing.B) {
		tmpDir := "/tmp/diskann_comparison"
		os.RemoveAll(tmpDir)
		defer os.RemoveAll(tmpDir)

		config := IndexConfig{
			R:               64,
			L:               100,
			BeamWidth:       8,
			Alpha:           1.2,
			DistanceFunc:    CosineSimilarity,
			DataPath:        tmpDir,
			NumSubvectors:   16,
			BitsPerCode:     8,
			MemoryGraphSize: 5000,
		}

		idx, err := New(config)
		if err != nil {
			b.Fatal(err)
		}
		defer idx.Close()

		for _, vec := range vectors {
			idx.AddVector(vec, nil)
		}

		if err := idx.Build(); err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			query := queries[i%len(queries)]
			_, err := idx.Search(query, 10)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("HNSW_10k", func(b *testing.B) {
		config := hnsw.DefaultConfig()

		idx := hnsw.New(config)

		for _, vec := range vectors {
			idx.Insert(vec)
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			query := queries[i%len(queries)]
			idx.Search(query, 10, 50)
		}
	})
}

// BenchmarkMemoryFootprint compares memory usage
func BenchmarkMemoryFootprint(b *testing.B) {
	b.Run("DiskANN", func(b *testing.B) {
		tmpDir := "/tmp/diskann_memory_bench"
		os.RemoveAll(tmpDir)
		defer os.RemoveAll(tmpDir)

		dimensions := 128
		numVectors := 5000

		vectors := generateRandomVectors(numVectors, dimensions)
		for i := range vectors {
			normalizeVector(vectors[i])
		}

		config := IndexConfig{
			R:               64,
			L:               100,
			BeamWidth:       8,
			Alpha:           1.2,
			DistanceFunc:    CosineSimilarity,
			DataPath:        tmpDir,
			NumSubvectors:   16,
			BitsPerCode:     8,
			MemoryGraphSize: 1000, // Only 1k in memory
		}

		idx, err := New(config)
		if err != nil {
			b.Fatal(err)
		}
		defer idx.Close()

		for _, vec := range vectors {
			idx.AddVector(vec, nil)
		}

		if err := idx.Build(); err != nil {
			b.Fatal(err)
		}

		b.Logf("DiskANN - Total: %d, Memory: %d, Disk: %d",
			idx.Size(), idx.memoryGraph.Size(), idx.diskGraph.Size())
	})

	b.Run("HNSW", func(b *testing.B) {
		dimensions := 128
		numVectors := 5000

		vectors := generateRandomVectors(numVectors, dimensions)
		for i := range vectors {
			normalizeVector(vectors[i])
		}

		config := hnsw.DefaultConfig()

		idx := hnsw.New(config)

		for _, vec := range vectors {
			idx.Insert(vec)
		}

		b.Logf("HNSW - Total: %d (all in memory)", idx.Size())
	})
}
