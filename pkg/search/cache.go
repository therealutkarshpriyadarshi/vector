package search

import (
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/therealutkarshpriyadarshi/vector/pkg/hnsw"
)

// CacheKey represents a unique key for caching search results
type CacheKey string

// LRUCache implements a thread-safe LRU (Least Recently Used) cache
type LRUCache struct {
	capacity int
	ttl      time.Duration // Time-to-live for cache entries

	mu    sync.RWMutex
	cache map[CacheKey]*list.Element
	lru   *list.List

	// Statistics
	hits   int64
	misses int64
}

// cacheEntry represents a single entry in the cache
type cacheEntry struct {
	key       CacheKey
	value     interface{}
	expiresAt time.Time
}

// NewLRUCache creates a new LRU cache with the given capacity
// capacity: maximum number of items to store
// ttl: time-to-live for entries (0 = no expiration)
func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		ttl:      ttl,
		cache:    make(map[CacheKey]*list.Element, capacity),
		lru:      list.New(),
	}
}

// Get retrieves a value from the cache
// Returns (value, true) if found, (nil, false) if not found or expired
func (c *LRUCache) Get(key CacheKey) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, exists := c.cache[key]
	if !exists {
		c.misses++
		return nil, false
	}

	entry := elem.Value.(*cacheEntry)

	// Check if expired
	if c.ttl > 0 && time.Now().After(entry.expiresAt) {
		c.removeElement(elem)
		c.misses++
		return nil, false
	}

	// Move to front (most recently used)
	c.lru.MoveToFront(elem)
	c.hits++

	return entry.value, true
}

// Put adds or updates a value in the cache
func (c *LRUCache) Put(key CacheKey, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key already exists
	if elem, exists := c.cache[key]; exists {
		// Update existing entry
		entry := elem.Value.(*cacheEntry)
		entry.value = value
		if c.ttl > 0 {
			entry.expiresAt = time.Now().Add(c.ttl)
		}
		c.lru.MoveToFront(elem)
		return
	}

	// Create new entry
	entry := &cacheEntry{
		key:   key,
		value: value,
	}
	if c.ttl > 0 {
		entry.expiresAt = time.Now().Add(c.ttl)
	}

	elem := c.lru.PushFront(entry)
	c.cache[key] = elem

	// Evict if over capacity
	if c.lru.Len() > c.capacity {
		c.evictOldest()
	}
}

// Invalidate removes a specific key from the cache
func (c *LRUCache) Invalidate(key CacheKey) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.cache[key]; exists {
		c.removeElement(elem)
	}
}

// Clear removes all entries from the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[CacheKey]*list.Element, c.capacity)
	c.lru.Init()
	c.hits = 0
	c.misses = 0
}

// Size returns the current number of items in the cache
func (c *LRUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lru.Len()
}

// Stats returns cache statistics
func (c *LRUCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Hits:    c.hits,
		Misses:  c.misses,
		Size:    c.lru.Len(),
		HitRate: hitRate,
	}
}

// evictOldest removes the least recently used item
func (c *LRUCache) evictOldest() {
	elem := c.lru.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// removeElement removes an element from the cache
func (c *LRUCache) removeElement(elem *list.Element) {
	c.lru.Remove(elem)
	entry := elem.Value.(*cacheEntry)
	delete(c.cache, entry.key)
}

// CacheStats holds cache performance statistics
type CacheStats struct {
	Hits    int64
	Misses  int64
	Size    int
	HitRate float64
}

// QueryCache wraps an LRU cache specifically for search query results
type QueryCache struct {
	cache *LRUCache
}

// NewQueryCache creates a new query result cache
func NewQueryCache(capacity int, ttl time.Duration) *QueryCache {
	return &QueryCache{
		cache: NewLRUCache(capacity, ttl),
	}
}

// GenerateVectorQueryKey creates a cache key for vector search queries
func GenerateVectorQueryKey(queryVector []float32, k int, efSearch int) CacheKey {
	h := sha256.New()

	// Hash the vector
	for _, v := range queryVector {
		bits := math.Float32bits(v)
		binary.Write(h, binary.LittleEndian, bits)
	}

	// Hash the parameters
	binary.Write(h, binary.LittleEndian, int32(k))
	binary.Write(h, binary.LittleEndian, int32(efSearch))

	return CacheKey(fmt.Sprintf("vec:%x", h.Sum(nil)[:16]))
}

// GenerateTextQueryKey creates a cache key for text search queries
func GenerateTextQueryKey(queryText string, k int) CacheKey {
	h := sha256.New()
	h.Write([]byte(queryText))
	binary.Write(h, binary.LittleEndian, int32(k))

	return CacheKey(fmt.Sprintf("text:%x", h.Sum(nil)[:16]))
}

// GenerateHybridQueryKey creates a cache key for hybrid search queries
func GenerateHybridQueryKey(queryVector []float32, queryText string, k int, efSearch int) CacheKey {
	h := sha256.New()

	// Hash the vector
	for _, v := range queryVector {
		bits := math.Float32bits(v)
		binary.Write(h, binary.LittleEndian, bits)
	}

	// Hash the text
	h.Write([]byte(queryText))

	// Hash the parameters
	binary.Write(h, binary.LittleEndian, int32(k))
	binary.Write(h, binary.LittleEndian, int32(efSearch))

	return CacheKey(fmt.Sprintf("hybrid:%x", h.Sum(nil)[:16]))
}

// GetHybridResults retrieves cached hybrid search results
func (qc *QueryCache) GetHybridResults(key CacheKey) ([]*HybridSearchResult, bool) {
	value, found := qc.cache.Get(key)
	if !found {
		return nil, false
	}

	results, ok := value.([]*HybridSearchResult)
	if !ok {
		// Invalid cache entry, remove it
		qc.cache.Invalidate(key)
		return nil, false
	}

	return results, true
}

// PutHybridResults stores hybrid search results in the cache
func (qc *QueryCache) PutHybridResults(key CacheKey, results []*HybridSearchResult) {
	qc.cache.Put(key, results)
}

// GetTextResults retrieves cached text search results
func (qc *QueryCache) GetTextResults(key CacheKey) ([]*FullTextResult, bool) {
	value, found := qc.cache.Get(key)
	if !found {
		return nil, false
	}

	results, ok := value.([]*FullTextResult)
	if !ok {
		qc.cache.Invalidate(key)
		return nil, false
	}

	return results, true
}

// PutTextResults stores text search results in the cache
func (qc *QueryCache) PutTextResults(key CacheKey, results []*FullTextResult) {
	qc.cache.Put(key, results)
}

// Clear removes all cached results
func (qc *QueryCache) Clear() {
	qc.cache.Clear()
}

// Stats returns cache statistics
func (qc *QueryCache) Stats() CacheStats {
	return qc.cache.Stats()
}

// InvalidateAll removes all cached results (alias for Clear)
func (qc *QueryCache) InvalidateAll() {
	qc.Clear()
}

// Size returns the number of cached entries
func (qc *QueryCache) Size() int {
	return qc.cache.Size()
}

// CachedHybridSearch wraps HybridSearch with caching
type CachedHybridSearch struct {
	*HybridSearch
	cache *QueryCache
}

// NewCachedHybridSearch creates a hybrid search with query caching
func NewCachedHybridSearch(vectorIndex *hnsw.Index, textIndex *FullTextIndex, cacheCapacity int, cacheTTL time.Duration) *CachedHybridSearch {
	return &CachedHybridSearch{
		HybridSearch: NewHybridSearch(vectorIndex, textIndex),
		cache:        NewQueryCache(cacheCapacity, cacheTTL),
	}
}

// Search performs cached hybrid search
func (chs *CachedHybridSearch) Search(queryVector []float32, queryText string, k int, efSearch int) []*HybridSearchResult {
	// Generate cache key
	key := GenerateHybridQueryKey(queryVector, queryText, k, efSearch)

	// Check cache
	if results, found := chs.cache.GetHybridResults(key); found {
		return results
	}

	// Cache miss - perform search
	results := chs.HybridSearch.Search(queryVector, queryText, k, efSearch)

	// Store in cache
	chs.cache.PutHybridResults(key, results)

	return results
}

// InvalidateCache clears the query cache
func (chs *CachedHybridSearch) InvalidateCache() {
	chs.cache.Clear()
}

// CacheStats returns cache performance statistics
func (chs *CachedHybridSearch) CacheStats() CacheStats {
	return chs.cache.Stats()
}
