// standalone demonstrates running all components together:
// - Relay Server (handles sessions, message routing, persistence)
// - Bot (processes messages, generates responses)
// - Chat Server (serves frontend UI for users)
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/internal/platform/webchat/chat"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/webchat/protocol"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/webchat/relay"
)

func main() {
	// Configuration
	relayAddr := getEnv("RELAY_ADDR", ":8080")
	chatAddr := getEnv("CHAT_ADDR", ":3000")
	botID := getEnv("BOT_ID", "demo-bot")
	dbPath := getEnv("DB_PATH", "data/webchat/standalone.db")

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ============================================================
	// 1. Start Relay Server
	// - Handles WebSocket connections from chat clients
	// - Routes messages between clients and bots
	// - Persists messages to SQLite
	// ============================================================
	log.Println("==========================================")
	log.Printf("🌐 [1/3] Starting Relay Server on %s", relayAddr)

	relayServer := relay.NewRelayServer(relay.Config{
		Addr:      relayAddr,
		DBPath:    dbPath,
		CacheSize: 100,
	})

	if err := relayServer.Start(ctx); err != nil {
		log.Fatalf("Failed to start relay server: %v", err)
	}
	log.Printf("   ✅ Relay Server started")
	log.Printf("   📡 WebSocket: ws://localhost%s/ws", relayAddr)
	log.Printf("   🔌 Bot API: http://localhost%s/api/bot", relayAddr)

	// ============================================================
	// 2. Start Bot
	// - Registers with relay server
	// - Receives messages from relay
	// - Sends responses back via relay API
	// ============================================================
	log.Println("==========================================")
	log.Printf("🤖 [2/3] Starting Bot '%s'", botID)

	bot := NewDemoBot(botID, relayAddr, relayServer)

	// Register bot with relay server to receive messages
	relayServer.RegisterBot(botID, bot)

	log.Printf("   ✅ Bot '%s' registered with relay", botID)
	log.Printf("   📤 Send API: http://localhost%s/api/bot/%s/send", relayAddr, botID)

	// ============================================================
	// 3. Start Chat Server
	// - Serves frontend UI
	// - Points to relay server for WebSocket connection
	// ============================================================
	log.Println("==========================================")
	log.Printf("🎨 [3/3] Starting Chat Server on %s", chatAddr)

	chatServer := chat.NewChatServer(chat.Config{
		Addr:          chatAddr,
		RelayAddr:     relayAddr,
		CustomHTMLDir: "",
	})

	if err := chatServer.Start(ctx); err != nil {
		log.Fatalf("Failed to start chat server: %v", err)
	}
	log.Printf("   ✅ Chat Server started")
	log.Printf("   🌍 Open in browser: http://localhost%s", chatAddr)

	// ============================================================
	// All components running
	// ============================================================
	log.Println("==========================================")
	log.Println("✅ All components started successfully!")
	log.Println("")
	log.Println("📋 Architecture:")
	log.Printf("   User Browser → http://localhost%s (Chat Server)", chatAddr)
	log.Printf("   Chat Server  → ws://localhost%s/ws (Relay Server)", relayAddr)
	log.Printf("   Bot          → http://localhost%s/api/bot (Relay Server)", relayAddr)
	log.Println("")
	log.Println("   Press Ctrl+C to stop.")
	log.Println("==========================================")

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("\n🛑 Shutting down...")
	cancel()

	// Stop components in reverse order
	log.Println("   Stopping Chat Server...")
	chatServer.Stop(ctx)
	log.Println("   Stopping Bot...")
	bot.Stop()
	log.Println("   Stopping Relay Server...")
	relayServer.Stop(ctx)

	log.Println("✅ All components stopped.")
}

// ============================================================
// DemoBot - Simple bot implementation
// ============================================================

// DemoBot implements relay.BotHandler
type DemoBot struct {
	botID       string
	relayAddr   string
	relayServer *relay.RelayServer
	httpClient  *http.Client
}

// NewDemoBot creates a new demo bot
func NewDemoBot(botID, relayAddr string, relayServer *relay.RelayServer) *DemoBot {
	return &DemoBot{
		botID:       botID,
		relayAddr:   relayAddr,
		relayServer: relayServer,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// HandleMessage handles incoming messages from relay (implements relay.BotHandler)
func (b *DemoBot) HandleMessage(sessionID string, msgData *protocol.MessageData) error {
	// Print incoming message
	fmt.Printf("[Bot] 📩 Message from %s: %s\n", msgData.SenderName, msgData.Text)

	// Handle text messages
	if msgData.Text != "" {
		response := b.processMessage(msgData.Text)

		// Send response back via relay server (direct call for in-process demo)
		responseData := &protocol.MessageData{
			ID:         generateMessageID(),
			Timestamp:  time.Now().Unix(),
			SenderID:   b.botID,
			SenderName: "Demo Bot",
			Text:       response,
		}

		if err := b.relayServer.SendToSession(sessionID, responseData); err != nil {
			log.Printf("[Bot] ❌ Failed to send response: %v", err)
			return err
		}
		fmt.Printf("[Bot] 📤 Sent response to session %s\n", sessionID)
	}

	return nil
}

// SessionJoined handles session join events (implements relay.BotHandler)
func (b *DemoBot) SessionJoined(sessionID string) {
	log.Printf("[Bot] ✅ Session joined: %s", sessionID)

	// Send welcome message
	msgData := &protocol.MessageData{
		ID:         generateMessageID(),
		Timestamp:  time.Now().Unix(),
		SenderID:   b.botID,
		SenderName: "Demo Bot",
		Text:       "👋 Welcome to WebChat Demo! Type /help for commands.",
	}

	if err := b.relayServer.SendToSession(sessionID, msgData); err != nil {
		log.Printf("[Bot] ❌ Failed to send welcome: %v", err)
	}
}

// SessionLeft handles session leave events (implements relay.BotHandler)
func (b *DemoBot) SessionLeft(sessionID string) {
	log.Printf("[Bot] 👋 Session left: %s", sessionID)
}

// Stop stops the bot
func (b *DemoBot) Stop() {
	b.httpClient.CloseIdleConnections()
}

// processMessage processes incoming text and returns a response
func (b *DemoBot) processMessage(text string) string {
	text = strings.TrimSpace(text)

	// Handle commands
	if strings.HasPrefix(text, "/") {
		return b.handleCommand(text)
	}

	// Echo with greeting
	greetings := []string{"Hello!", "Hi there!", "Hey!", "Howdy!", "Greetings!"}
	greeting := greetings[int(time.Now().Unix())%len(greetings)]
	return fmt.Sprintf("📨 %s\n\n💭 You said: \"%s\"", greeting, text)
}

// handleCommand handles bot commands
func (b *DemoBot) handleCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "❓ Empty command"
	}

	cmdName := strings.ToLower(parts[0][1:])
	args := parts[1:]

	switch cmdName {
	case "start", "help":
		return `👋 Welcome to WebChat Demo Bot!

📚 Commands:
  /help     - Show this help
  /ping     - Check responsiveness
  /time     - Current time
  /echo <text> - Echo back
  /reverse <text> - Reverse text
  /joke     - Random joke
  /roll     - Roll dice
  /flip     - Flip coin`

	case "ping":
		return "🏓 Pong!"

	case "time":
		now := time.Now()
		return fmt.Sprintf("🕐 %s", now.Format("2006-01-02 15:04:05"))

	case "echo":
		if len(args) == 0 {
			return "📢 Usage: /echo <text>"
		}
		return fmt.Sprintf("📢 %s", strings.Join(args, " "))

	case "reverse":
		if len(args) == 0 {
			return "🔃 Usage: /reverse <text>"
		}
		text := strings.Join(args, " ")
		runes := []rune(text)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return fmt.Sprintf("🔃 %s", string(runes))

	case "joke":
		jokes := []string{
			"Why do programmers prefer dark mode? Because light attracts bugs! 🐛",
			"Why did the developer go broke? Because he used up all his cache! 💰",
			"There are 10 types of people: those who understand binary and those who don't! 🔢",
		}
		return "😄 " + jokes[int(time.Now().Unix())%len(jokes)]

	case "roll":
		roll := int(time.Now().UnixNano())%100 + 1
		return fmt.Sprintf("🎲 You rolled: %d", roll)

	case "flip":
		if time.Now().UnixNano()%2 == 0 {
			return "🪙 Heads!"
		}
		return "🪙 Tails!"

	default:
		return fmt.Sprintf("❓ Unknown: /%s\nType /help for commands.", cmdName)
	}
}

// ============================================================
// Utility functions
// ============================================================

func generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
