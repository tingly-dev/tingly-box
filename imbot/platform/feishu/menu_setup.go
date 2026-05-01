package feishu

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/command"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// SetupQuickActions configures the quick actions for a Feishu/Lark bot using the CommandRegistry.
// Quick actions appear when users type "/" in the chat.
func SetupQuickActions(bot core.Bot, cmdRegistry *command.CommandRegistry) error {
	// Cast to FeishuBot interface to access Feishu-specific methods
	fsBot, ok := bot.(*Bot)
	if !ok {
		return fmt.Errorf("bot is not a Feishu/Lark bot: %T", bot)
	}

	// Build quick actions from the command registry
	quickActions := cmdRegistry.BuildFeishuQuickActions()

	// Convert to []map[string]string for the interface
	actions := make([]map[string]string, 0, len(quickActions))
	for _, action := range quickActions {
		a := make(map[string]string)
		if id, ok := action["id"].(string); ok {
			a["id"] = id
		}
		if label, ok := action["label"].(string); ok {
			a["label"] = label
		}
		if description, ok := action["description"].(string); ok {
			a["description"] = description
		}
		if command, ok := action["command"].(string); ok {
			a["command"] = command
		}
		if icon, ok := action["icon"].(string); ok {
			a["icon"] = icon
		}
		actions = append(actions, a)
	}

	// Set quick actions via FeishuBot interface
	if err := fsBot.SetQuickActions(actions); err != nil {
		return fmt.Errorf("failed to set quick actions: %w", err)
	}

	return nil
}
