package audit

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/remote_control/audit"
)

func TestNewLogger(t *testing.T) {
	logger := NewLogger(audit.Config{
		Console:    true,
		MaxEntries: 100,
	})

	if logger == nil {
		t.Fatal("NewLogger() returned nil")
	}
}

func TestLogger_LogPairingAttempt(t *testing.T) {
	logger := NewLogger(audit.Config{Console: false})

	// This should not panic
	logger.LogPairingAttempt("bot-123", "chat-456", "user-789", "telegram", "ABCD1234")
}

func TestLogger_LogPairingSuccess(t *testing.T) {
	logger := NewLogger(audit.Config{Console: false})

	// This should not panic
	logger.LogPairingSuccess("bot-123", "chat-456", "user-789", "telegram")
}

func TestLogger_LogPairingFailure(t *testing.T) {
	logger := NewLogger(audit.Config{Console: false})

	// This should not panic
	logger.LogPairingFailure("bot-123", "chat-456", "user-789", "telegram", "code expired")
}

func TestLogger_LogPermissionRequest(t *testing.T) {
	logger := NewLogger(audit.Config{Console: false})

	// This should not panic
	logger.LogPermissionRequest("bot-123", "chat-456", "bash_tool", "req-999")
}

func TestLogger_LogPermissionApproved(t *testing.T) {
	logger := NewLogger(audit.Config{Console: false})

	// This should not panic
	logger.LogPermissionApproved("bot-123", "chat-456", "bash_tool", "req-999", true)
}

func TestLogger_LogPermissionDenied(t *testing.T) {
	logger := NewLogger(audit.Config{Console: false})

	// This should not panic
	logger.LogPermissionDenied("bot-123", "chat-456", "bash_tool", "req-999", "user denied")
}

func TestLogger_LogUnpairedMessageRejected(t *testing.T) {
	logger := NewLogger(audit.Config{Console: false})

	// This should not panic
	logger.LogUnpairedMessageRejected("bot-123", "chat-456", "user-789", "telegram")
}

func TestLogger_LogWhitelistAdd(t *testing.T) {
	logger := NewLogger(audit.Config{Console: false})

	// This should not panic
	logger.LogWhitelistAdd("bot-123", "group-456", "user-789", "telegram")
}

func TestLogger_LogWhitelistRemove(t *testing.T) {
	logger := NewLogger(audit.Config{Console: false})

	// This should not panic
	logger.LogWhitelistRemove("bot-123", "group-456", "user-789", "telegram")
}

func TestLogger_GetLogger(t *testing.T) {
	logger := NewLogger(audit.Config{Console: false})

	if logger.GetLogger() == nil {
		t.Error("GetLogger() returned nil")
	}
}

func TestConsoleOnly(t *testing.T) {
	logger := ConsoleOnly()

	if logger == nil {
		t.Fatal("ConsoleOnly() returned nil")
	}

	if logger.GetLogger() == nil {
		t.Error("ConsoleOnly().GetLogger() returned nil")
	}
}

func TestNow(t *testing.T) {
	now := Now()

	if now.IsZero() {
		t.Error("Now() returned zero time")
	}

	// Check that it's within a reasonable range (last minute)
	if time.Since(now) > time.Minute {
		t.Error("Now() returned a time too far in the past")
	}
}
