package adapter

import (
	"context"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/testutil"
)

// MockTelegramAPI is a mock implementation for testing
type MockTelegramAPI struct {
	messages []tgbotapi.Message
}

func (m *MockTelegramAPI) GetChat(chatID int64) (tgbotapi.Chat, error) {
	return tgbotapi.Chat{}, nil
}

// TestTelegramAdapter tests the Telegram adapter
func TestTelegramAdapter_AdaptMessage(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformTelegram,
	}
	api := &MockTelegramAPI{}

	adapter := NewTelegramAdapter(config, api)

	// Create a test message
	msg := &tgbotapi.Message{
		MessageID: 12345,
		Date:      1705200000,
		Chat: tgbotapi.Chat{
			ID:   987654321,
			Type: "private",
		},
		From: &tgbotapi.User{
			ID:        111111,
			UserName:  "testuser",
			FirstName: "Test",
			LastName:  "User",
		},
		Text: "Hello, world!",
	}

	// Adapt the message
	result, err := adapter.AdaptMessage(context.Background(), msg)

	// Assert results
	if err != nil {
		t.Fatalf("AdaptMessage failed: %v", err)
	}

	if result.ID != "12345" {
		t.Errorf("Expected ID '12345', got '%s'", result.ID)
	}

	if result.Platform != core.PlatformTelegram {
		t.Errorf("Expected platform 'telegram', got '%s'", result.Platform)
	}

	if result.Sender.ID != "111111" {
		t.Errorf("Expected sender ID '111111', got '%s'", result.Sender.ID)
	}

	if result.Sender.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", result.Sender.Username)
	}

	if result.Sender.DisplayName != "Test User" {
		t.Errorf("Expected display name 'Test User', got '%s'", result.Sender.DisplayName)
	}

	if result.ChatType != core.ChatTypeDirect {
		t.Errorf("Expected chat type 'direct', got '%s'", result.ChatType)
	}

	text := result.GetText()
	if text != "Hello, world!" {
		t.Errorf("Expected text 'Hello, world!', got '%s'", text)
	}
}

func TestTelegramAdapter_AdaptMessageWithPhoto(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformTelegram,
	}
	api := &MockTelegramAPI{}

	adapter := NewTelegramAdapter(config, api)

	// Create a test message with photo
	msg := &tgbotapi.Message{
		MessageID: 12346,
		Date:      1705200000,
		Chat: tgbotapi.Chat{
			ID:   987654321,
			Type: "private",
		},
		From: &tgbotapi.User{
			ID:       111111,
			UserName: "photouser",
		},
		Photo: []tgbotapi.PhotoSize{
			{
				FileID:       "AgACAgADAADMADECzFZjSVK",
				FileUniqueID: "AQADMADECzFZjSVK",
				Width:        800,
				Height:       600,
			},
		},
		Caption: "Look at this!",
	}

	// Adapt the message
	result, err := adapter.AdaptMessage(context.Background(), msg)

	// Assert results
	if err != nil {
		t.Fatalf("AdaptMessage failed: %v", err)
	}

	if !result.IsMediaContent() {
		t.Fatal("Expected media content")
	}

	media := result.GetMedia()
	if len(media) != 1 {
		t.Fatalf("Expected 1 media item, got %d", len(media))
	}

	if media[0].Type != "image" {
		t.Errorf("Expected media type 'image', got '%s'", media[0].Type)
	}

	if media[0].Width != 800 {
		t.Errorf("Expected width 800, got %d", media[0].Width)
	}

	if media[0].Height != 600 {
		t.Errorf("Expected height 600, got %d", media[0].Height)
	}
}

func TestTelegramAdapter_AdaptMessageWithReply(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformTelegram,
	}
	api := &MockTelegramAPI{}

	adapter := NewTelegramAdapter(config, api)

	// Create a test message with reply
	msg := &tgbotapi.Message{
		MessageID: 12347,
		Date:      1705200000,
		Chat: tgbotapi.Chat{
			ID:   987654321,
			Type: "private",
		},
		From: &tgbotapi.User{
			ID:       111111,
			UserName: "replyuser",
		},
		Text: "This is a reply",
		ReplyToMessage: &tgbotapi.Message{
			MessageID: 12340,
		},
	}

	// Adapt the message
	result, err := adapter.AdaptMessage(context.Background(), msg)

	// Assert results
	if err != nil {
		t.Fatalf("AdaptMessage failed: %v", err)
	}

	if result.ThreadContext == nil {
		t.Fatal("Expected thread context")
	}

	if result.ThreadContext.ParentMessageID != "12340" {
		t.Errorf("Expected parent message ID '12340', got '%s'", result.ThreadContext.ParentMessageID)
	}
}

func TestTelegramAdapter_AdaptCallback(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformTelegram,
	}
	api := &MockTelegramAPI{}

	adapter := NewTelegramAdapter(config, api)

	// Create a test callback query
	query := &tgbotapi.CallbackQuery{
		ID:   "callback_123",
		Data: "button_click",
		From: &tgbotapi.User{
			ID:       111111,
			UserName: "callbackuser",
		},
		Message: &tgbotapi.Message{
			MessageID: 12345,
			Chat: tgbotapi.Chat{
				ID: 987654321,
			},
		},
	}

	// Adapt the callback
	result, err := adapter.AdaptCallback(context.Background(), query)

	// Assert results
	if err != nil {
		t.Fatalf("AdaptCallback failed: %v", err)
	}

	callbackText := result.GetText()
	if callbackText != "callback:button_click" {
		t.Errorf("Expected callback text 'callback:button_click', got '%s'", callbackText)
	}

	isCallback := result.Metadata["is_callback"].(bool)
	if !isCallback {
		t.Error("Expected is_callback metadata to be true")
	}
}
