package client

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"

	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// TransportPool manages shared HTTP transports for clients
// Transports are keyed by: apiBaseURL + proxyURL + oauthType
// This allows multiple clients to share the same connection pool
// when they connect to the same API endpoint through the same proxy.
type TransportPool struct {
	transports map[string]*http.Transport
	mutex      sync.RWMutex
}

// Global singleton transport pool
var globalTransportPool = &TransportPool{
	transports: make(map[string]*http.Transport),
}

// GetGlobalTransportPool returns the global transport pool singleton
func GetGlobalTransportPool() *TransportPool {
	return globalTransportPool
}

// GetTransport returns or creates a shared HTTP transport for the given configuration
// The transport key is based on: apiBaseURL + proxyURL + oauthType + tlsFingerprint
func (tp *TransportPool) GetTransport(apiBase, proxyURL string, oauthType oauth.ProviderType, tlsFingerprint TLSFingerprint) *http.Transport {
	key := tp.generateTransportKey(apiBase, proxyURL, oauthType, tlsFingerprint)

	// Try to get existing transport with read lock first
	tp.mutex.RLock()
	if transport, exists := tp.transports[key]; exists {
		tp.mutex.RUnlock()
		logrus.Debugf("Using cached transport for key: %s", key)
		return transport
	}
	tp.mutex.RUnlock()

	// Need to create new transport, acquire write lock
	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	// Double-check after acquiring write lock to avoid race conditions
	if transport, exists := tp.transports[key]; exists {
		logrus.Debugf("Using cached transport for key: %s (double-check)", key)
		return transport
	}

	// Create new transport
	logrus.Infof("Creating new transport for API: %s, Proxy: %s, OAuth: %s, TLS: %s", apiBase, proxyURL, oauthType, tlsFingerprint)
	transport := tp.createTransport(proxyURL, tlsFingerprint)
	tp.transports[key] = transport

	return transport
}

// generateTransportKey creates a unique key for transport caching
// The key is based on apiBaseURL + proxyURL + oauthType + tlsFingerprint to ensure:
// - Same API endpoint with same proxy = shared transport
// - Different API endpoints = separate transports
// - Same endpoint with different proxies = separate transports
// - Different OAuth hooks = separate transports (since hooks modify requests)
// - Different TLS fingerprints = separate transports
func (tp *TransportPool) generateTransportKey(apiBase, proxyURL string, oauthType oauth.ProviderType, tlsFingerprint TLSFingerprint) string {
	// Normalize API base URL
	apiBase = strings.TrimRight(apiBase, "/")

	// Build key string
	keyStr := apiBase + "|" + proxyURL + "|" + string(oauthType) + "|" + string(tlsFingerprint)

	// Hash the key to create a fixed-length identifier
	h := sha256.New()
	h.Write([]byte(keyStr))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// createTransport creates a new HTTP transport with proxy and TLS fingerprint support
func (tp *TransportPool) createTransport(proxyURL string, tlsFingerprint TLSFingerprint) *http.Transport {
	// Start with a clone of default transport
	var transport *http.Transport

	if proxyURL == "" {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	} else {
		// Parse the proxy URL
		parsedURL, err := url.Parse(proxyURL)
		if err != nil {
			logrus.Errorf("Failed to parse proxy URL %s: %v, using default transport", proxyURL, err)
			transport = http.DefaultTransport.(*http.Transport).Clone()
		} else {
			// Create transport with proxy
			transport = &http.Transport{
				// Use same defaults as http.DefaultTransport
				Proxy: http.ProxyFromEnvironment,
			}

			switch parsedURL.Scheme {
			case "http", "https":
				transport.Proxy = http.ProxyURL(parsedURL)
			case "socks5":
				dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, nil, proxy.Direct)
				if err != nil {
					logrus.Errorf("Failed to create SOCKS5 proxy dialer: %v, using default transport", err)
					transport = http.DefaultTransport.(*http.Transport).Clone()
				} else {
					dialContext, ok := dialer.(proxy.ContextDialer)
					if ok {
						transport.DialContext = dialContext.DialContext
					} else {
						transport = http.DefaultTransport.(*http.Transport).Clone()
					}
				}
			default:
				logrus.Errorf("Unsupported proxy scheme %s, supported schemes are http, https, socks5", parsedURL.Scheme)
				transport = http.DefaultTransport.(*http.Transport).Clone()
			}
		}
	}

	// Apply TLS fingerprint spoofing if configured
	if NeedsUTLS(tlsFingerprint) {
		logrus.Infof("Applying TLS fingerprint spoofing: %s", tlsFingerprint)
		tlsDialer := NewTLSDialer(tlsFingerprint, &tls.Config{})
		transport.DialTLSContext = tlsDialer.DialTLSContext
	}

	return transport
}

// Stats returns statistics about the transport pool
func (tp *TransportPool) Stats() map[string]interface{} {
	tp.mutex.RLock()
	defer tp.mutex.RUnlock()

	return map[string]interface{}{
		"transport_count": len(tp.transports),
	}
}

// Clear removes all transports from the pool
func (tp *TransportPool) Clear() {
	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	tp.transports = make(map[string]*http.Transport)
	logrus.Info("Transport pool cleared")
}
