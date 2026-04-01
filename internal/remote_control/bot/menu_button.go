package bot

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot"
)

// MenuButtonConfig configures the menu button for different platforms
// The menu button appears near the message input field and provides quick access to commands
//
// Platform support:
// - Telegram: Menu Button (Commands / Web App)
// - Feishu/Lark: Quick Actions / Card Menu
// - Slack: App Shortcut / Slash Commands
type MenuButtonConfig struct {
	// ID identifies this menu configuration
	ID string

	// Type determines what kind of menu this is
	Type MenuButtonType

	// ButtonText is the label shown on the menu button itself
	// e.g., "Menu", "Commands", "Actions"
	ButtonText string

	// Items are the menu items to show when button is tapped
	Items []MenuItemConfig

	// Platforms where this menu is available
	// If empty, shows on all platforms
	Platforms []imbot.Platform

	// Priority determines which config to use if multiple are registered
	// Higher priority wins. If 0, uses default priority.
	Priority int

	// Context filters when this menu should be shown
	Context *MenuContext
}

// MenuButtonType defines the type of menu button
type MenuButtonType string

const (
	// MenuTypeCommands shows a list of bot commands
	MenuTypeCommands MenuButtonType = "commands"

	// MenuTypeWebApp opens a Mini App / Web App
	MenuTypeWebApp MenuButtonType = "webapp"

	// MenuTypeCallbacks shows custom callback buttons
	MenuTypeCallbacks MenuButtonType = "callbacks"

	// MenuTypeDefault uses the platform default menu
	MenuTypeDefault MenuButtonType = "default"
)

// MenuItemConfig defines a single item in the menu
type MenuItemConfig struct {
	// ID is the unique identifier for this item
	ID string

	// Label is the display text
	Label string

	// Description is optional helper text (shown below label on some platforms)
	Description string

	// Value is the callback value or command to execute
	// For commands: "/help"
	// For callbacks: "action:confirm"
	Value string

	// URL opens a URL instead of executing a command (webapp buttons)
	URL string

	// Icon for platforms that support it (Feishu/Lark)
	Icon string

	// Group related items together (optional)
	Group string

	// Hidden skips this item but keeps it in registry
	Hidden bool
}

// MenuContext filters when a menu should be shown
type MenuContext struct {
	// ChatType restricts to specific chat types
	// "direct", "group", "channel", "all"
	ChatType string

	// UserIDs restricts to specific users (for personalization)
	UserIDs []string

	// ChatIDs restricts to specific chats
	ChatIDs []string

	// LanguageCode for localized menus (e.g., "en", "zh", "es")
	LanguageCode string
}

// MenuRegistry manages menu button configurations
type MenuRegistry struct {
	configs    []MenuButtonConfig
	byID       map[string]*MenuButtonConfig
	byPlatform map[imbot.Platform][]MenuButtonConfig
}

// NewMenuRegistry creates a new menu registry
func NewMenuRegistry() *MenuRegistry {
	return &MenuRegistry{
		configs:    make([]MenuButtonConfig, 0),
		byID:       make(map[string]*MenuButtonConfig),
		byPlatform: make(map[imbot.Platform][]MenuButtonConfig),
	}
}

// Register adds a menu configuration to the registry
func (r *MenuRegistry) Register(config MenuButtonConfig) {
	r.byID[config.ID] = &config
	r.configs = append(r.configs, config)

	// Index by platform
	if len(config.Platforms) == 0 {
		// Available on all platforms
		for _, p := range r.allPlatforms() {
			r.byPlatform[p] = append(r.byPlatform[p], config)
		}
	} else {
		for _, p := range config.Platforms {
			r.byPlatform[p] = append(r.byPlatform[p], config)
		}
	}
}

// allPlatforms returns a list of all supported platforms
func (r *MenuRegistry) allPlatforms() []imbot.Platform {
	return []imbot.Platform{
		imbot.PlatformTelegram,
		imbot.PlatformFeishu,
		imbot.PlatformLark,
		imbot.PlatformSlack,
		imbot.PlatformDiscord,
	}
}

// GetForPlatform returns the best menu configuration for a platform
// Uses priority to select the highest priority config that matches the context
func (r *MenuRegistry) GetForPlatform(platform imbot.Platform, context *MenuContext) *MenuButtonConfig {
	configs := r.byPlatform[platform]
	if len(configs) == 0 {
		return nil
	}

	// Filter by context
	var candidates []MenuButtonConfig
	for _, cfg := range configs {
		if r.matchesContext(cfg, context) {
			candidates = append(candidates, cfg)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Return highest priority
	best := candidates[0]
	for _, cfg := range candidates[1:] {
		if cfg.Priority > best.Priority {
			best = cfg
		}
	}

	return &best
}

// matchesContext checks if a config matches the given context
func (r *MenuRegistry) matchesContext(cfg MenuButtonConfig, ctx *MenuContext) bool {
	if ctx == nil || cfg.Context == nil {
		return true
	}

	// Check chat type
	if cfg.Context.ChatType != "" && cfg.Context.ChatType != "all" {
		if ctx.ChatType != "" && cfg.Context.ChatType != ctx.ChatType {
			return false
		}
	}

	// Check user IDs
	if len(cfg.Context.UserIDs) > 0 {
		if ctx == nil || len(ctx.UserIDs) == 0 {
			return false
		}
		found := false
		for _, uid := range cfg.Context.UserIDs {
			for _, ctxUID := range ctx.UserIDs {
				if uid == ctxUID {
					found = true
					break
				}
			}
		}
		if !found {
			return false
		}
	}

	// Check language
	if cfg.Context.LanguageCode != "" {
		if ctx == nil || ctx.LanguageCode != cfg.Context.LanguageCode {
			return false
		}
	}

	return true
}

// BuildForTelegram returns the Telegram Bot API configuration for the menu button
func (r *MenuRegistry) BuildForTelegram(context *MenuContext) map[string]interface{} {
	config := r.GetForPlatform(imbot.PlatformTelegram, context)
	if config == nil {
		// Return default commands menu
		return map[string]interface{}{
			"type": "commands",
		}
	}

	switch config.Type {
	case MenuTypeCommands:
		// Build commands list
		commands := make([]map[string]string, 0)
		for _, item := range config.Items {
			if item.Hidden {
				continue
			}
			commands = append(commands, map[string]string{
				"command":     item.Value,
				"description": item.Description,
			})
		}
		return map[string]interface{}{
			"type":     "commands",
			"commands": commands,
		}

	case MenuTypeWebApp:
		return map[string]interface{}{
			"type": "web_app",
			"text": config.ButtonText,
			"url":  config.Items[0].URL,
		}

	case MenuTypeCallbacks:
		// Telegram doesn't support callback menu buttons directly
		// Fall back to commands
		return map[string]interface{}{
			"type": "commands",
		}

	default:
		return map[string]interface{}{
			"type": "commands",
		}
	}
}

// BuildForFeishu returns the Feishu/Lark configuration for quick actions
func (r *MenuRegistry) BuildForFeishu(context *MenuContext) map[string]interface{} {
	config := r.GetForPlatform(imbot.PlatformFeishu, context)
	if config == nil {
		return nil
	}

	actions := make([]map[string]interface{}, 0)
	for _, item := range config.Items {
		if item.Hidden {
			continue
		}

		action := map[string]interface{}{
			"id":    item.ID,
			"text":  item.Label,
			"value": item.Value,
		}

		if item.Description != "" {
			action["description"] = item.Description
		}
		if item.Icon != "" {
			action["icon"] = item.Icon
		}
		if item.Group != "" {
			action["group"] = item.Group
		}

		actions = append(actions, action)
	}

	return map[string]interface{}{
		"type":    "quick_actions",
		"button":  config.ButtonText,
		"actions": actions,
	}
}

// BuildForSlack returns the Slack configuration for app shortcuts
func (r *MenuRegistry) BuildForSlack(context *MenuContext) map[string]interface{} {
	config := r.GetForPlatform(imbot.PlatformSlack, context)
	if config == nil {
		return nil
	}

	// Slack uses global shortcuts or slash commands
	shortcuts := make([]map[string]interface{}, 0)
	for _, item := range config.Items {
		if item.Hidden {
			continue
		}

		shortcuts = append(shortcuts, map[string]interface{}{
			"name":        item.Label,
			"callback":    item.Value,
			"description": item.Description,
		})
	}

	return map[string]interface{}{
		"type":      "app_shortcuts",
		"shortcuts": shortcuts,
	}
}

// Default menu configurations

// DefaultCommandMenu returns the default command menu configuration
func DefaultCommandMenu() MenuButtonConfig {
	return MenuButtonConfig{
		ID:         "default-commands",
		Type:       MenuTypeCommands,
		ButtonText: "Menu",
		Priority:   0,
		Items: []MenuItemConfig{
			{
				ID:          "help",
				Label:       "Help",
				Description: "Show available commands",
				Value:       "/help",
			},
			{
				ID:          "cd",
				Label:       "Change Directory",
				Description: "Bind to a project directory",
				Value:       "/cd",
			},
			{
				ID:          "status",
				Label:       "Status",
				Description: "Show current session status",
				Value:       "/status",
			},
			{
				ID:          "clear",
				Label:       "Clear",
				Description: "Start a new session",
				Value:       "/clear",
			},
			{
				ID:          "project",
				Label:       "Project",
				Description: "Switch projects",
				Value:       "/project",
			},
		},
	}
}

// DefaultActionMenu returns the default action menu (with callback buttons)
func DefaultActionMenu() MenuButtonConfig {
	return MenuButtonConfig{
		ID:         "default-actions",
		Type:       MenuTypeCallbacks,
		ButtonText: "Actions",
		Priority:   10,
		Platforms:  []imbot.Platform{imbot.PlatformFeishu, imbot.PlatformLark},
		Items: []MenuItemConfig{
			{
				ID:          "clear",
				Label:       "🗑 Clear",
				Description: "Clear context",
				Value:       "action:clear",
			},
			{
				ID:          "bind",
				Label:       "📁 Bind",
				Description: "Bind project",
				Value:       "action:bind",
			},
			{
				ID:          "status",
				Label:       "📊 Status",
				Description: "Show status",
				Value:       "action:status",
			},
		},
	}
}

// Global menu registry
var defaultMenuRegistry = NewMenuRegistry()

// Initialize with defaults
func init() {
	defaultMenuRegistry.Register(DefaultCommandMenu())
	defaultMenuRegistry.Register(DefaultActionMenu())
}

// GetMenuRegistry returns the default menu registry
func GetMenuRegistry() *MenuRegistry {
	return defaultMenuRegistry
}

// RegisterMenu registers a menu in the default registry
func RegisterMenu(config MenuButtonConfig) {
	defaultMenuRegistry.Register(config)
}

// GetMenuForPlatform returns the best menu for a platform
func GetMenuForPlatform(platform imbot.Platform, context *MenuContext) *MenuButtonConfig {
	return defaultMenuRegistry.GetForPlatform(platform, context)
}

// SetupMenuButtonForBot configures the menu button for a bot
// This should be called when the bot starts or when settings change
func SetupMenuButtonForBot(manager *imbot.Manager, uuid string, cmdRegistry *imbot.CommandRegistry) error {
	// Get the bot to check its platform (no platform filter)
	bot := manager.GetBotByUUID(uuid)
	if bot == nil {
		return fmt.Errorf("bot not found: %s", uuid)
	}

	platform := bot.PlatformInfo().ID

	switch platform {
	case imbot.PlatformTelegram:
		return setupTelegramMenuButton(bot, cmdRegistry)
	case imbot.PlatformFeishu, imbot.PlatformLark:
		return setupFeishuMenuButton(bot, cmdRegistry)
	default:
		// Other platforms don't support menu buttons, log and continue
		return nil
	}
}

// setupTelegramMenuButton configures the Telegram bot menu button
func setupTelegramMenuButton(bot imbot.Bot, cmdRegistry *imbot.CommandRegistry) error {
	// Cast to TelegramBot interface to access Telegram-specific methods
	tgBot, ok := imbot.AsTelegramBot(bot)
	if !ok {
		return fmt.Errorf("bot is not a Telegram bot: %T", bot)
	}

	// Build command list from the imbot command registry
	telegramCommands := cmdRegistry.BuildPlatformCommands(imbot.PlatformTelegram)

	if err := tgBot.SetCommandList(telegramCommands); err != nil {
		return fmt.Errorf("failed to set bot commands: %w", err)
	}

	// Then, set the menu button configuration
	config := GetMenuRegistry().BuildForTelegram(nil)
	if err := tgBot.SetMenuButton(config); err != nil {
		return fmt.Errorf("failed to set menu button: %w", err)
	}

	return nil
}

// setupFeishuMenuButton configures the Feishu/Lark quick actions
func setupFeishuMenuButton(bot imbot.Bot, cmdRegistry *imbot.CommandRegistry) error {
	// Cast to FeishuBot interface to access Feishu-specific methods
	fsBot, ok := imbot.AsFeishuBot(bot)
	if !ok {
		return fmt.Errorf("bot is not a Feishu/Lark bot: %T", bot)
	}

	// Build quick actions from the imbot command registry
	commands := cmdRegistry.ForPlatform(imbot.PlatformFeishu)

	quickActions := make([]map[string]string, 0, len(commands))
	for _, cmd := range commands {
		quickActions = append(quickActions, map[string]string{
			"id":          cmd.Name,
			"label":       buildFeishuActionLabel(cmd.Name, cmd.Aliases),
			"description": cmd.Description,
			"command":     "/" + cmd.Name,
		})
	}

	// Set quick actions via FeishuBot interface
	if err := fsBot.SetQuickActions(quickActions); err != nil {
		return fmt.Errorf("failed to set quick actions: %w", err)
	}

	return nil
}

// buildFeishuActionLabel builds a label for Feishu quick action
// Format: "Command (alias1, alias2)" or just "Command"
func buildFeishuActionLabel(command string, aliases []string) string {
	label := "/" + command
	if len(aliases) > 0 {
		aliasList := ""
		for i, alias := range aliases {
			if i > 0 {
				aliasList += ", "
			}
			aliasList += "/" + alias
		}
		label += " (" + aliasList + ")"
	}
	return label
}
