package bot

import (
	"context"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/audit"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/platform"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/platform/feishu"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/platform/telegram"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/platform/weixin"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
)

func NewBotHandler(
	ctx context.Context,
	botSetting BotSetting,
	chatStore ChatStoreInterface,
	sessionMgr *session.Manager,
	agentBoot *agentboot.AgentBoot,
	directoryBrowser *feature.DirectoryBrowser,
	manager *imbot.Manager,
	tbClient tbclient.TBClient,
	pairing *PairingManager,
	auditLog *audit.Logger,
) *BotHandler {
	// Create IM prompter for permission requests
	imPrompter := NewIMPrompter(manager)

	// Create interaction handler for platform-agnostic interactions
	interactionHandler := imbot.NewInteractionHandler(manager)

	// Create file store with proxy support
	fileStore, err := NewFileStoreWithProxy(botSetting.ProxyURL)
	if err != nil {
		logrus.WithError(err).Warn("Failed to create file store with proxy, using default")
		fileStore = NewFileStore()
	}

	// Set telegram token for file URL resolution
	if token, ok := botSetting.Auth["token"]; ok {
		fileStore.SetTelegramToken(token)
	}

	// Initialize handoff manager
	handoffMgr := smart_guide.NewHandoffManager()

	// Initialize SmartGuide rule if configured
	if tbClient != nil && botSetting.SmartGuideProvider != "" && botSetting.SmartGuideModel != "" {
		// Use bot-specific rule creation with bot UUID and name
		if err := tbClient.EnsureSmartGuideRuleForBot(ctx, botSetting.UUID, botSetting.Name, botSetting.SmartGuideProvider, botSetting.SmartGuideModel); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"bot_uuid": botSetting.UUID,
				"bot_name": botSetting.Name,
				"provider": botSetting.SmartGuideProvider,
				"model":    botSetting.SmartGuideModel,
			}).Error("Failed to initialize SmartGuide rule, @tb will be unavailable")
			// Don't block startup, SmartGuide will return errors when used
		} else {
			logrus.WithFields(logrus.Fields{
				"bot_uuid": botSetting.UUID,
				"bot_name": botSetting.Name,
				"provider": botSetting.SmartGuideProvider,
				"model":    botSetting.SmartGuideModel,
			}).Info("SmartGuide rule initialized successfully")
		}
	}

	// Create SmartGuide session store using data directory from tbClient
	var tbSessionStore *smart_guide.SessionStore
	if tbClient != nil {
		dataDir := tbClient.GetDataDir()
		if dataDir != "" {
			sessionsDir := filepath.Join(dataDir, "sessions")
			tbSessionStore, err = smart_guide.NewSessionStore(sessionsDir)
			if err != nil {
				logrus.WithError(err).WithField("sessionsDir", sessionsDir).Warn("Failed to create SmartGuide session store")
			} else {
				logrus.WithField("sessionsDir", sessionsDir).Info("Created SmartGuide session store")
			}
		}
	}

	// Initialize platform registry (Phase 4 refactoring)
	platformRegistry := platform.NewRegistry()

	// Create the BotHandler instance first (needed for method references)
	handler := &BotHandler{
		ctx:                 ctx,
		botSetting:          botSetting,
		chatStore:           chatStore,
		sessionMgr:          sessionMgr,
		agentBoot:           agentBoot,
		directoryBrowser:    directoryBrowser,
		manager:             manager,
		imPrompter:          imPrompter,
		fileStore:           fileStore,
		interaction:         interactionHandler,
		tbClient:            tbClient,
		handoffManager:      handoffMgr,
		tbSessionStore:      tbSessionStore,
		runningCancel:       make(map[string]context.CancelFunc),
		pendingBinds:        make(map[string]*PendingBind),
		actionMenuMessageID: make(map[string]string),
		verbose:             true, // Default to verbose mode
		feishuCardRenderer:  feature.NewFeishuCardRenderer(),
		pairing:             pairing,
		audit:               auditLog,
		platformRegistry:    platformRegistry, // Phase 4: Platform registry
	}

	// Register platform handlers
	handler.registerPlatformHandlers()

	// Initialize AgentRouter with dependencies
	deps := &ExecutorDependencies{
		BotSetting:                 botSetting,
		ChatStore:                  chatStore,
		SessionMgr:                 sessionMgr,
		AgentBoot:                  agentBoot,
		IMPrompter:                 imPrompter,
		FileStore:                  fileStore,
		TBClient:                   tbClient,
		TBSessionStore:             tbSessionStore,
		HandoffManager:             handoffMgr,
		RunningCancel:              handler.runningCancel,
		RunningCancelMu:            &handler.runningCancelMu,
		GetVerbose:                 handler.GetVerbose,
		FormatResponse:             handler.formatResponseWithHeader,
		FormatResponseWithFooter:   handler.formatResponseWithFooter,
		SendText:                   handler.SendText,
		SendTextWithReply:          handler.sendTextWithReply,
		SendTextWithActionKeyboard: handler.sendTextWithActionKeyboard,
		SendFile: func(hCtx HandlerContext, filePath, caption string) error {
			return handler.SendFile(context.Background(), hCtx, filePath, caption)
		},
		NewStreamingMessageHandler: handler.newStreamingMessageHandler,
	}
	handler.agentRouter = NewAgentRouter(deps)
	handler.InitCommandRegistry()

	// Initialize the new command system (Phase 1 refactoring)
	if err := handler.InitNewCommandSystem(); err != nil {
		logrus.WithError(err).Warn("Failed to initialize new command system, will use legacy system")
	}

	// Initialize the security system (Phase 2 refactoring)
	if err := handler.InitSecuritySystem(); err != nil {
		logrus.WithError(err).Warn("Failed to initialize security system")
	}

	return handler
}

// registerPlatformHandlers registers platform-specific handlers
// This is part of Phase 4: Platform Abstraction
func (h *BotHandler) registerPlatformHandlers() {
	// Note: Platform handlers need bot instances, which are created per-connection
	// This method sets up lazy initialization via the manager
	// Actual handler registration happens in HandleMessage when bot is available

	logrus.Debug("Platform handler registration initialized")
}

// GetPlatformHandler returns the platform handler for a given bot
// This is part of Phase 4: Platform Abstraction
func (h *BotHandler) GetPlatformHandler(bot imbot.Bot) platform.MessageHandler {
	// Check if already registered
	if handler := h.platformRegistry.Get(bot.PlatformInfo().ID); handler != nil {
		return handler
	}

	// Create and register handler based on platform
	var handler platform.MessageHandler
	switch bot.PlatformInfo().ID {
	case imbot.PlatformTelegram:
		handler = telegram.NewHandler(bot)
	case imbot.PlatformFeishu, imbot.PlatformLark:
		handler = feishu.NewHandler(bot)
	case imbot.Platform("weixin"):
		// QR client is optional for Weixin
		handler = weixin.NewHandler(bot, nil)
	default:
		// Default to a generic handler (no platform-specific behavior)
		return nil
	}

	if handler != nil {
		h.platformRegistry.Register(handler)
	}

	return handler
}

// SupportsFeature checks if the platform supports a specific feature
func (h *BotHandler) SupportsFeature(platform imbot.Platform, feature platform.Feature) bool {
	return h.platformRegistry.SupportsFeature(platform, feature)
}

// GetVerbose returns the current verbose mode setting for a chat
// Checks chat store first, then bot setting default
// Returns false for platforms that don't support verbose mode (e.g., Weixin)
func (h *BotHandler) GetVerbose(chatID string) bool {
	// Check platform first (Phase 4: Platform abstraction)
	// Get the platform for this chat from chat store
	chat, err := h.chatStore.GetChat(chatID)
	if err == nil && chat.Platform != "" {
		// Check if platform supports verbose mode
		if !h.SupportsFeature(imbot.Platform(chat.Platform), platform.FeatureVerbose) {
			return false
		}
	}

	h.verboseMu.RLock()
	defer h.verboseMu.RUnlock()

	// Check chat-specific setting first
	if chat, err := h.chatStore.GetChat(chatID); err == nil {
		if chat.Verbose != nil {
			return *chat.Verbose
		}
	}

	// Fall back to bot default
	return h.verbose
}

// SetVerbose sets the verbose mode for a chat
func (h *BotHandler) SetVerbose(chatID string, enabled bool) {
	h.verboseMu.Lock()
	defer h.verboseMu.Unlock()

	// Update chat store
	if err := h.chatStore.UpdateChat(chatID, func(c *Chat) {
		c.Verbose = &enabled
	}); err != nil {
		logrus.WithError(err).WithField("chatID", chatID).Warn("Failed to save verbose setting")
	}
}
