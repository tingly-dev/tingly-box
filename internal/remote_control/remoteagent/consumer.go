package remoteagent

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/remote_control/feature"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/remote/binding"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// consumer is the bot.Consumer for the "remote_agent" purpose: controlling
// Claude Code (@cc) and the SmartGuide agent (@tb) from a chat. It owns the
// purpose-specific dependencies (agent service, session manager, SmartGuide
// TBClient, settings store) so the bot lifecycle does not have to. It sends
// its approval/ask prompts through the bot's shared channel prompter
// (host-owned), like every other user of the channel.
type consumer struct {
	sessionMgr   *session.Manager
	agentService *agentboot.AgentService
	tbClient     tbclient.TBClient
	store        bot.SettingsStore
}

// NewConsumer builds the consumer that binds a bot to the remote-agent
// purpose. tbClient and store may be nil (standalone / test use): SmartGuide
// falls back to Claude Code and dynamic settings refresh is skipped.
func NewConsumer(
	sessionMgr *session.Manager,
	agentService *agentboot.AgentService,
	tbClient tbclient.TBClient,
	store bot.SettingsStore,
) bot.Consumer {
	return &consumer{
		sessionMgr:   sessionMgr,
		agentService: agentService,
		tbClient:     tbClient,
		store:        store,
	}
}

// Name identifies this purpose.
func (c *consumer) Name() string { return binding.RemoteAgentScenario }

// Mounted reports whether the remote_agent mount is on for this bot (absent
// binding counts as on — legacy default, see binding.ScenarioMounted).
func (c *consumer) Mounted(setting bot.BotSetting) bool {
	return binding.ScenarioMounted(setting.Scenarios, binding.RemoteAgentScenario)
}

// Attach constructs the remote-agent message handler for a connected bot and
// returns its inbound wiring.
func (c *consumer) Attach(
	ctx context.Context,
	setting bot.BotSetting,
	mgr *imbot.Manager,
	prompter *imchannel.IMPrompter,
	chatStore bot.ChatStoreInterface,
	pairing *bot.PairingManager,
) (*bot.Attached, error) {
	directoryBrowser := feature.NewDirectoryBrowser()

	handler := NewBotHandler(
		ctx,
		setting,
		chatStore,
		c.sessionMgr,
		c.agentService,
		directoryBrowser,
		mgr,
		prompter,
		c.tbClient,
		pairing,
		c.store,
	)

	attached := &bot.Attached{
		// The remote agent is the catch-all consumer: every message that
		// reaches it is considered handled, so it must sit last in the
		// dispatch order.
		OnMessage: func(msg imbot.Message, platform imbot.Platform, botUUID string) bool {
			handler.HandleMessage(msg, platform, botUUID)
			return true
		},
		CommandRegistry: handler.GetCommandRegistry(),
	}

	uuid := setting.UUID

	// On stop (ctx cancel → goroutine exit): drop the bot's SmartGuide routing
	// rule. A remote_agent concern, kept here so the bot lifecycle stays free
	// of agent/SmartGuide machinery.
	attached.Cleanup = func() {
		if c.tbClient != nil {
			if err := c.tbClient.DeleteSmartGuideRuleForBot(context.Background(), uuid); err != nil {
				logrus.WithError(err).WithField("uuid", uuid).Warn("Failed to delete SmartGuide routing rule")
			} else {
				logrus.WithField("uuid", uuid).Info("SmartGuide routing rule deleted")
			}
		}
	}

	return attached, nil
}
