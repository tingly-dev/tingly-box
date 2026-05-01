// Package cmdmenu provides command menu setup functionality for bots.
// This is separate from imbot/menu which handles interactive menus (cards, keyboards).
package setup

import (
	"github.com/tingly-dev/tingly-box/imbot/command"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/feishu"
	"github.com/tingly-dev/tingly-box/imbot/platform/telegram"
)

// Setup configures the command menu for a bot using the CommandRegistry.
// This dispatches to the platform-specific setup implementation based on the bot's platform.
func Setup(bot core.Bot, cmdRegistry *command.CommandRegistry) error {
	platform := bot.PlatformInfo().ID

	switch platform {
	case core.PlatformTelegram:
		return telegram.SetupMenuButton(bot, cmdRegistry)
	case core.PlatformFeishu, core.PlatformLark:
		return feishu.SetupQuickActions(bot, cmdRegistry)
	default:
		// Other platforms don't support menu configuration
		return nil
	}
}
