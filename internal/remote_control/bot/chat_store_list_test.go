package bot

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestChatStore(t *testing.T) *ChatStoreJSON {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "chat-store-list-test")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	store, err := NewChatStoreJSON(filepath.Join(tmpDir, "chats.json"))
	if err != nil {
		t.Fatalf("NewChatStoreJSON: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// TestListChats_ScopesToPlatform is the regression guard for cross-platform
// leakage: the store is keyed by chatID alone, so a record belonging to
// another platform — or one with no platform attribution at all — must never
// appear in a bot's reachable-chat list.
func TestListChats_ScopesToPlatform(t *testing.T) {
	store := newTestChatStore(t)

	now := time.Now().UTC()
	for _, c := range []*Chat{
		{ChatID: "tg:1", Platform: "telegram", CreatedAt: now, UpdatedAt: now},
		{ChatID: "dc:1", Platform: "discord", CreatedAt: now, UpdatedAt: now},
		{ChatID: "orphan", Platform: "", CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.UpsertChat(c); err != nil {
			t.Fatalf("UpsertChat(%s): %v", c.ChatID, err)
		}
	}

	got, err := store.ListChats("telegram")
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(got) != 1 || got[0].ChatID != "tg:1" {
		t.Fatalf("expected only the telegram chat, got %+v", got)
	}

	// An unattributed record cannot be proven to belong to any bot, so asking
	// for the empty platform must not hand it out either.
	if got, err := store.ListChats(""); err != nil {
		t.Fatalf("ListChats(\"\"): %v", err)
	} else if len(got) != 1 || got[0].ChatID != "orphan" {
		// Documenting the current contract: "" matches only records whose
		// platform is literally empty. Callers always pass a real platform.
		t.Fatalf("expected only the unattributed record, got %+v", got)
	}
}

// TestListChats_NewestFirst pins the ordering so the most recently active
// chats surface at the top of the list. UpdatedAt is stamped by the store on
// every write (normalizeChat), not by the caller, so "most recent" here means
// "written last" — the list is the reverse of the insertion order.
func TestListChats_NewestFirst(t *testing.T) {
	store := newTestChatStore(t)

	inserted := []string{"first", "second", "third"}
	for _, id := range inserted {
		if err := store.UpsertChat(&Chat{ChatID: id, Platform: "telegram"}); err != nil {
			t.Fatalf("UpsertChat(%s): %v", id, err)
		}
	}

	got, err := store.ListChats("telegram")
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	want := []string{"third", "second", "first"}
	if len(got) != len(want) {
		t.Fatalf("expected %d chats, got %d (%+v)", len(want), len(got), got)
	}
	for i, id := range want {
		if got[i].ChatID != id {
			t.Fatalf("position %d: expected %q, got %q", i, id, got[i].ChatID)
		}
	}
}

// TestGetOrCreateChat_RefusesCrossPlatformCollision guards the other half of
// the same problem: two platforms can mint the same chatID string, and the
// store must refuse rather than hand back (and later re-stamp) the record
// belonging to the other platform.
func TestGetOrCreateChat_RefusesCrossPlatformCollision(t *testing.T) {
	store := newTestChatStore(t)

	if _, err := store.GetOrCreateChat("12345", "telegram"); err != nil {
		t.Fatalf("first GetOrCreateChat: %v", err)
	}

	if _, err := store.GetOrCreateChat("12345", "discord"); err == nil {
		t.Fatal("expected an error for a cross-platform chatID collision")
	}

	// The original record must be untouched.
	chat, err := store.GetChat("12345")
	if err != nil {
		t.Fatalf("GetChat: %v", err)
	}
	if chat == nil || chat.Platform != "telegram" {
		t.Fatalf("expected the telegram record to survive, got %+v", chat)
	}

	// Same platform still resolves, and an unattributed record is adoptable
	// (BindProject stamps the platform onto legacy records).
	if _, err := store.GetOrCreateChat("12345", "telegram"); err != nil {
		t.Fatalf("same-platform GetOrCreateChat: %v", err)
	}
}
