package toolruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

const (
	defaultSearchCacheTTL = 1 * time.Hour
	defaultFetchCacheTTL  = 24 * time.Hour
	maxBuiltinCacheSize   = 1000
)

type builtinCacheEntry struct {
	Result      interface{}
	ExpiresAt   time.Time
	ContentType string
}

type builtinCache struct {
	mu          sync.RWMutex
	store       map[string]*builtinCacheEntry
	accessOrder []string
	maxSize     int
}

func newBuiltinCache() *builtinCache {
	return &builtinCache{
		store:       make(map[string]*builtinCacheEntry),
		accessOrder: make([]string, 0, maxBuiltinCacheSize),
		maxSize:     maxBuiltinCacheSize,
	}
}

func (c *builtinCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, exists := c.store[key]
	if !exists || time.Now().After(entry.ExpiresAt) {
		return nil, false
	}
	c.updateAccessOrder(key)
	return entry.Result, true
}

func (c *builtinCache) Set(key string, result interface{}, contentType string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ttl := defaultFetchCacheTTL
	if contentType == "search" {
		ttl = defaultSearchCacheTTL
	}
	if len(c.store) >= c.maxSize {
		c.evictLRU()
	}
	c.store[key] = &builtinCacheEntry{
		Result:      result,
		ExpiresAt:   time.Now().Add(ttl),
		ContentType: contentType,
	}
	c.updateAccessOrder(key)
}

func (c *builtinCache) updateAccessOrder(key string) {
	for i, k := range c.accessOrder {
		if k == key {
			c.accessOrder = append(c.accessOrder[:i], c.accessOrder[i+1:]...)
			break
		}
	}
	c.accessOrder = append(c.accessOrder, key)
}

func (c *builtinCache) evictLRU() {
	if len(c.accessOrder) == 0 {
		return
	}
	lruKey := c.accessOrder[0]
	delete(c.store, lruKey)
	c.accessOrder = c.accessOrder[1:]
}

func searchCacheKey(query string) string {
	h := sha256.New()
	h.Write([]byte("search:" + query))
	return hex.EncodeToString(h.Sum(nil))
}

func fetchCacheKey(targetURL string) string {
	h := sha256.New()
	h.Write([]byte("fetch:" + targetURL))
	return hex.EncodeToString(h.Sum(nil))
}
