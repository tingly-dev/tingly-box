package feishu

import (
	"context"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	itx "github.com/tingly-dev/tingly-box/imbot/internal/interaction"
)

// InteractionAdapter implements itx.Adapter for Feishu
type InteractionAdapter struct {
	*itx.BaseAdapter
}

// NewInteractionAdapter creates a new Feishu interaction adapter
func NewInteractionAdapter() *InteractionAdapter {
	return &InteractionAdapter{
		BaseAdapter: itx.NewBaseAdapter(true, false), // Supports cards but no editing via stream mode
	}
}

// SupportsInteractions returns true - Feishu supports interactive cards
func (a *InteractionAdapter) SupportsInteractions() bool {
	return true
}

// BuildMarkup creates Feishu card markup from interactions
// Note: Feishu cards use a different format than Telegram keyboards
func (a *InteractionAdapter) BuildMarkup(interactions []itx.Interaction) (any, error) {
	// Feishu card structure
	// https://open.feishu.cn/document/ukTMukTMukTMuUTjNj4xMjYU
	card := a.buildCard(interactions)
	return card, nil
}

// buildCard builds a Feishu interactive card
func (a *InteractionAdapter) buildCard(interactions []itx.Interaction) map[string]interface{} {
	// Build button elements
	var elements []map[string]interface{}
	for _, item := range interactions {
		if item.Type == itx.ActionSelect || item.Type == itx.ActionConfirm || item.Type == itx.ActionCancel {
			element := map[string]interface{}{
				"tag": "button",
				"text": map[string]interface{}{
					"tag":     "plain_text",
					"content": item.Label,
				},
				"type": "primary",
			}
			if item.Value != "" {
				element["url"] = "" // For Feishu, we'd need a callback URL
			}
			elements = append(elements, element)
		}
	}

	// Build card structure
	card := map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"elements": []map[string]interface{}{
			{
				"tag":    "div",
				"fields": []map[string]interface{}{},
			},
			{
				"tag":     "action",
				"actions": elements,
			},
		},
	}

	return card
}

// BuildFallbackText creates numbered text options
// This is used when Mode=Text or when cards are not appropriate
func (a *InteractionAdapter) BuildFallbackText(message string, interactions []itx.Interaction) string {
	return itx.BuildFallbackText(message, interactions, "请回复数字：", "取消")
}

// ParseResponse parses Feishu interaction responses
// Feishu interactions come via card button clicks
func (a *InteractionAdapter) ParseResponse(msg core.Message) (*itx.InteractionResponse, error) {
	// Check if this is a card interaction callback
	if action, ok := msg.Metadata["action"].(string); ok {
		// Parse Feishu action callback
		// Format: ia:interactionID:value
		parts := strings.Split(action, ":")
		if len(parts) >= 3 && parts[0] == "ia" {
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
				Action:    itx.Interaction{ID: parts[1], Value: parts[2]},
				Timestamp: timestamp,
			}, nil
		}
		return nil, itx.ErrNotInteraction
	}

	// Text replies are handled by Handler.parseTextResponse
	return nil, nil
}

// UpdateMessage updates a Feishu message
// Note: Feishu message editing is limited in stream mode
func (a *InteractionAdapter) UpdateMessage(ctx context.Context, bot core.Bot, chatID, messageID, text string, interactions []itx.Interaction) error {
	// Feishu doesn't support message editing via the same API
	// Would need to use the message update API separately
	return itx.ErrNotSupported
}

// CanEditMessages returns false - Feishu stream mode doesn't support editing
func (a *InteractionAdapter) CanEditMessages() bool {
	return false
}
