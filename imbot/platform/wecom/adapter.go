// Package wecom provides WeCom (Enterprise WeChat) platform bot implementation for ImBot.
package wecom

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/weixin/types"
)

// Adapter converts types.Message (already translated from WeCom wire format by the SDK)
// to core.Message.
type Adapter struct {
	*core.BaseAdapter
}

// NewAdapter creates a new WeCom adapter.
func NewAdapter(config *core.Config) *Adapter {
	return &Adapter{
		BaseAdapter: core.NewBaseAdapter(config),
	}
}

// Platform returns core.PlatformWecom.
func (a *Adapter) Platform() core.Platform {
	return core.PlatformWecom
}

// AdaptMessage converts a types.Message to core.Message.
func (a *Adapter) AdaptMessage(ctx context.Context, msg *types.Message) (*core.Message, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}

	// Determine recipient ID: use chatId from metadata when available (group),
	// otherwise fall back to msg.To.
	recipientID := msg.To
	chatType := mapChatType(msg.ChatType)
	if cid, ok := msg.Metadata["chatId"].(string); ok && cid != "" {
		recipientID = cid
	}

	builder := core.NewMessageBuilder(core.PlatformWecom).
		WithID(msg.MessageID).
		WithTimestamp(msg.Timestamp.Unix()).
		WithSender(msg.SenderID, "", "").
		WithRecipient(recipientID, string(chatType), "").
		WithContent(a.extractContent(msg)).
		WithMetadata("context_token", msg.ContextToken)

	if msg.ReplyToID != "" {
		builder.WithThreadContext(&core.ThreadContext{
			ParentMessageID: msg.ReplyToID,
		})
	}

	return builder.Build(), nil
}

// extractContent derives a core.Content value from the SDK message.
func (a *Adapter) extractContent(msg *types.Message) core.Content {
	if len(msg.Attachments) > 0 {
		media := make([]core.MediaAttachment, 0, len(msg.Attachments))
		for _, att := range msg.Attachments {
			media = append(media, core.MediaAttachment{
				Type:     mapMimeType(att.MimeType),
				URL:      att.URL,
				Filename: att.FileName,
			})
		}
		return core.NewMediaContent(media, msg.Text)
	}
	if msg.Text != "" {
		return core.NewTextContent(msg.Text)
	}
	return core.NewSystemContent("unknown", nil)
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
