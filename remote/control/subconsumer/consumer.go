// Package subconsumer implements the subscription purpose as a bot.Consumer:
// it claims inbound chat messages addressed to a Subscription (see
// remote/subscription and .design/subscription.md §6) and enqueues them into
// the subscription's mailbox. It sits between the host's prompt-reply router
// and the remote_agent catch-all in dispatch order.
//
// The security gate is the binding itself: claim rules only ever run for
// messages arriving in a chat some enabled subscription is bound to; unbound
// chats never reach a subscription (spec §3 — binding IS the authorization).
package subconsumer

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/smart_guide"
	"github.com/tingly-dev/tingly-box/remote/subscription"
)

// ConsumerName identifies the subscription purpose in logs and dispatch
// diagnostics. Like notify it is not a stored capability: it is mounted
// implicitly by the existence of enabled subscriptions ("a reason to run").
const ConsumerName = "subscription"

// agentState is the narrow slice of the chat store this consumer touches:
// the per-chat sticky peer. bot.ChatStoreInterface satisfies it.
type agentState interface {
	SetCurrentAgent(chatID, platform, agentType string) error
	GetCurrentAgent(chatID string) (string, error)
}

// Consumer is the subscription purpose. One instance serves every bot the
// lifecycle manager runs; per-bot state is resolved from the store per
// message (human-scale traffic, and it keeps subscriptions created while a
// bot is running visible without a restart).
type Consumer struct {
	store   subscription.Store
	mailbox *subscription.Mailbox
	// sends resolves reply-to addressing: platform message ids recently sent
	// by each subscription, per chat (tier 2).
	sends *subscription.RecentSends
}

// New builds the consumer. mailbox and sends may be shared with the HTTP
// module so outbound sends and inbound claims see the same state.
func New(store subscription.Store, mailbox *subscription.Mailbox, sends *subscription.RecentSends) *Consumer {
	return &Consumer{store: store, mailbox: mailbox, sends: sends}
}

// Name identifies this purpose.
func (c *Consumer) Name() string { return ConsumerName }

// Mounted reports whether the bot has a reason to run for this purpose: at
// least one enabled subscription bound to it. Mirrors notify's implicit,
// data-derived mount (no new toggle).
func (c *Consumer) Mounted(setting bot.BotSetting) bool {
	return c.store != nil && c.store.HasEnabledForBot(setting.UUID)
}

// Attach wires the inbound claim handler. No cleanup, no command registry —
// the channel this purpose relies on is host infrastructure.
func (c *Consumer) Attach(
	ctx context.Context,
	setting bot.BotSetting,
	mgr *imbot.Manager,
	prompter *imchannel.IMPrompter,
	chatStore bot.ChatStoreInterface,
	pairing *bot.PairingManager,
) (*bot.Attached, error) {
	return &bot.Attached{
		OnMessage: func(msg imbot.Message, platform imbot.Platform, botUUID string) bool {
			return c.handle(msg, platform, botUUID, mgr, chatStore)
		},
	}, nil
}

// handle applies the claim rules from spec §6. Returns true when the message
// was consumed.
func (c *Consumer) handle(msg imbot.Message, platform imbot.Platform, botUUID string, mgr *imbot.Manager, chatStore agentState) bool {
	chatID := msg.GetReplyTarget()
	if chatID == "" || msg.IsCallback() || !msg.IsTextContent() {
		return false
	}
	text := strings.TrimSpace(msg.GetText())
	if text == "" {
		return false
	}

	bound := c.boundSubscriptions(botUUID, chatID)
	if len(bound) == 0 {
		// Rule 0: unbound chat — nothing here belongs to a subscription.
		return false
	}

	send := func(reply string) {
		b := mgr.GetBot(botUUID, platform)
		if b == nil {
			return
		}
		opts := &imbot.SendMessageOptions{Text: reply, ParseMode: imbot.ParseModeMarkdown}
		bot.ForwardReplyContext(opts, msg)
		if _, err := b.SendMessage(context.Background(), chatID, opts); err != nil {
			logrus.WithError(err).WithField("chat_id", chatID).Warn("subscription consumer send failed")
		}
	}

	// /subs is the one command this consumer owns: live state for this chat.
	if text == "/subs" {
		send(c.subsOverview(bound, chatStore, chatID))
		return true
	}

	// remote_agent keeps owning its own handoff (@cc/@tb and friends) and
	// every other /-command, even in a sticky-subscription chat, so /stop,
	// /help, and switching away all keep working.
	if _, isHandoff, _ := smart_guide.DetectHandoffCommand(text); isHandoff {
		return false
	}
	if strings.HasPrefix(text, "/") {
		return false
	}

	// Tier 3 — explicit mention: "@name" or "@name trailing text" performs a
	// sticky handoff (CurrentAgent = sub:<uuid>) and enqueues the trailing
	// text when present.
	if sub, trailing, ok := matchMention(bound, text); ok {
		if err := chatStore.SetCurrentAgent(chatID, string(platform), sub.CurrentAgentValue()); err != nil {
			logrus.WithError(err).WithField("chat_id", chatID).Warn("subscription handoff: SetCurrentAgent failed")
		}
		if trailing != "" {
			c.enqueue(sub, msg, chatID, trailing)
		}
		send(fmt.Sprintf("🔗 Now talking to %s%s. Plain messages go to it — send @tb or @cc to switch back.",
			sub.AttributionPrefix(), c.onlineSuffix(sub)))
		c.reactReceived(mgr, botUUID, platform, msg)
		return true
	}

	// Tier 2 — reply-to: answering a message the subscription sent routes
	// this one message to it without touching the sticky state.
	if parentID := replyParentID(msg); parentID != "" && c.sends != nil {
		if subUUID := c.sends.Lookup(chatID, parentID); subUUID != "" {
			if sub, ok := findByUUID(bound, subUUID); ok {
				c.enqueue(sub, msg, chatID, text)
				c.reactReceived(mgr, botUUID, platform, msg)
				return true
			}
		}
	}

	// Sticky: the chat's current peer is a subscription.
	if agent, err := chatStore.GetCurrentAgent(chatID); err == nil {
		if subUUID := subscription.SubscriptionUUIDFromCurrentAgent(agent); subUUID != "" {
			if sub, ok := findByUUID(bound, subUUID); ok {
				c.enqueue(sub, msg, chatID, text)
				c.reactReceived(mgr, botUUID, platform, msg)
				return true
			}
			// Self-heal: the sticky target is gone (deleted/disabled/moved).
			// Reset and fall through so the message reaches the normal agent
			// path instead of a dead letter.
			if err := chatStore.SetCurrentAgent(chatID, string(platform), ""); err != nil {
				logrus.WithError(err).WithField("chat_id", chatID).Warn("subscription self-heal: reset CurrentAgent failed")
			}
		}
	}

	// Tier 1 — exclusive binding: every plain message in this chat is for
	// the subscription.
	for _, sub := range bound {
		if sub.Exclusive {
			c.enqueue(sub, msg, chatID, text)
			c.reactReceived(mgr, botUUID, platform, msg)
			return true
		}
	}

	return false
}

// boundSubscriptions returns the enabled subscriptions bound to this exact
// chat on this bot.
func (c *Consumer) boundSubscriptions(botUUID, chatID string) []subscription.Subscription {
	subs, err := c.store.ListByBot(botUUID)
	if err != nil {
		logrus.WithError(err).Warn("subscription consumer: list failed")
		return nil
	}
	out := subs[:0]
	for _, sub := range subs {
		if sub.Enabled && sub.ChatID == chatID {
			out = append(out, sub)
		}
	}
	return out
}

func (c *Consumer) enqueue(sub subscription.Subscription, msg imbot.Message, chatID, text string) {
	ev := subscription.Event{
		ChatID:    chatID,
		SenderID:  msg.Sender.ID,
		MessageID: msg.ID,
		Text:      text,
	}
	if msg.Metadata != nil {
		ev.ContextToken, _ = msg.Metadata["context_token"].(string)
	}
	if err := c.mailbox.Enqueue(sub, ev); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"subscription": sub.UUID,
			"chat_id":      chatID,
		}).Error("subscription enqueue failed")
	}
}

// reactReceived acknowledges a claimed message with the platform's
// "received" reaction, best-effort — the sender should see the message
// landed even when the tool answers much later.
func (c *Consumer) reactReceived(mgr *imbot.Manager, botUUID string, platform imbot.Platform, msg imbot.Message) {
	if msg.ID == "" {
		return
	}
	b := mgr.GetBot(botUUID, platform)
	if b == nil {
		return
	}
	emoji := imbot.ResolveReaction(platform, imbot.ReactionToken(imbot.ReactionReceived))
	_ = b.React(context.Background(), msg.ID, emoji)
}

// subsOverview renders the /subs answer: this chat's subscriptions, their
// live poller state, and the current sticky target.
func (c *Consumer) subsOverview(bound []subscription.Subscription, chatStore agentState, chatID string) string {
	currentUUID := ""
	if agent, err := chatStore.GetCurrentAgent(chatID); err == nil {
		currentUUID = subscription.SubscriptionUUIDFromCurrentAgent(agent)
	}
	var sb strings.Builder
	sb.WriteString("📡 *Subscriptions in this chat*\n")
	for _, sub := range bound {
		marker := "•"
		if sub.UUID == currentUUID {
			marker = "▶"
		}
		sb.WriteString(fmt.Sprintf("%s @%s%s", marker, sub.Name, c.onlineSuffix(sub)))
		if sub.Exclusive {
			sb.WriteString(" — exclusive")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\nSend `@<name>` to talk to one; @tb / @cc to return to the agents.")
	return sb.String()
}

// onlineSuffix marks whether the tool is connected right now (a poller is
// waiting on the mailbox).
func (c *Consumer) onlineSuffix(sub subscription.Subscription) string {
	if c.mailbox != nil && c.mailbox.HasWaiter(sub.UUID) {
		return " (online)"
	}
	return " (offline — messages are queued)"
}

// matchMention finds a bound subscription addressed as "@name" or
// "@name trailing…". Case-insensitive on the name.
func matchMention(bound []subscription.Subscription, text string) (subscription.Subscription, string, bool) {
	if !strings.HasPrefix(text, "@") {
		return subscription.Subscription{}, "", false
	}
	lower := strings.ToLower(text)
	for _, sub := range bound {
		prefix := "@" + sub.Name
		if lower == prefix {
			return sub, "", true
		}
		if strings.HasPrefix(lower, prefix+" ") {
			return sub, strings.TrimSpace(text[len(prefix)+1:]), true
		}
	}
	return subscription.Subscription{}, "", false
}

func findByUUID(bound []subscription.Subscription, uuid string) (subscription.Subscription, bool) {
	for _, sub := range bound {
		if sub.UUID == uuid {
			return sub, true
		}
	}
	return subscription.Subscription{}, false
}

// replyParentID extracts the platform message id this message replies to.
func replyParentID(msg imbot.Message) string {
	if msg.ThreadContext == nil {
		return ""
	}
	return msg.ThreadContext.ParentMessageID
}

var _ bot.Consumer = (*Consumer)(nil)
