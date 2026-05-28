package client

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"golang.org/x/net/proxy"

	"github.com/tingly-dev/tingly-box/internal/typ"
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
	DefaultTransportTTL             = 15 * time.Minute // Default time-to-live for cached transports (reduced from 120min for better session lifecycle management)
	DefaultTransportCleanupInterval = 5 * time.Minute  // Default interval for cleanup task
)

// pooledTransport wraps a transport with reference counting and last access timestamp for TTL tracking.
// refCount and lastAccess use atomic operations so they can be updated under RLock without
// upgrading to a full write lock.
type pooledTransport struct {
	transport  *http.Transport
	refCount   int32 // active in-flight requests (atomic)
	lastAccess int64 // UnixNano of last access (atomic)
}

// incrementRefCount atomically increments and returns the new value.
func (pt *pooledTransport) incrementRefCount() int32 {
	return atomic.AddInt32(&pt.refCount, 1)
}

// decrementRefCount atomically decrements and returns the new value.
func (pt *pooledTransport) decrementRefCount() int32 {
	return atomic.AddInt32(&pt.refCount, -1)
}

// getRefCount returns the current reference count.
func (pt *pooledTransport) getRefCount() int32 {
	return atomic.LoadInt32(&pt.refCount)
}

// setLastAccess atomically sets the last access time to now.
func (pt *pooledTransport) setLastAccess() {
	atomic.StoreInt64(&pt.lastAccess, time.Now().UnixNano())
}

// getLastAccess returns the last access time.
func (pt *pooledTransport) getLastAccess() time.Time {
	return time.Unix(0, atomic.LoadInt64(&pt.lastAccess))
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
// When RespectEnvProxy changes, cached transports are cleared immediately so
// that the new proxy policy takes effect on the next request.
func SetTransportConfig(config *TransportConfig) {
	globalTransportPool.mutex.Lock()
	defer globalTransportPool.mutex.Unlock()

	oldRespectEnvProxy := boolPtrVal(nil)
	if globalTransportPool.config != nil {
		oldRespectEnvProxy = boolPtrVal(globalTransportPool.config.RespectEnvProxy)
	}

	globalTransportPool.config = config

	newRespectEnvProxy := boolPtrVal(nil)
	if config != nil {
		newRespectEnvProxy = boolPtrVal(config.RespectEnvProxy)
	}

	if oldRespectEnvProxy != newRespectEnvProxy {
		removed, deferred := globalTransportPool.clearLocked()
		logrus.Infof("Transport pool cleared (respect_env_proxy: %v → %v): %d removed, %d deferred",
			oldRespectEnvProxy, newRespectEnvProxy, removed, deferred)
	}

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

// boolPtrVal returns the value of a *bool, treating nil as false.
func boolPtrVal(b *bool) bool {
	return b != nil && *b
}

// GetTransport returns or creates a shared HTTP transport for the given configuration.
// The transport key is based on: providerUUID + sessionID (for OAuth providers).
// proxyURL is used to configure the transport but is NOT part of the key.
// sessionID is used to scope transports for OAuth providers that require per-session isolation.
//
// Note: For in-flight request tracking (preventing cleanup of active transports),
// use AcquireTransport instead.
func (tp *TransportPool) GetTransport(providerUUID, model, proxyURL string, issuer ai.Issuer, sessionID typ.SessionID) *http.Transport {
	key := NewTransportKey(providerUUID, "", issuer, sessionID).String()

	// Try to get existing transport with read lock
	tp.mutex.RLock()
	if pooled, exists := tp.transports[key]; exists {
		pooled.setLastAccess() // atomic — no need to upgrade to write lock
		tp.mutex.RUnlock()
		logrus.Debugf("Using cached transport for key: %s", key)
		return pooled.transport
	}
	tp.mutex.RUnlock()

	// Need to create new transport, acquire write lock
	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	// Double-check after acquiring write lock to avoid race conditions
	if pooled, exists := tp.transports[key]; exists {
		pooled.setLastAccess()
		logrus.Debugf("Using cached transport for key: %s (double-check)", key)
		return pooled.transport
	}

	// Create new transport
	logrus.Infof("Creating new transport for provider: %s, model: %s, proxy: %s, oauth: %s, session: %s",
		providerUUID, model, proxyURL, issuer, sessionID.Value)
	transport := tp.createTransport(proxyURL)
	pt := &pooledTransport{transport: transport}
	pt.setLastAccess()
	tp.transports[key] = pt

	return transport
}

// AcquireTransport returns a transport for the given configuration and increments its
// reference count. The caller MUST call the returned release function exactly once
// when the request is complete (typically by wrapping the response body).
// This prevents the cleanup task from evicting a transport that has active in-flight requests.
func (tp *TransportPool) AcquireTransport(providerUUID, model, proxyURL string, issuer ai.Issuer, sessionID typ.SessionID) (*http.Transport, func()) {
	key := NewTransportKey(providerUUID, "", issuer, sessionID).String()

	// Fast path: read lock for cache hit
	tp.mutex.RLock()
	if pooled, exists := tp.transports[key]; exists {
		pooled.incrementRefCount()
		pooled.setLastAccess()
		tp.mutex.RUnlock()
		logrus.Debugf("Acquired cached transport for key: %s", key)
		return pooled.transport, func() { pooled.decrementRefCount() }
	}
	tp.mutex.RUnlock()

	// Slow path: write lock to create
	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	// Double-check after acquiring write lock
	if pooled, exists := tp.transports[key]; exists {
		pooled.incrementRefCount()
		pooled.setLastAccess()
		logrus.Debugf("Acquired cached transport for key: %s (double-check)", key)
		return pooled.transport, func() { pooled.decrementRefCount() }
	}

	// Create new transport
	logrus.Infof("Creating new transport for provider: %s, model: %s, proxy: %s, oauth: %s, session: %s",
		providerUUID, model, proxyURL, issuer, sessionID.Value)
	transport := tp.createTransport(proxyURL)
	pt := &pooledTransport{transport: transport}
	pt.setLastAccess()
	pt.incrementRefCount()
	tp.transports[key] = pt

	return transport, func() { pt.decrementRefCount() }
}

// generateTransportKey creates a unique key for transport caching.
// Kept for reference; production code uses typ.NewTransportKey directly.
// The key is based on providerUUID + sessionID (for OAuth) to ensure:
// - Same provider + same session = shared transport (connection reuse)
// - Different providers = separate transports
// - OAuth providers + different sessions = separate transports
//
// Note: ProxyURL is NOT part of the key - it's a provider configuration.
func (tp *TransportPool) generateTransportKey(providerUUID, proxyURL string, issuer ai.Issuer, sessionID typ.SessionID) string {
	return NewTransportKey(providerUUID, "", issuer, sessionID).String()
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

// clearLocked removes all transports from the pool and returns (removed, deferred) counts.
// Caller must hold tp.mutex (write lock). Logging is left to the caller so that context
// (reason for clearing) can be included in a single log line.
func (tp *TransportPool) clearLocked() (removed, deferred int) {
	for key, pooled := range tp.transports {
		if pooled.getRefCount() > 0 {
			pooled.transport.CloseIdleConnections()
			atomic.StoreInt64(&pooled.lastAccess, 0) // mark for deferred removal
			deferred++
		} else {
			pooled.transport.CloseIdleConnections()
			delete(tp.transports, key)
			removed++
		}
	}
	return removed, deferred
}

// Clear removes all transports from the pool and closes idle connections.
// Transports with active in-flight requests (refCount > 0) are marked for deferred
// removal (lastAccess set to epoch) and will be cleaned up on the next cycle after
// their requests complete.
func (tp *TransportPool) Clear() {
	tp.mutex.Lock()
	defer tp.mutex.Unlock()
	removed, deferred := tp.clearLocked()
	logrus.Infof("Transport pool cleared: %d removed, %d deferred (active requests)", removed, deferred)
}

// InvalidateProvider removes all transports associated with a specific provider UUID.
// This should be called when provider credentials are updated (e.g., OAuth token refresh).
// Transports with active requests are marked for deferred removal.
func (tp *TransportPool) InvalidateProvider(providerUUID string) {
	if providerUUID == "" {
		return
	}

	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	uuidToken := `"` + providerUUID + `"`
	removedCount := 0
	deferredCount := 0

	for key, pooled := range tp.transports {
		if strings.Contains(key, uuidToken) {
			if pooled.getRefCount() > 0 {
				pooled.transport.CloseIdleConnections()
				atomic.StoreInt64(&pooled.lastAccess, 0) // mark for deferred removal
				deferredCount++
			} else {
				pooled.transport.CloseIdleConnections()
				delete(tp.transports, key)
				removedCount++
			}
		}
	}

	if removedCount > 0 || deferredCount > 0 {
		logrus.Infof("Invalidated %d transport(s) for provider UUID: %s (%d deferred due to active requests)",
			removedCount, providerUUID, deferredCount)
	}
}

// InvalidateSession removes all transports associated with a specific session for a provider.
// This should be called when a session ends or its OAuth token is revoked.
// Transports with active requests are marked for deferred removal.
func (tp *TransportPool) InvalidateSession(providerUUID, sessionID string) {
	if sessionID == "" {
		return
	}

	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	uuidToken := `"` + providerUUID + `"`
	sessionToken := `"` + sessionID + `"`
	removedCount := 0
	deferredCount := 0

	for key, pooled := range tp.transports {
		if strings.Contains(key, uuidToken) && strings.Contains(key, sessionToken) {
			if pooled.getRefCount() > 0 {
				pooled.transport.CloseIdleConnections()
				atomic.StoreInt64(&pooled.lastAccess, 0) // mark for deferred removal
				deferredCount++
			} else {
				pooled.transport.CloseIdleConnections()
				delete(tp.transports, key)
				removedCount++
			}
		}
	}

	if removedCount > 0 || deferredCount > 0 {
		logrus.Infof("Invalidated %d transport(s) for provider UUID: %s session: %s (%d deferred due to active requests)",
			removedCount, providerUUID, sessionID, deferredCount)
	}
}

// cleanupExpiredTransports removes transports that haven't been accessed within the TTL period.
// Transports with active in-flight requests (refCount > 0) are skipped and their lastAccess
// is refreshed so they get a fresh TTL window after the active request completes.
func (tp *TransportPool) cleanupExpiredTransports(ttl time.Duration) {
	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	now := time.Now()
	expirationThreshold := now.Add(-ttl)

	removedCount := 0
	skippedCount := 0
	for key, pooled := range tp.transports {
		if pooled.getRefCount() > 0 {
			// Transport has active requests; refresh lastAccess so it gets a fresh
			// TTL window once the request completes instead of being immediately evicted.
			pooled.setLastAccess()
			skippedCount++
			continue
		}
		if pooled.getLastAccess().Before(expirationThreshold) {
			pooled.transport.CloseIdleConnections()
			delete(tp.transports, key)
			removedCount++
		}
	}

	if removedCount > 0 || skippedCount > 0 {
		logrus.Infof("Cleaned up %d expired transports from pool, skipped %d with active requests", removedCount, skippedCount)
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
	logrus.Debugf("Started transport pool cleanup task with interval: %v, TTL: %v", interval, ttl)
}
