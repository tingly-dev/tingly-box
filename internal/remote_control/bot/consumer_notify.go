package bot

import (
	"context"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/audit"
	"github.com/tingly-dev/tingly-box/remote/binding"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
)

// NotifyConsumerName identifies the notify purpose in logs and dispatch
// diagnostics. Unlike remote_agent it is not a stored mount name: notify is
// mounted implicitly by the bot's outbound scenario bindings (claude_code, …).
const NotifyConsumerName = "notify"

// notifyConsumer mounts a bot for the notify purpose: scenario plugins routed
// through /tingly/:scenario/notify deliver notifications and interactive
// prompts to the chats configured in the bot's scenario bindings.
//
// It carries no wiring of its own — the channel it relies on (registry entry,
// shared prompter, reply routing) is bot-host infrastructure that exists for
// every running bot. What this consumer contributes is the REASON TO RUN: its
// mount keeps a bot alive whose only job is delivering scenario traffic, so a
// bot with remote_agent off and an active binding still runs (channel-only /
// notify-only bot), and a bot nobody uses stays offline.
type notifyConsumer struct{}

// NewNotifyConsumer builds the notify consumer. It has no dependencies.
func NewNotifyConsumer() Consumer { return &notifyConsumer{} }

// Name identifies this purpose.
func (c *notifyConsumer) Name() string { return NotifyConsumerName }

// Mounted reports whether the bot serves the notify purpose: at least one
// outbound scenario binding (claude_code, …) is present and not disabled.
func (c *notifyConsumer) Mounted(setting BotSetting) bool {
	return binding.OutboundScenarioMounted(setting.Scenarios)
}

// Attach has nothing to wire: the host already provides the channel.
func (c *notifyConsumer) Attach(
	ctx context.Context,
	setting BotSetting,
	mgr *imbot.Manager,
	prompter *imchannel.IMPrompter,
	chatStore ChatStoreInterface,
	pairing *PairingManager,
	auditLog *audit.Logger,
	channels *channel.Registry,
) (*Attached, error) {
	return &Attached{}, nil
}
