//go:build e2e
// +build e2e

package feishu

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/interaction"
)

// TestE2E_FeishuBot_RealBot creates a real Feishu bot for debugging purposes
//
// Run with: go test -tags=e2e -v -run TestE2E_FeishuBot_RealBot ./imbot/internal/platform/feishu/
//
// Required environment variables:
//   - FEISHU_APP_ID: Your Feishu/Lark App ID
//   - FEISHU_APP_SECRET: Your Feishu/Lark App Secret
//
// Optional environment variables:
//   - FEISHU_TEST_CHAT_ID: User or group chat ID to send test messages to
//   - FEISHU_DOMAIN: Domain to test (feishu or lark, defaults to feishu)
//
// How to get credentials:
// 1. Feishu: Visit https://open.feishu.cn/
//   - Create a new app and enable Bot capability
//   - Get App ID and App Secret from "Credentials & Basic Info"
//
// 2. Lark: Visit https://open.larksuite.com/
//   - Create a new app and enable Bot capability
//   - Get App ID and App Secret from "Credentials & Basic Info"
//
// Interactive commands (after bot starts):
//
//	/status         - Show bot status
//	/stats          - Show message statistics
//	/text <msg>     - Send plain text message (requires FEISHU_TEST_CHAT_ID)
//	/md <msg>       - Send markdown message (requires FEISHU_TEST_CHAT_ID)
//	/card <msg>     - Send interactive card with buttons (requires FEISHU_TEST_CHAT_ID)
//	/keyboard       - Send keyboard with multiple buttons (requires FEISHU_TEST_CHAT_ID)
//	/approve        - Send approval card (bash tool approval simulation)
//	/echo on/off    - Enable/disable auto-echo
//	/help           - Show all available commands
//	/quit or q      - Exit the test
//
// To get chat ID:
// 1. Add the bot to a chat group
// 2. Send a message from the bot to the chat
// 3. The bot will log the incoming message with the chat ID
// 4. Use that chat ID as FEISHU_TEST_CHAT_ID
func TestE2E_FeishuBot_RealBot(t *testing.T) {
	appID := os.Getenv("FEISHU_APP_ID")
	appSecret := os.Getenv("FEISHU_APP_SECRET")
	if appID == "" || appSecret == "" {
		t.Skip("Skipping e2e test: FEISHU_APP_ID and FEISHU_APP_SECRET environment variables not set")
	}

	// Optional: specify a chat ID to send test messages to
	testChatID := os.Getenv("FEISHU_TEST_CHAT_ID")

	// Optional: specify domain (feishu or lark)
	domainStr := os.Getenv("FEISHU_DOMAIN")
	domain := DomainFeishu
	if domainStr == "lark" {
		domain = DomainLark
	}

	platformName := "Feishu"
	if domain == DomainLark {
		platformName = "Lark"
	}

	t.Logf("Testing %s bot with App ID: %s", platformName, appID)

	config := &core.Config{
		Platform: core.Platform(domain),
		Enabled:  true,
		Auth: core.AuthConfig{
			Type:         "oauth",
			ClientID:     appID,
			ClientSecret: appSecret,
		},
	}

	// Create bot with longer timeout for interactive testing
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	// Shared state for echo control (accessible from both message handler and commands)
	echoEnabled := true // Auto-echo is enabled by default

	bot, err := NewBot(config, domain)
	if err != nil {
		t.Fatalf("Failed to create %s bot: %v", platformName, err)
	}
	defer func() {
		bot.StopReceiving(ctx)
		bot.Disconnect(ctx)
	}()

	// Set up message handler to log received messages and echo back
	receivedMessages := 0

	bot.OnMessage(func(msg core.Message) {
		// Recover from any panics in message handler
		defer func() {
			if r := recover(); r != nil {
				t.Logf("❌ PANIC in message handler recovered: %v", r)
			}
		}()

		receivedMessages++
		now := time.Now().Format("15:04:05")

		// Print message header with separator
		t.Logf("\n════════════════════════════════════════════════════════════")
		t.Logf("📨 [MSG #%d] Received at %s", receivedMessages, now)
		t.Logf("────────────────────────────────────────────────────────────")

		// Basic message info
		t.Logf("ID:        %s", msg.ID)
		t.Logf("Timestamp: %d (%s)", msg.Timestamp, time.Unix(msg.Timestamp, 0).Format("2006-01-02 15:04:05"))
		t.Logf("Sender:    %s (display: %s)", msg.Sender.ID, msg.Sender.DisplayName)
		t.Logf("Chat:      %s (type: %s)", msg.Recipient.ID, msg.Recipient.Type)
		t.Logf("ChatType:  %s", msg.ChatType)

		// Content info
		t.Logf("Content:   %T", msg.Content)

		var echoText string
		var shouldSkipEcho bool

		switch content := msg.Content.(type) {
		case *core.TextContent:
			t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			t.Logf("📝 TEXT CONTENT:")
			t.Logf("%s", content.Text)
			if len(content.Entities) > 0 {
				t.Logf("Entities: %d", len(content.Entities))
			}

			// Check for commands in the message
			input := strings.TrimSpace(content.Text)

			// Determine target chat for test commands (test chat or current chat)
			targetChat := testChatID
			if targetChat == "" {
				// No test chat set, use current chat
				targetChat = msg.Recipient.ID
			}

			// Process commands from Feishu/Lark
			switch {
			case input == "/status":
				status := bot.Status()
				echoText = fmt.Sprintf("📊 **Bot Status**\n\nConnected: %v\nAuthenticated: %v\nReady: %v\nLast Activity: %s",
					status.Connected, status.Authenticated, status.Ready,
					time.Unix(status.LastActivity, 0).Format("15:04:05"))
				shouldSkipEcho = true // We'll send our own response

			case input == "/stats":
				echoText = fmt.Sprintf("📈 **Message Statistics**\n\nTotal Received: %d\nAuto-Echo: %v",
					receivedMessages, echoEnabled)
				shouldSkipEcho = true

			case input == "/echo on":
				echoEnabled = true
				echoText = "✅ Auto-echo enabled"
				shouldSkipEcho = true

			case input == "/echo off":
				echoEnabled = false
				echoText = "❌ Auto-echo disabled"
				shouldSkipEcho = true

			case input == "/help":
				helpMsg := "📖 **Available Commands**\n\n" +
					"/status - Show bot status\n" +
					"/stats - Show message statistics\n" +
					"/text <msg> - Send plain text message\n" +
					"/md <msg> - Send markdown message\n" +
					"/card <msg> - Send card with buttons\n" +
					"/keyboard - Send keyboard with buttons\n" +
					"/approve - Send approval card (bash tool)\n" +
					"/echo on/off - Toggle auto-echo\n" +
					"/help - Show this help\n\n" +
					"Any other text will be echoed back."
				echoText = helpMsg
				shouldSkipEcho = true

			case strings.HasPrefix(input, "/text "):
				// For messages sent to the test chat, forward them
				msgText := strings.TrimPrefix(input, "/text ")
				t.Logf("📤 Sending text to %s: %s", targetChat, msgText)
				result, err := bot.SendText(ctx, targetChat, msgText)
				if err != nil {
					echoText = fmt.Sprintf("❌ Failed: %v", err)
				} else {
					echoText = fmt.Sprintf("✅ Sent to %s: ID=%s", targetChat, result.MessageID)
				}
				shouldSkipEcho = true

			case strings.HasPrefix(input, "/md "):
				msgText := strings.TrimPrefix(input, "/md ")
				t.Logf("📤 Sending markdown to %s: %s", targetChat, msgText)
				result, err := bot.SendMessage(ctx, targetChat, &core.SendMessageOptions{
					Text:      msgText,
					ParseMode: core.ParseModeMarkdown,
				})
				if err != nil {
					echoText = fmt.Sprintf("❌ Failed: %v", err)
				} else {
					echoText = fmt.Sprintf("✅ Sent to %s: ID=%s", targetChat, result.MessageID)
				}
				shouldSkipEcho = true

			case strings.HasPrefix(input, "/card "):
				msgText := strings.TrimPrefix(input, "/card ")
				kb := interaction.NewKeyboardBuilder()
				kb.AddRow(
					interaction.CallbackButton("👍 Like", interaction.FormatCallbackData("reaction", "like")),
					interaction.CallbackButton("❤️ Love", interaction.FormatCallbackData("reaction", "love")),
				)
				kb.AddRow(
					interaction.CallbackButton("👎 Dislike", interaction.FormatCallbackData("reaction", "dislike")),
				)

				t.Logf("📤 Sending card to %s: %s", targetChat, msgText)
				result, err := bot.SendMessage(ctx, targetChat, &core.SendMessageOptions{
					Text: msgText,
					Metadata: map[string]interface{}{
						"replyMarkup": kb.Build(),
					},
				})
				if err != nil {
					echoText = fmt.Sprintf("❌ Failed: %v", err)
				} else {
					echoText = fmt.Sprintf("✅ Sent to %s: ID=%s", targetChat, result.MessageID)
				}
				shouldSkipEcho = true

			case input == "/keyboard":
				kb := interaction.NewKeyboardBuilder()
				kb.AddRow(
					interaction.CallbackButton("🔴 Red", interaction.FormatCallbackData("color", "red")),
					interaction.CallbackButton("🟢 Green", interaction.FormatCallbackData("color", "green")),
					interaction.CallbackButton("🔵 Blue", interaction.FormatCallbackData("color", "blue")),
				)
				kb.AddRow(
					interaction.CallbackButton("⬜ White", interaction.FormatCallbackData("color", "white")),
					interaction.CallbackButton("⬛ Black", interaction.FormatCallbackData("color", "black")),
				)
				kb.AddRow(
					interaction.CallbackButton("🟡 Yellow", interaction.FormatCallbackData("color", "yellow")),
				)

				t.Logf("📤 Sending keyboard to %s", targetChat)
				result, err := bot.SendMessage(ctx, targetChat, &core.SendMessageOptions{
					Text: "🎨 **Select a Color**\n\nClick a button below:",
					Metadata: map[string]interface{}{
						"replyMarkup": kb.Build(),
					},
				})
				if err != nil {
					echoText = fmt.Sprintf("❌ Failed: %v", err)
				} else {
					echoText = fmt.Sprintf("✅ Keyboard sent to %s: ID=%s", targetChat, result.MessageID)
				}
				shouldSkipEcho = true

			case input == "/approve":
				kb := interaction.NewKeyboardBuilder()
				kb.AddRow(
					interaction.CallbackButton("✅ Approve", interaction.FormatCallbackData("bash", "approve", "ls -la")),
					interaction.CallbackButton("❌ Reject", interaction.FormatCallbackData("bash", "reject", "ls -la")),
				)

				t.Logf("📤 Sending approval card to %s", targetChat)
				result, err := bot.SendMessage(ctx, targetChat, &core.SendMessageOptions{
					Text: "🔐 **Approval Request**\n\n" +
						"**Tool:** `Bash`\n" +
						"**Command:** `ls -la`\n\n" +
						"This command is not in the allowlist.\n" +
						"Do you approve this action?",
					Metadata: map[string]interface{}{
						"replyMarkup": kb.Build(),
					},
				})
				if err != nil {
					echoText = fmt.Sprintf("❌ Failed: %v", err)
				} else {
					echoText = fmt.Sprintf("✅ Approval card sent to %s: ID=%s", targetChat, result.MessageID)
				}
				shouldSkipEcho = true

			default:
				// Echo back regular text
				echoText = fmt.Sprintf("🔁 Echo: %s", content.Text)
			}

		case *core.MediaContent:
			t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			t.Logf("🖼️  MEDIA CONTENT:")
			t.Logf("Items: %d", len(content.Media))
			for i, media := range content.Media {
				t.Logf("  [%d] Type: %s, URL: %s", i, media.Type, media.URL)
			}
			if content.Caption != "" {
				t.Logf("Caption: %s", content.Caption)
			}
			// Echo back media info
			echoText = fmt.Sprintf("🔁 Echo: Received %d media item(s)", len(content.Media))

		case *core.SystemContent:
			t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			t.Logf("⚙️  SYSTEM CONTENT:")
			t.Logf("EventType: %s", content.EventType)
			t.Logf("Data: %+v", content.Data)
			// Don't echo system messages

		default:
			t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			t.Logf("❓ UNKNOWN CONTENT TYPE")
		}

		// Thread context if available
		if msg.ThreadContext != nil {
			t.Logf("Thread:    %s (parent: %s)", msg.ThreadContext.ID, msg.ThreadContext.ParentMessageID)
		}

		// Metadata
		if len(msg.Metadata) > 0 {
			t.Logf("Metadata keys: %d", len(msg.Metadata))
			// Print raw event for debugging
			if rawEvent, ok := msg.Metadata["raw_lark_event"]; ok {
				t.Logf("Raw event type: %T", rawEvent)
			}
		}

		t.Logf("════════════════════════════════════════════════════════════\n")

		// Send response back to the sender
		if echoText != "" && echoEnabled {
			t.Logf("📤 Sending response to %s...", msg.Recipient.ID)

			// Use SendMessage for commands (with markdown support)
			if shouldSkipEcho {
				result, err := bot.SendMessage(ctx, msg.Recipient.ID, &core.SendMessageOptions{
					Text:      echoText,
					ParseMode: core.ParseModeMarkdown,
				})
				if err != nil {
					t.Logf("❌ Response failed: %v", err)
				} else {
					t.Logf("✅ Response sent: ID=%s", result.MessageID)
				}
			} else {
				// Regular echo
				result, err := bot.SendText(ctx, msg.Recipient.ID, echoText)
				if err != nil {
					t.Logf("❌ Echo failed: %v", err)
				} else {
					t.Logf("✅ Echo sent: ID=%s", result.MessageID)
				}
			}
		}
	})

	// Connect to platform (authentication)
	if err := bot.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect to %s: %v", platformName, err)
	}

	t.Logf("Connected to %s successfully", platformName)

	// Start receiving events via WebSocket long connection
	if err := bot.StartReceiving(ctx); err != nil {
		t.Fatalf("Failed to start receiving: %v", err)
	}
	t.Logf("WebSocket long connection established for %s", platformName)

	// Check bot status
	info := bot.PlatformInfo()
	t.Logf("Bot info: ID=%s, Name=%s", info.ID, info.Name)

	status := bot.Status()
	t.Logf("Bot status: Connected=%v, Authenticated=%v, Ready=%v",
		status.Connected, status.Authenticated, status.Ready)

	// Send test message if chat ID is provided
	if testChatID != "" {
		t.Logf("Sending test message to chat: %s", testChatID)

		// Test 1: Plain text message
		textMsg := fmt.Sprintf("🤖 %s Bot Test\n\nThis is a test message from tingly-box imbot.\nTimestamp: %s",
			platformName, time.Now().Format("2006-01-02 15:04:05"))

		result, err := bot.SendText(ctx, testChatID, textMsg)
		if err != nil {
			t.Logf("Failed to send text message: %v", err)
		} else {
			t.Logf("Text message sent successfully: ID=%s", result.MessageID)
		}

		time.Sleep(1 * time.Second)

		// Test 2: Markdown message
		markdownText := fmt.Sprintf("**%s Bot Test**\n\n*Bold text*\n_Italic text_\n`Code block`\n\nTimestamp: %s",
			platformName, time.Now().Format("2006-01-02 15:04:05"))

		result, err = bot.SendMessage(ctx, testChatID, &core.SendMessageOptions{
			Text:      markdownText,
			ParseMode: core.ParseModeMarkdown,
		})
		if err != nil {
			t.Logf("Failed to send markdown message: %v", err)
		} else {
			t.Logf("Markdown message sent successfully: ID=%s", result.MessageID)
		}

		time.Sleep(1 * time.Second)

		// Test 3: Interactive card with buttons
		kb := interaction.NewKeyboardBuilder()
		kb.AddRow(
			interaction.CallbackButton("✅ Approve", interaction.FormatCallbackData("test", "approve", "123")),
			interaction.CallbackButton("❌ Reject", interaction.FormatCallbackData("test", "reject", "123")),
		)
		kb.AddRow(
			interaction.CallbackButton("🔄 Defer", interaction.FormatCallbackData("test", "defer", "123")),
			interaction.CallbackButton("ℹ️ Info", interaction.FormatCallbackData("test", "info", "123")),
		)

		cardText := fmt.Sprintf("**🔐 Permission Request**\n\nTool: `Bash`\nCommand: `ls -la`\n\nReason: Testing %s interactive card", platformName)

		result, err = bot.SendMessage(ctx, testChatID, &core.SendMessageOptions{
			Text:      cardText,
			ParseMode: core.ParseModeMarkdown,
			Metadata: map[string]interface{}{
				"replyMarkup": kb.Build(),
			},
		})
		if err != nil {
			t.Logf("Failed to send card message: %v", err)
		} else {
			t.Logf("Card message sent successfully: ID=%s", result.MessageID)
		}

		time.Sleep(1 * time.Second)

		// Test 4: Text-only fallback (simulating platforms without keyboard support)
		fallbackText := "**📋 Multiple Choice Test**\n\nTo select an option, reply with the number:\n\n1. Option One\n2. Option Two\n3. Option Three"

		result, err = bot.SendText(ctx, testChatID, fallbackText)
		if err != nil {
			t.Logf("Failed to send fallback text message: %v", err)
		} else {
			t.Logf("Fallback text message sent successfully: ID=%s", result.MessageID)
		}
	}

	// Keep bot running to receive events via WebSocket
	t.Logf("\n"+
		"╔═══════════════════════════════════════════════════════════════╗\n"+
		"║  🤖 %s Bot is RUNNING via WebSocket                       ║\n"+
		"║                                                               ║\n"+
		"║  📬 AUTO-ECHO ENABLED - All messages will be echoed back    ║\n"+
		"║                                                               ║\n"+
		"║  Available commands (type and press Enter):                 ║\n"+
		"║    /status     - Show bot status                             ║\n"+
		"║    /stats      - Show message statistics                     ║\n"+
		"║    /text <msg> - Send plain text message (needs chat ID)     ║\n"+
		"║    /md <msg>   - Send markdown message (needs chat ID)       ║\n"+
		"║    /card <msg> - Send card with buttons (needs chat ID)      ║\n"+
		"║    /keyboard   - Send keyboard with buttons (needs chat ID)  ║\n"+
		"║    /approve    - Send approval card (needs chat ID)          ║\n"+
		"║    /echo on    - Enable auto-echo (default: ON)             ║\n"+
		"║    /echo off   - Disable auto-echo                          ║\n"+
		"║    /help       - Show this help message                      ║\n"+
		"║    /quit or q  - Exit the test                              ║\n"+
		"║                                                               ║\n"+
		"║  Or send messages directly from %s!                 ║\n"+
		"╚═══════════════════════════════════════════════════════════════╝",
		platformName, platformName)

	// Interactive mode - read commands from stdin
	done := make(chan bool)
	scanner := bufio.NewScanner(os.Stdin)

	go func() {
		for scanner.Scan() {
			input := strings.TrimSpace(scanner.Text())
			if input == "" {
				continue
			}

			switch {
			case input == "/quit" || input == "q" || input == "exit":
				t.Logf("👋 Exiting...")
				done <- true
				return

			case input == "/status":
				status := bot.Status()
				t.Logf("\n📊 Bot Status:")
				t.Logf("  Connected:    %v", status.Connected)
				t.Logf("  Authenticated: %v", status.Authenticated)
				t.Logf("  Ready:        %v", status.Ready)
				t.Logf("  Last Activity: %v", time.Unix(status.LastActivity, 0).Format("15:04:05"))

			case input == "/stats":
				t.Logf("\n📈 Message Statistics:")
				t.Logf("  Total Received: %d", receivedMessages)
				t.Logf("  Auto-Echo:      %v", echoEnabled)

			case input == "/echo on":
				echoEnabled = true
				t.Logf("✅ Auto-echo enabled")

			case input == "/echo off":
				echoEnabled = false
				t.Logf("❌ Auto-echo disabled")

			case input == "/help":
				t.Logf("\n📖 Available Commands:")
				t.Logf("  /status         - Show bot status")
				t.Logf("  /stats          - Show message statistics")
				t.Logf("  /text <msg>     - Send plain text message")
				t.Logf("  /md <msg>       - Send markdown message")
				t.Logf("  /card <msg>     - Send card with buttons")
				t.Logf("  /keyboard       - Send keyboard with buttons")
				t.Logf("  /approve        - Send approval card (bash tool)")
				t.Logf("  /echo on/off    - Toggle auto-echo")
				t.Logf("  /help           - Show this help")
				t.Logf("  /quit           - Exit test")

			case input == "/keyboard":
				if testChatID == "" {
					t.Logf("❌ FEISHU_TEST_CHAT_ID not set - cannot send messages")
				} else {
					// Send keyboard with multiple buttons
					kb := interaction.NewKeyboardBuilder()
					kb.AddRow(
						interaction.CallbackButton("🔴 Red", interaction.FormatCallbackData("color", "red")),
						interaction.CallbackButton("🟢 Green", interaction.FormatCallbackData("color", "green")),
						interaction.CallbackButton("🔵 Blue", interaction.FormatCallbackData("color", "blue")),
					)
					kb.AddRow(
						interaction.CallbackButton("⬜ White", interaction.FormatCallbackData("color", "white")),
						interaction.CallbackButton("⬛ Black", interaction.FormatCallbackData("color", "black")),
					)
					kb.AddRow(
						interaction.CallbackButton("🟡 Yellow", interaction.FormatCallbackData("color", "yellow")),
					)

					result, err := bot.SendMessage(ctx, testChatID, &core.SendMessageOptions{
						Text: "🎨 **Select a Color**\n\nClick a button below:",
						Metadata: map[string]interface{}{
							"replyMarkup": kb.Build(),
						},
					})
					if err != nil {
						t.Logf("❌ Failed: %v", err)
					} else {
						t.Logf("✅ Keyboard sent: ID=%s", result.MessageID)
					}
				}

			case input == "/approve":
				if testChatID == "" {
					t.Logf("❌ FEISHU_TEST_CHAT_ID not set - cannot send messages")
				} else {
					// Simulate bash tool approval request
					kb := interaction.NewKeyboardBuilder()
					kb.AddRow(
						interaction.CallbackButton("✅ Approve", interaction.FormatCallbackData("bash", "approve", "ls -la")),
						interaction.CallbackButton("❌ Reject", interaction.FormatCallbackData("bash", "reject", "ls -la")),
					)

					result, err := bot.SendMessage(ctx, testChatID, &core.SendMessageOptions{
						Text: "🔐 **Approval Request**\n\n" +
							"**Tool:** `Bash`\n" +
							"**Command:** `ls -la`\n\n" +
							"This command is not in the allowlist.\n" +
							"Do you approve this action?",
						Metadata: map[string]interface{}{
							"replyMarkup": kb.Build(),
						},
					})
					if err != nil {
						t.Logf("❌ Failed: %v", err)
					} else {
						t.Logf("✅ Approval card sent: ID=%s", result.MessageID)
					}
				}

			case strings.HasPrefix(input, "/text "):
				if testChatID == "" {
					t.Logf("❌ FEISHU_TEST_CHAT_ID not set - cannot send messages")
				} else {
					msgText := strings.TrimPrefix(input, "/text ")
					t.Logf("📤 Sending plain text: %s", msgText)
					result, err := bot.SendText(ctx, testChatID, msgText)
					if err != nil {
						t.Logf("❌ Failed: %v", err)
					} else {
						t.Logf("✅ Sent: ID=%s", result.MessageID)
					}
				}

			case strings.HasPrefix(input, "/md "):
				if testChatID == "" {
					t.Logf("❌ FEISHU_TEST_CHAT_ID not set - cannot send messages")
				} else {
					msgText := strings.TrimPrefix(input, "/md ")
					t.Logf("📤 Sending markdown: %s", msgText)
					result, err := bot.SendMessage(ctx, testChatID, &core.SendMessageOptions{
						Text:      msgText,
						ParseMode: core.ParseModeMarkdown,
					})
					if err != nil {
						t.Logf("❌ Failed: %v", err)
					} else {
						t.Logf("✅ Sent: ID=%s", result.MessageID)
					}
				}

			case strings.HasPrefix(input, "/card "):
				if testChatID == "" {
					t.Logf("❌ FEISHU_TEST_CHAT_ID not set - cannot send messages")
				} else {
					msgText := strings.TrimPrefix(input, "/card ")
					kb := interaction.NewKeyboardBuilder()
					kb.AddRow(
						interaction.CallbackButton("👍 Like", interaction.FormatCallbackData("reaction", "like")),
						interaction.CallbackButton("❤️ Love", interaction.FormatCallbackData("reaction", "love")),
					)
					kb.AddRow(
						interaction.CallbackButton("👎 Dislike", interaction.FormatCallbackData("reaction", "dislike")),
					)

					t.Logf("📤 Sending card: %s", msgText)
					result, err := bot.SendMessage(ctx, testChatID, &core.SendMessageOptions{
						Text: msgText,
						Metadata: map[string]interface{}{
							"replyMarkup": kb.Build(),
						},
					})
					if err != nil {
						t.Logf("❌ Failed: %v", err)
					} else {
						t.Logf("✅ Sent: ID=%s", result.MessageID)
					}
				}

			default:
				t.Logf("❓ Unknown command: %s", input)
				t.Logf("   Type /help for available commands")
			}
		}
	}()

	// Wait for done signal or context timeout
	select {
	case <-done:
		// User requested exit
	case <-ctx.Done():
		t.Logf("\n⏱️  Test timeout reached (60 minutes)")
	}

	t.Logf("\n\n════════════════════════════════════════════════════════════")
	t.Logf("📊 FINAL STATISTICS")
	t.Logf("════════════════════════════════════════════════════════════")
	t.Logf("  Total Messages Received: %d", receivedMessages)
	t.Logf("  Platform:                %s", platformName)
	t.Logf("  Test Duration:            %s", time.Since(startTime).Round(time.Second))
	t.Logf("════════════════════════════════════════════════════════════\n")
	t.Logf("Test completed successfully for %s", platformName)
}

// TestE2E_LarkBot_RealBot creates a real Lark bot for debugging purposes
//
// Run with: go test -tags=e2e -v -run TestE2E_LarkBot_RealBot ./imbot/internal/platform/feishu/
//
// Required environment variables:
//   - LARK_APP_ID: Your Lark App ID
//   - LARK_APP_SECRET: Your Lark App Secret
//
// Optional environment variable:
//   - LARK_TEST_CHAT_ID: User or group chat ID to send test messages to
func TestE2E_LarkBot_RealBot(t *testing.T) {
	appID := os.Getenv("LARK_APP_ID")
	appSecret := os.Getenv("LARK_APP_SECRET")
	if appID == "" || appSecret == "" {
		t.Skip("Skipping e2e test: LARK_APP_ID and LARK_APP_SECRET environment variables not set")
	}

	// Set Feishu env vars for the helper test
	os.Setenv("FEISHU_APP_ID", appID)
	os.Setenv("FEISHU_APP_SECRET", appSecret)
	os.Setenv("FEISHU_DOMAIN", "lark")
	if chatID := os.Getenv("LARK_TEST_CHAT_ID"); chatID != "" {
		os.Setenv("FEISHU_TEST_CHAT_ID", chatID)
	}

	// Run the Feishu test with Lark domain
	TestE2E_FeishuBot_RealBot(t)
}

// Example helper function for manual testing
func Example_feishuAuth() {
	// This example shows how to authenticate with Feishu

	config := &core.Config{
		Platform: core.PlatformFeishu,
		Enabled:  true,
		Auth: core.AuthConfig{
			Type:         "oauth",
			ClientID:     "cli-your-app-id", // Get from Feishu Open Platform
			ClientSecret: "your-app-secret", // Get from Feishu Open Platform
		},
	}

	ctx := context.Background()
	bot, err := NewBot(config, DomainFeishu)
	if err != nil {
		fmt.Printf("Failed to create bot: %v\n", err)
		return
	}
	defer bot.Disconnect(ctx)

	if err := bot.Connect(ctx); err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}

	fmt.Println("Feishu bot connected successfully!")
}

// Example helper for Lark authentication
func Example_larkAuth() {
	// This example shows how to authenticate with Lark

	config := &core.Config{
		Platform: core.PlatformLark,
		Enabled:  true,
		Auth: core.AuthConfig{
			Type:         "oauth",
			ClientID:     "cli-your-app-id", // Get from Lark Open Platform
			ClientSecret: "your-app-secret", // Get from Lark Open Platform
		},
	}

	ctx := context.Background()
	bot, err := NewBot(config, DomainLark)
	if err != nil {
		fmt.Printf("Failed to create bot: %v\n", err)
		return
	}
	defer bot.Disconnect(ctx)

	if err := bot.Connect(ctx); err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}

	fmt.Println("Lark bot connected successfully!")
}
