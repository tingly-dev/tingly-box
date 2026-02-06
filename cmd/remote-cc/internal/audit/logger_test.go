package audit

import (
	"testing"
	"time"
)

func TestLogger_Log(t *testing.T) {
	logger := NewLogger(Config{
		Console:    false,
		MaxEntries: 100,
	})

	// Log an entry
	logger.Log(Entry{
		Level:   LevelInfo,
		Action:  "test_action",
		UserID:  "test_user",
		Success: true,
	})

	if logger.Size() != 1 {
		t.Errorf("Expected size 1, got %d", logger.Size())
	}

	// Check the entry
	entries := logger.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].Action != "test_action" {
		t.Errorf("Expected action 'test_action', got '%s'", entries[0].Action)
	}
}

func TestLogger_Info(t *testing.T) {
	logger := NewLogger(Config{
		Console:    false,
		MaxEntries: 100,
	})

	logger.Info("handshake", "user1", "192.168.1.1", "New session created", nil)

	entries := logger.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].Level != LevelInfo {
		t.Errorf("Expected level 'info', got '%s'", entries[0].Level)
	}
}

func TestLogger_Error(t *testing.T) {
	logger := NewLogger(Config{
		Console:    false,
		MaxEntries: 100,
	})

	logger.Error("execute", "user1", "192.168.1.1", "Execution failed", false, nil)

	entries := logger.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].Level != LevelError {
		t.Errorf("Expected level 'error', got '%s'", entries[0].Level)
	}

	if entries[0].Success != false {
		t.Error("Expected success to be false")
	}
}

func TestLogger_LogRequest(t *testing.T) {
	logger := NewLogger(Config{
		Console:    false,
		MaxEntries: 100,
	})

	logger.LogRequest("execute", "user1", "192.168.1.1", "session123", "req456", true, 100*time.Millisecond, nil)

	entries := logger.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].DurationMs != 100 {
		t.Errorf("Expected duration_ms 100, got %d", entries[0].DurationMs)
	}
}

func TestLogger_GetEntriesByUser(t *testing.T) {
	logger := NewLogger(Config{
		Console:    false,
		MaxEntries: 100,
	})

	logger.Info("action1", "user1", "192.168.1.1", "msg1", nil)
	logger.Info("action2", "user2", "192.168.1.2", "msg2", nil)
	logger.Info("action3", "user1", "192.168.1.1", "msg3", nil)

	entries := logger.GetEntriesByUser("user1")
	if len(entries) != 2 {
		t.Errorf("Expected 2 entries for user1, got %d", len(entries))
	}
}

func TestLogger_GetEntriesBySession(t *testing.T) {
	logger := NewLogger(Config{
		Console:    false,
		MaxEntries: 100,
	})

	logger.LogRequest("execute", "user1", "192.168.1.1", "session1", "req1", true, 100*time.Millisecond, nil)
	logger.LogRequest("execute", "user2", "192.168.1.2", "session2", "req2", true, 100*time.Millisecond, nil)
	logger.LogRequest("status", "user1", "192.168.1.1", "session1", "req3", true, 50*time.Millisecond, nil)

	entries := logger.GetEntriesBySession("session1")
	if len(entries) != 2 {
		t.Errorf("Expected 2 entries for session1, got %d", len(entries))
	}
}

func TestLogger_Clear(t *testing.T) {
	logger := NewLogger(Config{
		Console:    false,
		MaxEntries: 100,
	})

	logger.Info("action1", "user1", "192.168.1.1", "msg1", nil)
	logger.Info("action2", "user2", "192.168.1.2", "msg2", nil)

	if logger.Size() != 2 {
		t.Errorf("Expected size 2, got %d", logger.Size())
	}

	logger.Clear()

	if logger.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", logger.Size())
	}
}

func TestLogger_MaxEntries(t *testing.T) {
	logger := NewLogger(Config{
		Console:    false,
		MaxEntries: 5,
	})

	// Add more than max entries
	for i := 0; i < 10; i++ {
		logger.Info("action", "user", "ip", "msg", nil)
	}

	// Should only keep last 5
	if logger.Size() != 5 {
		t.Errorf("Expected size 5, got %d", logger.Size())
	}
}

func TestLogger_ExportJSON(t *testing.T) {
	logger := NewLogger(Config{
		Console:    false,
		MaxEntries: 100,
	})

	logger.Info("action1", "user1", "192.168.1.1", "msg1", nil)

	data, err := logger.ExportJSON()
	if err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Expected non-empty JSON output")
	}

	// Check it starts with [
	if data[0] != '[' {
		t.Errorf("Expected JSON array, got: %c", data[0])
	}
}
