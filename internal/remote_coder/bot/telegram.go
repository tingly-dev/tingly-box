package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/launcher"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/session"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/summarizer"
)

const (
	telegramMessageLimit = 4000
)

// RunTelegramBot starts a Telegram bot that proxies messages to remote-coder sessions.
func RunTelegramBot(ctx context.Context, store *Store, sessionMgr *session.Manager) error {
	if store == nil {
		return fmt.Errorf("bot store is nil")
	}

	settings, err := store.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to load bot settings: %w", err)
	}
	if strings.TrimSpace(settings.Token) == "" {
		return fmt.Errorf("telegram bot token is not configured")
	}

	if sessionMgr == nil {
		return fmt.Errorf("session manager is nil")
	}

	claudeLauncher := launcher.NewClaudeCodeLauncher()
	summaryEngine := summarizer.NewEngine()

	manager := imbot.NewManager(
		imbot.WithAutoReconnect(true),
		imbot.WithMaxReconnectAttempts(5),
		imbot.WithReconnectDelay(3000),
	)

	err = manager.AddBot(&imbot.Config{
		Platform: imbot.PlatformTelegram,
		Enabled:  true,
		Auth: imbot.AuthConfig{
			Type:  "token",
			Token: settings.Token,
		},
		Options: map[string]interface{}{
			"updateTimeout": 30,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to start telegram bot: %w", err)
	}

	manager.OnMessage(func(msg imbot.Message, platform imbot.Platform) {
		if platform != imbot.PlatformTelegram {
			return
		}
		go handleTelegramMessage(ctx, manager, store, sessionMgr, claudeLauncher, summaryEngine, msg)
	})

	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start bot manager: %w", err)
	}

	<-ctx.Done()
	return nil
}

func handleTelegramMessage(
	ctx context.Context,
	manager *imbot.Manager,
	store *Store,
	sessionMgr *session.Manager,
	ccLauncher *launcher.ClaudeCodeLauncher,
	summaryEngine *summarizer.Engine,
	msg imbot.Message,
) {
	bot := manager.GetBot(imbot.PlatformTelegram)
	if bot == nil {
		return
	}

	chatID := strings.TrimSpace(msg.Recipient.ID)
	if chatID == "" {
		return
	}

	allowed, err := store.IsAllowed(chatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to check allowlist")
	}
	if !allowed {
		sendText(bot, chatID, fmt.Sprintf("Access denied. Chat ID %s is not allowlisted.", chatID))
		return
	}

	if !msg.IsTextContent() {
		sendText(bot, chatID, "Only text messages are supported.")
		return
	}

	text := strings.TrimSpace(msg.GetText())
	if text == "" {
		return
	}

	sessionID, ok, err := store.GetSessionForChat(chatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}

	var sess *session.Session
	if ok {
		if s, exists := sessionMgr.GetOrLoad(sessionID); exists {
			sess = s
		}
	}

	if sess == nil || sess.Status == session.StatusExpired || sess.Status == session.StatusClosed || sess.ExpiresAt.Before(time.Now()) {
		sess = sessionMgr.Create()
		sessionID = sess.ID
		_ = store.SetSessionForChat(chatID, sessionID)
		sessionMgr.SetRequest(sessionID, text)
	}

	sessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "user",
		Content:   text,
		Timestamp: time.Now(),
	})

	sessionMgr.SetRunning(sessionID)

	execCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	result, err := ccLauncher.Execute(execCtx, text, launcher.ExecuteOptions{})
	response := result.Output
	if err != nil && result.Error != "" {
		response = result.Error
	}

	if err != nil {
		sessionMgr.SetFailed(sessionID, response)
		logrus.WithError(err).Warn("Remote-coder execution failed")
		sendText(bot, chatID, response)
		return
	}

	sessionMgr.SetCompleted(sessionID, response)

	summary := summaryEngine.Summarize(response)
	sessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "assistant",
		Content:   response,
		Summary:   summary,
		Timestamp: time.Now(),
	})

	sendText(bot, chatID, response)
}

func sendText(bot imbot.Bot, chatID string, text string) {
	for _, chunk := range chunkText(text, telegramMessageLimit) {
		_, err := bot.SendText(context.Background(), chatID, chunk)
		if err != nil {
			logrus.WithError(err).Warn("Failed to send telegram message")
			return
		}
	}
}

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
