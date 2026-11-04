package core

import "testing"

func TestBotTagSeparatesBotsOnOnePlatform(t *testing.T) {
	a := BotTag(PlatformTelegram, "1730600000123456789-imbot")
	b := BotTag(PlatformTelegram, "1730600044998877665-imbot")
	if a != "telegram/…456789" || b != "telegram/…877665" {
		t.Fatalf("got %q and %q", a, b)
	}
	if got := BotTag(PlatformFeishu, "mock-1"); got != "feishu/mock-1" {
		t.Fatalf("short ids stay whole, got %q", got)
	}
	if got := BotTag(PlatformFeishu, ""); got != "feishu" {
		t.Fatalf("got %q", got)
	}
}

func TestNewBaseBotLogsWithItsIdentity(t *testing.T) {
	b := NewBaseBot(&Config{UUID: "1730600000123456789-imbot", Platform: PlatformTelegram})
	l, ok := b.Logger().(*defaultLogger)
	if !ok {
		t.Fatalf("unexpected logger type %T", b.Logger())
	}
	if l.prefix != "[imbot telegram/…456789]" {
		t.Fatalf("prefix %q", l.prefix)
	}
	anon := NewBaseBot(&Config{})
	if l2, ok := anon.Logger().(*defaultLogger); !ok || l2.prefix != "[imbot]" {
		t.Fatalf("a bot without an id keeps the generic prefix")
	}
}
