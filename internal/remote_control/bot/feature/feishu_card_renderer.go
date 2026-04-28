package feature

import (
	"fmt"

	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	"github.com/tingly-dev/tingly-box/imbot"
)

// FeishuCardRenderer converts imbot.Card to Feishu card JSON format
// This is defined in internal/remote_control to avoid import cycles with imbot/platform packages
type FeishuCardRenderer struct{}

// NewFeishuCardRenderer creates a new Feishu card renderer
func NewFeishuCardRenderer() *FeishuCardRenderer {
	return &FeishuCardRenderer{}
}

// Render converts an imbot.Card to Feishu card JSON string
func (r *FeishuCardRenderer) Render(card imbot.Card) (string, error) {
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
func (r *FeishuCardRenderer) buildCardElements(card imbot.Card) []larkcard.MessageCardElement {
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
func (r *FeishuCardRenderer) buildSectionElements(section imbot.CardSection) []larkcard.MessageCardElement {
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
func (r *FeishuCardRenderer) buildActionButton(action imbot.CardAction) larkcard.MessageCardActionElement {
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

// mapActionStyleToButtonType maps imbot.CardActionStyle to Feishu button type
func (r *FeishuCardRenderer) mapActionStyleToButtonType(style imbot.CardActionStyle) larkcard.MessageCardButtonType {
	switch style {
	case imbot.CardActionStylePrimary:
		return larkcard.MessageCardButtonTypePrimary
	case imbot.CardActionStyleDanger:
		return larkcard.MessageCardButtonTypeDanger
	default:
		return larkcard.MessageCardButtonTypeDefault
	}
}

// fieldsToMarkdown converts card fields to markdown table format
func (r *FeishuCardRenderer) fieldsToMarkdown(fields []imbot.CardField) string {
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
