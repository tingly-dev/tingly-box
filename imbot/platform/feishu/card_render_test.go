package feishu

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/interaction"
)

func TestRenderCardRequiresID(t *testing.T) {
	if _, err := RenderCard(interaction.Card{Text: "hi"}); err == nil {
		t.Fatal("expected an error for a card with no ID")
	}
}

// TestRenderCardActionsSurviveAsButtons pins the property that actually matters:
// a neutral card's actions must come out as Feishu buttons carrying the
// callback value. remote_control's action menu (Clear / CD / Project) is
// unusable on Feishu if this silently produces a card with no buttons.
func TestRenderCardActionsSurviveAsButtons(t *testing.T) {
	card := interaction.NewCard("remote_control_action_menu").
		WithTitle("Title").
		WithText("Body").
		AddActions(
			interaction.CallbackCardAction("clear", "🗑 Clear", "action:clear").
				WithStyle(interaction.CardActionStyleDanger).
				Build(),
			interaction.CallbackCardAction("bind", "📁 CD", "action:bind").Build(),
		).
		Build()

	out, err := RenderCard(card)
	if err != nil {
		t.Fatalf("RenderCard: %v", err)
	}

	// The output must be valid JSON — it is handed straight to the Lark API.
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("rendered card is not valid JSON: %v\n%s", err, out)
	}

	for _, want := range []string{"🗑 Clear", "📁 CD", "action:clear", "action:bind", "Title", "Body"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered card is missing %q\n%s", want, out)
		}
	}
}

func TestRenderCardSectionFields(t *testing.T) {
	card := interaction.NewCard("card").
		AddSection(interaction.CardSection{
			Title:  "Session",
			Fields: []interaction.CardField{{Label: "Project", Value: "/srv/app"}},
		}).
		Build()

	out, err := RenderCard(card)
	if err != nil {
		t.Fatalf("RenderCard: %v", err)
	}
	for _, want := range []string{"Session", "Project", "/srv/app"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered card is missing %q\n%s", want, out)
		}
	}
}

// TestRenderCardStyleMapping guards the neutral-style → Feishu-button-type
// mapping, which is the one place this renderer makes a real translation
// decision rather than copying text through.
func TestRenderCardStyleMapping(t *testing.T) {
	tests := []struct {
		style interaction.CardActionStyle
		want  string
	}{
		{interaction.CardActionStyleDanger, "danger"},
		{interaction.CardActionStylePrimary, "primary"},
		{interaction.CardActionStyleDefault, "default"},
	}

	for _, tt := range tests {
		card := interaction.NewCard("card").
			AddActions(interaction.CallbackCardAction("a", "Label", "v").WithStyle(tt.style).Build()).
			Build()

		out, err := RenderCard(card)
		if err != nil {
			t.Fatalf("RenderCard(%s): %v", tt.style, err)
		}
		if !strings.Contains(out, tt.want) {
			t.Errorf("style %q: expected button type %q in output\n%s", tt.style, tt.want, out)
		}
	}
}
