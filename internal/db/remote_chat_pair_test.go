package db_test

import (
	"os"
	"testing"
)

// TestSetPaired_RoundTrip verifies SetPaired persists across reload and that
// IsChatPaired enforces the (chatID, botUUID) tuple.
func TestSetPaired_RoundTrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "chat-store-pair-test")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := openStore(t, tmpDir)

	const (
		chatID  = "chat-1"
		botUUID = "bot-1"
		sender  = "alice"
		plat    = "telegram"
	)

	if store.IsChatPaired(chatID, botUUID) {
		t.Fatalf("expected unpaired before SetPaired")
	}
	if err := store.SetPaired(chatID, plat, botUUID, sender); err != nil {
		t.Fatalf("SetPaired: %v", err)
	}
	if !store.IsChatPaired(chatID, botUUID) {
		t.Fatalf("expected paired after SetPaired")
	}
	if store.IsChatPaired(chatID, "other-bot") {
		t.Fatalf("pairing must be scoped to bot UUID")
	}

	// Survive a reopen through an independent connection.
	store2 := openStore(t, tmpDir)
	if !store2.IsChatPaired(chatID, botUUID) {
		t.Fatalf("pairing did not persist across reload")
	}
	chat, err := store2.GetChat(chatID)
	if err != nil || chat == nil {
		t.Fatalf("GetChat after reload: chat=%v err=%v", chat, err)
	}
	if chat.PairedSenderID != sender {
		t.Fatalf("expected sender %q, got %q", sender, chat.PairedSenderID)
	}
	if chat.PairedAt.IsZero() {
		t.Fatalf("PairedAt should be set")
	}
}

// TestClearPaired_DropsBindingPreservesOtherFields ensures that ClearPaired
// removes pairing state but does not drop unrelated chat fields.
func TestClearPaired_DropsBindingPreservesOtherFields(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "chat-store-pair-test")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := openStore(t, tmpDir)

	const chatID = "chat-2"

	// Pair it and bind a project path so we can later confirm that ClearPaired
	// only resets pairing fields.
	if err := store.SetPaired(chatID, "discord", "bot-2", "carol"); err != nil {
		t.Fatalf("SetPaired: %v", err)
	}
	if err := store.BindProject(chatID, "discord", "/tmp/project", "carol"); err != nil {
		t.Fatalf("BindProject: %v", err)
	}

	if err := store.ClearPaired(chatID); err != nil {
		t.Fatalf("ClearPaired: %v", err)
	}

	chat, err := store.GetChat(chatID)
	if err != nil || chat == nil {
		t.Fatalf("GetChat: chat=%v err=%v", chat, err)
	}
	if chat.IsPaired || chat.PairedBotUUID != "" || chat.PairedSenderID != "" || !chat.PairedAt.IsZero() {
		t.Fatalf("ClearPaired left residue: %+v", chat)
	}
	if chat.ProjectPath != "/tmp/project" {
		t.Fatalf("ClearPaired clobbered ProjectPath: %q", chat.ProjectPath)
	}
}
