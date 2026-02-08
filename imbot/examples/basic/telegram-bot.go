package main

import (
	"context"
	"fmt"
	"log"
	"os"

	pkg "github.com/tingly-dev/tingly-box/imbot/pkg"
)

func main() {
	// Get bot token from environment
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	// Create bot manager
	manager := pkg.NewManager(
		pkg.WithAutoReconnect(true),
		pkg.WithMaxReconnectAttempts(5),
	)

	// Add Telegram bot
	err := manager.AddBot(&pkg.Config{
		Platform: pkg.PlatformTelegram,
		Enabled:  true,
		Auth: pkg.AuthConfig{
			Type:  "token",
			Token: token,
		},
		Logging: &pkg.LoggingConfig{
			Level:      "info",
			Timestamps: true,
		},
	})
	if err != nil {
		log.Fatalf("Failed to add bot: %v", err)
	}

	// Set up message handler
	manager.OnMessage(func(msg pkg.Message, platform pkg.Platform) {
		// Print incoming message
		fmt.Printf("[%-10s] %s (%s): %s\n",
			platform,
			msg.Sender.DisplayName,
			msg.Sender.ID,
			msg.GetText(),
		)

		// Echo the message back
		bot := manager.GetBot(platform)
		if bot != nil {
			_, err := bot.SendText(context.Background(), msg.Sender.ID, fmt.Sprintf("Echo: %s", msg.GetText()))
			if err != nil {
				log.Printf("Failed to send message: %v", err)
			}
		}
	})

	// Set up error handler
	manager.OnError(func(err error, platform pkg.Platform) {
		log.Printf("[%s] Error: %v", platform, err)
	})

	// Set up connection handlers
	manager.OnConnected(func(platform pkg.Platform) {
		log.Printf("[%s] Bot connected", platform)
	})

	manager.OnDisconnected(func(platform pkg.Platform) {
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
