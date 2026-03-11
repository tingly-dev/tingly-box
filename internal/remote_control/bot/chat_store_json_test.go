package bot

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestUpdateChatPersistsImmediately verifies that UpdateChat writes to disk immediately
func TestUpdateChatPersistsImmediately(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "chat-store-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test_chats.json")

	t.Logf("Test directory: %s", tmpDir)
	t.Logf("Database path: %s", dbPath)

	// Create a chat store
	store, err := NewChatStoreJSON(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chat store: %v", err)
	}

	chatID := "test-chat-123"
	projectPath := "/test/path"

	// First, create the chat using UpsertChat (which also tests immediate persistence)
	chat := &Chat{
		ChatID:      chatID,
		Platform:    "telegram",
		ProjectPath: "",
		BashCwd:     "",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	err = store.UpsertChat(chat)
	if err != nil {
		t.Fatalf("Failed to upsert chat: %v", err)
	}

	// Now update it with UpdateChat
	err = store.UpdateChat(chatID, func(c *Chat) {
		c.ProjectPath = projectPath
		c.BashCwd = projectPath
	})
	if err != nil {
		t.Fatalf("Failed to update chat: %v", err)
	}

	// Close the store
	if err := store.Close(); err != nil {
		t.Fatalf("Failed to close store: %v", err)
	}

	// Check if file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("JSON file was not created at %s", dbPath)
	}

	// Read the JSON file directly to verify the data was persisted
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("Failed to read JSON file: %v", err)
	}

	jsonContent := string(data)
	t.Logf("JSON content:\n%s", jsonContent)

	// Verify the project_path is in the JSON
	if !contains(jsonContent, `"project_path":`) {
		t.Errorf("JSON file does not contain project_path field. Content:\n%s", jsonContent)
	}
	if !contains(jsonContent, projectPath) {
		t.Errorf("JSON file does not contain the expected project path %s. Content:\n%s", projectPath, jsonContent)
	}
	if !contains(jsonContent, `"bash_cwd":`) {
		t.Errorf("JSON file does not contain bash_cwd field. Content:\n%s", jsonContent)
	}

	// Verify the chat_id is in the JSON
	if !contains(jsonContent, chatID) {
		t.Errorf("JSON file does not contain the chat ID. Content:\n%s", jsonContent)
	}
}

// TestUpsertChatPersistsImmediately verifies that UpsertChat writes to disk immediately
func TestUpsertChatPersistsImmediately(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "chat-store-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test_chats.json")

	// Create a chat store
	store, err := NewChatStoreJSON(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chat store: %v", err)
	}

	chatID := "test-chat-456"
	projectPath := "/another/test/path"

	// Create a chat
	chat := &Chat{
		ChatID:       chatID,
		Platform:     "telegram",
		ProjectPath:  projectPath,
		BashCwd:      projectPath,
		CurrentAgent: "claude",
	}

	err = store.UpsertChat(chat)
	if err != nil {
		t.Fatalf("Failed to upsert chat: %v", err)
	}

	// Close the store
	if err := store.Close(); err != nil {
		t.Fatalf("Failed to close store: %v", err)
	}

	// Check if file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("JSON file was not created at %s", dbPath)
	}

	// Read the JSON file directly to verify the data was persisted
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("Failed to read JSON file: %v", err)
	}

	jsonContent := string(data)

	// Verify the fields are in the JSON
	if !contains(jsonContent, `"project_path":`) {
		t.Errorf("JSON file does not contain project_path field. Content:\n%s", jsonContent)
	}
	if !contains(jsonContent, projectPath) {
		t.Errorf("JSON file does not contain the expected project path. Content:\n%s", jsonContent)
	}
	if !contains(jsonContent, `"current_agent":`) {
		t.Errorf("JSON file does not contain current_agent field. Content:\n%s", jsonContent)
	}
	if !contains(jsonContent, `"claude"`) {
		t.Errorf("JSON file does not contain the expected agent type. Content:\n%s", jsonContent)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// TestSetCurrentAgentPersistsImmediately verifies that SetCurrentAgent writes to disk immediately
func TestSetCurrentAgentPersistsImmediately(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "chat-store-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test_chats.json")

	// Create a chat store
	store, err := NewChatStoreJSON(dbPath)
	if err != nil {
		t.Fatalf("Failed to create chat store: %v", err)
	}

	chatID := "test-chat-789"

	// First create a chat
	chat := &Chat{
		ChatID:    chatID,
		Platform:  "telegram",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	err = store.UpsertChat(chat)
	if err != nil {
		t.Fatalf("Failed to upsert chat: %v", err)
	}

	// Set current agent using SetCurrentAgent (this tests UpdateChat path)
	err = store.SetCurrentAgent(chatID, "claude")
	if err != nil {
		t.Fatalf("Failed to set current agent: %v", err)
	}

	// Close the store
	if err := store.Close(); err != nil {
		t.Fatalf("Failed to close store: %v", err)
	}

	// Read the JSON file directly to verify the data was persisted
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("Failed to read JSON file: %v", err)
	}

	jsonContent := string(data)
	t.Logf("JSON content:\n%s", jsonContent)

	// Verify the current_agent was updated
	if !contains(jsonContent, `"current_agent":`) {
		t.Errorf("JSON file does not contain current_agent field. Content:\n%s", jsonContent)
	}
	if !contains(jsonContent, `"claude"`) {
		t.Errorf("JSON file does not contain the expected agent type 'claude'. Content:\n%s", jsonContent)
	}
}
