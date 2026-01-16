package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"

	"tingly-box/internal/llmclient"
	"tingly-box/internal/typ"
)

// ClientPool manages unified client instances for different providers
type ClientPool struct {
	openaiClients    map[string]*llmclient.OpenAIClient
	anthropicClients map[string]*llmclient.AnthropicClient
	googleClients    map[string]*llmclient.GoogleClient
	mutex            sync.RWMutex
	debugMode        bool
}

// NewClientPool creates a new client pool
func NewClientPool() *ClientPool {
	return &ClientPool{
		openaiClients:    make(map[string]*llmclient.OpenAIClient),
		anthropicClients: make(map[string]*llmclient.AnthropicClient),
		googleClients:    make(map[string]*llmclient.GoogleClient),
	}
}

// GetOpenAIClient returns an OpenAI client wrapper for the specified provider
func (p *ClientPool) GetOpenAIClient(provider *typ.Provider, model string) *llmclient.OpenAIClient {
	// Generate unique key for provider
	key := p.generateProviderKey(provider, model)

	// Try to get existing client with read lock first
	p.mutex.RLock()
	if client, exists := p.openaiClients[key]; exists {
		p.mutex.RUnlock()
		logrus.Debugf("Using cached OpenAI client for provider: %s", provider.Name)
		return client
	}
	p.mutex.RUnlock()

	// Need to create new client, acquire write lock
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Double-check after acquiring write lock to avoid race conditions
	if client, exists := p.openaiClients[key]; exists {
		logrus.Debugf("Using cached OpenAI client for provider: %s (double-check)", provider.Name)
		return client
	}

	// Create new client using factory
	logrus.Infof("Creating new OpenAI client for provider: %s (API: %s)", provider.Name, provider.APIBase)

	client, err := llmclient.NewOpenAIClient(provider)
	if err != nil {
		logrus.Errorf("Failed to create OpenAI client for provider %s: %v", provider.Name, err)
		return nil
	}

	// Apply debug mode if enabled
	if p.debugMode {
		client.SetMode(true)
	}

	// Store in pool
	p.openaiClients[key] = client
	return client
}

// GetAnthropicClient returns an Anthropic client wrapper for the specified provider
func (p *ClientPool) GetAnthropicClient(provider *typ.Provider, model string) *llmclient.AnthropicClient {
	// Generate unique key for provider
	key := p.generateProviderKey(provider, model)

	// Try to get existing client with read lock first
	p.mutex.RLock()
	if client, exists := p.anthropicClients[key]; exists {
		p.mutex.RUnlock()
		logrus.Debugf("Using cached Anthropic client for provider: %s model: %s", provider.Name, model)
		return client
	}
	p.mutex.RUnlock()

	// Need to create new client, acquire write lock
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Double-check after acquiring write lock to avoid race conditions
	if client, exists := p.anthropicClients[key]; exists {
		logrus.Debugf("Using cached Anthropic client for provider: %s (double-check)", provider.Name)
		return client
	}

	// Create new client using factory
	logrus.Infof("Creating new Anthropic client for provider: %s (API: %s)", provider.Name, provider.APIBase)

	client, err := llmclient.NewAnthropicClient(provider)
	if err != nil {
		logrus.Errorf("Failed to create Anthropic client for provider %s: %v", provider.Name, err)
		return nil
	}

	// Apply debug mode if enabled
	if p.debugMode {
		client.SetMode(true)
	}

	// Store in pool
	p.anthropicClients[key] = client
	return client
}

// GetGoogleClient returns a Google client wrapper for the specified provider
func (p *ClientPool) GetGoogleClient(provider *typ.Provider, model string) *llmclient.GoogleClient {
	// Generate unique key for provider
	key := p.generateProviderKey(provider, model)

	// Try to get existing client with read lock first
	p.mutex.RLock()
	if client, exists := p.googleClients[key]; exists {
		p.mutex.RUnlock()
		logrus.Debugf("Using cached Google client for provider: %s model: %s", provider.Name, model)
		return client
	}
	p.mutex.RUnlock()

	// Need to create new client, acquire write lock
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Double-check after acquiring write lock to avoid race conditions
	if client, exists := p.googleClients[key]; exists {
		logrus.Debugf("Using cached Google client for provider: %s (double-check)", provider.Name)
		return client
	}

	// Create new client using factory
	logrus.Infof("Creating new Google client for provider: %s (API: %s)", provider.Name, provider.APIBase)

	client, err := llmclient.NewGoogleClient(provider)
	if err != nil {
		logrus.Errorf("Failed to create Google client for provider %s: %v", provider.Name, err)
		return nil
	}

	// Apply debug mode if enabled
	if p.debugMode {
		client.SetMode(true)
	}

	// Store in pool
	p.googleClients[key] = client
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

	p.openaiClients = make(map[string]*llmclient.OpenAIClient)
	p.anthropicClients = make(map[string]*llmclient.AnthropicClient)
	p.googleClients = make(map[string]*llmclient.GoogleClient)
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
		"debug_mode":              p.debugMode,
	}
}

// SetMode sets the debug mode for the pool and all existing clients.
// When debug is true, all API headers and bodies are logged as indented JSON.
func (p *ClientPool) SetMode(debug bool) {
	p.mutex.Lock()
	p.debugMode = debug
	p.mutex.Unlock()

	if debug {
		logrus.Info("Enabling debug mode for client pool - all API headers and bodies will be logged")
	} else {
		logrus.Info("Disabling debug mode for client pool")
	}

	// Apply mode to all existing OpenAI clients
	p.mutex.Lock()
	for _, client := range p.openaiClients {
		client.SetMode(debug)
	}
	for _, client := range p.anthropicClients {
		client.SetMode(debug)
	}
	for _, client := range p.googleClients {
		client.SetMode(debug)
	}
	p.mutex.Unlock()
}
