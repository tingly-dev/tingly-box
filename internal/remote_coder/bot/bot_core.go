package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/permission"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
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
	agentClaudeCode = "claude_code"
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

// RunBot starts a multi-platform bot that proxies messages to remote-coder sessions.
func RunBot(ctx context.Context, store *Store, sessionMgr *session.Manager, agentBoot *agentboot.AgentBoot, permHandler permission.Handler) error {
	delay := telegramStartDelay
	for attempt := 1; attempt <= telegramStartRetries; attempt++ {
		if ctx.Err() != nil {
			return nil
		}
		if err := runBotOnce(ctx, store, sessionMgr, agentBoot, permHandler); err != nil {
			if attempt == telegramStartRetries {
				return err
			}
			logrus.WithError(err).Warnf("Remote-coder bot failed to start; retrying in %s (%d/%d)", delay, attempt, telegramStartRetries)
			if !sleepWithContext(ctx, delay) {
				return nil
			}
			delay *= 2
			if delay > telegramStartMaxDelay {
				delay = telegramStartMaxDelay
			}
			continue
		}
		return nil
	}
	return nil
}

func runBotOnce(ctx context.Context, store *Store, sessionMgr *session.Manager, agentBoot *agentboot.AgentBoot, permHandler permission.Handler) error {
	if store == nil {
		return fmt.Errorf("bot store is nil")
	}

	settings, err := store.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to load bot settings: %w", err)
	}
	if strings.TrimSpace(settings.Token) == "" {
		return fmt.Errorf("bot token is not configured")
	}
	platform := strings.TrimSpace(settings.Platform)
	if platform == "" {
		platform = "telegram"
	}
	if platform != "telegram" {
		return fmt.Errorf("unsupported bot platform: %s", platform)
	}

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
	if strings.TrimSpace(settings.ProxyURL) != "" {
		options["proxy"] = strings.TrimSpace(settings.ProxyURL)
	}

	err = manager.AddBot(&imbot.Config{
		Platform: imbot.Platform(platform),
		Enabled:  true,
		Auth: imbot.AuthConfig{
			Type:  "token",
			Token: settings.Token,
		},
		Options: options,
	})
	if err != nil {
		return fmt.Errorf("failed to start %s bot: %w", platform, err)
	}

	// Register unified message handler with platform parameter
	handler := NewBotHandler(ctx, store, sessionMgr, agentBoot, permHandler, summaryEngine, directoryBrowser, manager)
	manager.OnMessage(handler.HandleMessage)

	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start bot manager: %w", err)
	}

	<-ctx.Done()
	return nil
}

// runBotWithSettings starts a bot using db.Settings instead of bot.Store
func runBotWithSettings(ctx context.Context, settings db.Settings, dbPath string, sessionMgr *session.Manager, agentBoot *agentboot.AgentBoot, permHandler permission.Handler) error {
	// Create a temporary bot.Store for chat state management
	store, err := NewStoreForChatOnly(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create chat store: %w", err)
	}
	defer store.Close()

	// Convert db.Settings to the legacy Settings format
	botSettings := Settings{
		UUID:          settings.UUID,
		Name:          settings.Name,
		Token:         settings.Auth["token"],
		Platform:      settings.Platform,
		AuthType:      settings.AuthType,
		Auth:          settings.Auth,
		ProxyURL:      settings.ProxyURL,
		ChatIDLock:    settings.ChatIDLock,
		BashAllowlist: settings.BashAllowlist,
		Enabled:       settings.Enabled,
	}

	if err := store.SaveSettings(botSettings); err != nil {
		return fmt.Errorf("failed to set bot settings: %w", err)
	}

	// Create platform-specific auth config
	authConfig := buildAuthConfig(settings)
	platform := imbot.Platform(settings.Platform)

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
	if settings.ProxyURL != "" {
		options["proxy"] = settings.ProxyURL
	}

	err = manager.AddBot(&imbot.Config{
		Platform: platform,
		Enabled:  true,
		Auth:     authConfig,
		Options:  options,
	})
	if err != nil {
		return fmt.Errorf("failed to start %s bot: %w", settings.Platform, err)
	}

	// Register unified message handler with platform parameter
	handler := NewBotHandler(ctx, store, sessionMgr, agentBoot, permHandler, summaryEngine, directoryBrowser, manager)
	manager.OnMessage(handler.HandleMessage)

	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start bot manager: %w", err)
	}

	<-ctx.Done()
	return nil
}

// buildAuthConfig creates auth config based on platform
func buildAuthConfig(settings db.Settings) imbot.AuthConfig {
	platform := settings.Platform
	auth := settings.Auth

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

// RunBotWithSettingsOnly runs a bot using only the settings
func RunBotWithSettingsOnly(ctx context.Context, settings Settings, store *Store, sessionMgr *session.Manager, agentBoot *agentboot.AgentBoot, permHandler permission.Handler) error {
	if err := store.SaveSettings(settings); err != nil {
		return fmt.Errorf("failed to save bot settings: %w", err)
	}
	return runBotOnce(ctx, store, sessionMgr, agentBoot, permHandler)
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
func getProjectPathForGroup(store *Store, chatID string, platform string) (string, bool) {
	if store == nil || store.ChatStore() == nil {
		return "", false
	}
	path, ok, err := store.ChatStore().GetProjectPath(chatID)
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
