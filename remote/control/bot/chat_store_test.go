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
		BashCwd:     "",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.UpsertChat(chat); err != nil {
		t.Fatalf("Failed to upsert chat: %v", err)
	}

	if err := store.UpdateChat(chatID, func(c *bot.Chat) {
		c.ProjectPath = projectPath
		c.BashCwd = projectPath
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
	if got.BashCwd != projectPath {
		t.Errorf("BashCwd = %q, want %q", got.BashCwd, projectPath)
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
		ChatID:       chatID,
		Platform:     "telegram",
		ProjectPath:  projectPath,
		BashCwd:      projectPath,
		CurrentAgent: "claude",
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
	if got.CurrentAgent != "claude" {
		t.Errorf("CurrentAgent = %q, want %q", got.CurrentAgent, "claude")
	}
}

// TestSetCurrentAgentPersistsImmediately verifies SetCurrentAgent is durable
// on return.
func TestSetCurrentAgentPersistsImmediately(t *testing.T) {
	dir := t.TempDir()
	store := openStore(t, dir)

	const chatID = "test-chat-789"

	if err := store.UpsertChat(&bot.Chat{
		ChatID:    chatID,
		Platform:  "telegram",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Failed to upsert chat: %v", err)
	}
	if err := store.SetCurrentAgent(chatID, "telegram", "claude"); err != nil {
		t.Fatalf("Failed to set current agent: %v", err)
	}
	// A second store manager over the same directory is a genuinely separate
	// connection, so reading back through it proves the write reached disk.
	reopened := openStore(t, dir)

	got, err := reopened.GetCurrentAgent(chatID)
	if err != nil {
		t.Fatalf("Failed to read current agent back: %v", err)
	}
	if got != "claude" {
		t.Errorf("CurrentAgent = %q, want %q", got, "claude")
	}
}

// TestSetCurrentAgentCreatesMissingChat covers the @cc/@tb handoff-on-fresh-chat
// case: a chat that has not been bound (/cd) or paired (/bind) yet must still
// have its current-agent persisted on the first handoff. Previously
// SetCurrentAgent silently no-op'd because UpdateChat skips missing rows.
func TestSetCurrentAgentCreatesMissingChat(t *testing.T) {
	store := openStore(t, t.TempDir())

	const chatID = "tg-fresh-chat-1"
	if err := store.SetCurrentAgent(chatID, "telegram", "claude"); err != nil {
		t.Fatalf("SetCurrentAgent on missing chat: %v", err)
	}

	got, err := store.GetCurrentAgent(chatID)
	if err != nil {
		t.Fatalf("GetCurrentAgent: %v", err)
	}
	if got != "claude" {
		t.Fatalf("current agent not persisted: got %q, want \"claude\"", got)
	}

	chat, err := store.GetChat(chatID)
	if err != nil || chat == nil {
		t.Fatalf("chat row not created: chat=%v err=%v", chat, err)
	}
	if chat.Platform != "telegram" {
		t.Errorf("platform not set on auto-created chat: got %q, want \"telegram\"", chat.Platform)
	}
}
