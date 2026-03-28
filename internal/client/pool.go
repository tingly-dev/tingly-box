package client

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// PoolMode defines the client lifecycle management strategy
type PoolMode string

const (
	// PoolModeOnce creates a new client for each request (no caching)
	PoolModeOnce PoolMode = "once"

	// PoolModeShared caches clients and reuses them with TTL management
	PoolModeShared PoolMode = "shared"
)

// pooledClient wraps a client with its last access timestamp for TTL tracking
type pooledClient struct {
	client     interface{} // *OpenAIClient, *AnthropicClient, or *GoogleClient
	lastAccess time.Time
}

// Constants for client TTL and cleanup interval
const (
	DefaultClientTTL       = 60 * time.Minute // Default time-to-live for cached clients
	DefaultCleanupInterval = 60 * time.Minute // Default interval for cleanup task
)

// ClientPool manages unified client instances for different providers
type ClientPool struct {
	mode PoolMode

	// Shared mode fields (nil in once mode)
	openaiClients    map[string]*pooledClient
	anthropicClients map[string]*pooledClient
	googleClients    map[string]*pooledClient
	mutex            sync.RWMutex
	clientTTL        time.Duration

	// Common fields
	recordSink *obs.Sink
}

// ClientPoolBuilder builds a ClientPool with specified configuration
type ClientPoolBuilder struct {
	mode            PoolMode
	clientTTL       time.Duration
	cleanupInterval time.Duration
	recordSink      *obs.Sink
}

// NewClientPoolBuilder creates a new builder with default settings
func NewClientPoolBuilder() *ClientPoolBuilder {
	return &ClientPoolBuilder{
		mode:            PoolModeOnce, // Default to once mode
		clientTTL:       DefaultClientTTL,
		cleanupInterval: DefaultCleanupInterval,
	}
}

// WithMode sets the pool mode
func (b *ClientPoolBuilder) WithMode(mode PoolMode) *ClientPoolBuilder {
	b.mode = mode
	return b
}

// WithSharedMode enables shared mode with caching (convenience method)
func (b *ClientPoolBuilder) WithSharedMode() *ClientPoolBuilder {
	b.mode = PoolModeShared
	return b
}

// WithOnceMode enables once mode (no caching, convenience method)
func (b *ClientPoolBuilder) WithOnceMode() *ClientPoolBuilder {
	b.mode = PoolModeOnce
	return b
}

// WithClientTTL sets the TTL for cached clients (shared mode only)
func (b *ClientPoolBuilder) WithClientTTL(ttl time.Duration) *ClientPoolBuilder {
	b.clientTTL = ttl
	return b
}

// WithCleanupInterval sets the cleanup interval (shared mode only)
func (b *ClientPoolBuilder) WithCleanupInterval(interval time.Duration) *ClientPoolBuilder {
	b.cleanupInterval = interval
	return b
}

// WithRecordSink sets the record sink for all clients
func (b *ClientPoolBuilder) WithRecordSink(sink *obs.Sink) *ClientPoolBuilder {
	b.recordSink = sink
	return b
}

// Build creates the ClientPool with configured settings
func (b *ClientPoolBuilder) Build() *ClientPool {
	pool := &ClientPool{
		mode:       b.mode,
		recordSink: b.recordSink,
	}

	if b.mode == PoolModeShared {
		pool.openaiClients = make(map[string]*pooledClient)
		pool.anthropicClients = make(map[string]*pooledClient)
		pool.googleClients = make(map[string]*pooledClient)
		pool.clientTTL = b.clientTTL
		pool.StartCleanupTask(b.cleanupInterval)
	}

	return pool
}

// NewClientPool creates a pool with once mode (new default)
func NewClientPool() *ClientPool {
	return NewClientPoolBuilder().
		WithOnceMode().
		Build()
}

// NewSharedClientPool creates a pool with shared mode (explicit)
func NewSharedClientPool() *ClientPool {
	return NewClientPoolBuilder().
		WithSharedMode().
		Build()
}

// GetOpenAIClient returns an OpenAI client wrapper for the specified provider
func (p *ClientPool) GetOpenAIClient(provider *typ.Provider, model string) *OpenAIClient {
	switch p.mode {
	case PoolModeOnce:
		return p.createOnceOpenAIClient(provider, model)
	case PoolModeShared:
		return p.getOrCreateOpenAIClient(provider, model)
	default:
		logrus.Warnf("Unknown pool mode: %s, falling back to once mode", p.mode)
		return p.createOnceOpenAIClient(provider, "")
	}
}

// createOnceOpenAIClient creates a fresh client without caching
func (p *ClientPool) createOnceOpenAIClient(provider *typ.Provider, model string) *OpenAIClient {
	logrus.Debugf("Creating once-mode OpenAI client for provider: %s", provider.Name)

	client, err := NewOpenAIClient(provider, model)
	if err != nil {
		logrus.Errorf("Failed to create OpenAI client for provider %s: %v", provider.Name, err)
		return nil
	}

	if p.recordSink != nil && p.recordSink.IsEnabled() {
		client.SetRecordSink(p.recordSink)
	}

	// Set finalizer for automatic cleanup when GC collects the client
	// This avoids requiring explicit Close() calls in once mode
	runtime.SetFinalizer(client, func(c *OpenAIClient) {
		if c != nil {
			c.Close()
			logrus.Debugf("Auto-closed once-mode OpenAI client for provider via finalizer")
		}
	})

	return client
}

// getOrCreateOpenAIClient gets cached client or creates new one (shared mode)
func (p *ClientPool) getOrCreateOpenAIClient(provider *typ.Provider, model string) *OpenAIClient {
	// Generate unique key for provider
	key := p.generateProviderKey(provider, model)

	// Try to get existing client with read lock first
	p.mutex.RLock()
	if pooled, exists := p.openaiClients[key]; exists {
		p.mutex.RUnlock()
		// Update last access time
		p.mutex.Lock()
		pooled.lastAccess = time.Now()
		p.mutex.Unlock()
		logrus.Debugf("Using cached OpenAI client for provider: %s", provider.Name)
		return pooled.client.(*OpenAIClient)
	}
	p.mutex.RUnlock()

	// Need to create new client, acquire write lock
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Double-check after acquiring write lock to avoid race conditions
	if pooled, exists := p.openaiClients[key]; exists {
		pooled.lastAccess = time.Now()
		logrus.Debugf("Using cached OpenAI client for provider: %s (double-check)", provider.Name)
		return pooled.client.(*OpenAIClient)
	}

	// Create new client using factory
	logrus.Infof("Creating new OpenAI client for provider: %s (API: %s)", provider.Name, provider.APIBase)

	client, err := NewOpenAIClient(provider, model)
	if err != nil {
		logrus.Errorf("Failed to create OpenAI client for provider %s: %v", provider.Name, err)
		return nil
	}

	// Apply record sink if enabled
	if p.recordSink != nil && p.recordSink.IsEnabled() {
		client.SetRecordSink(p.recordSink)
	}

	// Store in pool with timestamp
	p.openaiClients[key] = &pooledClient{
		client:     client,
		lastAccess: time.Now(),
	}
	return client
}

// GetAnthropicClient returns an Anthropic client wrapper for the specified provider
func (p *ClientPool) GetAnthropicClient(provider *typ.Provider, model string) *AnthropicClient {
	switch p.mode {
	case PoolModeOnce:
		return p.createOnceAnthropicClient(provider, model)
	case PoolModeShared:
		return p.getOrCreateAnthropicClient(provider, model)
	default:
		logrus.Warnf("Unknown pool mode: %s, falling back to once mode", p.mode)
		return p.createOnceAnthropicClient(provider, "")
	}
}

// createOnceAnthropicClient creates a fresh client without caching
func (p *ClientPool) createOnceAnthropicClient(provider *typ.Provider, model string) *AnthropicClient {
	logrus.Debugf("Creating once-mode Anthropic client for provider: %s", provider.Name)

	client, err := NewAnthropicClient(provider, model)
	if err != nil {
		logrus.Errorf("Failed to create Anthropic client for provider %s: %v", provider.Name, err)
		return nil
	}

	if p.recordSink != nil && p.recordSink.IsEnabled() {
		client.SetRecordSink(p.recordSink)
	}

	// Set finalizer for automatic cleanup when GC collects the client
	runtime.SetFinalizer(client, func(c *AnthropicClient) {
		if c != nil {
			c.Close()
			logrus.Debugf("Auto-closed once-mode Anthropic client for provider via finalizer")
		}
	})

	return client
}

// getOrCreateAnthropicClient gets cached client or creates new one (shared mode)
func (p *ClientPool) getOrCreateAnthropicClient(provider *typ.Provider, model string) *AnthropicClient {
	// Generate unique key for provider
	key := p.generateProviderKey(provider, model)

	// Try to get existing client with read lock first
	p.mutex.RLock()
	if pooled, exists := p.anthropicClients[key]; exists {
		p.mutex.RUnlock()
		// Update last access time
		p.mutex.Lock()
		pooled.lastAccess = time.Now()
		p.mutex.Unlock()
		logrus.Debugf("Using cached Anthropic client for provider: %s model: %s", provider.Name, model)
		return pooled.client.(*AnthropicClient)
	}
	p.mutex.RUnlock()

	// Need to create new client, acquire write lock
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Double-check after acquiring write lock to avoid race conditions
	if pooled, exists := p.anthropicClients[key]; exists {
		pooled.lastAccess = time.Now()
		logrus.Debugf("Using cached Anthropic client for provider: %s (double-check)", provider.Name)
		return pooled.client.(*AnthropicClient)
	}

	// Create new client using factory
	logrus.Infof("Creating new Anthropic client for provider: %s (API: %s) model: %s", provider.Name, provider.APIBase, model)

	client, err := NewAnthropicClient(provider, model)
	if err != nil {
		logrus.Errorf("Failed to create Anthropic client for provider %s: %v", provider.Name, err)
		return nil
	}

	// Apply record sink if enabled
	if p.recordSink != nil && p.recordSink.IsEnabled() {
		client.SetRecordSink(p.recordSink)
	}

	// Store in pool with timestamp
	p.anthropicClients[key] = &pooledClient{
		client:     client,
		lastAccess: time.Now(),
	}
	return client
}

// GetGoogleClient returns a Google client wrapper for the specified provider
func (p *ClientPool) GetGoogleClient(provider *typ.Provider, model string) *GoogleClient {
	switch p.mode {
	case PoolModeOnce:
		return p.createOnceGoogleClient(provider, model)
	case PoolModeShared:
		return p.getOrCreateGoogleClient(provider, model)
	default:
		logrus.Warnf("Unknown pool mode: %s, falling back to once mode", p.mode)
		return p.createOnceGoogleClient(provider, "")
	}
}

// createOnceGoogleClient creates a fresh client without caching
func (p *ClientPool) createOnceGoogleClient(provider *typ.Provider, model string) *GoogleClient {
	logrus.Debugf("Creating once-mode Google client for provider: %s", provider.Name)

	client, err := NewGoogleClient(provider, model)
	if err != nil {
		logrus.Errorf("Failed to create Google client for provider %s: %v", provider.Name, err)
		return nil
	}

	if p.recordSink != nil && p.recordSink.IsEnabled() {
		client.SetRecordSink(p.recordSink)
	}

	// Set finalizer for automatic cleanup when GC collects the client
	runtime.SetFinalizer(client, func(c *GoogleClient) {
		if c != nil {
			c.Close()
			logrus.Debugf("Auto-closed once-mode Google client for provider via finalizer")
		}
	})

	return client
}

// getOrCreateGoogleClient gets cached client or creates new one (shared mode)
func (p *ClientPool) getOrCreateGoogleClient(provider *typ.Provider, model string) *GoogleClient {
	// Generate unique key for provider
	key := p.generateProviderKey(provider, model)

	// Try to get existing client with read lock first
	p.mutex.RLock()
	if pooled, exists := p.googleClients[key]; exists {
		p.mutex.RUnlock()
		// Update last access time
		p.mutex.Lock()
		pooled.lastAccess = time.Now()
		p.mutex.Unlock()
		logrus.Debugf("Using cached Google client for provider: %s model: %s", provider.Name, model)
		return pooled.client.(*GoogleClient)
	}
	p.mutex.RUnlock()

	// Need to create new client, acquire write lock
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Double-check after acquiring write lock to avoid race conditions
	if pooled, exists := p.googleClients[key]; exists {
		pooled.lastAccess = time.Now()
		logrus.Debugf("Using cached Google client for provider: %s (double-check)", provider.Name)
		return pooled.client.(*GoogleClient)
	}

	// Create new client using factory
	logrus.Infof("Creating new Google client for provider: %s (API: %s)", provider.Name, provider.APIBase)

	client, err := NewGoogleClient(provider, "")
	if err != nil {
		logrus.Errorf("Failed to create Google client for provider %s: %v", provider.Name, err)
		return nil
	}

	// Apply record sink if enabled
	if p.recordSink != nil && p.recordSink.IsEnabled() {
		client.SetRecordSink(p.recordSink)
	}

	// Store in pool with timestamp
	p.googleClients[key] = &pooledClient{
		client:     client,
		lastAccess: time.Now(),
	}
	return client
}

// generateProviderKey creates a unique key for a provider
// Uses only UUID and model - token changes will be handled by explicit invalidation
func (p *ClientPool) generateProviderKey(provider *typ.Provider, model string) string {
	return fmt.Sprintf("%s:%s", provider.UUID, model)
}

// Clear removes all clients from the pool
func (p *ClientPool) Clear() {
	if p.mode == PoolModeOnce {
		logrus.Debug("Clear called on once-mode pool (no-op)")
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.openaiClients = make(map[string]*pooledClient)
	p.anthropicClients = make(map[string]*pooledClient)
	p.googleClients = make(map[string]*pooledClient)
	logrus.Info("Client pools cleared")
}

// RemoveProvider removes a specific provider's client from the pool
func (p *ClientPool) RemoveProvider(provider *typ.Provider, model string) {
	if p.mode == PoolModeOnce {
		logrus.Debug("RemoveProvider called on once-mode pool (no-op)")
		return
	}

	key := p.generateProviderKey(provider, model)

	p.mutex.Lock()
	defer p.mutex.Unlock()

	removed := false
	if _, exists := p.openaiClients[key]; exists {
		delete(p.openaiClients, key)
		removed = true
	}
	if _, exists := p.anthropicClients[key]; exists {
		delete(p.anthropicClients, key)
		removed = true
	}
	if _, exists := p.googleClients[key]; exists {
		delete(p.googleClients, key)
		removed = true
	}

	if removed {
		logrus.Infof("Removed client for provider: %s", provider.Name)
	}
}

// InvalidateProvider removes all cached clients for a specific provider UUID
// This should be called when provider credentials are updated (e.g., OAuth token refresh)
func (p *ClientPool) InvalidateProvider(providerUUID string) {
	if p.mode == PoolModeOnce {
		logrus.Debug("InvalidateProvider called on once-mode pool (no-op)")
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	count := 0

	// Remove all OpenAI clients matching this provider UUID
	for key := range p.openaiClients {
		if strings.HasPrefix(key, providerUUID+":") {
			delete(p.openaiClients, key)
			count++
		}
	}

	// Remove all Anthropic clients matching this provider UUID
	for key := range p.anthropicClients {
		if strings.HasPrefix(key, providerUUID+":") {
			delete(p.anthropicClients, key)
			count++
		}
	}

	// Remove all Google clients matching this provider UUID
	for key := range p.googleClients {
		if strings.HasPrefix(key, providerUUID+":") {
			delete(p.googleClients, key)
			count++
		}
	}

	if count > 0 {
		logrus.Infof("Invalidated %d client(s) for provider UUID: %s", count, providerUUID)
	} else {
		logrus.Debugf("No clients found to invalidate for provider UUID: %s", providerUUID)
	}
}

// Size returns the total number of clients currently in both pools
func (p *ClientPool) Size() int {
	if p.mode == PoolModeOnce {
		return 0
	}

	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return len(p.openaiClients) + len(p.anthropicClients) + len(p.googleClients)
}

// GetProviderKeys returns all provider keys currently in the pool
func (p *ClientPool) GetProviderKeys() []string {
	if p.mode == PoolModeOnce {
		return []string{}
	}

	p.mutex.RLock()
	defer p.mutex.RUnlock()

	keys := make([]string, 0, len(p.openaiClients)+len(p.anthropicClients)+len(p.googleClients))

	// Add OpenAI client keys
	for key := range p.openaiClients {
		keys = append(keys, "openai:"+key)
	}

	// Add Anthropic client keys
	for key := range p.anthropicClients {
		keys = append(keys, "anthropic:"+key)
	}

	// Add Google client keys
	for key := range p.googleClients {
		keys = append(keys, "google:"+key)
	}

	return keys
}

// Stats provides statistics about the client pool
func (p *ClientPool) Stats() map[string]interface{} {
	stats := map[string]interface{}{
		"mode": string(p.mode),
	}

	if p.mode == PoolModeOnce {
		stats["openai_clients_count"] = 0
		stats["anthropic_clients_count"] = 0
		stats["google_clients_count"] = 0
		stats["total_clients"] = 0
		stats["provider_keys"] = []string{}
		return stats
	}

	p.mutex.RLock()
	defer p.mutex.RUnlock()

	stats["openai_clients_count"] = len(p.openaiClients)
	stats["anthropic_clients_count"] = len(p.anthropicClients)
	stats["google_clients_count"] = len(p.googleClients)
	stats["total_clients"] = len(p.openaiClients) + len(p.anthropicClients) + len(p.googleClients)
	stats["provider_keys"] = p.GetProviderKeys()

	return stats
}

// SetRecordSink sets the record sink for the client pool
func (p *ClientPool) SetRecordSink(sink *obs.Sink) {
	if sink == nil {
		return
	}

	p.recordSink = sink

	// Only update existing clients in shared mode
	if p.mode == PoolModeShared {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		// Apply record sink to all existing clients
		for _, pooled := range p.openaiClients {
			pooled.client.(*OpenAIClient).SetRecordSink(sink)
		}
		for _, pooled := range p.anthropicClients {
			pooled.client.(*AnthropicClient).SetRecordSink(sink)
		}
		for _, pooled := range p.googleClients {
			pooled.client.(*GoogleClient).SetRecordSink(sink)
		}

		if sink.IsEnabled() {
			logrus.Info("Record sink enabled for client pool")
		}
	}
}

// GetRecordSink returns the record sink
func (p *ClientPool) GetRecordSink() *obs.Sink {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.recordSink
}

// cleanupExpiredClients removes clients that haven't been accessed within the TTL period
func (p *ClientPool) cleanupExpiredClients() {
	if p.mode == PoolModeOnce {
		return // No cleanup needed in once mode
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	now := time.Now()
	expirationThreshold := now.Add(-p.clientTTL)

	removedCount := 0

	// Clean up OpenAI clients
	for key, pooled := range p.openaiClients {
		if pooled.lastAccess.Before(expirationThreshold) {
			delete(p.openaiClients, key)
			removedCount++
		}
	}

	// Clean up Anthropic clients
	for key, pooled := range p.anthropicClients {
		if pooled.lastAccess.Before(expirationThreshold) {
			delete(p.anthropicClients, key)
			removedCount++
		}
	}

	// Clean up Google clients
	for key, pooled := range p.googleClients {
		if pooled.lastAccess.Before(expirationThreshold) {
			delete(p.googleClients, key)
			removedCount++
		}
	}

	if removedCount > 0 {
		logrus.Infof("Cleaned up %d expired clients from pool", removedCount)
	}
}

// StartCleanupTask starts a periodic cleanup task that removes expired clients
// The cleanup runs in a background goroutine and continues until the process exits
func (p *ClientPool) StartCleanupTask(interval time.Duration) {
	if p.mode == PoolModeOnce {
		logrus.Debug("Cleanup task not started for once-mode pool")
		return
	}

	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			p.cleanupExpiredClients()
		}
	}()
	logrus.Infof("Started client pool cleanup task with interval: %v", interval)
}
