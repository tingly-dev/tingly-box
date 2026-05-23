package bot

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/remote/audit"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// BotHandler encapsulates all bot message handling logic and dependencies
type BotHandler struct {
	ctx              context.Context
	botSetting       BotSetting
	chatStore        ChatStoreInterface // Use interface for flexibility
	sessionMgr       *session.Manager
	agentService     *agentboot.AgentService
	directoryBrowser *feature.DirectoryBrowser
	manager          *imbot.Manager
	imPrompter       *imchannel.IMPrompter
	fileStore        *FileStore
	interaction      *imbot.InteractionHandler // New interaction handler
	tbClient         tbclient.TBClient         // TB Client for model configuration

	// Agent router for delegating execution to agent executors
	agentRouter *AgentRouter

	// Handoff manager for agent switching
	handoffManager *smart_guide.HandoffManager

	// SmartGuide session store for conversation history
	tbSessionStore *smart_guide.SessionStore

	// runningCancel tracks cancel functions for active executions per chatID
	runningCancel   map[string]context.CancelFunc
	runningCancelMu sync.RWMutex

	// pendingBinds tracks bind confirmation requests for unbound chats
	pendingBinds   map[string]*PendingBind
	pendingBindsMu sync.RWMutex

	// actionMenuMessageID tracks the message ID of the action keyboard menu per chatID
	actionMenuMessageID   map[string]string
	actionMenuMessageIDMu sync.RWMutex

	// resumeListings caches the session-id list most recently shown by /resume
	// per chat, so /resume <n> can resolve N back to a session_id without
	// re-reading the on-disk store. Best-effort, no persistence.
	resumeListings   map[string][]string
	resumeListingsMu sync.RWMutex

	// verbose controls whether to show intermediate messages (onMessage details)
	// true = show all messages (default), false = show only final results
	verbose   bool
	verboseMu sync.RWMutex

	// commandRegistry holds the strongly-typed command registry
	commandRegistry *imbot.CommandRegistry

	// commandAdapter bridges BotHandler to the command system
	commandAdapter BotHandlerAdapter

	// feishuCardRenderer converts imbot.Card to Feishu card JSON
	feishuCardRenderer *feature.FeishuCardRenderer

	// pairing handles TOFU pairing-code verification for direct messages.
	pairing *PairingManager

	// audit emits security events (pairing attempts, rejections, …).
	audit *audit.Logger
}

// PendingBind represents a pending bind confirmation request
type PendingBind struct {
	OriginalMessage string
	ProposedPath    string
	ExpiresAt       time.Time
}

// HandlerContext contains per-message context data
type HandlerContext struct {
	Bot       imbot.Bot
	BotUUID   string
	ChatID    string
	SenderID  string
	MessageID string
	Platform  imbot.Platform
	Message   imbot.Message
}

func (c *HandlerContext) IsDirect() bool {
	return c.Message.IsDirectMessage()
}

func (c *HandlerContext) IsGroup() bool {
	return c.Message.IsGroupMessage()
}

func (c *HandlerContext) Text() string {
	return strings.TrimSpace(c.Message.GetText())
}

// NewBotHandler creates a new bot handler with all dependencies
