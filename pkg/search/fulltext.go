package search

import (
	"math"
	"strings"
	"sync"
	"unicode"
)

// Document represents a searchable document with text content and metadata
type Document struct {
	ID       uint64
	Text     string
	Metadata map[string]interface{}
}

// FullTextIndex implements BM25-based full-text search
// BM25 (Best Matching 25) is a probabilistic ranking function used by search engines
type FullTextIndex struct {
	// Configuration parameters
	k1 float64 // Term frequency saturation parameter (typical: 1.2-2.0)
	b  float64 // Length normalization parameter (typical: 0.75)

	// Index structures
	documents     map[uint64]*Document         // Document storage
	invertedIndex map[string]map[uint64]int    // term -> {docID -> term frequency}
	docLengths    map[uint64]int               // Document lengths (word count)
	avgDocLength  float64                      // Average document length
	docCount      int                          // Total number of documents

	mu sync.RWMutex
}

// FullTextResult represents a search result with BM25 score
type FullTextResult struct {
	ID       uint64
	Score    float64
	Document *Document
}

// NewFullTextIndex creates a new full-text search index with BM25 scoring
func NewFullTextIndex() *FullTextIndex {
	return &FullTextIndex{
		k1:            1.5,  // Standard BM25 k1 parameter
		b:             0.75, // Standard BM25 b parameter
		documents:     make(map[uint64]*Document),
		invertedIndex: make(map[string]map[uint64]int),
		docLengths:    make(map[uint64]int),
	}
}

// SetParameters allows customization of BM25 parameters
func (idx *FullTextIndex) SetParameters(k1, b float64) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.k1 = k1
	idx.b = b
}

// tokenize splits text into lowercase words, removing punctuation
func tokenize(text string) []string {
	// Convert to lowercase and split into words
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	// Filter out very short tokens (< 2 chars)
	filtered := make([]string, 0, len(words))
	for _, word := range words {
		if len(word) >= 2 {
			filtered = append(filtered, word)
		}
	}

	return filtered
}

// Index adds or updates a document in the full-text index
func (idx *FullTextIndex) Index(doc *Document) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Tokenize the document text
	tokens := tokenize(doc.Text)

	// Remove old document if it exists
	if oldDoc, exists := idx.documents[doc.ID]; exists {
		idx.removeDocumentLocked(oldDoc)
	}

	// Store the document
	idx.documents[doc.ID] = doc
	idx.docLengths[doc.ID] = len(tokens)
	idx.docCount++

	// Build term frequencies for this document
	termFreq := make(map[string]int)
	for _, token := range tokens {
		termFreq[token]++
	}

	// Update inverted index
	for term, freq := range termFreq {
		if idx.invertedIndex[term] == nil {
			idx.invertedIndex[term] = make(map[uint64]int)
		}
		idx.invertedIndex[term][doc.ID] = freq
	}

	// Update average document length
	idx.updateAvgDocLengthLocked()

	return nil
}

// BatchIndex indexes multiple documents efficiently
func (idx *FullTextIndex) BatchIndex(docs []*Document) error {
	for _, doc := range docs {
		if err := idx.Index(doc); err != nil {
			return err
		}
	}
	return nil
}

// Remove removes a document from the index
func (idx *FullTextIndex) Remove(docID uint64) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	doc, exists := idx.documents[docID]
	if !exists {
		return nil // Document doesn't exist, nothing to remove
	}

	idx.removeDocumentLocked(doc)
	return nil
}

// removeDocumentLocked removes a document (must be called with lock held)
func (idx *FullTextIndex) removeDocumentLocked(doc *Document) {
	tokens := tokenize(doc.Text)
	termFreq := make(map[string]int)
	for _, token := range tokens {
		termFreq[token]++
	}

	// Remove from inverted index
	for term := range termFreq {
		if postings, exists := idx.invertedIndex[term]; exists {
			delete(postings, doc.ID)
			if len(postings) == 0 {
				delete(idx.invertedIndex, term)
			}
		}
	}

	delete(idx.documents, doc.ID)
	delete(idx.docLengths, doc.ID)
	idx.docCount--
	idx.updateAvgDocLengthLocked()
}

// updateAvgDocLengthLocked recalculates average document length
func (idx *FullTextIndex) updateAvgDocLengthLocked() {
	if idx.docCount == 0 {
		idx.avgDocLength = 0
		return
	}

	totalLength := 0
	for _, length := range idx.docLengths {
		totalLength += length
	}
	idx.avgDocLength = float64(totalLength) / float64(idx.docCount)
}

// Search performs BM25-based full-text search
// Returns top k documents ranked by BM25 score
func (idx *FullTextIndex) Search(query string, k int) []*FullTextResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.docCount == 0 {
		return nil
	}

	// Tokenize query
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	// Calculate BM25 scores for all documents
	scores := make(map[uint64]float64)

	for _, term := range queryTokens {
		postings, exists := idx.invertedIndex[term]
		if !exists {
			continue // Term not in index
		}

		// Calculate IDF for this term using IDF+ (ensures positive values)
		// IDF = log(1 + (N - df + 0.5) / (df + 0.5))
		// where N = total docs, df = document frequency
		N := float64(idx.docCount)
		df := float64(len(postings))
		idf := math.Log(1 + (N-df+0.5)/(df+0.5))

		// Calculate BM25 component for each document containing this term
		for docID, termFreq := range postings {
			// BM25 formula:
			// score = IDF * (tf * (k1 + 1)) / (tf + k1 * (1 - b + b * (dl / avgdl)))
			// where:
			//   tf = term frequency in document
			//   dl = document length
			//   avgdl = average document length
			//   k1, b = tuning parameters

			tf := float64(termFreq)
			dl := float64(idx.docLengths[docID])
			avgdl := idx.avgDocLength

			numerator := tf * (idx.k1 + 1)
			denominator := tf + idx.k1*(1-idx.b+idx.b*(dl/avgdl))

			scores[docID] += idf * (numerator / denominator)
		}
	}

	// Convert to results and sort by score
	results := make([]*FullTextResult, 0, len(scores))
	for docID, score := range scores {
		results = append(results, &FullTextResult{
			ID:       docID,
			Score:    score,
			Document: idx.documents[docID],
		})
	}

	// Sort by score (descending)
	sortByScore(results)

	// Return top k
	if k < len(results) {
		results = results[:k]
	}

	return results
}

// SearchWithFilter performs full-text search with metadata filtering
func (idx *FullTextIndex) SearchWithFilter(query string, k int, filter FilterFunc) []*FullTextResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.docCount == 0 {
		return nil
	}

	// Tokenize query
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	// Calculate BM25 scores for all documents
	scores := make(map[uint64]float64)

	for _, term := range queryTokens {
		postings, exists := idx.invertedIndex[term]
		if !exists {
			continue
		}

		N := float64(idx.docCount)
		df := float64(len(postings))
		idf := math.Log(1 + (N-df+0.5)/(df+0.5))

		for docID, termFreq := range postings {
			// Apply filter
			doc := idx.documents[docID]
			if filter != nil && !filter(doc.Metadata) {
				continue
			}

			tf := float64(termFreq)
			dl := float64(idx.docLengths[docID])
			avgdl := idx.avgDocLength

			numerator := tf * (idx.k1 + 1)
			denominator := tf + idx.k1*(1-idx.b+idx.b*(dl/avgdl))

			scores[docID] += idf * (numerator / denominator)
		}
	}

	// Convert to results and sort
	results := make([]*FullTextResult, 0, len(scores))
	for docID, score := range scores {
		results = append(results, &FullTextResult{
			ID:       docID,
			Score:    score,
			Document: idx.documents[docID],
		})
	}

	sortByScore(results)

	if k < len(results) {
		results = results[:k]
	}

	return results
}

// GetDocument retrieves a document by ID
func (idx *FullTextIndex) GetDocument(id uint64) *Document {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.documents[id]
}

// Size returns the number of documents in the index
func (idx *FullTextIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.docCount
}

// sortByScore sorts results by score in descending order
func sortByScore(results []*FullTextResult) {
	// Simple insertion sort (efficient for small k)
	for i := 1; i < len(results); i++ {
		key := results[i]
		j := i - 1
		for j >= 0 && results[j].Score < key.Score {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}
}

// FilterFunc is a function that filters documents based on metadata
type FilterFunc func(metadata map[string]interface{}) bool
