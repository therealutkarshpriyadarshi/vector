package search

import (
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/vector/pkg/hnsw"
)

func TestLRUCache_Basic(t *testing.T) {
	cache := NewLRUCache(2, 0) // Capacity 2, no TTL

	// Put first item
	cache.Put("key1", "value1")
	if cache.Size() != 1 {
		t.Errorf("Size() = %d, want 1", cache.Size())
	}

	// Get existing item
	val, found := cache.Get("key1")
	if !found {
		t.Error("Get() didn't find existing key")
	}
	if val != "value1" {
		t.Errorf("Get() = %v, want value1", val)
	}

	// Get non-existent item
	_, found = cache.Get("key2")
	if found {
		t.Error("Get() found non-existent key")
	}
}

func TestLRUCache_Eviction(t *testing.T) {
	cache := NewLRUCache(2, 0)

	cache.Put("key1", "value1")
	cache.Put("key2", "value2")
	cache.Put("key3", "value3") // Should evict key1

	if cache.Size() != 2 {
		t.Errorf("Size() = %d, want 2", cache.Size())
	}

	// key1 should be evicted
	_, found := cache.Get("key1")
	if found {
		t.Error("key1 should have been evicted")
	}

	// key2 and key3 should still exist
	_, found = cache.Get("key2")
	if !found {
		t.Error("key2 should still exist")
	}

	_, found = cache.Get("key3")
	if !found {
		t.Error("key3 should still exist")
	}
}

func TestLRUCache_LRUOrdering(t *testing.T) {
	cache := NewLRUCache(2, 0)

	cache.Put("key1", "value1")
	cache.Put("key2", "value2")

	// Access key1 to make it more recently used
	cache.Get("key1")

	// Add key3 - should evict key2 (least recently used)
	cache.Put("key3", "value3")

	// key1 should still exist
	_, found := cache.Get("key1")
	if !found {
		t.Error("key1 should still exist")
	}

	// key2 should be evicted
	_, found = cache.Get("key2")
	if found {
		t.Error("key2 should have been evicted")
	}

	// key3 should exist
	_, found = cache.Get("key3")
	if !found {
		t.Error("key3 should exist")
	}
}

func TestLRUCache_Update(t *testing.T) {
	cache := NewLRUCache(2, 0)

	cache.Put("key1", "value1")
	cache.Put("key1", "value2") // Update

	if cache.Size() != 1 {
		t.Errorf("Size() = %d, want 1", cache.Size())
	}

	val, found := cache.Get("key1")
	if !found {
		t.Error("Get() didn't find updated key")
	}
	if val != "value2" {
		t.Errorf("Get() = %v, want value2", val)
	}
}

func TestLRUCache_TTL(t *testing.T) {
	cache := NewLRUCache(10, 100*time.Millisecond)

	cache.Put("key1", "value1")

	// Should exist immediately
	_, found := cache.Get("key1")
	if !found {
		t.Error("key1 should exist immediately after put")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	_, found = cache.Get("key1")
	if found {
		t.Error("key1 should be expired")
	}
}

func TestLRUCache_Invalidate(t *testing.T) {
	cache := NewLRUCache(10, 0)

	cache.Put("key1", "value1")
	cache.Put("key2", "value2")

	// Invalidate key1
	cache.Invalidate("key1")

	if cache.Size() != 1 {
		t.Errorf("Size() after invalidate = %d, want 1", cache.Size())
	}

	_, found := cache.Get("key1")
	if found {
		t.Error("key1 should be invalidated")
	}

	_, found = cache.Get("key2")
	if !found {
		t.Error("key2 should still exist")
	}
}

func TestLRUCache_Clear(t *testing.T) {
	cache := NewLRUCache(10, 0)

	cache.Put("key1", "value1")
	cache.Put("key2", "value2")
	cache.Put("key3", "value3")

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Size() after clear = %d, want 0", cache.Size())
	}

	stats := cache.Stats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Error("Stats should be reset after clear")
	}
}

func TestLRUCache_Stats(t *testing.T) {
	cache := NewLRUCache(10, 0)

	cache.Put("key1", "value1")
	cache.Put("key2", "value2")

	// Generate some hits
	cache.Get("key1")
	cache.Get("key1")
	cache.Get("key2")

	// Generate some misses
	cache.Get("key3")
	cache.Get("key4")

	stats := cache.Stats()

	if stats.Hits != 3 {
		t.Errorf("Stats.Hits = %d, want 3", stats.Hits)
	}

	if stats.Misses != 2 {
		t.Errorf("Stats.Misses = %d, want 2", stats.Misses)
	}

	expectedHitRate := 3.0 / 5.0
	if stats.HitRate != expectedHitRate {
		t.Errorf("Stats.HitRate = %f, want %f", stats.HitRate, expectedHitRate)
	}
}

func TestGenerateVectorQueryKey(t *testing.T) {
	vec1 := []float32{1.0, 2.0, 3.0}
	vec2 := []float32{1.0, 2.0, 3.0}
	vec3 := []float32{1.0, 2.0, 3.1}

	key1 := GenerateVectorQueryKey(vec1, 10, 50)
	key2 := GenerateVectorQueryKey(vec2, 10, 50)
	key3 := GenerateVectorQueryKey(vec3, 10, 50)

	// Same vectors should generate same key
	if key1 != key2 {
		t.Error("Same vectors should generate same cache key")
	}

	// Different vectors should generate different keys
	if key1 == key3 {
		t.Error("Different vectors should generate different cache keys")
	}

	// Different parameters should generate different keys
	key4 := GenerateVectorQueryKey(vec1, 20, 50)
	if key1 == key4 {
		t.Error("Different k parameter should generate different cache key")
	}
}

func TestGenerateTextQueryKey(t *testing.T) {
	key1 := GenerateTextQueryKey("vector search", 10)
	key2 := GenerateTextQueryKey("vector search", 10)
	key3 := GenerateTextQueryKey("vector database", 10)

	// Same queries should generate same key
	if key1 != key2 {
		t.Error("Same queries should generate same cache key")
	}

	// Different queries should generate different keys
	if key1 == key3 {
		t.Error("Different queries should generate different cache keys")
	}
}

func TestGenerateHybridQueryKey(t *testing.T) {
	vec1 := []float32{1.0, 2.0, 3.0}
	vec2 := []float32{1.0, 2.0, 3.0}

	key1 := GenerateHybridQueryKey(vec1, "search", 10, 50)
	key2 := GenerateHybridQueryKey(vec2, "search", 10, 50)
	key3 := GenerateHybridQueryKey(vec1, "query", 10, 50)

	// Same inputs should generate same key
	if key1 != key2 {
		t.Error("Same inputs should generate same cache key")
	}

	// Different text should generate different keys
	if key1 == key3 {
		t.Error("Different text should generate different cache keys")
	}
}

func TestQueryCache_HybridResults(t *testing.T) {
	cache := NewQueryCache(10, 0)

	results := []*HybridSearchResult{
		{ID: 1, FusedScore: 0.9},
		{ID: 2, FusedScore: 0.8},
	}

	key := CacheKey("test-key")

	// Put results
	cache.PutHybridResults(key, results)

	// Get results
	cached, found := cache.GetHybridResults(key)
	if !found {
		t.Error("Results should be in cache")
	}

	if len(cached) != len(results) {
		t.Errorf("Cached results length = %d, want %d", len(cached), len(results))
	}

	if cached[0].ID != results[0].ID {
		t.Error("Cached results don't match original")
	}
}

func TestQueryCache_TextResults(t *testing.T) {
	cache := NewQueryCache(10, 0)

	results := []*FullTextResult{
		{ID: 1, Score: 0.9},
		{ID: 2, Score: 0.8},
	}

	key := CacheKey("test-key")

	cache.PutTextResults(key, results)

	cached, found := cache.GetTextResults(key)
	if !found {
		t.Error("Results should be in cache")
	}

	if len(cached) != len(results) {
		t.Errorf("Cached results length = %d, want %d", len(cached), len(results))
	}
}

func TestCachedHybridSearch(t *testing.T) {
	// Create test index
	vectorIdx := hnsw.New(hnsw.DefaultConfig())
	textIdx := NewFullTextIndex()

	// Index test data
	vec1 := []float32{1.0, 0.0, 0.0}
	id1, _ := vectorIdx.Insert(vec1)
	textIdx.Index(&Document{
		ID:   id1,
		Text: "vector database search",
	})

	// Create cached search
	chs := NewCachedHybridSearch(vectorIdx, textIdx, 10, 0)

	// First search (cache miss)
	results1 := chs.Search(vec1, "vector", 10, 50)
	stats1 := chs.CacheStats()

	if stats1.Misses != 1 {
		t.Errorf("First search should be cache miss, got %d misses", stats1.Misses)
	}

	// Second identical search (cache hit)
	results2 := chs.Search(vec1, "vector", 10, 50)
	stats2 := chs.CacheStats()

	if stats2.Hits != 1 {
		t.Errorf("Second search should be cache hit, got %d hits", stats2.Hits)
	}

	// Results should be the same
	if len(results1) != len(results2) {
		t.Error("Cached and uncached results should have same length")
	}

	if len(results1) > 0 && results1[0].ID != results2[0].ID {
		t.Error("Cached and uncached results should be identical")
	}
}

func TestCachedHybridSearch_Invalidate(t *testing.T) {
	vectorIdx := hnsw.New(hnsw.DefaultConfig())
	textIdx := NewFullTextIndex()

	vec1 := []float32{1.0, 0.0, 0.0}
	id1, _ := vectorIdx.Insert(vec1)
	textIdx.Index(&Document{
		ID:   id1,
		Text: "vector database",
	})

	chs := NewCachedHybridSearch(vectorIdx, textIdx, 10, 0)

	// Perform search to populate cache
	chs.Search(vec1, "vector", 10, 50)

	stats1 := chs.CacheStats()
	if stats1.Size != 1 {
		t.Errorf("Cache size = %d, want 1", stats1.Size)
	}

	// Invalidate cache
	chs.InvalidateCache()

	stats2 := chs.CacheStats()
	if stats2.Size != 0 {
		t.Errorf("Cache size after invalidate = %d, want 0", stats2.Size)
	}
}

func BenchmarkLRUCache_Put(b *testing.B) {
	cache := NewLRUCache(1000, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := CacheKey(string(rune(i % 1000)))
		cache.Put(key, i)
	}
}

func BenchmarkLRUCache_Get(b *testing.B) {
	cache := NewLRUCache(1000, 0)

	// Populate cache
	for i := 0; i < 1000; i++ {
		key := CacheKey(string(rune(i)))
		cache.Put(key, i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := CacheKey(string(rune(i % 1000)))
		cache.Get(key)
	}
}

func BenchmarkGenerateVectorQueryKey(b *testing.B) {
	vec := make([]float32, 768) // Typical embedding size
	for i := range vec {
		vec[i] = float32(i) / 768.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GenerateVectorQueryKey(vec, 10, 50)
	}
}
