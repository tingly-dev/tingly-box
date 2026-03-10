package bot

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJSONStoreBasicOperations(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bot_chats.json")

	store, err := NewJSONStore(filePath)
	require.NoError(t, err)
	require.NotNil(t, store)

	// File doesn't exist yet until we save data
	_, err = os.Stat(filePath)
	require.True(t, os.IsNotExist(err), "File should not exist until data is saved")

	// Add some data and close
	testChat := &Chat{
		ChatID:    "test-chat",
		Platform:  "telegram",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	err = store.UpsertChat(testChat)
	require.NoError(t, err)

	// Test Close (this will save the data)
	err = store.Close()
	require.NoError(t, err)

	// Now file should exist with content
	_, err = os.Stat(filePath)
	require.NoError(t, err)

	// Verify file has content
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Contains(t, string(data), `"version"`)
	require.Contains(t, string(data), `"chats"`)
}

func TestJSONStoreGetChat(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bot_chats.json")

	store, err := NewJSONStore(filePath)
	require.NoError(t, err)
	defer store.Close()

	// Get non-existent chat
	chat, err := store.GetChat("non-existent")
	require.NoError(t, err)
	require.Nil(t, chat)

	// Create a chat
	testChat := &Chat{
		ChatID:    "test-chat-1",
		Platform:  "telegram",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	err = store.UpsertChat(testChat)
	require.NoError(t, err)

	// Get the chat
	chat, err = store.GetChat("test-chat-1")
	require.NoError(t, err)
	require.NotNil(t, chat)
	require.Equal(t, "test-chat-1", chat.ChatID)
	require.Equal(t, "telegram", chat.Platform)
}

func TestJSONStoreGetOrCreateChat(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bot_chats.json")

	store, err := NewJSONStore(filePath)
	require.NoError(t, err)
	defer store.Close()

	// Create new chat
	chat, err := store.GetOrCreateChat("new-chat", "telegram")
	require.NoError(t, err)
	require.NotNil(t, chat)
	require.Equal(t, "new-chat", chat.ChatID)
	require.Equal(t, "telegram", chat.Platform)

	// Get existing chat
	chat2, err := store.GetOrCreateChat("new-chat", "discord")
	require.NoError(t, err)
	require.Equal(t, chat.ChatID, chat2.ChatID)
	// Platform should not be updated on get
	require.Equal(t, "telegram", chat2.Platform)
}

func TestJSONStoreUpdateChat(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bot_chats.json")

	store, err := NewJSONStore(filePath)
	require.NoError(t, err)
	defer store.Close()

	// Create a chat
	testChat := &Chat{
		ChatID:    "test-chat",
		Platform:  "telegram",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	err = store.UpsertChat(testChat)
	require.NoError(t, err)

	// Update the chat
	err = store.UpdateChat("test-chat", func(chat *Chat) {
		chat.ProjectPath = "/path/to/project"
		chat.OwnerID = "user123"
	})
	require.NoError(t, err)

	// Verify update
	chat, err := store.GetChat("test-chat")
	require.NoError(t, err)
	require.Equal(t, "/path/to/project", chat.ProjectPath)
	require.Equal(t, "user123", chat.OwnerID)
}

func TestJSONStoreBindProject(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bot_chats.json")

	store, err := NewJSONStore(filePath)
	require.NoError(t, err)
	defer store.Close()

	// Bind project to non-existent chat
	err = store.BindProject("chat-1", "telegram", "/project/path", "user1")
	require.NoError(t, err)

	// Verify binding
	path, ok, err := store.GetProjectPath("chat-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "/project/path", path)

	// Update existing chat
	err = store.BindProject("chat-1", "telegram", "/new/path", "user2")
	require.NoError(t, err)

	path, ok, err = store.GetProjectPath("chat-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "/new/path", path)
}

func TestJSONStoreSession(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bot_chats.json")

	store, err := NewJSONStore(filePath)
	require.NoError(t, err)
	defer store.Close()

	// Set session for non-existent chat
	err = store.SetSession("chat-1", "session-123")
	require.NoError(t, err)

	// Get session
	sessionID, ok, err := store.GetSession("chat-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "session-123", sessionID)

	// Update session
	err = store.SetSession("chat-1", "session-456")
	require.NoError(t, err)

	sessionID, ok, err = store.GetSession("chat-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "session-456", sessionID)

	// Get non-existent session
	sessionID, ok, err = store.GetSession("non-existent")
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, sessionID)
}

func TestJSONStoreWhitelist(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bot_chats.json")

	store, err := NewJSONStore(filePath)
	require.NoError(t, err)
	defer store.Close()

	// Add to whitelist
	err = store.AddToWhitelist("group-1", "telegram", "admin")
	require.NoError(t, err)

	// Check whitelisted
	require.True(t, store.IsWhitelisted("group-1"))
	require.False(t, store.IsWhitelisted("group-2"))

	// Remove from whitelist
	err = store.RemoveFromWhitelist("group-1")
	require.NoError(t, err)
	require.False(t, store.IsWhitelisted("group-1"))
}

func TestJSONStoreBashCwd(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bot_chats.json")

	store, err := NewJSONStore(filePath)
	require.NoError(t, err)
	defer store.Close()

	// Create a chat first
	_, err = store.GetOrCreateChat("chat-1", "telegram")
	require.NoError(t, err)

	// Set bash cwd
	err = store.SetBashCwd("chat-1", "/path/to/project")
	require.NoError(t, err)

	// Get bash cwd
	cwd, ok, err := store.GetBashCwd("chat-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "/path/to/project", cwd)

	// Get non-existent cwd
	cwd, ok, err = store.GetBashCwd("non-existent")
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, cwd)
}

func TestJSONStoreAgentState(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bot_chats.json")

	store, err := NewJSONStore(filePath)
	require.NoError(t, err)
	defer store.Close()

	// Create a chat first
	_, err = store.GetOrCreateChat("chat-1", "telegram")
	require.NoError(t, err)

	// Set current agent
	err = store.SetCurrentAgent("chat-1", "tingly-box")
	require.NoError(t, err)

	// Get current agent
	agent, err := store.GetCurrentAgent("chat-1")
	require.NoError(t, err)
	require.Equal(t, "tingly-box", agent)

	// Get default agent for non-existent chat
	agent, err = store.GetCurrentAgent("non-existent")
	require.NoError(t, err)
	require.Equal(t, "claude", agent) // Default

	// Set agent state
	state := []byte(`{"key": "value"}`)
	err = store.SetAgentState("chat-1", state)
	require.NoError(t, err)

	// Get agent state
	retrievedState, err := store.GetAgentState("chat-1")
	require.NoError(t, err)
	require.Equal(t, state, retrievedState)
}

func TestJSONStoreListChatsByOwner(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bot_chats.json")

	store, err := NewJSONStore(filePath)
	require.NoError(t, err)
	defer store.Close()

	// Create multiple chats
	chat1 := &Chat{
		ChatID:      "chat-1",
		Platform:    "telegram",
		OwnerID:     "user1",
		ProjectPath: "/project1",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	chat2 := &Chat{
		ChatID:      "chat-2",
		Platform:    "telegram",
		OwnerID:     "user1",
		ProjectPath: "/project2",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	chat3 := &Chat{
		ChatID:      "chat-3",
		Platform:    "telegram",
		OwnerID:     "user2",
		ProjectPath: "/project3",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	require.NoError(t, store.UpsertChat(chat1))
	require.NoError(t, store.UpsertChat(chat2))
	require.NoError(t, store.UpsertChat(chat3))

	// List chats by owner
	chats, err := store.ListChatsByOwner("user1", "telegram")
	require.NoError(t, err)
	require.Len(t, chats, 2)

	chatIDs := make(map[string]bool)
	for _, chat := range chats {
		chatIDs[chat.ChatID] = true
	}
	require.True(t, chatIDs["chat-1"])
	require.True(t, chatIDs["chat-2"])
	require.False(t, chatIDs["chat-3"])
}

func TestJSONStorePersistence(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bot_chats.json")

	// Create and populate store
	store1, err := NewJSONStore(filePath)
	require.NoError(t, err)

	testChat := &Chat{
		ChatID:      "persist-chat",
		Platform:    "telegram",
		ProjectPath: "/persistent/project",
		OwnerID:     "persistent-user",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err = store1.UpsertChat(testChat)
	require.NoError(t, err)

	err = store1.Close()
	require.NoError(t, err)

	// Open new store instance
	store2, err := NewJSONStore(filePath)
	require.NoError(t, err)
	defer store2.Close()

	// Verify data persisted
	chat, err := store2.GetChat("persist-chat")
	require.NoError(t, err)
	require.NotNil(t, chat)
	require.Equal(t, "persist-chat", chat.ChatID)
	require.Equal(t, "/persistent/project", chat.ProjectPath)
	require.Equal(t, "persistent-user", chat.OwnerID)
}

func TestChatStoreJSONInterface(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bot_chats.json")

	// Test ChatStoreJSON implements ChatStoreInterface
	chatStore, err := NewChatStoreJSON(filePath)
	require.NoError(t, err)
	defer chatStore.Close()

	// Test basic operations through the interface
	var storeInterface ChatStoreInterface = chatStore

	// Create chat
	testChat := &Chat{
		ChatID:    "interface-test",
		Platform:  "telegram",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	err = storeInterface.UpsertChat(testChat)
	require.NoError(t, err)

	// Get chat
	chat, err := storeInterface.GetChat("interface-test")
	require.NoError(t, err)
	require.Equal(t, "interface-test", chat.ChatID)

	// Test whitelist
	err = storeInterface.AddToWhitelist("interface-test", "telegram", "admin")
	require.NoError(t, err)
	require.True(t, storeInterface.IsWhitelisted("interface-test"))
}
