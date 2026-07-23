package bot

import (
	"context"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/audit"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
)

// OnMessage is a consumer's inbound message callback. It returns true when the
// consumer consumed (claimed) the message; the bot host dispatches each inbound
// message through the mounted consumers in order and stops at the first claim.
// A catch-all consumer (remote_agent) always returns true and therefore sits
// last in the dispatch order.
type OnMessage func(msg imbot.Message, platform imbot.Platform, botUUID string) bool

// Attached is the inbound wiring a Consumer hands back after binding to a
// freshly connected bot.
type Attached struct {
	// OnMessage receives each inbound message via the host's dispatcher
	// (after the host's own prompt-reply routing). nil = this consumer needs
	// no inbound handling.
	OnMessage OnMessage
	// CommandRegistry drives platform menu / quick-action setup. The bot host
	// applies it after the bot connects; nil skips menu setup.
	CommandRegistry *imbot.CommandRegistry
	// Cleanup runs when the bot stops (context cancelled / goroutine exits).
	// nil = nothing to clean up.
	Cleanup func()
}

// Consumer is a purpose that uses a bot's channel. The bot itself is a
// connection resource; its channel — the send/prompt surface plus the shared
// prompter and reply routing — is host infrastructure that exists whenever the
// bot runs. Consumers are the channel's users: remote_agent (control Claude
// Code / SmartGuide from chat) and notify (scenario notifications routed via
// /tingly/:scenario/notify) today; future consumers implement the same
// interface and are injected the same way.
//
// Naming: the bot is the resource, a Consumer uses it — hence "consumer", not
// "provider" (which in this codebase already means an LLM provider).
//
// The bot host owns the generic pieces (imbot.Manager, chat store, pairing,
// audit, channel registry, the shared channel prompter) and passes them to
// Attach; a consumer owns only its purpose-specific dependencies (agent
// service, sessions, SmartGuide) captured at construction. None of Attach's
// parameters reference a purpose's machinery, which is what keeps the
// lifecycle decoupled.
type Consumer interface {
	// Name identifies the purpose, e.g. "remote_agent" or "notify".
	Name() string
	// Mounted reports whether this purpose is mounted on the given bot, based
	// on its settings (typically the Scenarios mount list). The lifecycle only
	// attaches mounted consumers, and a bot with no mounted consumer at all
	// does not run — "no mount, no bot".
	Mounted(setting BotSetting) bool
	// Attach binds to a connected bot and returns its inbound wiring.
	// prompter is the bot's shared channel prompter (host-owned; replies to
	// its prompts are routed by the host before any consumer sees them).
	Attach(
		ctx context.Context,
		setting BotSetting,
		mgr *imbot.Manager,
		prompter *imchannel.IMPrompter,
		chatStore ChatStoreInterface,
		pairing *PairingManager,
		auditLog *audit.Logger,
		channels *channel.Registry,
	) (*Attached, error)
}
