package dingtalk

import (
	"context"

	"github.com/tingly-dev/tingly-box/imbot/core"
	itx "github.com/tingly-dev/tingly-box/imbot/interaction"
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
	return itx.BuildFallbackText(message, interactions, "请回复数字：", "取消")
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
