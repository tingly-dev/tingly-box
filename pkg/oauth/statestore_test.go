package oauth

import (
	"testing"
	"time"
)

// TestMemoryStateStorage tests the in-memory state storage implementation
func TestMemoryStateStorage(t *testing.T) {
	storage := NewMemoryStateStorage()

	state := "test-state-123"
	data := &StateData{
		State:     state,
		UserID:    "user123",
		Provider:   ProviderClaudeCode,
		RedirectTo: "http://example.com/callback",
		Name:       "My Provider",
		SessionID:   "session-456",
	}

	t.Run("SaveState", func(t *testing.T) {
		err := storage.SaveState(state, data)
		if err != nil {
			t.Fatalf("SaveState failed: %v", err)
		}
	})

	t.Run("GetState", func(t *testing.T) {
		retrieved, err := storage.GetState(state)
		if err != nil {
			t.Fatalf("GetState failed: %v", err)
		}
		if retrieved.State != data.State {
			t.Errorf("Expected state %s, got %s", data.State, retrieved.State)
		}
		if retrieved.UserID != data.UserID {
			t.Errorf("Expected userID %s, got %s", data.UserID, retrieved.UserID)
		}
		if retrieved.Provider != data.Provider {
			t.Errorf("Expected provider %s, got %s", data.Provider, retrieved.Provider)
		}
		// Verify ExpiresAt was set
		if retrieved.ExpiresAt.IsZero() {
			t.Error("Expected ExpiresAt to be set")
		}
		// Verify Timestamp was set
		if retrieved.Timestamp == 0 {
			t.Error("Expected Timestamp to be set")
		}
	})

	t.Run("GetStateNotFound", func(t *testing.T) {
		_, err := storage.GetState("unknown-state")
		if err != ErrInvalidState {
			t.Errorf("Expected ErrInvalidState, got %v", err)
		}
	})

	t.Run("DeleteState", func(t *testing.T) {
		err := storage.DeleteState(state)
		if err != nil {
			t.Fatalf("DeleteState failed: %v", err)
		}
		// Verify state is deleted
		_, err = storage.GetState(state)
		if err != ErrInvalidState {
			t.Errorf("Expected ErrInvalidState after delete, got %v", err)
		}
	})
}

// TestMemoryStateStorageExpiry tests state expiration
func TestMemoryStateStorageExpiry(t *testing.T) {
	storage := NewMemoryStateStorage()

	state := "expiring-state"
	data := &StateData{
		State:    state,
		UserID:   "user123",
		Provider: ProviderClaudeCode,
	}

	// Save with short expiry
	data.ExpiresAt = time.Now().Add(10 * time.Millisecond)
	err := storage.SaveState(state, data)
	if err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(20 * time.Millisecond)

	// Try to get expired state
	_, err = storage.GetState(state)
	if err != ErrStateExpired {
		t.Errorf("Expected ErrStateExpired, got %v", err)
	}
}

// TestMemoryStateStorageCleanup tests cleanup of expired states
func TestMemoryStateStorageCleanup(t *testing.T) {
	storage := NewMemoryStateStorage()

	// Add some states
	now := time.Now()
	states := []struct {
		state string
		data  *StateData
	}{
		{
			state: "valid-state-1",
			data: &StateData{
				State:     "valid-state-1",
				UserID:    "user1",
				Provider:  ProviderClaudeCode,
				ExpiresAt: now.Add(1 * time.Hour),
			},
		},
		{
			state: "expired-state-1",
			data: &StateData{
				State:     "expired-state-1",
				UserID:    "user2",
				Provider:  ProviderOpenAI,
				ExpiresAt: now.Add(-1 * time.Hour),
			},
		},
		{
			state: "expired-state-2",
			data: &StateData{
				State:     "expired-state-2",
				UserID:    "user3",
				Provider:  ProviderGemini,
				ExpiresAt: now.Add(-1 * time.Minute),
			},
		},
	}

	for _, s := range states {
		err := storage.SaveState(s.state, s.data)
		if err != nil {
			t.Fatalf("SaveState failed for %s: %v", s.state, err)
		}
	}

	// Verify count before cleanup
	if storage.Count() != 3 {
		t.Errorf("Expected 3 states before cleanup, got %d", storage.Count())
	}

	// Run cleanup
	err := storage.CleanupExpired()
	if err != nil {
		t.Fatalf("CleanupExpired failed: %v", err)
	}

	// Verify count after cleanup
	if storage.Count() != 1 {
		t.Errorf("Expected 1 state after cleanup, got %d", storage.Count())
	}

	// Verify valid state still exists
	_, err = storage.GetState("valid-state-1")
	if err != nil {
		t.Errorf("Valid state should still exist: %v", err)
	}

	// Verify expired states are gone
	_, err = storage.GetState("expired-state-1")
	if err != ErrInvalidState {
		t.Errorf("Expired state 1 should be removed, got error: %v", err)
	}
	_, err = storage.GetState("expired-state-2")
	if err != ErrInvalidState {
		t.Errorf("Expired state 2 should be removed, got error: %v", err)
	}
}
