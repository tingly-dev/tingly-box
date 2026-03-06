package dingtalk

import (
	"context"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/itx"
)

// InteractionAdapter implements itx.Adapter for DingTalk
type InteractionAdapter struct {
	*itx.BaseAdapter
}

// NewInteractionAdapter creates a new DingTalk interaction adapter
func NewInteractionAdapter() *InteractionAdapter {
	return &InteractionAdapter{
		BaseAdapter: itx.NewBaseAdapter(false, false), // No native interactions or editing
	}
}

// BuildMarkup is not supported for DingTalk stream mode
func (a *InteractionAdapter) BuildMarkup(interactions []itx.Interaction) (any, error) {
	return nil, itx.ErrNotSupported
}

// BuildFallbackText creates numbered text options
// This is the PRIMARY mode for DingTalk, not a fallback
func (a *InteractionAdapter) BuildFallbackText(message string, interactions []itx.Interaction) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString("\n\n")
	sb.WriteString("请回复数字：\n")

	for i, item := range interactions {
		if item.Type == itx.ActionSelect || item.Type == itx.ActionConfirm {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.Label))
		}
	}
	sb.WriteString("0. 取消")

	return sb.String()
}

// ParseResponse returns nil - text replies are handled by Handler.parseTextResponse
func (a *InteractionAdapter) ParseResponse(msg core.Message) (*itx.InteractionResponse, error) {
	// All text replies are handled by Handler.parseTextResponse
	return nil, nil
}

// UpdateMessage is not supported for DingTalk stream mode
func (a *InteractionAdapter) UpdateMessage(ctx context.Context, bot core.Bot, chatID, messageID, text string, interactions []itx.Interaction) error {
	return itx.ErrNotSupported
}