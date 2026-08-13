// Package peerconsumer implements the peer purpose as a bot.Consumer: it
// claims inbound chat messages addressed to a Peer (see remote/peer and
// .design/peer.md §6) and enqueues them into the peer's inbound stream. It
// sits between the host's prompt-reply router and the remote_agent catch-all
// in dispatch order.
//
// The security gate is the binding itself: claim rules only ever run for
// messages arriving in a chat some enabled peer is bound to; unbound chats
// never reach a peer (spec §3 — binding IS the authorization).
package peerconsumer

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/smart_guide"
	"github.com/tingly-dev/tingly-box/remote/peer"
)

// ConsumerName identifies the peer purpose in logs and dispatch diagnostics.
// Like notify it is not a stored capability: it is mounted implicitly by the
// existence of enabled peers ("a reason to run").
const ConsumerName = "peer"

// agentState is the narrow slice of the chat store this consumer touches:
// the per-chat sticky peer. bot.ChatStoreInterface satisfies it.
type agentState interface {
	SetCurrentAgent(chatID, platform, agentType string) error
	GetCurrentAgent(chatID string) (string, error)
}

// Consumer is the peer purpose. One instance serves every bot the lifecycle
// manager runs; per-bot state is resolved from the store per message
// (human-scale traffic, and it keeps peers created while a bot is running
// visible without a restart).
type Consumer struct {
	store peer.Store
	inbox *peer.Inbox
	// sends resolves reply-to addressing: platform message ids recently sent
	// by each peer, per chat (tier 2).
	sends *peer.RecentSends
}

// New builds the consumer. inbox and sends may be shared with the HTTP
// module so outbound sends and inbound claims see the same state.
func New(store peer.Store, inbox *peer.Inbox, sends *peer.RecentSends) *Consumer {
	return &Consumer{store: store, inbox: inbox, sends: sends}
}

// Name identifies this purpose.
func (c *Consumer) Name() string { return ConsumerName }

// Mounted reports whether the bot has a reason to run for this purpose: at
// least one enabled peer bound to it. Mirrors notify's implicit,
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

	bound := c.boundPeers(botUUID, chatID)
	if len(bound) == 0 {
		// Rule 0: unbound chat — nothing here belongs to a peer.
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
			logrus.WithError(err).WithField("chat_id", chatID).Warn("peer consumer send failed")
		}
	}

	// /peers is the one command this consumer owns: live state for this chat.
	if text == "/peers" {
		send(c.peersOverview(bound, chatStore, chatID))
		return true
	}

	// remote_agent keeps owning its own handoff (@cc/@tb and friends) and
	// every other /-command, even in a sticky-peer chat, so /stop, /help,
	// and switching away all keep working.
	if _, isHandoff, _ := smart_guide.DetectHandoffCommand(text); isHandoff {
		return false
	}
	if strings.HasPrefix(text, "/") {
		return false
	}

	// Tier 3 — explicit mention: "@name" or "@name trailing text" performs a
	// sticky handoff (CurrentAgent = peer:<uuid>) and enqueues the trailing
	// text when present.
	if p, trailing, ok := matchMention(bound, text); ok {
		if err := chatStore.SetCurrentAgent(chatID, string(platform), p.CurrentAgentValue()); err != nil {
			logrus.WithError(err).WithField("chat_id", chatID).Warn("peer handoff: SetCurrentAgent failed")
		}
		if trailing != "" {
			c.enqueue(p, msg, chatID, trailing)
		}
		send(fmt.Sprintf("🔗 Now talking to %s%s. Plain messages go to it — send @tb or @cc to switch back.",
			p.AttributionPrefix(), c.onlineSuffix(p)))
		c.reactReceived(mgr, botUUID, platform, msg)
		return true
	}

	// Tier 2 — reply-to: answering a message the peer sent routes this one
	// message to it without touching the sticky state.
	if parentID := replyParentID(msg); parentID != "" && c.sends != nil {
		if peerUUID := c.sends.Lookup(chatID, parentID); peerUUID != "" {
			if p, ok := findByUUID(bound, peerUUID); ok {
				c.enqueue(p, msg, chatID, text)
				c.reactReceived(mgr, botUUID, platform, msg)
				return true
			}
		}
	}

	// Sticky: the chat's current peer marker points at a peer.
	if agent, err := chatStore.GetCurrentAgent(chatID); err == nil {
		if peerUUID := peer.PeerUUIDFromCurrentAgent(agent); peerUUID != "" {
			if p, ok := findByUUID(bound, peerUUID); ok {
				c.enqueue(p, msg, chatID, text)
				c.reactReceived(mgr, botUUID, platform, msg)
				return true
			}
			// Self-heal: the sticky target is gone (deleted/disabled/moved).
			// Reset and fall through so the message reaches the normal agent
			// path instead of a dead letter.
			if err := chatStore.SetCurrentAgent(chatID, string(platform), ""); err != nil {
				logrus.WithError(err).WithField("chat_id", chatID).Warn("peer self-heal: reset CurrentAgent failed")
			}
		}
	}

	// Tier 1 — exclusive binding: every plain message in this chat is for
	// the peer.
	for _, p := range bound {
		if p.Exclusive {
			c.enqueue(p, msg, chatID, text)
			c.reactReceived(mgr, botUUID, platform, msg)
			return true
		}
	}

	return false
}

// boundPeers returns the enabled peers bound to this exact chat on this bot.
func (c *Consumer) boundPeers(botUUID, chatID string) []peer.Peer {
	peers, err := c.store.ListByBot(botUUID)
	if err != nil {
		logrus.WithError(err).Warn("peer consumer: list failed")
		return nil
	}
	out := peers[:0]
	for _, p := range peers {
		if p.Enabled && p.ChatID == chatID {
			out = append(out, p)
		}
	}
	return out
}

func (c *Consumer) enqueue(p peer.Peer, msg imbot.Message, chatID, text string) {
	u := peer.Update{
		Type:      peer.UpdateTypeMessage,
		ChatID:    chatID,
		SenderID:  msg.Sender.ID,
		MessageID: msg.ID,
		Text:      text,
	}
	if msg.Metadata != nil {
		u.ContextToken, _ = msg.Metadata["context_token"].(string)
	}
	if err := c.inbox.Enqueue(p, u); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"peer":    p.UUID,
			"chat_id": chatID,
		}).Error("peer enqueue failed")
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

// peersOverview renders the /peers answer: this chat's peers, their live
// poller state, and the current sticky target.
func (c *Consumer) peersOverview(bound []peer.Peer, chatStore agentState, chatID string) string {
	currentUUID := ""
	if agent, err := chatStore.GetCurrentAgent(chatID); err == nil {
		currentUUID = peer.PeerUUIDFromCurrentAgent(agent)
	}
	var sb strings.Builder
	sb.WriteString("📡 *Peers in this chat*\n")
	for _, p := range bound {
		marker := "•"
		if p.UUID == currentUUID {
			marker = "▶"
		}
		sb.WriteString(fmt.Sprintf("%s @%s%s", marker, p.Name, c.onlineSuffix(p)))
		if p.Exclusive {
			sb.WriteString(" — exclusive")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\nSend `@<name>` to talk to one; @tb / @cc to return to the agents.")
	return sb.String()
}

// onlineSuffix marks whether the tool is connected right now (a poller is
// waiting on the inbox).
func (c *Consumer) onlineSuffix(p peer.Peer) string {
	if c.inbox != nil && c.inbox.HasWaiter(p.UUID) {
		return " (online)"
	}
	return " (offline — messages are queued)"
}

// matchMention finds a bound peer addressed as "@name" or "@name trailing…".
// Case-insensitive on the name.
func matchMention(bound []peer.Peer, text string) (peer.Peer, string, bool) {
	if !strings.HasPrefix(text, "@") {
		return peer.Peer{}, "", false
	}
	lower := strings.ToLower(text)
	for _, p := range bound {
		prefix := "@" + p.Name
		if lower == prefix {
			return p, "", true
		}
		if strings.HasPrefix(lower, prefix+" ") {
			return p, strings.TrimSpace(text[len(prefix)+1:]), true
		}
	}
	return peer.Peer{}, "", false
}

func findByUUID(bound []peer.Peer, uuid string) (peer.Peer, bool) {
	for _, p := range bound {
		if p.UUID == uuid {
			return p, true
		}
	}
	return peer.Peer{}, false
}

// replyParentID extracts the platform message id this message replies to.
func replyParentID(msg imbot.Message) string {
	if msg.ThreadContext == nil {
		return ""
	}
	return msg.ThreadContext.ParentMessageID
}

var _ bot.Consumer = (*Consumer)(nil)
