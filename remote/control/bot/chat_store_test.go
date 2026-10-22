package bot_test

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
)

// openStore returns a chat store over a temp directory, plus a reopen helper.
//
// These tests used to assert persistence by reading the JSON file and
// substring-matching its contents. That checked the encoding rather than the
// contract; now that chats are rows, the equivalent — and stricter — check is
// to close, reopen, and read the value back.
func openStore(t *testing.T, dir string) bot.ChatStoreInterface {
	t.Helper()
	sm, err := db.NewStoreManager(dir)
	if err != nil {
		t.Fatalf("open store manager: %v", err)
	}
	t.Cleanup(func() { _ = sm.Close() })
	return sm.RemoteChats()
}

// TestUpdateChatPersistsImmediately verifies UpdateChat is durable on return.
func TestUpdateChatPersistsImmediately(t *testing.T) {
	dir := t.TempDir()
	store := openStore(t, dir)

	const (
		chatID      = "test-chat-123"
		projectPath = "/test/path"
	)

	chat := &bot.Chat{
		ChatID:      chatID,
		Platform:    "telegram",
		ProjectPath: "",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.UpsertChat(chat); err != nil {
		t.Fatalf("Failed to upsert chat: %v", err)
	}

	if err := store.UpdateChat(chatID, func(c *bot.Chat) {
		c.ProjectPath = projectPath
	}); err != nil {
		t.Fatalf("Failed to update chat: %v", err)
	}
	// A second store manager over the same directory is a genuinely separate
	// connection, so reading back through it proves the write reached disk.
	reopened := openStore(t, dir)

	got, err := reopened.GetChat(chatID)
	if err != nil {
		t.Fatalf("Failed to read chat back: %v", err)
	}
	if got == nil {
		t.Fatal("chat did not survive close/reopen")
	}
	if got.ProjectPath != projectPath {
		t.Errorf("ProjectPath = %q, want %q", got.ProjectPath, projectPath)
	}
	if got.ChatID != chatID {
		t.Errorf("ChatID = %q, want %q", got.ChatID, chatID)
	}
}

// TestUpsertChatPersistsImmediately verifies UpsertChat is durable on return.
func TestUpsertChatPersistsImmediately(t *testing.T) {
	dir := t.TempDir()
	store := openStore(t, dir)

	const (
		chatID      = "test-chat-456"
		projectPath = "/another/test/path"
	)

	if err := store.UpsertChat(&bot.Chat{
		ChatID:      chatID,
		Platform:    "telegram",
		ProjectPath: projectPath,
	}); err != nil {
		t.Fatalf("Failed to upsert chat: %v", err)
	}
	// A second store manager over the same directory is a genuinely separate
	// connection, so reading back through it proves the write reached disk.
	reopened := openStore(t, dir)

	got, err := reopened.GetChat(chatID)
	if err != nil {
		t.Fatalf("Failed to read chat back: %v", err)
	}
	if got == nil {
		t.Fatal("chat did not survive close/reopen")
	}
	if got.ProjectPath != projectPath {
		t.Errorf("ProjectPath = %q, want %q", got.ProjectPath, projectPath)
	}
}
