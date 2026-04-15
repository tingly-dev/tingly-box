package client

import (
	"context"
	"testing"

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
