package bot_test

import (
	"testing"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/bot"
)

func newIncomingMessage(chatID, text string) imbot.Message {
	return imbot.Message{
		Recipient: imbot.Recipient{ID: chatID},
		Content:   imbot.NewTextContent(text),
	}
}

// TestDisabledChatGate_ClaimsBeforeAnyoneElse is the regression test for the
// promptReplyRouter bypass: the gate must report true (claim and drop) for a
// disabled chat regardless of message shape, so it can sit ahead of
// promptReplyRouter in the dispatch chain and stop a disabled chat's pending
// permission-prompt callback or text answer from ever reaching it.
func TestDisabledChatGate_ClaimsBeforeAnyoneElse(t *testing.T) {
	store := openStore(t, t.TempDir())
	const chatID = "disabled-chat"
	if _, err := store.GetOrCreateChat(chatID, "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.SetChatDisabled(chatID, true); err != nil {
		t.Fatalf("disable: %v", err)
	}

	gate := bot.DisabledChatGate(store)

	msg := newIncomingMessage(chatID, "yes")
	if claimed := gate(msg, imbot.PlatformTelegram, "bot-1"); !claimed {
		t.Fatal("gate did not claim a disabled chat's message")
	}

	// A plausible trigger for the bypass this fixes: a "perm" callback that
	// promptReplyRouter would otherwise claim and answer.
	callback := newIncomingMessage(chatID, "")
	callback.Payload = imbot.NewPayload("perm", "approve", "req-123")
	if claimed := gate(callback, imbot.PlatformTelegram, "bot-1"); !claimed {
		t.Fatal("gate did not claim a disabled chat's callback")
	}
}

// TestDisabledChatGate_PassesThroughEnabledChats ensures the gate never
// claims traffic from a chat that isn't disabled, including one that has
// never been seen before (no row in the store yet).
func TestDisabledChatGate_PassesThroughEnabledChats(t *testing.T) {
	store := openStore(t, t.TempDir())
	gate := bot.DisabledChatGate(store)

	fresh := newIncomingMessage("brand-new-chat", "hi")
	if claimed := gate(fresh, imbot.PlatformTelegram, "bot-1"); claimed {
		t.Fatal("gate claimed a message from a chat it has never seen")
	}

	const chatID = "enabled-chat"
	if _, err := store.GetOrCreateChat(chatID, "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	msg := newIncomingMessage(chatID, "hi")
	if claimed := gate(msg, imbot.PlatformTelegram, "bot-1"); claimed {
		t.Fatal("gate claimed a message from an enabled chat")
	}
}

// TestDisabledChatGate_NilStoreNeverClaims covers a nil-injected store (e.g.
// misconfigured host) failing open rather than panicking or blocking traffic.
func TestDisabledChatGate_NilStoreNeverClaims(t *testing.T) {
	gate := bot.DisabledChatGate(nil)
	msg := newIncomingMessage("any-chat", "hi")
	if claimed := gate(msg, imbot.PlatformTelegram, "bot-1"); claimed {
		t.Fatal("gate with nil store claimed a message")
	}
}
