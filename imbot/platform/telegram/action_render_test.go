package telegram

import (
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/interaction"
)

func TestBuildInlineKeyboardPreservesRows(t *testing.T) {
	set := core.NewActionSet().
		AddRow(core.Action{Label: "A", CallbackData: "a"}, core.Action{Label: "B", CallbackData: "b"}).
		AddRow(core.Action{Label: "C", CallbackData: "c"})

	kb := BuildInlineKeyboard(set)
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("rows = %d, want 2", len(kb.InlineKeyboard))
	}
	if len(kb.InlineKeyboard[0]) != 2 || len(kb.InlineKeyboard[1]) != 1 {
		t.Fatalf("row layout not preserved: %v", kb.InlineKeyboard)
	}
	if kb.InlineKeyboard[0][0].CallbackData != "a" {
		t.Errorf("callback = %q, want a", kb.InlineKeyboard[0][0].CallbackData)
	}
}

func TestBuildInlineKeyboardURLAction(t *testing.T) {
	kb := BuildInlineKeyboard(core.NewActionSet().AddRow(
		core.Action{Label: "Docs", URL: "https://example.test", Kind: core.ActionOpenURL},
	))
	if got := kb.InlineKeyboard[0][0].URL; got != "https://example.test" {
		t.Errorf("URL = %q", got)
	}
}

// TestBuildInlineKeyboardDropsUnsendableAction guards against emitting a button
// with neither callback data nor a URL, which Telegram rejects outright — and
// a rejected button fails the whole send, not just the button.
func TestBuildInlineKeyboardDropsUnsendableAction(t *testing.T) {
	kb := BuildInlineKeyboard(core.NewActionSet().AddRow(core.Action{Label: "orphan"}))
	if len(kb.InlineKeyboard) != 0 {
		t.Errorf("expected the unsendable action to be dropped, got %v", kb.InlineKeyboard)
	}
}

// TestWebAppButton covers the Tier 3 escape hatch: Telegram-only capabilities
// are reachable through this package's constructors, not through neutral
// fields, and they still declare what other platforms should do instead.
func TestWebAppButton(t *testing.T) {
	action := WebAppButton("Dashboard", "https://example.test/app")

	if action.Fallback != core.FallbackAsURL {
		t.Errorf("Fallback = %q, want as_url — other platforms must degrade explicitly", action.Fallback)
	}
	if action.URL == "" {
		t.Error("the neutral URL must be set so the fallback has something to use")
	}

	kb := BuildInlineKeyboard(core.NewActionSet().AddRow(action))
	btn := kb.InlineKeyboard[0][0]
	if btn.WebApp == nil {
		t.Fatal("expected a web_app button on Telegram")
	}
	if btn.WebApp.URL != "https://example.test/app" {
		t.Errorf("web_app URL = %q", btn.WebApp.URL)
	}
	if btn.CallbackData != "" {
		t.Error("a web_app button must not also carry callback data")
	}
}

func TestSwitchInlineButton(t *testing.T) {
	kb := BuildInlineKeyboard(core.NewActionSet().AddRow(SwitchInlineButton("Share", "query")))
	btn := kb.InlineKeyboard[0][0]
	if btn.SwitchInlineQuery == nil || *btn.SwitchInlineQuery != "query" {
		t.Errorf("expected switch_inline_query to be set, got %+v", btn)
	}
}

func TestKeyboardToActionSetRoundTrip(t *testing.T) {
	kb := interaction.NewKeyboardBuilder().
		AddRow(interaction.CallbackButton("Yes", "yes"), interaction.CallbackButton("No", "no")).
		AddRow(interaction.URLButton("Docs", "https://example.test"))

	out := BuildInlineKeyboard(kb.BuildActions())
	if len(out.InlineKeyboard) != 2 {
		t.Fatalf("rows = %d, want 2", len(out.InlineKeyboard))
	}
	if out.InlineKeyboard[0][1].CallbackData != "no" {
		t.Errorf("callback = %q, want no", out.InlineKeyboard[0][1].CallbackData)
	}
	if out.InlineKeyboard[1][0].URL != "https://example.test" {
		t.Errorf("URL = %q", out.InlineKeyboard[1][0].URL)
	}
}
