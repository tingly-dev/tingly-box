// Package weixin provides Weixin platform bot implementation for ImBot.
package weixin

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/weixin/api"
	"github.com/tingly-dev/weixin/types"
)

// Adapter adapts Weixin channel messages to core.Message
type Adapter struct {
	*core.BaseAdapter
	account *types.WeChatAccount
}

// NewAdapter creates a new Weixin adapter
func NewAdapter(config *core.Config, account *types.WeChatAccount) *Adapter {
	return &Adapter{
		BaseAdapter: core.NewBaseAdapter(config),
		account:     account,
	}
}

// Platform returns core.PlatformWeixin
func (a *Adapter) Platform() core.Platform {
	return core.PlatformWeixin
}

// AdaptMessage converts a types.Message to core.Message
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

// ConvertToOutboundMessage converts SendMessageOptions to Weixin outbound message format
// Note: This function is kept for compatibility but the actual message sending
// uses the API client directly with context tokens
func (a *Adapter) ConvertToOutboundMessage(opts *core.SendMessageOptions) (*types.OutboundMessage, string, []api.MessageItem) {
	outbound := &types.OutboundMessage{
		To:           "", // Will be set by caller
		Text:         opts.Text,
		ContextToken: "", // Will be set by caller
	}

	var items []api.MessageItem

	// Add text item
	if opts.Text != "" {
		items = append(items, api.MessageItem{
			Type: api.MessageItemTypeText,
			TextItem: &api.TextItem{
				Text: opts.Text,
			},
		})
	}

	// Add media items - TODO: implement media upload via CDN
	// For now, media is not supported in the weixin platform
	if len(opts.Media) > 0 {
		for _, media := range opts.Media {
			item := a.mediaToItem(media)
			if item != nil {
				items = append(items, *item)
			}
		}
	}

	return outbound, "", items
}

// extractContent extracts content from a types.Message
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

// mapContentType maps Weixin content type to core media type
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

// mediaToItem converts a core MediaAttachment to weixinapi MessageItem
func (a *Adapter) mediaToItem(media core.MediaAttachment) *api.MessageItem {
	switch media.Type {
	case "image":
		return &api.MessageItem{
			Type: api.MessageItemTypeImage,
			ImageItem: &api.ImageItem{
				URL: media.URL,
			},
		}
	case "video":
		return &api.MessageItem{
			Type:      api.MessageItemTypeVideo,
			VideoItem: &api.VideoItem{},
		}
	case "audio", "voice":
		return &api.MessageItem{
			Type:      api.MessageItemTypeVoice,
			VoiceItem: &api.VoiceItem{},
		}
	default:
		// Treat as file
		return &api.MessageItem{
			Type: api.MessageItemTypeFile,
			FileItem: &api.FileItem{
				FileName: media.Filename,
			},
		}
	}
}

// AdaptCoreToChannel converts a core.Message to types.OutboundMessage
func (a *Adapter) AdaptCoreToChannel(ctx context.Context, msg *core.Message) (*types.OutboundMessage, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}

	outbound := &types.OutboundMessage{
		To: msg.GetReplyTarget(),
	}

	// Extract text
	if msg.IsTextContent() {
		outbound.Text = msg.GetText()
	} else if mc, ok := msg.Content.(*core.MediaContent); ok {
		// Media content with caption
		if len(mc.Media) > 0 {
			outbound.Text = mc.Caption
		}
	}

	// Extract media
	if msg.IsMediaContent() {
		media := msg.GetMedia()
		if len(media) > 0 {
			firstMedia := media[0]
			outbound.MediaURL = firstMedia.URL
			outbound.ContentType = firstMedia.Type
			outbound.FileName = firstMedia.Filename
		}
	}

	return outbound, nil
}

// BuildReplyTarget builds the reply target from sender/recipient info
func (a *Adapter) BuildReplyTarget(senderID, recipientID, sessionID string) string {
	// For Weixin, use the other party's ID as reply target
	// If we're the sender (bot), reply to the recipient
	// If we're the recipient, reply to the sender

	// Check if the sender is the bot (matches our account ID)
	if a.account != nil && senderID == a.account.UserID {
		return recipientID
	}

	return senderID
}

// GetMessageLimit returns the message length limit for Weixin
func (a *Adapter) GetMessageLimit() int {
	// Weixin message limit is typically 2048 bytes
	return 2048
}

// ShouldChunkText determines if text should be chunked
func (a *Adapter) ShouldChunkText(text string) bool {
	return len([]rune(text)) > a.GetMessageLimit()
}
