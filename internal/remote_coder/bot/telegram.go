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

// getReplyTarget returns the reply target ID for the message.
// Different platforms may use different IDs:
// - Telegram: Recipient.ID (chat ID)
// - DingTalk/Feishu: Recipient.ID (conversation ID)
func getReplyTarget(msg imbot.Message) string {
	return strings.TrimSpace(msg.Recipient.ID)
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
	bot := manager.GetBot(msg.Platform)
	if bot == nil {
		return
	}

	// get recipient, different platform may require different source and id
	// Telegram: Recipient.ID (chat ID)
	// DingTalk/Feishu: Recipient.ID (conversation ID)
	chatID := getReplyTarget(msg)
	if chatID == "" {
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

	// Determine if message is from direct chat or group
	isDirectChat := msg.IsDirectMessage()
	isGroupChat := msg.IsGroupMessage()

	if isDirectChat {
		settings, err := store.GetSettings()
		if err != nil {
			logrus.WithError(err).Warn("Failed to load bot settings")
		}
		if settings.ChatIDLock != "" && chatID != settings.ChatIDLock {
			return
		}
	}

	if strings.HasPrefix(text, "/") {
		// Check for agent commands (/cc, /claude) first
		if agent, msgText, matched := parseAgentCommand(text); matched {
			handleAgentMessage(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, agent, msgText, msg.Sender.ID)
			return
		}
		handleTelegramCommand(ctx, bot, store, sessionMgr, chatID, text, msg.Sender.ID, isDirectChat, isGroupChat)
		return
	}

	// Check for @agent mention pattern
	if agent, msgText := parseAgentMention(text); agent != "" {
		handleAgentMessage(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, agent, msgText, msg.Sender.ID)
		return
	}

	// In group chat, check for whitelist and project binding
	if isGroupChat {
		logrus.Infof("Group chat ID: %s", chatID)

		// Check whitelist first
		if !store.IsGroupWhitelisted(chatID) {
			logrus.Debugf("Group %s is not whitelisted, ignoring message", chatID)
			return
		}

		if projectPath, ok := getProjectPathForGroup(store, chatID, string(msg.Platform)); ok {
			// Route to Claude Code with the bound project path
			handleAgentMessageWithProject(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, agentClaudeCode, text, msg.Sender.ID, projectPath)
			return
		}
		// No binding, show guidance
		sendText(bot, chatID, "No project bound to this group. Use /bind <path> to bind a project.")
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
	handleAgentMessageWithProject(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, agent, text, senderID, "")
}

// handleAgentMessageWithProject routes message to the appropriate agent handler with a specific project path.
func handleAgentMessageWithProject(
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
	projectPathOverride string,
) {
	logrus.WithFields(logrus.Fields{
		"agent":    agent,
		"chatID":   chatID,
		"senderID": senderID,
	}).Infof("Agent call: %s", text)

	switch agent {
	case agentClaudeCode:
		handleClaudeCodeMessage(ctx, bot, store, sessionMgr, ccLauncher, summaryEngine, chatID, text, senderID, projectPathOverride)
	default:
		sendText(bot, chatID, fmt.Sprintf("Unknown agent: %s", agent))
	}
}

// getProjectPathForGroup retrieves the project path bound to a group chat.
func getProjectPathForGroup(store *Store, chatID string, platform string) (string, bool) {
	if store == nil || store.DB() == nil {
		return "", false
	}

	bindingStore, err := NewBindingStore(store.DB())
	if err != nil {
		return "", false
	}

	binding, err := bindingStore.GetGroupBinding(chatID, platform)
	if err != nil || binding == nil {
		return "", false
	}

	projectStore, err := NewProjectStore(store.DB())
	if err != nil {
		return "", false
	}

	project, err := projectStore.GetProject(binding.ProjectID)
	if err != nil || project == nil {
		return "", false
	}

	return project.Path, true
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
	projectPathOverride string,
) {
	if strings.TrimSpace(text) == "" {
		sendText(bot, chatID, "Please provide a message for Claude Code. Usage: /cc <message> or @cc <message>")
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

	// Auto-create session for group chats with project override (persistent, no expiration)
	if (sess == nil || sess.Status == session.StatusExpired || sess.Status == session.StatusClosed) && projectPathOverride != "" {
		sess = sessionMgr.Create()
		sessionID = sess.ID
		sessionMgr.SetContext(sessionID, "project_path", projectPathOverride)
		// Clear expiration for group sessions - they should be persistent
		sessionMgr.Update(sessionID, func(s *session.Session) {
			s.ExpiresAt = time.Time{} // Zero value means no expiration
		})
		_ = store.SetSessionForChat(chatID, sessionID)
	}

	if !ok || sessionID == "" {
		sendText(bot, chatID, "No session mapped. Use /new <project_path> or /use <session_id> first.")
		return
	}

	// Refresh session activity for group chats to keep them alive
	if projectPathOverride != "" && sess != nil {
		sessionMgr.Update(sessionID, func(s *session.Session) {
			s.LastActivity = time.Now()
		})
	}

	// Use override project path if provided, otherwise get from session context
	projectPath := projectPathOverride
	if projectPath == "" && sess != nil && sess.Context != nil {
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

	// Use context.Background() to avoid cancellation when bot reconnects
	// The 10-minute timeout is sufficient to prevent runaway executions
	execCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := ccLauncher.Execute(execCtx, text, launcher.ExecuteOptions{
		ProjectPath: projectPath,
	})
	response := ""
	if result != nil {
		response = result.Output
		if err != nil && result.Error != "" {
			response = result.Error
		}
	} else if err != nil {
		response = fmt.Sprintf("Execution failed: %v", err)
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

func handleTelegramCommand(ctx context.Context, bot imbot.Bot, store *Store, sessionMgr *session.Manager, chatID string, text string, senderID string, isDirectChat bool, isGroupChat bool) {
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
/join <invite_link> - Join a group and add to whitelist (direct chat only)
/bind <path> - Bind a project (creates group in direct chat, rebinds in group)
/project - Show current project info
/projects - List your bound projects
/info - Show current session info
/status - Show current task status
/list - List all sessions
/use <session_id> - Switch to a session
/new <project_path> - Create a new session
/bash <cmd> - Execute allowed bash commands (cd, ls, pwd)`, senderID)
		sendText(bot, chatID, helpText)
	case "/join":
		handleJoinCommand(bot, store, chatID, fields, senderID, isDirectChat)
	case "/bind":
		handleBindCommand(ctx, bot, store, chatID, fields, senderID, isDirectChat, isGroupChat)
	case "/project":
		handleProjectCommand(bot, store, chatID, string(imbot.PlatformTelegram))
	case "/projects":
		handleProjectsCommand(bot, store, chatID, senderID, string(imbot.PlatformTelegram))
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

// handleJoinCommand handles the /join command to add a group to whitelist
func handleJoinCommand(bot imbot.Bot, store *Store, chatID string, fields []string, senderID string, isDirectChat bool) {
	if !isDirectChat {
		sendText(bot, chatID, "/join can only be used in direct chat.")
		return
	}

	if len(fields) < 2 {
		sendText(bot, chatID, "Usage: /join <group_id|@username|invite_link>")
		return
	}

	input := strings.TrimSpace(strings.Join(fields[1:], " "))
	if input == "" {
		sendText(bot, chatID, "Usage: /join <group_id|@username|invite_link>")
		return
	}

	// Try to cast bot to TelegramBot interface
	tgBot, ok := imbot.AsTelegramBot(bot)
	if !ok {
		sendText(bot, chatID, "Join command is only supported for Telegram bot.")
		return
	}

	// Resolve the chat ID
	groupID, err := tgBot.ResolveChatID(input)
	if err != nil {
		logrus.WithError(err).Error("Failed to resolve chat ID")
		sendText(bot, chatID, fmt.Sprintf("Failed to resolve chat ID: %v\n\nNote: Bot must already be a member of the group to add it to whitelist.", err))
		return
	}

	// Check if already whitelisted
	if store.IsGroupWhitelisted(groupID) {
		sendText(bot, chatID, fmt.Sprintf("Group %s is already in whitelist.", groupID))
		return
	}

	// Add group to whitelist
	platform := string(imbot.PlatformTelegram)
	if err := store.AddGroupToWhitelist(groupID, platform, senderID); err != nil {
		logrus.WithError(err).Error("Failed to add group to whitelist")
		sendText(bot, chatID, fmt.Sprintf("Failed to add group to whitelist: %v", err))
		return
	}

	sendText(bot, chatID, fmt.Sprintf("Successfully added group to whitelist.\nGroup ID: %s", groupID))
	logrus.Infof("Group %s added to whitelist by %s", groupID, senderID)
}

// handleBindCommand handles the /bind command for project binding
func handleBindCommand(ctx context.Context, bot imbot.Bot, store *Store, chatID string, fields []string, senderID string, isDirectChat bool, isGroupChat bool) {
	if len(fields) < 2 {
		sendText(bot, chatID, "Usage: /bind <project_path>")
		return
	}

	projectPath := strings.TrimSpace(strings.Join(fields[1:], " "))
	if projectPath == "" {
		sendText(bot, chatID, "Usage: /bind <project_path>")
		return
	}

	// Expand and validate path
	expandedPath, err := ExpandPath(projectPath)
	if err != nil {
		sendText(bot, chatID, fmt.Sprintf("Invalid path: %v", err))
		return
	}

	if err := ValidateProjectPath(expandedPath); err != nil {
		sendText(bot, chatID, fmt.Sprintf("Path validation failed: %v", err))
		return
	}

	platform := string(imbot.PlatformTelegram)
	settings, _ := store.GetSettings()
	botUUID := settings.UUID
	if botUUID == "" {
		botUUID = "default"
	}

	projectStore, err := NewProjectStore(store.DB())
	if err != nil {
		sendText(bot, chatID, "Failed to initialize project store")
		logrus.WithError(err).Error("Failed to create project store")
		return
	}

	bindingStore, err := NewBindingStore(store.DB())
	if err != nil {
		sendText(bot, chatID, "Failed to initialize binding store")
		logrus.WithError(err).Error("Failed to create binding store")
		return
	}

	if isDirectChat {
		// Direct chat: create project and instruct user to create group
		// Check if project already exists
		existingProject, err := projectStore.GetProjectByPath(expandedPath, botUUID)
		if err != nil {
			logrus.WithError(err).Warn("Failed to check existing project")
		}

		var project *Project
		if existingProject != nil {
			project = existingProject
		} else {
			// Create new project
			project = &Project{
				Path:     expandedPath,
				OwnerID:  senderID,
				Platform: platform,
				BotUUID:  botUUID,
			}
			if err := projectStore.CreateProject(project); err != nil {
				sendText(bot, chatID, fmt.Sprintf("Failed to create project: %v", err))
				return
			}
		}

		// Instruct user to create a group
		msg := fmt.Sprintf(`Project created: %s (ID: %s)

To complete binding:
1. Create a new group and add this bot
2. In the group, run: /bind %s

This will bind the group to the project.`, project.Path, project.ID, project.Path)
		sendText(bot, chatID, msg)

	} else if isGroupChat {
		// Group chat: create/update project and bind to this group
		// Check if project already exists
		existingProject, err := projectStore.GetProjectByPath(expandedPath, botUUID)
		if err != nil {
			logrus.WithError(err).Warn("Failed to check existing project")
		}

		var project *Project
		if existingProject != nil {
			project = existingProject
		} else {
			// Create new project
			project = &Project{
				Path:     expandedPath,
				OwnerID:  senderID,
				Platform: platform,
				BotUUID:  botUUID,
			}
			if err := projectStore.CreateProject(project); err != nil {
				sendText(bot, chatID, fmt.Sprintf("Failed to create project: %v", err))
				return
			}
		}

		// Create or update group binding
		binding := &GroupProjectBinding{
			GroupID:   chatID,
			Platform:  platform,
			ProjectID: project.ID,
			BotUUID:   botUUID,
		}
		if err := bindingStore.UpsertGroupBinding(binding); err != nil {
			sendText(bot, chatID, fmt.Sprintf("Failed to bind group: %v", err))
			return
		}

		sendText(bot, chatID, fmt.Sprintf("Group bound to project: %s", project.Path))
	} else {
		sendText(bot, chatID, "This command only works in direct or group chats.")
	}
}

// handleProjectCommand handles the /project command to show current project info
func handleProjectCommand(bot imbot.Bot, store *Store, chatID string, platform string) {
	if store == nil || store.DB() == nil {
		sendText(bot, chatID, "Store not available")
		return
	}

	bindingStore, err := NewBindingStore(store.DB())
	if err != nil {
		sendText(bot, chatID, "Failed to initialize binding store")
		return
	}

	projectStore, err := NewProjectStore(store.DB())
	if err != nil {
		sendText(bot, chatID, "Failed to initialize project store")
		return
	}

	// Get group binding
	binding, err := bindingStore.GetGroupBinding(chatID, platform)
	if err != nil {
		logrus.WithError(err).Warn("Failed to get group binding")
	}
	if binding == nil {
		sendText(bot, chatID, "No project bound to this chat. Use /bind <path> to bind a project.")
		return
	}

	// Get project details
	project, err := projectStore.GetProject(binding.ProjectID)
	if err != nil || project == nil {
		sendText(bot, chatID, "Project not found. The binding may be invalid.")
		return
	}

	msg := fmt.Sprintf(`Project: %s
Path: %s
Owner: %s
Created: %s`, project.Name, project.Path, project.OwnerID, project.CreatedAt.Format("2006-01-02 15:04"))
	sendText(bot, chatID, msg)
}

// handleProjectsCommand handles the /projects command to list all user's projects
func handleProjectsCommand(bot imbot.Bot, store *Store, chatID string, senderID string, platform string) {
	if store == nil || store.DB() == nil {
		sendText(bot, chatID, "Store not available")
		return
	}

	bindingStore, err := NewBindingStore(store.DB())
	if err != nil {
		sendText(bot, chatID, "Failed to initialize binding store")
		return
	}

	// Get all projects with bindings for this user
	results, err := bindingStore.ListGroupBindingsByOwner(senderID, platform)
	if err != nil {
		logrus.WithError(err).Warn("Failed to list projects")
		sendText(bot, chatID, "Failed to list projects")
		return
	}

	if len(results) == 0 {
		sendText(bot, chatID, "No projects found. Use /bind <path> to create a project.")
		return
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Your projects (%d):", len(results)))
	for _, result := range results {
		project := result.Project
		bindingInfo := ""
		if result.Binding != nil {
			bindingInfo = " [group bound]"
		}
		lines = append(lines, fmt.Sprintf("- %s%s", project.Path, bindingInfo))
	}

	sendText(bot, chatID, strings.Join(lines, "\n"))
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
	for _, chunk := range chunkText(text, imbot.DefaultMessageLimit) {
		_, err := bot.SendText(context.Background(), chatID, chunk)
		if err != nil {
			logrus.WithError(err).Warn("Failed to send message")
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
