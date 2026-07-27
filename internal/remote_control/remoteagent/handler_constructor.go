package remoteagent

import (
	"context"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/remote_control/feature"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
	"github.com/tingly-dev/tingly-box/remote/session"
)

func NewBotHandler(
	ctx context.Context,
	botSetting bot.BotSetting,
	chatStore bot.ChatStoreInterface,
	sessionMgr *session.Manager,
	agentService *agentboot.AgentService,
	directoryBrowser *feature.DirectoryBrowser,
	manager *imbot.Manager,
	prompter *imchannel.IMPrompter,
	tbClient tbclient.TBClient,
	pairing *bot.PairingManager,
	store bot.SettingsStore,
) *BotHandler {
	// The bot's channel prompter for permission/ask requests. In the managed
	// path the host supplies the bot's shared prompter (and routes replies to
	// it before this handler runs); standalone callers (CLI, test harness)
	// pass nil and get a private one, with this handler doing the routing.
	imPrompter := prompter
	if imPrompter == nil {
		imPrompter = imchannel.NewIMPrompter(manager)
	}

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
		ctx:              ctx,
		botSetting:       botSetting,
		chatStore:        chatStore,
		sessionMgr:       sessionMgr,
		agentService:     agentService,
		directoryBrowser: directoryBrowser,
		manager:          manager,
		imPrompter:       imPrompter,
		fileStore:        fileStore,
		tbClient:         tbClient,
		handoffManager:   handoffMgr,
		tbSessionStore:   tbSessionStore,
		executions:       newExecutionRegistry(),
		resumeListings:   make(map[string][]string),
		pairing:          pairing,
	}

	// Initialize AgentRouter with dependencies
	deps := &ExecutorDependencies{
		ChatStore:         chatStore,
		SessionMgr:        sessionMgr,
		AgentService:      agentService,
		IMPrompter:        imPrompter,
		FileStore:         fileStore,
		TBClient:          tbClient,
		TBSessionStore:    tbSessionStore,
		Executions:        handler.executions,
		SendText:          handler.SendText,
		SendTextWithReply: handler.sendTextWithReply,
		SendFile: func(hCtx HandlerContext, filePath, caption string) error {
			return handler.SendFile(context.Background(), hCtx, filePath, caption)
		},
		NewStreamingMessageHandler: handler.newStreamingMessageHandler,
		// GetBotSetting dynamically fetches the current bot settings from the store
		GetBotSetting: func() (bot.BotSetting, error) {
			if store == nil {
				return botSetting, nil
			}
			record, err := store.GetSettingsByUUID(botSetting.UUID)
			if err != nil {
				return botSetting, err
			}
			return bot.SettingFromRecord(record), nil
		},
	}
	handler.agentRouter = NewAgentRouter(deps)
	handler.InitCommandRegistry()

	return handler
}
