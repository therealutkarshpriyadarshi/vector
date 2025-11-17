package search

import (
	"github.com/therealutkarshpriyadarshi/vector/pkg/hnsw"
)

// HybridSearchResult represents a result from hybrid search combining vector and text scores
type HybridSearchResult struct {
	ID          uint64
	VectorScore float32                // Distance from vector search (lower is better)
	TextScore   float64                // BM25 score from text search (higher is better)
	FusedScore  float64                // Combined RRF score (higher is better)
	Metadata    map[string]interface{} // Document metadata
}

// HybridSearch performs hybrid search combining vector similarity and full-text search
// using Reciprocal Rank Fusion (RRF) to merge results
type HybridSearch struct {
	vectorIndex *hnsw.Index
	textIndex   *FullTextIndex

	// RRF parameters
	k        int     // Constant for RRF formula (typically 60)
	alpha    float64 // Weight for vector results (0-1)
	beta     float64 // Weight for text results (0-1)
	useRRF   bool    // If false, use weighted score combination instead
}

// NewHybridSearch creates a new hybrid search instance
func NewHybridSearch(vectorIndex *hnsw.Index, textIndex *FullTextIndex) *HybridSearch {
	return &HybridSearch{
		vectorIndex: vectorIndex,
		textIndex:   textIndex,
		k:           60,   // Standard RRF constant
		alpha:       0.5,  // Equal weight for vector and text by default
		beta:        0.5,
		useRRF:      true,
	}
}

// SetRRFParameter sets the k parameter for RRF formula
func (hs *HybridSearch) SetRRFParameter(k int) {
	hs.k = k
}

// SetWeights sets the weights for vector and text search
// alpha: weight for vector results (0-1)
// beta: weight for text results (0-1)
// Typically alpha + beta = 1.0
func (hs *HybridSearch) SetWeights(alpha, beta float64) {
	hs.alpha = alpha
	hs.beta = beta
}

// SetFusionMethod sets whether to use RRF (true) or weighted combination (false)
func (hs *HybridSearch) SetFusionMethod(useRRF bool) {
	hs.useRRF = useRRF
}

// Search performs hybrid search
// queryVector: the query embedding for vector search
// queryText: the query text for full-text search
// k: number of results to return
// efSearch: HNSW efSearch parameter
func (hs *HybridSearch) Search(queryVector []float32, queryText string, k int, efSearch int) []*HybridSearchResult {
	// Perform vector search
	vectorSearchResult, err := hs.vectorIndex.Search(queryVector, k*2, efSearch) // Get more to ensure good fusion
	if err != nil {
		// If vector search fails, return text-only results
		return hs.TextOnlySearch(queryText, k)
	}
	vectorResults := vectorSearchResult.Results

	// Perform text search
	textResults := hs.textIndex.Search(queryText, k*2)

	// Merge using RRF or weighted combination
	if hs.useRRF {
		return hs.reciprocalRankFusion(vectorResults, textResults, k)
	}
	return hs.weightedCombination(vectorResults, textResults, k)
}

// SearchWithFilter performs hybrid search with metadata filtering
func (hs *HybridSearch) SearchWithFilter(queryVector []float32, queryText string, k int, efSearch int, filter FilterFunc) []*HybridSearchResult {
	// Perform vector search (we'll filter after)
	vectorSearchResult, err := hs.vectorIndex.Search(queryVector, k*3, efSearch) // Get extra for filtering
	var vectorResults []hnsw.Result
	if err == nil {
		vectorResults = vectorSearchResult.Results
	}

	// Perform text search with filter
	textResults := hs.textIndex.SearchWithFilter(queryText, k*2, filter)

	// Filter vector results
	filteredVectorResults := make([]hnsw.Result, 0, len(vectorResults))
	for _, vr := range vectorResults {
		// Get document from text index to check metadata
		doc := hs.textIndex.GetDocument(vr.ID)
		if doc != nil && (filter == nil || filter(doc.Metadata)) {
			filteredVectorResults = append(filteredVectorResults, vr)
		}
	}

	// Merge using RRF or weighted combination
	if hs.useRRF {
		return hs.reciprocalRankFusion(filteredVectorResults, textResults, k)
	}
	return hs.weightedCombination(filteredVectorResults, textResults, k)
}

// reciprocalRankFusion implements the RRF algorithm
// RRF score = Σ(α / (k + rank_vector)) + Σ(β / (k + rank_text))
func (hs *HybridSearch) reciprocalRankFusion(vectorResults []hnsw.Result, textResults []*FullTextResult, topK int) []*HybridSearchResult {
	// Build rank maps for efficient lookup
	vectorRanks := make(map[uint64]int)
	for rank, result := range vectorResults {
		vectorRanks[result.ID] = rank + 1 // Rank starts at 1
	}

	textRanks := make(map[uint64]int)
	for rank, result := range textResults {
		textRanks[result.ID] = rank + 1
	}

	// Collect all unique document IDs
	allDocs := make(map[uint64]bool)
	for id := range vectorRanks {
		allDocs[id] = true
	}
	for id := range textRanks {
		allDocs[id] = true
	}

	// Calculate RRF scores
	results := make([]*HybridSearchResult, 0, len(allDocs))

	for docID := range allDocs {
		rrfScore := 0.0

		// Add vector contribution
		if vectorRank, exists := vectorRanks[docID]; exists {
			rrfScore += hs.alpha / float64(hs.k+vectorRank)
		}

		// Add text contribution
		if textRank, exists := textRanks[docID]; exists {
			rrfScore += hs.beta / float64(hs.k+textRank)
		}

		// Get original scores for reference
		var vectorScore float32
		var textScore float64

		for _, vr := range vectorResults {
			if vr.ID == docID {
				vectorScore = vr.Distance
				break
			}
		}

		for _, tr := range textResults {
			if tr.ID == docID {
				textScore = tr.Score
				break
			}
		}

		// Get metadata
		var metadata map[string]interface{}
		if doc := hs.textIndex.GetDocument(docID); doc != nil {
			metadata = doc.Metadata
		}

		results = append(results, &HybridSearchResult{
			ID:          docID,
			VectorScore: vectorScore,
			TextScore:   textScore,
			FusedScore:  rrfScore,
			Metadata:    metadata,
		})
	}

	// Sort by RRF score (descending)
	sortByFusedScore(results)

	// Return top k
	if topK < len(results) {
		results = results[:topK]
	}

	return results
}

// weightedCombination uses weighted score combination instead of RRF
// This normalizes scores and combines them with weights
func (hs *HybridSearch) weightedCombination(vectorResults []hnsw.Result, textResults []*FullTextResult, topK int) []*HybridSearchResult {
	// Normalize vector scores (distances) to [0, 1] where 1 is best
	// We invert distances so that smaller distance = higher score
	var maxVectorDist float32 = 0
	for _, vr := range vectorResults {
		if vr.Distance > maxVectorDist {
			maxVectorDist = vr.Distance
		}
	}

	vectorScores := make(map[uint64]float64)
	for _, vr := range vectorResults {
		if maxVectorDist > 0 {
			// Normalize and invert: 1 - (dist / maxDist)
			vectorScores[vr.ID] = float64(1.0 - (vr.Distance / maxVectorDist))
		} else {
			vectorScores[vr.ID] = 1.0
		}
	}

	// Normalize text scores to [0, 1]
	var maxTextScore float64 = 0
	for _, tr := range textResults {
		if tr.Score > maxTextScore {
			maxTextScore = tr.Score
		}
	}

	textScores := make(map[uint64]float64)
	for _, tr := range textResults {
		if maxTextScore > 0 {
			textScores[tr.ID] = tr.Score / maxTextScore
		} else {
			textScores[tr.ID] = 1.0
		}
	}

	// Collect all unique document IDs
	allDocs := make(map[uint64]bool)
	for id := range vectorScores {
		allDocs[id] = true
	}
	for id := range textScores {
		allDocs[id] = true
	}

	// Calculate combined scores
	results := make([]*HybridSearchResult, 0, len(allDocs))

	for docID := range allDocs {
		combinedScore := hs.alpha*vectorScores[docID] + hs.beta*textScores[docID]

		// Get original scores
		var vectorScore float32
		var textScore float64

		for _, vr := range vectorResults {
			if vr.ID == docID {
				vectorScore = vr.Distance
				break
			}
		}

		for _, tr := range textResults {
			if tr.ID == docID {
				textScore = tr.Score
				break
			}
		}

		// Get metadata
		var metadata map[string]interface{}
		if doc := hs.textIndex.GetDocument(docID); doc != nil {
			metadata = doc.Metadata
		}

		results = append(results, &HybridSearchResult{
			ID:          docID,
			VectorScore: vectorScore,
			TextScore:   textScore,
			FusedScore:  combinedScore,
			Metadata:    metadata,
		})
	}

	// Sort by combined score (descending)
	sortByFusedScore(results)

	// Return top k
	if topK < len(results) {
		results = results[:topK]
	}

	return results
}

// sortByFusedScore sorts results by fused score in descending order
func sortByFusedScore(results []*HybridSearchResult) {
	// Insertion sort (efficient for small k)
	for i := 1; i < len(results); i++ {
		key := results[i]
		j := i - 1
		for j >= 0 && results[j].FusedScore < key.FusedScore {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}
}

// VectorOnlySearch performs vector-only search (no text fusion)
func (hs *HybridSearch) VectorOnlySearch(queryVector []float32, k int, efSearch int) []*HybridSearchResult {
	vectorSearchResult, err := hs.vectorIndex.Search(queryVector, k, efSearch)
	if err != nil {
		return nil
	}
	vectorResults := vectorSearchResult.Results

	results := make([]*HybridSearchResult, len(vectorResults))
	for i, vr := range vectorResults {
		var metadata map[string]interface{}
		if doc := hs.textIndex.GetDocument(vr.ID); doc != nil {
			metadata = doc.Metadata
		}

		results[i] = &HybridSearchResult{
			ID:          vr.ID,
			VectorScore: vr.Distance,
			TextScore:   0,
			FusedScore:  float64(-vr.Distance), // Negative distance as score
			Metadata:    metadata,
		}
	}

	return results
}

// TextOnlySearch performs text-only search (no vector fusion)
func (hs *HybridSearch) TextOnlySearch(queryText string, k int) []*HybridSearchResult {
	textResults := hs.textIndex.Search(queryText, k)

	results := make([]*HybridSearchResult, len(textResults))
	for i, tr := range textResults {
		results[i] = &HybridSearchResult{
			ID:          tr.ID,
			VectorScore: 0,
			TextScore:   tr.Score,
			FusedScore:  tr.Score,
			Metadata:    tr.Document.Metadata,
		}
	}

	return results
}
