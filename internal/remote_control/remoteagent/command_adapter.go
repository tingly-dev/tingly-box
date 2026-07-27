package remoteagent

import (
	"context"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// botHandlerAdapter implements command.BotHandlerAdapter by delegating to BotHandler.
type botHandlerAdapter struct {
	handler *BotHandler
}

// NewBotHandlerAdapter creates a new adapter for the given handler.
func NewBotHandlerAdapter(handler *BotHandler) BotHandlerAdapter {
	return &botHandlerAdapter{handler: handler}
}

// ctx builds a HandlerContext for a command-registry invocation, carrying the
// bot's OWN platform. The registry dispatches without a message, so there is
// no inbound HandlerContext to inherit — but the platform must still be the
// real one: it is persisted onto the chat record (BindProject), and a wrong
// value both mis-attributes the chat and hides it from its own bot's
// GET /bots/:bot/chats listing.
func (a *botHandlerAdapter) ctx(chatID string) HandlerContext {
	return HandlerContext{
		ChatID:   chatID,
		Platform: imbot.Platform(a.handler.botSetting.Platform),
	}
}

// SendText sends a text message to a chat.
func (a *botHandlerAdapter) SendText(chatID, text string) error {
	a.handler.SendText(a.ctx(chatID), text)
	return nil
}

// GetProjectPath gets the current project path for a chat.
func (a *botHandlerAdapter) GetProjectPath(chatID string) (string, error) {
	projectPath, _, err := a.handler.chatStore.GetProjectPath(chatID)
	return projectPath, err
}

// SetProjectPath sets the project path for a chat.
func (a *botHandlerAdapter) SetProjectPath(chatID, path string) error {
	expandedPath, err := ExpandPath(path)
	if err != nil {
		return err
	}
	a.handler.completeBind(a.ctx(chatID), expandedPath)
	return nil
}

// sessionInfoFrom converts a session record into the command-facing SessionInfo.
func sessionInfoFrom(sess *session.Session) *SessionInfo {
	return &SessionInfo{
		ID:             sess.ID,
		Status:         string(sess.Status),
		Project:        sess.Project,
		Request:        sess.Request,
		Error:          sess.Error,
		PermissionMode: sess.PermissionMode,
		LastActivity:   sess.LastActivity,
	}
}

// GetSession gets session info.
func (a *botHandlerAdapter) GetSession(chatID, agentType, projectPath string) (*SessionInfo, error) {
	sess := a.handler.sessionMgr.FindBy(chatID, agentType, projectPath)
	if sess == nil {
		return nil, fmt.Errorf("session not found")
	}
	return sessionInfoFrom(sess), nil
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
	return sessionInfoFrom(sess), nil
}

// UpdatePermissionMode updates the permission mode for a session.
func (a *botHandlerAdapter) UpdatePermissionMode(sessionID, mode string) error {
	if !claude.IsValidPermissionMode(mode) {
		return fmt.Errorf("invalid permission mode: %q, must be one of: default, manual, plan, auto, acceptEdits, dontAsk, bypassPermissions", mode)
	}
	a.handler.sessionMgr.Update(sessionID, func(s *session.Session) {
		s.PermissionMode = mode
	})
	return nil
}

// ClearSession clears the current agent's session for the chat.
func (a *botHandlerAdapter) ClearSession(chatID string) error {
	a.handler.handleClearCommand(a.ctx(chatID))
	return nil
}

// StopExecution cancels a running execution, returns true if one was running.
func (a *botHandlerAdapter) StopExecution(chatID string) bool {
	return a.handler.executions.cancel(chatID)
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

// BuildHelpText renders the command registry's help listing.
func (a *botHandlerAdapter) BuildHelpText(isDirect bool) string {
	if a.handler.commandRegistry == nil {
		return ""
	}
	return a.handler.commandRegistry.BuildHelpText(isDirect)
}

// GetBashAllowlist returns the configured bash allowlist.
func (a *botHandlerAdapter) GetBashAllowlist() map[string]struct{} {
	allowlist := normalizeAllowlistToMap(a.handler.botSetting.BashAllowlist)
	if len(allowlist) == 0 {
		return defaultBashAllowlist
	}
	return allowlist
}

// ListChatProjectPaths returns the MRU project-path history for a chat.
func (a *botHandlerAdapter) ListChatProjectPaths(chatID string) ([]string, error) {
	return a.handler.chatStore.ListChatProjectPaths(chatID)
}

// VerifyAndPair runs pairing-code verification and persists the binding.
func (a *botHandlerAdapter) VerifyAndPair(botUUID, chatID, senderID, platform, code string) error {
	return a.handler.VerifyAndPair(botUUID, chatID, senderID, platform, code)
}

// BuildReplyFooter assembles the standard command-reply footer for a chat.
// Resolves project path with the same group fallback used elsewhere and the
// current agent for the chat. Returns "" when neither piece is available,
// so unpaired/unbound chats see no decoration.
func (a *botHandlerAdapter) BuildReplyFooter(chatID, platform string) string {
	projectPath := resolveProjectPath(a, chatID)
	agentType, _ := a.GetCurrentAgent(chatID)
	if projectPath == "" && agentType == "" {
		return ""
	}
	return BuildFooter(agentType, projectPath)
}

// ListResumableSessions delegates to the AgentBoot session store, returning
// the most recent Claude on-disk sessions for the project. Returns an empty
// slice (and no error) if the store has no entries; only configuration or I/O
// failures bubble up.
func (a *botHandlerAdapter) ListResumableSessions(projectPath string, limit int) ([]ResumableSession, error) {
	if a.handler.agentService == nil {
		return nil, fmt.Errorf("agent service not configured")
	}
	if limit <= 0 {
		limit = resumeListLimit
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	metas, err := a.handler.agentService.ListSessions(ctx, projectPath, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ResumableSession, 0, len(metas))
	for _, m := range metas {
		out = append(out, ResumableSession{
			SessionID:    m.SessionID,
			StartTime:    m.StartTime,
			EndTime:      m.EndTime,
			NumTurns:     m.NumTurns,
			Status:       string(m.Status),
			FirstMessage: m.FirstMessage,
		})
	}
	return out, nil
}

// PrepareResume closes any existing remote-session record for (chat, agent,
// project) and creates a new one whose ID matches the picked Claude session
// on disk. The next user message hitting resolveSession will then run with
// Resume=true. We deliberately do NOT launch Claude here — Claude needs an
// initial prompt, and the user picking from a list is not yet that prompt.
func (a *botHandlerAdapter) PrepareResume(chatID, agentType, projectPath, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("empty session id")
	}
	if old := a.handler.sessionMgr.FindBy(chatID, agentType, projectPath); old != nil && old.ID != sessionID {
		a.handler.sessionMgr.Close(old.ID)
	}
	if _, ok := a.handler.sessionMgr.GetOrLoad(sessionID); ok {
		// Already present (perhaps user re-armed the same id). Just refresh
		// the binding to make sure FindBy returns it next time.
		a.handler.sessionMgr.Update(sessionID, func(s *session.Session) {
			s.ChatID = chatID
			s.Agent = agentType
			s.Project = projectPath
			s.Status = session.StatusPending
			s.ExpiresAt = time.Time{}
		})
		return nil
	}
	if sess := a.handler.sessionMgr.CreateWithID(sessionID, chatID, agentType, projectPath); sess == nil {
		return fmt.Errorf("failed to bind resume session %s", sessionID)
	}
	return nil
}

// RememberResumeListing stores the displayed session-id list for /resume <n>.
func (a *botHandlerAdapter) RememberResumeListing(chatID string, sessionIDs []string) {
	a.handler.resumeListingsMu.Lock()
	defer a.handler.resumeListingsMu.Unlock()
	if len(sessionIDs) == 0 {
		delete(a.handler.resumeListings, chatID)
		return
	}
	clone := make([]string, len(sessionIDs))
	copy(clone, sessionIDs)
	a.handler.resumeListings[chatID] = clone
}

// RecallResumeListing returns a copy of the cached listing for /resume <n>.
func (a *botHandlerAdapter) RecallResumeListing(chatID string) []string {
	a.handler.resumeListingsMu.RLock()
	defer a.handler.resumeListingsMu.RUnlock()
	ids, ok := a.handler.resumeListings[chatID]
	if !ok {
		return nil
	}
	out := make([]string, len(ids))
	copy(out, ids)
	return out
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

// GetCommandRegistry returns the command registry.
func (h *BotHandler) GetCommandRegistry() *imbot.CommandRegistry {
	return h.commandRegistry
}

// resolveProjectPath returns the chat's bound project path ("" when none).
// Direct and group chats are stored under the same chat-id key, so one store
// query answers both.
func resolveProjectPath(adapter BotHandlerAdapter, chatID string) string {
	projectPath, _ := adapter.GetProjectPath(chatID)
	return projectPath
}
