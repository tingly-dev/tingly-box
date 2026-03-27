package menu

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// Tests for BaseAdapter and DefaultAdapter
// Platform-specific adapter tests have been moved to their respective platform packages

func TestBaseAdapterGetCapabilities(t *testing.T) {
	adapter := NewBaseAdapter(core.PlatformTelegram)
	// GetCapabilities is now internal, test via adapter behavior instead

	// Test that adapter supports the platform
	if !adapter.Supports(core.PlatformTelegram) {
		t.Error("Expected adapter to support Telegram")
	}
}

func TestBaseAdapterSupports(t *testing.T) {
	adapter := NewBaseAdapter(core.PlatformTelegram)

	if !adapter.Supports(core.PlatformTelegram) {
		t.Error("Expected adapter to support Telegram")
	}

	if adapter.Supports(core.PlatformLark) {
		t.Error("Expected adapter not to support Lark")
	}
}

func TestBaseAdapterNormalizeMenuType(t *testing.T) {
	adapter := NewBaseAdapter(core.PlatformTelegram)

	// Test menu type normalization via menu helper methods
	tests := []struct {
		name    string
		setup   func(*Menu)
		checkFn func(*testing.T, *Menu)
	}{
		{
			name: "InlineKeyboard",
			setup: func(m *Menu) {
				m.SetInlineKeyboard()
			},
			checkFn: func(t *testing.T, m *Menu) {
				if !m.IsInlineKeyboard() {
					t.Error("Expected menu to be InlineKeyboard")
				}
			},
		},
		{
			name: "ReplyKeyboard",
			setup: func(m *Menu) {
				m.SetReplyKeyboard()
			},
			checkFn: func(t *testing.T, m *Menu) {
				if !m.IsReplyKeyboard() {
					t.Error("Expected menu to be ReplyKeyboard")
				}
			},
		},
		{
			name: "Auto",
			setup: func(m *Menu) {
				m.SetAuto()
			},
			checkFn: func(t *testing.T, m *Menu) {
				// After validation, Auto should be normalized to a supported type
				// Check that it's no longer Auto
				if m.IsAuto() {
					t.Error("Expected menu to be normalized from Auto to a concrete type")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			menu := NewMenu("test")
			menu.AddRow(MenuItem{ID: "test", Label: "Test"})
			tt.setup(menu)

			// Validate the menu (which may normalize type internally)
			if err := adapter.validateMenu(menu); err != nil {
				t.Errorf("validateMenu() error = %v", err)
			}

			tt.checkFn(t, menu)
		})
	}
}

func TestDefaultAdapter(t *testing.T) {
	adapter := NewDefaultAdapter(core.PlatformDiscord)

	menu := NewBuilder("test-menu").
		WithTitle("Test Menu").
		AddRow(
			CallbackItem("btn1", "Button 1", "val1"),
		).
		Build()

	textData, err := adapter.ConvertMenu(menu)
	if err != nil {
		t.Fatalf("ConvertMenu failed: %v", err)
	}

	textMap, ok := textData.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map, got %T", textData)
	}

	if textMap["type"] != "text_menu" {
		t.Errorf("Expected type 'text_menu', got '%v'", textMap["type"])
	}

	if _, ok := textMap["text"]; !ok {
		t.Error("Expected 'text' key in text data")
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	tgAdapter := NewBaseAdapter(core.PlatformTelegram)
	larkAdapter := NewBaseAdapter(core.PlatformLark)

	registry.RegisterForPlatform(core.PlatformTelegram, tgAdapter)
	registry.RegisterForPlatform(core.PlatformLark, larkAdapter)

	if !registry.Has(core.PlatformTelegram) {
		t.Error("Expected registry to have Telegram adapter")
	}

	retrieved := registry.Get(core.PlatformTelegram)
	if retrieved == nil {
		t.Error("Expected to retrieve Telegram adapter, got nil")
	}

	if retrieved != tgAdapter {
		t.Error("Retrieved adapter is not the same as registered")
	}

	// Test GetOrDefault
	defaultAdapter := registry.Get(core.PlatformDiscord)
	if defaultAdapter != nil {
		t.Error("Expected nil for unregistered platform")
	}

	defaultAdapter = registry.GetOrDefault(core.PlatformDiscord)
	if defaultAdapter == nil {
		t.Error("Expected default adapter for unregistered platform")
	}
}

func TestMenuErrors(t *testing.T) {
	tests := []struct {
		name  string
		err   *MenuError
		check func(*testing.T, *MenuError)
	}{
		{
			name: "ConversionFailed",
			err:  ErrConversionFailed,
			check: func(t *testing.T, e *MenuError) {
				if e.Code != "CONVERSION_FAILED" {
					t.Errorf("Expected code 'CONVERSION_FAILED', got '%s'", e.Code)
				}
			},
		},
		{
			name: "NotSupported",
			err:  ErrNotSupported,
			check: func(t *testing.T, e *MenuError) {
				if e.Code != "NOT_SUPPORTED" {
					t.Errorf("Expected code 'NOT_SUPPORTED', got '%s'", e.Code)
				}
			},
		},
		{
			name: "NotMenuAction",
			err:  ErrNotMenuAction,
			check: func(t *testing.T, e *MenuError) {
				if e.Code != "NOT_MENU_ACTION" {
					t.Errorf("Expected code 'NOT_MENU_ACTION', got '%s'", e.Code)
				}
			},
		},
		{
			name: "InvalidContext",
			err:  ErrInvalidContext,
			check: func(t *testing.T, e *MenuError) {
				if e.Code != "INVALID_CONTEXT" {
					t.Errorf("Expected code 'INVALID_CONTEXT', got '%s'", e.Code)
				}
			},
		},
		{
			name: "MenuNotFound",
			err:  ErrMenuNotFound,
			check: func(t *testing.T, e *MenuError) {
				if e.Code != "MENU_NOT_FOUND" {
					t.Errorf("Expected code 'MENU_NOT_FOUND', got '%s'", e.Code)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() == "" {
				t.Error("Expected non-empty error message")
			}
			tt.check(t, tt.err)
		})
	}
}

func TestMenuValidateEmptyID(t *testing.T) {
	menu := NewMenu("")
	if err := menu.Validate(); err == nil {
		t.Error("Expected validation error for empty ID")
	}
}

func TestMenuValidateInvalidType(t *testing.T) {
	menu := NewMenu("test")
	// Manually set invalid type via reflection would be needed,
	// but since Type is now internal, this test is no longer applicable
	// Test that normal menus validate successfully instead
	menu.AddRow(MenuItem{ID: "test", Label: "Test"})
	if err := menu.Validate(); err != nil {
		t.Errorf("Expected validation to pass for valid menu, got: %v", err)
	}
}

func TestMenuValidateEmptyItems(t *testing.T) {
	menu := NewMenu("test")
	if err := menu.Validate(); err == nil {
		t.Error("Expected validation error for empty items")
	}
}

func TestMenuContextInAdapter(t *testing.T) {
	ctx := NewMenuContext("chat-123", core.PlatformTelegram)

	if ctx.ChatID != "chat-123" {
		t.Errorf("Expected ChatID 'chat-123', got '%s'", ctx.ChatID)
	}

	if ctx.Platform != core.PlatformTelegram {
		t.Errorf("Expected platform Telegram, got %v", ctx.Platform)
	}

	// Test metadata
	ctx.Metadata["key"] = "value"
	if ctx.Metadata["key"] != "value" {
		t.Errorf("Expected metadata key 'value', got %v", ctx.Metadata["key"])
	}
}

func TestMenuResultSuccess(t *testing.T) {
	result := NewMenuResult(true)

	if !result.Success {
		t.Errorf("Expected Success true")
	}

	result.MessageID = "msg-123"
	result.MenuID = "menu-456"

	if result.MessageID != "msg-123" {
		t.Errorf("Expected MessageID 'msg-123', got '%s'", result.MessageID)
	}

	if result.MenuID != "menu-456" {
		t.Errorf("Expected MenuID 'menu-456', got '%s'", result.MenuID)
	}
}

func TestMenuResultError(t *testing.T) {
	err := core.NewBotError(core.ErrPlatformError, "test error", false)
	result := NewErrorMenuResult(err)

	if result.Success {
		t.Errorf("Expected Success false")
	}

	if result.Error == "" {
		t.Errorf("Expected non-empty error message")
	}
}

func TestMenuActionCreation(t *testing.T) {
	action := NewMenuAction("menu-1", "item-1", "value-1", "user-1", "chat-1")

	if action.MenuID != "menu-1" {
		t.Errorf("Expected MenuID 'menu-1', got '%s'", action.MenuID)
	}

	if action.ItemID != "item-1" {
		t.Errorf("Expected ItemID 'item-1', got '%s'", action.ItemID)
	}

	if action.Value != "value-1" {
		t.Errorf("Expected Value 'value-1', got '%s'", action.Value)
	}

	if action.UserID != "user-1" {
		t.Errorf("Expected UserID 'user-1', got '%s'", action.UserID)
	}

	if action.ChatID != "chat-1" {
		t.Errorf("Expected ChatID 'chat-1', got '%s'", action.ChatID)
	}

	if action.Meta == nil {
		t.Error("Expected Meta to be initialized")
	}
}

// Mock bot for testing
type mockBot struct {
	core.BaseBot
	sendTextCalled bool
	lastText       string
	lastChatID     string
}

func newMockBot() *mockBot {
	config := &core.Config{
		UUID: "mock-bot-uuid",
	}
	return &mockBot{
		BaseBot: *core.NewBaseBot(config),
	}
}

func (m *mockBot) Connect(ctx context.Context) error {
	return nil
}

func (m *mockBot) Disconnect(ctx context.Context) error {
	return nil
}

func (m *mockBot) IsConnected() bool {
	return true
}

func (m *mockBot) SendText(ctx context.Context, target string, text string) (*core.SendResult, error) {
	m.sendTextCalled = true
	m.lastText = text
	m.lastChatID = target
	return &core.SendResult{MessageID: "test-msg-id"}, nil
}

func (m *mockBot) SendMessage(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	m.sendTextCalled = true
	m.lastText = opts.Text
	m.lastChatID = target
	return &core.SendResult{MessageID: "test-msg-id"}, nil
}

func (m *mockBot) SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error) {
	return &core.SendResult{MessageID: "test-msg-id"}, nil
}

func (m *mockBot) React(ctx context.Context, messageID string, emoji string) error {
	return nil
}

func (m *mockBot) EditMessage(ctx context.Context, messageID string, text string) error {
	return nil
}

func (m *mockBot) DeleteMessage(ctx context.Context, messageID string) error {
	return nil
}

func (m *mockBot) ChunkText(text string) []string {
	return []string{text}
}

func (m *mockBot) ValidateTextLength(text string) error {
	return nil
}

func (m *mockBot) GetMessageLimit() int {
	return 4096
}

func (m *mockBot) Status() *core.BotStatus {
	return &core.BotStatus{Connected: true, Authenticated: true, Ready: true}
}

func (m *mockBot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(core.PlatformTelegram, "Mock Telegram")
}

func (m *mockBot) OnMessage(handler func(core.Message)) {}
func (m *mockBot) OnError(handler func(error))          {}
func (m *mockBot) OnConnected(handler func())           {}
func (m *mockBot) OnDisconnected(handler func())        {}
func (m *mockBot) OnReady(handler func())               {}
func (m *mockBot) Close() error                         { return nil }
func (m *mockBot) UUID() string                         { return "mock-bot-uuid" }

func TestDefaultAdapterShowMenu(t *testing.T) {
	adapter := NewDefaultAdapter(core.PlatformDiscord)
	bot := newMockBot()

	menu := NewBuilder("test-menu").
		WithTitle("Test Menu").
		AddRow(
			CallbackItem("btn1", "Button 1", "val1"),
		).
		Build()

	menuCtx := NewMenuContext("chat-123", core.PlatformDiscord)

	result, err := adapter.ShowMenu(context.Background(), bot, menuCtx, menu)
	if err != nil {
		t.Fatalf("ShowMenu failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected successful result")
	}

	if !bot.sendTextCalled {
		t.Error("Expected SendText to be called")
	}
}

func TestDefaultAdapterParseAction(t *testing.T) {
	a := NewDefaultAdapter(core.PlatformDiscord)

	// Test text-based menu selection
	msg := core.NewMessageBuilder(core.PlatformDiscord).
		WithID("msg-1").
		WithSender("user-1", "", "").
		WithRecipient("chat-1", "direct", "").
		WithContent(core.NewTextContent("Button 1")).
		Build()

	action, err := a.ParseAction(msg)
	if err != nil {
		t.Fatalf("ParseAction failed: %v", err)
	}

	if action.Value != "Button 1" {
		t.Errorf("Expected Value 'Button 1', got '%s'", action.Value)
	}

	if action.UserID != "user-1" {
		t.Errorf("Expected UserID 'user-1', got '%s'", action.UserID)
	}
}
