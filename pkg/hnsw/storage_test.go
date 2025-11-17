package hnsw

import (
	"math/rand"
	"testing"
)

// TestVectorStorage tests that vectors are stored and retrieved correctly
func TestVectorStorage(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 10

	// Insert 10 vectors and verify they can be retrieved
	originalVectors := make([][]float32, 10)

	for i := 0; i < 10; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		originalVectors[i] = vec

		id, err := idx.Insert(vec)
		if err != nil {
			t.Fatalf("Insert %d failed: %v", i, err)
		}

		if id != uint64(i) {
			t.Errorf("Expected ID %d, got %d", i, id)
		}
	}

	// Retrieve and verify each vector
	for i := 0; i < 10; i++ {
		retrieved, err := idx.GetVector(uint64(i))
		if err != nil {
			t.Fatalf("GetVector(%d) failed: %v", i, err)
		}

		// Compare with original
		for j := 0; j < dim; j++ {
			if !almostEqual(retrieved[j], originalVectors[i][j]) {
				t.Errorf("Vector %d, dim %d: got %f, expected %f",
					i, j, retrieved[j], originalVectors[i][j])
			}
		}

		// Calculate distance to self (should be 0)
		dist := idx.distance(originalVectors[i], retrieved)
		if !almostEqual(dist, 0.0) {
			t.Errorf("Distance from original to retrieved vector %d is %f (expected 0)", i, dist)
		}
	}

	t.Log("All vectors stored and retrieved correctly")
}

// TestSearchForInsertedVector tests that we can find the exact inserted vector
func TestSearchForInsertedVector(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 10
	count := 100

	vectors := make([][]float32, count)

	// Insert vectors
	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		vectors[i] = vec

		id, err := idx.Insert(vec)
		if err != nil {
			t.Fatalf("Insert %d failed: %v", i, err)
		}
		if id != uint64(i) {
			t.Fatalf("ID mismatch: expected %d, got %d", i, id)
		}
	}

	// Now search for each inserted vector
	for i := 0; i < count; i++ {
		result, err := idx.Search(vectors[i], 1, 50)
		if err != nil {
			t.Fatalf("Search for vector %d failed: %v", i, err)
		}

		if len(result.Results) == 0 {
			t.Fatalf("Search for vector %d returned no results", i)
		}

		// First result should be the vector itself
		firstID := result.Results[0].ID
		firstDist := result.Results[0].Distance

		// Distance should be very close to 0
		if firstDist > 0.01 {
			t.Logf("Vector %d: found ID %d with distance %f (visited %d nodes)",
				i, firstID, firstDist, result.Visited)
			t.Errorf("Vector %d: distance to closest match is %f (expected ~0)", i, firstDist)

			// Also check if the correct vector exists in the index
			stored, err := idx.GetVector(uint64(i))
			if err != nil {
				t.Errorf("  Vector %d not found in index!", i)
			} else {
				// Calculate distance manually
				manualDist := idx.distance(vectors[i], stored)
				t.Errorf("  Manual distance from query to stored vector %d: %f", i, manualDist)

				// Compare vectors element by element
				matches := true
				for j := 0; j < dim; j++ {
					if !almostEqual(vectors[i][j], stored[j]) {
						matches = false
						break
					}
				}
				if matches {
					t.Errorf("  Stored vector %d matches query exactly, but search didn't find it!", i)
				} else {
					t.Errorf("  Stored vector %d does NOT match query", i)
				}
			}
		}

		// The ID should match (unless there are duplicates)
		if firstID != uint64(i) {
			// This might be OK if there's a closer vector, but dist should still be ~0
			if firstDist > 0.01 {
				t.Errorf("Vector %d: found ID %d instead of %d", i, firstID, i)
			}
		}
	}
}
