package remoteagent

import (
	"testing"

	"github.com/tingly-dev/tingly-box/imbot"
)

func TestPersistentPoolKey_DistinguishesPlatformBotAndChat(t *testing.T) {
	base := persistentPoolKey(HandlerContext{
		Platform: imbot.Platform("telegram"),
		BotUUID:  "bot-a",
		ChatID:   "chat-1",
	}, "/proj")

	cases := []struct {
		name string
		hCtx HandlerContext
	}{
		{"different platform", HandlerContext{Platform: imbot.Platform("discord"), BotUUID: "bot-a", ChatID: "chat-1"}},
		{"different bot", HandlerContext{Platform: imbot.Platform("telegram"), BotUUID: "bot-b", ChatID: "chat-1"}},
		{"different chat", HandlerContext{Platform: imbot.Platform("telegram"), BotUUID: "bot-a", ChatID: "chat-2"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key := persistentPoolKey(c.hCtx, "/proj")
			if key == base {
				t.Fatalf("expected a distinct key from base %q, got the same value", base)
			}
		})
	}

	// A different project path for the same bot/chat is also a distinct key
	// — a chat's persistent session for one project must not answer for
	// another.
	if got := persistentPoolKey(HandlerContext{Platform: imbot.Platform("telegram"), BotUUID: "bot-a", ChatID: "chat-1"}, "/other"); got == base {
		t.Fatal("expected a distinct key for a different project path")
	}

	// Identical inputs must produce identical keys (Acquire/Open rely on this).
	again := persistentPoolKey(HandlerContext{
		Platform: imbot.Platform("telegram"),
		BotUUID:  "bot-a",
		ChatID:   "chat-1",
	}, "/proj")
	if again != base {
		t.Fatalf("expected a stable key, got %q and %q", base, again)
	}
}
