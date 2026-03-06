package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
)

var WHITE_LIST []string

func init() {
	WHITE_LIST = []string{}
}

// CommandHandler represents a command handler function
type CommandHandler func(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error

// Command represents a bot command
type Command struct {
	Name        string
	Description string
	Handler     CommandHandler
	Aliases     []string
}

// BotCommands holds all bot commands
var BotCommands = []Command{
	{
		Name:        "start",
		Description: "Start using the bot",
		Handler:     handleStart,
		Aliases:     []string{"help"},
	},
	{
		Name:        "ping",
		Description: "Check if the bot is online",
		Handler:     handlePing,
	},
	{
		Name:        "echo",
		Description: "Repeat message",
		Handler:     handleEcho,
	},
	{
		Name:        "time",
		Description: "Show current time",
		Handler:     handleTime,
	},
	{
		Name:        "info",
		Description: "Show user information",
		Handler:     handleInfo,
	},
	{
		Name:        "status",
		Description: "Show bot status",
		Handler:     handleStatus,
	},
	{
		Name:        "about",
		Description: "About this bot",
		Handler:     handleAbout,
	},
}

func main() {
	// Get credentials from environment
	appKey := os.Getenv("DINGTALK_APP_KEY")
	appSecret := os.Getenv("DINGTALK_APP_SECRET")
	streamURL := os.Getenv("DINGTALK_STREAM_URL")

	if appKey == "" || appSecret == "" {
		log.Fatal("DINGTALK_APP_KEY and DINGTALK_APP_SECRET environment variables are required")
	}

	// Create cancellable context for shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create bot manager
	manager := imbot.NewManager(
		imbot.WithAutoReconnect(true),
		imbot.WithMaxReconnectAttempts(5),
	)

	// Add DingTalk bot
	config := &imbot.Config{
		Platform: imbot.PlatformDingTalk,
		Enabled:  true,
		Auth: imbot.AuthConfig{
			Type:         "oauth",
			ClientID:     appKey,
			ClientSecret: appSecret,
		},
		Logging: &imbot.LoggingConfig{
			Level:      "info",
			Timestamps: true,
		},
	}

	// Add stream URL only if provided
	if streamURL != "" {
		config.Options = map[string]interface{}{
			"streamURL": streamURL,
		}
	}

	err := manager.AddBot(config)
	if err != nil {
		log.Fatalf("Failed to add bot: %v", err)
	}

	// Set up message handler
	manager.OnMessage(func(msg imbot.Message, platform imbot.Platform, botUUID string) {
		// Print incoming message for logging
		fmt.Printf("[%-10s] %s (%s): %s\n",
			platform,
			msg.GetSenderDisplayName(),
			msg.Sender.ID,
			msg.GetText(),
		)

		// Get bot instance by UUID (preferred) or fallback to platform
		var bot imbot.Bot
		bot = manager.GetBot(botUUID, platform)
		if bot == nil {
			log.Printf("Bot not found for platform: %s, UUID: %s", platform, botUUID)
			return
		}

		// Check whitelist
		if !isWhitelisted(msg.Sender.ID) {
			log.Printf("User %s rejected by whitelist", msg.Sender.ID)
			bot.SendText(context.Background(), getChatID(msg), "⛔ Sorry, you do not have permission to use this bot.")
			return
		}

		// Handle text messages
		if msg.IsTextContent() {
			handleTextMessage(context.Background(), bot, msg)
			return
		}

		// Handle other content types
		switch msg.Content.ContentType() {
		case "media":
			handleMediaMessage(context.Background(), bot, msg)
		default:
			log.Printf("Unhandled content type: %s", msg.Content.ContentType())
		}
	})

	// Set up error handler
	manager.OnError(func(err error, platform imbot.Platform, botUUID string) {
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
	log.Println("🤖 Starting DingTalk bot...")
	if err := manager.Start(ctx); err != nil {
		log.Fatalf("Failed to start manager: %v", err)
	}

	log.Println("✅ Bot is running. Press Ctrl+C to stop.")

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Wait for interrupt signal
	<-sigCh
	log.Println("🛑 Shutting down...")

	// Stop the manager
	cancel() // Cancel the context first
	if err := manager.Stop(context.Background()); err != nil {
		log.Printf("Error stopping manager: %v", err)
	}

	log.Println("✅ Bot stopped.")
}

// isWhitelisted checks if a user ID is in the whitelist
func isWhitelisted(userID string) bool {
	// always return true if white list is empty
	if len(WHITE_LIST) == 0 {
		return true
	}
	return slices.Contains(WHITE_LIST, userID)
}

// handleTextMessage processes text messages and commands
func handleTextMessage(ctx context.Context, bot imbot.Bot, msg imbot.Message) {
	text := strings.TrimSpace(msg.GetText())

	// Check if it's a command (starts with /)
	if strings.HasPrefix(text, "/") {
		handleCommand(ctx, bot, msg, text)
		return
	}

	// Handle regular text messages (echo)
	handleEcho(ctx, bot, msg, []string{text})
}

// handleCommand processes bot commands
func handleCommand(ctx context.Context, bot imbot.Bot, msg imbot.Message, text string) {
	// Parse command and arguments
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}

	// Extract command name (remove / prefix)
	cmdName := strings.ToLower(parts[0][1:])
	args := parts[1:]

	// Find and execute the command
	for _, cmd := range BotCommands {
		// Check main command name
		if cmd.Name == cmdName {
			executeCommand(ctx, bot, msg, cmd, args)
			return
		}
		// Check aliases
		for _, alias := range cmd.Aliases {
			if alias == cmdName {
				executeCommand(ctx, bot, msg, cmd, args)
				return
			}
		}
	}

	// Command not found
	sendUnknownCommandMessage(ctx, bot, msg.Sender.ID, cmdName)
}

// executeCommand executes a command with error handling
func executeCommand(ctx context.Context, bot imbot.Bot, msg imbot.Message, cmd Command, args []string) {
	if err := cmd.Handler(ctx, bot, msg, args); err != nil {
		log.Printf("Command /%s error: %v", cmd.Name, err)
		bot.SendText(ctx, getChatID(msg), fmt.Sprintf("❌ Error executing command: %v", err))
	}
}

// sendUnknownCommandMessage echoes the message for unknown commands
func sendUnknownCommandMessage(ctx context.Context, bot imbot.Bot, chatID, cmdName string) {
	// Echo the unknown command back
	echoMsg := fmt.Sprintf("📢 %s", cmdName)
	bot.SendText(ctx, chatID, echoMsg)
}

// ===== Command Handlers =====

// handleStart sends a welcome message
func handleStart(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	welcomeMsg := `👋 Welcome to the DingTalk bot!

Available commands:
/start, /help - Show this help message
/ping - Check bot status
/echo <text> - Repeat message
/time - Show current time
/info - Show your information
/status - Show bot status
/about - About this bot`

	_, err := bot.SendText(ctx, getChatID(msg), welcomeMsg)
	return err
}

// handlePing responds with pong
func handlePing(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	startTime := time.Now()
	_, err := bot.SendText(ctx, getChatID(msg), "🏓 Pong!")
	if err != nil {
		return err
	}

	// Send latency info
	latency := time.Since(startTime).Milliseconds()
	_, err = bot.SendText(ctx, getChatID(msg), fmt.Sprintf("⏱️ Latency: %dms", latency))
	return err
}

// handleEcho repeats the message back
func handleEcho(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	if len(args) == 0 {
		_, err := bot.SendText(ctx, getChatID(msg), "📢 Please enter a message to repeat.\nUsage: /echo <message>")
		return err
	}

	echoMsg := fmt.Sprintf("📢 %s", strings.Join(args, " "))
	_, err := bot.SendText(ctx, getChatID(msg), echoMsg)
	return err
}

// handleTime sends the current time
func handleTime(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	now := time.Now()
	timeMsg := fmt.Sprintf("🕐 Current time:\n📅 %s\n⏰ %s",
		now.Format("2006-01-02 Monday"),
		now.Format("15:04:05 MST"))
	_, err := bot.SendText(ctx, getChatID(msg), timeMsg)
	return err
}

// handleInfo sends user information
func handleInfo(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	infoMsg := fmt.Sprintf(`👤 User information:

🆔 ID: %s
👤 Display name: %s`,
		msg.Sender.ID,
		msg.GetSenderDisplayName())

	if msg.Sender.Username != "" {
		infoMsg = fmt.Sprintf(`👤 User information:

🆔 ID: %s
👤 Display name: %s
🔒 Username: %s`,
			msg.Sender.ID,
			msg.GetSenderDisplayName(),
			msg.Sender.Username)
	}

	_, err := bot.SendText(ctx, getChatID(msg), infoMsg)
	return err
}

// handleStatus sends bot status information
func handleStatus(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	status := bot.Status()

	statusMsg := fmt.Sprintf(`🤖 Bot status:

🔗 Connection status: %s
🔐 Authentication status: %s
✅ Ready status: %s`,
		getStatusEmoji(status.Connected),
		getStatusEmoji(status.Authenticated),
		getStatusEmoji(status.Ready))

	if status.Error != "" {
		statusMsg += fmt.Sprintf("\n❌ Error: %s", status.Error)
	}

	_, err := bot.SendText(ctx, getChatID(msg), statusMsg)
	return err
}

// handleAbout sends information about the bot
func handleAbout(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	aboutMsg := `ℹ️ About this bot

This is a DingTalk bot example based on the imbot framework.

Features:
• Command handling system
• User whitelist
• Multimedia support
• Error handling
• Auto reconnect

Version: 1.0.0
Framework: github.com/tingly-dev/tingly-box/imbot`

	_, err := bot.SendText(ctx, getChatID(msg), aboutMsg)
	return err
}

// handleMediaMessage processes media messages
func handleMediaMessage(ctx context.Context, bot imbot.Bot, msg imbot.Message) {
	media := msg.GetMedia()
	if len(media) == 0 {
		return
	}

	var response string
	switch media[0].Type {
	case "image":
		response = "🖼️ Image received!"
	case "video":
		response = "🎬 Video received!"
	case "audio":
		response = "🎵 Audio received!"
	case "document":
		response = "📄 Document received!"
	default:
		response = fmt.Sprintf("📎 Media file received: %s", media[0].Type)
	}

	bot.SendText(ctx, getChatID(msg), response)
}

// ===== Helper Functions =====

// getChatID returns the correct chat ID for sending messages
// For DingTalk stream mode, we need to use conversation ID (Recipient.ID)
// because webhook URLs are stored with conversation ID as key
func getChatID(msg imbot.Message) string {
	// For DingTalk, use Recipient.ID (conversation ID)
	// For other platforms, use Sender.ID may work differently
	if msg.Platform == imbot.PlatformDingTalk {
		return msg.Recipient.ID
	}
	return msg.Sender.ID
}

// getStatusEmoji returns an emoji for boolean status
func getStatusEmoji(status bool) string {
	if status {
		return "✅ Yes"
	}
	return "❌ No"
}
