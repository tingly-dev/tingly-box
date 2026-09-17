package remoteagent

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/imbot"
)

func TestPersistentPoolKey_DistinguishesBotChatAndProject(t *testing.T) {
	base := persistentPoolKey(HandlerContext{
		Platform: imbot.Platform("telegram"),
		BotUUID:  "bot-a",
		ChatID:   "chat-1",
	}, "/proj")

	cases := []struct {
		name string
		hCtx HandlerContext
	}{
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

	// Platform is deliberately NOT part of the key: BotUUID alone already
	// uniquely identifies one bot on one platform (it's the imbot_settings
	// primary key), so including platform too would be redundant and would
	// break EvictPersistentSessionsForBot's "<botUUID>|" prefix match.
	if got := persistentPoolKey(HandlerContext{Platform: imbot.Platform("discord"), BotUUID: "bot-a", ChatID: "chat-1"}, "/proj"); got != base {
		t.Fatalf("platform must not affect the key: got %q, want %q", got, base)
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

func TestPersistentPoolKey_HasBotUUIDPrefix(t *testing.T) {
	key := persistentPoolKey(HandlerContext{
		Platform: imbot.Platform("telegram"),
		BotUUID:  "bot-a",
		ChatID:   "chat-1",
	}, "/proj")

	if !strings.HasPrefix(key, "bot-a|") {
		t.Fatalf("EvictPersistentSessionsForBot matches on a %q prefix; got key %q", "bot-a|", key)
	}
}
