package telegram

import (
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/internal/builder"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/menu"
)

func TestMenuAdapterSupports(t *testing.T) {
	adapter := NewMenuAdapter()

	if !adapter.Supports(core.PlatformTelegram) {
		t.Errorf("Expected adapter to support Telegram")
	}

	if adapter.Supports(core.PlatformLark) {
		t.Errorf("Expected adapter not to support Lark")
	}
}

func TestMenuAdapterConvertToInlineKeyboard(t *testing.T) {
	adapter := NewMenuAdapter()

	m := menu.NewBuilder("test-menu", menu.MenuTypeInlineKeyboard).
		AddRow(
			menu.CallbackItem("btn1", "Button 1", "val1"),
			menu.CallbackItem("btn2", "Button 2", "val2"),
		).
		AddRow(
			menu.URLItem("btn3", "Link", "https://example.com"),
		).
		Build()

	markup, err := adapter.ConvertMenu(m)
	if err != nil {
		t.Fatalf("ConvertMenu failed: %v", err)
	}

	kb, ok := markup.(builder.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("Expected InlineKeyboardMarkup, got %T", markup)
	}

	if len(kb.InlineKeyboard) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(kb.InlineKeyboard))
	}

	if len(kb.InlineKeyboard[0]) != 2 {
		t.Errorf("Expected 2 buttons in first row, got %d", len(kb.InlineKeyboard[0]))
	}

	if kb.InlineKeyboard[0][0].Text != "Button 1" {
		t.Errorf("Expected text 'Button 1', got '%s'", kb.InlineKeyboard[0][0].Text)
	}

	if kb.InlineKeyboard[1][0].URL != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got '%s'", kb.InlineKeyboard[1][0].URL)
	}
}

func TestMenuAdapterConvertToReplyKeyboard(t *testing.T) {
	adapter := NewMenuAdapter()

	m := menu.NewBuilder("test-menu", menu.MenuTypeReplyKeyboard).
		AddRow(
			menu.NewItem("btn1", "Button 1").WithIcon("1️⃣"),
			menu.NewItem("btn2", "Button 2").WithIcon("2️⃣"),
		).
		Build()

	markup, err := adapter.ConvertMenu(m)
	if err != nil {
		t.Fatalf("ConvertMenu failed: %v", err)
	}

	if markup == nil {
		t.Fatal("Expected non-nil markup")
	}
}

func TestMenuAdapterParseAction(t *testing.T) {
	adapter := NewMenuAdapter()

	msg := builder.NewMessageBuilder(core.PlatformTelegram).
		WithID("msg-1").
		WithSender("user-1", "", "").
		WithRecipient("chat-1", "direct", "").
		WithContent(core.NewTextContent("test")).
		WithMetadata("callback_data", "menu_id:item_id:value").
		Build()

	action, err := adapter.ParseAction(msg)
	if err != nil {
		t.Fatalf("ParseAction failed: %v", err)
	}

	if action.MenuID != "menu_id" {
		t.Errorf("Expected MenuID 'menu_id', got '%s'", action.MenuID)
	}

	if action.ItemID != "item_id" {
		t.Errorf("Expected ItemID 'item_id', got '%s'", action.ItemID)
	}

	if action.Value != "value" {
		t.Errorf("Expected Value 'value', got '%s'", action.Value)
	}

	if action.Action != "callback" {
		t.Errorf("Expected Action 'callback', got '%s'", action.Action)
	}
}

func TestMenuAdapterGetKeyboardMarkupForMessage(t *testing.T) {
	adapter := NewMenuAdapter()

	m := menu.NewBuilder("test-menu", menu.MenuTypeInlineKeyboard).
		AddRow(
			menu.CallbackItem("btn1", "Button 1", "val1"),
		).
		Build()

	kb, err := adapter.GetKeyboardMarkupForMessage(m)
	if err != nil {
		t.Fatalf("GetKeyboardMarkupForMessage failed: %v", err)
	}

	if len(kb.InlineKeyboard) != 1 {
		t.Errorf("Expected 1 row, got %d", len(kb.InlineKeyboard))
	}
}

func TestMenuAdapterResolveChatIDString(t *testing.T) {
	adapter := NewMenuAdapter()

	tests := []struct {
		name    string
		chatID  string
		wantErr bool
	}{
		{"Numeric", "123456789", false},
		{"String", "abc123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := adapter.ResolveChatIDString(tt.chatID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveChatIDString() error = %v, wantErr %v", err, tt.wantErr)
			}
			if result != tt.chatID {
				t.Errorf("ResolveChatIDString() = %v, want %v", result, tt.chatID)
			}
		})
	}
}
