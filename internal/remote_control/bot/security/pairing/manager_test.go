package pairing

import (
	"testing"
	"time"
)

// mockAuditor implements security.PairingAuditor for testing
type mockAuditor struct {
	infoCalls [][]string
	warnCalls [][]string
}

func (m *mockAuditor) Info(action, userID, clientIP, message string, details map[string]interface{}) {
	m.infoCalls = append(m.infoCalls, []string{action, userID, message})
}

func (m *mockAuditor) Warn(action, userID, clientIP, message string, details map[string]interface{}) {
	m.warnCalls = append(m.warnCalls, []string{action, userID, message})
}

func TestNewManager(t *testing.T) {
	auditor := &mockAuditor{}
	manager := NewManager(auditor)

	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
}

func TestManagerWithTTL(t *testing.T) {
	auditor := &mockAuditor{}
	ttl := 5 * time.Minute

	manager := NewManager(auditor, WithTTL(ttl))

	if manager == nil {
		t.Fatal("NewManager(WithTTL()) returned nil")
	}
}

func TestManagerWithCodeLen(t *testing.T) {
	auditor := &mockAuditor{}
	codeLen := 6

	manager := NewManager(auditor, WithCodeLen(codeLen))

	if manager == nil {
		t.Fatal("NewManager(WithCodeLen()) returned nil")
	}
}

func TestManagerWithOptions(t *testing.T) {
	auditor := &mockAuditor{}

	manager := NewManager(auditor,
		WithTTL(10*time.Minute),
		WithCodeLen(8),
		WithMaxFails(3),
		WithLockout(5*time.Minute),
	)

	if manager == nil {
		t.Fatal("NewManager() with multiple options returned nil")
	}
}

func TestBotIntegration(t *testing.T) {
	auditor := &mockAuditor{}
	manager := NewManager(auditor)

	// Note: We can't create a full BotIntegration without an imbot.Manager
	// So we just test that NewManager creates a valid Manager
	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}

	// Test that we can get the manager from BotIntegration
	integration := &BotIntegration{
		manager: manager,
	}

	if integration.GetManager() == nil {
		t.Error("GetManager() returned nil")
	}
}
