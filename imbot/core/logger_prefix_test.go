package core

import "testing"

func TestBotLogPrefixSeparatesBotsOnOnePlatform(t *testing.T) {
	a := BotLogPrefix(PlatformTelegram, "1730600000123456789-imbot")
	b := BotLogPrefix(PlatformTelegram, "1730600044998877665-imbot")
	if a != "[imbot telegram/…456789]" || b != "[imbot telegram/…877665]" {
		t.Fatalf("got %q and %q", a, b)
	}
	if got := BotLogPrefix(PlatformFeishu, "mock-1"); got != "[imbot feishu/mock-1]" {
		t.Fatalf("short ids stay whole, got %q", got)
	}
	if got := BotLogPrefix(PlatformFeishu, ""); got != "[imbot feishu]" {
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

func TestBotTagIsThePrefixWithoutBrackets(t *testing.T) {
	if got := BotTag(PlatformTelegram, "1730600000123456789-imbot"); got != "telegram/…456789" {
		t.Fatalf("got %q", got)
	}
	if BotLogPrefix(PlatformTelegram, "x") != "[imbot "+BotTag(PlatformTelegram, "x")+"]" {
		t.Fatal("prefix must wrap the tag")
	}
}
