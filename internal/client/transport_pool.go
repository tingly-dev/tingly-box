package client

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"

	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// TransportConfig holds the configuration for HTTP transport connection pooling
// All fields are pointers so that zero-value (nil) means "use Go default"
type TransportConfig struct {
	MaxIdleConns        *int  // nil = use Go default (100)
	MaxIdleConnsPerHost *int  // nil = use Go default (2)
	MaxConnsPerHost     *int  // nil = use Go default (0, no limit)
	DisableKeepAlives   *bool // nil = use Go default (false)
}

// Go defaults for reference (not used directly, only for documentation)
const (
	DefaultMaxIdleConns        = 100
	DefaultMaxIdleConnsPerHost = 2
)

// TransportPool manages shared HTTP transports for clients
// Transports are keyed by: providerUUID + model + proxyURL
// This allows multiple clients to share the same connection pool
// when they use the same provider+model+proxy combination.
type TransportPool struct {
	transports map[string]*http.Transport
	config     *TransportConfig // nil = use Go defaults
	mutex      sync.RWMutex
}

// Global singleton transport pool
var globalTransportPool = &TransportPool{
	transports: make(map[string]*http.Transport),
	config:     nil, // nil = use Go defaults (backward compatible with TB)
}

// GetGlobalTransportPool returns the global transport pool singleton
func GetGlobalTransportPool() *TransportPool {
	return globalTransportPool
}

// SetTransportConfig updates the transport pool configuration
// Pass nil to reset to Go defaults (backward compatible)
// This affects newly created transports only, existing transports are not modified
func SetTransportConfig(config *TransportConfig) {
	globalTransportPool.mutex.Lock()
	defer globalTransportPool.mutex.Unlock()

	globalTransportPool.config = config

	if config == nil {
		logrus.Info("Transport pool config reset to Go defaults")
	} else {
		maxIdle := "default"
		maxIdlePerHost := "default"
		if config.MaxIdleConns != nil {
			maxIdle = fmt.Sprintf("%d", *config.MaxIdleConns)
		}
		if config.MaxIdleConnsPerHost != nil {
			maxIdlePerHost = fmt.Sprintf("%d", *config.MaxIdleConnsPerHost)
		}
		logrus.Infof("Transport pool config updated: MaxIdleConns=%s, MaxIdleConnsPerHost=%s",
			maxIdle, maxIdlePerHost)
	}
}

// GetTransport returns or creates a shared HTTP transport for the given configuration
// The transport key is based on: providerUUID + model + proxyURL
// oauthType is used for transport creation but not part of the key
func (tp *TransportPool) GetTransport(providerUUID, model, proxyURL string, oauthType oauth.ProviderType) *http.Transport {
	key := tp.generateTransportKey(providerUUID, model, proxyURL)

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
	logrus.Infof("Creating new transport for provider: %s, model: %s, proxy: %s, oauth: %s", providerUUID, model, proxyURL, oauthType)
	transport := tp.createTransport(proxyURL)
	tp.transports[key] = transport

	return transport
}

// generateTransportKey creates a unique key for transport caching
// The key is based on providerUUID + model + proxyURL to ensure:
// - Same provider + same model + same proxy = shared transport
// - Different providers = separate transports
// - Same provider + different models = separate transports
// - Same provider + same model + different proxies = separate transports
func (tp *TransportPool) generateTransportKey(providerUUID, model, proxyURL string) string {
	// Build key string
	keyStr := providerUUID + "|" + model + "|" + proxyURL

	// Hash the key to create a fixed-length identifier
	h := sha256.New()
	h.Write([]byte(keyStr))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// createTransport creates a new HTTP transport with proxy support
func (tp *TransportPool) createTransport(proxyURL string) *http.Transport {
	if proxyURL == "" {
		// Return a copy of default transport to avoid mutation issues
		transport := http.DefaultTransport.(*http.Transport).Clone()
		tp.applyConfig(transport)
		return transport
	}

	// Parse the proxy URL
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		logrus.Errorf("Failed to parse proxy URL %s: %v, using default transport", proxyURL, err)
		return http.DefaultTransport.(*http.Transport).Clone()
	}

	// Create transport with proxy
	transport := &http.Transport{
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
			return http.DefaultTransport.(*http.Transport).Clone()
		}
		dialContext, ok := dialer.(proxy.ContextDialer)
		if ok {
			transport.DialContext = dialContext.DialContext
		} else {
			return http.DefaultTransport.(*http.Transport).Clone()
		}
	default:
		logrus.Errorf("Unsupported proxy scheme %s, supported schemes are http, https, socks5", parsedURL.Scheme)
		return http.DefaultTransport.(*http.Transport).Clone()
	}

	tp.applyConfig(transport)
	return transport
}

// applyConfig applies custom configuration to transport if set
// TB (tingly-box) will have tp.config == nil, so this is a no-op
func (tp *TransportPool) applyConfig(transport *http.Transport) {
	if tp.config == nil {
		return
	}
	if tp.config.MaxIdleConns != nil {
		transport.MaxIdleConns = *tp.config.MaxIdleConns
	}
	if tp.config.MaxIdleConnsPerHost != nil {
		transport.MaxIdleConnsPerHost = *tp.config.MaxIdleConnsPerHost
	}
	if tp.config.MaxConnsPerHost != nil {
		transport.MaxConnsPerHost = *tp.config.MaxConnsPerHost
	}
	if tp.config.DisableKeepAlives != nil {
		transport.DisableKeepAlives = *tp.config.DisableKeepAlives
	}
	logrus.Debugf("Applied custom transport config: MaxIdleConns=%d, MaxIdleConnsPerHost=%d",
		transport.MaxIdleConns, transport.MaxIdleConnsPerHost)
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
