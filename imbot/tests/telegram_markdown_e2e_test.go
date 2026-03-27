//go:build e2e
// +build e2e

package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/telegram"
)

// TestE2E_TelegramMarkdown tests Telegram markdown rendering
// Run: go test -tags=e2e -v -run TestE2E_TelegramMarkdown ./imbot/tests/
// Environment variables:
//   - TELEGRAM_BOT_TOKEN: Bot token
//   - TELEGRAM_TEST_CHAT_ID: Chat ID to send test message
func TestE2E_TelegramMarkdown(t *testing.T) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		t.Skip("TELEGRAM_BOT_TOKEN not set")
	}

	chatID := os.Getenv("TELEGRAM_TEST_CHAT_ID")
	if chatID == "" {
		t.Skip("TELEGRAM_TEST_CHAT_ID not set")
	}

	// Create bot
	bot, err := telegram.NewTelegramBot(&core.Config{
		Platform: core.PlatformTelegram,
		Enabled:  true,
		Auth:     core.AuthConfig{Type: "token", Token: token},
	})
	if err != nil {
		t.Fatalf("Failed to create bot: %v", err)
	}

	ctx := context.Background()
	defer bot.Disconnect(ctx)

	if err := bot.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Test markdown message
	markdown := `**Markdown Test**

*Bold:* **bold text**
*Italic:* _italic text_
*Code:* ` + "`code`" + `

Code block:
` + "```" + `
func main() {
    println("hello")
}
` + "```" + `

Special chars: test_file.txt, 2 + 2 = 4`

	result, err := bot.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
		Text:      markdown,
		ParseMode: imbot.ParseModeMarkdown,
	})

	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	t.Logf("✅ Message sent: %s", result.MessageID)
	t.Log("Check Telegram: markdown should be rendered (not raw text)")
}
