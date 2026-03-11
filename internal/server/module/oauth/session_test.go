package oauth

import (
	"testing"
	"time"
)

func TestNewSessionManager(t *testing.T) {
	sm := NewSessionManager()
	if sm == nil {
		t.Fatal("expected non-nil session manager")
	}
	if sm.sessions == nil {
		t.Error("expected sessions map to be initialized")
	}
}

func TestCreateSession(t *testing.T) {
	sm := NewSessionManager()

	sessionID := sm.CreateSession("anthropic", "user123", "http://localhost:3000", "json", "my-provider", "")
	if sessionID == "" {
		t.Fatal("expected non-empty session ID")
	}

	session := sm.GetSession(sessionID)
	if session == nil {
		t.Fatal("expected session to be created")
	}

	if session.SessionID != sessionID {
		t.Errorf("expected SessionID %q, got %q", sessionID, session.SessionID)
	}
	if session.Provider != "anthropic" {
		t.Errorf("expected Provider 'anthropic', got %q", session.Provider)
	}
	if session.UserID != "user123" {
		t.Errorf("expected UserID 'user123', got %q", session.UserID)
	}
	if session.Redirect != "http://localhost:3000" {
		t.Errorf("expected Redirect 'http://localhost:3000', got %q", session.Redirect)
	}
	if session.ResponseType != "json" {
		t.Errorf("expected ResponseType 'json', got %q", session.ResponseType)
	}
	if session.Name != "my-provider" {
		t.Errorf("expected Name 'my-provider', got %q", session.Name)
	}
	if session.Status != "pending" {
		t.Errorf("expected Status 'pending', got %q", session.Status)
	}
	if time.Since(session.CreatedAt) > time.Second {
		t.Error("expected CreatedAt to be recent")
	}
	if time.Until(session.ExpiresAt) < 29*time.Minute {
		t.Error("expected ExpiresAt to be ~30 minutes from now")
	}
}

func TestCreateSessionWithProxy(t *testing.T) {
	sm := NewSessionManager()

	proxyURL := "http://127.0.0.1:7890"
	sessionID := sm.CreateSession("qwen", "user456", "http://localhost:3000", "redirect", "qwen-provider", proxyURL)

	session := sm.GetSession(sessionID)
	if session == nil {
		t.Fatal("expected session to be created")
	}

	if session.ProxyURL != proxyURL {
		t.Errorf("expected ProxyURL %q, got %q", proxyURL, session.ProxyURL)
	}
}

func TestGetSession(t *testing.T) {
	sm := NewSessionManager()

	// Test non-existent session
	session := sm.GetSession("non-existent")
	if session != nil {
		t.Error("expected nil for non-existent session")
	}

	// Test existing session
	sessionID := sm.CreateSession("anthropic", "user123", "", "", "", "")
	session = sm.GetSession(sessionID)
	if session == nil {
		t.Error("expected session to be found")
	}
}

func TestGetSessionExpired(t *testing.T) {
	sm := NewSessionManager()

	// Create a session with an already expired time
	sm.mu.Lock()
	sessionID := "test-expired-session"
	sm.sessions[sessionID] = &Session{
		SessionID: sessionID,
		Provider:  "anthropic",
		Status:    "pending",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired
	}
	sm.mu.Unlock()

	// GetSession should return nil for expired sessions
	session := sm.GetSession(sessionID)
	if session != nil {
		t.Error("expected nil for expired session")
	}
}

func TestCompleteSession(t *testing.T) {
	sm := NewSessionManager()

	sessionID := sm.CreateSession("anthropic", "user123", "", "", "", "")
	providerUUID := "provider-uuid-123"

	sm.CompleteSession(sessionID, providerUUID)

	session := sm.GetSession(sessionID)
	if session == nil {
		t.Fatal("expected session to exist")
	}

	if session.Status != "success" {
		t.Errorf("expected Status 'success', got %q", session.Status)
	}
	if session.ProviderUUID != providerUUID {
		t.Errorf("expected ProviderUUID %q, got %q", providerUUID, session.ProviderUUID)
	}
}

func TestFailSession(t *testing.T) {
	sm := NewSessionManager()

	sessionID := sm.CreateSession("anthropic", "user123", "", "", "", "")
	errorMsg := "authorization failed"

	sm.FailSession(sessionID, errorMsg)

	session := sm.GetSession(sessionID)
	if session == nil {
		t.Fatal("expected session to exist")
	}

	if session.Status != "failed" {
		t.Errorf("expected Status 'failed', got %q", session.Status)
	}
	if session.Error != errorMsg {
		t.Errorf("expected Error %q, got %q", errorMsg, session.Error)
	}
}

func TestGetStatus(t *testing.T) {
	sm := NewSessionManager()

	// Test non-existent session
	status := sm.GetStatus("non-existent")
	if status.Status != "not_found" {
		t.Errorf("expected Status 'not_found', got %q", status.Status)
	}
	if status.SessionID != "non-existent" {
		t.Errorf("expected SessionID 'non-existent', got %q", status.SessionID)
	}

	// Test pending session
	sessionID := sm.CreateSession("anthropic", "user123", "", "", "", "")
	status = sm.GetStatus(sessionID)
	if status.Status != "pending" {
		t.Errorf("expected Status 'pending', got %q", status.Status)
	}
	if status.SessionID != sessionID {
		t.Errorf("expected SessionID %q, got %q", sessionID, status.SessionID)
	}

	// Test success session
	sm.CompleteSession(sessionID, "provider-uuid")
	status = sm.GetStatus(sessionID)
	if status.Status != "success" {
		t.Errorf("expected Status 'success', got %q", status.Status)
	}
	if status.ProviderUUID != "provider-uuid" {
		t.Errorf("expected ProviderUUID 'provider-uuid', got %q", status.ProviderUUID)
	}

	// Test failed session
	sessionID2 := sm.CreateSession("anthropic", "user456", "", "", "", "")
	sm.FailSession(sessionID2, "error message")
	status = sm.GetStatus(sessionID2)
	if status.Status != "failed" {
		t.Errorf("expected Status 'failed', got %q", status.Status)
	}
	if status.Error != "error message" {
		t.Errorf("expected Error 'error message', got %q", status.Error)
	}

	// Test expired session
	sm.mu.Lock()
	expiredSessionID := "test-expired-status"
	sm.sessions[expiredSessionID] = &Session{
		SessionID: expiredSessionID,
		Provider:  "anthropic",
		Status:    "pending",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	sm.mu.Unlock()

	status = sm.GetStatus(expiredSessionID)
	if status.Status != "expired" {
		t.Errorf("expected Status 'expired', got %q", status.Status)
	}
}

func TestCancelSession(t *testing.T) {
	sm := NewSessionManager()

	sessionID := sm.CreateSession("anthropic", "user123", "", "", "", "")

	// Cancel the session
	cancelled := sm.CancelSession(sessionID)
	if !cancelled {
		t.Error("expected CancelSession to return true")
	}

	// Verify status is cancelled
	status := sm.GetStatus(sessionID)
	if status.Status != "cancelled" {
		t.Errorf("expected Status 'cancelled', got %q", status.Status)
	}

	// Try cancelling again - should return false
	cancelled = sm.CancelSession(sessionID)
	if cancelled {
		t.Error("expected second CancelSession to return false")
	}

	// Try cancelling non-existent session
	cancelled = sm.CancelSession("non-existent")
	if cancelled {
		t.Error("expected CancelSession of non-existent to return false")
	}
}

func TestSessionExpiration(t *testing.T) {
	sm := NewSessionManager()

	sessionID := sm.CreateSession("anthropic", "user123", "", "", "", "")

	// Session should be accessible immediately
	session := sm.GetSession(sessionID)
	if session == nil {
		t.Fatal("expected session to exist immediately")
	}

	// Create a session with very short expiration for testing
	sm.mu.Lock()
	shortSessionID := "short-lived"
	sm.sessions[shortSessionID] = &Session{
		SessionID: shortSessionID,
		Provider:  "anthropic",
		Status:    "pending",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(-1 * time.Second), // Already expired
	}
	sm.mu.Unlock()

	// Should not be able to get expired session
	session = sm.GetSession(shortSessionID)
	if session != nil {
		t.Error("expected nil for expired session")
	}
}

func TestMultipleSessions(t *testing.T) {
	sm := NewSessionManager()

	// Create multiple sessions
	sessionID1 := sm.CreateSession("anthropic", "user1", "", "", "", "")
	sessionID2 := sm.CreateSession("qwen", "user2", "", "", "", "")
	sessionID3 := sm.CreateSession("openai", "user3", "", "", "", "")

	if sessionID1 == sessionID2 {
		t.Error("expected different session IDs")
	}
	if sessionID2 == sessionID3 {
		t.Error("expected different session IDs")
	}

	// Verify all sessions exist
	sessions := []string{sessionID1, sessionID2, sessionID3}
	for _, sid := range sessions {
		if sm.GetSession(sid) == nil {
			t.Errorf("expected session %q to exist", sid)
		}
	}

	// Complete one session
	sm.CompleteSession(sessionID1, "provider-1")
	if sm.GetStatus(sessionID1).Status != "success" {
		t.Error("expected session1 to be success")
	}

	// Fail another session
	sm.FailSession(sessionID2, "error")
	if sm.GetStatus(sessionID2).Status != "failed" {
		t.Error("expected session2 to be failed")
	}

	// Third should still be pending
	if sm.GetStatus(sessionID3).Status != "pending" {
		t.Error("expected session3 to be pending")
	}
}
