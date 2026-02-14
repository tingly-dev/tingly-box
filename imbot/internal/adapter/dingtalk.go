package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/tingly-dev/tingly-box/imbot/internal/builder"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// DingTalkAdapter adapts DingTalk events to core.Message
type DingTalkAdapter struct {
	*BaseAdapter
}

// NewDingTalkAdapter creates a new DingTalk adapter
func NewDingTalkAdapter(config *core.Config) *DingTalkAdapter {
	return &DingTalkAdapter{
		BaseAdapter: NewBaseAdapter(config),
	}
}

// Platform returns core.PlatformDingTalk
func (a *DingTalkAdapter) Platform() core.Platform {
	return core.PlatformDingTalk
}

// AdaptChatBotMessage converts a DingTalk chatbot callback to core.Message
func (a *DingTalkAdapter) AdaptChatBotMessage(ctx context.Context, data *chatbot.BotCallbackDataModel) (*core.Message, error) {
	if data == nil {
		return nil, fmt.Errorf("nil callback data")
	}

	// Determine chat type
	chatType := a.getChatType(data.ConversationType)

	// Build message using fluent builder
	messageBuilder := builder.NewMessageBuilder(core.PlatformDingTalk).
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
func (a *DingTalkAdapter) AdaptIncomingMessage(ctx context.Context, conversationID, senderID, senderNick, text string, timestamp int64) (*core.Message, error) {
	chatType := core.ChatTypeDirect // Default to direct for 1:1 messages

	messageBuilder := builder.NewMessageBuilder(core.PlatformDingTalk).
		WithID(fmt.Sprintf("incoming_%d", timestamp)).
		WithTimestamp(timestamp).
		WithRecipient(conversationID, string(chatType), "").
		WithSender(senderID, "", senderNick).
		WithTextContent(text, nil)

	return messageBuilder.Build(), nil
}

// extractSender extracts sender info from DingTalk callback data
func (a *DingTalkAdapter) extractSender(data *chatbot.BotCallbackDataModel) core.Sender {
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
func (a *DingTalkAdapter) extractContent(data *chatbot.BotCallbackDataModel) core.Content {
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
func (a *DingTalkAdapter) getChatType(conversationType string) core.ChatType {
	switch conversationType {
	case "1":
		return core.ChatTypeDirect
	case "2":
		return core.ChatTypeGroup
	default:
		return core.ChatTypeDirect
	}
}
