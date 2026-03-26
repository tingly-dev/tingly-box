// Package menu provides a cross-platform menu system for imbot
//
// Menu represents a user interface element that can be displayed in different
// locations depending on platform capabilities:
//   - InlineKeyboard: Buttons attached to a message
//   - ReplyKeyboard: Persistent keyboard above input field (Telegram)
//   - ChatMenu: Menu button in chat interface (Telegram)
//   - CommandMenu: Slash commands
//   - QuickActions: Quick action buttons (Lark/Feishu)
package menu

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// menuType defines where and how a menu is displayed (internal use only)
// Platform adapters use this to determine the best rendering approach.
type menuType string

const (
	// menuTypeInlineKeyboard displays buttons attached to a specific message
	// Supported: Telegram (inline keyboard), Lark/Feishu (card buttons), Discord (components)
	menuTypeInlineKeyboard menuType = "inline_keyboard"

	// menuTypeReplyKeyboard displays a persistent keyboard above the input field
	// Supported: Telegram (reply keyboard), some mobile platforms
	menuTypeReplyKeyboard menuType = "reply_keyboard"

	// menuTypeChatMenu displays in a menu button in the chat interface
	// Supported: Telegram (menu button), some platforms with similar UI
	menuTypeChatMenu menuType = "chat_menu"

	// menuTypeQuickActions displays as quick action buttons
	// Supported: Lark/Feishu (quick actions in input area)
	menuTypeQuickActions menuType = "quick_actions"

	// menuTypeCommandMenu displays as slash commands
	// Supported: Most platforms (/, /command, etc.)
	menuTypeCommandMenu menuType = "command_menu"

	// menuTypeAuto lets the platform choose the best available menu type (default)
	menuTypeAuto menuType = "auto"
)

// IsInlineKeyboard returns true if menu type is inline keyboard
func (m *Menu) IsInlineKeyboard() bool {
	return m.Type == menuTypeInlineKeyboard
}

// IsReplyKeyboard returns true if menu type is reply keyboard
func (m *Menu) IsReplyKeyboard() bool {
	return m.Type == menuTypeReplyKeyboard
}

// IsChatMenu returns true if menu type is chat menu
func (m *Menu) IsChatMenu() bool {
	return m.Type == menuTypeChatMenu
}

// IsQuickActions returns true if menu type is quick actions
func (m *Menu) IsQuickActions() bool {
	return m.Type == menuTypeQuickActions
}

// IsCommandMenu returns true if menu type is command menu
func (m *Menu) IsCommandMenu() bool {
	return m.Type == menuTypeCommandMenu
}

// IsAuto returns true if menu type is auto (platform chooses)
func (m *Menu) IsAuto() bool {
	return m.Type == menuTypeAuto
}

// SetInlineKeyboard sets the menu type to inline keyboard (internal use by adapters)
func (m *Menu) SetInlineKeyboard() {
	m.Type = menuTypeInlineKeyboard
}

// SetReplyKeyboard sets the menu type to reply keyboard (internal use by adapters)
func (m *Menu) SetReplyKeyboard() {
	m.Type = menuTypeReplyKeyboard
}

// SetChatMenu sets the menu type to chat menu (internal use by adapters)
func (m *Menu) SetChatMenu() {
	m.Type = menuTypeChatMenu
}

// SetQuickActions sets the menu type to quick actions (internal use by adapters)
func (m *Menu) SetQuickActions() {
	m.Type = menuTypeQuickActions
}

// SetCommandMenu sets the menu type to command menu (internal use by adapters)
func (m *Menu) SetCommandMenu() {
	m.Type = menuTypeCommandMenu
}

// SetAuto sets the menu type to auto (internal use by adapters)
func (m *Menu) SetAuto() {
	m.Type = menuTypeAuto
}

// isValid checks if the menu type is valid
func (m menuType) isValid() bool {
	switch m {
	case menuTypeInlineKeyboard, menuTypeReplyKeyboard, menuTypeChatMenu,
		menuTypeQuickActions, menuTypeCommandMenu, menuTypeAuto:
		return true
	default:
		return false
	}
}

// String returns the string representation of the menu type
func (m menuType) String() string {
	return string(m)
}

// MenuItem represents a single item in a menu
type MenuItem struct {
	ID       string                 `json:"id"`                 // Unique identifier for this item
	Label    string                 `json:"label"`              // Display label
	Icon     string                 `json:"icon,omitempty"`     // Optional icon/emoji
	Value    string                 `json:"value,omitempty"`    // Associated value
	Action   string                 `json:"action,omitempty"`   // Action to perform (callback, url, etc.)
	URL      string                 `json:"url,omitempty"`      // URL for link buttons
	Meta     map[string]interface{} `json:"meta,omitempty"`     // Additional metadata
	Disabled bool                   `json:"disabled,omitempty"` // Whether the item is disabled
	SubItems []*MenuItem            `json:"subItems,omitempty"` // Nested items for sub-menus
}

// HasSubItems returns true if this item has sub-items
func (i *MenuItem) HasSubItems() bool {
	return len(i.SubItems) > 0
}

// Menu represents a complete menu structure
type Menu struct {
	ID        string                 `json:"id"`                  // Unique menu identifier
	Type      menuType               `json:"type"`                // Menu display type (internal, managed by adapters)
	Platform  core.Platform          `json:"platform"`            // Target platform
	Title     string                 `json:"title,omitempty"`     // Menu title (for some platforms)
	Items     [][]MenuItem           `json:"items"`               // Menu items arranged in rows
	Meta      map[string]interface{} `json:"meta,omitempty"`      // Additional metadata
	OneTime   bool                   `json:"oneTime,omitempty"`   // For reply keyboard: hide after one use
	Resizable bool                   `json:"resizable,omitempty"` // For some platforms: allow user to resize
}

// NewMenu creates a new menu with the given ID
// The menu type is automatically determined by the platform adapter
func NewMenu(id string) *Menu {
	return &Menu{
		ID:    id,
		Type:  menuTypeAuto,
		Items: make([][]MenuItem, 0),
		Meta:  make(map[string]interface{}),
	}
}

// NewMenuForPlatform creates a new menu optimized for a specific platform
// The menu type is automatically determined by the platform adapter
func NewMenuForPlatform(id string, platform core.Platform) *Menu {
	menu := NewMenu(id)
	menu.Platform = platform
	return menu
}

// AddRow adds a row of menu items
func (m *Menu) AddRow(items ...MenuItem) *Menu {
	row := make([]MenuItem, len(items))
	copy(row, items)
	m.Items = append(m.Items, row)
	return m
}

// AddItem adds a single item to the last row (creates row if needed)
func (m *Menu) AddItem(item MenuItem) *Menu {
	if len(m.Items) == 0 {
		m.Items = append(m.Items, []MenuItem{item})
	} else {
		lastRow := m.Items[len(m.Items)-1]
		m.Items[len(m.Items)-1] = append(lastRow, item)
	}
	return m
}

// RowCount returns the number of rows in the menu
func (m *Menu) RowCount() int {
	return len(m.Items)
}

// ItemCount returns the total number of items in the menu
func (m *Menu) ItemCount() int {
	count := 0
	for _, row := range m.Items {
		count += len(row)
	}
	return count
}

// Validate validates the menu structure
func (m *Menu) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("menu ID cannot be empty")
	}
	if !m.Type.isValid() {
		return fmt.Errorf("invalid menu type: %s", m.Type)
	}
	if len(m.Items) == 0 {
		return fmt.Errorf("menu must have at least one item")
	}
	return nil
}

// Clone creates a deep copy of the menu
func (m *Menu) Clone() *Menu {
	clone := &Menu{
		ID:        m.ID,
		Type:      m.Type,
		Platform:  m.Platform,
		Title:     m.Title,
		OneTime:   m.OneTime,
		Resizable: m.Resizable,
		Items:     make([][]MenuItem, len(m.Items)),
		Meta:      make(map[string]interface{}),
	}

	// Copy items
	for i, row := range m.Items {
		clone.Items[i] = make([]MenuItem, len(row))
		for j, item := range row {
			clone.Items[i][j] = item
		}
	}

	// Copy metadata
	for k, v := range m.Meta {
		clone.Meta[k] = v
	}

	return clone
}

// menuCapability describes which menu types a platform supports (internal use only)
type menuCapability struct {
	InlineKeyboard bool `json:"inlineKeyboard"`
	ReplyKeyboard  bool `json:"replyKeyboard"`
	ChatMenu       bool `json:"chatMenu"`
	QuickActions   bool `json:"quickActions"`
	CommandMenu    bool `json:"commandMenu"`
}

// supportsMenuType checks if the capability supports a given menu type
func (c *menuCapability) supportsMenuType(menuType menuType) bool {
	switch menuType {
	case menuTypeInlineKeyboard:
		return c.InlineKeyboard
	case menuTypeReplyKeyboard:
		return c.ReplyKeyboard
	case menuTypeChatMenu:
		return c.ChatMenu
	case menuTypeQuickActions:
		return c.QuickActions
	case menuTypeCommandMenu:
		return c.CommandMenu
	case menuTypeAuto:
		return c.InlineKeyboard || c.ReplyKeyboard || c.ChatMenu
	default:
		return false
	}
}

// getRecommendedMenuType returns the best menu type for a platform
// given a preference and platform capabilities
func (c *menuCapability) getRecommendedMenuType(preference menuType) menuType {
	if preference != menuTypeAuto && c.supportsMenuType(preference) {
		return preference
	}

	// Default preference order
	order := []menuType{
		menuTypeInlineKeyboard,
		menuTypeChatMenu,
		menuTypeQuickActions,
		menuTypeReplyKeyboard,
		menuTypeCommandMenu,
	}

	for _, mt := range order {
		if c.supportsMenuType(mt) {
			return mt
		}
	}

	return menuTypeInlineKeyboard // Fallback
}

// getPlatformMenuCapabilities is the internal implementation (for adapters)
func getPlatformMenuCapabilities(platform core.Platform) *menuCapability {
	capabilities := map[core.Platform]*menuCapability{
		core.PlatformTelegram: {
			InlineKeyboard: true,
			ReplyKeyboard:  true,
			ChatMenu:       true, // Via bot menu button
			QuickActions:   false,
			CommandMenu:    true, // Bot commands with /
		},
		core.PlatformLark: {
			InlineKeyboard: true, // Via card buttons
			ReplyKeyboard:  false,
			ChatMenu:       false,
			QuickActions:   true, // Quick actions in input area
			CommandMenu:    true, // Slash commands
		},
		core.PlatformFeishu: {
			InlineKeyboard: true, // Via card buttons
			ReplyKeyboard:  false,
			ChatMenu:       false,
			QuickActions:   true, // Quick actions in input area
			CommandMenu:    true, // Slash commands
		},
		core.PlatformDiscord: {
			InlineKeyboard: true, // Via components
			ReplyKeyboard:  false,
			ChatMenu:       false,
			QuickActions:   false,
			CommandMenu:    true, // Slash commands
		},
		core.PlatformSlack: {
			InlineKeyboard: true, // Via Block Kit
			ReplyKeyboard:  false,
			ChatMenu:       false,
			QuickActions:   true, // Message shortcuts
			CommandMenu:    true, // Slash commands
		},
	}

	if caps, ok := capabilities[platform]; ok {
		return caps
	}

	// Default minimal capabilities
	return &menuCapability{
		InlineKeyboard: false,
		ReplyKeyboard:  false,
		ChatMenu:       false,
		QuickActions:   false,
		CommandMenu:    false,
	}
}

// MenuContext provides context for menu rendering
type MenuContext struct {
	ChatID    string                 `json:"chatId"`    // Target chat ID
	MessageID string                 `json:"messageId"` // Associated message ID (for inline keyboards)
	UserID    string                 `json:"userId"`    // User ID (for user-specific menus)
	Platform  core.Platform          `json:"platform"`  // Platform identifier
	Metadata  map[string]interface{} `json:"metadata"`  // Additional context
}

// NewMenuContext creates a new menu context
func NewMenuContext(chatID string, platform core.Platform) *MenuContext {
	return &MenuContext{
		ChatID:   chatID,
		Platform: platform,
		Metadata: make(map[string]interface{}),
	}
}

// MenuResult represents the result of a menu operation
type MenuResult struct {
	Success   bool                   `json:"success"`
	MessageID string                 `json:"messageId,omitempty"` // For inline keyboards
	MenuID    string                 `json:"menuId,omitempty"`    // Menu identifier
	RawData   map[string]interface{} `json:"rawData,omitempty"`   // Platform-specific data
	Error     string                 `json:"error,omitempty"`     // Error message if failed
}

// NewMenuResult creates a new menu result
func NewMenuResult(success bool) *MenuResult {
	return &MenuResult{
		Success: success,
		RawData: make(map[string]interface{}),
	}
}

// NewErrorMenuResult creates a new error menu result
func NewErrorMenuResult(err error) *MenuResult {
	return &MenuResult{
		Success: false,
		Error:   err.Error(),
	}
}

// MenuAction represents a user action on a menu item
type MenuAction struct {
	MenuID    string                 `json:"menuId"`    // Menu identifier
	ItemID    string                 `json:"itemId"`    // Item identifier
	Value     string                 `json:"value"`     // Item value
	Action    string                 `json:"action"`    // Action type
	UserID    string                 `json:"userId"`    // User who performed the action
	ChatID    string                 `json:"chatId"`    // Chat where action occurred
	MessageID string                 `json:"messageId"` // Message ID (for inline menus)
	Timestamp int64                  `json:"timestamp"` // When the action occurred
	Meta      map[string]interface{} `json:"meta"`      // Additional metadata
}

// NewMenuAction creates a new menu action
func NewMenuAction(menuID, itemID, value, userID, chatID string) *MenuAction {
	return &MenuAction{
		MenuID: menuID,
		ItemID: itemID,
		Value:  value,
		UserID: userID,
		ChatID: chatID,
		Meta:   make(map[string]interface{}),
	}
}
