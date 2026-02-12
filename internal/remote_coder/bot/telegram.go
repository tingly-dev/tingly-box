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
	listSummaryLimit     = 160
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

	claudeLauncher := launcher.NewClaudeCodeLauncher()
	summaryEngine := summarizer.NewEngine()

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
		Platform: imbot.PlatformTelegram,
		Enabled:  true,
		Auth: imbot.AuthConfig{
			Type:  "token",
			Token: settings.Token,
		},
		Options: options,
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

	settings, err := store.GetSettings()
	if err != nil {
		logrus.WithError(err).Warn("Failed to load bot settings")
	}
	if settings.ChatIDLock != "" && chatID != settings.ChatIDLock {
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

	if strings.HasPrefix(text, "/") {
		handleTelegramCommand(bot, store, sessionMgr, chatID, text)
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

func handleTelegramCommand(bot imbot.Bot, store *Store, sessionMgr *session.Manager, chatID string, text string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return
	}
	cmd := strings.ToLower(fields[0])

	switch cmd {
	case "/info":
		sessionID, ok, err := store.GetSessionForChat(chatID)
		if err != nil {
			logrus.WithError(err).Warn("Failed to load session mapping")
		}
		if !ok || sessionID == "" {
			sendText(bot, chatID, "No session mapped. Send a message or use /new to create one.")
			return
		}
		projectPath := ""
		summary := ""
		if sess, exists := sessionMgr.GetOrLoad(sessionID); exists && sess.Context != nil {
			if v, ok := sess.Context["project_path"]; ok {
				if pv, ok := v.(string); ok {
					projectPath = pv
				}
			}
			summary = lastAssistantSummary(sessionMgr, sessionID)
		}
		if projectPath == "" {
			projectPath = "(none)"
		}
		if summary == "" {
			summary = "(no assistant summary yet)"
		}
		sendText(bot, chatID, fmt.Sprintf("Session: %s\nProject Path: %s\nLast Summary: %s", sessionID, projectPath, summary))
	case "/list":
		sessions := sessionMgr.List()
		if len(sessions) == 0 {
			sendText(bot, chatID, "No sessions available.")
			return
		}
		lines := make([]string, 0, len(sessions)+1)
		lines = append(lines, "Sessions:")
		for _, sess := range sessions {
			projectPath := ""
			if sess.Context != nil {
				if v, ok := sess.Context["project_path"]; ok {
					if pv, ok := v.(string); ok {
						projectPath = pv
					}
				}
			}
			summary := lastAssistantSummary(sessionMgr, sess.ID)
			if summary == "" {
				summary = "(no assistant summary yet)"
			}
			pathLabel := projectPath
			if pathLabel == "" {
				pathLabel = "(none)"
			}
			lines = append(lines, fmt.Sprintf("- %s [%s] %s: %s", sess.ID, sess.Status, pathLabel, summary))
		}
		sendText(bot, chatID, strings.Join(lines, "\n"))
	case "/use":
		if len(fields) < 2 {
			sendText(bot, chatID, "Usage: /use <session_id>")
			return
		}
		targetID := strings.TrimSpace(fields[1])
		if targetID == "" {
			sendText(bot, chatID, "Usage: /use <session_id>")
			return
		}
		if _, exists := sessionMgr.GetOrLoad(targetID); !exists {
			sendText(bot, chatID, "Session not found.")
			return
		}
		if err := store.SetSessionForChat(chatID, targetID); err != nil {
			logrus.WithError(err).Warn("Failed to update session mapping")
			sendText(bot, chatID, "Failed to switch session.")
			return
		}
		sendText(bot, chatID, fmt.Sprintf("Switched to session %s.", targetID))
	case "/new":
		if len(fields) < 2 {
			sendText(bot, chatID, "Usage: /new <project_path>")
			return
		}
		projectPath := strings.TrimSpace(strings.Join(fields[1:], " "))
		if projectPath == "" {
			sendText(bot, chatID, "Usage: /new <project_path>")
			return
		}
		sess := sessionMgr.Create()
		sessionMgr.SetContext(sess.ID, "project_path", projectPath)
		if err := store.SetSessionForChat(chatID, sess.ID); err != nil {
			logrus.WithError(err).Warn("Failed to update session mapping")
			sendText(bot, chatID, "Failed to create new session.")
			return
		}
		sendText(bot, chatID, fmt.Sprintf("New session created: %s", sess.ID))
	default:
		sendText(bot, chatID, "Unknown command. Try /info, /list, /use <session_id>, /new.")
	}
}

func lastAssistantSummary(sessionMgr *session.Manager, sessionID string) string {
	if sessionMgr == nil {
		return ""
	}
	msgs, ok := sessionMgr.GetMessages(sessionID)
	if !ok {
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		if msg.Role != "assistant" {
			continue
		}
		text := strings.TrimSpace(msg.Summary)
		if text == "" {
			text = strings.TrimSpace(msg.Content)
		}
		if text == "" {
			return ""
		}
		if len(text) > listSummaryLimit {
			return text[:listSummaryLimit] + "..."
		}
		return text
	}
	return ""
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
