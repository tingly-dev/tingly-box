package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestClientPool_ClientConstruction exercises the GetXxxClient constructors
// across providers, OAuth and non-OAuth, with and without a record sink. These
// are pure smoke tests: each case just builds clients via the public API and
// asserts they're non-nil. Behavior that's not just "constructor returns
// non-nil" (no client-level caching, mode reporting, record sink wiring) is
// verified once on the baseline OpenAI provider.
func TestClientPool_ClientConstruction(t *testing.T) {
	openaiProvider := &typ.Provider{
		UUID:    "openai-uuid",
		Name:    "OpenAI Provider",
		Token:   "sk-test-12345678",
		APIBase: "https://api.openai.com/v1",
	}
	openaiProvider2 := &typ.Provider{
		UUID:    "openai-uuid-2",
		Name:    "OpenAI Provider 2",
		Token:   "sk-test-87654321",
		APIBase: "https://api.openai.com/v1",
	}
	anthropicProvider := &typ.Provider{
		UUID:    "anthropic-uuid",
		Name:    "Anthropic Provider",
		Token:   "sk-ant-test",
		APIBase: "https://api.anthropic.com/v1",
	}
	googleProvider := &typ.Provider{
		UUID:    "google-uuid",
		Name:    "Google Provider",
		Token:   "google-test-key",
		APIBase: "https://generativelanguage.googleapis.com/v1beta/openai",
	}
	claudeCodeOAuth := &typ.Provider{
		UUID:     "claude-code-uuid",
		Name:     "Claude Code",
		Token:    "sk-ant-oa-test",
		APIBase:  "https://api.anthropic.com/v1",
		AuthType: typ.AuthTypeOAuth,
	}
	codexOAuth := &typ.Provider{
		UUID:     "codex-uuid",
		Name:     "Codex",
		Token:    "sk-codex-test",
		APIBase:  "https://api.openai.com/v1",
		AuthType: typ.AuthTypeOAuth,
	}

	sessionCtx := typ.WithSessionID(
		context.Background(),
		typ.SessionID{Source: typ.SessionSourceUser, Value: "oauth-test"},
	)

	cases := []struct {
		name     string
		ctx      context.Context
		provider *typ.Provider
		kind     string // "openai" | "anthropic" | "google"
	}{
		{"openai", context.Background(), openaiProvider, "openai"},
		{"openai-second-provider", context.Background(), openaiProvider2, "openai"},
		{"anthropic", context.Background(), anthropicProvider, "anthropic"},
		{"google", context.Background(), googleProvider, "google"},
		{"oauth-claude-code", sessionCtx, claudeCodeOAuth, "openai"},
		{"oauth-codex", sessionCtx, codexOAuth, "openai"},
	}

	t.Run("constructors", func(t *testing.T) {
		pool := NewClientPool()
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				var client interface{}
				switch tc.kind {
				case "openai":
					client = pool.GetOpenAIClient(tc.ctx, tc.provider, "gpt-4")
				case "anthropic":
					client = pool.GetAnthropicClient(tc.ctx, tc.provider, "claude-3")
				case "google":
					client = pool.GetGoogleClient(tc.ctx, tc.provider, "gemini-pro")
				}
				if client == nil {
					t.Fatalf("Expected non-nil %s client for %s", tc.kind, tc.provider.Name)
				}
			})
		}
	})

	t.Run("no-client-level-caching-and-mode", func(t *testing.T) {
		pool := NewClientPool()
		c1 := pool.GetOpenAIClient(context.Background(), openaiProvider, "gpt-4")
		c2 := pool.GetOpenAIClient(context.Background(), openaiProvider, "gpt-4")
		if c1 == nil || c2 == nil {
			t.Fatal("Expected non-nil clients")
		}
		if c1 == c2 {
			t.Error("Expected different client instances (no client-level caching)")
		}
		if stats := pool.Stats(); stats["mode"] != "once" {
			t.Errorf("Expected mode 'once', got %v", stats["mode"])
		}
	})

	t.Run("different-providers-different-clients", func(t *testing.T) {
		pool := NewClientPool()
		c1 := pool.GetOpenAIClient(context.Background(), openaiProvider, "")
		c2 := pool.GetOpenAIClient(context.Background(), openaiProvider2, "")
		if c1 == nil || c2 == nil {
			t.Fatal("Expected non-nil clients")
		}
		if c1 == c2 {
			t.Error("Expected different clients for different providers")
		}
	})

	t.Run("with-record-sink", func(t *testing.T) {
		sink := &obs.Sink{}
		pool := NewClientPoolBuilder().WithRecordSink(sink).Build()
		if client := pool.GetOpenAIClient(context.Background(), openaiProvider, "gpt-4"); client == nil {
			t.Fatal("Expected non-nil client")
		}
		if pool.GetRecordSink() == nil {
			t.Error("Record sink not set correctly")
		}
	})
}

// TestClientPool_WithSessionID tests session ID propagation through context
func TestClientPool_WithSessionID(t *testing.T) {
	pool := NewClientPool()

	provider := &typ.Provider{
		UUID:     "oauth-uuid-session",
		Name:     "oauth-provider",
		Token:    "sk-ant-oa-test123",
		APIBase:  "https://api.anthropic.com/v1",
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			Issuer:      ai.IssuerCodex,
			AccessToken: "sk-ant-oa-test123",
		},
	}

	sessionAlice := typ.SessionID{Source: typ.SessionSourceUser, Value: "session-alice"}

	// Create client with sessionID in context
	ctx := typ.WithSessionID(context.Background(), sessionAlice)
	client := pool.GetOpenAIClient(ctx, provider, "gpt-4")
	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	// Trigger transport creation by calling GetTransport directly
	transport := GetGlobalTransportPool().GetTransport(
		provider.UUID,
		"",
		"",
		"codex",
		sessionAlice,
	)

	if transport == nil {
		t.Fatal("Expected non-nil transport")
	}

	// Get transport stats
	stats := pool.Stats()
	if tc, ok := stats["transport_pool"].(map[string]interface{}); ok {
		if count, ok := tc["transport_count"].(int); ok {
			if count == 0 {
				t.Error("Expected non-zero transport count")
			}
		}
	}
}

// TestClientPool_InvalidateSession tests session invalidation
func TestClientPool_InvalidateSession(t *testing.T) {
	pool := NewClientPool()

	oauthProvider := &typ.Provider{
		UUID:     "oauth-uuid-invalidate",
		Name:     "oauth-provider",
		Token:    "sk-ant-oa-test123",
		APIBase:  "https://api.anthropic.com/v1",
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			Issuer:      ai.IssuerCodex,
			AccessToken: "sk-ant-oa-test123",
		},
	}

	sessionAlice := typ.SessionID{Source: typ.SessionSourceUser, Value: "session-alice"}

	// Create a client (this creates a SessionBoundTransport)
	ctx := typ.WithSessionID(context.Background(), sessionAlice)
	client := pool.GetOpenAIClient(ctx, oauthProvider, "gpt-4")
	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	// Trigger transport creation by calling GetTransport directly
	// This simulates what happens during an actual HTTP request
	transport := GetGlobalTransportPool().GetTransport(
		oauthProvider.UUID,
		"",
		"",
		"codex",
		sessionAlice,
	)

	if transport == nil {
		t.Fatal("Expected non-nil transport")
	}

	// Get transport stats before invalidation
	statsBefore := pool.Stats()
	transportCountBefore := 0
	if tc, ok := statsBefore["transport_pool"].(map[string]interface{}); ok {
		if count, ok := tc["transport_count"].(int); ok {
			transportCountBefore = count
		}
	}

	// Invalidate the session
	pool.InvalidateSession(oauthProvider.UUID, sessionAlice.Value)

	// Get transport stats after invalidation
	statsAfter := pool.Stats()
	transportCountAfter := 0
	if tc, ok := statsAfter["transport_pool"].(map[string]interface{}); ok {
		if count, ok := tc["transport_count"].(int); ok {
			transportCountAfter = count
		}
	}

	// Transport count should be less after invalidation
	if transportCountAfter >= transportCountBefore {
		t.Errorf("Expected transport count to decrease after invalidation, before: %d, after: %d", transportCountBefore, transportCountAfter)
	}
}

// TestClientPool_ConcurrentAccess tests concurrent client creation
func TestClientPool_ConcurrentAccess(t *testing.T) {
	pool := NewClientPool()

	provider := &typ.Provider{
		UUID:    "concurrent-uuid",
		Name:    "concurrent-provider",
		Token:   "concurrent-token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	sessionID := typ.SessionID{Source: typ.SessionSourceUser, Value: "concurrent-session"}
	ctx := typ.WithSessionID(context.Background(), sessionID)

	// Launch multiple goroutines to create clients concurrently
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			client := pool.GetOpenAIClient(ctx, provider, "gpt-4")
			if client == nil {
				t.Error("Expected non-nil client")
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// --- Transport Pool Reference Counting Tests ---

// newTestTransportPool creates an isolated TransportPool for testing
// (not the global singleton).
func newTestTransportPool() *TransportPool {
	return &TransportPool{
		transports: make(map[string]*pooledTransport),
		config:     nil,
	}
}

// TestTransportPool_AcquireReleaseRefCount verifies that AcquireTransport increments
// the refCount and the release callback decrements it.
func TestTransportPool_AcquireReleaseRefCount(t *testing.T) {
	pool := newTestTransportPool()
	sessionID := typ.SessionID{Source: typ.SessionSourceUser, Value: "refcount-test"}
	providerUUID := "test-provider-refcount"

	// Acquire transport
	_, release := pool.AcquireTransport(providerUUID, "", "", ai.IssuerClaudeCode, sessionID)
	if release == nil {
		t.Fatal("Expected non-nil release callback")
	}

	// Verify refCount == 1
	key := NewTransportKey(providerUUID, "", ai.IssuerClaudeCode, sessionID).String()
	pool.mutex.RLock()
	pooled, exists := pool.transports[key]
	pool.mutex.RUnlock()
	if !exists {
		t.Fatal("Expected transport to exist in pool")
	}
	if pooled.getRefCount() != 1 {
		t.Errorf("Expected refCount=1, got %d", pooled.getRefCount())
	}

	// Release
	release()

	// Verify refCount == 0
	if pooled.getRefCount() != 0 {
		t.Errorf("Expected refCount=0 after release, got %d", pooled.getRefCount())
	}
}

// TestTransportPool_CleanupSkipsActiveTransports verifies that cleanup does not
// evict transports that have active in-flight requests (refCount > 0).
func TestTransportPool_CleanupSkipsActiveTransports(t *testing.T) {
	pool := newTestTransportPool()
	sessionID := typ.SessionID{Source: typ.SessionSourceUser, Value: "cleanup-skip-test"}
	providerUUID := "test-provider-cleanup-skip"

	// Acquire transport (refCount = 1)
	_, release := pool.AcquireTransport(providerUUID, "", "", ai.IssuerClaudeCode, sessionID)

	// Set lastAccess to far in the past so it's "expired"
	key := NewTransportKey(providerUUID, "", ai.IssuerClaudeCode, sessionID).String()
	pool.mutex.RLock()
	pooled := pool.transports[key]
	pool.mutex.RUnlock()
	atomic.StoreInt64(&pooled.lastAccess, time.Now().Add(-30*time.Minute).UnixNano())

	// Run cleanup with short TTL
	pool.cleanupExpiredTransports(15 * time.Minute)

	// Transport should still be in the map (refCount > 0)
	pool.mutex.RLock()
	_, stillExists := pool.transports[key]
	pool.mutex.RUnlock()
	if !stillExists {
		t.Error("Expected transport to still exist after cleanup (refCount > 0)")
	}

	// Release
	release()
}

// TestTransportPool_CleanupEvictsAfterRelease verifies that cleanup evicts
// a transport once its refCount drops to 0 and TTL has expired.
func TestTransportPool_CleanupEvictsAfterRelease(t *testing.T) {
	pool := newTestTransportPool()
	sessionID := typ.SessionID{Source: typ.SessionSourceUser, Value: "cleanup-evict-test"}
	providerUUID := "test-provider-cleanup-evict"

	// Acquire and set lastAccess to far past
	_, release := pool.AcquireTransport(providerUUID, "", "", ai.IssuerClaudeCode, sessionID)

	key := NewTransportKey(providerUUID, "", ai.IssuerClaudeCode, sessionID).String()
	pool.mutex.RLock()
	pooled := pool.transports[key]
	pool.mutex.RUnlock()
	atomic.StoreInt64(&pooled.lastAccess, time.Now().Add(-30*time.Minute).UnixNano())

	// Cleanup while active — should skip
	pool.cleanupExpiredTransports(15 * time.Minute)

	// Release (refCount -> 0)
	release()

	// Set lastAccess to far past again (first cleanup refreshed it when skipping)
	atomic.StoreInt64(&pooled.lastAccess, time.Now().Add(-30*time.Minute).UnixNano())

	// Cleanup again — should now evict
	pool.cleanupExpiredTransports(15 * time.Minute)

	pool.mutex.RLock()
	_, stillExists := pool.transports[key]
	pool.mutex.RUnlock()
	if stillExists {
		t.Error("Expected transport to be removed after release and cleanup")
	}
}

// TestTransportPool_InvalidateSessionDefersActive verifies that InvalidateSession
// defers removal of transports with active requests.
func TestTransportPool_InvalidateSessionDefersActive(t *testing.T) {
	pool := newTestTransportPool()
	sessionID := typ.SessionID{Source: typ.SessionSourceUser, Value: "invalidate-defer-test"}
	providerUUID := "test-provider-invalidate-defer"

	// Acquire transport (refCount = 1)
	_, release := pool.AcquireTransport(providerUUID, "", "", ai.IssuerClaudeCode, sessionID)

	key := NewTransportKey(providerUUID, "", ai.IssuerClaudeCode, sessionID).String()

	// Invalidate session while active
	pool.InvalidateSession(providerUUID, sessionID.Value)

	// Transport should still be in the map (refCount > 0), but marked for removal
	pool.mutex.RLock()
	_, stillExists := pool.transports[key]
	pool.mutex.RUnlock()
	if !stillExists {
		t.Error("Expected transport to still exist after invalidation (refCount > 0)")
	}

	// Verify lastAccess was set to epoch (marked for deferred removal)
	pool.mutex.RLock()
	lastAccessNano := atomic.LoadInt64(&pool.transports[key].lastAccess)
	pool.mutex.RUnlock()
	if lastAccessNano != 0 {
		t.Errorf("Expected lastAccess to be zero (epoch), got %d", lastAccessNano)
	}

	// Release (refCount -> 0)
	release()

	// Cleanup should now evict it
	pool.cleanupExpiredTransports(1 * time.Minute)

	pool.mutex.RLock()
	_, stillExists = pool.transports[key]
	pool.mutex.RUnlock()
	if stillExists {
		t.Error("Expected transport to be removed after release and cleanup")
	}
}

// TestTransportPool_AcquireReleaseConcurrentRace tests that concurrent AcquireTransport
// and release calls do not cause data races (run with -race).
func TestTransportPool_AcquireReleaseConcurrentRace(t *testing.T) {
	pool := newTestTransportPool()
	sessionID := typ.SessionID{Source: typ.SessionSourceUser, Value: "race-test"}
	providerUUID := "test-provider-race"

	const numGoroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, release := pool.AcquireTransport(providerUUID, "", "", ai.IssuerClaudeCode, sessionID)
			// Simulate some work
			time.Sleep(time.Microsecond * 10)
			release()
		}()
	}

	wg.Wait()

	// All releases done; refCount should be 0
	key := NewTransportKey(providerUUID, "", ai.IssuerClaudeCode, sessionID).String()
	pool.mutex.RLock()
	pooled := pool.transports[key]
	pool.mutex.RUnlock()
	if pooled.getRefCount() != 0 {
		t.Errorf("Expected refCount=0 after all releases, got %d", pooled.getRefCount())
	}
}

// TestSessionBoundTransport_RefCountedBodyRelease tests that the response body
// wrapper correctly decrements the refCount when closed.
func TestSessionBoundTransport_RefCountedBodyRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Use a real TransportPool (not test double) to verify ref counting
	pool := newTestTransportPool()
	sessionID := typ.SessionID{Source: typ.SessionSourceUser, Value: "body-release-test"}
	providerUUID := "test-provider-body-release"

	transport := &SessionBoundTransport{
		transportPool: pool,
		providerUUID:  providerUUID,
		proxyURL:      "",
		issuer:        ai.IssuerClaudeCode,
		sessionID:     sessionID,
	}

	client := &http.Client{Transport: transport}

	key := NewTransportKey(providerUUID, "", ai.IssuerClaudeCode, sessionID).String()

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// During the request, refCount should be 1 (body not yet closed)
	pool.mutex.RLock()
	pooled := pool.transports[key]
	pool.mutex.RUnlock()
	if pooled.getRefCount() != 1 {
		t.Errorf("Expected refCount=1 during request, got %d", pooled.getRefCount())
	}

	// Close the body — should decrement refCount
	resp.Body.Close()

	if pooled.getRefCount() != 0 {
		t.Errorf("Expected refCount=0 after body close, got %d", pooled.getRefCount())
	}
}

// TestSessionBoundTransport_RefCountedBodyDoubleClose tests that double-closing
// the response body does not double-decrement (sync.Once protection).
func TestSessionBoundTransport_RefCountedBodyDoubleClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	pool := newTestTransportPool()
	sessionID := typ.SessionID{Source: typ.SessionSourceUser, Value: "double-close-test"}
	providerUUID := "test-provider-double-close"

	sbt := &SessionBoundTransport{
		transportPool: pool,
		providerUUID:  providerUUID,
		proxyURL:      "",
		issuer:        ai.IssuerClaudeCode,
		sessionID:     sessionID,
	}

	client := &http.Client{Transport: sbt}
	key := NewTransportKey(providerUUID, "", ai.IssuerClaudeCode, sessionID).String()

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Close body twice
	resp.Body.Close()
	resp.Body.Close() // Second close should be safe (sync.Once)

	pool.mutex.RLock()
	pooled := pool.transports[key]
	pool.mutex.RUnlock()
	if pooled.getRefCount() != 0 {
		t.Errorf("Expected refCount=0 after double close, got %d", pooled.getRefCount())
	}
}

// TestTransportPool_LastAccessAtomicRace tests that the lastAccess timestamp
// is updated atomically without the RUnlock->Lock race (run with -race).
func TestTransportPool_LastAccessAtomicRace(t *testing.T) {
	pool := newTestTransportPool()
	sessionID := typ.SessionID{Source: typ.SessionSourceUser, Value: "atomic-race-test"}
	providerUUID := "test-provider-atomic-race"

	const numGoroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.GetTransport(providerUUID, "", "", ai.IssuerClaudeCode, sessionID)
		}()
	}

	// Also run cleanup concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.cleanupExpiredTransports(1 * time.Nanosecond) // Very short TTL to trigger cleanup
		}()
	}

	wg.Wait()
}

// TestTransportPool_EnvProxyNotInherited verifies that transports created without an
// explicit proxy URL do not inherit HTTP_PROXY / HTTPS_PROXY from the environment by
// default (RespectEnvProxy == false).
func TestTransportPool_EnvProxyNotInherited(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://should-not-be-used.invalid:9999")
	t.Setenv("HTTPS_PROXY", "http://should-not-be-used.invalid:9999")

	pool := newTestTransportPool() // config nil → RespectEnvProxy defaults to false

	transport := pool.GetTransport("provider-1", "", "", ai.Issuer(""), typ.SessionID{})
	if transport.Proxy != nil {
		t.Error("transport.Proxy should be nil when no proxy_url is configured and RespectEnvProxy is false")
	}
}

// TestTransportPool_EnvProxyRespectedWhenEnabled verifies that setting RespectEnvProxy=true
// causes transports created without an explicit proxy URL to inherit env proxy settings.
func TestTransportPool_EnvProxyRespectedWhenEnabled(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")

	boolTrue := true
	pool := newTestTransportPool()
	pool.config = &TransportConfig{RespectEnvProxy: &boolTrue}

	transport := pool.GetTransport("provider-2", "", "", ai.Issuer(""), typ.SessionID{})
	if transport.Proxy == nil {
		t.Error("transport.Proxy should be non-nil when RespectEnvProxy=true and HTTP_PROXY is set")
	}
}

// TestSetTransportConfig_ClearsPoolOnRespectEnvProxyChange verifies that changing
// RespectEnvProxy via SetTransportConfig immediately evicts all cached transports so
// the new proxy policy takes effect on the next request.
func TestSetTransportConfig_ClearsPoolOnRespectEnvProxyChange(t *testing.T) {
	// Use a fresh isolated pool so the test doesn't touch the global singleton.
	pool := newTestTransportPool()
	session := typ.SessionID{Source: typ.SessionSourceUser, Value: "clear-test"}

	// Populate the pool with a transport.
	pool.GetTransport("provider-clear", "", "", ai.Issuer(""), session)
	if len(pool.transports) != 1 {
		t.Fatalf("expected 1 cached transport before config change, got %d", len(pool.transports))
	}

	// Simulate a RespectEnvProxy toggle (false → true) directly on the isolated pool
	// (bypassing the global SetTransportConfig so we don't mutate the singleton).
	boolTrue := true
	pool.mutex.Lock()
	oldVal := false // pool.config is nil → defaults to false
	pool.config = &TransportConfig{RespectEnvProxy: &boolTrue}
	removed, deferred := pool.clearLocked()
	pool.mutex.Unlock()

	_ = oldVal
	total := removed + deferred
	if total == 0 {
		t.Error("expected clearLocked to report at least one transport removed or deferred")
	}
	if len(pool.transports) != 0 {
		t.Errorf("expected pool to be empty after clear, got %d entries", len(pool.transports))
	}
}
