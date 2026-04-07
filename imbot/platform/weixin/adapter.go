// Package weixin provides Weixin platform bot implementation for ImBot.
package weixin

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/weixin/types"
)

// Adapter converts Weixin channel messages to core.Message format.
type Adapter struct {
	*core.BaseAdapter
	account *types.WeChatAccount
}

// NewAdapter creates a new Weixin adapter with the given config and account.
func NewAdapter(config *core.Config, account *types.WeChatAccount) *Adapter {
	return &Adapter{
		BaseAdapter: core.NewBaseAdapter(config),
		account:     account,
	}
}

// Platform returns core.PlatformWeixin.
func (a *Adapter) Platform() core.Platform {
	return core.PlatformWeixin
}

// AdaptMessage converts a types.Message to core.Message.
func (a *Adapter) AdaptMessage(ctx context.Context, msg *types.Message) (*core.Message, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}

	// Extract message metadata
	sessionID, _ := msg.Metadata["session_id"].(string)
	messageType, _ := msg.Metadata["message_type"].(int)
	messageState, _ := msg.Metadata["message_state"].(int)

	// Build message using fluent builder
	messageBuilder := core.NewMessageBuilder(core.PlatformWeixin).
		WithID(msg.MessageID).
		WithTimestamp(msg.Timestamp.Unix()).
		WithRecipient(msg.To, string(msg.ChatType), "").
		WithSender(msg.From, "", "").
		WithContent(a.extractContent(msg)).
		WithMetadata("session_id", sessionID).
		WithMetadata("message_type", messageType).
		WithMetadata("message_state", messageState).
		WithMetadata("context_token", msg.ContextToken)

	// Add thread context if available
	if sessionID != "" {
		threadCtx := &core.ThreadContext{
			ID: sessionID,
		}
		if msg.ReplyToID != "" {
			threadCtx.ParentMessageID = msg.ReplyToID
		}
		messageBuilder.WithThreadContext(threadCtx)
	}

	return messageBuilder.Build(), nil
}

// extractContent extracts the content from a types.Message.
func (a *Adapter) extractContent(msg *types.Message) core.Content {
	// Check if there's text
	if msg.Text != "" {
		// Check if there are also attachments
		if len(msg.Attachments) > 0 {
			// Compound content: text + media
			media := make([]core.MediaAttachment, 0, len(msg.Attachments))
			for _, att := range msg.Attachments {
				media = append(media, core.MediaAttachment{
					Type:     a.mapContentType(att.ContentType),
					URL:      att.URL,
					Filename: att.FileName,
				})
			}
			return core.NewMediaContent(media, msg.Text)
		}
		return core.NewTextContent(msg.Text)
	}

	// Only attachments (media)
	if len(msg.Attachments) > 0 {
		media := make([]core.MediaAttachment, 0, len(msg.Attachments))
		for _, att := range msg.Attachments {
			media = append(media, core.MediaAttachment{
				Type:     a.mapContentType(att.ContentType),
				URL:      att.URL,
				Filename: att.FileName,
			})
		}
		return core.NewMediaContent(media, "")
	}

	// Unknown content
	return core.NewSystemContent("unknown", nil)
}

// mapContentType maps Weixin content type to core media type.
func (a *Adapter) mapContentType(contentType string) string {
	switch contentType {
	case "image":
		return "image"
	case "video":
		return "video"
	case "audio":
		return "audio"
	case "file":
		return "document"
	case "voice":
		return "audio"
	default:
		return "document"
	}
}

// BuildReplyTarget builds the reply target from sender/recipient info.
//
// For Weixin, we use the other party's ID as the reply target:
// - If we're the sender (bot), reply to the recipient
// - If we're the recipient, reply to the sender
func (a *Adapter) BuildReplyTarget(senderID, recipientID, sessionID string) string {
	// Check if the sender is the bot (matches our account ID)
	if a.account != nil && senderID == a.account.UserID {
		return recipientID
	}

	return senderID
}

// GetMessageLimit returns the message length limit for Weixin (2048 bytes).
func (a *Adapter) GetMessageLimit() int {
	return 2048
}

// ShouldChunkText reports whether text should be chunked.
func (a *Adapter) ShouldChunkText(text string) bool {
	return len([]rune(text)) > a.GetMessageLimit()
}
