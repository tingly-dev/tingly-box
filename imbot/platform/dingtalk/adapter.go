package dingtalk

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// Adapter adapts DingTalk events to core.Message
type Adapter struct {
	*core.BaseAdapter
}

// NewAdapter creates a new DingTalk adapter
func NewAdapter(config *core.Config) *Adapter {
	return &Adapter{
		BaseAdapter: core.NewBaseAdapter(config),
	}
}

// Platform returns core.PlatformDingTalk
func (a *Adapter) Platform() core.Platform {
	return core.PlatformDingTalk
}

// AdaptChatBotMessage converts a DingTalk chatbot callback to core.Message
func (a *Adapter) AdaptChatBotMessage(ctx context.Context, data *chatbot.BotCallbackDataModel) (*core.Message, error) {
	if data == nil {
		return nil, fmt.Errorf("nil callback data")
	}

	// Determine chat type
	chatType := a.getChatType(data.ConversationType)

	// Build message using fluent builder
	messageBuilder := core.NewMessageBuilder(core.PlatformDingTalk).
		WithID(data.MsgId).
		WithTimestamp(time.Now().Unix()).
		WithRecipient(data.ConversationId, string(chatType), data.ConversationTitle).
		WithSenderFrom(a.extractSender(data)).
		WithContent(a.extractContent(data)).
		WithMetadata("conversation_type", data.ConversationType).
		WithMetadata("conversation_title", data.ConversationTitle).
		WithMetadata("sender_staff_id", data.SenderStaffId).
		WithMetadata("conversation_id", data.ConversationId).
		WithMetadata("session_webhook", data.SessionWebhook)

	return messageBuilder.Build(), nil
}

// AdaptIncomingMessage converts an incoming text message to core.Message
func (a *Adapter) AdaptIncomingMessage(ctx context.Context, conversationID, senderID, senderNick, text string, timestamp int64) (*core.Message, error) {
	chatType := core.ChatTypeDirect // Default to direct for 1:1 messages

	messageBuilder := core.NewMessageBuilder(core.PlatformDingTalk).
		WithID(fmt.Sprintf("incoming_%d", timestamp)).
		WithTimestamp(timestamp).
		WithRecipient(conversationID, string(chatType), "").
		WithSender(senderID, "", senderNick).
		WithTextContent(text, nil)

	return messageBuilder.Build(), nil
}

// extractSender extracts sender info from DingTalk callback data
func (a *Adapter) extractSender(data *chatbot.BotCallbackDataModel) core.Sender {
	return core.Sender{
		ID:          data.SenderStaffId,
		DisplayName: data.SenderNick,
		Raw: map[string]interface{}{
			"staff_id":        data.SenderStaffId,
			"conversation_id": data.ConversationId,
		},
	}
}

// extractContent extracts content from DingTalk callback data
func (a *Adapter) extractContent(data *chatbot.BotCallbackDataModel) core.Content {
	if mc := a.extractMedia(data); mc != nil {
		return mc
	}

	// Check for text content in Text field
	if data.Text.Content != "" {
		return core.NewTextContent(data.Text.Content)
	}

	// Check for content in Content field (for rich media, etc.)
	if data.Content != nil {
		// Try to extract text from content
		if contentStr, ok := data.Content.(string); ok {
			if contentStr != "" {
				return core.NewTextContent(contentStr)
			}
		}
	}

	// For other content types, create system content
	contentType := data.Msgtype
	if contentType == "" {
		contentType = "unknown"
	}

	return core.NewSystemContent(contentType, map[string]interface{}{
		"msgtype": data.Msgtype,
		"content": data.Content,
	})
}

// getChatType maps DingTalk conversation type to core ChatType
func (a *Adapter) getChatType(conversationType string) core.ChatType {
	switch conversationType {
	case "1":
		return core.ChatTypeDirect
	case "2":
		return core.ChatTypeGroup
	default:
		return core.ChatTypeDirect
	}
}

// downloadCodeKey is where a media attachment keeps the code the robot API
// trades for a download link (see Bot.resolveMediaURLs).
const downloadCodeKey = "download_code"

// extractMedia reads the file, picture and rich-text messages (video and audio
// are not file types a chat can hand to a session). A
// file carries its name in the payload; the others have none, which is left
// empty for the receiver to name.
func (a *Adapter) extractMedia(data *chatbot.BotCallbackDataModel) *core.MediaContent {
	content, _ := data.Content.(map[string]any)
	str := func(m map[string]any, key string) string { s, _ := m[key].(string); return s }
	item := func(typ, code, name, mimeType string) core.MediaAttachment {
		return core.MediaAttachment{
			Type: typ, Filename: name, MimeType: mimeType,
			Raw: map[string]interface{}{downloadCodeKey: code, "platform": "dingtalk"},
		}
	}
	// A picture's real type is only known once it is downloaded; the receiver
	// names and types it from the bytes, so this only has to be an image type.
	const pictureType = "image/jpeg"
	switch data.Msgtype {
	case "file":
		name := str(content, "fileName")
		return core.NewMediaContent([]core.MediaAttachment{item("document", str(content, "downloadCode"), name, core.MimeTypeByFilename(name))}, "")
	case "picture":
		return core.NewMediaContent([]core.MediaAttachment{item("image", str(content, "downloadCode"), "", pictureType)}, "")
	case "richText":
		parts, _ := content["richText"].([]any)
		var text strings.Builder
		var media []core.MediaAttachment
		for _, p := range parts {
			part, _ := p.(map[string]any)
			if str(part, "type") == "picture" {
				media = append(media, item("image", str(part, "downloadCode"), "", pictureType))
			} else {
				text.WriteString(str(part, "text"))
			}
		}
		if len(media) == 0 {
			return nil
		}
		return core.NewMediaContent(media, strings.TrimSpace(text.String()))
	}
	return nil
}
