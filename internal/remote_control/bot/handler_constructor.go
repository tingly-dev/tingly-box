package bot

import (
	"context"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/remote/audit"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
	"github.com/tingly-dev/tingly-box/remote/session"
)

func NewBotHandler(
	ctx context.Context,
	botSetting BotSetting,
	chatStore ChatStoreInterface,
	sessionMgr *session.Manager,
	agentService *agentboot.AgentService,
	directoryBrowser *feature.DirectoryBrowser,
	manager *imbot.Manager,
	tbClient tbclient.TBClient,
	pairing *PairingManager,
	auditLog *audit.Logger,
	store SettingsStore,
) *BotHandler {
	// Create IM prompter for permission requests
	imPrompter := imchannel.NewIMPrompter(manager)

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

	// Create the BotHandler instance first (needed for method references)
	handler := &BotHandler{
		ctx:                 ctx,
		botSetting:          botSetting,
		chatStore:           chatStore,
		sessionMgr:          sessionMgr,
		agentService:        agentService,
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
		resumeListings:      make(map[string][]string),
		verbose:             true, // Default to verbose mode
		feishuCardRenderer:  feature.NewFeishuCardRenderer(),
		pairing:             pairing,
		audit:               auditLog,
	}

	// Initialize AgentRouter with dependencies
	deps := &ExecutorDependencies{
		ChatStore:                  chatStore,
		SessionMgr:                 sessionMgr,
		AgentService:               agentService,
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
		// GetBotSetting dynamically fetches the current bot settings from the store
		GetBotSetting: func() (BotSetting, error) {
			if store == nil {
				return botSetting, nil
			}
			settingsAny, err := store.GetSettingsByUUIDInterface(botSetting.UUID)
			if err != nil {
				return botSetting, err
			}
			// Handle both bot.Settings and db.Settings types
			if record, ok := settingsAny.(db.Settings); ok {
				return BotSetting{
					UUID:               record.UUID,
					Name:               record.Name,
					Token:              record.Auth["token"],
					Platform:           record.Platform,
					AuthType:           record.AuthType,
					Auth:               record.Auth,
					ProxyURL:           record.ProxyURL,
					ChatIDLock:         record.ChatIDLock,
					BashAllowlist:      record.BashAllowlist,
					DefaultCwd:         record.DefaultCwd,
					Enabled:            record.Enabled,
					SmartGuideProvider: record.SmartGuideProvider,
					SmartGuideModel:    record.SmartGuideModel,
					RequirePairing:     record.RequirePairing,
				}, nil
			}
			return botSetting, nil
		},
	}
	handler.agentRouter = NewAgentRouter(deps)
	handler.InitCommandRegistry()

	return handler
}

// GetVerbose returns the current verbose mode setting for a chat
// Checks chat store first, then bot setting default
// Returns false for platforms that don't support verbose mode (e.g., Weixin)
