package bot

import (
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/command"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
)

// InitNewCommandSystem initializes the new command system with the dispatcher.
// This is the new implementation that will eventually replace InitCommandRegistry.
func (h *BotHandler) InitNewCommandSystem() error {
	// Create adapter that bridges BotHandler to command.Handler
	adapter := command.NewBotHandlerAdapter(func(chatID, text string) error {
		hCtx := HandlerContext{
			ChatID: chatID,
		}
		h.SendText(hCtx, text)
		return nil
	})

	// Configure all adapter methods
	adapter.
		WithProjectPathFuncs(
			func(chatID string) (string, error) {
				projectPath, _, err := h.chatStore.GetProjectPath(chatID)
				return projectPath, err
			},
			func(chatID, path string) error {
				hCtx := HandlerContext{
					ChatID:   chatID,
					Platform: imbot.Platform(h.botSetting.Platform),
					SenderID: "",
				}
				expandedPath, err := ExpandPath(path)
				if err != nil {
					return err
				}
				h.completeBind(hCtx, expandedPath)
				return nil
			},
			func(chatID, platform string) (string, bool) {
				return getProjectPathForGroup(h.chatStore, chatID, platform)
			},
		).
		WithStopExecution(func(chatID string) bool {
			h.runningCancelMu.Lock()
			cancel, exists := h.runningCancel[chatID]
			if exists {
				delete(h.runningCancel, chatID)
			}
			h.runningCancelMu.Unlock()

			if exists && cancel != nil {
				cancel()
				return true
			}
			return false
		}).
		WithAgentFuncs(
			func(chatID string) (string, error) {
				agent, err := h.getCurrentAgent(chatID)
				if err != nil {
					return agentTinglyBoxStr, nil
				}
				return string(agent), nil
			},
		).
		WithVerboseFuncs(
			h.GetVerbose,
			h.SetVerbose,
		).
		WithWhitelistFuncs(
			h.chatStore.IsWhitelisted,
			h.chatStore.AddToWhitelist,
		).
		WithBashCwdFuncs(
			func(chatID string) (string, error) {
				cwd, _, err := h.chatStore.GetBashCwd(chatID)
				return cwd, err
			},
			h.chatStore.SetBashCwd,
		).
		WithChatIDResolution(func(input string) (string, error) {
			return input, nil
		}).
		WithDefaultProjectPath(h.getDefaultProjectPath).
		WithBashAllowlist(func() map[string]struct{} {
			allowlist := normalizeAllowlistToMap(h.botSetting.BashAllowlist)
			if len(allowlist) == 0 {
				return defaultBashAllowlist
			}
			return allowlist
		}).
		WithProjectPathsList(func(ownerID, platform string) ([]string, error) {
			chats, err := h.chatStore.ListChatsByOwner(ownerID, platform)
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
		}).
		WithPairing(h.VerifyAndPair).
		WithSessionFuncs(
			func(chatID, agentType string) error {
				hCtx := HandlerContext{
					ChatID:   chatID,
					Platform: imbot.Platform(h.botSetting.Platform),
					SenderID: "",
				}
				h.handleClearCommand(hCtx)
				return nil
			},
			func(chatID, agentType, projectPath string) (*command.SessionInfo, error) {
				sess := h.sessionMgr.FindBy(chatID, agentType, projectPath)
				if sess == nil {
					sess = h.sessionMgr.CreateWith(chatID, agentType, projectPath)
					h.sessionMgr.Update(sess.ID, func(s *session.Session) {
						s.ExpiresAt = time.Time{} // Persistent session
					})
				}
				return &command.SessionInfo{
					ID:             sess.ID,
					Status:         string(sess.Status),
					Project:        sess.Project,
					Request:        sess.Request,
					Error:          sess.Error,
					PermissionMode: sess.PermissionMode,
					LastActivity:   sess.LastActivity,
				}, nil
			},
			func(sessionID, mode string) error {
				h.sessionMgr.Update(sessionID, func(s *session.Session) {
					s.PermissionMode = mode
				})
				return nil
			},
			func(chatID, agentType, projectPath string) (*command.SessionInfo, error) {
				sess := h.sessionMgr.FindBy(chatID, agentType, projectPath)
				if sess == nil {
					return nil, command.ErrNotFound
				}
				return &command.SessionInfo{
					ID:             sess.ID,
					Status:         string(sess.Status),
					Project:        sess.Project,
					Request:        sess.Request,
					Error:          sess.Error,
					PermissionMode: sess.PermissionMode,
					LastActivity:   sess.LastActivity,
				}, nil
			},
		)

	// Create the new dispatcher
	h.newCommandDispatcher = command.NewDispatcher(adapter)

	// Register all built-in commands
	if err := command.RegisterBuiltinCommands(h.newCommandDispatcher); err != nil {
		return err
	}

	return nil
}

// HandleCommandViaNewDispatcher handles a command using the new command dispatcher.
func (h *BotHandler) HandleCommandViaNewDispatcher(hCtx HandlerContext, text string) (bool, error) {
	if h.newCommandDispatcher == nil {
		return false, nil
	}

	// Build command context
	cmdCtx := &command.Context{
		ChatID:    hCtx.ChatID,
		SenderID:  hCtx.SenderID,
		BotUUID:   hCtx.BotUUID,
		Platform:  hCtx.Platform,
		MessageID: hCtx.MessageID,
		Text:      hCtx.Text(),
		IsDirect:  hCtx.IsDirect(),
		Bot:       hCtx.Bot,
	}

	return h.newCommandDispatcher.Handle(cmdCtx, text)
}

// GetCommandDispatcher returns the new command dispatcher (for testing and help text generation).
func (h *BotHandler) GetCommandDispatcher() *command.Dispatcher {
	return h.newCommandDispatcher
}

const (
	agentTinglyBoxStr = "tingly-box"
)
