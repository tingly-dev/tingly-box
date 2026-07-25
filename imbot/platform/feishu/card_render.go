package feishu

import (
	"fmt"

	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	"github.com/tingly-dev/tingly-box/imbot/interaction"
)

// CardRenderer converts a platform-neutral interaction.Card into the Feishu
// card JSON the Lark API expects.
//
// It lives here, next to the Feishu bot, because rendering a neutral card into
// a platform's wire format is the platform's job. It previously lived in
// internal/remote_control/bot/feature with a note claiming imbot/platform
// packages could not be imported without a cycle; that was not the case — the
// renderer only needs interaction.Card, which this package already depends on.
type CardRenderer struct{}

// NewCardRenderer creates a new Feishu card renderer.
func NewCardRenderer() *CardRenderer {
	return &CardRenderer{}
}

// RenderCard converts a neutral card to Feishu card JSON. Convenience wrapper
// for callers that do not need to hold a renderer.
func RenderCard(card interaction.Card) (string, error) {
	return NewCardRenderer().Render(card)
}

// Render converts an interaction.Card to a Feishu card JSON string.
func (r *CardRenderer) Render(card interaction.Card) (string, error) {
	if card.ID == "" {
		return "", fmt.Errorf("card ID cannot be empty")
	}

	elements := r.buildCardElements(card)

	// Build the card
	wideScreen := true
	larkCard := larkcard.NewMessageCard().
		Config(larkcard.NewMessageCardConfig().WideScreenMode(wideScreen)).
		Elements(elements)

	// Serialize to JSON string
	cardStr, err := larkCard.String()
	if err != nil {
		return "", fmt.Errorf("failed to serialize card: %w", err)
	}

	return cardStr, nil
}

// buildCardElements converts card sections and actions to Feishu card elements
func (r *CardRenderer) buildCardElements(card interaction.Card) []larkcard.MessageCardElement {
	var elements []larkcard.MessageCardElement

	// Add title if present
	if card.Title != "" {
		divElement := larkcard.NewMessageCardDiv().
			Text(larkcard.NewMessageCardLarkMd().Content(card.Title))
		elements = append(elements, divElement)
	}

	// Add main text if present
	if card.Text != "" {
		divElement := larkcard.NewMessageCardDiv().
			Text(larkcard.NewMessageCardLarkMd().Content(card.Text))
		elements = append(elements, divElement)
	}

	// Add sections
	for _, section := range card.Sections {
		sectionElements := r.buildSectionElements(section)
		elements = append(elements, sectionElements...)
	}

	// Build action buttons
	if len(card.Actions) > 0 {
		var buttons []larkcard.MessageCardActionElement
		for _, action := range card.Actions {
			button := r.buildActionButton(action)
			buttons = append(buttons, button)
		}

		layout := larkcard.MessageCardActionLayoutFlow
		action := larkcard.NewMessageCardAction().
			Layout(&layout).
			Actions(buttons)
		elements = append(elements, action)
	}

	return elements
}

// buildSectionElements converts a card section to Feishu card elements
func (r *CardRenderer) buildSectionElements(section interaction.CardSection) []larkcard.MessageCardElement {
	var elements []larkcard.MessageCardElement

	// Section title
	if section.Title != "" {
		divElement := larkcard.NewMessageCardDiv().
			Text(larkcard.NewMessageCardLarkMd().Content("**" + section.Title + "**"))
		elements = append(elements, divElement)
	}

	// Section text
	if section.Text != "" {
		divElement := larkcard.NewMessageCardDiv().
			Text(larkcard.NewMessageCardLarkMd().Content(section.Text))
		elements = append(elements, divElement)
	}

	// Section fields as markdown table
	if len(section.Fields) > 0 {
		mdText := r.fieldsToMarkdown(section.Fields)
		divElement := larkcard.NewMessageCardDiv().
			Text(larkcard.NewMessageCardLarkMd().Content(mdText))
		elements = append(elements, divElement)
	}

	return elements
}

// buildActionButton converts a card action to Feishu button element
func (r *CardRenderer) buildActionButton(action interaction.CardAction) larkcard.MessageCardActionElement {
	button := larkcard.NewMessageCardEmbedButton().
		Text(larkcard.NewMessageCardPlainText().Content(action.Label))

	// Map action style to Feishu button type
	buttonType := r.mapActionStyleToButtonType(action.Style)
	button.Type(buttonType)

	// Set disabled state
	if action.Disabled {
		button.Type(larkcard.MessageCardButtonTypeDefault)
	}

	// Set value/call back data
	button.Value(map[string]interface{}{
		"callback": action.Value,
		"actionId": action.ID,
	})

	return button
}

// mapActionStyleToButtonType maps interaction.CardActionStyle to Feishu button type
func (r *CardRenderer) mapActionStyleToButtonType(style interaction.CardActionStyle) larkcard.MessageCardButtonType {
	switch style {
	case interaction.CardActionStylePrimary:
		return larkcard.MessageCardButtonTypePrimary
	case interaction.CardActionStyleDanger:
		return larkcard.MessageCardButtonTypeDanger
	default:
		return larkcard.MessageCardButtonTypeDefault
	}
}

// fieldsToMarkdown converts card fields to markdown table format
func (r *CardRenderer) fieldsToMarkdown(fields []interaction.CardField) string {
	if len(fields) == 0 {
		return ""
	}

	// Build markdown table
	md := "| Field | Value |\n|------|------|\n"
	for _, field := range fields {
		md += fmt.Sprintf("| %s | %s |\n", field.Label, field.Value)
	}

	return md
}
