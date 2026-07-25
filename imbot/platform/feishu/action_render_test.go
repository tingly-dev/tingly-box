package feishu

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/interaction"
)

// TestBuildActionCardRendersButtons is the regression test for the defect this
// seam exists to fix: remote_control's action menu reached Feishu users as a
// card with no buttons, because the caller pre-rendered a go-telegram markup
// that this package's type switch did not recognise.
func TestBuildActionCardRendersButtons(t *testing.T) {
	set := core.NewActionSet().
		AddRow(
			core.Action{ID: "clear", Label: "🗑 Clear", CallbackData: "action:clear"},
			core.Action{ID: "bind", Label: "📁 CD", CallbackData: "action:bind"},
		).
		AddRow(core.Action{ID: "project", Label: "🔧 Project", CallbackData: "action:project"})

	out, err := buildActionCard("Task done.", set).String()
	if err != nil {
		t.Fatalf("card serialize: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("card is not valid JSON: %v\n%s", err, out)
	}

	// Feishu flattens rows, but every action must survive with its callback.
	for _, want := range []string{
		"🗑 Clear", "📁 CD", "🔧 Project",
		"action:clear", "action:bind", "action:project",
		"Task done.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("card is missing %q\n%s", want, out)
		}
	}
	if got := strings.Count(out, `"tag":"button"`); got != 3 {
		t.Errorf("expected 3 buttons, got %d\n%s", got, out)
	}
}

func TestBuildActionCardWithoutActions(t *testing.T) {
	out, err := buildActionCard("just text", core.NewActionSet()).String()
	if err != nil {
		t.Fatalf("card serialize: %v", err)
	}
	if strings.Contains(out, `"tag":"button"`) {
		t.Errorf("expected no buttons\n%s", out)
	}
	if !strings.Contains(out, "just text") {
		t.Errorf("expected the text to survive\n%s", out)
	}
}

// TestBuildActionCardTier3Fallback covers the escape-hatch contract: an action
// carrying another platform's Tier 3 payload must degrade here per its
// FallbackPolicy, not vanish and not leak the other platform's shape.
func TestBuildActionCardTier3Fallback(t *testing.T) {
	// FallbackAsURL: a Telegram mini-app button becomes a plain link.
	asURL := core.Action{
		Label:    "Open dashboard",
		URL:      "https://example.test/dash",
		Kind:     core.ActionOpenMiniApp,
		Fallback: core.FallbackAsURL,
		Ext:      map[core.Platform]any{core.PlatformTelegram: struct{ x int }{1}},
	}
	out, err := buildActionCard("hi", core.NewActionSet().AddRow(asURL)).String()
	if err != nil {
		t.Fatalf("card serialize: %v", err)
	}
	if !strings.Contains(out, "https://example.test/dash") {
		t.Errorf("FallbackAsURL should keep the link\n%s", out)
	}

	// FallbackDrop: a capability with no Feishu equivalent is omitted.
	dropped := core.Action{
		Label:    "Switch inline",
		Kind:     core.ActionOpenMiniApp,
		URL:      "https://example.test/x",
		Fallback: core.FallbackDrop,
		Ext:      map[core.Platform]any{core.PlatformTelegram: struct{ x int }{1}},
	}
	out, err = buildActionCard("hi", core.NewActionSet().AddRow(dropped)).String()
	if err != nil {
		t.Fatalf("card serialize: %v", err)
	}
	if strings.Contains(out, `"tag":"button"`) {
		t.Errorf("FallbackDrop should omit the action\n%s", out)
	}
}

func TestActionSetFromLegacyMarkup(t *testing.T) {
	kb := interaction.InlineKeyboardMarkup{
		InlineKeyboard: [][]interaction.InlineKeyboardButton{
			{{Text: "Yes", CallbackData: "yes"}},
		},
	}
	set := actionSetFromLegacyMarkup(kb)
	if set.IsEmpty() {
		t.Fatal("expected the legacy generic markup to decode")
	}
	if got := set.Flatten()[0].Label; got != "Yes" {
		t.Errorf("label = %q, want Yes", got)
	}

	// A shape nobody recognises must report as empty so the caller can warn,
	// rather than silently shipping a button-less card.
	if !actionSetFromLegacyMarkup(struct{ Nope bool }{}).IsEmpty() {
		t.Error("expected an unknown markup shape to decode to nothing")
	}
}
