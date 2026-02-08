package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tingly-dev/tingly-box/imbot/pkg"
)

func main() {
	// Get bot tokens from environment
	telegramToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	discordToken := os.Getenv("DISCORD_BOT_TOKEN")

	if telegramToken == "" && discordToken == "" {
		log.Fatal("At least one bot token (TELEGRAM_BOT_TOKEN or DISCORD_BOT_TOKEN) is required")
	}

	// Create bot manager with custom options
	manager := imbot.NewManager(
		imbot.WithAutoReconnect(true),
		imbot.WithMaxReconnectAttempts(10),
		imbot.WithReconnectDelay(3000), // 3 seconds
	)

	// Collect configs
	var configs []*imbot.Config

	// Add Telegram if token provided
	if telegramToken != "" {
		configs = append(configs, &imbot.Config{
			Platform: imbot.PlatformTelegram,
			Enabled:  true,
			Auth: imbot.AuthConfig{
				Type:  "token",
				Token: telegramToken,
			},
			Logging: &imbot.LoggingConfig{
				Level: "info",
			},
		})
		log.Println("✓ Telegram bot configured")
	}

	// Add Discord if token provided
	if discordToken != "" {
		configs = append(configs, &imbot.Config{
			Platform: imbot.PlatformDiscord,
			Enabled:  true,
			Auth: imbot.AuthConfig{
				Type:  "token",
				Token: discordToken,
			},
			Logging: &imbot.LoggingConfig{
				Level: "info",
			},
			Options: map[string]interface{}{
				"intents": []string{"Guilds", "GuildMessages", "MessageContent"},
			},
		})
		log.Println("✓ Discord bot configured")
	}

	// Add all bots to manager
	if err := manager.AddBots(configs); err != nil {
		log.Fatalf("Failed to add bots: %v", err)
	}

	// Unified message handler for all platforms
	manager.OnMessage(func(msg imbot.Message, platform imbot.Platform) {
		// Log the message
		logMsg := fmt.Sprintf("[%-10s] %s: %s",
			platform,
			msg.GetSenderDisplayName(),
			msg.GetText(),
		)
		log.Println(logMsg)

		// Handle different content types
		switch msg.Content.ContentType() {
		case "text":
			handleTextMessage(manager, msg, platform)
		case "media":
			handleMediaMessage(manager, msg, platform)
		default:
			log.Printf("Unhandled content type: %s", msg.Content.ContentType())
		}
	})

	// Error handler
	manager.OnError(func(err error, platform imbot.Platform) {
		log.Printf("[%-10s] ❌ Error: %v", platform, err)
	})

	// Connection handlers
	manager.OnConnected(func(platform imbot.Platform) {
		log.Printf("[%-10s] ✅ Connected", platform)
	})

	manager.OnDisconnected(func(platform imbot.Platform) {
		log.Printf("[%-10s] ❌ Disconnected", platform)
	})

	// Start the manager
	log.Println("Starting multi-platform bot...")
	if err := manager.Start(context.Background()); err != nil {
		log.Fatalf("Failed to start manager: %v", err)
	}

	// Print status
	statuses := manager.GetStatus()
	for key, status := range statuses {
		if status.Connected {
			log.Printf("[%-10s] Status: ✅ Connected", key)
		} else {
			log.Printf("[%-10s] Status: ❌ Disconnected", key)
		}
	}

	log.Println("\n🤖 Multi-platform bot is running!")
	log.Println("Supported commands:")
	log.Println("  /help - Show help message")
	log.Println("  /ping - Ping the bot")
	log.Println("  /status - Show bot status")
	log.Println("Press Ctrl+C to stop.")

	<-context.Background().Done()
	log.Println("\n🛑 Shutting down...")

	if err := manager.Stop(context.Background()); err != nil {
		log.Printf("Error stopping manager: %v", err)
	}

	log.Println("✅ Bot stopped cleanly")
}

func handleTextMessage(manager *imbot.Manager, msg imbot.Message, platform imbot.Platform) {
	text := msg.GetText()

	// Handle commands
	switch {
	case text == "/help":
		sendHelp(manager, msg, platform)
	case text == "/ping":
		sendPong(manager, msg, platform)
	case text == "/status":
		sendStatus(manager, msg, platform)
	default:
		// Echo with platform indicator
		bot := manager.GetBot(platform)
		if bot != nil {
			reply := fmt.Sprintf("[%s] %s", platform, text)
			bot.SendText(context.Background(), msg.Sender.ID, reply)
		}
	}
}

func handleMediaMessage(manager *imbot.Manager, msg imbot.Message, platform imbot.Platform) {
	media := msg.GetMedia()
	if len(media) > 0 {
		bot := manager.GetBot(platform)
		if bot != nil {
			bot.SendText(context.Background(), msg.Sender.ID,
				fmt.Sprintf("Received %d media file(s)", len(media)))
		}
	}
}

func sendHelp(manager *imbot.Manager, msg imbot.Message, platform imbot.Platform) {
	bot := manager.GetBot(platform)
	if bot == nil {
		return
	}

	helpText := `🤖 *Available Commands*

/help - Show this help message
/ping - Check if bot is responsive
/status - Show bot status

This bot works on multiple platforms!`

	bot.SendMessage(context.Background(), msg.Sender.ID, &imbot.SendMessageOptions{
		Text:      helpText,
		ParseMode: imbot.ParseModeMarkdown,
	})
}

func sendPong(manager *imbot.Manager, msg imbot.Message, platform imbot.Platform) {
	bot := manager.GetBot(platform)
	if bot == nil {
		return
	}

	bot.SendText(context.Background(), msg.Sender.ID, "🏓 Pong!")
}

func sendStatus(manager *imbot.Manager, msg imbot.Message, platform imbot.Platform) {
	bot := manager.GetBot(platform)
	if bot == nil {
		return
	}

	statuses := manager.GetStatus()
	statusText := fmt.Sprintf("📊 *Bot Status*\n\nTotal bots: %d", len(statuses))

	for key, status := range statuses {
		emoji := "❌"
		if status.Connected {
			emoji = "✅"
		}
		statusText += fmt.Sprintf("\n%s %s: %s", emoji, key, getStatusText(status))
	}

	bot.SendMessage(context.Background(), msg.Sender.ID, &imbot.SendMessageOptions{
		Text:      statusText,
		ParseMode: imbot.ParseModeMarkdown,
	})
}

func getStatusText(status imbot.BotStatus) string {
	if status.Connected {
		return "Connected"
	}
	if status.Error != "" {
		return fmt.Sprintf("Error: %s", status.Error)
	}
	return "Disconnected"
}
