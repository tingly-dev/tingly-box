// Package wecom provides WeCom (Enterprise WeChat) platform bot implementation for ImBot.
package wecom

import (
	"context"

	"github.com/tingly-dev/tingly-box/imbot/core"
	weixinadapter "github.com/tingly-dev/tingly-box/imbot/platform/weixin"
	"github.com/tingly-dev/weixin/types"
)

// Adapter converts types.Message (already translated from WeCom wire format by the SDK)
// to core.Message.
type Adapter struct {
	*weixinadapter.Adapter
}

// NewAdapter creates a new WeCom adapter.
func NewAdapter(config *core.Config) *Adapter {
	return &Adapter{
		Adapter: weixinadapter.NewAdapterForPlatform(config, nil, core.PlatformWecom),
	}
}

// AdaptMessage converts a types.Message to core.Message.
func (a *Adapter) AdaptMessage(ctx context.Context, msg *types.Message) (*core.Message, error) {
	if msg == nil {
		return nil, core.NewAdaptError(core.PlatformWecom, "adapt message", msg, context.Canceled)
	}

	// Determine recipient ID: use chatId from metadata when available (group),
	// otherwise fall back to msg.To.
	recipientID := msg.To
	chatType := mapChatType(msg.ChatType)
	if cid, ok := msg.Metadata["chatId"].(string); ok && cid != "" {
		recipientID = cid
	}

	builder := core.NewMessageBuilder(a.Platform()).
		WithID(msg.MessageID).
		WithTimestamp(msg.Timestamp.Unix()).
		WithSender(msg.SenderID, "", "").
		WithRecipient(recipientID, string(chatType), "").
		WithContent(weixinadapter.BuildContent(msg, mapMimeType)).
		WithMetadata("context_token", msg.ContextToken)

	if msg.ReplyToID != "" {
		builder.WithThreadContext(&core.ThreadContext{
			ParentMessageID: msg.ReplyToID,
		})
	}

	return builder.Build(), nil
}

// mapChatType maps SDK ChatType to core ChatType.
func mapChatType(ct types.ChatType) core.ChatType {
	if ct == types.ChatTypeGroup {
		return core.ChatTypeGroup
	}
	return core.ChatTypeDirect
}

// mapMimeType maps WeCom MIME/content type strings to core media type names.
func mapMimeType(mimeType string) string {
	switch mimeType {
	case "image":
		return "image"
	case "video":
		return "video"
	case "voice", "audio":
		return "audio"
	default:
		return "document"
	}
}

// GetMessageLimit returns the character limit for WeCom messages.
func (a *Adapter) GetMessageLimit() int {
	return 4000
}
