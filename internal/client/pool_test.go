package client

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Test once mode - no caching
func TestClientPool_OnceMode(t *testing.T) {
	pool := NewClientPoolBuilder().
		WithOnceMode().
		Build()

	provider := &typ.Provider{
		UUID:    "test-uuid-1",
		Name:    "test-provider",
		Token:   "test-token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	// First call should create new client
	client1 := pool.GetOpenAIClient(provider, "gpt-4", typ.SessionID{})
	if client1 == nil {
		t.Fatal("Expected non-nil client")
	}

	// Second call should create different client (no caching)
	client2 := pool.GetOpenAIClient(provider, "gpt-4", typ.SessionID{})
	if client2 == nil {
		t.Fatal("Expected non-nil client")
	}

	// Clients should be different instances
	if client1 == client2 {
		t.Error("Expected different client instances in once mode")
	}

	// Pool size should be 0 (no caching)
	if pool.Size() != 0 {
		t.Errorf("Expected pool size 0 in once mode, got %d", pool.Size())
	}

	// Stats should show once mode
	stats := pool.Stats()
	if stats["mode"] != string(PoolModeOnce) {
		t.Errorf("Expected mode 'once', got %v", stats["mode"])
	}
}

// Test shared mode - with caching
func TestClientPool_SharedMode(t *testing.T) {
	pool := NewClientPoolBuilder().
		WithSharedMode().
		WithClientTTL(10 * time.Minute).
		Build()

	provider := &typ.Provider{
		UUID:    "test-uuid-1",
		Name:    "test-provider",
		Token:   "test-token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	// First call should create new client
	client1 := pool.GetOpenAIClient(provider, "gpt-4", typ.SessionID{})
	if client1 == nil {
		t.Fatal("Expected non-nil client")
	}

	// Second call should return same client (cached)
	client2 := pool.GetOpenAIClient(provider, "gpt-4", typ.SessionID{})
	if client1 != client2 {
		t.Error("Expected same client instance in shared mode")
	}

	// Pool size should be 1
	if pool.Size() != 1 {
		t.Errorf("Expected pool size 1, got %d", pool.Size())
	}

	// Stats should show shared mode
	stats := pool.Stats()
	if stats["mode"] != string(PoolModeShared) {
		t.Errorf("Expected mode 'shared', got %v", stats["mode"])
	}
}

// Test default NewClientPool uses once mode
func TestClientPool_DefaultIsOnceMode(t *testing.T) {
	pool := NewClientPool()

	stats := pool.Stats()
	if stats["mode"] != string(PoolModeOnce) {
		t.Errorf("Expected default mode 'once', got %v", stats["mode"])
	}

	if pool.Size() != 0 {
		t.Errorf("Expected pool size 0 for once mode, got %d", pool.Size())
	}
}

// Test NewSharedClientPool uses shared mode
func TestClientPool_SharedPoolConstructor(t *testing.T) {
	pool := NewSharedClientPool()

	stats := pool.Stats()
	if stats["mode"] != string(PoolModeShared) {
		t.Errorf("Expected mode 'shared', got %v", stats["mode"])
	}
}

func TestClientPool_GetClient(t *testing.T) {
	pool := NewSharedClientPool() // Use shared mode for backward compatibility tests

	// Create test provider
	provider := &typ.Provider{
		UUID:    "test-uuid-1",
		Name:    "test-provider",
		Token:   "test-token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	// First call should create new client
	client1 := pool.GetOpenAIClient(provider, "", typ.SessionID{})
	if client1 == nil {
		t.Fatal("Expected non-nil client")
	}

	// Second call should return same client
	client2 := pool.GetOpenAIClient(provider, "", typ.SessionID{})
	if client1 != client2 {
		t.Error("Expected same client instance for same provider")
	}

	// Verify pool size
	if pool.Size() != 1 {
		t.Errorf("Expected pool size 1, got %d", pool.Size())
	}
}

func TestClientPool_DifferentProviders(t *testing.T) {
	pool := NewSharedClientPool() // Use shared mode for caching test

	// Create different providers
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
	client1 := pool.GetOpenAIClient(provider1, "", typ.SessionID{})
	client2 := pool.GetOpenAIClient(provider2, "", typ.SessionID{})

	if client1 == client2 {
		t.Error("Expected different clients for different providers")
	}

	// Verify pool size
	if pool.Size() != 2 {
		t.Errorf("Expected pool size 2, got %d", pool.Size())
	}
}

func TestClientPool_ConcurrentAccess(t *testing.T) {
	pool := NewSharedClientPool() // Use shared mode for concurrency test

	provider := &typ.Provider{
		UUID:    "concurrent-uuid",
		Name:    "concurrent-provider",
		Token:   "concurrent-token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	// Launch multiple goroutines to access the same provider
	const numGoroutines = 10
	clients := make([]*OpenAIClient, numGoroutines)

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			client := pool.GetOpenAIClient(provider, "", typ.SessionID{})
			clients[index] = client
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// All clients should be the same instance
	firstClient := clients[0]
	for i := 1; i < numGoroutines; i++ {
		if clients[i] != firstClient {
			t.Error("Expected same client instance across all goroutines")
			break
		}
	}

	// Verify pool size
	if pool.Size() != 1 {
		t.Errorf("Expected pool size 1, got %d", pool.Size())
	}
}

func TestClientPool_Clear(t *testing.T) {
	pool := NewSharedClientPool()

	// Add some clients
	provider1 := &typ.Provider{
		UUID:    "clear-uuid-1",
		Name:    "provider1",
		Token:   "token1-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	provider2 := &typ.Provider{
		UUID:    "clear-uuid-2",
		Name:    "provider2",
		Token:   "token2-87654321",
		APIBase: "https://api.openai.com/v1",
	}

	pool.GetOpenAIClient(provider1, "", typ.SessionID{})
	pool.GetOpenAIClient(provider2, "", typ.SessionID{})

	// Verify pool has clients
	if pool.Size() != 2 {
		t.Errorf("Expected pool size 2 before clear, got %d", pool.Size())
	}

	// Clear pool
	pool.Clear()

	// Verify pool is empty
	if pool.Size() != 0 {
		t.Errorf("Expected pool size 0 after clear, got %d", pool.Size())
	}
}

func TestClientPool_RemoveProvider(t *testing.T) {
	pool := NewSharedClientPool()

	provider1 := &typ.Provider{
		UUID:    "remove-uuid-1",
		Name:    "provider1",
		Token:   "token1-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	provider2 := &typ.Provider{
		UUID:    "remove-uuid-2",
		Name:    "provider2",
		Token:   "token2-87654321",
		APIBase: "https://api.openai.com/v1",
	}

	// Add clients
	pool.GetOpenAIClient(provider1, "", typ.SessionID{})
	pool.GetOpenAIClient(provider2, "", typ.SessionID{})

	// Verify pool size
	if pool.Size() != 2 {
		t.Errorf("Expected pool size 2 before removal, got %d", pool.Size())
	}

	// Remove one provider
	pool.RemoveProvider(provider1, "")

	// Verify pool size decreased
	if pool.Size() != 1 {
		t.Errorf("Expected pool size 1 after removal, got %d", pool.Size())
	}

	// Verify remaining client is for provider2
	client := pool.GetOpenAIClient(provider2, "", typ.SessionID{})
	if client == nil {
		t.Error("Expected provider2 client to still exist")
	}
}

func TestClientPool_Stats(t *testing.T) {
	pool := NewSharedClientPool()

	provider := &typ.Provider{
		UUID:    "stats-uuid",
		Name:    "stats-provider",
		Token:   "stats-token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	// Add a client
	pool.GetOpenAIClient(provider, "", typ.SessionID{})

	// Get stats
	stats := pool.Stats()

	totalClients, ok := stats["total_clients"].(int)
	if !ok {
		t.Error("Expected total_clients to be an int")
	} else if totalClients != 1 {
		t.Errorf("Expected total_clients to be 1, got %d", totalClients)
	}

	keys, ok := stats["provider_keys"].([]string)
	if !ok {
		t.Error("Expected provider_keys to be a string slice")
	} else if len(keys) != 1 {
		t.Errorf("Expected 1 provider key, got %d", len(keys))
	}

	mode, ok := stats["mode"].(string)
	if !ok {
		t.Error("Expected mode to be a string")
	} else if mode != string(PoolModeShared) {
		t.Errorf("Expected mode 'shared', got %s", mode)
	}
}

// Test InvalidateProvider in both modes
func TestClientPool_InvalidateProvider_OnceMode(t *testing.T) {
	pool := NewClientPoolBuilder().WithOnceMode().Build()

	provider := &typ.Provider{
		UUID:    "invalidate-uuid",
		Name:    "invalidate-provider",
		Token:   "token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	// Create a client (but it won't be cached)
	client := pool.GetOpenAIClient(provider, "gpt-4", typ.SessionID{})
	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	// Invalidate should be no-op in once mode
	pool.InvalidateProvider(provider.UUID)

	// Size should still be 0
	if pool.Size() != 0 {
		t.Errorf("Expected pool size 0 after invalidate in once mode, got %d", pool.Size())
	}
}

func TestClientPool_InvalidateProvider_SharedMode(t *testing.T) {
	pool := NewClientPoolBuilder().WithSharedMode().Build()

	provider := &typ.Provider{
		UUID:    "invalidate-uuid",
		Name:    "invalidate-provider",
		Token:   "token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	// Create clients
	pool.GetOpenAIClient(provider, "gpt-4", typ.SessionID{})
	pool.GetAnthropicClient(provider, "claude-3", typ.SessionID{})

	// Size should be 2
	if pool.Size() != 2 {
		t.Errorf("Expected pool size 2 before invalidate, got %d", pool.Size())
	}

	// Invalidate all clients for this provider
	pool.InvalidateProvider(provider.UUID)

	// Size should be 0
	if pool.Size() != 0 {
		t.Errorf("Expected pool size 0 after invalidate, got %d", pool.Size())
	}
}

// Test builder patterns
func TestClientPoolBuilder_FluentAPI(t *testing.T) {
	pool := NewClientPoolBuilder().
		WithSharedMode().
		WithClientTTL(30 * time.Minute).
		WithCleanupInterval(15 * time.Minute).
		Build()

	stats := pool.Stats()
	if stats["mode"] != string(PoolModeShared) {
		t.Errorf("Expected mode 'shared', got %v", stats["mode"])
	}
}

func TestClientPoolBuilder_WithMode(t *testing.T) {
	pool := NewClientPoolBuilder().
		WithMode(PoolModeShared).
		Build()

	stats := pool.Stats()
	if stats["mode"] != string(PoolModeShared) {
		t.Errorf("Expected mode 'shared', got %v", stats["mode"])
	}
}

// TestClientPool_OAuthSessionKey verifies that OAuth providers with a session get a separate
// cache entry from the same provider without a session (or with a different session).
func TestClientPool_OAuthSessionKey(t *testing.T) {
	pool := NewSharedClientPool()

	oauthProvider := &typ.Provider{
		UUID:     "oauth-uuid-1",
		Name:     "oauth-provider",
		Token:    "test-token-12345678",
		APIBase:  "https://api.openai.com/v1",
		AuthType: typ.AuthTypeOAuth,
	}

	sessionAlice := typ.SessionID{Source: typ.SessionSourceUser, Value: "session-alice"}
	sessionBob := typ.SessionID{Source: typ.SessionSourceUser, Value: "session-bob"}

	// Same provider, different sessions → different clients
	c1 := pool.GetOpenAIClient(oauthProvider, "gpt-4", sessionAlice)
	c2 := pool.GetOpenAIClient(oauthProvider, "gpt-4", sessionBob)
	if c1 == c2 {
		t.Error("Expected different clients for different OAuth sessions")
	}

	// Same provider, same session → same client (cached)
	c3 := pool.GetOpenAIClient(oauthProvider, "gpt-4", sessionAlice)
	if c1 != c3 {
		t.Error("Expected same client for same OAuth session")
	}

	// Pool should have 2 entries (one per session)
	if pool.Size() != 2 {
		t.Errorf("Expected pool size 2 for 2 sessions, got %d", pool.Size())
	}
}

// TestClientPool_IPSessionNotUsedForOAuth verifies that ip:-prefixed sessions
// (fallback values) are not used as OAuth session keys.
func TestClientPool_IPSessionNotUsedForOAuth(t *testing.T) {
	pool := NewSharedClientPool()

	oauthProvider := &typ.Provider{
		UUID:     "oauth-uuid-2",
		Name:     "oauth-provider",
		Token:    "test-token-12345678",
		APIBase:  "https://api.openai.com/v1",
		AuthType: typ.AuthTypeOAuth,
	}

	// ip: prefix should be treated as no session → provider-level key
	c1 := pool.GetOpenAIClient(oauthProvider, "gpt-4", typ.SessionID{Source: typ.SessionSourceIP, Value: "1.2.3.4"})
	c2 := pool.GetOpenAIClient(oauthProvider, "gpt-4", typ.SessionID{Source: typ.SessionSourceIP, Value: "5.6.7.8"})
	if c1 != c2 {
		t.Error("Expected same client for ip:-prefixed sessions (no session isolation)")
	}

	if pool.Size() != 1 {
		t.Errorf("Expected pool size 1 for ip: sessions (shared), got %d", pool.Size())
	}
}

// TestClientPool_NonOAuthIgnoresSession verifies that non-OAuth providers
// always use provider-level keys regardless of sessionID.
func TestClientPool_NonOAuthIgnoresSession(t *testing.T) {
	pool := NewSharedClientPool()

	apiKeyProvider := &typ.Provider{
		UUID:     "apikey-uuid-1",
		Name:     "apikey-provider",
		Token:    "test-token-12345678",
		APIBase:  "https://api.openai.com/v1",
		AuthType: typ.AuthTypeAPIKey,
	}

	c1 := pool.GetOpenAIClient(apiKeyProvider, "gpt-4", typ.SessionID{Source: typ.SessionSourceUser, Value: "session-alice"})
	c2 := pool.GetOpenAIClient(apiKeyProvider, "gpt-4", typ.SessionID{Source: typ.SessionSourceUser, Value: "session-bob"})
	if c1 != c2 {
		t.Error("Expected same client for non-OAuth provider regardless of session")
	}

	if pool.Size() != 1 {
		t.Errorf("Expected pool size 1 for non-OAuth provider, got %d", pool.Size())
	}
}

// TestClientPool_InvalidateSession verifies session-scoped clients are removed.
func TestClientPool_InvalidateSession(t *testing.T) {
	pool := NewSharedClientPool()

	oauthProvider := &typ.Provider{
		UUID:     "oauth-uuid-3",
		Name:     "oauth-provider",
		Token:    "test-token-12345678",
		APIBase:  "https://api.openai.com/v1",
		AuthType: typ.AuthTypeOAuth,
	}

	sessionAlice := typ.SessionID{Source: typ.SessionSourceUser, Value: "session-alice"}
	sessionBob := typ.SessionID{Source: typ.SessionSourceUser, Value: "session-bob"}

	pool.GetOpenAIClient(oauthProvider, "gpt-4", sessionAlice)
	pool.GetOpenAIClient(oauthProvider, "gpt-4", sessionBob)

	if pool.Size() != 2 {
		t.Fatalf("Expected pool size 2 before invalidate, got %d", pool.Size())
	}

	pool.InvalidateSession(oauthProvider.UUID, sessionAlice.Value)

	if pool.Size() != 1 {
		t.Errorf("Expected pool size 1 after InvalidateSession, got %d", pool.Size())
	}
}
