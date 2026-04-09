package client

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"

	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// Constants for proxy URL values
const (
	ProxyURLNone = "none" // Special value to force direct connection (disable proxy)
)

// TransportConfig holds the configuration for HTTP transport connection pooling
// All fields are pointers so that zero-value (nil) means "use Go default"
type TransportConfig struct {
	MaxIdleConns        *int  // nil = use Go default (100)
	MaxIdleConnsPerHost *int  // nil = use Go default (2)
	MaxConnsPerHost     *int  // nil = use Go default (0, no limit)
	DisableKeepAlives   *bool // nil = use Go default (false)

	// RespectEnvProxy controls whether providers without explicit proxy configuration
	// should use environment/system proxy settings (HTTP_PROXY, HTTPS_PROXY, macOS system proxy, etc.)
	// Default (nil): false - providers without proxy_url connect directly
	// Set to true: providers without proxy_url will use system/environment proxy
	RespectEnvProxy *bool // nil = use default (false)
}

// Go defaults for reference (not used directly, only for documentation)
const (
	DefaultMaxIdleConns        = 100
	DefaultMaxIdleConnsPerHost = 2
)

// Constants for transport TTL and cleanup interval
const (
	DefaultTransportTTL             = 120 * time.Minute // Default time-to-live for cached transports
	DefaultTransportCleanupInterval = 60 * time.Minute  // Default interval for cleanup task
)

// pooledTransport wraps a transport with its last access timestamp for TTL tracking
type pooledTransport struct {
	transport  *http.Transport
	lastAccess time.Time
}

// TransportPool manages shared HTTP transports for clients
// Transports are keyed by: providerUUID + sessionID (for OAuth providers)
// This allows multiple clients to share the same connection pool
// when they use the same provider+session combination.
// For OAuth providers, transports are session-scoped to prevent cross-session contamination.
//
// Note: ProxyURL is NOT part of the transport key. It's used to configure
// how the transport is created, but doesn't create a separate pool.
type TransportPool struct {
	transports map[string]*pooledTransport
	config     *TransportConfig // nil = use Go defaults
	mutex      sync.RWMutex
}

// Global singleton transport pool
var globalTransportPool = &TransportPool{
	transports: make(map[string]*pooledTransport),
	config:     nil, // nil = use Go defaults (backward compatible with TB)
}

func init() {
	globalTransportPool.StartCleanupTask(DefaultTransportCleanupInterval, DefaultTransportTTL)
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

// GetTransport returns or creates a shared HTTP transport for the given configuration.
// The transport key is based on: providerUUID + sessionID (for OAuth providers).
// proxyURL is used to configure the transport but is NOT part of the key.
// sessionID is used to scope transports for OAuth providers that require per-session isolation.
func (tp *TransportPool) GetTransport(providerUUID, model, proxyURL string, oauthType oauth.ProviderType, sessionID typ.SessionID) *http.Transport {
	key := NewTransportKey(providerUUID, "", oauthType, sessionID).String()

	// Try to get existing transport with read lock first
	tp.mutex.RLock()
	if pooled, exists := tp.transports[key]; exists {
		tp.mutex.RUnlock()
		// Update last access time
		tp.mutex.Lock()
		pooled.lastAccess = time.Now()
		tp.mutex.Unlock()
		logrus.Debugf("Using cached transport for key: %s", key)
		return pooled.transport
	}
	tp.mutex.RUnlock()

	// Need to create new transport, acquire write lock
	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	// Double-check after acquiring write lock to avoid race conditions
	if pooled, exists := tp.transports[key]; exists {
		pooled.lastAccess = time.Now()
		logrus.Debugf("Using cached transport for key: %s (double-check)", key)
		return pooled.transport
	}

	// Create new transport
	logrus.Infof("Creating new transport for provider: %s, model: %s, proxy: %s, oauth: %s, session: %s",
		providerUUID, model, proxyURL, oauthType, sessionID.Value)
	transport := tp.createTransport(proxyURL)
	tp.transports[key] = &pooledTransport{
		transport:  transport,
		lastAccess: time.Now(),
	}

	return transport
}

// generateTransportKey creates a unique key for transport caching.
// Kept for reference; production code uses typ.NewTransportKey directly.
// The key is based on providerUUID + sessionID (for OAuth) to ensure:
// - Same provider + same session = shared transport (connection reuse)
// - Different providers = separate transports
// - OAuth providers + different sessions = separate transports
//
// Note: ProxyURL is NOT part of the key - it's a provider configuration.
func (tp *TransportPool) generateTransportKey(providerUUID, proxyURL string, oauthType oauth.ProviderType, sessionID typ.SessionID) string {
	return NewTransportKey(providerUUID, "", oauthType, sessionID).String()
}

// newDirectTransport returns a transport with env proxy disabled (direct connection).
func (tp *TransportPool) newDirectTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport
}

// respectEnvProxy returns true if providers without explicit proxy should use env/system proxy.
// Default is false — only use proxy when explicitly configured.
func (tp *TransportPool) respectEnvProxy() bool {
	if tp.config != nil && tp.config.RespectEnvProxy != nil {
		return *tp.config.RespectEnvProxy
	}
	return false
}

// createTransport creates a new HTTP transport with proxy support
func (tp *TransportPool) createTransport(proxyURL string) *http.Transport {
	if proxyURL == "" {
		// Clone default transport for connection pool settings, then clear proxy
		// unless the user has explicitly opted into env proxy via RespectEnvProxy.
		transport := http.DefaultTransport.(*http.Transport).Clone()
		if !tp.respectEnvProxy() {
			transport.Proxy = nil
		}
		tp.applyConfig(transport)
		return transport
	}

	// Parse the proxy URL
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		logrus.Errorf("Failed to parse proxy URL %s: %v, using default transport", proxyURL, err)
		transport := http.DefaultTransport.(*http.Transport).Clone()
		if !tp.respectEnvProxy() {
			transport.Proxy = nil
		}
		return transport
	}

	// Create transport with explicit proxy — no env proxy fallback
	transport := &http.Transport{}

	switch parsedURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsedURL)
	case "socks5":
		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, nil, proxy.Direct)
		if err != nil {
			logrus.Errorf("Failed to create SOCKS5 proxy dialer: %v, using direct transport", err)
			return tp.newDirectTransport()
		}
		dialContext, ok := dialer.(proxy.ContextDialer)
		if ok {
			transport.DialContext = dialContext.DialContext
		} else {
			return tp.newDirectTransport()
		}
	default:
		logrus.Errorf("Unsupported proxy scheme %s, supported schemes are http, https, socks5", parsedURL.Scheme)
		return tp.newDirectTransport()
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

// Clear removes all transports from the pool and closes idle connections
func (tp *TransportPool) Clear() {
	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	for key, pooled := range tp.transports {
		pooled.transport.CloseIdleConnections()
		logrus.Debugf("Closed idle connections for transport key: %s", key)
	}
	tp.transports = make(map[string]*pooledTransport)
	logrus.Info("Transport pool cleared")
}

// InvalidateSession removes all transports associated with a specific session for a provider.
// This should be called when a session ends or its OAuth token is revoked.
func (tp *TransportPool) InvalidateSession(providerUUID, sessionID string) {
	if sessionID == "" {
		return
	}

	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	// Keys are JSON from TransportKey.String(); match both provider_uuid and session_id value fields.
	uuidToken := `"` + providerUUID + `"`
	sessionToken := `"` + sessionID + `"`
	count := 0

	for key, pooled := range tp.transports {
		// Note: ProxyURL is no longer part of the key, so we don't need to match it
		if strings.Contains(key, uuidToken) && strings.Contains(key, sessionToken) {
			pooled.transport.CloseIdleConnections()
			delete(tp.transports, key)
			count++
		}
	}

	if count > 0 {
		logrus.Infof("Invalidated %d transport(s) for provider UUID: %s session: %s", count, providerUUID, sessionID)
	}
}

// cleanupExpiredTransports removes transports that haven't been accessed within the TTL period
func (tp *TransportPool) cleanupExpiredTransports(ttl time.Duration) {
	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	now := time.Now()
	expirationThreshold := now.Add(-ttl)

	removedCount := 0
	for key, pooled := range tp.transports {
		if pooled.lastAccess.Before(expirationThreshold) {
			pooled.transport.CloseIdleConnections()
			delete(tp.transports, key)
			removedCount++
		}
	}

	if removedCount > 0 {
		logrus.Infof("Cleaned up %d expired transports from pool", removedCount)
	}
}

// StartCleanupTask starts a periodic cleanup task that removes expired transports
func (tp *TransportPool) StartCleanupTask(interval, ttl time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			tp.cleanupExpiredTransports(ttl)
		}
	}()
	logrus.Infof("Started transport pool cleanup task with interval: %v, TTL: %v", interval, ttl)
}
