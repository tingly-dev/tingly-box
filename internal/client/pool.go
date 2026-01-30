package client

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/typ"
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
	openaiClients    map[string]*pooledClient
	anthropicClients map[string]*pooledClient
	googleClients    map[string]*pooledClient
	mutex            sync.RWMutex
	recordSink       *obs.Sink
	clientTTL        time.Duration
}

// NewClientPool creates a new client pool
func NewClientPool() *ClientPool {
	pool := &ClientPool{
		openaiClients:    make(map[string]*pooledClient),
		anthropicClients: make(map[string]*pooledClient),
		googleClients:    make(map[string]*pooledClient),
		clientTTL:        DefaultClientTTL,
	}
	// Start cleanup task for expired clients
	pool.StartCleanupTask(DefaultCleanupInterval)
	return pool
}

// GetOpenAIClient returns an OpenAI client wrapper for the specified provider
func (p *ClientPool) GetOpenAIClient(provider *typ.Provider, model string) *OpenAIClient {
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

	client, err := NewOpenAIClient(provider)
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

	client, err := NewAnthropicClient(provider)
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

	client, err := NewGoogleClient(provider)
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
func (p *ClientPool) generateProviderKey(provider *typ.Provider, model string) string {
	return fmt.Sprintf("%s:%s:%s", provider.UUID, model, hashToken(provider.ProxyURL))
}

// hashToken creates a secure hash of the token for key generation
func hashToken(token string) string {
	if token == "" {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(token))
	// Use first 16 characters, providing sufficient entropy while maintaining reasonable length
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Clear removes all clients from the pool
func (p *ClientPool) Clear() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.openaiClients = make(map[string]*pooledClient)
	p.anthropicClients = make(map[string]*pooledClient)
	p.googleClients = make(map[string]*pooledClient)
	logrus.Info("Client pools cleared")
}

// RemoveProvider removes a specific provider's client from the pool
func (p *ClientPool) RemoveProvider(provider *typ.Provider, model string) {
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

// Size returns the total number of clients currently in both pools
func (p *ClientPool) Size() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return len(p.openaiClients) + len(p.anthropicClients) + len(p.googleClients)
}

// GetProviderKeys returns all provider keys currently in the pool
func (p *ClientPool) GetProviderKeys() []string {
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
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return map[string]interface{}{
		"openai_clients_count":    len(p.openaiClients),
		"anthropic_clients_count": len(p.anthropicClients),
		"google_clients_count":    len(p.googleClients),
		"total_clients":           len(p.openaiClients) + len(p.anthropicClients) + len(p.googleClients),
		"provider_keys":           p.GetProviderKeys(),
	}
}

// SetRecordSink sets the record sink for the client pool
func (p *ClientPool) SetRecordSink(sink *obs.Sink) {
	if sink == nil {
		return
	}
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.recordSink = sink

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

	if sink != nil && sink.IsEnabled() {
		logrus.Info("Record sink enabled for client pool")
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
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			p.cleanupExpiredClients()
		}
	}()
	logrus.Infof("Started client pool cleanup task with interval: %v", interval)
}
