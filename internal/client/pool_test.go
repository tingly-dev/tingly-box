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

// TestClientPool_BasicCreation tests basic client creation
func TestClientPool_BasicCreation(t *testing.T) {
	pool := NewClientPool()

	provider := &typ.Provider{
		UUID:    "test-uuid-1",
		Name:    "test-provider",
		Token:   "test-token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	// Create client
	client := pool.GetOpenAIClient(context.Background(), provider, "gpt-4")
	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	// Creating another client should return a new instance
	client2 := pool.GetOpenAIClient(context.Background(), provider, "gpt-4")
	if client2 == nil {
		t.Fatal("Expected non-nil client")
	}

	// Clients should be different instances (no caching at client level)
	if client == client2 {
		t.Error("Expected different client instances (no client-level caching)")
	}

	// Verify stats shows "once" mode
	stats := pool.Stats()
	if stats["mode"] != "once" {
		t.Errorf("Expected mode 'once', got %v", stats["mode"])
	}
}

// TestClientPool_WithRecordSink tests record sink configuration
func TestClientPool_WithRecordSink(t *testing.T) {
	sink := &obs.Sink{} // Mock sink
	pool := NewClientPoolBuilder().
		WithRecordSink(sink).
		Build()

	provider := &typ.Provider{
		UUID:    "test-uuid-sink",
		Name:    "test-provider",
		Token:   "test-token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	client := pool.GetOpenAIClient(context.Background(), provider, "gpt-4")
	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	// Verify record sink is set
	retrievedSink := pool.GetRecordSink()
	if retrievedSink == nil {
		t.Error("Record sink not set correctly")
	}
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
			ProviderType: "claude_code",
			AccessToken:  "sk-ant-oa-test123",
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
		"claude_code",
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
			ProviderType: "claude_code",
			AccessToken:  "sk-ant-oa-test123",
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
		"claude_code",
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

// TestClientPool_DifferentProviders tests that different providers work correctly
func TestClientPool_DifferentProviders(t *testing.T) {
	pool := NewClientPool()

	provider1 := &typ.Provider{
		UUID:    "provider-uuid-1",
		Name:    "provider1",
		Token:   "token1-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	provider2 := &typ.Provider{
		UUID:    "provider-uuid-2",
		Name:    "provider2",
		Token:   "token2-87654321",
		APIBase: "https://api.openai.com/v1",
	}

	// Get clients for different providers
	client1 := pool.GetOpenAIClient(context.Background(), provider1, "")
	client2 := pool.GetOpenAIClient(context.Background(), provider2, "")

	if client1 == nil || client2 == nil {
		t.Fatal("Expected non-nil clients")
	}

	// Clients should be different instances
	if client1 == client2 {
		t.Error("Expected different clients for different providers")
	}
}

// TestClientPool_AllProviderTypes tests all provider client types
func TestClientPool_AllProviderTypes(t *testing.T) {
	pool := NewClientPool()

	openaiProvider := &typ.Provider{
		UUID:    "openai-uuid",
		Name:    "OpenAI Provider",
		Token:   "sk-test",
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

	openaiClient := pool.GetOpenAIClient(context.Background(), openaiProvider, "gpt-4")
	anthropicClient := pool.GetAnthropicClient(context.Background(), anthropicProvider, "claude-3")
	googleClient := pool.GetGoogleClient(context.Background(), googleProvider, "gemini-pro")

	if openaiClient == nil {
		t.Error("Expected non-nil OpenAI client")
	}
	if anthropicClient == nil {
		t.Error("Expected non-nil Anthropic client")
	}
	if googleClient == nil {
		t.Error("Expected non-nil Google client")
	}
}

// TestClientPool_OAuthProviderTypes tests various OAuth provider types
func TestClientPool_OAuthProviderTypes(t *testing.T) {
	pool := NewClientPool()

	sessionID := typ.SessionID{Source: typ.SessionSourceUser, Value: "oauth-test"}

	providers := []*typ.Provider{
		{
			UUID:     "claude-code-uuid",
			Name:     "Claude Code",
			Token:    "sk-ant-oa-test",
			APIBase:  "https://api.anthropic.com/v1",
			AuthType: typ.AuthTypeOAuth,
		},
		{
			UUID:     "codex-uuid",
			Name:     "Codex",
			Token:    "sk-codex-test",
			APIBase:  "https://api.openai.com/v1",
			AuthType: typ.AuthTypeOAuth,
		},
	}

	ctx := typ.WithSessionID(context.Background(), sessionID)
	for _, provider := range providers {
		client := pool.GetOpenAIClient(ctx, provider, "gpt-4")
		if client == nil {
			t.Errorf("Expected non-nil client for provider %s", provider.Name)
		}
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
		oauthType:     ai.IssuerClaudeCode,
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
		oauthType:     ai.IssuerClaudeCode,
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
