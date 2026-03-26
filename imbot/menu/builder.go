package menu

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// Builder provides a fluent API for constructing menus
type Builder struct {
	menu *Menu
}

// NewBuilder creates a new menu builder
// The menu type is automatically determined by the platform adapter
func NewBuilder(id string) *Builder {
	return &Builder{
		menu: NewMenu(id),
	}
}

// NewBuilderForPlatform creates a new menu builder optimized for a specific platform
// The menu type is automatically determined by the platform adapter
func NewBuilderForPlatform(id string, platform core.Platform) *Builder {
	return &Builder{
		menu: NewMenuForPlatform(id, platform),
	}
}

// WithPlatform sets the target platform
func (b *Builder) WithPlatform(platform core.Platform) *Builder {
	b.menu.Platform = platform
	return b
}

// WithType sets a specific menu type hint for the platform adapter (optional)
// Note: This is only a hint. The platform adapter may choose a different type if needed.
// Deprecated: Menu type selection is now handled automatically by platform adapters.
func (b *Builder) WithType(menuType menuType) *Builder {
	b.menu.Type = menuType
	return b
}

// WithTitle sets the menu title
func (b *Builder) WithTitle(title string) *Builder {
	b.menu.Title = title
	return b
}

// WithOneTime sets the one-time flag (for reply keyboards)
func (b *Builder) WithOneTime(oneTime bool) *Builder {
	b.menu.OneTime = oneTime
	return b
}

// WithResizable sets the resizable flag
func (b *Builder) WithResizable(resizable bool) *Builder {
	b.menu.Resizable = resizable
	return b
}

// WithMetadata adds metadata to the menu
func (b *Builder) WithMetadata(key string, value interface{}) *Builder {
	if b.menu.Meta == nil {
		b.menu.Meta = make(map[string]interface{})
	}
	b.menu.Meta[key] = value
	return b
}

// AddRow adds a row of menu items
func (b *Builder) AddRow(items ...*ItemBuilder) *Builder {
	row := make([]MenuItem, len(items))
	for i, itemBuilder := range items {
		row[i] = itemBuilder.Build()
	}
	b.menu.Items = append(b.menu.Items, row)
	return b
}

// AddItems adds multiple items as a single row
func (b *Builder) AddItems(items ...*ItemBuilder) *Builder {
	return b.AddRow(items...)
}

// AddItem adds a single item to the last row
func (b *Builder) AddItem(item *ItemBuilder) *Builder {
	itemBuilt := item.Build()
	if len(b.menu.Items) == 0 {
		b.menu.Items = append(b.menu.Items, []MenuItem{itemBuilt})
	} else {
		lastRowIdx := len(b.menu.Items) - 1
		b.menu.Items[lastRowIdx] = append(b.menu.Items[lastRowIdx], itemBuilt)
	}
	return b
}

// AddRowFromRaw adds a row from raw MenuItem structs
func (b *Builder) AddRowFromRaw(items ...MenuItem) *Builder {
	b.menu.Items = append(b.menu.Items, items)
	return b
}

// Build returns the constructed menu
func (b *Builder) Build() *Menu {
	return b.menu
}

// MustBuild returns the menu or panics if invalid
func (b *Builder) MustBuild() *Menu {
	if err := b.menu.Validate(); err != nil {
		panic(err)
	}
	return b.menu
}

// Clone creates a copy of the builder with a new menu ID
func (b *Builder) Clone(newID string) *Builder {
	newMenu := b.menu.Clone()
	newMenu.ID = newID
	return &Builder{menu: newMenu}
}

// ItemBuilder provides a fluent API for constructing menu items
type ItemBuilder struct {
	item MenuItem
}

// NewItem creates a new menu item builder
func NewItem(id, label string) *ItemBuilder {
	return &ItemBuilder{
		item: MenuItem{
			ID:       id,
			Label:    label,
			Meta:     make(map[string]interface{}),
			SubItems: make([]*MenuItem, 0),
		},
	}
}

// CallbackItem creates a callback button item
func CallbackItem(id, label, value string) *ItemBuilder {
	return NewItem(id, label).WithValue(value).WithAction("callback")
}

// URLItem creates a URL button item
func URLItem(id, label, url string) *ItemBuilder {
	return NewItem(id, label).WithURL(url).WithAction("url")
}

// ToggleItem creates a toggle/checkbox item
func ToggleItem(id, label string, checked bool) *ItemBuilder {
	return NewItem(id, label).
		WithValue(fmt.Sprintf("%v", checked)).
		WithAction("toggle").
		WithMetadata("checked", checked)
}

// WithIcon sets the icon/emoji for the item
func (b *ItemBuilder) WithIcon(icon string) *ItemBuilder {
	b.item.Icon = icon
	return b
}

// WithValue sets the value for the item
func (b *ItemBuilder) WithValue(value string) *ItemBuilder {
	b.item.Value = value
	return b
}

// WithAction sets the action type for the item
func (b *ItemBuilder) WithAction(action string) *ItemBuilder {
	b.item.Action = action
	return b
}

// WithURL sets the URL for link buttons
func (b *ItemBuilder) WithURL(url string) *ItemBuilder {
	b.item.URL = url
	return b
}

// WithMetadata adds metadata to the item
func (b *ItemBuilder) WithMetadata(key string, value interface{}) *ItemBuilder {
	if b.item.Meta == nil {
		b.item.Meta = make(map[string]interface{})
	}
	b.item.Meta[key] = value
	return b
}

// WithDisabled sets the disabled state
func (b *ItemBuilder) WithDisabled(disabled bool) *ItemBuilder {
	b.item.Disabled = disabled
	return b
}

// WithSubItems adds sub-items to create a nested menu
func (b *ItemBuilder) WithSubItems(items ...*ItemBuilder) *ItemBuilder {
	subItems := make([]*MenuItem, len(items))
	for i, itemBuilder := range items {
		item := itemBuilder.Build()
		subItems[i] = &item
	}
	b.item.SubItems = subItems
	return b
}

// Build returns the constructed menu item
func (b *ItemBuilder) Build() MenuItem {
	return b.item
}

// Helper functions for common menu patterns

// NewConfirmMenu creates a menu with Yes/No buttons
func NewConfirmMenu(id string, platform core.Platform, message string) *Menu {
	return NewBuilderForPlatform(id, platform).
		AddRow(
			CallbackItem(id+"_yes", "✓ Yes", "true"),
			CallbackItem(id+"_no", "✗ No", "false"),
		).
		WithMetadata("message", message).
		Build()
}

// NewActionMenu creates a menu with action buttons
func NewActionMenu(id string, platform core.Platform, actions map[string]string) *Menu {
	items := make([]*ItemBuilder, 0, len(actions))
	for actionID, label := range actions {
		items = append(items, CallbackItem(id+"_"+actionID, label, actionID))
	}
	return NewBuilderForPlatform(id, platform).
		AddRow(items...).
		Build()
}

// NewPaginationMenu creates a menu with pagination controls
func NewPaginationMenu(id string, platform core.Platform, currentPage, totalPages int) *Menu {
	builder := NewBuilderForPlatform(id, platform)

	// Previous button row
	if currentPage > 1 {
		builder.AddRow(
			CallbackItem(id+"_prev", "⬅ Previous", fmt.Sprintf("%d", currentPage-1)),
		)
	}

	// Page indicator and next button
	if currentPage < totalPages {
		builder.AddRow(
			CallbackItem(id+"_next", "Next ➡", fmt.Sprintf("%d", currentPage+1)),
		)
	}

	return builder.
		WithMetadata("current_page", currentPage).
		WithMetadata("total_pages", totalPages).
		Build()
}

// NewCommandMenu creates a command menu (slash command list)
func NewCommandMenu(id string, platform core.Platform, commands map[string]string) *Menu {
	items := make([]*ItemBuilder, 0, len(commands))
	for cmd, description := range commands {
		items = append(items,
			NewItem(id+"_"+cmd, cmd).
				WithValue(description).
				WithAction("command").
				WithMetadata("description", description),
		)
	}
	return NewBuilderForPlatform(id, platform).
		AddRow(items...).
		Build()
}

// NewQuickActionMenu creates a quick action menu for Lark/Feishu
func NewQuickActionMenu(id string, platform core.Platform, actions map[string]string) *Menu {
	items := make([]*ItemBuilder, 0, len(actions))
	for actionID, label := range actions {
		items = append(items,
			CallbackItem(id+"_"+actionID, label, actionID).
				WithMetadata("quick_action", "true"),
		)
	}
	return NewBuilderForPlatform(id, platform).
		AddRow(items...).
		Build()
}

// NewNavigationMenu creates a navigation menu with common options
func NewNavigationMenu(id string, platform core.Platform, options []string) *Menu {
	items := make([]*ItemBuilder, 0, len(options))
	for i, option := range options {
		items = append(items,
			CallbackItem(fmt.Sprintf("%s_%d", id, i), option, option),
		)
	}
	return NewBuilderForPlatform(id, platform).
		AddRow(items...).
		Build()
}

// NewGridMenu creates a grid-style menu with items arranged in columns
func NewGridMenu(id string, platform core.Platform, items []string, columns int) *Menu {
	builder := NewBuilderForPlatform(id, platform)

	currentRow := make([]*ItemBuilder, 0, columns)
	for i, itemLabel := range items {
		currentRow = append(currentRow,
			CallbackItem(fmt.Sprintf("%s_%d", id, i), itemLabel, itemLabel),
		)

		if len(currentRow) == columns || i == len(items)-1 {
			builder.AddRow(currentRow...)
			currentRow = make([]*ItemBuilder, 0, columns)
		}
	}

	return builder.Build()
}
