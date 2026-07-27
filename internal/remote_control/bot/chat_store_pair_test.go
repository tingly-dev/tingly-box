package bot

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

// TestBotSetting_IsRequirePairing verifies the helper returns the right
// answer for the explicit/explicit/platform-default tri-state.
func TestBotSetting_IsRequirePairing(t *testing.T) {
	yes := true
	no := false
	cases := []struct {
		name     string
		v        *bool
		platform string
		want     bool
	}{
		// Explicit values always win, regardless of platform.
		{"explicit true on telegram", &yes, "telegram", true},
		{"explicit true on feishu", &yes, "feishu", true},
		{"explicit false on telegram", &no, "telegram", false},
		{"explicit false on feishu", &no, "feishu", false},

		// Platform defaults: token-DM platforms enforce, others don't.
		{"nil on telegram defaults on", nil, "telegram", true},
		{"nil on discord defaults on", nil, "discord", true},
		{"nil on slack defaults on", nil, "slack", true},
		{"nil on feishu defaults off", nil, "feishu", false},
		{"nil on dingtalk defaults off", nil, "dingtalk", false},
		{"nil on whatsapp defaults off", nil, "whatsapp", false},
		{"nil on weixin defaults off", nil, "weixin", false},
		{"nil on empty platform defaults off", nil, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := BotSetting{RequirePairing: tc.v, Platform: tc.platform}
			if got := b.IsRequirePairing(); got != tc.want {
				t.Fatalf("IsRequirePairing()=%v want %v", got, tc.want)
			}
		})
	}
}
