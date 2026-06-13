package server

import (
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

const defaultEndpointCacheTTL = 24 * time.Hour

type endpointCacheEntry struct {
	target   protocol.APIType
	cachedAt time.Time
}

type EndpointCache struct {
	mu    sync.RWMutex
	store map[string]endpointCacheEntry
	ttl   time.Duration
}

func NewEndpointCache(ttl time.Duration) *EndpointCache {
	if ttl <= 0 {
		ttl = defaultEndpointCacheTTL
	}
	return &EndpointCache{
		store: make(map[string]endpointCacheEntry),
		ttl:   ttl,
	}
}

func endpointCacheKey(providerUUID, model string) string {
	return providerUUID + ":" + model
}

func (c *EndpointCache) Get(providerUUID, model string) (protocol.APIType, bool) {
	key := endpointCacheKey(providerUUID, model)
	c.mu.RLock()
	entry, ok := c.store[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Since(entry.cachedAt) > c.ttl {
		c.mu.Lock()
		delete(c.store, key)
		c.mu.Unlock()
		return "", false
	}
	return entry.target, true
}

func (c *EndpointCache) Set(providerUUID, model string, target protocol.APIType) {
	key := endpointCacheKey(providerUUID, model)
	c.mu.Lock()
	c.store[key] = endpointCacheEntry{
		target:   target,
		cachedAt: time.Now(),
	}
	c.mu.Unlock()
}
