package dingtalk

import (
	"context"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/interaction"
)

// Adapter implements interaction.Adapter for DingTalk
type Adapter struct {
	*interaction.BaseAdapter
}

// NewAdapter creates a new DingTalk interaction adapter
func NewAdapter() *Adapter {
	return &Adapter{
		BaseAdapter: interaction.NewBaseAdapter(false, false), // No native interactions or editing
	}
}

// BuildMarkup is not supported for DingTalk stream mode
func (a *Adapter) BuildMarkup(interactions []interaction.Interaction) (any, error) {
	return nil, interaction.ErrNotSupported
}

// BuildFallbackText creates numbered text options
// This is the PRIMARY mode for DingTalk, not a fallback
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

// ParseResponse returns nil - text replies are handled by Handler.parseTextResponse
func (a *Adapter) ParseResponse(msg core.Message) (*interaction.InteractionResponse, error) {
	// All text replies are handled by Handler.parseTextResponse
	return nil, nil
}

// UpdateMessage is not supported for DingTalk stream mode
func (a *Adapter) UpdateMessage(ctx context.Context, bot core.Bot, chatID, messageID, text string, interactions []interaction.Interaction) error {
	return interaction.ErrNotSupported
}
