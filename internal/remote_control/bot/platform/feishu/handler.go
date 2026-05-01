package feishu

import (
	"context"
	"fmt"

	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/platform"
)

// Handler implements platform-specific handling for Feishu
type Handler struct {
	bot              imbot.Bot
	cardRenderer     *CardRenderer
	logger           *larkLogger
}

// NewHandler creates a new Feishu platform handler
func NewHandler(bot imbot.Bot) *Handler {
	return &Handler{
		bot:          bot,
		cardRenderer: NewCardRenderer(),
		logger:       &larkLogger{},
	}
}

// Platform returns the Feishu platform identifier
func (h *Handler) Platform() imbot.Platform {
	return imbot.PlatformFeishu
}

// SupportsFeature checks if Feishu supports a specific feature
func (h *Handler) SupportsFeature(feature platform.Feature) bool {
	switch feature {
	case platform.FeatureInlineKeyboard,
		platform.FeatureCardRendering,
		platform.FeatureMessageEditing,
		platform.FeatureMediaUpload:
		return true
	case platform.FeatureVerbose,
		platform.FeatureMarkdown,
		platform.FeatureReactions,
		platform.FeatureKeyboardRemoval,
		platform.FeatureQRAuth:
		return false
	default:
		return false
	}
}

// HandlePlatformMessage handles Feishu-specific message preprocessing
func (h *Handler) HandlePlatformMessage(ctx *platform.Context) (bool, error) {
	// Check for card callback interactions
	if cardCallback, ok := ctx.Message.Metadata["card_callback"].(map[string]interface{}); ok && cardCallback != nil {
		// Card callbacks should be handled by the main handler
		return false, nil
	}

	// No special preprocessing needed for Feishu
	return false, nil
}

// SendMessage sends a message with Feishu-specific formatting
func (h *Handler) SendMessage(ctx context.Context, chatID string, opts *imbot.SendMessageOptions) error {
	// Check if this is a card message
	if card, ok := opts.Metadata["card"].(imbot.Card); ok {
		// Render card to Feishu format
		cardJSON, err := h.cardRenderer.Render(card)
		if err != nil {
			return fmt.Errorf("failed to render card: %w", err)
		}

		// Update opts with rendered card
		if opts.Metadata == nil {
			opts.Metadata = make(map[string]interface{})
		}
		opts.Metadata["card_json"] = cardJSON
	}

	return nil
}

// SendCard sends a card message to Feishu
func (h *Handler) SendCard(ctx context.Context, chatID string, card imbot.Card) error {
	cardJSON, err := h.cardRenderer.Render(card)
	if err != nil {
		return fmt.Errorf("failed to render card: %w", err)
	}

	_, err = h.bot.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
		Metadata: map[string]interface{}{
			"card_json": cardJSON,
		},
	})
	return err
}

// CardRenderer converts imbot.Card to Feishu card JSON format
type CardRenderer struct{}

// NewCardRenderer creates a new Feishu card renderer
func NewCardRenderer() *CardRenderer {
	return &CardRenderer{}
}

// Render converts an imbot.Card to Feishu card JSON string
func (r *CardRenderer) Render(card imbot.Card) (string, error) {
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
func (r *CardRenderer) buildCardElements(card imbot.Card) []larkcard.MessageCardElement {
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
func (r *CardRenderer) buildSectionElements(section imbot.CardSection) []larkcard.MessageCardElement {
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
func (r *CardRenderer) buildActionButton(action imbot.CardAction) larkcard.MessageCardActionElement {
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
func (r *CardRenderer) mapActionStyleToButtonType(style imbot.CardActionStyle) larkcard.MessageCardButtonType {
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
func (r *CardRenderer) fieldsToMarkdown(fields []imbot.CardField) string {
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

// larkLogger is a minimal logger for Feishu operations
type larkLogger struct{}

// Debug logs debug messages
func (l *larkLogger) Debug(args ...interface{}) {}

// Info logs info messages
func (l *larkLogger) Info(args ...interface{}) {}

// Warn logs warning messages
func (l *larkLogger) Warn(args ...interface{}) {}

// Error logs error messages
func (l *larkLogger) Error(args ...interface{}) {}
