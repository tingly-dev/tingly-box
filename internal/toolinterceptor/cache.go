package toolinterceptor

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

const (
	// Default TTL values for cache entries
	defaultSearchCacheTTL = 1 * time.Hour
	defaultFetchCacheTTL  = 24 * time.Hour

	// Maximum number of cache entries
	maxCacheSize = 1000
)

// Cache implements an in-memory cache for search and fetch results
type Cache struct {
	mu    sync.RWMutex
	store map[string]*CacheEntry

	// Track access for LRU eviction
	accessOrder []string
	maxSize     int
}

// NewCache creates a new cache instance
func NewCache() *Cache {
	return &Cache{
		store:       make(map[string]*CacheEntry),
		accessOrder: make([]string, 0, maxCacheSize),
		maxSize:     maxCacheSize,
	}
}

// Get retrieves a cached entry if it exists and hasn't expired
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.store[key]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	// Update access order (simple LRU)
	c.updateAccessOrder(key)

	return entry.Result, true
}

// Set stores a value in the cache with expiration
func (c *Cache) Set(key string, result interface{}, contentType string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Determine TTL based on content type
	var ttl time.Duration
	if contentType == "search" {
		ttl = defaultSearchCacheTTL
	} else {
		ttl = defaultFetchCacheTTL
	}

	// Create cache entry
	entry := &CacheEntry{
		Result:      result,
		ExpiresAt:   time.Now().Add(ttl),
		ContentType: contentType,
	}

	// Check if we need to evict (simple LRU when at capacity)
	if len(c.store) >= c.maxSize {
		c.evictLRU()
	}

	c.store[key] = entry
	c.updateAccessOrder(key)
}

// SearchCacheKey generates a cache key for search queries
func SearchCacheKey(query string) string {
	h := sha256.New()
	h.Write([]byte("search:" + query))
	return hex.EncodeToString(h.Sum(nil))
}

// FetchCacheKey generates a cache key for fetch URLs
func FetchCacheKey(url string) string {
	h := sha256.New()
	h.Write([]byte("fetch:" + url))
	return hex.EncodeToString(h.Sum(nil))
}

// Clear removes all entries from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.store = make(map[string]*CacheEntry)
	c.accessOrder = make([]string, 0, c.maxSize)
}

// Size returns the current number of cache entries
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.store)
}

// updateAccessOrder updates the access order for LRU tracking
func (c *Cache) updateAccessOrder(key string) {
	// Remove key from existing position if present
	for i, k := range c.accessOrder {
		if k == key {
			c.accessOrder = append(c.accessOrder[:i], c.accessOrder[i+1:]...)
			break
		}
	}
	// Add to end (most recently used)
	c.accessOrder = append(c.accessOrder, key)
}

// evictLRU removes the least recently used entry
func (c *Cache) evictLRU() {
	if len(c.accessOrder) == 0 {
		return
	}

	// Remove first entry (least recently used)
	lruKey := c.accessOrder[0]
	delete(c.store, lruKey)
	c.accessOrder = c.accessOrder[1:]
}
