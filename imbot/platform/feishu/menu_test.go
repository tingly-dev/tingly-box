package feishu

import (
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/menu"
)

func TestMenuAdapterSupports(t *testing.T) {
	adapter := NewMenuAdapter()

	if !adapter.Supports(core.PlatformFeishu) {
		t.Errorf("Expected adapter to support Feishu")
	}

	if !adapter.Supports(core.PlatformLark) {
		t.Errorf("Expected adapter to support Lark")
	}

	if adapter.Supports(core.PlatformTelegram) {
		t.Errorf("Expected adapter not to support Telegram")
	}
}

func TestMenuAdapterConvertToInteractiveCard(t *testing.T) {
	adapter := NewMenuAdapter()

	m := menu.NewBuilder("test-menu").
		WithTitle("Test Menu").
		AddRow(
			menu.CallbackItem("btn1", "Button 1", "val1"),
			menu.CallbackItem("btn2", "Button 2", "val2"),
		).
		Build()

	cardData, err := adapter.ConvertMenu(m)
	if err != nil {
		t.Fatalf("ConvertMenu failed: %v", err)
	}

	cardMap, ok := cardData.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map, got %T", cardData)
	}

	if cardMap["type"] != "interactive_card" {
		t.Errorf("Expected type 'interactive_card', got '%v'", cardMap["type"])
	}

	if _, ok := cardMap["card_json"]; !ok {
		t.Error("Expected 'card_json' key in card data")
	}
}

func TestMenuAdapterConvertToQuickActions(t *testing.T) {
	adapter := NewMenuAdapter()

	m := menu.NewBuilder("quick-menu").
		AddRow(
			menu.CallbackItem("action1", "Action 1", "val1"),
			menu.CallbackItem("action2", "Action 2", "val2"),
		).
		Build()

	// Set menu type to QuickActions
	m.SetQuickActions()

	actionData, err := adapter.ConvertMenu(m)
	if err != nil {
		t.Fatalf("ConvertMenu failed: %v", err)
	}

	actionMap, ok := actionData.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map, got %T", actionData)
	}

	if actionMap["type"] != "quick_actions" {
		t.Errorf("Expected type 'quick_actions', got '%v'", actionMap["type"])
	}

	if _, ok := actionMap["actions"]; !ok {
		t.Error("Expected 'actions' key in action data")
	}
}

func TestMenuAdapterParseAction(t *testing.T) {
	a := NewMenuAdapter()

	// Test with callback data in metadata
	msg := core.NewMessageBuilder(core.PlatformFeishu).
		WithID("msg-1").
		WithSender("user-1", "", "").
		WithRecipient("chat-1", "direct", "").
		WithContent(core.NewTextContent("test")).
		WithMetadata("action", "menu_id:item_id:value").
		Build()

	action, err := a.ParseAction(msg)
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

func TestMenuAdapterBuildCardFromMenu(t *testing.T) {
	adapter := NewMenuAdapter()

	m := menu.NewBuilder("test-menu").
		WithTitle("Test Card").
		AddRow(
			menu.CallbackItem("btn1", "Button 1", "val1"),
		).
		Build()

	card, err := adapter.BuildCardFromMenu(m)
	if err != nil {
		t.Fatalf("BuildCardFromMenu failed: %v", err)
	}

	if card == nil {
		t.Fatal("Expected non-nil card")
	}
}

func TestMenuAdapterGetCardJSON(t *testing.T) {
	adapter := NewMenuAdapter()

	m := menu.NewBuilder("test-menu").
		WithTitle("Test Card").
		AddRow(
			menu.CallbackItem("btn1", "Button 1", "val1"),
		).
		Build()

	json, err := adapter.GetCardJSON(m)
	if err != nil {
		t.Fatalf("GetCardJSON failed: %v", err)
	}

	if json == "" {
		t.Error("Expected non-empty JSON string")
	}
}

func TestMenuAdapterCreateQuickActionConfig(t *testing.T) {
	adapter := NewMenuAdapter()

	m := menu.NewBuilder("quick-menu").
		AddRow(
			menu.CallbackItem("action1", "Action 1", "val1"),
		).
		Build()

	config, err := adapter.CreateQuickActionConfig(m)
	if err != nil {
		t.Fatalf("CreateQuickActionConfig failed: %v", err)
	}

	if config == nil {
		t.Fatal("Expected non-nil config")
	}
}

func TestMenuAdapterWithDomain(t *testing.T) {
	feishuAdapter := NewMenuAdapter()
	larkAdapter := NewMenuAdapterWithDomain(DomainLark)

	if !feishuAdapter.Supports(core.PlatformFeishu) {
		t.Error("Expected Feishu adapter to support Feishu")
	}

	if !larkAdapter.Supports(core.PlatformLark) {
		t.Error("Expected Lark adapter to support Lark")
	}
}
