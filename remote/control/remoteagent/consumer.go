package remoteagent

import (
	"context"

	bot2 "github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/feature"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/binding"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
)

// consumer is the bot.Consumer for the "remote_agent" purpose: talking to
// Claude Code from a chat, through Projects (ProjectRouter).
type consumer struct {
	store bot2.SettingsStore
	// projectRouter hands chats to Projects; nil leaves chats with access
	// and notifications only.
	projectRouter ProjectRouter
}

// ConsumerOption customises NewConsumer.
type ConsumerOption func(*consumer)

// WithProjectRouter hands chats to Projects (see ProjectRouter).
func WithProjectRouter(r ProjectRouter) ConsumerOption {
	return func(c *consumer) { c.projectRouter = r }
}

// NewConsumer builds the consumer that binds a bot to the remote-agent
// purpose. store may be nil (test use): settings are then read once.
func NewConsumer(store bot2.SettingsStore, opts ...ConsumerOption) bot2.Consumer {
	c := &consumer{store: store}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name identifies this purpose.
func (c *consumer) Name() string { return binding.RemoteAgentScenario }

// Mounted reports whether the remote_agent mount is on for this bot (absent
// binding counts as on — legacy default, see binding.ScenarioMounted).
func (c *consumer) Mounted(setting bot2.BotSetting) bool {
	return binding.ScenarioMounted(setting.Scenarios, binding.RemoteAgentScenario)
}

// Attach constructs the remote-agent message handler for a connected bot and
// returns its inbound wiring.
func (c *consumer) Attach(
	ctx context.Context,
	setting bot2.BotSetting,
	mgr *imbot.Manager,
	prompter *imchannel.IMPrompter,
	chatStore bot2.ChatStoreInterface,
	pairing *bot2.PairingManager,
) (*bot2.Attached, error) {
	directoryBrowser := feature.NewDirectoryBrowser()

	handler := NewBotHandler(ctx, setting, chatStore, directoryBrowser, mgr, pairing, c.store)
	if c.projectRouter != nil {
		handler.SetProjectRouter(c.projectRouter)
	}

	attached := &bot2.Attached{
		// The remote agent is the catch-all consumer: every message that
		// reaches it is considered handled, so it must sit last in the
		// dispatch order.
		OnMessage: func(msg imbot.Message, platform imbot.Platform, botUUID string) bool {
			handler.HandleMessage(msg, platform, botUUID)
			return true
		},
		CommandRegistry: handler.GetCommandRegistry(),
	}

	return attached, nil
}
