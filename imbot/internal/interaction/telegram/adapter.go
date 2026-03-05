package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/interaction"
)

// Adapter implements interaction.Adapter for Telegram
type Adapter struct {
	*interaction.BaseAdapter
}

// NewAdapter creates a new Telegram interaction adapter
func NewAdapter() *Adapter {
	return &Adapter{
		BaseAdapter: interaction.NewBaseAdapter(true, true), // Supports interactions and editing
	}
}

// BuildMarkup creates Telegram inline keyboard markup from interactions
func (a *Adapter) BuildMarkup(interactions []interaction.Interaction) (any, error) {
	kb := &keyboardBuilder{
		rows: make([][]tgbotapi.InlineKeyboardButton, 0),
	}

	for _, item := range interactions {
		switch item.Type {
		case interaction.ActionSelect, interaction.ActionConfirm, interaction.ActionCancel:
			callbackData := formatCallbackData("ia", item.ID, item.Value)
			kb.AddRow(tgbotapi.InlineKeyboardButton{
				Text:         item.Label,
				CallbackData: &callbackData,
			})

		case interaction.ActionNavigate:
			callbackData := formatCallbackData("ia", item.ID, item.Value)
			kb.AddButton(tgbotapi.InlineKeyboardButton{
				Text:         item.Label,
				CallbackData: &callbackData,
			})

		case interaction.ActionInput:
			// Input actions don't translate to buttons, skip
			continue
		}
	}

	return tgbotapi.NewInlineKeyboardMarkup(kb.rows...), nil
}

// BuildFallbackText creates numbered text options for text mode
func (a *Adapter) BuildFallbackText(message string, interactions []interaction.Interaction) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString("\n\n")
	sb.WriteString("Reply with number:\n")

	for i, item := range interactions {
		if item.Type == interaction.ActionSelect || item.Type == interaction.ActionConfirm {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.Label))
		}
	}
	sb.WriteString("0. Cancel")

	return sb.String()
}

// ParseResponse parses Telegram callback queries or returns nil for text handling
func (a *Adapter) ParseResponse(msg core.Message) (*interaction.InteractionResponse, error) {
	// Check if this is a callback query
	if isCallback, _ := msg.Metadata["is_callback"].(bool); isCallback {
		if callbackData, ok := msg.Metadata["callback_data"].(string); ok {
			parts := parseCallbackData(callbackData)
			if len(parts) >= 3 && parts[0] == "ia" {
				// Format: ia:interactionID:value
				// Or: ia:interactionID:requestID:value (for responses)
				timestamp := time.Unix(msg.Timestamp, 0)
				if len(parts) >= 4 {
					return &interaction.InteractionResponse{
						RequestID: parts[2],
						Action: interaction.Interaction{
							ID:    parts[1],
							Value: parts[3],
						},
						Timestamp: timestamp,
					}, nil
				}
				// Simple format without requestID
				return &interaction.InteractionResponse{
					Action: interaction.Interaction{
						ID:    parts[1],
						Value: parts[2],
					},
					Timestamp: timestamp,
				}, nil
			}
		}
		return nil, interaction.ErrNotInteraction
	}

	// Text replies are handled by Handler.parseTextResponse
	return nil, nil
}

// UpdateMessage edits a Telegram message
func (a *Adapter) UpdateMessage(ctx context.Context, bot core.Bot, chatID, messageID, text string, interactions []interaction.Interaction) error {
	// Need to use platform-specific bot interface
	// This is a placeholder - actual implementation would use the platform adapter
	return interaction.ErrNotSupported
}

// keyboardBuilder helps build Telegram inline keyboards
type keyboardBuilder struct {
	rows [][]tgbotapi.InlineKeyboardButton
}

// AddRow adds a new row with buttons
func (b *keyboardBuilder) AddRow(buttons ...tgbotapi.InlineKeyboardButton) {
	b.rows = append(b.rows, buttons)
}

// AddButton adds a button to the last row
func (b *keyboardBuilder) AddButton(button tgbotapi.InlineKeyboardButton) {
	if len(b.rows) == 0 {
		b.rows = append(b.rows, []tgbotapi.InlineKeyboardButton{})
	}
	b.rows[len(b.rows)-1] = append(b.rows[len(b.rows)-1], button)
}

// Callback data helpers

// formatCallbackData formats callback data parts with colon separator
func formatCallbackData(parts ...string) string {
	return strings.Join(parts, ":")
}

// parseCallbackData parses callback data into parts
func parseCallbackData(data string) []string {
	return strings.Split(data, ":")
}
