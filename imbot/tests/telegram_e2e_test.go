//go:build e2e
// +build e2e

package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/telegram"
)

// TestE2E_TelegramBot_RealBot creates a real bot for debugging purposes
// Run with: go test -tags=e2e -v -run TestE2E_TelegramBot_RealBot ./imbot/internal/platform/telegram/
// Required environment variable: TELEGRAM_BOT_TOKEN
// Optional environment variable: TELEGRAM_TEST_CHAT_ID (for sending test messages)
func TestE2E_TelegramBot_RealBot(t *testing.T) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		t.Skip("Skipping e2e test: TELEGRAM_BOT_TOKEN environment variable not set")
	}

	// Optional: specify a chat ID to send test messages to
	testChatID := os.Getenv("TELEGRAM_TEST_CHAT_ID")

	config := &core.Config{
		Platform: core.PlatformTelegram,
		Enabled:  true,
		Auth: core.AuthConfig{
			Type:  "token",
			Token: token,
		},
	}

	// Start bot
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create bot
	bot, err := telegram.NewTelegramBot(config)
	if err != nil {
		t.Fatalf("Failed to create bot: %v", err)
	}
	defer bot.Disconnect(ctx)

	if err := bot.Connect(ctx); err != nil {
		t.Fatalf("Failed to start bot: %v", err)
	}

	// Wait for bot to be ready
	time.Sleep(2 * time.Second)

	// Check bot status
	info := bot.PlatformInfo()
	t.Logf("Bot info: ID=%s, Name=%s", info.ID, info.Name)

	status := bot.Status()
	t.Logf("Bot status: Connected=%v, Authenticated=%v, Ready=%v",
		status.Connected, status.Authenticated, status.Ready)

	// Send test message if chat ID is provided
	if testChatID != "" {
		t.Logf("Sending test message to chat: %s", testChatID)

		// Build a test message with keyboard
		kb := imbot.NewKeyboardBuilder()
		kb.AddRow(
			imbot.CallbackButton("✅ Allow", imbot.FormatCallbackData("test", "allow", "123")),
			imbot.CallbackButton("❌ Deny", imbot.FormatCallbackData("test", "deny", "123")),
		)
		kb.AddRow(
			imbot.CallbackButton("🔄 Always", imbot.FormatCallbackData("test", "always", "123")),
		)

		msg, err := bot.SendMessage(context.Background(), testChatID, &imbot.SendMessageOptions{
			Text:      "🔐 *Tool Permission Request*\n\nTool: `Bash`\n\nArgs: \n\tcmd: `ls -la...`\n\nReason: test_permission",
			ParseMode: imbot.ParseModeMarkdown,
			Metadata: map[string]interface{}{
				"replyMarkup": imbot.BuildTelegramActionKeyboard(kb.Build()),
			},
		})

		if err != nil {
			t.Logf("Failed to send message: %v", err)
		} else {
			t.Logf("Message sent successfully: ID=%s", msg.MessageID)
		}
	}

	// Listen for updates for a short time
	t.Log("Listening for updates (10 seconds)...")
	updateCount := 0
	messageCount := 0

	time.Sleep(10 * time.Second)

	t.Logf("Test completed. Updates received: %d, Messages: %d", updateCount, messageCount)
}
