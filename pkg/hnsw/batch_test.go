package hnsw

import (
	"math/rand"
	"testing"
)

// randomVector generates a random vector of the given dimension
func randomVector(dim int) []float32 {
	vec := make([]float32, dim)
	for i := range vec {
		vec[i] = rand.Float32()
	}
	return vec
}

func TestBatchInsert(t *testing.T) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	// Create test vectors
	vectors := make([][]float32, 100)
	for i := 0; i < 100; i++ {
		vectors[i] = randomVector(768)
	}

	// Batch insert
	result := idx.BatchInsert(vectors, nil)

	if result.TotalProcessed != 100 {
		t.Errorf("Expected 100 processed, got %d", result.TotalProcessed)
	}

	if result.SuccessCount != 100 {
		t.Errorf("Expected 100 successes, got %d", result.SuccessCount)
	}

	if result.FailureCount != 0 {
		t.Errorf("Expected 0 failures, got %d", result.FailureCount)
	}

	if len(result.VectorIDs) != 100 {
		t.Errorf("Expected 100 IDs, got %d", len(result.VectorIDs))
	}

	// Verify index size
	if idx.Size() != 100 {
		t.Errorf("Expected index size 100, got %d", idx.Size())
	}
}

func TestBatchInsertWithProgress(t *testing.T) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	vectors := make([][]float32, 100)
	for i := 0; i < 100; i++ {
		vectors[i] = randomVector(768)
	}

	progressCalls := 0
	lastProcessed := 0

	result := idx.BatchInsert(vectors, func(processed, total int) {
		progressCalls++
		if processed < lastProcessed {
			t.Errorf("Progress decreased: %d -> %d", lastProcessed, processed)
		}
		lastProcessed = processed
		if total != 100 {
			t.Errorf("Expected total 100, got %d", total)
		}
	})

	if result.SuccessCount != 100 {
		t.Errorf("Expected 100 successes, got %d", result.SuccessCount)
	}

	if progressCalls == 0 {
		t.Error("Expected progress callbacks to be called")
	}
}

func TestBatchInsertSequential(t *testing.T) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	vectors := make([][]float32, 50)
	for i := 0; i < 50; i++ {
		vectors[i] = randomVector(768)
	}

	result := idx.BatchInsertSequential(vectors, nil)

	if result.SuccessCount != 50 {
		t.Errorf("Expected 50 successes, got %d", result.SuccessCount)
	}

	// Verify IDs are sequential
	for i := 1; i < len(result.VectorIDs); i++ {
		if result.VectorIDs[i] <= result.VectorIDs[i-1] {
			t.Errorf("IDs not sequential: %d, %d", result.VectorIDs[i-1], result.VectorIDs[i])
		}
	}
}

func TestBatchDelete(t *testing.T) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	// Insert vectors first
	vectors := make([][]float32, 50)
	ids := make([]uint64, 50)
	for i := 0; i < 50; i++ {
		vectors[i] = randomVector(768)
		id, _ := idx.Insert(vectors[i])
		ids[i] = id
	}

	initialSize := idx.Size()

	// Delete first 20
	deleteIDs := ids[:20]
	result := idx.BatchDelete(deleteIDs, nil)

	if result.SuccessCount != 20 {
		t.Errorf("Expected 20 deletions, got %d", result.SuccessCount)
	}

	// Verify size decreased
	if idx.Size() != initialSize-20 {
		t.Errorf("Expected size %d, got %d", initialSize-20, idx.Size())
	}
}

func TestBatchDeleteWithProgress(t *testing.T) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	// Insert vectors
	ids := make([]uint64, 30)
	for i := 0; i < 30; i++ {
		id, _ := idx.Insert(randomVector(768))
		ids[i] = id
	}

	progressCalls := 0
	result := idx.BatchDelete(ids, func(processed, total int) {
		progressCalls++
	})

	if result.SuccessCount != 30 {
		t.Errorf("Expected 30 deletions, got %d", result.SuccessCount)
	}

	if progressCalls == 0 {
		t.Error("Expected progress callbacks")
	}
}

func TestBatchUpdate(t *testing.T) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	// Insert vectors
	ids := make([]uint64, 20)
	for i := 0; i < 20; i++ {
		id, _ := idx.Insert(randomVector(768))
		ids[i] = id
	}

	// Create updates
	updates := make([]VectorUpdate, 20)
	for i := 0; i < 20; i++ {
		updates[i] = VectorUpdate{
			ID:     ids[i],
			Vector: randomVector(768),
		}
	}

	result := idx.BatchUpdate(updates, nil)

	if result.SuccessCount != 20 {
		t.Errorf("Expected 20 updates, got %d", result.SuccessCount)
	}

	if result.FailureCount != 0 {
		t.Errorf("Expected 0 failures, got %d", result.FailureCount)
	}
}

func TestBatchUpdateNonexistent(t *testing.T) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	// Try to update non-existent vectors
	updates := []VectorUpdate{
		{ID: 999999, Vector: randomVector(768)},
		{ID: 888888, Vector: randomVector(768)},
	}

	result := idx.BatchUpdate(updates, nil)

	if result.FailureCount != 2 {
		t.Errorf("Expected 2 failures, got %d", result.FailureCount)
	}

	if len(result.Errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(result.Errors))
	}
}

func TestBatchInsertWithBuffer(t *testing.T) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	// Create large batch
	vectors := make([][]float32, 500)
	for i := 0; i < 500; i++ {
		vectors[i] = randomVector(768)
	}

	// Insert with buffer size of 100
	result := idx.BatchInsertWithBuffer(vectors, 100, nil)

	if result.SuccessCount != 500 {
		t.Errorf("Expected 500 successes, got %d", result.SuccessCount)
	}

	if idx.Size() != 500 {
		t.Errorf("Expected index size 500, got %d", idx.Size())
	}
}

func TestBatchInsertEmpty(t *testing.T) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	var vectors [][]float32
	result := idx.BatchInsert(vectors, nil)

	if result.TotalProcessed != 0 {
		t.Errorf("Expected 0 processed, got %d", result.TotalProcessed)
	}
}

func TestBatchDeleteEmpty(t *testing.T) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	var ids []uint64
	result := idx.BatchDelete(ids, nil)

	if result.TotalProcessed != 0 {
		t.Errorf("Expected 0 processed, got %d", result.TotalProcessed)
	}
}

func TestGetBatchStats(t *testing.T) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	// Insert some vectors
	for i := 0; i < 50; i++ {
		idx.Insert(randomVector(768))
	}

	stats := idx.GetBatchStats()

	totalVectors, ok := stats["total_vectors"].(int64)
	if !ok || totalVectors != 50 {
		t.Errorf("Expected total_vectors 50, got %v", stats["total_vectors"])
	}

	maxLayer, ok := stats["max_layer"].(int)
	if !ok {
		t.Error("Expected max_layer in stats")
	}

	if maxLayer < 0 {
		t.Errorf("Invalid max_layer: %d", maxLayer)
	}
}

func BenchmarkBatchInsert(b *testing.B) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	vectors := make([][]float32, 1000)
	for i := 0; i < 1000; i++ {
		vectors[i] = randomVector(768)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.BatchInsert(vectors, nil)
	}
}

func BenchmarkBatchInsertSequential(b *testing.B) {
	idx := New(IndexConfig{
		M:              16,
		efConstruction: 200,
		DistanceFunc:   CosineSimilarity,
	})

	vectors := make([][]float32, 1000)
	for i := 0; i < 1000; i++ {
		vectors[i] = randomVector(768)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.BatchInsertSequential(vectors, nil)
	}
}
