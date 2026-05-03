package tingly

import (
	"context"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
	itx "github.com/tingly-dev/tingly-box/imbot/interaction"
)

// InteractionAdapter implements interaction.Adapter for the tingly platform.
//
// It mirrors the Telegram adapter: callbacks travel through metadata as
// is_callback / callback_data, and the callback data uses the conventional
// "ia:<interactionID>:<value>" or "ia:<interactionID>:<requestID>:<value>"
// shape.
type InteractionAdapter struct {
	*itx.BaseAdapter
}

// NewInteractionAdapter creates an adapter that supports both interactions
// and message editing.
func NewInteractionAdapter() *InteractionAdapter {
	return &InteractionAdapter{
		BaseAdapter: itx.NewBaseAdapter(true, true),
	}
}

// BuildMarkup converts platform-agnostic interactions into the generic
// imbot inline-keyboard markup. Tingly does not need a platform-specific
// representation, so the imbot type goes through unchanged.
func (a *InteractionAdapter) BuildMarkup(interactions []itx.Interaction) (any, error) {
	rows := make([][]itx.InlineKeyboardButton, 0)
	for _, item := range interactions {
		switch item.Type {
		case itx.ActionSelect, itx.ActionConfirm, itx.ActionCancel:
			rows = append(rows, []itx.InlineKeyboardButton{{
				Text:         item.Label,
				CallbackData: formatCallbackData("ia", item.ID, item.Value),
			}})
		case itx.ActionNavigate:
			if len(rows) == 0 {
				rows = append(rows, []itx.InlineKeyboardButton{})
			}
			rows[len(rows)-1] = append(rows[len(rows)-1], itx.InlineKeyboardButton{
				Text:         item.Label,
				CallbackData: formatCallbackData("ia", item.ID, item.Value),
			})
		case itx.ActionInput:
			// Input actions don't translate to buttons.
			continue
		}
	}
	return itx.InlineKeyboardMarkup{InlineKeyboard: rows}, nil
}

// BuildFallbackText delegates to the package-default numbered-list helper.
func (a *InteractionAdapter) BuildFallbackText(message string, interactions []itx.Interaction) string {
	return itx.BuildFallbackText(message, interactions, "Reply with number:", "Cancel")
}

// ParseResponse decodes a callback-query message into an InteractionResponse.
// For non-callback messages it returns (nil, nil) so the handler falls
// through to numbered-text parsing.
func (a *InteractionAdapter) ParseResponse(msg core.Message) (*itx.InteractionResponse, error) {
	if isCallback, _ := msg.Metadata["is_callback"].(bool); !isCallback {
		return nil, nil
	}
	callbackData, _ := msg.Metadata["callback_data"].(string)
	parts := strings.Split(callbackData, ":")
	if len(parts) < 3 || parts[0] != "ia" {
		return nil, itx.ErrNotInteraction
	}
	timestamp := time.Unix(msg.Timestamp, 0)
	if len(parts) >= 4 {
		return &itx.InteractionResponse{
			RequestID: parts[2],
			Action: itx.Interaction{
				ID:    parts[1],
				Value: parts[3],
			},
			Timestamp: timestamp,
		}, nil
	}
	return &itx.InteractionResponse{
		Action: itx.Interaction{
			ID:    parts[1],
			Value: parts[2],
		},
		Timestamp: timestamp,
	}, nil
}

// UpdateMessage edits an existing message via the bot. Tingly supports
// editing, so we delegate to bot.EditMessage.
func (a *InteractionAdapter) UpdateMessage(ctx context.Context, bot core.Bot, chatID, messageID, text string, interactions []itx.Interaction) error {
	return bot.EditMessage(ctx, messageID, text)
}

func formatCallbackData(parts ...string) string {
	return strings.Join(parts, ":")
}
