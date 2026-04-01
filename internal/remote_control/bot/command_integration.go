package bot

import (
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
)

// botHandlerAdapter implements command.BotHandlerAdapter by delegating to BotHandler.
type botHandlerAdapter struct {
	handler *BotHandler
}

// NewBotHandlerAdapter creates a new adapter for the given handler.
func NewBotHandlerAdapter(handler *BotHandler) BotHandlerAdapter {
	return &botHandlerAdapter{handler: handler}
}

// SendText sends a text message to a chat.
func (a *botHandlerAdapter) SendText(chatID, text string) error {
	hCtx := HandlerContext{
		ChatID: chatID,
	}
	a.handler.SendText(hCtx, text)
	return nil
}

// GetProjectPath gets the current project path for a chat.
func (a *botHandlerAdapter) GetProjectPath(chatID string) (string, error) {
	projectPath, _, err := a.handler.chatStore.GetProjectPath(chatID)
	return projectPath, err
}

// SetProjectPath sets the project path for a chat.
func (a *botHandlerAdapter) SetProjectPath(chatID, path string) error {
	hCtx := HandlerContext{
		ChatID:   chatID,
		Platform: imbot.PlatformTelegram,
		SenderID: "",
	}
	expandedPath, err := ExpandPath(path)
	if err != nil {
		return err
	}
	a.handler.completeBind(hCtx, expandedPath)
	return nil
}

// GetProjectPathForGroup gets project path with group fallback.
func (a *botHandlerAdapter) GetProjectPathForGroup(chatID, platform string) (string, bool) {
	return getProjectPathForGroup(a.handler.chatStore, chatID, platform)
}

// GetSession gets session info.
func (a *botHandlerAdapter) GetSession(chatID, agentType, projectPath string) (*SessionInfo, error) {
	sess := a.handler.sessionMgr.FindBy(chatID, agentType, projectPath)
	if sess == nil {
		return nil, fmt.Errorf("session not found")
	}
	return &SessionInfo{
		ID:             sess.ID,
		Status:         string(sess.Status),
		Project:        sess.Project,
		Request:        sess.Request,
		Error:          sess.Error,
		PermissionMode: sess.PermissionMode,
		LastActivity:   sess.LastActivity,
	}, nil
}

// FindOrCreateSession finds an existing session or creates a new one.
func (a *botHandlerAdapter) FindOrCreateSession(chatID, agentType, projectPath string) (*SessionInfo, error) {
	sess := a.handler.sessionMgr.FindBy(chatID, agentType, projectPath)
	if sess == nil {
		sess = a.handler.sessionMgr.CreateWith(chatID, agentType, projectPath)
		a.handler.sessionMgr.Update(sess.ID, func(s *session.Session) {
			s.ExpiresAt = time.Time{} // Persistent session
		})
	}
	return &SessionInfo{
		ID:             sess.ID,
		Status:         string(sess.Status),
		Project:        sess.Project,
		Request:        sess.Request,
		Error:          sess.Error,
		PermissionMode: sess.PermissionMode,
		LastActivity:   sess.LastActivity,
	}, nil
}

// UpdatePermissionMode updates the permission mode for a session.
func (a *botHandlerAdapter) UpdatePermissionMode(sessionID, mode string) error {
	if !claude.IsValidPermissionMode(mode) {
		return fmt.Errorf("invalid permission mode: %q, must be one of: default, plan, auto, acceptEdits, dontAsk, bypassPermissions", mode)
	}
	a.handler.sessionMgr.Update(sessionID, func(s *session.Session) {
		s.PermissionMode = mode
	})
	return nil
}

// ClearSession clears a session.
func (a *botHandlerAdapter) ClearSession(chatID, agentType string) error {
	hCtx := HandlerContext{
		ChatID:   chatID,
		Platform: imbot.PlatformTelegram,
		SenderID: "",
	}
	a.handler.handleClearCommand(hCtx)
	return nil
}

// StopExecution cancels a running execution, returns true if one was running.
func (a *botHandlerAdapter) StopExecution(chatID string) bool {
	a.handler.runningCancelMu.Lock()
	cancel, exists := a.handler.runningCancel[chatID]
	if exists {
		delete(a.handler.runningCancel, chatID)
	}
	a.handler.runningCancelMu.Unlock()

	if exists && cancel != nil {
		cancel()
		return true
	}
	return false
}

// GetCurrentAgent gets the current agent for a chat.
func (a *botHandlerAdapter) GetCurrentAgent(chatID string) (string, error) {
	agent, err := a.handler.getCurrentAgent(chatID)
	if err != nil {
		return AgentNameTinglyBox, nil
	}
	return string(agent), nil
}

// SetVerbose sets verbose mode for a chat.
func (a *botHandlerAdapter) SetVerbose(chatID string, enabled bool) {
	a.handler.SetVerbose(chatID, enabled)
}

// GetVerbose gets verbose mode for a chat.
func (a *botHandlerAdapter) GetVerbose(chatID string) bool {
	return a.handler.GetVerbose(chatID)
}

// IsWhitelisted checks if a group is whitelisted.
func (a *botHandlerAdapter) IsWhitelisted(groupID string) bool {
	return a.handler.chatStore.IsWhitelisted(groupID)
}

// AddToWhitelist adds a group to whitelist.
func (a *botHandlerAdapter) AddToWhitelist(groupID, platform, userID string) error {
	return a.handler.chatStore.AddToWhitelist(groupID, platform, userID)
}

// GetBashCwd gets the bash working directory.
func (a *botHandlerAdapter) GetBashCwd(chatID string) (string, error) {
	cwd, _, err := a.handler.chatStore.GetBashCwd(chatID)
	return cwd, err
}

// SetBashCwd sets the bash working directory.
func (a *botHandlerAdapter) SetBashCwd(chatID, path string) error {
	return a.handler.chatStore.SetBashCwd(chatID, path)
}

// ResolveChatID resolves a chat ID using the Telegram bot.
func (a *botHandlerAdapter) ResolveChatID(input string) (string, error) {
	return input, nil
}

// GetDefaultProjectPath returns the default project path.
func (a *botHandlerAdapter) GetDefaultProjectPath() string {
	return a.handler.getDefaultProjectPath()
}

// GetBashAllowlist returns the configured bash allowlist.
func (a *botHandlerAdapter) GetBashAllowlist() map[string]struct{} {
	allowlist := normalizeAllowlistToMap(a.handler.botSetting.BashAllowlist)
	if len(allowlist) == 0 {
		return defaultBashAllowlist
	}
	return allowlist
}

// ListProjectPaths lists all project paths for a user.
func (a *botHandlerAdapter) ListProjectPaths(ownerID, platform string) ([]string, error) {
	chats, err := a.handler.chatStore.ListChatsByOwner(ownerID, platform)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var paths []string
	for _, chat := range chats {
		if chat.ProjectPath != "" && !seen[chat.ProjectPath] {
			paths = append(paths, chat.ProjectPath)
			seen[chat.ProjectPath] = true
		}
	}
	return paths, nil
}

// SendMessageWithKeyboard sends a text message with an inline keyboard.
// Note: For commands that need keyboards, use ctx.Bot directly in the handler.
func (a *botHandlerAdapter) SendMessageWithKeyboard(chatID, text string, keyboard interface{}) error {
	return fmt.Errorf("SendMessageWithKeyboard requires bot context, use ctx.Bot directly")
}

// FormatHelpWithHeader formats help text with meta information.
func (a *botHandlerAdapter) FormatHelpWithHeader(chatID, senderID, text string, isDirect bool, platform string) string {
	hCtx := HandlerContext{
		ChatID:   chatID,
		SenderID: senderID,
		Platform: imbot.Platform(platform),
	}
	return a.handler.formatHelpWithHeader(hCtx, text)
}

// StartInteractiveBind starts an interactive directory browser for project binding.
func (a *botHandlerAdapter) StartInteractiveBind(chatID string) error {
	hCtx := HandlerContext{
		ChatID:   chatID,
		Platform: imbot.PlatformTelegram,
	}
	a.handler.handleBindInteractive(hCtx)
	return nil
}

// InitCommandRegistry initializes the command registry with built-in commands.
func (h *BotHandler) InitCommandRegistry() error {
	registry := imbot.NewCommandRegistry()
	adapter := NewBotHandlerAdapter(h)

	if err := RegisterBuiltinCommands(registry, adapter); err != nil {
		return fmt.Errorf("failed to register built-in commands: %w", err)
	}

	h.commandRegistry = registry
	h.commandAdapter = adapter
	return nil
}

// HandleCommandViaRegistry handles a command using the new command registry.
func (h *BotHandler) HandleCommandViaRegistry(hCtx HandlerContext, cmdName string, args []string) error {
	if h.commandRegistry == nil {
		return fmt.Errorf("command registry not initialized")
	}

	handler, ok := h.commandRegistry.Match(cmdName)
	if !ok {
		return fmt.Errorf("command not found: %s", cmdName)
	}

	cmdCtx := imbot.NewHandlerContext(hCtx.Bot, hCtx.ChatID, hCtx.SenderID, hCtx.Platform).
		WithText(hCtx.Text()).
		WithDirectMessage(hCtx.IsDirect()).
		WithMessageID(hCtx.MessageID)

	return handler(cmdCtx, args)
}

// GetCommandRegistry returns the command registry.
func (h *BotHandler) GetCommandRegistry() *imbot.CommandRegistry {
	return h.commandRegistry
}

// resolveProjectPath is a helper that resolves project path with group fallback.
func resolveProjectPath(adapter BotHandlerAdapter, chatID, platform string) string {
	projectPath, _ := adapter.GetProjectPath(chatID)
	if projectPath == "" {
		if path, found := adapter.GetProjectPathForGroup(chatID, platform); found {
			projectPath = path
		}
	}
	return projectPath
}
