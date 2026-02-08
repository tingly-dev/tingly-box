package telegram

import (
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// NewDiscordBot creates a new Discord bot (stub)
func NewDiscordBot(config *core.Config) (core.Bot, error) {
	// TODO: Implement Discord bot
	return nil, core.NewBotError(core.ErrPlatformError, "Discord bot not yet implemented", false)
}

// NewSlackBot creates a new Slack bot (stub)
func NewSlackBot(config *core.Config) (core.Bot, error) {
	// TODO: Implement Slack bot
	return nil, core.NewBotError(core.ErrPlatformError, "Slack bot not yet implemented", false)
}
