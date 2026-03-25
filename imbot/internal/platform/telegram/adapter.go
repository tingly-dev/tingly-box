package telegram

import (
	"context"
	"fmt"
	"strconv"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/tingly-dev/tingly-box/imbot/internal/adapter"
	"github.com/tingly-dev/tingly-box/imbot/internal/builder"
	"github.com/tingly-dev/tingly-box/imbot/internal/content"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Adapter adapts Telegram events to core.Message
// It implements adapter.MessageAdapter interface
type Adapter struct {
	*adapter.BaseAdapter
	api *tgbot.Bot
}

// NewAdapter creates a new Telegram adapter
func NewAdapter(config *core.Config, api *tgbot.Bot) *Adapter {
	return &Adapter{
		BaseAdapter: adapter.NewBaseAdapter(config),
		api:         api,
	}
}

// Platform returns core.PlatformTelegram
func (a *Adapter) Platform() core.Platform {
	return core.PlatformTelegram
}

// AdaptMessage converts a Telegram message to core.Message
func (a *Adapter) AdaptMessage(ctx context.Context, msg *models.Message) (*core.Message, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}

	// Determine chat type
	chatType := a.getChatType(&msg.Chat)

	// Build message using fluent builder
	messageBuilder := builder.NewMessageBuilder(core.PlatformTelegram).
		WithID(strconv.Itoa(msg.ID)).
		WithTimestamp(int64(msg.Date)).
		WithRecipient(strconv.FormatInt(msg.Chat.ID, 10), string(chatType), msg.Chat.Title).
		WithSenderFrom(a.extractSender(msg.From)).
		WithContent(a.extractContent(msg)).
		WithMetadata("raw_update", msg) // Store raw for platform-specific access

	// Add thread context if reply
	if msg.ReplyToMessage != nil {
		messageBuilder.WithReplyTo(
			strconv.Itoa(msg.ReplyToMessage.ID),
			strconv.Itoa(msg.ReplyToMessage.ID),
		)
	}

	return messageBuilder.Build(), nil
}

// AdaptCallback converts a Telegram callback query to core.Message
func (a *Adapter) AdaptCallback(ctx context.Context, query *models.CallbackQuery) (*core.Message, error) {
	if query == nil {
		return nil, fmt.Errorf("nil callback query")
	}

	// Handle MaybeInaccessibleMessage - check the Type field
	var chatID int64
	var messageID int
	var hasMessage bool
	var textContent string

	msg := query.Message
	switch msg.Type {
	case models.MaybeInaccessibleMessageTypeMessage:
		if msg.Message != nil {
			chatID = msg.Message.Chat.ID
			messageID = msg.Message.ID
			hasMessage = true
			textContent = msg.Message.Text
			if textContent == "" {
				textContent = msg.Message.Caption
			}
		}
	case models.MaybeInaccessibleMessageTypeInaccessibleMessage:
		// Message is inaccessible
		hasMessage = false
		textContent = ""
	}

	// Build callback data text
	callbackText := fmt.Sprintf("callback:%s", query.Data)
	if hasMessage && textContent != "" {
		callbackText = textContent + "\n\n" + callbackText
	}

	messageBuilder := builder.NewMessageBuilder(core.PlatformTelegram).
		WithTimestamp(time.Now().Unix()).
		WithSenderFrom(a.extractSender(&query.From))

	if hasMessage {
		messageBuilder = messageBuilder.
			WithID(strconv.Itoa(messageID)).
			WithRecipient(strconv.FormatInt(chatID, 10), "direct", "")
	}

	messageBuilder = messageBuilder.
		WithTextContent(callbackText, nil).
		WithMetadata("callback_query_id", query.ID).
		WithMetadata("callback_data", query.Data).
		WithMetadata("is_callback", true)

	return messageBuilder.Build(), nil
}

// extractSender extracts sender info from Telegram User
func (a *Adapter) extractSender(user *models.User) core.Sender {
	if user == nil {
		return core.Sender{ID: "unknown"}
	}

	sender := core.Sender{
		ID:          strconv.FormatInt(user.ID, 10),
		Username:    user.Username,
		DisplayName: "",
		Raw:         make(map[string]interface{}),
	}

	// Build display name from first and last name
	if user.FirstName != "" || user.LastName != "" {
		sender.DisplayName = fmt.Sprintf("%s %s", user.FirstName, user.LastName)
	} else if user.Username != "" {
		sender.DisplayName = user.Username
	}

	return sender
}

// extractContent extracts content from a Telegram message
func (a *Adapter) extractContent(msg *models.Message) core.Content {
	// Create content registry
	registry := content.NewRegistry[*models.Message]()

	// Register handlers
	registry.Register(content.NewTextHandler(func(m *models.Message) (string, []core.Entity, bool) {
		if m.Text != "" {
			return m.Text, a.extractEntities(m.Entities), true
		}
		if m.Caption != "" {
			return m.Caption, a.extractEntities(m.CaptionEntities), true
		}
		return "", nil, false
	}))

	registry.Register(content.NewMediaHandler("image", func(m *models.Message) ([]core.MediaAttachment, string, bool) {
		if len(m.Photo) > 0 {
			media := make([]core.MediaAttachment, len(m.Photo))
			for i, photo := range m.Photo {
				media[i] = core.MediaAttachment{
					Type:     "image",
					URL:      a.getTelegramFileURL(photo.FileID),
					Width:    photo.Width,
					Height:   photo.Height,
					Size:     int64(photo.FileSize),
					Raw:      map[string]interface{}{"file_id": photo.FileID, "file_unique_id": photo.FileUniqueID, "platform": "telegram"},
					MimeType: "image/jpeg",
				}
			}
			caption := m.Caption
			return media, caption, true
		}
		return nil, "", false
	}))

	registry.Register(content.NewMediaHandler("document", func(m *models.Message) ([]core.MediaAttachment, string, bool) {
		if m.Document != nil {
			return []core.MediaAttachment{{
				Type:     "document",
				URL:      a.getTelegramFileURL(m.Document.FileID),
				MimeType: m.Document.MimeType,
				Filename: m.Document.FileName,
				Size:     m.Document.FileSize,
				Raw:      map[string]interface{}{"file_unique_id": m.Document.FileUniqueID, "platform": "telegram"},
			}}, m.Caption, true
		}
		return nil, "", false
	}))

	registry.Register(content.NewMediaHandler("sticker", func(m *models.Message) ([]core.MediaAttachment, string, bool) {
		if m.Sticker != nil {
			return []core.MediaAttachment{{
				Type:   "sticker",
				URL:    a.getTelegramFileURL(m.Sticker.FileID),
				Width:  m.Sticker.Width,
				Height: m.Sticker.Height,
				Raw:    map[string]interface{}{"file_unique_id": m.Sticker.FileUniqueID, "platform": "telegram"},
			}}, "", true
		}
		return nil, "", false
	}))

	registry.Register(content.NewCompoundHandler(
		content.NewMediaHandler("video", func(m *models.Message) ([]core.MediaAttachment, string, bool) {
			if m.Video != nil {
				return []core.MediaAttachment{{
					Type:     "video",
					URL:      fmt.Sprintf("file://%s", m.Video.FileID),
					Width:    m.Video.Width,
					Height:   m.Video.Height,
					Duration: m.Video.Duration,
					Raw:      map[string]interface{}{"file_unique_id": m.Video.FileUniqueID},
				}}, m.Caption, true
			}
			return nil, "", false
		}),
		content.NewMediaHandler("audio", func(m *models.Message) ([]core.MediaAttachment, string, bool) {
			if m.Audio != nil {
				return []core.MediaAttachment{{
					Type:     "audio",
					URL:      fmt.Sprintf("file://%s", m.Audio.FileID),
					Duration: m.Audio.Duration,
					Size:     m.Audio.FileSize,
					Raw:      map[string]interface{}{"file_unique_id": m.Audio.FileUniqueID},
				}}, m.Caption, true
			}
			return nil, "", false
		}),
	))

	// Set default for unknown content
	registry.SetDefault(content.NewSystemHandler("unknown", func(m *models.Message) (string, map[string]interface{}, bool) {
		return "unknown", map[string]interface{}{"message_type": "unsupported"}, true
	}))

	// Handle content
	result, err := registry.Handle(context.Background(), msg)
	if err != nil {
		a.Logger().Error("Failed to extract content: %v", err)
		return core.NewSystemContent("error", map[string]interface{}{"error": err.Error()})
	}

	return result
}

// extractEntities converts Telegram entities to core entities
func (a *Adapter) extractEntities(entities []models.MessageEntity) []core.Entity {
	if len(entities) == 0 {
		return nil
	}

	result := make([]core.Entity, len(entities))
	for i, entity := range entities {
		result[i] = core.Entity{
			Type:   a.mapEntityType(string(entity.Type)),
			Offset: entity.Offset,
			Length: entity.Length,
			Data:   a.extractEntityData(entity),
		}
	}
	return result
}

// mapEntityType maps Telegram entity type to core entity type
func (a *Adapter) mapEntityType(entityType string) string {
	// Direct mapping for common entity types
	mappings := map[string]string{
		"mention":       "mention",
		"hashtag":       "hashtag",
		"bot_command":   "bot_command",
		"url":           "url",
		"email":         "email",
		"phone_number":  "phone_number",
		"bold":          "bold",
		"italic":        "italic",
		"code":          "code",
		"pre":           "pre",
		"text_link":     "text_link",
		"text_mention":  "text_mention",
		"underline":     "underline",
		"strikethrough": "strikethrough",
		"spoiler":       "spoiler",
		"cashtag":       "cashtag",
	}

	if mapped, ok := mappings[entityType]; ok {
		return mapped
	}
	return entityType
}

// extractEntityData extracts entity-specific data
func (a *Adapter) extractEntityData(entity models.MessageEntity) map[string]interface{} {
	data := make(map[string]interface{})

	// URL is available in MessageEntity
	if entity.Type == "url" || entity.Type == "text_link" {
		data["url"] = entity.URL
	}

	return data
}

// getChatType maps Telegram chat type to core ChatType
func (a *Adapter) getChatType(chat *models.Chat) core.ChatType {
	switch chat.Type {
	case "private":
		return core.ChatTypeDirect
	case "group", "supergroup":
		return core.ChatTypeGroup
	case "channel":
		return core.ChatTypeChannel
	default:
		return core.ChatTypeDirect
	}
}

// getTelegramFileURL returns the download URL for a Telegram file
func (a *Adapter) getTelegramFileURL(fileID string) string {
	// Store the file ID as a URL scheme that the FileStore can resolve
	// The actual download will be done by the FileStore which has access to the bot API
	return fmt.Sprintf("tgfile://%s", fileID)
}
