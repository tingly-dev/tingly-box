package client

import (
	"net/http"
	"sync"

	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// TestTransportPool is a test double for TransportPool that tracks
// which transport keys were requested.
type TestTransportPool struct {
	transports map[string]*http.Transport
	keys       []string
	mutex      sync.Mutex
}

// NewTestTransportPool creates a new test transport pool.
func NewTestTransportPool() *TestTransportPool {
	return &TestTransportPool{
		transports: make(map[string]*http.Transport),
		keys:       make([]string, 0),
	}
}

// GetTransport returns or creates a transport for the given configuration.
// It tracks the key for test verification.
func (p *TestTransportPool) GetTransport(providerUUID, model, proxyURL string, oauthType oauth.ProviderType, sessionID typ.SessionID) *http.Transport {
	key := NewTransportKey(providerUUID, proxyURL, oauthType, sessionID).String()

	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Track the key for test verification
	p.keys = append(p.keys, key)

	// Return existing transport if available
	if transport, exists := p.transports[key]; exists {
		return transport
	}

	// Create new transport
	transport := &http.Transport{}
	p.transports[key] = transport
	return transport
}

// Keys returns all transport keys that were requested.
func (p *TestTransportPool) Keys() []string {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Return a copy to prevent race conditions
	keys := make([]string, len(p.keys))
	copy(keys, p.keys)
	return keys
}

// Clear clears all transports and keys.
func (p *TestTransportPool) Clear() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.transports = make(map[string]*http.Transport)
	p.keys = make([]string, 0)
}

// Stats returns statistics about the transport pool.
func (p *TestTransportPool) Stats() map[string]interface{} {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return map[string]interface{}{
		"transport_count": len(p.transports),
	}
}

// InvalidateSession is a no-op for tests (unless specifically needed).
func (p *TestTransportPool) InvalidateSession(providerUUID, sessionID string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Remove transports matching the session
	// This is a simplified implementation for tests
	newTransports := make(map[string]*http.Transport)
	newKeys := make([]string, 0)

	sessionToken := `"` + sessionID + `"`
	for key, transport := range p.transports {
		// Keep transports that don't match the session
		if key == "" || !contains(key, sessionToken) {
			newTransports[key] = transport
		}
	}

	for _, key := range p.keys {
		if key == "" || !contains(key, sessionToken) {
			newKeys = append(newKeys, key)
		}
	}

	p.transports = newTransports
	p.keys = newKeys
}

// contains is a simple string contains helper.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
