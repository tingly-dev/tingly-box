//go:build e2e
// +build e2e

package tests

import (
	"context"
	"os"
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/markdown"
	"github.com/tingly-dev/tingly-box/imbot/platform/telegram"
)

// TestE2E_TelegramMarkdownEntity tests entity-based markdown rendering
// Run: go test -tags=e2e -v -run TestE2E_TelegramMarkdownEntity ./imbot/tests/
// Environment variables:
//   - TELEGRAM_BOT_TOKEN: Bot token
//   - TELEGRAM_TEST_CHAT_ID: Chat ID to send test message
func TestE2E_TelegramMarkdownEntity(t *testing.T) {
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

	// Test 1: Simple bold and italic
	t.Run("Bold and Italic", func(t *testing.T) {
		md := "**Bold text** and *italic text*"
		result, err := markdown.Convert(md)
		if err != nil {
			t.Fatalf("Convert error: %v", err)
		}

		entities := markdown.ToIMBotEntities(result.Entities)
		_, err = bot.SendMessage(ctx, chatID, &core.SendMessageOptions{
			Text:     result.Text,
			Entities: entities,
		})
		if err != nil {
			t.Fatalf("SendMessage error: %v", err)
		}
		t.Log("✅ Bold and italic rendered")
	})

	// Test 2: Code and links
	t.Run("Code and Links", func(t *testing.T) {
		md := "Inline `code` and [link](https://example.com)"
		result, err := markdown.Convert(md)
		if err != nil {
			t.Fatalf("Convert error: %v", err)
		}

		entities := markdown.ToIMBotEntities(result.Entities)
		_, err = bot.SendMessage(ctx, chatID, &core.SendMessageOptions{
			Text:     result.Text,
			Entities: entities,
		})
		if err != nil {
			t.Fatalf("SendMessage error: %v", err)
		}
		t.Log("✅ Code and links rendered")
	})

	// Test 3: Code block with language
	t.Run("Code Block", func(t *testing.T) {
		md := "```go\nfunc main() {\n    println(\"hello\")\n}\n```"
		result, err := markdown.Convert(md)
		if err != nil {
			t.Fatalf("Convert error: %v", err)
		}

		entities := markdown.ToIMBotEntities(result.Entities)
		_, err = bot.SendMessage(ctx, chatID, &core.SendMessageOptions{
			Text:     result.Text,
			Entities: entities,
		})
		if err != nil {
			t.Fatalf("SendMessage error: %v", err)
		}
		t.Log("✅ Code block rendered")
	})

	// Test 4: Mixed formatting
	t.Run("Mixed Formatting", func(t *testing.T) {
		md := `**Markdown Test**

*Bold:* **bold text**
*Italic:* _italic text_
*Code:* ` + "`code`" + `
*Strikethrough:* ~~strikethrough~~

Link: [Click here](https://example.com)`

		result, err := markdown.Convert(md)
		if err != nil {
			t.Fatalf("Convert error: %v", err)
		}

		entities := markdown.ToIMBotEntities(result.Entities)
		_, err = bot.SendMessage(ctx, chatID, &core.SendMessageOptions{
			Text:     result.Text,
			Entities: entities,
		})
		if err != nil {
			t.Fatalf("SendMessage error: %v", err)
		}
		t.Log("✅ Mixed formatting rendered")
	})

	// Test 5: Emoji and UTF-16 handling
	t.Run("Emoji UTF-16", func(t *testing.T) {
		md := "**Hello 👍** and *你好 🎉*"
		result, err := markdown.Convert(md)
		if err != nil {
			t.Fatalf("Convert error: %v", err)
		}

		entities := markdown.ToIMBotEntities(result.Entities)
		_, err = bot.SendMessage(ctx, chatID, &core.SendMessageOptions{
			Text:     result.Text,
			Entities: entities,
		})
		if err != nil {
			t.Fatalf("SendMessage error: %v", err)
		}
		t.Log("✅ Emoji and CJK rendered")
	})

	t.Log("✅ All tests passed! Check Telegram to verify rendering")
}
