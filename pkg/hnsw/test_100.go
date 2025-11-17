package hnsw

import (
	"math/rand"
	"testing"
)

func TestSearch100Vectors(t *testing.T) {
	config := DefaultConfig()
	idx := New(config)

	rng := rand.New(rand.NewSource(42))
	dim := 10
	count := 100

	vectors := make([][]float32, count)

	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = rng.Float32()
		}
		vectors[i] = vec
		idx.Insert(vec)
	}

	failures := 0
	for i := 0; i < count; i++ {
		result, _ := idx.Search(vectors[i], 1, 50)
		if len(result.Results) > 0 {
			if result.Results[0].ID != uint64(i) || result.Results[0].Distance > 0.01 {
				failures++
				t.Logf("Vector %d: got ID %d, distance %f", i, result.Results[0].ID, result.Results[0].Distance)
			}
		}
	}

	t.Logf("Failures: %d/%d", failures, count)
	if failures > count/10 {
		t.Errorf("Too many failures: %d", failures)
	}
}
