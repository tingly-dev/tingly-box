package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/remote/control/bot"
)

// stores opens the shared database in dir. Note initRemoteStores already runs
// the legacy import, so tests that seed legacy files must write them BEFORE
// calling this — which is what makes the "already migrated" assertions real.
func stores(t *testing.T, dir string) *RemoteChatStore {
	t.Helper()
	sm, err := NewStoreManager(dir)
	if err != nil {
		t.Fatalf("open store manager: %v", err)
	}
	t.Cleanup(func() { _ = sm.Close() })
	return sm.RemoteChats()
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// legacy envelopes, written the way pkg/jsonstore used to write them.
func writeLegacyChats(t *testing.T, dir string, chats map[string]*bot.Chat) {
	t.Helper()
	writeJSON(t, filepath.Join(dir, "bot_chats.json"), map[string]any{
		"version": 1,
		"items":   chats,
		"updated": time.Now().UTC(),
	})
}

func TestImportMovesChats(t *testing.T) {
	dir := t.TempDir()
	chatStore := stores(t, dir)

	writeLegacyChats(t, dir, map[string]*bot.Chat{
		"chat-1": {
			ChatID:         "chat-1",
			Platform:       "telegram",
			ProjectPath:    "/proj",
			ProjectHistory: []string{"/proj", "/old"},
			OwnerID:        "owner-1",
			IsPaired:       true,
			PairedBotUUID:  "bot-uuid",
			PairedSenderID: "sender-1",
			IsWhitelisted:  true,
			WhitelistedBy:  "admin",
		},
	})
	if err := importLegacyRemoteJSON(dir, chatStore); err != nil {
		t.Fatalf("import: %v", err)
	}

	chat, err := chatStore.GetChat("chat-1")
	if err != nil {
		t.Fatalf("get chat: %v", err)
	}
	if chat == nil {
		t.Fatal("chat was not imported")
	}
	if chat.ProjectPath != "/proj" || chat.OwnerID != "owner-1" {
		t.Errorf("chat binding lost: %+v", chat)
	}
	if !chat.IsPaired || chat.PairedBotUUID != "bot-uuid" || chat.PairedSenderID != "sender-1" {
		t.Errorf("pairing lost: %+v", chat)
	}
	if !chat.IsWhitelisted || chat.WhitelistedBy != "admin" {
		t.Errorf("whitelist lost: %+v", chat)
	}
	if len(chat.ProjectHistory) != 2 {
		t.Errorf("project history = %v, want 2 entries", chat.ProjectHistory)
	}
}

// TestImportRenamesFiles checks the files are moved aside — not deleted, so a
// bad import can still be recovered from.
func TestImportRenamesFiles(t *testing.T) {
	dir := t.TempDir()
	chatStore := stores(t, dir)

	writeLegacyChats(t, dir, map[string]*bot.Chat{"chat-1": {ChatID: "chat-1"}})

	if err := importLegacyRemoteJSON(dir, chatStore); err != nil {
		t.Fatalf("import: %v", err)
	}

	for _, name := range []string{"bot_chats.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s still in place after import", name)
		}
		if _, err := os.Stat(filepath.Join(dir, name+migratedSuffix)); err != nil {
			t.Errorf("%s was not preserved as %s: %v", name, migratedSuffix, err)
		}
	}
}

// TestImportIsIdempotent checks a second run is a harmless no-op, and — more
// importantly — that a re-appearing legacy file cannot overwrite state the
// running system has since written.
func TestImportIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	chatStore := stores(t, dir)

	writeLegacyChats(t, dir, map[string]*bot.Chat{
		"chat-1": {ChatID: "chat-1", Platform: "telegram", ProjectPath: "/stale"},
	})
	if err := importLegacyRemoteJSON(dir, chatStore); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// The system moves on.
	if err := chatStore.BindProject("chat-1", "telegram", "/current", "owner"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// The stale file comes back (restored from a backup, say) and is imported
	// again. Live state must win.
	writeLegacyChats(t, dir, map[string]*bot.Chat{
		"chat-1": {ChatID: "chat-1", Platform: "telegram", ProjectPath: "/stale"},
	})
	if err := importLegacyRemoteJSON(dir, chatStore); err != nil {
		t.Fatalf("second import: %v", err)
	}

	got, ok, err := chatStore.GetProjectPath("chat-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok || got != "/current" {
		t.Errorf("project path = %q, want /current — a stale file overwrote live state", got)
	}
}

// TestImportNoFilesIsNotAnError is the steady state on every start after the
// first one.
func TestImportNoFilesIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	chatStore := stores(t, dir)

	if err := importLegacyRemoteJSON(dir, chatStore); err != nil {
		t.Fatalf("import with no files: %v", err)
	}
}

// TestImportCorruptFileIsReported checks a malformed file surfaces as an error
// and is left in place for a retry rather than being renamed away.
func TestImportCorruptFileIsReported(t *testing.T) {
	dir := t.TempDir()
	chatStore := stores(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "bot_chats.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := importLegacyRemoteJSON(dir, chatStore); err == nil {
		t.Error("expected an error for a corrupt file")
	}
	if _, err := os.Stat(filepath.Join(dir, "bot_chats.json")); err != nil {
		t.Error("corrupt file was renamed away; it should be kept for a retry")
	}
}
