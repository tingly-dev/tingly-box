package db

import (
	"errors"
	"testing"
)

// ---------- hard delete ----------

// TestDeleteChatRemovesRow covers the "真删" half of the lifecycle: after
// DeleteChat the row is gone, and if the chat comes back through any of the
// auto-create paths it starts from a blank slate — no pairing, no whitelist,
// no project binding survives.
func TestDeleteChatRemovesRow(t *testing.T) {
	store := newChatStore(t)

	if err := store.BindProject("chat-1", "telegram", "/proj", "owner"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := store.SetPaired("chat-1", "telegram", "bot-uuid", "sender"); err != nil {
		t.Fatalf("pair: %v", err)
	}
	if err := store.AddToWhitelist("chat-1", "telegram", "admin"); err != nil {
		t.Fatalf("whitelist: %v", err)
	}

	if err := store.DeleteChat("chat-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	chat, err := store.GetChat("chat-1")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if chat != nil {
		t.Fatalf("chat survived delete: %+v", chat)
	}

	// Natural recreate: the chat messaging again mints a fresh row with none
	// of the old state.
	fresh, err := store.GetOrCreateChat("chat-1", "telegram")
	if err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if fresh.IsPaired || fresh.IsWhitelisted || fresh.ProjectPath != "" {
		t.Errorf("recreated chat inherited old state: %+v", fresh)
	}
}

// TestDeleteChatMissingIsNoop keeps delete idempotent — deleting a chat that
// doesn't exist (or was already deleted) is not an error.
func TestDeleteChatMissingIsNoop(t *testing.T) {
	store := newChatStore(t)
	if err := store.DeleteChat("never-existed"); err != nil {
		t.Errorf("delete missing: %v", err)
	}
}

// TestDeleteChatRequiresID keeps the validation the store inherited.
func TestDeleteChatRequiresID(t *testing.T) {
	store := newChatStore(t)
	if err := store.DeleteChat(""); !errors.Is(err, ErrChatIDRequired) {
		t.Errorf("delete with empty id: err = %v, want ErrChatIDRequired", err)
	}
}

// ---------- disable blocklist ----------

// TestSetChatDisabledRoundTrips covers the basic flag: set → IsChatDisabled
// true + DisabledAt stamped; clear → false + DisabledAt zeroed.
func TestSetChatDisabledRoundTrips(t *testing.T) {
	store := newChatStore(t)

	if _, err := store.GetOrCreateChat("chat-1", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.SetChatDisabled("chat-1", true); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !store.IsChatDisabled("chat-1") {
		t.Fatal("chat should be disabled")
	}
	chat, err := store.GetChat("chat-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if chat.DisabledAt.IsZero() {
		t.Error("DisabledAt not stamped on disable")
	}

	if err := store.SetChatDisabled("chat-1", false); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if store.IsChatDisabled("chat-1") {
		t.Error("disable flag was not cleared")
	}
	chat, err = store.GetChat("chat-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !chat.DisabledAt.IsZero() {
		t.Errorf("DisabledAt survived re-enable: %v", chat.DisabledAt)
	}
}

// TestDisabledSurvivesStateWrites is the core blocklist guarantee: the flag
// must NOT be silently cleared by the auto-create/state paths (pairing,
// whitelist, bind, agent handoff) — only an explicit enable clears it.
func TestDisabledSurvivesStateWrites(t *testing.T) {
	store := newChatStore(t)

	if _, err := store.GetOrCreateChat("chat-1", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.SetChatDisabled("chat-1", true); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if err := store.SetPaired("chat-1", "telegram", "bot-uuid", "sender"); err != nil {
		t.Fatalf("pair: %v", err)
	}
	if err := store.AddToWhitelist("chat-1", "telegram", "admin"); err != nil {
		t.Fatalf("whitelist: %v", err)
	}
	if err := store.BindProject("chat-1", "telegram", "/proj", "owner"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := store.SetCurrentAgent("chat-1", "telegram", "claude"); err != nil {
		t.Fatalf("agent: %v", err)
	}

	if !store.IsChatDisabled("chat-1") {
		t.Error("disable flag was cleared by a state write")
	}
}

// TestIsChatDisabledUnknownIsFalse keeps pushes to fresh chat ids working:
// a chat the store has never seen is not blocked.
func TestIsChatDisabledUnknownIsFalse(t *testing.T) {
	store := newChatStore(t)
	if store.IsChatDisabled("never-seen") {
		t.Error("unknown chat reported disabled")
	}
}

// TestSetChatDisabledMissingIsNoop mirrors UpdateChat's contract: disabling a
// chat that doesn't exist changes nothing and creates nothing.
func TestSetChatDisabledMissingIsNoop(t *testing.T) {
	store := newChatStore(t)
	if err := store.SetChatDisabled("nope", true); err != nil {
		t.Fatalf("disable missing: %v", err)
	}
	chat, err := store.GetChat("nope")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if chat != nil {
		t.Errorf("SetChatDisabled created a chat: %+v", chat)
	}
}

// ---------- listing ----------

// TestListChatsExcludesDisabled checks the default listing hides blocklisted
// chats and the includeDisabled flag brings them back.
func TestListChatsExcludesDisabled(t *testing.T) {
	store := newChatStore(t)

	if _, err := store.GetOrCreateChat("visible", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.GetOrCreateChat("blocked", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.SetChatDisabled("blocked", true); err != nil {
		t.Fatalf("disable: %v", err)
	}

	chats, err := store.ListChats("telegram", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(chats) != 1 || chats[0].ChatID != "visible" {
		t.Errorf("default list = %+v, want just the visible chat", chats)
	}

	all, err := store.ListChats("telegram", true)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("includeDisabled list has %d chats, want 2: %+v", len(all), all)
	}
}

// TestListChatsIncludesLegacyNullDisabled guards the migration edge that hid
// every pre-existing chat: AutoMigrate adds the disabled column with NULL for
// rows created before it, and `disabled = false` never matches NULL. The
// default listing must treat NULL as "not disabled".
func TestListChatsIncludesLegacyNullDisabled(t *testing.T) {
	store := newChatStore(t)

	if _, err := store.GetOrCreateChat("legacy", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Simulate the pre-migration state: NULL, not false.
	if err := store.db.Exec("UPDATE remote_chats SET disabled = NULL WHERE chat_id = ?", "legacy").Error; err != nil {
		t.Fatalf("null out disabled: %v", err)
	}

	chats, err := store.ListChats("telegram", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(chats) != 1 || chats[0].ChatID != "legacy" {
		t.Errorf("legacy NULL-disabled chat missing from default list: %+v", chats)
	}
}
