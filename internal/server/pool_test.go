package server

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"tingly-box/internal/config"
)

func TestClientPool_GetClient(t *testing.T) {
	pool := NewClientPool()

	// Create test provider
	provider := &config.Provider{
		Name:    "test-provider",
		Token:   "test-token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	// First call should create new client
	client1 := pool.GetClient(provider)
	if client1 == nil {
		t.Fatal("Expected non-nil client")
	}

	// Second call should return same client
	client2 := pool.GetClient(provider)
	if client1 != client2 {
		t.Error("Expected same client instance for same provider")
	}

	// Verify pool size
	if pool.Size() != 1 {
		t.Errorf("Expected pool size 1, got %d", pool.Size())
	}
}

func TestClientPool_DifferentProviders(t *testing.T) {
	pool := NewClientPool()

	// Create different providers
	provider1 := &config.Provider{
		Name:    "provider1",
		Token:   "token1-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	provider2 := &config.Provider{
		Name:    "provider2",
		Token:   "token2-87654321",
		APIBase: "https://api.openai.com/v1",
	}

	// Get clients for different providers
	client1 := pool.GetClient(provider1)
	client2 := pool.GetClient(provider2)

	if client1 == client2 {
		t.Error("Expected different clients for different providers")
	}

	// Verify pool size
	if pool.Size() != 2 {
		t.Errorf("Expected pool size 2, got %d", pool.Size())
	}
}

func TestClientPool_ConcurrentAccess(t *testing.T) {
	pool := NewClientPool()

	provider := &config.Provider{
		Name:    "concurrent-provider",
		Token:   "concurrent-token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	// Launch multiple goroutines to access the same provider
	const numGoroutines = 10
	clients := make([]*openai.Client, numGoroutines)

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			client := pool.GetClient(provider)
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
	pool := NewClientPool()

	// Add some clients
	provider1 := &config.Provider{
		Name:    "provider1",
		Token:   "token1-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	provider2 := &config.Provider{
		Name:    "provider2",
		Token:   "token2-87654321",
		APIBase: "https://api.openai.com/v1",
	}

	pool.GetClient(provider1)
	pool.GetClient(provider2)

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
	pool := NewClientPool()

	provider1 := &config.Provider{
		Name:    "provider1",
		Token:   "token1-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	provider2 := &config.Provider{
		Name:    "provider2",
		Token:   "token2-87654321",
		APIBase: "https://api.openai.com/v1",
	}

	// Add clients
	pool.GetClient(provider1)
	pool.GetClient(provider2)

	// Verify pool size
	if pool.Size() != 2 {
		t.Errorf("Expected pool size 2 before removal, got %d", pool.Size())
	}

	// Remove one provider
	pool.RemoveProvider(provider1)

	// Verify pool size decreased
	if pool.Size() != 1 {
		t.Errorf("Expected pool size 1 after removal, got %d", pool.Size())
	}

	// Verify remaining client is for provider2
	client := pool.GetClient(provider2)
	if client == nil {
		t.Error("Expected provider2 client to still exist")
	}
}

func TestClientPool_Stats(t *testing.T) {
	pool := NewClientPool()

	provider := &config.Provider{
		Name:    "stats-provider",
		Token:   "stats-token-12345678",
		APIBase: "https://api.openai.com/v1",
	}

	// Add a client
	pool.GetClient(provider)

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
}
