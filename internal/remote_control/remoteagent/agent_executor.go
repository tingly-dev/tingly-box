package remoteagent

import (
	"context"
	"os"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// AgentExecutor defines the interface for executing agent requests.
// Each agent type (Claude Code, Smart Guide) implements this interface.
type AgentExecutor interface {
	// Execute processes a prepared request. Streaming output and completion
	// cards go straight to the chat; the caller only needs the error.
	Execute(ctx context.Context, req PreparedRequest) error

	// GetAgentType returns the agent type identifier
	GetAgentType() agentboot.AgentType
}

// ExecutionRequest contains caller-provided parameters (from bot handler layer).
type ExecutionRequest struct {
	HCtx             HandlerContext
	Text             string
	ProjectPath      string // optional override
	ReplyToMessageID string
}

// PreparedRequest is the fully-resolved request built by AgentRouter.
// All executors receive this — shared *ResponseMeta ensures path changes propagate.
type PreparedRequest struct {
	HCtx           HandlerContext
	Text           string
	ProjectPath    string        // fully resolved: override > ChatStore > default
	Meta           *ResponseMeta // shared pointer, created by router
	SessionID      string        // resolved session ID (chatID for SmartGuide)
	IsNewSession   bool          // whether session was just created
	PermissionMode string        // resolved from session (Claude Code)
	ReplyTo        string
}

// ExecutorDependencies holds shared dependencies for agent executors and router.
type ExecutorDependencies struct {
	// GetBotSetting dynamically retrieves the current bot settings from the store.
	// This ensures that any configuration changes (provider, model, etc.) are reflected
	// immediately without requiring a bot restart.
	GetBotSetting func() (bot.BotSetting, error)

	ChatStore                  bot.ChatStoreInterface
	SessionMgr                 *session.Manager
	AgentService               *agentboot.AgentService
	IMPrompter                 *imchannel.IMPrompter
	FileStore                  *FileStore
	TBClient                   tbclient.TBClient
	TBSessionStore             *smart_guide.SessionStore
	Executions                 *executionRegistry
	SendText                   func(hCtx HandlerContext, text string)
	SendTextWithReply          func(hCtx HandlerContext, text string, replyTo string)
	SendFile                   func(hCtx HandlerContext, filePath, caption string) error
	NewStreamingMessageHandler func(hCtx HandlerContext) *streamingMessageHandler
}

// GetBotSettingOrCache returns the current bot setting.
// If dynamic lookup fails, returns an empty setting.
func (d *ExecutorDependencies) GetBotSettingOrCache() bot.BotSetting {
	if setting, err := d.GetBotSetting(); err == nil {
		return setting
	}
	return bot.BotSetting{}
}

// resolveProjectPath resolves project path: override > ChatStore > default.
func (d *ExecutorDependencies) resolveProjectPath(chatID string, override string) string {
	if override != "" {
		return override
	}
	if p, ok, _ := d.ChatStore.GetProjectPath(chatID); ok && p != "" {
		return p
	}
	return d.ResolveDefaultProjectPath()
}

// ResolveDefaultProjectPath returns the default project path from bot settings.
func (d *ExecutorDependencies) ResolveDefaultProjectPath() string {
	setting := d.GetBotSettingOrCache()
	if setting.DefaultCwd != "" {
		if expanded, err := ExpandPath(setting.DefaultCwd); err == nil {
			return expanded
		}
		logrus.WithField("path", setting.DefaultCwd).Warn("Failed to expand DefaultCwd")
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}

// resolveSession finds or creates a session for session-based agents (Claude Code).
// Returns (sessionID, isNew, permissionMode).
func (d *ExecutorDependencies) resolveSession(chatID string, agentType string, projectPath string) (string, bool, string) {
	sess := d.SessionMgr.FindBy(chatID, agentType, projectPath)

	if sess == nil || sess.Status == session.StatusExpired || sess.Status == session.StatusClosed {
		sess = d.SessionMgr.CreateWith(chatID, agentType, projectPath)
		d.SessionMgr.Update(sess.ID, func(s *session.Session) {
			s.ExpiresAt = time.Time{}
			s.Status = session.StatusRunning
		})
		return sess.ID, true, ""
	}

	d.SessionMgr.Update(sess.ID, func(s *session.Session) {
		s.Status = session.StatusRunning
		s.LastActivity = time.Now()
	})
	return sess.ID, false, sess.PermissionMode
}
