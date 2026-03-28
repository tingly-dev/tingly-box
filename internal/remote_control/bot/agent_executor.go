package bot

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
)

// AgentExecutor defines the interface for executing agent requests.
// Each agent type (Claude Code, Smart Guide, Mock) implements this interface.
type AgentExecutor interface {
	// Execute processes a prepared request and returns the result.
	Execute(ctx context.Context, req PreparedRequest) (*ExecutionResult, error)

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
	PermissionMode string        // resolved from session (Claude Code / Mock)
	ReplyTo        string
}

// ExecutionResult contains the outcome of agent execution
type ExecutionResult struct {
	Response     string
	SessionID    string
	Success      bool
	Error        error
	Meta         *ResponseMeta
	IsNewSession bool
	Duration     time.Duration
}

// ExecutorDependencies holds shared dependencies for agent executors and router.
type ExecutorDependencies struct {
	BotSetting                 BotSetting
	ChatStore                  ChatStoreInterface
	SessionMgr                 *SessionManager
	AgentBoot                  *agentboot.AgentBoot
	IMPrompter                 *IMPrompter
	FileStore                  *FileStore
	TBClient                   TBClient
	TBSessionStore             *SmartGuideSessionStore
	HandoffManager             *smart_guide.HandoffManager
	RunningCancel              map[string]context.CancelFunc
	RunningCancelMu            *sync.RWMutex
	GetVerbose                 func(chatID string) bool
	FormatResponse             func(meta ResponseMeta, response string, showMeta bool) string
	FormatResponseWithFooter   func(meta ResponseMeta, response string) string
	SendText                   func(hCtx HandlerContext, text string)
	SendTextWithReply          func(hCtx HandlerContext, text string, replyTo string)
	SendTextWithActionKeyboard func(hCtx HandlerContext, text string, replyTo string)
	NewStreamingMessageHandler func(hCtx HandlerContext, meta *ResponseMeta) *streamingMessageHandler
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

// ResolveDefaultProjectPath returns the default project path.
// Priority: 1. DefaultCwd from bot setting, 2. Current working directory, 3. User home directory
func (d *ExecutorDependencies) ResolveDefaultProjectPath() string {
	if d.BotSetting.DefaultCwd != "" {
		if expanded, err := ExpandPath(d.BotSetting.DefaultCwd); err == nil {
			return expanded
		}
		logrus.WithField("path", d.BotSetting.DefaultCwd).Warn("Failed to expand DefaultCwd")
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}

// resolveSession finds or creates a session for session-based agents (Claude Code, Mock).
// Returns (sessionID, isNew, permissionMode).
func (d *ExecutorDependencies) resolveSession(chatID string, agentType string, projectPath string) (string, bool, string) {
	sess := d.SessionMgr.FindBy(chatID, agentType, projectPath)

	if sess == nil || sess.Status == session.StatusExpired || sess.Status == session.StatusClosed || sess.Status == session.StatusPending {
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

// agentTypeToName maps agent type constant to display name.
func agentTypeToName(t agentboot.AgentType) string {
	switch t {
	case agentTinglyBox:
		return AgentNameTinglyBox
	case agentClaudeCode:
		return AgentNameClaude
	default:
		return string(t)
	}
}

// Type aliases to avoid import cycles
type SessionManager = session.Manager
type TBClient = tbclient.TBClient
type SmartGuideSessionStore = smart_guide.SessionStore
