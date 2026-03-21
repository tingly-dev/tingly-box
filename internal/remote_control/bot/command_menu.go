package bot

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/imbot"
)

// CommandMenuConfig defines a command that appears in platform menus
// Each platform can choose how to display these commands:
// - Telegram: BotCommand list (shows in command palette)
// - Lark/Feishu: Quick Actions
// - Slack: Slash Commands
type CommandMenuConfig struct {
	// Command is the command name (without slash, e.g., "help", "cd")
	Command string

	// Description is shown to users
	Description string

	// Aliases are alternative names for this command
	Aliases []string

	// Platforms where this command is available
	// If nil, available on all platforms
	Platforms []imbot.Platform

	// PlatformSpecific provides platform-specific overrides
	PlatformSpecific map[imbot.Platform]CommandOverride

	// Handler is the function to execute when command is invoked
	// This is for reference; actual handling is done in bot_command.go
	Handler string // e.g., "handleHelpCommand"

	// Priority determines order in menu (higher = first)
	// If 0, uses default ordering
	Priority int

	// Category groups related commands together
	// e.g., "session", "project", "system"
	Category string

	// Hidden hides this command from the menu (but command still works)
	Hidden bool
}

// CommandOverride provides platform-specific configuration
type CommandOverride struct {
	// Override description if needed
	Description string

	// Different command name for this platform
	Command string

	// Additional platform-specific parameters
	Params map[string]interface{}
}

// CommandRegistry manages command menu configurations
type CommandRegistry struct {
	commands []CommandMenuConfig
	byName   map[string]*CommandMenuConfig
}

// NewCommandRegistry creates a new command registry
func NewCommandRegistry() *CommandRegistry {
	r := &CommandRegistry{
		commands: make([]CommandMenuConfig, 0),
		byName:   make(map[string]*CommandMenuConfig),
	}
	r.registerDefaultCommands()
	return r
}

// registerDefaultCommands registers the built-in commands
func (r *CommandRegistry) registerDefaultCommands() {
	// Session commands (high priority)
	r.Register(CommandMenuConfig{
		Command:     "help",
		Description: "Show available commands and help",
		Aliases:     []string{"h", "start"},
		Priority:    100,
		Category:    "session",
	})

	r.Register(CommandMenuConfig{
		Command:     "cd",
		Description: "Bind and cd into a project directory",
		Aliases:     []string{"bind"},
		Priority:    90,
		Category:    "project",
	})

	r.Register(CommandMenuConfig{
		Command:     "clear",
		Description: "Clear context and start new session",
		Priority:    80,
		Category:    "session",
	})

	r.Register(CommandMenuConfig{
		Command:     "stop",
		Description: "Stop the current running task",
		Aliases:     []string{"interrupt"},
		Priority:    70,
		Category:    "session",
	})

	// Project commands
	r.Register(CommandMenuConfig{
		Command:     "project",
		Description: "Show and switch between projects",
		Priority:    60,
		Category:    "project",
	})

	r.Register(CommandMenuConfig{
		Command:     "status",
		Description: "Show current session status",
		Priority:    50,
		Category:    "project",
	})

	// System commands
	r.Register(CommandMenuConfig{
		Command:     "bash",
		Description: "Execute bash commands (cd, ls, pwd)",
		Priority:    40,
		Category:    "system",
	})

	r.Register(CommandMenuConfig{
		Command:     "join",
		Description: "Add group to whitelist (Telegram only)",
		Platforms:   []imbot.Platform{imbot.PlatformTelegram},
		Priority:    30,
		Category:    "system",
	})

	// Advanced commands (lower priority, may be hidden)
	r.Register(CommandMenuConfig{
		Command:     "verbose",
		Description: "Show all message details (default)",
		Priority:    20,
		Category:    "advanced",
	})

	r.Register(CommandMenuConfig{
		Command:     "noverbose",
		Description: "Hide intermediate messages",
		Priority:    15,
		Category:    "advanced",
	})

	r.Register(CommandMenuConfig{
		Command:     "yolo",
		Description: "Toggle auto-approve mode (Claude Code)",
		Priority:    10,
		Category:    "advanced",
	})

	r.Register(CommandMenuConfig{
		Command:     "mock",
		Description: "Test with mock agent",
		Hidden:      true, // Hidden from menu, but still works
		Category:    "advanced",
	})
}

// Register adds a new command to the registry
func (r *CommandRegistry) Register(config CommandMenuConfig) {
	r.byName[config.Command] = &config
	r.commands = append(r.commands, config)
}

// Get returns a command configuration by name
func (r *CommandRegistry) Get(command string) (*CommandMenuConfig, bool) {
	cfg, ok := r.byName[command]
	return cfg, ok
}

// GetByAlias returns a command configuration by alias
func (r *CommandRegistry) GetByAlias(alias string) (*CommandMenuConfig, bool) {
	for _, cfg := range r.commands {
		if cfg.Command == alias {
			return &cfg, true
		}
		for _, a := range cfg.Aliases {
			if a == alias {
				return &cfg, true
			}
		}
	}
	return nil, false
}

// ForPlatform returns commands visible for a specific platform, ordered by priority
func (r *CommandRegistry) ForPlatform(platform imbot.Platform) []CommandMenuConfig {
	result := make([]CommandMenuConfig, 0, len(r.commands))

	for _, cfg := range r.commands {
		if cfg.Hidden {
			continue
		}

		// Check if command is available for this platform
		if len(cfg.Platforms) > 0 {
			found := false
			for _, p := range cfg.Platforms {
				if p == platform {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		result = append(result, cfg)
	}

	// Sort by priority (higher first)
	// Simple bubble sort for small slices
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Priority < result[j].Priority {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// ForCategory returns commands in a specific category
func (r *CommandRegistry) ForCategory(category string) []CommandMenuConfig {
	result := make([]CommandMenuConfig, 0)
	for _, cfg := range r.commands {
		if cfg.Category == category && !cfg.Hidden {
			result = append(result, cfg)
		}
	}
	return result
}

// AllCommands returns all registered commands
func (r *CommandRegistry) AllCommands() []CommandMenuConfig {
	return r.commands
}

// BuildTelegramCommands returns commands formatted for Telegram Bot API
func (r *CommandRegistry) BuildTelegramCommands() []map[string]string {
	commands := r.ForPlatform(imbot.PlatformTelegram)
	result := make([]map[string]string, 0, len(commands))

	for _, cfg := range commands {
		cmd := cfg.Command
		if override, ok := cfg.PlatformSpecific[imbot.PlatformTelegram]; ok && override.Command != "" {
			cmd = override.Command
		}
		desc := cfg.Description
		if override, ok := cfg.PlatformSpecific[imbot.PlatformTelegram]; ok && override.Description != "" {
			desc = override.Description
		}

		result = append(result, map[string]string{
			"command":     "/" + cmd,
			"description": desc,
		})
	}

	return result
}

// BuildFeishuQuickActions returns commands formatted for Feishu/Lark Quick Actions
func (r *CommandRegistry) BuildFeishuQuickActions() []map[string]interface{} {
	commands := r.ForPlatform(imbot.PlatformFeishu)
	result := make([]map[string]interface{}, 0, len(commands))

	for _, cfg := range commands {
		cmd := cfg.Command
		if override, ok := cfg.PlatformSpecific[imbot.PlatformFeishu]; ok && override.Command != "" {
			cmd = override.Command
		}

		result = append(result, map[string]interface{}{
			"id":       "/" + cmd,
			"text":     cfg.Description,
			"value":    cmd,
			"category": cfg.Category,
			"priority": cfg.Priority,
		})
	}

	return result
}

// BuildSlackCommands returns commands formatted for Slack manifest
func (r *CommandRegistry) BuildSlackCommands() []map[string]string {
	commands := r.ForPlatform(imbot.PlatformSlack)
	result := make([]map[string]string, 0, len(commands))

	for _, cfg := range commands {
		cmd := cfg.Command
		if override, ok := cfg.PlatformSpecific[imbot.PlatformSlack]; ok && override.Command != "" {
			cmd = override.Command
		}

		result = append(result, map[string]string{
			"command":     "/" + cmd,
			"description": cfg.Description,
			"usage_hint":  "", // Could be added per command
		})
	}

	return result
}

// BuildHelpText generates help text for a platform and chat type
func (r *CommandRegistry) BuildHelpText(platform imbot.Platform, isDirectMessage bool) string {
	commands := r.ForPlatform(platform)

	var text string
	if isDirectMessage {
		text = "Bot Commands:\n"
	} else {
		text = "Group Chat Commands:\n"
	}

	// Find max command length for alignment
	maxCmdLen := 0
	for _, cfg := range commands {
		cmdLen := len(cfg.Command)
		for _, a := range cfg.Aliases {
			cmdLen += len(a) + 2 // ", /alias"
		}
		if cmdLen > maxCmdLen {
			maxCmdLen = cmdLen
		}
	}

	currentCategory := ""
	for _, cfg := range commands {
		// Add category headers
		if cfg.Category != currentCategory {
			if currentCategory != "" {
				text += "\n"
			}
			currentCategory = cfg.Category
		}

		cmdDisplay := "/" + cfg.Command
		aliases := ""
		if len(cfg.Aliases) > 0 {
			aliasStr := ""
			for _, a := range cfg.Aliases {
				aliasStr += ", /" + a
			}
			aliases = aliasStr
		}

		// Build full command part including aliases
		fullCmd := cmdDisplay + aliases

		// Pad with spaces to align descriptions
		padding := maxCmdLen - len(fullCmd) + 4 // 4 spaces minimum gap
		if padding < 2 {
			padding = 2
		}
		text += fmt.Sprintf("%s%s%s\n", fullCmd, strings.Repeat(" ", padding), cfg.Description)
	}

	return text
}

// Global registry instance
var defaultRegistry = NewCommandRegistry()

// GetCommandRegistry returns the default command registry
func GetCommandRegistry() *CommandRegistry {
	return defaultRegistry
}

// RegisterCommand registers a command in the default registry
func RegisterCommand(config CommandMenuConfig) {
	defaultRegistry.Register(config)
}

// GetCommand returns a command from the default registry
func GetCommand(command string) (*CommandMenuConfig, bool) {
	return defaultRegistry.Get(command)
}

// GetCommandByAlias returns a command by alias from the default registry
func GetCommandByAlias(alias string) (*CommandMenuConfig, bool) {
	return defaultRegistry.GetByAlias(alias)
}
