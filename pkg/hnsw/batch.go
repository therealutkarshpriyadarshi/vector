package hnsw

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// BatchInsertResult represents the result of a batch insert operation
type BatchInsertResult struct {
	TotalProcessed int
	SuccessCount   int
	FailureCount   int
	Errors         []error
	VectorIDs      []uint64
}

// BatchDeleteResult represents the result of a batch delete operation
type BatchDeleteResult struct {
	TotalProcessed int
	SuccessCount   int
	FailureCount   int
	Errors         []error
}

// BatchUpdateResult represents the result of a batch update operation
type BatchUpdateResult struct {
	TotalProcessed int
	SuccessCount   int
	FailureCount   int
	Errors         []error
}

// ProgressCallback is called during batch operations to report progress
type ProgressCallback func(processed, total int)

// BatchInsert inserts multiple vectors efficiently
func (idx *Index) BatchInsert(vectors [][]float32, progressCb ProgressCallback) *BatchInsertResult {
	result := &BatchInsertResult{
		TotalProcessed: len(vectors),
		Errors:         make([]error, 0),
		VectorIDs:      make([]uint64, len(vectors)),
	}

	if len(vectors) == 0 {
		return result
	}

	// Use buffered channel for worker pool
	const numWorkers = 8
	jobs := make(chan int, len(vectors))
	var wg sync.WaitGroup

	// Track success/failure atomically
	var successCount, failureCount int64

	// Worker pool for parallel insertion
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				vector := vectors[i]

				// Insert vector
				id, err := idx.Insert(vector)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("vector %d: %w", i, err))
					atomic.AddInt64(&failureCount, 1)
				} else {
					result.VectorIDs[i] = id
					atomic.AddInt64(&successCount, 1)
				}

				// Progress callback
				if progressCb != nil {
					processed := int(atomic.LoadInt64(&successCount) + atomic.LoadInt64(&failureCount))
					progressCb(processed, len(vectors))
				}
			}
		}()
	}

	// Send jobs to workers
	for i := 0; i < len(vectors); i++ {
		jobs <- i
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()

	result.SuccessCount = int(successCount)
	result.FailureCount = int(failureCount)

	return result
}

// BatchInsertSequential inserts vectors sequentially (for when order matters)
func (idx *Index) BatchInsertSequential(vectors [][]float32, progressCb ProgressCallback) *BatchInsertResult {
	result := &BatchInsertResult{
		TotalProcessed: len(vectors),
		Errors:         make([]error, 0),
		VectorIDs:      make([]uint64, len(vectors)),
	}

	if len(vectors) == 0 {
		return result
	}

	for i, vector := range vectors {
		id, err := idx.Insert(vector)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("vector %d: %w", i, err))
			result.FailureCount++
		} else {
			result.VectorIDs[i] = id
			result.SuccessCount++
		}

		// Progress callback
		if progressCb != nil {
			progressCb(i+1, len(vectors))
		}
	}

	return result
}

// BatchDelete deletes multiple vectors by ID
func (idx *Index) BatchDelete(ids []uint64, progressCb ProgressCallback) *BatchDeleteResult {
	result := &BatchDeleteResult{
		TotalProcessed: len(ids),
		Errors:         make([]error, 0),
	}

	if len(ids) == 0 {
		return result
	}

	// Use worker pool for parallel deletion
	const numWorkers = 8
	jobs := make(chan uint64, len(ids))
	var wg sync.WaitGroup

	// Track success/failure atomically
	var successCount, failureCount int64

	// Worker pool
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				err := idx.Delete(id)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("vector %d: %w", id, err))
					atomic.AddInt64(&failureCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}

				// Progress callback
				if progressCb != nil {
					processed := int(atomic.LoadInt64(&successCount) + atomic.LoadInt64(&failureCount))
					progressCb(processed, len(ids))
				}
			}
		}()
	}

	// Send jobs to workers
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)

	// Wait for completion
	wg.Wait()

	result.SuccessCount = int(successCount)
	result.FailureCount = int(failureCount)

	return result
}

// BatchUpdate updates multiple vectors
func (idx *Index) BatchUpdate(updates []VectorUpdate, progressCb ProgressCallback) *BatchUpdateResult {
	result := &BatchUpdateResult{
		TotalProcessed: len(updates),
		Errors:         make([]error, 0),
	}

	if len(updates) == 0 {
		return result
	}

	// Use worker pool for parallel updates
	const numWorkers = 8
	jobs := make(chan VectorUpdate, len(updates))
	var wg sync.WaitGroup

	// Track success/failure atomically
	var successCount, failureCount int64

	// Worker pool
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for update := range jobs {
				err := idx.Update(update.ID, update.Vector)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("vector %d: %w", update.ID, err))
					atomic.AddInt64(&failureCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}

				// Progress callback
				if progressCb != nil {
					processed := int(atomic.LoadInt64(&successCount) + atomic.LoadInt64(&failureCount))
					progressCb(processed, len(updates))
				}
			}
		}()
	}

	// Send jobs to workers
	for _, update := range updates {
		jobs <- update
	}
	close(jobs)

	// Wait for completion
	wg.Wait()

	result.SuccessCount = int(successCount)
	result.FailureCount = int(failureCount)

	return result
}

// VectorUpdate represents an update operation
type VectorUpdate struct {
	ID     uint64
	Vector []float32
}

// BatchInsertWithBuffer uses buffering to optimize memory usage for large batches
func (idx *Index) BatchInsertWithBuffer(vectors [][]float32, bufferSize int, progressCb ProgressCallback) *BatchInsertResult {
	result := &BatchInsertResult{
		TotalProcessed: len(vectors),
		Errors:         make([]error, 0),
		VectorIDs:      make([]uint64, len(vectors)),
	}

	if len(vectors) == 0 {
		return result
	}

	if bufferSize <= 0 {
		bufferSize = 1000 // Default buffer size
	}

	// Process in chunks
	for start := 0; start < len(vectors); start += bufferSize {
		end := start + bufferSize
		if end > len(vectors) {
			end = len(vectors)
		}

		vectorChunk := vectors[start:end]

		// Process chunk with progress callback
		chunkCb := func(processed, total int) {
			if progressCb != nil {
				progressCb(start+processed, len(vectors))
			}
		}

		chunkResult := idx.BatchInsert(vectorChunk, chunkCb)

		// Merge results
		result.SuccessCount += chunkResult.SuccessCount
		result.FailureCount += chunkResult.FailureCount
		result.Errors = append(result.Errors, chunkResult.Errors...)

		// Copy IDs
		copy(result.VectorIDs[start:end], chunkResult.VectorIDs)
	}

	return result
}

// GetBatchStats returns statistics about batch operations
func (idx *Index) GetBatchStats() map[string]interface{} {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return map[string]interface{}{
		"total_vectors":    idx.size,
		"max_layer":        idx.maxLayer,
		"entry_point_id":   func() interface{} {
			if idx.entryPoint != nil {
				return idx.entryPoint.id
			}
			return nil
		}(),
	}
}
