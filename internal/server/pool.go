package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"tingly-box/internal/config"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"
)

// ClientPool manages OpenAI and Anthropic client instances for different providers
type ClientPool struct {
	openaiClients    map[string]*openai.Client
	anthropicClients map[string]anthropic.Client
	mutex            sync.RWMutex
}

// NewClientPool creates a new client pool
func NewClientPool() *ClientPool {
	return &ClientPool{
		openaiClients:    make(map[string]*openai.Client),
		anthropicClients: make(map[string]anthropic.Client),
	}
}

// createHTTPClientWithProxy creates an HTTP client with proxy support
func createHTTPClientWithProxy(proxyURL string) *http.Client {
	if proxyURL == "" {
		return http.DefaultClient
	}

	// Parse the proxy URL
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		logrus.Errorf("Failed to parse proxy URL %s: %v, using default client", proxyURL, err)
		return http.DefaultClient
	}

	// Create transport with proxy
	transport := &http.Transport{}

	switch parsedURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsedURL)
	case "socks5":
		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, nil, proxy.Direct)
		if err != nil {
			logrus.Errorf("Failed to create SOCKS5 proxy dialer: %v, using default client", err)
			return http.DefaultClient
		}
		dialContext, ok := dialer.(proxy.ContextDialer)
		if ok {
			transport.DialContext = dialContext.DialContext
		} else {
			return http.DefaultClient
		}
	default:
		logrus.Errorf("Unsupported proxy scheme %s, supported schemes are http, https, socks5", parsedURL.Scheme)
		return http.DefaultClient
	}

	return &http.Client{
		Transport: transport,
	}
}

// GetOpenAIClient returns an OpenAI client for the specified provider
// It creates a new client if one doesn't exist for the provider
func (p *ClientPool) GetOpenAIClient(provider *config.Provider) *openai.Client {
	// Generate unique key for provider
	key := p.generateProviderKey(provider)

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

	// Create new client with proxy support if configured
	logrus.Infof("Creating new OpenAI client for provider: %s (API: %s)", provider.Name, provider.APIBase)

	options := []openaiOption.RequestOption{
		openaiOption.WithAPIKey(provider.Token),
		openaiOption.WithBaseURL(provider.APIBase),
	}

	// Add proxy if configured
	if provider.ProxyURL != "" {
		httpClient := createHTTPClientWithProxy(provider.ProxyURL)
		options = append(options, openaiOption.WithHTTPClient(httpClient))
		logrus.Infof("Using proxy for OpenAI client: %s", provider.ProxyURL)
	}

	client := openai.NewClient(options...)

	// Store in pool
	p.openaiClients[key] = &client
	return &client
}

// GetAnthropicClient returns an Anthropic client for the specified provider
// It creates a new client if one doesn't exist for the provider
func (p *ClientPool) GetAnthropicClient(provider *config.Provider) anthropic.Client {
	// Generate unique key for provider
	key := p.generateProviderKey(provider)

	// Try to get existing client with read lock first
	p.mutex.RLock()
	if client, exists := p.anthropicClients[key]; exists {
		p.mutex.RUnlock()
		logrus.Debugf("Using cached Anthropic client for provider: %s", provider.Name)
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

	// Create new client with proxy support if configured
	var apiBase = provider.APIBase
	if strings.HasSuffix(apiBase, "/v1") {
		apiBase = apiBase[:len(apiBase)-3]
	}

	logrus.Infof("Creating new Anthropic client for provider: %s (API: %s)", provider.Name, apiBase)

	options := []anthropicOption.RequestOption{
		anthropicOption.WithAPIKey(provider.Token),
		anthropicOption.WithBaseURL(apiBase),
	}

	// Add proxy if configured
	if provider.ProxyURL != "" {
		httpClient := createHTTPClientWithProxy(provider.ProxyURL)
		options = append(options, anthropicOption.WithHTTPClient(httpClient))
		logrus.Infof("Using proxy for Anthropic client: %s", provider.ProxyURL)
	}

	client := anthropic.NewClient(options...)

	// Store in pool
	p.anthropicClients[key] = client
	return client
}

// generateProviderKey creates a unique key for a provider
// Uses combination of name, API base, hash of the token, and proxy URL for uniqueness
func (p *ClientPool) generateProviderKey(provider *config.Provider) string {
	return fmt.Sprintf("%s:%s:%s:%s", provider.Name, provider.APIBase, hashToken(provider.Token), hashToken(provider.ProxyURL))
}

// hashToken creates a secure hash of the token for key generation
// This ensures different tokens for the same provider get different clients
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
// Useful for cleanup or when all providers change
func (p *ClientPool) Clear() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.openaiClients = make(map[string]*openai.Client)
	p.anthropicClients = make(map[string]anthropic.Client)
	logrus.Info("Client pools cleared")
}

// RemoveProvider removes a specific provider's client from the pool
func (p *ClientPool) RemoveProvider(provider *config.Provider) {
	key := p.generateProviderKey(provider)

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

	if removed {
		logrus.Infof("Removed clients for provider: %s", provider.Name)
	}
}

// Size returns the total number of clients currently in both pools
func (p *ClientPool) Size() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return len(p.openaiClients) + len(p.anthropicClients)
}

// GetProviderKeys returns all provider keys currently in the pool
// Useful for debugging and monitoring
func (p *ClientPool) GetProviderKeys() []string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	keys := make([]string, 0, len(p.openaiClients)+len(p.anthropicClients))

	// Add OpenAI client keys
	for key := range p.openaiClients {
		keys = append(keys, "openai:"+key)
	}

	// Add Anthropic client keys
	for key := range p.anthropicClients {
		keys = append(keys, "anthropic:"+key)
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
		"total_clients":           len(p.openaiClients) + len(p.anthropicClients),
		"provider_keys":           p.GetProviderKeys(),
	}
}
