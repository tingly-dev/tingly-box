package discord

import (
	"context"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	itx "github.com/tingly-dev/tingly-box/imbot/internal/interaction"
)

// InteractionAdapter implements itx.Adapter for Discord
type InteractionAdapter struct {
	*itx.BaseAdapter
}

// NewInteractionAdapter creates a new Discord interaction adapter
func NewInteractionAdapter() *InteractionAdapter {
	return &InteractionAdapter{
		BaseAdapter: itx.NewBaseAdapter(true, true), // Supports interactions and editing
	}
}

// BuildMarkup creates Discord component buttons from interactions
func (a *InteractionAdapter) BuildMarkup(interactions []itx.Interaction) (any, error) {
	// Discord components are organized into rows (ActionsRow)
	// We'll create a single row with all buttons for now
	components := make([]discordgo.Button, 0)

	for _, item := range interactions {
		switch item.Type {
		case itx.ActionSelect, itx.ActionConfirm, itx.ActionCancel:
			style := discordgo.PrimaryButton
			if item.Type == itx.ActionCancel {
				style = discordgo.DangerButton
			}

			btn := discordgo.Button{
				Label:    item.Label,
				CustomID: formatCustomID("ia", item.ID, item.Value),
				Style:    style,
			}
			components = append(components, btn)

		case itx.ActionNavigate:
			btn := discordgo.Button{
				Label:    item.Label,
				CustomID: formatCustomID("ia", item.ID, item.Value),
				Style:    discordgo.SecondaryButton,
			}
			components = append(components, btn)

		case itx.ActionInput:
			continue
		}
	}

	if len(components) == 0 {
		return nil, nil
	}

	// Wrap in ActionsRow (Discord requires components to be in rows)
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: toMessageComponents(components),
		},
	}, nil
}

// toMessageComponents converts []discordgo.Button to []discordgo.MessageComponent
func toMessageComponents(buttons []discordgo.Button) []discordgo.MessageComponent {
	result := make([]discordgo.MessageComponent, len(buttons))
	for i, btn := range buttons {
		result[i] = btn
	}
	return result
}

// BuildFallbackText creates numbered text options for text mode
func (a *InteractionAdapter) BuildFallbackText(message string, interactions []itx.Interaction) string {
	return itx.BuildFallbackText(message, interactions, "Reply with number:", "Cancel")
}

// ParseResponse parses Discord interactions or returns nil for text handling
func (a *InteractionAdapter) ParseResponse(msg core.Message) (*itx.InteractionResponse, error) {
	// Check if this is a Discord component interaction
	// We'll try to extract custom_id from the metadata
	if customID, ok := msg.Metadata["custom_id"].(string); ok {
		parts := parseCustomID(customID)
		if len(parts) >= 3 && parts[0] == "ia" {
			timestamp := time.Unix(msg.Timestamp, 0)
			// Format: ia:interactionID:value
			// Or: ia:interactionID:requestID:value (for responses)
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
		return nil, itx.ErrNotInteraction
	}

	// Text replies are handled by Handler.parseTextResponse
	return nil, nil
}

// UpdateMessage edits a Discord message
func (a *InteractionAdapter) UpdateMessage(ctx context.Context, bot core.Bot, chatID, messageID, text string, interactions []itx.Interaction) error {
	// Discord message editing requires the Discord-specific bot interface
	// For now, return not supported as we need to add this to the imbot interface
	return itx.ErrNotSupported
}

// Custom ID helpers

// formatCustomID formats Discord custom ID with colon separator
func formatCustomID(parts ...string) string {
	return strings.Join(parts, ":")
}

// parseCustomID parses Discord custom ID into parts
func parseCustomID(id string) []string {
	return strings.Split(id, ":")
}
