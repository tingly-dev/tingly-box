package menu

import (
	"context"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Adapter is the interface for platform-specific menu adapters
//
// Each platform implements this interface to convert the generic Menu structure
// into platform-specific formats and handle menu operations.
type Adapter interface {
	// Supports checks if this adapter supports the given platform
	Supports(platform core.Platform) bool

	// ConvertMenu converts a generic Menu to platform-specific format
	// Returns platform-specific data (e.g., InlineKeyboardMarkup for Telegram)
	ConvertMenu(menu *Menu) (interface{}, error)

	// ShowMenu displays a menu in the specified context
	ShowMenu(ctx context.Context, bot core.Bot, menuCtx *MenuContext, menu *Menu) (*MenuResult, error)

	// HideMenu removes a menu from the chat
	HideMenu(ctx context.Context, bot core.Bot, menuCtx *MenuContext, menuID string) error

	// UpdateMenu updates an existing menu
	UpdateMenu(ctx context.Context, bot core.Bot, menuCtx *MenuContext, menu *Menu) error

	// ParseAction parses a platform-specific callback into a MenuAction
	ParseAction(msg *core.Message) (*MenuAction, error)
}

// BaseAdapter provides common functionality for menu adapters
type BaseAdapter struct {
	platform     core.Platform
	capabilities *MenuCapability
}

// NewBaseAdapter creates a new base adapter
func NewBaseAdapter(platform core.Platform) *BaseAdapter {
	return &BaseAdapter{
		platform:     platform,
		capabilities: GetPlatformMenuCapabilities(platform),
	}
}

// Supports checks if this adapter supports the given platform
func (a *BaseAdapter) Supports(platform core.Platform) bool {
	return a.platform == platform
}

// GetCapabilities returns the menu capabilities for this adapter's platform
func (a *BaseAdapter) GetCapabilities() *MenuCapability {
	return a.capabilities
}

// ConvertMenu converts a generic Menu to platform-specific format
// Base implementation should be overridden by platform-specific adapters
func (a *BaseAdapter) ConvertMenu(menu *Menu) (interface{}, error) {
	return nil, ErrNotSupported
}

// ShowMenu displays a menu (base implementation)
func (a *BaseAdapter) ShowMenu(ctx context.Context, bot core.Bot, menuCtx *MenuContext, menu *Menu) (*MenuResult, error) {
	return nil, ErrNotSupported
}

// HideMenu removes a menu (base implementation)
func (a *BaseAdapter) HideMenu(ctx context.Context, bot core.Bot, menuCtx *MenuContext, menuID string) error {
	return ErrNotSupported
}

// UpdateMenu updates a menu (base implementation)
func (a *BaseAdapter) UpdateMenu(ctx context.Context, bot core.Bot, menuCtx *MenuContext, menu *Menu) error {
	return ErrNotSupported
}

// ParseAction parses a callback (base implementation)
func (a *BaseAdapter) ParseAction(msg *core.Message) (*MenuAction, error) {
	return nil, ErrNotMenuAction
}

// NormalizeMenuType ensures the menu type is supported, falling back if needed
func (a *BaseAdapter) NormalizeMenuType(menuType MenuType) MenuType {
	if menuType == MenuTypeAuto {
		return a.capabilities.GetRecommendedMenuType(menuType)
	}
	if !a.capabilities.SupportsMenuType(menuType) {
		return a.capabilities.GetRecommendedMenuType(MenuTypeAuto)
	}
	return menuType
}

// ValidateMenu validates a menu for this platform
func (a *BaseAdapter) ValidateMenu(menu *Menu) error {
	if err := menu.Validate(); err != nil {
		return err
	}

	// Check if menu type is supported
	normalizedType := a.NormalizeMenuType(menu.Type)
	if normalizedType != menu.Type {
		// Menu type was normalized, update the menu
		menu.Type = normalizedType
	}

	return nil
}

// Registry manages menu adapters for different platforms
type Registry struct {
	adapters map[core.Platform]Adapter
}

// NewRegistry creates a new menu adapter registry
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[core.Platform]Adapter),
	}
}

// Register registers an adapter for a platform
func (r *Registry) Register(adapter Adapter) {
	// Get supported platforms from the adapter
	// For simplicity, we'll extract it from the adapter's type
	// In production, adapters would expose their supported platforms explicitly
	if base, ok := adapter.(*BaseAdapter); ok {
		r.adapters[base.platform] = adapter
	}
}

// RegisterForPlatform registers an adapter for a specific platform
func (r *Registry) RegisterForPlatform(platform core.Platform, adapter Adapter) {
	r.adapters[platform] = adapter
}

// Get returns an adapter for the given platform, or nil if not found
func (r *Registry) Get(platform core.Platform) Adapter {
	return r.adapters[platform]
}

// GetOrDefault returns an adapter for the platform, or a default adapter if not found
func (r *Registry) GetOrDefault(platform core.Platform) Adapter {
	if adapter := r.Get(platform); adapter != nil {
		return adapter
	}
	// Return a default adapter that supports basic inline keyboard
	return NewDefaultAdapter(platform)
}

// Has returns true if an adapter is registered for the platform
func (r *Registry) Has(platform core.Platform) bool {
	_, ok := r.adapters[platform]
	return ok
}

// DefaultAdapter provides a basic implementation for unsupported platforms
type DefaultAdapter struct {
	*BaseAdapter
}

// NewDefaultAdapter creates a new default adapter
func NewDefaultAdapter(platform core.Platform) *DefaultAdapter {
	return &DefaultAdapter{
		BaseAdapter: NewBaseAdapter(platform),
	}
}

// ConvertMenu converts a menu to a simple text-based format
func (a *DefaultAdapter) ConvertMenu(menu *Menu) (interface{}, error) {
	// For unsupported platforms, return a simple text representation
	text := menu.Title
	for _, row := range menu.Items {
		for _, item := range row {
			if item.Icon != "" {
				text += "\n" + item.Icon + " " + item.Label
			} else {
				text += "\n" + item.Label
			}
		}
	}
	return map[string]interface{}{
		"text": text,
		"type": "text_menu",
	}, nil
}

// ShowMenu displays the menu as a text message
func (a *DefaultAdapter) ShowMenu(ctx context.Context, bot core.Bot, menuCtx *MenuContext, menu *Menu) (*MenuResult, error) {
	// Convert to text and send as message
	data, err := a.ConvertMenu(menu)
	if err != nil {
		return nil, err
	}

	textMap, ok := data.(map[string]interface{})
	if !ok {
		return nil, ErrConversionFailed
	}

	text, _ := textMap["text"].(string)

	result, err := bot.SendText(ctx, menuCtx.ChatID, text)
	if err != nil {
		return NewErrorMenuResult(err), nil
	}

	return &MenuResult{
		Success:   true,
		MessageID: result.MessageID,
		MenuID:    menu.ID,
	}, nil
}

// HideMenu is a no-op for default adapter
func (a *DefaultAdapter) HideMenu(ctx context.Context, bot core.Bot, menuCtx *MenuContext, menuID string) error {
	// Cannot hide text messages
	return ErrNotSupported
}

// UpdateMenu sends a new message with updated menu
func (a *DefaultAdapter) UpdateMenu(ctx context.Context, bot core.Bot, menuCtx *MenuContext, menu *Menu) error {
	_, err := a.ShowMenu(ctx, bot, menuCtx, menu)
	return err
}

// ParseAction parses text-based menu selections
func (a *DefaultAdapter) ParseAction(msg *core.Message) (*MenuAction, error) {
	// Try to parse the text content as a menu selection
	if textContent, ok := msg.Content.(*core.TextContent); ok {
		// Check if the text matches a menu item pattern
		// This is a simple implementation
		return &MenuAction{
			Value:     textContent.Text,
			UserID:    msg.Sender.ID,
			ChatID:    msg.Recipient.ID,
			MessageID: msg.ID,
		}, nil
	}
	return nil, ErrNotMenuAction
}

// Menu errors
var (
	ErrConversionFailed = &MenuError{Code: "CONVERSION_FAILED", Message: "failed to convert menu"}
	ErrNotSupported     = &MenuError{Code: "NOT_SUPPORTED", Message: "menu type not supported"}
	ErrNotMenuAction    = &MenuError{Code: "NOT_MENU_ACTION", Message: "message is not a menu action"}
	ErrInvalidContext   = &MenuError{Code: "INVALID_CONTEXT", Message: "invalid menu context"}
	ErrMenuNotFound     = &MenuError{Code: "MENU_NOT_FOUND", Message: "menu not found"}
)

// MenuError represents a menu-specific error
type MenuError struct {
	Code    string
	Message string
	Cause   error
}

// Error returns the error message
func (e *MenuError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// Unwrap returns the underlying error
func (e *MenuError) Unwrap() error {
	return e.Cause
}
