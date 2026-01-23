package server

import (
	"sync"
	"time"
)

// ProbeCache provides in-memory caching for model endpoint capability probe results
type ProbeCache struct {
	mu    sync.RWMutex
	cache map[string]*CachedModelCapability
	ttl   time.Duration
}

// CachedModelCapability represents a cached model endpoint capability with expiration
type CachedModelCapability struct {
	Capability ModelEndpointCapability
	ExpiresAt  time.Time
}

// ModelEndpointCapability represents the endpoint capability information for a model
type ModelEndpointCapability struct {
	ProviderUUID       string
	ModelID            string
	SupportsChat       bool
	ChatLatencyMs      int
	ChatError          string
	SupportsResponses  bool
	ResponsesLatencyMs int
	ResponsesError     string
	PreferredEndpoint  string // "chat", "responses", or ""
	LastVerified       time.Time
}

// EndpointStatus represents the status of a single endpoint
type EndpointStatus struct {
	Available    bool
	LatencyMs    int
	ErrorMessage string
	LastChecked  time.Time
}

// ProbeResult represents the complete probe result for a model
type ProbeResult struct {
	ProviderUUID      string
	ModelID           string
	ChatEndpoint      EndpointStatus
	ResponsesEndpoint EndpointStatus
	PreferredEndpoint string
	LastUpdated       time.Time
}

// ProbeCacheRequest represents a request to probe a model
type ProbeCacheRequest struct {
	ProviderUUID string
	ModelID      string
	ForceRefresh bool // Force new probe even if cached
}

// NewProbeCache creates a new probe cache with the specified TTL
func NewProbeCache(ttl time.Duration) *ProbeCache {
	return &ProbeCache{
		cache: make(map[string]*CachedModelCapability),
		ttl:   ttl,
	}
}

// Get retrieves cached capability for a model
func (pc *ProbeCache) Get(providerUUID, modelID string) *ModelEndpointCapability {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	key := pc.makeKey(providerUUID, modelID)
	cached, found := pc.cache[key]
	if !found {
		return nil
	}

	// Check if expired
	if time.Now().After(cached.ExpiresAt) {
		// Expired - return nil (caller should trigger refresh)
		return nil
	}

	return &cached.Capability
}

// Set stores capability in cache
func (pc *ProbeCache) Set(providerUUID, modelID string, capability *ModelEndpointCapability) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	key := pc.makeKey(providerUUID, modelID)
	pc.cache[key] = &CachedModelCapability{
		Capability: *capability,
		ExpiresAt:  time.Now().Add(pc.ttl),
	}
}

// SetFromProbeResult stores probe result in cache
func (pc *ProbeCache) SetFromProbeResult(result *ProbeResult) {
	capability := &ModelEndpointCapability{
		ProviderUUID:       result.ProviderUUID,
		ModelID:            result.ModelID,
		SupportsChat:       result.ChatEndpoint.Available,
		ChatLatencyMs:      result.ChatEndpoint.LatencyMs,
		ChatError:          result.ChatEndpoint.ErrorMessage,
		SupportsResponses:  result.ResponsesEndpoint.Available,
		ResponsesLatencyMs: result.ResponsesEndpoint.LatencyMs,
		ResponsesError:     result.ResponsesEndpoint.ErrorMessage,
		PreferredEndpoint:  result.PreferredEndpoint,
		LastVerified:       result.LastUpdated,
	}
	pc.Set(result.ProviderUUID, result.ModelID, capability)
}

// Invalidate removes cached capability for a specific model
func (pc *ProbeCache) Invalidate(providerUUID, modelID string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	key := pc.makeKey(providerUUID, modelID)
	delete(pc.cache, key)
}

// InvalidateProvider removes all cached capabilities for a provider
func (pc *ProbeCache) InvalidateProvider(providerUUID string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Remove all entries for this provider
	prefix := providerUUID + "/"
	for key := range pc.cache {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(pc.cache, key)
		}
	}
}

// Clear removes all cached entries
func (pc *ProbeCache) Clear() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.cache = make(map[string]*CachedModelCapability)
}

// CleanupExpired removes expired entries from cache
func (pc *ProbeCache) CleanupExpired() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	now := time.Now()
	for key, cached := range pc.cache {
		if now.After(cached.ExpiresAt) {
			delete(pc.cache, key)
		}
	}
}

// makeKey creates a cache key from provider UUID and model ID
func (pc *ProbeCache) makeKey(providerUUID, modelID string) string {
	return providerUUID + "/" + modelID
}

// StartCleanupTask starts a background task to periodically clean up expired entries
func (pc *ProbeCache) StartCleanupTask(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			pc.CleanupExpired()
		}
	}()
}
