package tingly

import (
	"context"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// Bot is the tingly platform implementation of core.Bot.
//
// All outbound operations go through a Transport. Inbound messages arrive
// via Transport.Subscribe and are delivered to handlers registered through
// BaseBot.OnMessage / EmitMessage.
type Bot struct {
	*core.BaseBot
	transport Transport
}

// NewBot constructs a Bot bound to an explicit Transport. Callers that
// hold the transport directly (the testenv harness) use this.
func NewBot(config *core.Config, transport Transport) (*Bot, error) {
	if config == nil {
		return nil, core.NewBotError(core.ErrUnknown, "tingly: config is nil", false)
	}
	if transport == nil {
		transport = NewInProcessTransport()
	}
	return &Bot{
		BaseBot:   core.NewBaseBot(config),
		transport: transport,
	}, nil
}

// NewBotFromConfig is the platform-registry factory. It looks up a
// pre-registered transport for the bot UUID (registered by testenv via
// Register); if none exists, a fresh InProcessTransport is created.
func NewBotFromConfig(config *core.Config) (core.Bot, error) {
	var transport Transport
	if config != nil {
		transport = lookup(config.UUID)
	}
	return NewBot(config, transport)
}

// Transport exposes the underlying transport. testenv uses this to drive
// the bot when it didn't construct it directly.
func (b *Bot) Transport() Transport {
	return b.transport
}

// Connect implements core.Bot.
func (b *Bot) Connect(ctx context.Context) error {
	b.transport.Subscribe(func(msg core.Message) {
		b.EmitMessage(msg)
	})
	b.MarkConnected(true)
	b.MarkReady()
	b.Logger().Info("tingly bot connected")
	return nil
}

// Disconnect implements core.Bot.
func (b *Bot) Disconnect(ctx context.Context) error {
	b.MarkDisconnected()
	if b.transport != nil {
		_ = b.transport.Close()
	}
	b.Logger().Info("tingly bot disconnected")
	return nil
}

// SendMessage implements core.Bot.
func (b *Bot) SendMessage(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}
	b.UpdateLastActivity()
	return b.transport.Send(ctx, target, opts)
}

// SendText implements core.Bot.
func (b *Bot) SendText(ctx context.Context, target, text string) (*core.SendResult, error) {
	return b.SendMessage(ctx, target, &core.SendMessageOptions{Text: text})
}

// SendMedia implements core.Bot.
func (b *Bot) SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error) {
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}
	b.UpdateLastActivity()
	return b.transport.SendMedia(ctx, target, media)
}

// React implements core.Bot.
func (b *Bot) React(ctx context.Context, messageID, emoji string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}
	b.UpdateLastActivity()
	return b.transport.React(ctx, messageID, emoji)
}

// EditMessage implements core.Bot.
func (b *Bot) EditMessage(ctx context.Context, messageID, text string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}
	b.UpdateLastActivity()
	return b.transport.Edit(ctx, messageID, text)
}

// DeleteMessage implements core.Bot.
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}
	b.UpdateLastActivity()
	return b.transport.Delete(ctx, messageID)
}

// PlatformInfo implements core.Bot.
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(core.PlatformTingly, "Tingly")
}
