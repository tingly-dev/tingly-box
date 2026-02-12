package bot

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/launcher"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/session"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/summarizer"
)

const (
	telegramMessageLimit  = 4000
	listSummaryLimit      = 160
	telegramStartRetries  = 10
	telegramStartDelay    = 5 * time.Second
	telegramStartMaxDelay = 5 * time.Minute
)

// Agent routing constants
const (
	agentClaudeCode = "claude_code"
)

// agentPatterns maps agent aliases to their internal identifier
var agentPatterns = map[string]string{
	"@claude": agentClaudeCode,
	"@cc":     agentClaudeCode,
}

// agentCommands maps command aliases to their internal identifier
var agentCommands = map[string]string{
	"/claude": agentClaudeCode,
	"/cc":     agentClaudeCode,
}

var defaultBashAllowlist = map[string]struct{}{
	"cd":  {},
	"ls":  {},
	"pwd": {},
}

// RunTelegramBot starts a Telegram bot that proxies messages to remote-coder sessions.
func RunTelegramBot(ctx context.Context, store *Store, sessionMgr *session.Manager) error {
	delay := telegramStartDelay
	for attempt := 1; attempt <= telegramStartRetries; attempt++ {
		if ctx.Err() != nil {
			return nil
		}
		if err := runTelegramBotOnce(ctx, store, sessionMgr); err != nil {
			if attempt == telegramStartRetries {
				return err
			}
			logrus.WithError(err).Warnf("Remote-coder Telegram bot failed to start; retrying in %s (%d/%d)", delay, attempt, telegramStartRetries)
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

func runTelegramBotOnce(ctx context.Context, store *Store, sessionMgr *session.Manager) error {
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
		// Check for agent commands (/cc, /claude) first
		if agent, msgText, matched := parseAgentCommand(text); matched {
			handleAgentMessage(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, agent, msgText, msg.Sender.ID)
			return
		}
		handleTelegramCommand(ctx, bot, store, sessionMgr, chatID, text, msg.Sender.ID)
		return
	}

	// Check for @agent mention pattern
	if agent, msgText := parseAgentMention(text); agent != "" {
		handleAgentMessage(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, agent, msgText, msg.Sender.ID)
		return
	}

	// No agent mentioned - check if there's an active session to auto-route to cc
	sessionID, ok, err := store.GetSessionForChat(chatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}
	if ok && sessionID != "" {
		// Has active session, auto-route to cc
		handleAgentMessage(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, agentClaudeCode, text, msg.Sender.ID)
		return
	}

	// No session - show guidance
	sendText(bot, chatID, "No active session. Use /new <project_path> to create one, then just send messages directly.")
}

// parseAgentMention checks if text starts with @agent pattern and returns the agent and remaining message.
func parseAgentMention(text string) (agent string, message string) {
	text = strings.TrimSpace(text)
	for pattern, agentID := range agentPatterns {
		if strings.HasPrefix(text, pattern) {
			remaining := strings.TrimSpace(strings.TrimPrefix(text, pattern))
			return agentID, remaining
		}
	}
	return "", ""
}

// parseAgentCommand checks if text is an agent command (e.g., /cc <message>) and returns the agent and message.
func parseAgentCommand(text string) (agent string, message string, matched bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", "", false
	}
	cmd := strings.ToLower(fields[0])
	if agentID, ok := agentCommands[cmd]; ok {
		remaining := strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
		return agentID, remaining, true
	}
	return "", "", false
}

// handleAgentMessage routes message to the appropriate agent handler.
func handleAgentMessage(
	ctx context.Context,
	bot imbot.Bot,
	store *Store,
	sessionMgr *session.Manager,
	ccLauncher *launcher.ClaudeCodeLauncher,
	summaryEngine *summarizer.Engine,
	chatID string,
	agent string,
	text string,
	senderID string,
) {
	logrus.WithFields(logrus.Fields{
		"agent":    agent,
		"chatID":   chatID,
		"senderID": senderID,
	}).Infof("Agent call: %s", text)

	switch agent {
	case agentClaudeCode:
		handleClaudeCodeMessage(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, text, senderID)
	default:
		sendText(bot, chatID, fmt.Sprintf("Unknown agent: %s", agent))
	}
}

// handleClaudeCodeMessage executes a message through Claude Code.
func handleClaudeCodeMessage(
	ctx context.Context,
	bot imbot.Bot,
	store *Store,
	sessionMgr *session.Manager,
	ccLauncher *launcher.ClaudeCodeLauncher,
	summaryEngine *summarizer.Engine,
	chatID string,
	text string,
	senderID string,
) {
	if strings.TrimSpace(text) == "" {
		sendText(bot, chatID, "Please provide a message for Claude Code. Usage: /cc <message> or @cc <message>")
		return
	}

	sessionID, ok, err := store.GetSessionForChat(chatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}
	if !ok || sessionID == "" {
		sendText(bot, chatID, "No session mapped. Use /new <project_path> or /use <session_id> first.")
		return
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
	projectPath := ""
	if sess != nil && sess.Context != nil {
		if v, ok := sess.Context["project_path"]; ok {
			if pv, ok := v.(string); ok {
				projectPath = strings.TrimSpace(pv)
			}
		}
	}
	if projectPath == "" {
		sendText(bot, chatID, "Project path is required. Use /new <project_path> or /bash cd <path>.")
		return
	}

	sessionMgr.AppendMessage(sessionID, session.Message{
		Role:      "user",
		Content:   text,
		Timestamp: time.Now(),
	})

	sessionMgr.SetRunning(sessionID)

	execCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	result, err := ccLauncher.Execute(execCtx, text, launcher.ExecuteOptions{
		ProjectPath: projectPath,
	})
	response := result.Output
	if err != nil && result.Error != "" {
		response = result.Error
	}

	if err != nil {
		sessionMgr.SetFailed(sessionID, response)
		logrus.WithError(err).Warn("Remote-coder execution failed")
		sendText(bot, chatID, formatResponseWithMeta(projectPath, sessionID, senderID, response))
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

	sendText(bot, chatID, formatResponseWithMeta(projectPath, sessionID, senderID, response))
}

func handleTelegramCommand(ctx context.Context, bot imbot.Bot, store *Store, sessionMgr *session.Manager, chatID string, text string, senderID string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return
	}
	cmd := strings.ToLower(fields[0])

	// Check for agent commands (/claude, /cc)
	if _, ok := agentCommands[cmd]; ok {
		// Agent commands need special handling with launcher
		// Fall through to switch for now, will be handled in default case
	}

	switch cmd {
	case "/help", "/start":
		helpText := fmt.Sprintf(`Your User ID: %s

Available commands:
/help - Show this help message
/cc <message> - Send message to Claude Code
/info - Show current session info
/status - Show current task status
/list - List all sessions
/use <session_id> - Switch to a session
/new <project_path> - Create a new session
/bash <cmd> - Execute allowed bash commands (cd, ls, pwd)`, senderID)
		sendText(bot, chatID, helpText)
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
	case "/status":
		sessionID, ok, err := store.GetSessionForChat(chatID)
		if err != nil {
			logrus.WithError(err).Warn("Failed to load session mapping")
		}
		if !ok || sessionID == "" {
			sendText(bot, chatID, "No session mapped. Use /new <project_path> to create one.")
			return
		}
		sess, exists := sessionMgr.GetOrLoad(sessionID)
		if !exists {
			sendText(bot, chatID, "Session not found.")
			return
		}

		// Build status message
		var statusParts []string
		statusParts = append(statusParts, fmt.Sprintf("Session: %s", sessionID))
		statusParts = append(statusParts, fmt.Sprintf("Status: %s", sess.Status))

		// Show running duration if running
		if sess.Status == session.StatusRunning {
			runningFor := time.Since(sess.LastActivity).Round(time.Second)
			statusParts = append(statusParts, fmt.Sprintf("Running for: %s", runningFor))
		}

		// Show current request if any
		if sess.Request != "" {
			reqPreview := sess.Request
			if len(reqPreview) > 100 {
				reqPreview = reqPreview[:100] + "..."
			}
			statusParts = append(statusParts, fmt.Sprintf("Current task: %s", reqPreview))
		}

		// Show project path
		if sess.Context != nil {
			if v, ok := sess.Context["project_path"]; ok {
				if pv, ok := v.(string); ok {
					statusParts = append(statusParts, fmt.Sprintf("Project: %s", pv))
				}
			}
		}

		// Show error if failed
		if sess.Status == session.StatusFailed && sess.Error != "" {
			errPreview := sess.Error
			if len(errPreview) > 100 {
				errPreview = errPreview[:100] + "..."
			}
			statusParts = append(statusParts, fmt.Sprintf("Error: %s", errPreview))
		}

		sendText(bot, chatID, strings.Join(statusParts, "\n"))
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
	case "/bash":
		handleBashCommand(ctx, bot, store, sessionMgr, chatID, fields)
	default:
		sendText(bot, chatID, "Unknown command. Use /help to see available commands.")
	}
}

func handleBashCommand(ctx context.Context, bot imbot.Bot, store *Store, sessionMgr *session.Manager, chatID string, fields []string) {
	if len(fields) < 2 {
		sendText(bot, chatID, "Usage: /bash <command>")
		return
	}
	settings, err := store.GetSettings()
	if err != nil {
		logrus.WithError(err).Warn("Failed to load bot settings")
	}
	allowlist := normalizeAllowlistToMap(settings.BashAllowlist)
	if len(allowlist) == 0 {
		allowlist = defaultBashAllowlist
	}
	subcommand := strings.ToLower(strings.TrimSpace(fields[1]))
	if _, ok := allowlist[subcommand]; !ok {
		sendText(bot, chatID, "Command not allowed.")
		return
	}

	sessionID, ok, err := store.GetSessionForChat(chatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load session mapping")
	}
	var sess *session.Session
	if ok && sessionID != "" {
		if s, exists := sessionMgr.GetOrLoad(sessionID); exists {
			sess = s
		}
	}
	projectPath := ""
	if sess != nil && sess.Context != nil {
		if v, ok := sess.Context["project_path"]; ok {
			if pv, ok := v.(string); ok {
				projectPath = pv
			}
		}
	}
	bashCwd, _, err := store.GetBashCwd(chatID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load bash cwd")
	}
	baseDir := bashCwd
	if baseDir == "" {
		baseDir = projectPath
	}

	switch subcommand {
	case "pwd":
		if baseDir == "" {
			cwd, err := os.Getwd()
			if err != nil {
				sendText(bot, chatID, "Unable to resolve working directory.")
				return
			}
			sendText(bot, chatID, cwd)
			return
		}
		sendText(bot, chatID, baseDir)
	case "cd":
		if len(fields) < 3 {
			sendText(bot, chatID, "Usage: /bash cd <path>")
			return
		}
		nextPath := strings.TrimSpace(strings.Join(fields[2:], " "))
		if nextPath == "" {
			sendText(bot, chatID, "Usage: /bash cd <path>")
			return
		}
		cdBase := baseDir
		if cdBase == "" {
			cwd, err := os.Getwd()
			if err != nil {
				sendText(bot, chatID, "Unable to resolve working directory.")
				return
			}
			cdBase = cwd
		}
		if !filepath.IsAbs(nextPath) {
			nextPath = filepath.Join(cdBase, nextPath)
		}
		if stat, err := os.Stat(nextPath); err != nil || !stat.IsDir() {
			sendText(bot, chatID, "Directory not found.")
			return
		}
		absPath, err := filepath.Abs(nextPath)
		if err == nil {
			nextPath = absPath
		}
		if err := store.SetBashCwd(chatID, nextPath); err != nil {
			logrus.WithError(err).Warn("Failed to update bash cwd")
		}
		sendText(bot, chatID, fmt.Sprintf("Bash working directory set to %s", nextPath))
	case "ls":
		if baseDir == "" {
			cwd, err := os.Getwd()
			if err != nil {
				sendText(bot, chatID, "Unable to resolve working directory.")
				return
			}
			baseDir = cwd
		}
		var args []string
		if len(fields) > 2 {
			args = append(args, fields[2:]...)
		}
		execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(execCtx, "ls", args...)
		cmd.Dir = baseDir
		output, err := cmd.CombinedOutput()
		if err != nil && len(output) == 0 {
			sendText(bot, chatID, fmt.Sprintf("Command failed: %v", err))
			return
		}
		sendText(bot, chatID, strings.TrimSpace(string(output)))
	default:
		sendText(bot, chatID, "Command not allowed.")
	}
}

func normalizeAllowlistToMap(values []string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, entry := range values {
		entry = strings.TrimSpace(strings.ToLower(entry))
		if entry == "" {
			continue
		}
		out[entry] = struct{}{}
	}
	return out
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

// formatResponseWithMeta adds project/session/user metadata to the response for better readability.
func formatResponseWithMeta(projectPath, sessionID, userID, response string) string {
	var meta strings.Builder
	meta.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
	if projectPath != "" {
		// Show only the last 2 directories for brevity
		shortPath := projectPath
		parts := strings.Split(projectPath, string(filepath.Separator))
		if len(parts) > 2 {
			shortPath = filepath.Join(parts[len(parts)-2], parts[len(parts)-1])
		}
		meta.WriteString(fmt.Sprintf("📁 %s\n", shortPath))
	}
	if sessionID != "" {
		meta.WriteString(fmt.Sprintf("🔄 %s\n", sessionID))
	}
	if userID != "" {
		meta.WriteString(fmt.Sprintf("👤 %s\n", userID))
	}
	meta.WriteString("━━━━━━━━━━━━━━━━━━━━\n\n")
	return meta.String() + response
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
