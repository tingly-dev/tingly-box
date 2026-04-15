// Package wecom provides WeCom (Enterprise WeChat) platform bot implementation for ImBot.
package wecom

import (
	"context"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/weixin/types"
	"github.com/tingly-dev/weixin/wecom"
)

// ---------------------------------------------------------------------------
// NewBot construction tests
// ---------------------------------------------------------------------------

// TestNewBot_ValidConfig verifies a bot is created when both clientId and clientSecret are present.
func TestNewBot_ValidConfig(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWecom,
		Auth: core.AuthConfig{
			Type:         "oauth",
			ClientID:     "my-bot-id",
			ClientSecret: "my-secret",
		},
	}
	bot, err := NewBot(config)
	if err != nil {
		t.Fatalf("NewBot() unexpected error: %v", err)
	}
	if bot == nil {
		t.Fatal("NewBot() returned nil bot")
	}
}

// TestNewBot_MissingClientID verifies an error is returned when clientId is absent.
func TestNewBot_MissingClientID(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWecom,
		Auth: core.AuthConfig{
			Type:         "oauth",
			ClientSecret: "my-secret",
		},
	}
	_, err := NewBot(config)
	if err == nil {
		t.Fatal("NewBot() expected error for missing clientId, got nil")
	}
}

// TestNewBot_MissingClientSecret verifies an error is returned when clientSecret is absent.
func TestNewBot_MissingClientSecret(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWecom,
		Auth: core.AuthConfig{
			Type:     "oauth",
			ClientID: "my-bot-id",
		},
	}
	_, err := NewBot(config)
	if err == nil {
		t.Fatal("NewBot() expected error for missing clientSecret, got nil")
	}
}

// ---------------------------------------------------------------------------
// Bot state tests
// ---------------------------------------------------------------------------

// TestBotState_InitiallyDisconnected verifies a freshly created bot is not connected.
func TestBotState_InitiallyDisconnected(t *testing.T) {
	bot := newTestBot(t)

	if bot.IsConnected() {
		t.Error("new bot should not be connected")
	}
	status := bot.Status()
	if status.Connected || status.Authenticated || status.Ready {
		t.Error("initial status should be all false")
	}
}

// TestPlatformInfo verifies PlatformInfo returns the WeCom identifier and name.
func TestPlatformInfo(t *testing.T) {
	bot := newTestBot(t)

	info := bot.PlatformInfo()
	if info.ID != core.PlatformWecom {
		t.Errorf("PlatformInfo().ID = %q, want %q", info.ID, core.PlatformWecom)
	}
	if info.Name != "WeCom" {
		t.Errorf("PlatformInfo().Name = %q, want %q", info.Name, "WeCom")
	}
}

// ---------------------------------------------------------------------------
// Unsupported operations tests
// ---------------------------------------------------------------------------

// TestUnsupportedOperations_React verifies React returns ErrPlatformError (not supported on WeCom).
func TestUnsupportedOperations_React(t *testing.T) {
	bot := newTestBot(t)
	err := bot.React(context.Background(), "msg-1", "👍")
	if err == nil {
		t.Fatal("React() should return an error")
	}
	if core.GetErrorCode(err) != core.ErrPlatformError {
		t.Errorf("React() error code = %v, want %v", core.GetErrorCode(err), core.ErrPlatformError)
	}
}

// TestUnsupportedOperations_EditMessage verifies EditMessage returns ErrPlatformError.
func TestUnsupportedOperations_EditMessage(t *testing.T) {
	bot := newTestBot(t)
	err := bot.EditMessage(context.Background(), "msg-1", "new text")
	if err == nil {
		t.Fatal("EditMessage() should return an error")
	}
	if core.GetErrorCode(err) != core.ErrPlatformError {
		t.Errorf("EditMessage() error code = %v, want %v", core.GetErrorCode(err), core.ErrPlatformError)
	}
}

// TestUnsupportedOperations_DeleteMessage verifies DeleteMessage returns ErrPlatformError.
func TestUnsupportedOperations_DeleteMessage(t *testing.T) {
	bot := newTestBot(t)
	err := bot.DeleteMessage(context.Background(), "msg-1")
	if err == nil {
		t.Fatal("DeleteMessage() should return an error")
	}
	if core.GetErrorCode(err) != core.ErrPlatformError {
		t.Errorf("DeleteMessage() error code = %v, want %v", core.GetErrorCode(err), core.ErrPlatformError)
	}
}

// ---------------------------------------------------------------------------
// Adapter message conversion tests
// ---------------------------------------------------------------------------

// TestAdapter_Platform verifies the adapter reports PlatformWecom.
func TestAdapter_Platform(t *testing.T) {
	adapter := newTestAdapter(t)
	if adapter.Platform() != core.PlatformWecom {
		t.Errorf("Adapter.Platform() = %q, want %q", adapter.Platform(), core.PlatformWecom)
	}
}

// TestAdapter_TextMessage verifies a text types.Message converts to a core.Message with TextContent.
func TestAdapter_TextMessage(t *testing.T) {
	adapter := newTestAdapter(t)

	msg := &types.Message{
		MessageID:    "m001",
		From:         "user-abc",
		SenderID:     "user-abc",
		To:           "bot-xyz",
		ChatType:     types.ChatTypeDirect,
		Timestamp:    time.Unix(1700000000, 0),
		Text:         "Hello WeCom",
		ContextToken: "req-001",
		Metadata:     map[string]interface{}{"chatId": "chat-1"},
	}

	coreMsg, err := adapter.AdaptMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("AdaptMessage() error: %v", err)
	}
	if coreMsg.Platform != core.PlatformWecom {
		t.Errorf("Platform = %q, want %q", coreMsg.Platform, core.PlatformWecom)
	}
	if coreMsg.ID != "m001" {
		t.Errorf("ID = %q, want m001", coreMsg.ID)
	}
	if coreMsg.Sender.ID != "user-abc" {
		t.Errorf("Sender.ID = %q, want user-abc", coreMsg.Sender.ID)
	}
	content, ok := coreMsg.Content.(*core.TextContent)
	if !ok {
		t.Fatalf("Content type = %T, want *core.TextContent", coreMsg.Content)
	}
	if content.Text != "Hello WeCom" {
		t.Errorf("Content.Text = %q, want %q", content.Text, "Hello WeCom")
	}
	// ContextToken must be preserved in metadata for reply routing
	if ct, _ := coreMsg.Metadata["context_token"].(string); ct != "req-001" {
		t.Errorf("metadata[context_token] = %q, want req-001", ct)
	}
}

// TestAdapter_NilMessage verifies AdaptMessage returns an error on nil input.
func TestAdapter_NilMessage(t *testing.T) {
	adapter := newTestAdapter(t)
	_, err := adapter.AdaptMessage(context.Background(), nil)
	if err == nil {
		t.Fatal("AdaptMessage(nil) should return an error")
	}
}

// TestAdapter_DirectChatType verifies "single" / ChatTypeDirect maps to ChatTypeDirect.
func TestAdapter_DirectChatType(t *testing.T) {
	adapter := newTestAdapter(t)
	msg := baseMessage()
	msg.ChatType = types.ChatTypeDirect

	coreMsg, err := adapter.AdaptMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("AdaptMessage() error: %v", err)
	}
	if coreMsg.Recipient.Type != string(core.ChatTypeDirect) {
		t.Errorf("ChatType = %q, want %q", coreMsg.Recipient.Type, core.ChatTypeDirect)
	}
}

// TestAdapter_GroupChatType verifies ChatTypeGroup maps to ChatTypeGroup.
func TestAdapter_GroupChatType(t *testing.T) {
	adapter := newTestAdapter(t)
	msg := baseMessage()
	msg.ChatType = types.ChatTypeGroup
	msg.Metadata = map[string]interface{}{"chatId": "group-chat-1"}

	coreMsg, err := adapter.AdaptMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("AdaptMessage() error: %v", err)
	}
	if coreMsg.Recipient.Type != string(core.ChatTypeGroup) {
		t.Errorf("ChatType = %q, want %q", coreMsg.Recipient.Type, core.ChatTypeGroup)
	}
}

// TestAdapter_ImageAttachment verifies an image attachment is mapped to core.MediaContent.
func TestAdapter_ImageAttachment(t *testing.T) {
	adapter := newTestAdapter(t)
	msg := baseMessage()
	msg.Text = ""
	msg.Attachments = []types.Attachment{
		{URL: "https://cdn.example.com/img.png", MimeType: "image"},
	}

	coreMsg, err := adapter.AdaptMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("AdaptMessage() error: %v", err)
	}
	mediaContent, ok := coreMsg.Content.(*core.MediaContent)
	if !ok {
		t.Fatalf("Content type = %T, want *core.MediaContent", coreMsg.Content)
	}
	if len(mediaContent.Media) == 0 {
		t.Fatal("MediaContent has no attachments")
	}
	if mediaContent.Media[0].Type != "image" {
		t.Errorf("media[0].Type = %q, want image", mediaContent.Media[0].Type)
	}
}

// TestAdapter_MixedTextAndImage verifies mixed text+image maps to MediaContent with caption.
func TestAdapter_MixedTextAndImage(t *testing.T) {
	adapter := newTestAdapter(t)
	msg := baseMessage()
	msg.Text = "check this out"
	msg.Attachments = []types.Attachment{
		{URL: "https://cdn.example.com/photo.jpg", MimeType: "image"},
	}

	coreMsg, err := adapter.AdaptMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("AdaptMessage() error: %v", err)
	}
	mediaContent, ok := coreMsg.Content.(*core.MediaContent)
	if !ok {
		t.Fatalf("Content type = %T, want *core.MediaContent", coreMsg.Content)
	}
	if mediaContent.Caption != "check this out" {
		t.Errorf("Caption = %q, want %q", mediaContent.Caption, "check this out")
	}
}

// TestAdapter_VoiceMessage verifies a voice message (speech-to-text) maps to TextContent.
func TestAdapter_VoiceMessage(t *testing.T) {
	adapter := newTestAdapter(t)
	msg := baseMessage()
	msg.Text = "transcribed voice text"

	coreMsg, err := adapter.AdaptMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("AdaptMessage() error: %v", err)
	}
	content, ok := coreMsg.Content.(*core.TextContent)
	if !ok {
		t.Fatalf("Content type = %T, want *core.TextContent", coreMsg.Content)
	}
	if content.Text != "transcribed voice text" {
		t.Errorf("Content.Text = %q, want %q", content.Text, "transcribed voice text")
	}
}

// TestAdapter_ContextTokenPreserved verifies the SDK's ContextToken (req_id) is
// round-tripped through metadata["context_token"] for reply routing.
func TestAdapter_ContextTokenPreserved(t *testing.T) {
	adapter := newTestAdapter(t)
	msg := baseMessage()
	msg.ContextToken = "ws-req-id-999"

	coreMsg, err := adapter.AdaptMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("AdaptMessage() error: %v", err)
	}
	got, _ := coreMsg.Metadata["context_token"].(string)
	if got != "ws-req-id-999" {
		t.Errorf("metadata[context_token] = %q, want ws-req-id-999", got)
	}
}

// TestAdapter_GetMessageLimit verifies WeCom's message limit matches WeCom's limit.
func TestAdapter_GetMessageLimit(t *testing.T) {
	adapter := newTestAdapter(t)
	// WeCom supports up to 4000 characters for markdown proactive messages.
	if adapter.GetMessageLimit() <= 0 {
		t.Errorf("GetMessageLimit() = %d, want > 0", adapter.GetMessageLimit())
	}
}

// ---------------------------------------------------------------------------
// EventHandler wiring test
// ---------------------------------------------------------------------------

// TestBot_EventHandlerWired verifies that when the WecomBot fires a message event,
// the bot's OnMessage callbacks are invoked.
func TestBot_EventHandlerWired(t *testing.T) {
	bot := newTestBot(t)

	received := make(chan core.Message, 1)
	bot.OnMessage(func(msg core.Message) {
		received <- msg
	})

	// Simulate the SDK delivering a message via the internal event handler.
	// We call the handler directly since we cannot actually connect to WeCom in tests.
	testMsg := &types.Message{
		MessageID:    "test-msg-1",
		From:         "user-1",
		SenderID:     "user-1",
		To:           "bot-id",
		ChatType:     types.ChatTypeDirect,
		Timestamp:    time.Now(),
		Text:         "ping",
		ContextToken: "req-123",
		Metadata:     map[string]interface{}{},
	}

	bot.handleIncomingMessage(context.Background(), testMsg)

	select {
	case msg := <-received:
		if msg.ID != "test-msg-1" {
			t.Errorf("received message ID = %q, want test-msg-1", msg.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: OnMessage callback not called")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestBot(t *testing.T) *Bot {
	t.Helper()
	config := &core.Config{
		Platform: core.PlatformWecom,
		Auth: core.AuthConfig{
			Type:         "oauth",
			ClientID:     "test-bot-id",
			ClientSecret: "test-secret",
		},
	}
	bot, err := NewBot(config)
	if err != nil {
		t.Fatalf("NewBot() error: %v", err)
	}
	return bot
}

func newTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	config := &core.Config{
		Platform: core.PlatformWecom,
		Auth: core.AuthConfig{
			Type:         "oauth",
			ClientID:     "test-bot-id",
			ClientSecret: "test-secret",
		},
	}
	return NewAdapter(config)
}

func baseMessage() *types.Message {
	return &types.Message{
		MessageID:    "m001",
		From:         "user-abc",
		SenderID:     "user-abc",
		To:           "bot-xyz",
		ChatType:     types.ChatTypeDirect,
		Timestamp:    time.Unix(1700000000, 0),
		Text:         "hello",
		ContextToken: "req-001",
		Metadata:     map[string]interface{}{},
	}
}

// Ensure WecomBot is importable (compile-time check).
var _ = (*wecom.WecomBot)(nil)
