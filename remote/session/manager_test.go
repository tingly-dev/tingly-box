package session

import (
	"testing"
	"time"
)

func TestManager_Create(t *testing.T) {
	cfg := Config{Timeout: 30 * time.Minute}
	mgr := NewManager(cfg, nil)
	defer mgr.Stop()

	session := mgr.Create()

	if session == nil {
		t.Fatal("Expected session to be created")
	}

	if session.ID == "" {
		t.Error("Expected session ID to be non-empty")
	}

	if session.Status != StatusPending {
		t.Errorf("Expected status to be %s, got %s", StatusPending, session.Status)
	}

	if session.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}

	if session.ExpiresAt.Before(session.CreatedAt) {
		t.Error("Expected ExpiresAt to be after CreatedAt")
	}
}

func TestManager_Get(t *testing.T) {
	cfg := Config{Timeout: 30 * time.Minute}
	mgr := NewManager(cfg, nil)
	defer mgr.Stop()

	// Create a session
	created := mgr.Create()

	// Get the session
	retrieved, exists := mgr.Get(created.ID)

	if !exists {
		t.Fatal("Expected session to exist")
	}

	if retrieved.ID != created.ID {
		t.Errorf("Expected ID %s, got %s", created.ID, retrieved.ID)
	}
}

func TestManager_Get_NotFound(t *testing.T) {
	cfg := Config{Timeout: 30 * time.Minute}
	mgr := NewManager(cfg, nil)
	defer mgr.Stop()

	_, exists := mgr.Get("non-existent-id")

	if exists {
		t.Error("Expected session not to exist")
	}
}

func TestManager_Update(t *testing.T) {
	cfg := Config{Timeout: 30 * time.Minute}
	mgr := NewManager(cfg, nil)
	defer mgr.Stop()

	session := mgr.Create()

	// Update the session
	updated := mgr.Update(session.ID, func(s *Session) {
		s.Status = StatusRunning
	})

	if !updated {
		t.Error("Expected update to succeed")
	}

	// Verify the update
	retrieved, _ := mgr.Get(session.ID)
	if retrieved.Status != StatusRunning {
		t.Errorf("Expected status %s, got %s", StatusRunning, retrieved.Status)
	}
}

func TestManager_Close(t *testing.T) {
	cfg := Config{Timeout: 30 * time.Minute}
	mgr := NewManager(cfg, nil)
	defer mgr.Stop()

	session := mgr.Create()

	// Close the session
	closed := mgr.Close(session.ID)

	if !closed {
		t.Error("Expected close to succeed")
	}

	// Verify the session is closed
	_, exists := mgr.Get(session.ID)
	if exists {
		t.Error("Expected session to be deleted after close")
	}
}

func TestManager_SetRunning(t *testing.T) {
	cfg := Config{Timeout: 30 * time.Minute}
	mgr := NewManager(cfg, nil)
	defer mgr.Stop()

	session := mgr.Create()

	mgr.SetRunning(session.ID)

	retrieved, _ := mgr.Get(session.ID)
	if retrieved.Status != StatusRunning {
		t.Errorf("Expected status %s, got %s", StatusRunning, retrieved.Status)
	}
}

func TestManager_SetCompleted(t *testing.T) {
	cfg := Config{Timeout: 30 * time.Minute}
	mgr := NewManager(cfg, nil)
	defer mgr.Stop()

	session := mgr.Create()

	mgr.SetCompleted(session.ID, "Test summary")

	retrieved, _ := mgr.Get(session.ID)
	if retrieved.Status != StatusCompleted {
		t.Errorf("Expected status %s, got %s", StatusCompleted, retrieved.Status)
	}

	if retrieved.Response != "Test summary" {
		t.Errorf("Expected response 'Test summary', got '%s'", retrieved.Response)
	}
}

func TestManager_SetFailed(t *testing.T) {
	cfg := Config{Timeout: 30 * time.Minute}
	mgr := NewManager(cfg, nil)
	defer mgr.Stop()

	session := mgr.Create()

	mgr.SetFailed(session.ID, "Test error")

	retrieved, _ := mgr.Get(session.ID)
	if retrieved.Status != StatusFailed {
		t.Errorf("Expected status %s, got %s", StatusFailed, retrieved.Status)
	}

	if retrieved.Error != "Test error" {
		t.Errorf("Expected error 'Test error', got '%s'", retrieved.Error)
	}
}

func TestManager_SetRequest(t *testing.T) {
	cfg := Config{Timeout: 30 * time.Minute}
	mgr := NewManager(cfg, nil)
	defer mgr.Stop()

	session := mgr.Create()

	mgr.SetRequest(session.ID, "Test request")

	request, exists := mgr.GetRequest(session.ID)
	if !exists {
		t.Error("Expected request to exist")
	}

	if request != "Test request" {
		t.Errorf("Expected request 'Test request', got '%s'", request)
	}
}

func TestManager_SetContext(t *testing.T) {
	cfg := Config{Timeout: 30 * time.Minute}
	mgr := NewManager(cfg, nil)
	defer mgr.Stop()

	session := mgr.Create()

	mgr.SetContext(session.ID, "key1", "value1")
	mgr.SetContext(session.ID, "key2", 123)

	value1, exists := mgr.GetContext(session.ID, "key1")
	if !exists {
		t.Error("Expected context key1 to exist")
	}

	if value1 != "value1" {
		t.Errorf("Expected value1 'value1', got '%v'", value1)
	}

	value2, _ := mgr.GetContext(session.ID, "key2")
	if value2 != 123 {
		t.Errorf("Expected value2 123, got '%v'", value2)
	}
}

func TestManager_Stats(t *testing.T) {
	cfg := Config{Timeout: 30 * time.Minute}
	mgr := NewManager(cfg, nil)
	defer mgr.Stop()

	// Create multiple sessions
	mgr.Create()
	mgr.Create()
	session := mgr.Create()
	mgr.SetRunning(session.ID)

	stats := mgr.Stats()

	if stats[string(StatusPending)] != 2 {
		t.Errorf("Expected 2 pending sessions, got %d", stats[string(StatusPending)])
	}

	if stats[string(StatusRunning)] != 1 {
		t.Errorf("Expected 1 running session, got %d", stats[string(StatusRunning)])
	}
}

func TestSessionExpiry(t *testing.T) {
	cfg := Config{Timeout: 1 * time.Second}
	mgr := NewManager(cfg, nil)
	defer mgr.Stop()

	session := mgr.Create()

	// Wait for session to expire
	time.Sleep(2 * time.Second)

	// Manually trigger cleanup
	mgr.cleanupExpired()

	// Session should be expired
	_, exists := mgr.Get(session.ID)
	if exists {
		t.Error("Expected session to be expired and cleaned up")
	}
}
