package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"

	"github.com/tingly-dev/tingly-box/imbot"
)

var WHITE_LIST []string

func init() {
	WHITE_LIST = []string{""}
}

func main() {
	// Get bot token from environment
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	// Create bot manager
	manager := imbot.NewManager(
		imbot.WithAutoReconnect(true),
		imbot.WithMaxReconnectAttempts(5),
	)

	// Add Telegram bot
	err := manager.AddBot(&imbot.Config{
		Platform: imbot.PlatformTelegram,
		Enabled:  true,
		Auth: imbot.AuthConfig{
			Type:  "token",
			Token: token,
		},
		Logging: &imbot.LoggingConfig{
			Level:      "info",
			Timestamps: true,
		},
	})
	if err != nil {
		log.Fatalf("Failed to add bot: %v", err)
	}

	// Set up message handler
	manager.OnMessage(func(msg imbot.Message, platform imbot.Platform) {
		// Print incoming message
		fmt.Printf("[%-10s] %s (%s): %s\n",
			platform,
			msg.Sender.DisplayName,
			msg.Sender.ID,
			msg.GetText(),
		)

		// Echo the message back
		bot := manager.GetBot(platform)

		// check user
		if !slices.Contains(WHITE_LIST, msg.Sender.ID) {
			log.Printf("Rejected by white list")
			bot.SendText(context.Background(), msg.Sender.ID, fmt.Sprintf("Rejected"))
			return
		}

		if bot != nil {
			_, err := bot.SendText(context.Background(), msg.Sender.ID, fmt.Sprintf("Echo: %s", msg.GetText()))
			if err != nil {
				log.Printf("Failed to send message: %v", err)
			}
		}
	})

	// Set up error handler
	manager.OnError(func(err error, platform imbot.Platform) {
		log.Printf("[%s] Error: %v", platform, err)
	})

	// Set up connection handlers
	manager.OnConnected(func(platform imbot.Platform) {
		log.Printf("[%s] Bot connected", platform)
	})

	manager.OnDisconnected(func(platform imbot.Platform) {
		log.Printf("[%s] Bot disconnected", platform)
	})

	// Start the manager
	log.Println("Starting Telegram bot...")
	if err := manager.Start(context.Background()); err != nil {
		log.Fatalf("Failed to start manager: %v", err)
	}

	log.Println("Bot is running. Press Ctrl+C to stop.")

	// Wait for interrupt signal
	<-context.Background().Done()
	log.Println("Shutting down...")

	// Stop the manager
	if err := manager.Stop(context.Background()); err != nil {
		log.Printf("Error stopping manager: %v", err)
	}

	log.Println("Bot stopped.")
}
