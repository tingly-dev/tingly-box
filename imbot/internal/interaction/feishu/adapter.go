package feishu

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/interaction"
)

// Adapter implements interaction.Adapter for Feishu
type Adapter struct {
	*interaction.BaseAdapter
}

// NewAdapter creates a new Feishu interaction adapter
func NewAdapter() *Adapter {
	return &Adapter{
		BaseAdapter: interaction.NewBaseAdapter(true, false), // Supports cards but no editing via stream mode
	}
}

// SupportsInteractions returns true - Feishu supports interactive cards
func (a *Adapter) SupportsInteractions() bool {
	return true
}

// BuildMarkup creates Feishu card markup from interactions
// Note: Feishu cards use a different format than Telegram keyboards
func (a *Adapter) BuildMarkup(interactions []interaction.Interaction) (any, error) {
	// Feishu card structure
	// https://open.feishu.cn/document/ukTMukTMukTMuUTjNj4xMjYU
	card := a.buildCard(interactions)
	return card, nil
}

// buildCard builds a Feishu interactive card
func (a *Adapter) buildCard(interactions []interaction.Interaction) map[string]interface{} {
	// Build button elements
	var elements []map[string]interface{}
	for _, item := range interactions {
		if item.Type == interaction.ActionSelect || item.Type == interaction.ActionConfirm || item.Type == interaction.ActionCancel {
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
				"tag": "div",
				"fields": []map[string]interface{}{},
			},
			{
				"tag":      "action",
				"actions":  elements,
			},
		},
	}

	return card
}

// BuildFallbackText creates numbered text options
// This is used when Mode=Text or when cards are not appropriate
func (a *Adapter) BuildFallbackText(message string, interactions []interaction.Interaction) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString("\n\n")
	sb.WriteString("请回复数字：\n")

	for i, item := range interactions {
		if item.Type == interaction.ActionSelect || item.Type == interaction.ActionConfirm {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.Label))
		}
	}
	sb.WriteString("0. 取消")

	return sb.String()
}

// ParseResponse parses Feishu interaction responses
// Feishu interactions come via card button clicks
func (a *Adapter) ParseResponse(msg core.Message) (*interaction.InteractionResponse, error) {
	// Check if this is a card interaction callback
	if action, ok := msg.Metadata["action"].(string); ok {
		// Parse Feishu action callback
		// Format: ia:interactionID:value
		parts := strings.Split(action, ":")
		if len(parts) >= 3 && parts[0] == "ia" {
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
			return &interaction.InteractionResponse{
				Action:    interaction.Interaction{ID: parts[1], Value: parts[2]},
				Timestamp: timestamp,
			}, nil
		}
		return nil, interaction.ErrNotInteraction
	}

	// Text replies are handled by Handler.parseTextResponse
	return nil, nil
}

// UpdateMessage updates a Feishu message
// Note: Feishu message editing is limited in stream mode
func (a *Adapter) UpdateMessage(ctx context.Context, bot core.Bot, chatID, messageID, text string, interactions []interaction.Interaction) error {
	// Feishu doesn't support message editing via the same API
	// Would need to use the message update API separately
	return interaction.ErrNotSupported
}

// CanEditMessages returns false - Feishu stream mode doesn't support editing
func (a *Adapter) CanEditMessages() bool {
	return false
}
