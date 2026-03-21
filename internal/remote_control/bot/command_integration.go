package bot

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot"
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
	}, nil
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

// ResolveChatID resolves a chat ID (for Telegram join command).
func (a *botHandlerAdapter) ResolveChatID(input string) (string, error) {
	return input, nil
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
