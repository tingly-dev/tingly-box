package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/gin-gonic/gin" // For custom route setup (routeSetupFunc option)
	"github.com/tingly-dev/tingly-box/imbot"
)

func main() {
	// Get server address from environment (default: :8080)
	addr := os.Getenv("WEBSHOT_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	// Create bot manager
	manager := imbot.NewManager(
		imbot.WithAutoReconnect(true),
		imbot.WithMaxReconnectAttempts(5),
	)

	// Add WebChat bot
	err := manager.AddBot(&imbot.Config{
		Platform: imbot.PlatformWebChat,
		Enabled:  true,
		Auth: imbot.AuthConfig{
			Type: "none", // No authentication for MVP
		},
		Options: map[string]any{
			"addr":         addr,
			"dbPath":       "data/webchat/webchat.db", // SQLite database path
			"historyLimit": 50,                        // Messages to send on reconnect
			"cacheSize":    100,                       // In-memory cache per session

			// --- CUSTOM OPTIONS ---
			// To use custom frontend, uncomment and set htmlPath:
			// "htmlPath": "./my-frontend/dist",

			// To register custom gin routes, use routeSetupFunc:
			// "routeSetupFunc": imbot.RouteSetupFunc(func(engine *gin.Engine) {
			//     // Custom API endpoints
			//     engine.GET("/api/custom/info", func(c *gin.Context) {
			//         c.JSON(200, gin.H{"app": "My Bot", "version": "1.0"})
			//     })
			//     // Serve static assets
			//     engine.Static("/assets", "./assets")
			// }),
		},
		Logging: &imbot.LoggingConfig{
			Level:      "info",
			Timestamps: true,
		},
	})
	if err != nil {
		log.Fatalf("Failed to add WebChat bot: %v", err)
	}

	// Set up message handler
	simpleBot := NewSimpleBot("WebChat Demo Bot")
	manager.OnMessage(func(msg imbot.Message, platform imbot.Platform) {
		// Print incoming message
		fmt.Printf("[%-10s] %s (%s): %s\n",
			platform,
			msg.GetSenderDisplayName(),
			msg.Sender.ID,
			msg.GetText(),
		)

		// Get bot instance
		bot := manager.GetBot(platform)
		if bot == nil {
			log.Printf("Bot not found for platform: %s", platform)
			return
		}

		// Handle text messages with SimpleBot
		if msg.IsTextContent() {
			response, shouldReply := simpleBot.HandleMessage(context.Background(), msg)
			if shouldReply {
				// For WebChat, use Recipient.ID (session ID) as the target
				target := msg.Sender.ID
				if platform == imbot.PlatformWebChat {
					target = msg.Recipient.ID
				}
				bot.SendText(context.Background(), target, response)
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
	log.Printf("🌐 Starting WebChat bot on %s...", addr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		log.Fatalf("Failed to start manager: %v", err)
	}

	log.Println("✅ WebChat bot is running.")
	log.Printf("   📱 Open http://localhost%s in your browser", addr)
	log.Printf("   🔌 WebSocket: ws://localhost%s/ws", addr)
	log.Println("   Press Ctrl+C to stop.")

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("🛑 Shutting down...")
	cancel()

	// Stop the manager
	if err := manager.Stop(context.Background()); err != nil {
		log.Printf("Error stopping manager: %v", err)
	}

	log.Println("✅ WebChat bot stopped.")
}
