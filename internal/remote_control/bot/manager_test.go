package bot

import (
	"testing"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// MockStore is a mock implementation of SettingsStore for testing
type MockStore struct{}

func (m *MockStore) GetSettingsByUUIDInterface(uuid string) (interface{}, error) {
	return db.Settings{
		UUID:     uuid,
		Name:     "Test Bot",
		Platform: "telegram",
		Auth: map[string]string{
			"token": "test-token",
		},
		AuthType: "token",
		Enabled:  true,
	}, nil
}

func (m *MockStore) ListEnabledSettingsInterface() (interface{}, error) {
	return []db.Settings{}, nil
}

func TestRunningBotFields(t *testing.T) {
	// Test that runningBot struct has the new fields
	rb := &runningBot{
		imbotMgr: imbot.NewManager(),
		botUUID:  "test-uuid",
	}

	if rb.imbotMgr == nil {
		t.Error("imbotMgr should not be nil")
	}

	if rb.botUUID != "test-uuid" {
		t.Errorf("botUUID should be 'test-uuid', got '%s'", rb.botUUID)
	}
}

func TestSendMessage_BotNotRunning(t *testing.T) {
	store := &MockStore{}
	manager := NewManager(store, nil, nil)

	// Try to send a message when no bot is running
	err := manager.SendMessage("non-existent-uuid", "chat-id", "test message")
	if err == nil {
		t.Error("Expected error when bot is not running")
	}

	if err.Error() != "bot non-existent-uuid is not running" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestSendMessage_BotRunningManagerNil(t *testing.T) {
	// This test would require a more complex setup with actual bot running
	// For now, we just verify the method signature is correct
	store := &MockStore{}
	manager := NewManager(store, nil, nil)

	// Manually create a runningBot with nil imbotMgr to test that code path
	manager.mu.Lock()
	manager.running["test-uuid"] = &runningBot{
		imbotMgr: nil,
		botUUID:  "test-uuid",
		stopped:  false,
		cancel:   func() {},
		doneChan: make(chan struct{}),
	}
	manager.mu.Unlock()

	err := manager.SendMessage("test-uuid", "chat-id", "test message")
	if err == nil {
		t.Error("Expected error when imbotMgr is nil")
	}

	expectedErr := "bot manager not initialized for test-uuid"
	if err.Error() != expectedErr {
		t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestManager_StoresImbotManagerReference(t *testing.T) {
	// This is an integration-style test that verifies the structure is correct
	// In a real scenario, we'd need to mock the imbot.Manager behavior

	store := &MockStore{}
	manager := NewManager(store, nil, nil)

	// Verify manager is created
	if manager == nil {
		t.Fatal("Manager should not be nil")
	}

	// Verify running map is initialized
	if manager.running == nil {
		t.Error("running map should be initialized")
	}
}
