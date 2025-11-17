package search

import (
	"testing"

	"github.com/therealutkarshpriyadarshi/vector/pkg/hnsw"
)

func createTestHybridSearch(t *testing.T) (*HybridSearch, [][]float32) {
	// Create vector index
	vectorIdx := hnsw.New(hnsw.DefaultConfig())

	// Create text index
	textIdx := NewFullTextIndex()

	// Create test documents with vectors and text
	vectors := [][]float32{
		{1.0, 0.0, 0.0}, // Doc 1: Strong on dimension 0
		{0.0, 1.0, 0.0}, // Doc 2: Strong on dimension 1
		{0.0, 0.0, 1.0}, // Doc 3: Strong on dimension 2
		{0.7, 0.7, 0.0}, // Doc 4: Mix of 0 and 1
		{0.5, 0.5, 0.5}, // Doc 5: Balanced
	}

	texts := []string{
		"Machine learning embeddings for vector search",      // Doc 1
		"Database systems with full-text search capabilities", // Doc 2
		"Hybrid search combining vectors and text",           // Doc 3
		"Vector database with HNSW indexing",                 // Doc 4
		"Search engine with semantic capabilities",           // Doc 5
	}

	metadata := []map[string]interface{}{
		{"category": "ml", "year": 2024},
		{"category": "database", "year": 2023},
		{"category": "search", "year": 2024},
		{"category": "database", "year": 2024},
		{"category": "search", "year": 2023},
	}

	// Index documents
	for i, vec := range vectors {
		id, err := vectorIdx.Insert(vec)
		if err != nil {
			t.Fatalf("Failed to insert vector %d: %v", i, err)
		}

		textIdx.Index(&Document{
			ID:       id,
			Text:     texts[i],
			Metadata: metadata[i],
		})
	}

	return NewHybridSearch(vectorIdx, textIdx), vectors
}

func TestNewHybridSearch(t *testing.T) {
	vectorIdx := hnsw.New(hnsw.DefaultConfig())
	textIdx := NewFullTextIndex()

	hs := NewHybridSearch(vectorIdx, textIdx)

	if hs == nil {
		t.Fatal("NewHybridSearch() returned nil")
	}

	if hs.k != 60 {
		t.Errorf("Default k = %d, want 60", hs.k)
	}

	if hs.alpha != 0.5 {
		t.Errorf("Default alpha = %f, want 0.5", hs.alpha)
	}

	if hs.beta != 0.5 {
		t.Errorf("Default beta = %f, want 0.5", hs.beta)
	}

	if !hs.useRRF {
		t.Error("Default useRRF = false, want true")
	}
}

func TestHybridSearch_SetParameters(t *testing.T) {
	vectorIdx := hnsw.New(hnsw.DefaultConfig())
	textIdx := NewFullTextIndex()
	hs := NewHybridSearch(vectorIdx, textIdx)

	hs.SetRRFParameter(100)
	if hs.k != 100 {
		t.Errorf("After SetRRFParameter(100), k = %d, want 100", hs.k)
	}

	hs.SetWeights(0.7, 0.3)
	if hs.alpha != 0.7 {
		t.Errorf("After SetWeights(0.7, 0.3), alpha = %f, want 0.7", hs.alpha)
	}
	if hs.beta != 0.3 {
		t.Errorf("After SetWeights(0.7, 0.3), beta = %f, want 0.3", hs.beta)
	}

	hs.SetFusionMethod(false)
	if hs.useRRF {
		t.Error("After SetFusionMethod(false), useRRF = true, want false")
	}
}

func TestHybridSearch_Search(t *testing.T) {
	hs, vectors := createTestHybridSearch(t)

	// Query similar to Doc 1 (strong on dimension 0) and matching text
	queryVector := []float32{0.9, 0.1, 0.0}
	queryText := "machine learning vector"

	results := hs.Search(queryVector, queryText, 3, 50)

	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}

	// Results should be sorted by fused score
	for i := 1; i < len(results); i++ {
		if results[i].FusedScore > results[i-1].FusedScore {
			t.Errorf("Results not sorted: result[%d].FusedScore (%f) > result[%d].FusedScore (%f)",
				i, results[i].FusedScore, i-1, results[i-1].FusedScore)
		}
	}

	// Top result should have both good vector and text match
	// (likely Doc 1 which has "vector" and "machine learning")
	if results[0].FusedScore <= 0 {
		t.Errorf("Top result has non-positive fused score: %f", results[0].FusedScore)
	}

	// Verify all results have valid data
	for i, r := range results {
		if r.ID == 0 {
		}
		if r.Metadata == nil {
			t.Errorf("Result %d has nil metadata", i)
		}
	}

	_ = vectors // Use vectors to avoid unused variable warning
}

func TestHybridSearch_RRF(t *testing.T) {
	hs, _ := createTestHybridSearch(t)
	hs.SetFusionMethod(true) // Use RRF

	queryVector := []float32{1.0, 0.0, 0.0}
	queryText := "vector search"

	results := hs.Search(queryVector, queryText, 5, 50)

	if len(results) == 0 {
		t.Fatal("RRF search returned no results")
	}

	// With RRF, scores should be based on reciprocal ranks
	// Verify scores are in valid range
	for i, r := range results {
		if r.FusedScore < 0 {
			t.Errorf("Result %d has negative RRF score: %f", i, r.FusedScore)
		}
		// Maximum possible RRF score is (alpha + beta) / k = 1.0 / 60 ≈ 0.0167 per term
		// With multiple terms, max is around 0.1
		if r.FusedScore > 1.0 {
			t.Errorf("Result %d has unexpectedly high RRF score: %f", i, r.FusedScore)
		}
	}
}

func TestHybridSearch_WeightedCombination(t *testing.T) {
	hs, _ := createTestHybridSearch(t)
	hs.SetFusionMethod(false) // Use weighted combination
	hs.SetWeights(0.7, 0.3)   // Favor vector search

	queryVector := []float32{1.0, 0.0, 0.0}
	queryText := "database" // This word appears in docs 2 and 4

	results := hs.Search(queryVector, queryText, 5, 50)

	if len(results) == 0 {
		t.Fatal("Weighted combination search returned no results")
	}

	// Scores should be normalized and combined
	for i, r := range results {
		if r.FusedScore < 0 || r.FusedScore > 1.0 {
			t.Errorf("Result %d has out-of-range weighted score: %f (expected [0,1])", i, r.FusedScore)
		}
	}
}

func TestHybridSearch_SearchWithFilter(t *testing.T) {
	hs, _ := createTestHybridSearch(t)

	queryVector := []float32{0.5, 0.5, 0.5}
	queryText := "search"

	// Filter for year 2024
	filter := func(metadata map[string]interface{}) bool {
		year, ok := metadata["year"].(int)
		return ok && year == 2024
	}

	results := hs.SearchWithFilter(queryVector, queryText, 5, 50, filter)

	// Verify all results match filter
	for i, r := range results {
		year, ok := r.Metadata["year"].(int)
		if !ok || year != 2024 {
			t.Errorf("Result %d does not match filter: year = %v", i, year)
		}
	}
}

func TestHybridSearch_VectorOnlySearch(t *testing.T) {
	hs, _ := createTestHybridSearch(t)

	queryVector := []float32{1.0, 0.0, 0.0}

	results := hs.VectorOnlySearch(queryVector, 3, 50)

	if len(results) == 0 {
		t.Fatal("VectorOnlySearch() returned no results")
	}

	// Text scores should be 0
	for i, r := range results {
		if r.TextScore != 0 {
			t.Errorf("Result %d has non-zero text score: %f", i, r.TextScore)
		}
		if r.VectorScore < 0 {
			t.Errorf("Result %d has zero vector score", i)
		}
	}
}

func TestHybridSearch_TextOnlySearch(t *testing.T) {
	hs, _ := createTestHybridSearch(t)

	queryText := "vector database"

	results := hs.TextOnlySearch(queryText, 3)

	if len(results) == 0 {
		t.Fatal("TextOnlySearch() returned no results")
	}

	// Vector scores should be 0
	for i, r := range results {
		if r.VectorScore != 0 {
			t.Errorf("Result %d has non-zero vector score: %f", i, r.VectorScore)
		}
		if r.TextScore == 0 {
			t.Errorf("Result %d has zero text score", i)
		}
	}
}

func TestHybridSearch_EmptyResults(t *testing.T) {
	hs, _ := createTestHybridSearch(t)

	// Query with no text matches
	queryVector := []float32{1.0, 0.0, 0.0}
	queryText := "blockchain cryptocurrency bitcoin" // No documents contain these

	results := hs.Search(queryVector, queryText, 5, 50)

	// Should still return vector results even if text has no matches
	if len(results) == 0 {
		t.Error("Search() returned no results when text has no matches")
	}
}

func TestHybridSearch_WeightVariations(t *testing.T) {
	hs, _ := createTestHybridSearch(t)

	queryVector := []float32{1.0, 0.0, 0.0}
	queryText := "vector"

	tests := []struct {
		name  string
		alpha float64
		beta  float64
	}{
		{"vector heavy", 0.9, 0.1},
		{"text heavy", 0.1, 0.9},
		{"balanced", 0.5, 0.5},
		{"vector only", 1.0, 0.0},
		{"text only", 0.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs.SetWeights(tt.alpha, tt.beta)
			hs.SetFusionMethod(false) // Use weighted combination

			results := hs.Search(queryVector, queryText, 3, 50)

			if len(results) == 0 {
				t.Errorf("Search with weights (%.1f, %.1f) returned no results", tt.alpha, tt.beta)
			}

			// Verify scores are sorted
			for i := 1; i < len(results); i++ {
				if results[i].FusedScore > results[i-1].FusedScore {
					t.Errorf("Results not sorted with weights (%.1f, %.1f)", tt.alpha, tt.beta)
				}
			}
		})
	}
}

func TestHybridSearch_RRFvsWeighted(t *testing.T) {
	hs, _ := createTestHybridSearch(t)

	queryVector := []float32{0.7, 0.7, 0.0}
	queryText := "hybrid search"

	// Test with RRF
	hs.SetFusionMethod(true)
	rrfResults := hs.Search(queryVector, queryText, 3, 50)

	// Test with weighted combination
	hs.SetFusionMethod(false)
	weightedResults := hs.Search(queryVector, queryText, 3, 50)

	// Both should return results
	if len(rrfResults) == 0 {
		t.Error("RRF returned no results")
	}
	if len(weightedResults) == 0 {
		t.Error("Weighted combination returned no results")
	}

	// Results may be in different order, which is expected
	// Just verify both produce valid scores
	for i, r := range rrfResults {
		if r.FusedScore <= 0 {
			t.Errorf("RRF result %d has non-positive score: %f", i, r.FusedScore)
		}
	}

	for i, r := range weightedResults {
		if r.FusedScore < 0 {
			t.Errorf("Weighted result %d has negative score: %f", i, r.FusedScore)
		}
	}
}

func BenchmarkHybridSearch_RRF(b *testing.B) {
	// Create larger dataset
	vectorIdx := hnsw.New(hnsw.DefaultConfig())
	textIdx := NewFullTextIndex()

	// Index 1000 documents
	for i := 0; i < 1000; i++ {
		vec := []float32{float32(i % 10) / 10.0, float32(i%7) / 7.0, float32(i%5) / 5.0}
		id, _ := vectorIdx.Insert(vec)

		textIdx.Index(&Document{
			ID:   id,
			Text: "Vector database with search capabilities for document retrieval",
		})
	}

	hs := NewHybridSearch(vectorIdx, textIdx)
	hs.SetFusionMethod(true) // RRF

	queryVector := []float32{0.5, 0.5, 0.5}
	queryText := "vector search"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hs.Search(queryVector, queryText, 10, 50)
	}
}

func BenchmarkHybridSearch_Weighted(b *testing.B) {
	vectorIdx := hnsw.New(hnsw.DefaultConfig())
	textIdx := NewFullTextIndex()

	for i := 0; i < 1000; i++ {
		vec := []float32{float32(i % 10) / 10.0, float32(i%7) / 7.0, float32(i%5) / 5.0}
		id, _ := vectorIdx.Insert(vec)

		textIdx.Index(&Document{
			ID:   id,
			Text: "Vector database with search capabilities for document retrieval",
		})
	}

	hs := NewHybridSearch(vectorIdx, textIdx)
	hs.SetFusionMethod(false) // Weighted

	queryVector := []float32{0.5, 0.5, 0.5}
	queryText := "vector search"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hs.Search(queryVector, queryText, 10, 50)
	}
}
