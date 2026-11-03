package remoteagent

import (
	"strings"
	"testing"
)

func TestHelpTextNamesTheBotAndKeepsTheIDsToPaste(t *testing.T) {
	got := helpText("Telegram · Ops bot", "42", "-100123")
	if !strings.HasPrefix(got, "Bot: Telegram · Ops bot\n") {
		t.Fatalf("help must open with the bot's name:\n%s", got)
	}
	for _, want := range []string{"Your ID: 42", "Chat ID: -100123", "/status:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help lost %q:\n%s", want, got)
		}
	}
}

func TestHelpTextOmitsBotLineWhenUnnamed(t *testing.T) {
	got := helpText("", "42", "")
	if strings.Contains(got, "Bot:") {
		t.Fatalf("an unnamed bot has nothing to say:\n%s", got)
	}
	if strings.Contains(got, "Chat ID") {
		t.Fatalf("no chat id given, none printed:\n%s", got)
	}
}

func TestPairedMessageNamesTheBot(t *testing.T) {
	if got := pairedMessage("Telegram · Ops bot"); !strings.Contains(got, "Paired with Telegram · Ops bot") {
		t.Fatalf("got %q", got)
	}
	if got := pairedMessage(""); got != "✅ Paired. You can now send commands to this bot." {
		t.Fatalf("got %q", got)
	}
}
