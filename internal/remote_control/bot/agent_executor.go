package bot

import (
	"context"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
)

// AgentExecutor defines the interface for executing agent requests
// Each agent type (Claude Code, Smart Guide, Mock) implements this interface
type AgentExecutor interface {
	// Execute processes a user message and returns the result
	Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error)

	// GetAgentType returns the agent type identifier
	GetAgentType() agentboot.AgentType
}

// ExecutionRequest contains all parameters needed to execute an agent request
type ExecutionRequest struct {
	// Context for the request
	HCtx HandlerContext

	// User message text
	Text string

	// Project path (may be override, bound, or default)
	ProjectPath string

	// Session ID (optional, will be created if empty)
	SessionID string

	// Whether to resume from existing session history
	ShouldResume bool

	// Permission mode ("auto" for yolo mode, empty for default)
	PermissionMode string

	// Message ID to reply to
	ReplyToMessageID string
}

// ExecutionResult contains the outcome of agent execution
type ExecutionResult struct {
	// Response text to send to user
	Response string

	// Session ID used (created or resumed)
	SessionID string

	// Whether execution succeeded
	Success bool

	// Error if execution failed
	Error error

	// Metadata for response formatting
	Meta ResponseMeta

	// Whether this is a new session (vs resumed)
	IsNewSession bool

	// Execution duration
	Duration time.Duration
}

// ExecutorDependencies holds shared dependencies for agent executors
// This reduces the number of parameters passed to each executor
type ExecutorDependencies struct {
	// Bot settings
	BotSetting BotSetting

	// Chat store for project binding
	ChatStore ChatStoreInterface

	// Session manager
	SessionMgr *SessionManager

	// AgentBoot for getting agent instances
	AgentBoot *agentboot.AgentBoot

	// IM Prompter for permission requests
	IMPrompter *IMPrompter

	// File store for media handling
	FileStore *FileStore

	// TB Client for Smart Guide
	TBClient TBClient

	// TB Session store for Smart Guide
	TBSessionStore *SmartGuideSessionStore

	// Handoff manager
	HandoffManager *smart_guide.HandoffManager

	// Running cancel functions (for /stop command)
	RunningCancel   map[string]context.CancelFunc
	RunningCancelMu *sync.RWMutex

	// Verbose mode getter
	GetVerbose func(chatID string) bool

	// Format response helper
	FormatResponse func(meta ResponseMeta, response string, showMeta bool) string

	// Format response with footer (agent + path info)
	FormatResponseWithFooter func(meta ResponseMeta, response string) string

	// Send text helper
	SendText func(hCtx HandlerContext, text string)

	// Send text with reply helper
	SendTextWithReply func(hCtx HandlerContext, text string, replyTo string)

	// Send text with action keyboard
	SendTextWithActionKeyboard func(hCtx HandlerContext, text string, replyTo string)

	// New streaming message handler
	NewStreamingMessageHandler func(hCtx HandlerContext, meta ResponseMeta) *streamingMessageHandler
}

// SessionManager is an alias to avoid import cycle
// This allows agent executors to use session manager without importing the session package
type SessionManager = session.Manager

// TBClient is an alias to avoid exposing internal package
type TBClient = tbclient.TBClient

// SmartGuideSessionStore is an alias
type SmartGuideSessionStore = smart_guide.SessionStore
