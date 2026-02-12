package oauth

import (
	"testing"
	"time"
)

// TestMemorySessionStorage tests the in-memory session storage implementation
func TestMemorySessionStorage(t *testing.T) {
	storage := NewMemorySessionStorage()

	sessionID := "test-session-123"
	session := &SessionState{
		SessionID: sessionID,
		Status:    SessionStatusPending,
		Provider:  ProviderClaudeCode,
		UserID:    "user123",
	}

	t.Run("SaveSession", func(t *testing.T) {
		err := storage.SaveSession(sessionID, session)
		if err != nil {
			t.Fatalf("SaveSession failed: %v", err)
		}
	})

	t.Run("GetSession", func(t *testing.T) {
		retrieved, err := storage.GetSession(sessionID)
		if err != nil {
			t.Fatalf("GetSession failed: %v", err)
		}
		if retrieved.SessionID != session.SessionID {
			t.Errorf("Expected sessionID %s, got %s", session.SessionID, retrieved.SessionID)
		}
		if retrieved.Status != session.Status {
			t.Errorf("Expected status %s, got %s", session.Status, retrieved.Status)
		}
		if retrieved.UserID != session.UserID {
			t.Errorf("Expected userID %s, got %s", session.UserID, retrieved.UserID)
		}
		// Verify CreatedAt was set
		if retrieved.CreatedAt.IsZero() {
			t.Error("Expected CreatedAt to be set")
		}
		// Verify ExpiresAt was set
		if retrieved.ExpiresAt.IsZero() {
			t.Error("Expected ExpiresAt to be set")
		}
	})

	t.Run("GetSessionNotFound", func(t *testing.T) {
		_, err := storage.GetSession("unknown-session")
		if err != ErrSessionNotFound {
			t.Errorf("Expected ErrSessionNotFound, got %v", err)
		}
	})

	t.Run("DeleteSession", func(t *testing.T) {
		err := storage.DeleteSession(sessionID)
		if err != nil {
			t.Fatalf("DeleteSession failed: %v", err)
		}
		// Verify session is deleted
		_, err = storage.GetSession(sessionID)
		if err != ErrSessionNotFound {
			t.Errorf("Expected ErrSessionNotFound after delete, got %v", err)
		}
	})
}

// TestMemorySessionStorageUpdateStatus tests updating session status
func TestMemorySessionStorageUpdateStatus(t *testing.T) {
	storage := NewMemorySessionStorage()

	sessionID := "test-session-456"
	session := &SessionState{
		SessionID: sessionID,
		Status:    SessionStatusPending,
		Provider:  ProviderClaudeCode,
		UserID:    "user123",
	}

	err := storage.SaveSession(sessionID, session)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	t.Run("UpdateStatusToSuccess", func(t *testing.T) {
		err := storage.UpdateSessionStatus(sessionID, SessionStatusSuccess, "provider-uuid-123", "")
		if err != nil {
			t.Fatalf("UpdateSessionStatus failed: %v", err)
		}

		// Verify update
		retrieved, _ := storage.GetSession(sessionID)
		if retrieved.Status != SessionStatusSuccess {
			t.Errorf("Expected status %s, got %s", SessionStatusSuccess, retrieved.Status)
		}
		if retrieved.ProviderUUID != "provider-uuid-123" {
			t.Errorf("Expected ProviderUUID provider-uuid-123, got %s", retrieved.ProviderUUID)
		}
	})

	t.Run("UpdateStatusToFailed", func(t *testing.T) {
		err := storage.UpdateSessionStatus(sessionID, SessionStatusFailed, "", "authorization failed")
		if err != nil {
			t.Fatalf("UpdateSessionStatus failed: %v", err)
		}

		// Verify update
		retrieved, _ := storage.GetSession(sessionID)
		if retrieved.Status != SessionStatusFailed {
			t.Errorf("Expected status %s, got %s", SessionStatusFailed, retrieved.Status)
		}
		if retrieved.Error != "authorization failed" {
			t.Errorf("Expected Error 'authorization failed', got %s", retrieved.Error)
		}
	})

	t.Run("UpdateSessionNotFound", func(t *testing.T) {
		err := storage.UpdateSessionStatus("unknown-session", SessionStatusSuccess, "uuid", "")
		if err != ErrSessionNotFound {
			t.Errorf("Expected ErrSessionNotFound, got %v", err)
		}
	})
}

// TestMemorySessionStorageCleanup tests cleanup of expired sessions
func TestMemorySessionStorageCleanup(t *testing.T) {
	storage := NewMemorySessionStorage()

	// Add some sessions
	now := time.Now()
	sessions := []struct {
		sessionID string
		session   *SessionState
	}{
		{
			sessionID: "valid-session-1",
			session: &SessionState{
				SessionID: "valid-session-1",
				Status:    SessionStatusSuccess,
				Provider:  ProviderClaudeCode,
				UserID:    "user1",
				CreatedAt: now,
				ExpiresAt: now.Add(1 * time.Hour),
			},
		},
		{
			sessionID: "expired-session-1",
			session: &SessionState{
				SessionID: "expired-session-1",
				Status:    SessionStatusPending,
				Provider:  ProviderOpenAI,
				UserID:    "user2",
				CreatedAt: now.Add(-2 * time.Hour),
				ExpiresAt: now.Add(-1 * time.Hour),
			},
		},
		{
			sessionID: "expired-session-2",
			session: &SessionState{
				SessionID: "expired-session-2",
				Status:    SessionStatusFailed,
				Provider:  ProviderGemini,
				UserID:    "user3",
				CreatedAt: now.Add(-10 * time.Minute),
				ExpiresAt: now.Add(-5 * time.Minute),
			},
		},
	}

	for _, s := range sessions {
		err := storage.SaveSession(s.sessionID, s.session)
		if err != nil {
			t.Fatalf("SaveSession failed for %s: %v", s.sessionID, err)
		}
	}

	// Verify count before cleanup
	if storage.Count() != 3 {
		t.Errorf("Expected 3 sessions before cleanup, got %d", storage.Count())
	}

	// Run cleanup
	err := storage.CleanupExpired()
	if err != nil {
		t.Fatalf("CleanupExpired failed: %v", err)
	}

	// Verify count after cleanup
	if storage.Count() != 1 {
		t.Errorf("Expected 1 session after cleanup, got %d", storage.Count())
	}

	// Verify valid session still exists
	_, err = storage.GetSession("valid-session-1")
	if err != nil {
		t.Errorf("Valid session should still exist: %v", err)
	}

	// Verify expired sessions are gone
	_, err = storage.GetSession("expired-session-1")
	if err != ErrSessionNotFound {
		t.Errorf("Expired session 1 should be removed, got error: %v", err)
	}
	_, err = storage.GetSession("expired-session-2")
	if err != ErrSessionNotFound {
		t.Errorf("Expired session 2 should be removed, got error: %v", err)
	}
}

// TestMemorySessionStorageWithNilExpiry tests sessions with zero expiry
func TestMemorySessionStorageWithNilExpiry(t *testing.T) {
	storage := NewMemorySessionStorage()

	sessionID := "no-expiry-session"
	session := &SessionState{
		SessionID: sessionID,
		Status:    SessionStatusPending,
		Provider:  ProviderClaudeCode,
		UserID:    "user123",
		// ExpiresAt will be set by SaveSession to default
	}

	err := storage.SaveSession(sessionID, session)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Retrieve and verify default expiry was set
	retrieved, err := storage.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if retrieved.ExpiresAt.IsZero() {
		t.Error("Expected ExpiresAt to be set to default (1 hour)")
	}
}
