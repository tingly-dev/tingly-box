package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/imbot/internal/builder"
	"github.com/tingly-dev/tingly-box/imbot/internal/content"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// FeishuMessageEvent represents the incoming event structure from Feishu/Lark
type FeishuMessageEvent struct {
	Header EventHeader        `json:"header"`
	Event  MessageEventDetail `json:"event"`
}

// EventHeader represents the event header
type EventHeader struct {
	EventID   string `json:"event_id"`
	Timestamp string `json:"timestamp"`
	Token     string `json:"token"`
	EventType string `json:"event_type"`
}

// MessageEventDetail represents the message event details
type MessageEventDetail struct {
	MessageID  string        `json:"message_id"`
	RootID     interface{}   `json:"root_id"`
	ParentID   interface{}   `json:"parent_id"`
	CreateTime string        `json:"create_time"`
	ChatType   string        `json:"chat_type"`
	MsgType    string        `json:"msg_type"`
	Content    interface{}   `json:"content"`
	Mention    MentionDetail `json:"mention"`
	Sender     SenderDetail  `json:"sender"`
	ChatID     string        `json:"chat_id"`
}

// MentionDetail represents mention details
type MentionDetail struct {
	MentionList []MentionItem `json:"mention_list"`
}

// MentionItem represents a mention item
type MentionItem struct {
	ID        string `json:"id"`
	IDType    string `json:"id_type"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	TenantKey string `json:"tenant_key"`
}

// SenderDetail represents sender details
type SenderDetail struct {
	SenderID   string `json:"sender_id"`
	SenderType string `json:"sender_type"`
	TenantKey  string `json:"tenant_key"`
}

// FeishuAdapter adapts Feishu/Lark events to core.Message
type FeishuAdapter struct {
	*BaseAdapter
}

// NewFeishuAdapter creates a new Feishu adapter
func NewFeishuAdapter(config *core.Config) *FeishuAdapter {
	return &FeishuAdapter{
		BaseAdapter: NewBaseAdapter(config),
	}
}

// Platform returns core.PlatformFeishu
func (a *FeishuAdapter) Platform() core.Platform {
	return core.PlatformFeishu
}

// AdaptWebhook converts a Feishu webhook event to core.Message
func (a *FeishuAdapter) AdaptWebhook(ctx context.Context, body []byte) (*core.Message, error) {
	var event FeishuMessageEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("failed to parse webhook: %w", err)
	}

	// Only handle message events
	if event.Header.EventType != "im.message.receive_v1" {
		return nil, fmt.Errorf("unsupported event type: %s", event.Header.EventType)
	}

	return a.AdaptMessage(ctx, event.Event)
}

// AdaptMessage converts a Feishu message event to core.Message
func (a *FeishuAdapter) AdaptMessage(ctx context.Context, event MessageEventDetail) (*core.Message, error) {
	// Determine chat type
	chatType := a.getChatType(event.ChatType)

	// Build message using fluent builder
	messageBuilder := builder.NewMessageBuilder(core.PlatformFeishu).
		WithID(event.MessageID).
		WithTimestamp(parseFeishuTimestamp(event.CreateTime)).
		WithRecipient(event.ChatID, string(chatType), "").
		WithSenderFrom(a.extractSender(event.Sender)).
		WithContent(a.extractContent(event)).
		WithMetadata("raw_event", event) // Store raw for platform-specific access

	// Add thread context if reply
	if event.ParentID != nil {
		parentID := a.convertToString(event.ParentID)
		messageBuilder.WithReplyTo(event.MessageID, parentID)
	}

	return messageBuilder.Build(), nil
}

// extractSender extracts sender info from Feishu SenderDetail
func (a *FeishuAdapter) extractSender(detail SenderDetail) core.Sender {
	return core.Sender{
		ID:          detail.SenderID,
		DisplayName: detail.SenderID, // In production, fetch user info
		Raw:         make(map[string]interface{}),
	}
}

// extractContent extracts content from a Feishu message event
func (a *FeishuAdapter) extractContent(event MessageEventDetail) core.Content {
	// Create content registry
	registry := content.NewRegistry[MessageEventDetail]()

	// Register handlers for different content types
	registry.Register(content.NewTextHandler(func(e MessageEventDetail) (string, []core.Entity, bool) {
		if e.MsgType != "text" {
			return "", nil, false
		}

		// Extract text from content
		if textContent, ok := e.Content.(map[string]interface{}); ok {
			if text, ok := textContent["text"].(string); ok {
				return text, nil, true
			}
		}
		return "", nil, false
	}))

	registry.Register(content.NewMediaHandler("image", func(e MessageEventDetail) ([]core.MediaAttachment, string, bool) {
		if e.MsgType != "image" {
			return nil, "", false
		}

		if imageContent, ok := e.Content.(map[string]interface{}); ok {
			if imageKey, ok := imageContent["image_key"].(string); ok {
				return []core.MediaAttachment{{
					Type: "image",
					URL:  imageKey,
					Raw:  make(map[string]interface{}),
				}}, "", true
			}
		}
		return nil, "", false
	}))

	registry.Register(content.NewMediaHandler("video", func(e MessageEventDetail) ([]core.MediaAttachment, string, bool) {
		if e.MsgType != "video" {
			return nil, "", false
		}

		if videoContent, ok := e.Content.(map[string]interface{}); ok {
			if videoKey, ok := videoContent["video_key"].(string); ok {
				return []core.MediaAttachment{{
					Type: "video",
					URL:  videoKey,
					Raw:  make(map[string]interface{}),
				}}, "", true
			}
		}
		return nil, "", false
	}))

	registry.Register(content.NewMediaHandler("audio", func(e MessageEventDetail) ([]core.MediaAttachment, string, bool) {
		if e.MsgType != "audio" {
			return nil, "", false
		}

		if audioContent, ok := e.Content.(map[string]interface{}); ok {
			if fileKey, ok := audioContent["file_key"].(string); ok {
				return []core.MediaAttachment{{
					Type: "audio",
					URL:  fileKey,
					Raw:  make(map[string]interface{}),
				}}, "", true
			}
		}
		return nil, "", false
	}))

	registry.Register(content.NewMediaHandler("file", func(e MessageEventDetail) ([]core.MediaAttachment, string, bool) {
		if e.MsgType != "file" {
			return nil, "", false
		}

		if fileContent, ok := e.Content.(map[string]interface{}); ok {
			if fileKey, ok := fileContent["file_key"].(string); ok {
				return []core.MediaAttachment{{
					Type: "document",
					URL:  fileKey,
					Raw:  make(map[string]interface{}),
				}}, "", true
			}
		}
		return nil, "", false
	}))

	// Handle rich post content (Feishu's formatted messages)
	registry.Register(content.NewTextHandler(func(e MessageEventDetail) (string, []core.Entity, bool) {
		if e.MsgType != "post" {
			return "", nil, false
		}

		// Extract text from post content
		text := a.extractPostText(e.Content)
		if text != "" {
			return text, nil, true
		}
		return "", nil, false
	}))

	// Set default for unknown content
	registry.SetDefault(content.NewSystemHandler("unknown", func(e MessageEventDetail) (string, map[string]interface{}, bool) {
		return "unknown", map[string]interface{}{"msg_type": e.MsgType}, true
	}))

	// Handle content
	result, err := registry.Handle(context.Background(), event)
	if err != nil {
		a.Logger().Error("Failed to extract content: %v", err)
		return core.NewSystemContent("error", map[string]interface{}{"error": err.Error()})
	}

	return result
}

// extractPostText extracts text from Feishu post content
func (a *FeishuAdapter) extractPostText(contentInterface interface{}) string {
	postMap, ok := contentInterface.(map[string]interface{})
	if !ok {
		return ""
	}

	zhCn, ok := postMap["zh_cn"].(map[string]interface{})
	if !ok {
		return ""
	}

	var textBuilder strings.Builder

	// Add title if present
	if title, ok := zhCn["title"].(string); ok && title != "" {
		textBuilder.WriteString(title)
		textBuilder.WriteString("\n")
	}

	// Extract content elements
	if contentArr, ok := zhCn["content"].([]interface{}); ok {
		for _, row := range contentArr {
			if rowArr, ok := row.([]interface{}); ok {
				for _, elem := range rowArr {
					if elemMap, ok := elem.(map[string]interface{}); ok {
						if tag, ok := elemMap["tag"].(string); ok && tag == "text" {
							if text, ok := elemMap["text"].(string); ok {
								textBuilder.WriteString(text)
								textBuilder.WriteString("\n")
							}
						}
					}
				}
			}
		}
	}

	return textBuilder.String()
}

// getChatType maps Feishu chat type to core ChatType
func (a *FeishuAdapter) getChatType(chatType string) core.ChatType {
	switch chatType {
	case "p2p":
		return core.ChatTypeDirect
	case "group":
		return core.ChatTypeGroup
	case "channel":
		return core.ChatTypeChannel
	default:
		return core.ChatTypeDirect
	}
}

// convertToString safely converts interface{} to string
func (a *FeishuAdapter) convertToString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// parseFeishuTimestamp parses Feishu timestamp string to Unix timestamp
func parseFeishuTimestamp(ts string) int64 {
	if ts == "" {
		return 0
	}

	// Feishu timestamps are in milliseconds
	var ms int64
	if _, err := fmt.Sscanf(ts, "%d", &ms); err == nil {
		return ms / 1000
	}

	return 0
}

// ParseFeishuTimestamp is exported for use in platform code
func ParseFeishuTimestamp(ts string) int64 {
	return parseFeishuTimestamp(ts)
}
