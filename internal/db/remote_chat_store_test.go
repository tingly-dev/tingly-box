package db

import (
	"testing"

	"github.com/tingly-dev/tingly-box/remote/control/bot"
)

func newChatStore(t *testing.T) *RemoteChatStore {
	t.Helper()
	return openChatStoreAt(t, t.TempDir())
}

// openChatStoreAt opens the shared database in dir and returns its chat store.
// Two calls with the same dir give two independent connections to one database,
// which is what the concurrency test needs.
func openChatStoreAt(t *testing.T, dir string) *RemoteChatStore {
	t.Helper()
	sm, err := NewStoreManager(dir)
	if err != nil {
		t.Fatalf("open store manager: %v", err)
	}
	t.Cleanup(func() { _ = sm.Close() })
	return sm.RemoteChats()
}

// TestConcurrentStoresDoNotClobber is the point of the whole migration. Two
// stores over the same database — a CLI command and a running server, say —
// must both see each other's writes. The JSON files this replaces failed
// exactly here: each holder rewrote the file whole from a snapshot taken at
// open, so the second writer erased the first one's chats.
func TestConcurrentStoresDoNotClobber(t *testing.T) {
	dir := t.TempDir()

	first := openChatStoreAt(t, dir)
	second := openChatStoreAt(t, dir)

	if err := first.BindProject("chat-a", "telegram", "/proj/a", "owner-a"); err != nil {
		t.Fatalf("bind on first: %v", err)
	}
	if err := second.BindProject("chat-b", "feishu", "/proj/b", "owner-b"); err != nil {
		t.Fatalf("bind on second: %v", err)
	}

	// Both writes survive, seen from either connection.
	for _, probe := range []struct {
		store  *RemoteChatStore
		chatID string
		want   string
	}{
		{first, "chat-a", "/proj/a"},
		{first, "chat-b", "/proj/b"},
		{second, "chat-a", "/proj/a"},
		{second, "chat-b", "/proj/b"},
	} {
		got, ok, err := probe.store.GetProjectPath(probe.chatID)
		if err != nil {
			t.Fatalf("get %s: %v", probe.chatID, err)
		}
		if !ok || got != probe.want {
			t.Errorf("GetProjectPath(%s) = (%q, %v), want (%q, true)",
				probe.chatID, got, ok, probe.want)
		}
	}
}

// TestClearingFlagsPersists guards the GORM trap this store had to be written
// around: struct-based Updates silently skips zero values, so turning a flag
// back off would have looked like it worked and changed nothing.
func TestClearingFlagsPersists(t *testing.T) {
	store := newChatStore(t)

	if err := store.AddToWhitelist("chat-1", "telegram", "admin"); err != nil {
		t.Fatalf("whitelist: %v", err)
	}
	if !store.IsWhitelisted("chat-1") {
		t.Fatal("chat should be whitelisted")
	}
	if err := store.RemoveFromWhitelist("chat-1"); err != nil {
		t.Fatalf("unwhitelist: %v", err)
	}
	if store.IsWhitelisted("chat-1") {
		t.Error("whitelist flag was not cleared")
	}

	if err := store.SetPaired("chat-1", "telegram", "bot-uuid", "sender-1"); err != nil {
		t.Fatalf("pair: %v", err)
	}
	if !store.IsChatPaired("chat-1", "bot-uuid") {
		t.Fatal("chat should be paired")
	}
	if err := store.ClearPaired("chat-1"); err != nil {
		t.Fatalf("clear pairing: %v", err)
	}
	if store.IsChatPaired("chat-1", "bot-uuid") {
		t.Error("pairing was not cleared")
	}
	chat, err := store.GetChat("chat-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if chat.PairedBotUUID != "" || chat.PairedSenderID != "" {
		t.Errorf("pairing identifiers survived the clear: %+v", chat)
	}
}

// TestClearPairedPreservesOtherState checks unpairing is scoped: the chat's
// project binding and whitelist status must survive it.
func TestClearPairedPreservesOtherState(t *testing.T) {
	store := newChatStore(t)

	if err := store.BindProject("chat-1", "telegram", "/proj", "owner"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := store.AddToWhitelist("chat-1", "telegram", "admin"); err != nil {
		t.Fatalf("whitelist: %v", err)
	}
	if err := store.SetPaired("chat-1", "telegram", "bot-uuid", "sender"); err != nil {
		t.Fatalf("pair: %v", err)
	}
	if err := store.ClearPaired("chat-1"); err != nil {
		t.Fatalf("clear: %v", err)
	}

	chat, err := store.GetChat("chat-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if chat.ProjectPath != "/proj" {
		t.Errorf("ProjectPath = %q, want /proj", chat.ProjectPath)
	}
	if !chat.IsWhitelisted {
		t.Error("whitelist was cleared by unpairing")
	}
}

// TestUpdateChatMissingIsNoop keeps the contract the JSON store had: updating
// a chat that doesn't exist is silently ignored, not an error.
func TestUpdateChatMissingIsNoop(t *testing.T) {
	store := newChatStore(t)

	called := false
	if err := store.UpdateChat("nope", func(c *bot.Chat) { called = true }); err != nil {
		t.Fatalf("update missing: %v", err)
	}
	if called {
		t.Error("update function ran for a chat that does not exist")
	}
}

// TestListChatsByOwnerFiltersUnbound checks the owner listing only surfaces
// chats that actually have a project bound.
func TestListChatsByOwnerFiltersUnbound(t *testing.T) {
	store := newChatStore(t)

	if err := store.BindProject("bound", "telegram", "/proj", "owner-1"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := store.GetOrCreateChat("unbound", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.BindProject("other-owner", "telegram", "/proj", "owner-2"); err != nil {
		t.Fatalf("bind other: %v", err)
	}

	chats, err := store.ListChatsByOwner("owner-1", "telegram")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(chats) != 1 || chats[0].ChatID != "bound" {
		t.Errorf("chats = %+v, want just the bound chat for owner-1", chats)
	}
}

// TestUpsertRequiresChatID keeps the validation the store inherited.
func TestUpsertRequiresChatID(t *testing.T) {
	store := newChatStore(t)

	if err := store.UpsertChat(&bot.Chat{}); err == nil {
		t.Error("expected an error upserting a chat with no ID")
	}
}

// TestContextTokenRoundTrip verifies the persisted reply-context token
// (WeChat ilink) survives a write/read cycle — the property proactive
// notifications rely on. Cross-store durability (two connections to one
// database) is already covered by TestConcurrentStoresDoNotClobber.
func TestContextTokenRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := openChatStoreAt(t, dir)

	// Create the chat (UpdateChat is a no-op on a missing row), then set the token.
	if err := store.BindProject("chat-1", "weixin", "/proj", "owner"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := store.UpdateChat("chat-1", func(c *bot.Chat) {
		c.ContextToken = "tok-from-inbound"
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := store.GetChat("chat-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ContextToken != "tok-from-inbound" {
		t.Errorf("ContextToken = %q, want %q", got.ContextToken, "tok-from-inbound")
	}

	// A fresh store over the same database dir sees the persisted token
	// (restart simulation: the in-memory cache is gone, the DB row survives).
	store2 := openChatStoreAt(t, dir)
	got2, err := store2.GetChat("chat-1")
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if got2.ContextToken != "tok-from-inbound" {
		t.Errorf("after reopen ContextToken = %q, want %q", got2.ContextToken, "tok-from-inbound")
	}
}

// ---------- project-history MRU ----------
//
// PushProjectHistory is now a method on *bot.Chat; its unit tests moved to
// remote/control/bot/chat_test.go alongside the type. BindProject (below in
// remote_chat_store.go) is the store-level caller exercised by the import
// and binding tests above.
