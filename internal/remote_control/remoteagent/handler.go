package remoteagent

import (
	"context"
	"strings"
	"sync"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/feature"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// BotHandler encapsulates all bot message handling logic and dependencies
type BotHandler struct {
	ctx              context.Context
	botSetting       bot.BotSetting
	chatStore        bot.ChatStoreInterface // Use interface for flexibility
	sessionMgr       *session.Manager
	agentService     *agentboot.AgentService
	directoryBrowser *feature.DirectoryBrowser
	manager          *imbot.Manager
	imPrompter       *imchannel.IMPrompter
	fileStore        *FileStore
	tbClient         tbclient.TBClient // TB Client for model configuration

	// Agent router for delegating execution to agent executors
	agentRouter *AgentRouter

	// Handoff manager for agent switching
	handoffManager *smart_guide.HandoffManager

	// SmartGuide session store for conversation history
	tbSessionStore *smart_guide.SessionStore

	// executions tracks the one running execution per chat; shared with the
	// AgentRouter (duplicate-run guard) and the /stop paths (cancel).
	executions *executionRegistry

	// resumeListings caches the session-id list most recently shown by /resume
	// per chat, so /resume <n> can resolve N back to a session_id without
	// re-reading the on-disk store. Best-effort, no persistence.
	resumeListings   map[string][]string
	resumeListingsMu sync.RWMutex

	// commandRegistry holds the strongly-typed command registry
	commandRegistry *imbot.CommandRegistry

	// commandAdapter bridges BotHandler to the command system
	commandAdapter BotHandlerAdapter

	// pairing handles TOFU pairing-code verification for direct messages.
	pairing *bot.PairingManager
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

func (c *HandlerContext) Text() string {
	return strings.TrimSpace(c.Message.GetText())
}
