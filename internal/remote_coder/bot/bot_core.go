package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/session"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/summarizer"
)

const (
	listSummaryLimit      = 160
	telegramStartRetries  = 10
	telegramStartDelay    = 5 * time.Second
	telegramStartMaxDelay = 5 * time.Minute
)

// Agent routing constants
const (
	agentClaudeCode = agentboot.AgentTypeClaude
	agentMock       = agentboot.AgentTypeMockAgent
)

// Bot command constants
const (
	botCommandHelp    = "help"
	botCommandBind    = "bind"
	botCommandJoin    = "join"
	botCommandProject = "project"
	botCommandStatus  = "status"
	botCommandClear   = "clear"
	botCommandBash    = "bash"
)

var defaultBashAllowlist = map[string]struct{}{
	"cd":  {},
	"ls":  {},
	"pwd": {},
}

// ResponseMeta contains metadata for response formatting
type ResponseMeta struct {
	ProjectPath string
	ChatID      string
	UserID      string
	SessionID   string
}

// runBotWithSettings starts a bot using db.Settings instead of bot.Store
func runBotWithSettings(ctx context.Context, setting BotSetting, dbPath string, sessionMgr *session.Manager, agentBoot *agentboot.AgentBoot) error {
	// Create a temporary bot.Store for chat state management
	store, err := NewStoreForChatOnly(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create chat store: %w", err)
	}
	defer store.Close()

	// Create platform-specific auth config
	authConfig := buildAuthConfig(setting)
	platform := imbot.Platform(setting.Platform)

	if sessionMgr == nil {
		return fmt.Errorf("session manager is nil")
	}

	summaryEngine := summarizer.NewEngine()
	directoryBrowser := NewDirectoryBrowser()

	manager := imbot.NewManager(
		imbot.WithAutoReconnect(true),
		imbot.WithMaxReconnectAttempts(5),
		imbot.WithReconnectDelay(3000),
	)

	options := map[string]interface{}{
		"updateTimeout": 30,
	}
	if setting.ProxyURL != "" {
		options["proxy"] = setting.ProxyURL
	}

	err = manager.AddBot(&imbot.Config{
		UUID:     setting.UUID,
		Platform: platform,
		Enabled:  true,
		Auth:     authConfig,
		Options:  options,
	})
	if err != nil {
		return fmt.Errorf("failed to start %s bot: %w", setting.Platform, err)
	}

	// Register unified message handler with platform parameter
	handler := NewBotHandler(ctx, setting, store.ChatStore(), sessionMgr, agentBoot, summaryEngine, directoryBrowser, manager)
	manager.OnMessage(handler.HandleMessage)

	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start bot manager: %w", err)
	}

	<-ctx.Done()
	return nil
}

// buildAuthConfig creates auth config based on platform
func buildAuthConfig(setting BotSetting) imbot.AuthConfig {
	platform := setting.Platform
	auth := setting.Auth

	switch platform {
	case "telegram", "discord", "slack":
		return imbot.AuthConfig{
			Type:  "token",
			Token: auth["token"],
		}
	case "dingtalk", "feishu":
		return imbot.AuthConfig{
			Type:         "oauth",
			ClientID:     auth["clientId"],
			ClientSecret: auth["clientSecret"],
		}
	case "whatsapp":
		return imbot.AuthConfig{
			Type:      "token",
			Token:     auth["token"],
			AccountID: auth["phoneNumberId"],
		}
	default:
		return imbot.AuthConfig{
			Type:  "token",
			Token: auth["token"],
		}
	}
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// getReplyTarget returns the reply target ID for the message.
// Different platforms may use different IDs:
// - Telegram: Recipient.ID (chat ID)
// - DingTalk/Feishu: Recipient.ID (conversation ID)
// - Discord: Recipient.ID (channel ID)
func getReplyTarget(msg imbot.Message) string {
	return strings.TrimSpace(msg.Recipient.ID)
}

// getProjectPathForGroup retrieves the project path bound to a group chat.
func getProjectPathForGroup(chatStore *ChatStore, chatID string, platform string) (string, bool) {
	if chatStore == nil {
		return "", false
	}
	path, ok, err := chatStore.GetProjectPath(chatID)
	if err != nil {
		return "", false
	}
	return path, ok
}

// normalizeAllowlistToMap converts a string slice to a map for O(1) lookups
func normalizeAllowlistToMap(values []string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, v := range values {
		normalized := strings.ToLower(strings.TrimSpace(v))
		if normalized != "" {
			result[normalized] = struct{}{}
		}
	}
	return result
}

// chunkText splits text into chunks of the specified limit
func chunkText(text string, limit int) []string {
	if limit <= 0 || len(text) <= limit {
		return []string{text}
	}

	var chunks []string
	remaining := text
	for len(remaining) > 0 {
		if len(remaining) <= limit {
			chunks = append(chunks, remaining)
			break
		}
		chunks = append(chunks, remaining[:limit])
		remaining = remaining[limit:]
	}
	return chunks
}

// convertActionKeyboardToTelegram converts imbot.InlineKeyboardMarkup to tgbotapi.InlineKeyboardMarkup
func convertActionKeyboardToTelegram(kb imbot.InlineKeyboardMarkup) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, row := range kb.InlineKeyboard {
		var buttons []tgbotapi.InlineKeyboardButton
		for _, btn := range row {
			tgBtn := tgbotapi.InlineKeyboardButton{
				Text: btn.Text,
			}
			if btn.CallbackData != "" {
				tgBtn.CallbackData = &btn.CallbackData
			}
			if btn.URL != "" {
				tgBtn.URL = &btn.URL
			}
			buttons = append(buttons, tgBtn)
		}
		rows = append(rows, buttons)
	}
	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// NewStoreForChatOnly creates a minimal bot.Store for chat state management only
func NewStoreForChatOnly(dbPath string) (*Store, error) {
	store, err := NewStore(dbPath)
	if err != nil {
		return nil, err
	}
	return store, nil
}
