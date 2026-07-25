package platform

import (
	"github.com/tingly-dev/tingly-box/imbot/command"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/feishu"
	"github.com/tingly-dev/tingly-box/imbot/platform/telegram"
)

// commandMenuSetup maps a platform to its native command-menu installer.
//
// This lives beside the bot-creator registry because it is the same kind of
// fact: what a platform can do and how to reach it. It used to be a switch in
// internal/remote_control's bot manager, which meant the consumer had to
// import platform packages and be edited whenever a platform gained a menu.
//
// A platform with no entry simply has no native command menu — that is a
// normal outcome, not an error.
var commandMenuSetup = map[core.Platform]func(core.Bot, *command.CommandRegistry) error{
	core.PlatformTelegram: telegram.SetupMenuButton,
	core.PlatformFeishu:   feishu.SetupQuickActions,
	core.PlatformLark:     feishu.SetupQuickActions,
}

// SetupCommandMenu installs the command registry into the platform's native
// menu. Platforms without one are a no-op, so callers do not need to ask first.
func SetupCommandMenu(bot core.Bot, platform core.Platform, reg *command.CommandRegistry) error {
	setup, ok := commandMenuSetup[platform]
	if !ok {
		return nil
	}
	return setup(bot, reg)
}
