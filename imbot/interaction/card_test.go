package interaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCardBuilder_New(t *testing.T) {
	card := NewCard("test-card").Build()

	assert.Equal(t, "test-card", card.ID)
	assert.NotNil(t, card.Sections)
	assert.Empty(t, card.Sections)
	assert.NotNil(t, card.Actions)
	assert.Empty(t, card.Actions)
	assert.NotNil(t, card.Meta)
	assert.Empty(t, card.Meta)
}

func TestCardBuilder_WithTitleAndText(t *testing.T) {
	card := NewCard("test-card").
		WithTitle("Test Title").
		WithText("Test text").
		Build()

	assert.Equal(t, "Test Title", card.Title)
	assert.Equal(t, "Test text", card.Text)
}

func TestCardBuilder_AddSection(t *testing.T) {
	section := NewCardSection().
		WithTitle("Details").
		WithText("Build information").
		AddField("Status", "Running").
		AddShortField("Agent", "Claude").
		Build()

	card := NewCard("test-card").AddSection(section).Build()

	assert.Len(t, card.Sections, 1)
	assert.Equal(t, "Details", card.Sections[0].Title)
	assert.Equal(t, "Build information", card.Sections[0].Text)
	assert.Len(t, card.Sections[0].Fields, 2)
	assert.Equal(t, "Status", card.Sections[0].Fields[0].Label)
	assert.Equal(t, "Running", card.Sections[0].Fields[0].Value)
	assert.False(t, card.Sections[0].Fields[0].Short)
	assert.Equal(t, "Agent", card.Sections[0].Fields[1].Label)
	assert.True(t, card.Sections[0].Fields[1].Short)
}

func TestCardBuilder_AddAction(t *testing.T) {
	callbackAction := CallbackCardAction("clear", "Clear", "action:clear").
		WithType(ActionCustom).
		WithStyle(CardActionStyleDanger).
		Build()
	urlAction := URLCardAction("docs", "Docs", "https://example.com").Build()

	card := NewCard("test-card").AddActions(callbackAction, urlAction).Build()

	assert.Len(t, card.Actions, 2)
	assert.Equal(t, "clear", card.Actions[0].ID)
	assert.Equal(t, "Clear", card.Actions[0].Label)
	assert.Equal(t, "action:clear", card.Actions[0].Value)
	assert.Equal(t, ActionCustom, card.Actions[0].Type)
	assert.Equal(t, CardActionStyleDanger, card.Actions[0].Style)
	assert.Equal(t, "docs", card.Actions[1].ID)
	assert.Equal(t, "https://example.com", card.Actions[1].URL)
	assert.Equal(t, CardActionStyleDefault, card.Actions[1].Style)
}

func TestCardBuilder_WithMeta(t *testing.T) {
	card := NewCard("test-card").
		WithMeta("source", "remote_control").
		Build()
	action := NewCardAction("approve", "Approve").
		WithMeta("request_id", "req-1").
		Build()

	assert.Equal(t, "remote_control", card.Meta["source"])
	assert.Equal(t, "req-1", action.Meta["request_id"])
}

func TestCardBuilder_Chaining(t *testing.T) {
	card := NewCard("result").
		WithTitle("Task Result").
		WithText("Done").
		AddSectionBuilder(NewCardSection().WithTitle("Summary").AddField("Files", "3")).
		AddActionBuilder(CallbackCardAction("continue", "Continue", "action:continue").WithStyle(CardActionStylePrimary)).
		AddActionBuilder(URLCardAction("open", "Open", "https://example.com")).
		Build()

	assert.Equal(t, "result", card.ID)
	assert.Equal(t, "Task Result", card.Title)
	assert.Equal(t, "Done", card.Text)
	assert.Len(t, card.Sections, 1)
	assert.Len(t, card.Actions, 2)
	assert.Equal(t, CardActionStylePrimary, card.Actions[0].Style)
}

func TestCard_ToInteractions(t *testing.T) {
	card := NewCard("test-card").
		AddActionBuilder(NewCardAction("run", "Run").
			WithValue("action:run").
			WithType(ActionConfirm).
			WithMeta("scope", "local")).
		Build()

	interactions := card.ToInteractions()

	assert.Len(t, interactions, 1)
	assert.Equal(t, "run", interactions[0].ID)
	assert.Equal(t, "Run", interactions[0].Label)
	assert.Equal(t, "action:run", interactions[0].Value)
	assert.Equal(t, ActionConfirm, interactions[0].Type)
	assert.Equal(t, "local", interactions[0].Meta["scope"])

	card.Actions[0].Meta["scope"] = "changed"
	assert.Equal(t, "local", interactions[0].Meta["scope"])
}

func TestCardBuilder_ClearSectionsAndActions(t *testing.T) {
	card := NewCard("test-card").
		AddSectionBuilder(NewCardSection().WithTitle("Section"))
	assert.Len(t, card.Build().Sections, 1)

	card.ClearSections()
	assert.Empty(t, card.Build().Sections)

	card.AddActionBuilder(CallbackCardAction("action", "Action", "action:value"))
	assert.Len(t, card.Build().Actions, 1)

	card.ClearActions()
	assert.Empty(t, card.Build().Actions)
}

func TestCardBuilder_NilBuilders(t *testing.T) {
	card := NewCard("test-card").
		AddSectionBuilder(nil).
		AddActionBuilder(nil).
		Build()

	assert.Empty(t, card.Sections)
	assert.Empty(t, card.Actions)
}
