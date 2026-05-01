package telegram

import (
	"fmt"

	"github.com/go-telegram/bot/models"
	"github.com/tingly-dev/tingly-box/imbot/command"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// SetupMenuButton configures the menu button for a Telegram bot using the CommandRegistry.
// This sets up both the bot command list and the menu button type.
func SetupMenuButton(bot core.Bot, cmdRegistry *command.CommandRegistry) error {
	// Cast to TelegramBot interface to access Telegram-specific methods
	tgBot, ok := bot.(*Bot)
	if !ok {
		return fmt.Errorf("bot is not a Telegram bot: %T", bot)
	}

	// Build command list from the command registry
	commands := buildTelegramBotCommands(cmdRegistry.BuildTelegramMenuCommands())

	if err := tgBot.SetCommandList(commands); err != nil {
		return fmt.Errorf("failed to set bot commands: %w", err)
	}

	// Set the menu button to show commands
	config := MenuButtonConfig{Type: MenuButtonTypeCommands}
	if err := tgBot.SetMenuButton(config); err != nil {
		return fmt.Errorf("failed to set menu button: %w", err)
	}

	return nil
}

// buildTelegramBotCommands converts command registry output to Telegram BotCommand format.
func buildTelegramBotCommands(cmds []map[string]string) []models.BotCommand {
	commands := make([]models.BotCommand, 0, len(cmds))
	for _, cmd := range cmds {
		command := cmd["command"]
		// Remove leading slash for Telegram API
		if len(command) > 0 && command[0] == '/' {
			command = command[1:]
		}
		commands = append(commands, models.BotCommand{
			Command:     command,
			Description: cmd["description"],
		})
	}
	return commands
}
