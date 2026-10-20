package projects

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"

	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/control/remoteagent"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// ChatBridge joins Remote's IM chats to Projects (.design/projects-im.md):
//
//   - inbound: a chat that is bound to a project hands its messages to that
//     project's coordinator instead of @cc / @tb (remoteagent.ProjectRouter);
//   - outbound: the coordinator's replies and thread reports reach the bound
//     chats through the bot's channel, the same send path notifications use
//     (projectsapp.ChannelSink, implemented by Host).
//
// The bridge exists before Projects does (the IM handler is built first), so
// it is created empty and bound to the app and the channel registry later.
// Until then, or when the Projects flag is off, it takes no messages.
type ChatBridge struct {
	mu       sync.RWMutex
	app      *projectsapp.App
	registry *channel.Registry
	enabled  func() bool
	start    func()
	// disabled reports a blocklisted chat; nothing is sent to one.
	disabled func(chatID string) bool
	// bots remembers which bot a bound chat last wrote through, so replies
	// go back through the same one when several bots share a platform.
	bots map[string]string
}

var (
	_ remoteagent.ProjectRouter = (*ChatBridge)(nil)
	_ projectsapp.ChannelSink   = (*ChatBridge)(nil)
)

// NewChatBridge returns a bridge that does nothing until Bind.
func NewChatBridge() *ChatBridge {
	return &ChatBridge{bots: map[string]string{}}
}

// Bind connects the bridge to Projects and to the running bots' channels.
// enabled is the Projects feature flag and start brings the runtime up on
// first use, like the HTTP gate does; disabled may be nil.
func (b *ChatBridge) Bind(app *projectsapp.App, registry *channel.Registry, enabled func() bool, start func(), disabled func(chatID string) bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.app, b.registry, b.enabled, b.start, b.disabled = app, registry, enabled, start, disabled
}

func chatKey(platform, chatID string) string { return platform + "\x00" + chatID }

// live returns the app when Projects is switched on.
func (b *ChatBridge) live() *projectsapp.App {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.app == nil || b.enabled == nil || !b.enabled() {
		return nil
	}
	return b.app
}

// Bound implements remoteagent.ProjectRouter.
func (b *ChatBridge) Bound(platform, chatID string) bool {
	app := b.live()
	return app != nil && app.ChannelBound(platform, chatID)
}

// RouteInbound implements remoteagent.ProjectRouter.
func (b *ChatBridge) RouteInbound(_ context.Context, in remoteagent.ProjectInbound) (bool, error) {
	app := b.live()
	if app == nil || !app.ChannelBound(in.Platform, in.ChatID) {
		return false, nil
	}
	b.mu.Lock()
	b.bots[chatKey(in.Platform, in.ChatID)] = in.BotUUID
	start := b.start
	b.mu.Unlock()
	if start != nil {
		start()
	}
	return app.PostFromChannel(in.Platform, in.ChatID, in.Text)
}

// RouteCommand implements remoteagent.ProjectRouter.
func (b *ChatBridge) RouteCommand(_ context.Context, name string, in remoteagent.ProjectInbound) (bool, string, error) {
	app := b.live()
	if app == nil || !app.ChannelBound(in.Platform, in.ChatID) {
		return false, "", nil
	}
	b.mu.Lock()
	b.bots[chatKey(in.Platform, in.ChatID)] = in.BotUUID
	start := b.start
	b.mu.Unlock()
	if start != nil {
		start()
	}
	return app.ChannelCommand(in.Platform, in.ChatID, name, in.Text)
}

var _ projectsapp.ChannelSink = (*Host)(nil)

// SetChatBridge gives the host the bridge it sends chat messages through.
func (h *Host) SetChatBridge(b *ChatBridge) { h.bridge = b }

// SendChannelText is the host's half of Projects' optional ChannelSink: the
// coordinator's replies reach the IM chats bound to a project.
func (h *Host) SendChannelText(ctx context.Context, platform, chatID, text string) error {
	if h.bridge == nil {
		return ErrNoChannel
	}
	return h.bridge.SendChannelText(ctx, platform, chatID, text)
}

// ErrNoChannel means no running bot can reach the chat.
var ErrNoChannel = errors.New("no running bot for this chat")

// SendChannelText implements projectsapp.ChannelSink: it sends text to a chat
// through the channel of the bot the chat last wrote through, else through the
// bots of the chat's platform.
func (b *ChatBridge) SendChannelText(ctx context.Context, platform, chatID, text string) error {
	b.mu.RLock()
	reg, disabled, botUUID := b.registry, b.disabled, b.bots[chatKey(platform, chatID)]
	b.mu.RUnlock()
	if reg == nil {
		return ErrNoChannel
	}
	if disabled != nil && disabled(chatID) {
		// A blocklisted chat gets nothing, bound or not.
		return nil
	}
	send := func(ch channel.Channel) error {
		return ch.Send(ctx, channel.Target{ChatID: chatID}, interaction.Notification{Body: text})
	}
	if botUUID != "" {
		if ch, ok := reg.Get(botUUID); ok {
			return send(ch)
		}
	}
	// Nothing written yet since the server started (or that bot stopped): try
	// the platform's bots. A bot that does not know the chat fails, and the
	// next one is tried.
	var errs []error
	for _, ch := range reg.ByPlatform(platform) {
		err := send(ch)
		if err == nil {
			b.mu.Lock()
			b.bots[chatKey(platform, chatID)] = ch.ID()
			b.mu.Unlock()
			return nil
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return fmt.Errorf("%w: %s", ErrNoChannel, platform)
	}
	logrus.WithField("platform", platform).Debug("projects: chat unreachable through every bot of its platform")
	return errors.Join(errs...)
}
