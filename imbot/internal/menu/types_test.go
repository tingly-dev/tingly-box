package menu

import (
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

func TestMenuType(t *testing.T) {
	tests := []struct {
		name     string
		menuType MenuType
		valid    bool
	}{
		{"InlineKeyboard", MenuTypeInlineKeyboard, true},
		{"ReplyKeyboard", MenuTypeReplyKeyboard, true},
		{"ChatMenu", MenuTypeChatMenu, true},
		{"QuickActions", MenuTypeQuickActions, true},
		{"CommandMenu", MenuTypeCommandMenu, true},
		{"Auto", MenuTypeAuto, true},
		{"Invalid", MenuType("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.menuType.IsValid(); got != tt.valid {
				t.Errorf("MenuType.IsValid() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestMenuCreation(t *testing.T) {
	menu := NewMenu("test-menu", MenuTypeInlineKeyboard)
	if menu.ID != "test-menu" {
		t.Errorf("Expected ID 'test-menu', got '%s'", menu.ID)
	}
	if menu.Type != MenuTypeInlineKeyboard {
		t.Errorf("Expected type InlineKeyboard, got %v", menu.Type)
	}
}

func TestMenuWithPlatform(t *testing.T) {
	menu := NewMenuForPlatform("test-menu", MenuTypeInlineKeyboard, core.PlatformTelegram)
	if menu.Platform != core.PlatformTelegram {
		t.Errorf("Expected platform Telegram, got %v", menu.Platform)
	}
}

func TestMenuBuilder(t *testing.T) {
	menu := NewBuilder("test-menu", MenuTypeInlineKeyboard).
		WithPlatform(core.PlatformTelegram).
		WithTitle("Test Menu").
		AddRow(
			NewItem("btn1", "Button 1"),
			NewItem("btn2", "Button 2"),
		).
		AddRow(
			NewItem("btn3", "Button 3"),
		).
		Build()

	if err := menu.Validate(); err != nil {
		t.Errorf("Menu validation failed: %v", err)
	}

	if menu.RowCount() != 2 {
		t.Errorf("Expected 2 rows, got %d", menu.RowCount())
	}

	if menu.ItemCount() != 3 {
		t.Errorf("Expected 3 items, got %d", menu.ItemCount())
	}
}

func TestMenuBuilderWithCallbacks(t *testing.T) {
	menu := NewBuilder("confirm-menu", MenuTypeInlineKeyboard).
		WithPlatform(core.PlatformTelegram).
		AddRow(
			CallbackItem("yes", "✓ Yes", "true"),
			CallbackItem("no", "✗ No", "false"),
		).
		Build()

	if menu.RowCount() != 1 {
		t.Errorf("Expected 1 row, got %d", menu.RowCount())
	}

	if len(menu.Items[0]) != 2 {
		t.Errorf("Expected 2 items in first row, got %d", len(menu.Items[0]))
	}

	if menu.Items[0][0].Action != "callback" {
		t.Errorf("Expected action 'callback', got '%s'", menu.Items[0][0].Action)
	}
}

func TestMenuItemBuilder(t *testing.T) {
	item := NewItem("test-item", "Test Item").
		WithIcon("🔥").
		WithValue("test-value").
		WithAction("callback").
		WithMetadata("key", "value").
		Build()

	if item.ID != "test-item" {
		t.Errorf("Expected ID 'test-item', got '%s'", item.ID)
	}

	if item.Icon != "🔥" {
		t.Errorf("Expected icon '🔥', got '%s'", item.Icon)
	}

	if item.Value != "test-value" {
		t.Errorf("Expected value 'test-value', got '%s'", item.Value)
	}

	if item.Action != "callback" {
		t.Errorf("Expected action 'callback', got '%s'", item.Action)
	}

	if item.Meta["key"] != "value" {
		t.Errorf("Expected metadata key 'value', got %v", item.Meta["key"])
	}
}

func TestConfirmMenu(t *testing.T) {
	menu := NewConfirmMenu("confirm", MenuTypeInlineKeyboard, core.PlatformTelegram, "Are you sure?")

	if err := menu.Validate(); err != nil {
		t.Errorf("Confirm menu validation failed: %v", err)
	}

	if menu.ItemCount() != 2 {
		t.Errorf("Expected 2 items, got %d", menu.ItemCount())
	}

	if menu.Meta["message"] != "Are you sure?" {
		t.Errorf("Expected message metadata 'Are you sure?', got %v", menu.Meta["message"])
	}
}

func TestActionMenu(t *testing.T) {
	actions := map[string]string{
		"action1": "Do Action 1",
		"action2": "Do Action 2",
		"action3": "Do Action 3",
	}

	menu := NewActionMenu("actions", MenuTypeInlineKeyboard, core.PlatformTelegram, actions)

	if err := menu.Validate(); err != nil {
		t.Errorf("Action menu validation failed: %v", err)
	}

	if menu.ItemCount() != 3 {
		t.Errorf("Expected 3 items, got %d", menu.ItemCount())
	}
}

func TestPaginationMenu(t *testing.T) {
	menu := NewPaginationMenu("pages", MenuTypeInlineKeyboard, core.PlatformTelegram, 2, 5)

	if err := menu.Validate(); err != nil {
		t.Errorf("Pagination menu validation failed: %v", err)
	}

	// Should have "Previous" button (not on first page)
	// Should have "Next" button (not on last page)
	if menu.ItemCount() < 1 {
		t.Errorf("Expected at least 1 pagination button, got %d", menu.ItemCount())
	}

	if menu.Meta["current_page"] != 2 {
		t.Errorf("Expected current_page 2, got %v", menu.Meta["current_page"])
	}

	if menu.Meta["total_pages"] != 5 {
		t.Errorf("Expected total_pages 5, got %v", menu.Meta["total_pages"])
	}
}

func TestGridMenu(t *testing.T) {
	items := []string{"A", "B", "C", "D", "E"}
	menu := NewGridMenu("grid", MenuTypeInlineKeyboard, core.PlatformTelegram, items, 2)

	if err := menu.Validate(); err != nil {
		t.Errorf("Grid menu validation failed: %v", err)
	}

	// With 5 items and 2 columns, we should have 3 rows
	if menu.RowCount() != 3 {
		t.Errorf("Expected 3 rows, got %d", menu.RowCount())
	}

	// First two rows should have 2 items each
	if len(menu.Items[0]) != 2 {
		t.Errorf("Expected 2 items in first row, got %d", len(menu.Items[0]))
	}

	if len(menu.Items[1]) != 2 {
		t.Errorf("Expected 2 items in second row, got %d", len(menu.Items[1]))
	}

	// Last row should have 1 item
	if len(menu.Items[2]) != 1 {
		t.Errorf("Expected 1 item in last row, got %d", len(menu.Items[2]))
	}
}

func TestMenuClone(t *testing.T) {
	original := NewBuilder("original", MenuTypeInlineKeyboard).
		WithPlatform(core.PlatformTelegram).
		WithTitle("Original Menu").
		WithOneTime(true).
		AddRow(NewItem("btn1", "Button 1")).
		WithMetadata("key", "value").
		Build()

	cloned := original.Clone()

	// Modify original
	original.Title = "Modified"
	original.OneTime = false

	// Check clone is independent
	if cloned.Title != "Original Menu" {
		t.Errorf("Clone should have original title, got '%s'", cloned.Title)
	}

	if !cloned.OneTime {
		t.Errorf("Clone should have original OneTime value")
	}

	if cloned.Meta["key"] != "value" {
		t.Errorf("Clone should have copied metadata")
	}
}

func TestMenuCapability(t *testing.T) {
	caps := &MenuCapability{
		InlineKeyboard: true,
		ReplyKeyboard:  true,
		ChatMenu:       true,
		QuickActions:   false,
		CommandMenu:    true,
	}

	tests := []struct {
		name     string
		menuType MenuType
		supports bool
	}{
		{"InlineKeyboard", MenuTypeInlineKeyboard, true},
		{"ReplyKeyboard", MenuTypeReplyKeyboard, true},
		{"ChatMenu", MenuTypeChatMenu, true},
		{"QuickActions", MenuTypeQuickActions, false},
		{"CommandMenu", MenuTypeCommandMenu, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := caps.SupportsMenuType(tt.menuType); got != tt.supports {
				t.Errorf("MenuCapability.SupportsMenuType() = %v, want %v", got, tt.supports)
			}
		})
	}
}

func TestGetPlatformMenuCapabilities(t *testing.T) {
	tests := []struct {
		name           string
		platform       core.Platform
		inlineKeyboard bool
		replyKeyboard  bool
		chatMenu       bool
		quickActions   bool
	}{
		{"Telegram", core.PlatformTelegram, true, true, true, false},
		{"Lark", core.PlatformLark, true, false, false, true},
		{"Feishu", core.PlatformFeishu, true, false, false, true},
		{"Discord", core.PlatformDiscord, true, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := GetPlatformMenuCapabilities(tt.platform)

			if caps.InlineKeyboard != tt.inlineKeyboard {
				t.Errorf("InlineKeyboard = %v, want %v", caps.InlineKeyboard, tt.inlineKeyboard)
			}

			if caps.ReplyKeyboard != tt.replyKeyboard {
				t.Errorf("ReplyKeyboard = %v, want %v", caps.ReplyKeyboard, tt.replyKeyboard)
			}

			if caps.ChatMenu != tt.chatMenu {
				t.Errorf("ChatMenu = %v, want %v", caps.ChatMenu, tt.chatMenu)
			}

			if caps.QuickActions != tt.quickActions {
				t.Errorf("QuickActions = %v, want %v", caps.QuickActions, tt.quickActions)
			}
		})
	}
}

func TestGetRecommendedMenuType(t *testing.T) {
	caps := &MenuCapability{
		InlineKeyboard: true,
		ReplyKeyboard:  false,
		ChatMenu:       false,
		QuickActions:   false,
		CommandMenu:    true,
	}

	tests := []struct {
		name       string
		preference MenuType
		expected   MenuType
	}{
		{"Auto", MenuTypeAuto, MenuTypeInlineKeyboard},
		{"InlineKeyboard", MenuTypeInlineKeyboard, MenuTypeInlineKeyboard},
		{"Unsupported", MenuTypeReplyKeyboard, MenuTypeInlineKeyboard},
		{"CommandMenu", MenuTypeCommandMenu, MenuTypeCommandMenu},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := caps.GetRecommendedMenuType(tt.preference); got != tt.expected {
				t.Errorf("GetRecommendedMenuType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMenuItemWithSubItems(t *testing.T) {
	item := NewItem("parent", "Parent Item").
		WithSubItems(
			NewItem("child1", "Child 1"),
			NewItem("child2", "Child 2"),
		).
		Build()

	if !item.HasSubItems() {
		t.Errorf("Expected item to have sub-items")
	}

	if len(item.SubItems) != 2 {
		t.Errorf("Expected 2 sub-items, got %d", len(item.SubItems))
	}

	if item.SubItems[0].Label != "Child 1" {
		t.Errorf("Expected first sub-item label 'Child 1', got '%s'", item.SubItems[0].Label)
	}
}

func TestMenuContext(t *testing.T) {
	ctx := NewMenuContext("chat-123", core.PlatformTelegram)

	if ctx.ChatID != "chat-123" {
		t.Errorf("Expected ChatID 'chat-123', got '%s'", ctx.ChatID)
	}

	if ctx.Platform != core.PlatformTelegram {
		t.Errorf("Expected platform Telegram, got %v", ctx.Platform)
	}

	ctx.MessageID = "msg-456"
	ctx.UserID = "user-789"

	ctx.Metadata["test"] = "value"

	if ctx.MessageID != "msg-456" {
		t.Errorf("Expected MessageID 'msg-456', got '%s'", ctx.MessageID)
	}

	if ctx.Metadata["test"] != "value" {
		t.Errorf("Expected metadata test 'value', got %v", ctx.Metadata["test"])
	}
}

func TestMenuAction(t *testing.T) {
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
}

func TestMenuResult(t *testing.T) {
	result := NewMenuResult(true)

	if !result.Success {
		t.Errorf("Expected Success true, got %v", result.Success)
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

func TestNewErrorMenuResult(t *testing.T) {
	err := core.NewBotError(core.ErrPlatformError, "test error", false)
	result := NewErrorMenuResult(err)

	if result.Success {
		t.Errorf("Expected Success false, got %v", result.Success)
	}

	if result.Error == "" {
		t.Errorf("Expected error message, got empty string")
	}
}

func TestCommandMenu(t *testing.T) {
	commands := map[string]string{
		"/start":    "Start the bot",
		"/help":     "Show help",
		"/settings": "Configure settings",
	}

	menu := NewCommandMenu("cmds", core.PlatformTelegram, commands)

	if menu.Type != MenuTypeCommandMenu {
		t.Errorf("Expected type CommandMenu, got %v", menu.Type)
	}

	if menu.ItemCount() != 3 {
		t.Errorf("Expected 3 items, got %d", menu.ItemCount())
	}

	// Check that items have the correct action type
	for _, row := range menu.Items {
		for _, item := range row {
			if item.Action != "command" {
				t.Errorf("Expected action 'command', got '%s'", item.Action)
			}
		}
	}
}

func TestQuickActionMenu(t *testing.T) {
	actions := map[string]string{
		"quick1": "Quick Action 1",
		"quick2": "Quick Action 2",
	}

	menu := NewQuickActionMenu("quick", core.PlatformLark, actions)

	if menu.Type != MenuTypeQuickActions {
		t.Errorf("Expected type QuickActions, got %v", menu.Type)
	}

	if menu.ItemCount() != 2 {
		t.Errorf("Expected 2 items, got %d", menu.ItemCount())
	}
}

func TestNavigationMenu(t *testing.T) {
	options := []string{"Home", "Profile", "Settings", "Help"}
	menu := NewNavigationMenu("nav", MenuTypeInlineKeyboard, core.PlatformTelegram, options)

	if err := menu.Validate(); err != nil {
		t.Errorf("Navigation menu validation failed: %v", err)
	}

	if menu.ItemCount() != 4 {
		t.Errorf("Expected 4 items, got %d", menu.ItemCount())
	}
}

func TestURLItem(t *testing.T) {
	item := URLItem("link", "Open Link", "https://example.com").Build()

	if item.URL != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got '%s'", item.URL)
	}

	if item.Action != "url" {
		t.Errorf("Expected action 'url', got '%s'", item.Action)
	}
}

func TestToggleItem(t *testing.T) {
	item := ToggleItem("toggle", "Enable Feature", true).Build()

	if item.Value != "true" {
		t.Errorf("Expected value 'true', got '%s'", item.Value)
	}

	if item.Action != "toggle" {
		t.Errorf("Expected action 'toggle', got '%s'", item.Action)
	}

	if item.Meta["checked"] != true {
		t.Errorf("Expected metadata checked true, got %v", item.Meta["checked"])
	}
}

func TestMenuItemWithDisabled(t *testing.T) {
	item := NewItem("disabled-btn", "Disabled Button").
		WithDisabled(true).
		Build()

	if !item.Disabled {
		t.Errorf("Expected Disabled true, got %v", item.Disabled)
	}
}
