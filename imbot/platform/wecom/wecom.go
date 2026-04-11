// Package wecom provides WeCom (Enterprise WeChat) platform bot implementation for ImBot.
package wecom

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/weixin/types"
	"github.com/tingly-dev/weixin/wecom"
)

// Bot implements the core.Bot interface for WeCom (Enterprise WeChat AI Bot).
// It wraps wecom.WecomBot which manages the WebSocket connection.
type Bot struct {
	*core.BaseBot
	sdk     *wecom.WecomBot
	botID   string
	secret  string
	adapter *Adapter
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewBot creates a new WeCom bot from the given config.
// Auth type must be "oauth" with non-empty ClientID (botID) and ClientSecret.
func NewBot(config *core.Config) (*Bot, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	botID := config.Auth.ClientID
	secret := config.Auth.ClientSecret
	if botID == "" {
		return nil, fmt.Errorf("wecom: clientId (bot ID) is required")
	}
	if secret == "" {
		return nil, fmt.Errorf("wecom: clientSecret (bot secret) is required")
	}

	sdk := wecom.NewWecomBot(&wecom.WecomConfig{})

	bot := &Bot{
		BaseBot: core.NewBaseBot(config),
		sdk:     sdk,
		botID:   botID,
		secret:  secret,
		adapter: NewAdapter(config),
	}

	return bot, nil
}

// Connect establishes a WebSocket connection to WeCom.
func (b *Bot) Connect(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Register SDK event handler before connecting.
	b.sdk.SetEventHandler(&sdkEventHandler{bot: b})

	if err := b.sdk.Connect(b.ctx, b.botID, b.secret); err != nil {
		return core.NewAuthFailedError(core.PlatformWecom, fmt.Sprintf("connect failed: %v", err), err)
	}

	b.UpdateConnected(true)
	b.UpdateAuthenticated(true)

	b.wg.Add(1)
	go b.runUntilDone()

	return nil
}

// runUntilDone waits for context cancellation then marks the bot as disconnected.
func (b *Bot) runUntilDone() {
	defer b.wg.Done()

	b.UpdateReady(true)
	b.EmitReady()
	b.Logger().Info("WeCom bot ready: botID=%s", b.botID)

	<-b.ctx.Done()

	b.UpdateReady(false)
	b.UpdateConnected(false)
}

// Disconnect closes the WebSocket connection.
func (b *Bot) Disconnect(ctx context.Context) error {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()

	_ = b.sdk.Disconnect()

	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()
	b.Logger().Info("WeCom bot disconnected")

	return nil
}

// IsConnected reports whether the bot is connected.
func (b *Bot) IsConnected() bool {
	return b.sdk.IsConnected() && b.BaseBot.IsConnected()
}

// SendMessage sends a message to the given target.
// When opts.Metadata["context_token"] is set, the message is sent as a reply
// (passive stream response). Otherwise a proactive markdown message is sent.
func (b *Bot) SendMessage(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	if opts.Text != "" && len(opts.Media) == 0 {
		return b.sendText(ctx, target, opts)
	}
	if len(opts.Media) > 0 {
		return b.sendMedia(ctx, target, opts)
	}
	return nil, core.NewBotError(core.ErrUnknown, "no content to send", false)
}

// SendText sends a plain text message.
func (b *Bot) SendText(ctx context.Context, target string, text string) (*core.SendResult, error) {
	return b.SendMessage(ctx, target, &core.SendMessageOptions{Text: text})
}

// SendMedia sends media attachments. Requires wecom_media_id in opts.Metadata.
func (b *Bot) SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error) {
	return b.SendMessage(ctx, target, &core.SendMessageOptions{Media: media})
}

// React is not supported on WeCom.
func (b *Bot) React(ctx context.Context, messageID string, emoji string) error {
	return core.NewBotError(core.ErrPlatformError, "reactions not supported on WeCom", false)
}

// EditMessage is not supported on WeCom.
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	return core.NewBotError(core.ErrPlatformError, "editing messages not supported on WeCom", false)
}

// DeleteMessage is not supported on WeCom.
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	return core.NewBotError(core.ErrPlatformError, "deleting messages not supported on WeCom", false)
}

// PlatformInfo returns WeCom platform metadata.
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(core.PlatformWecom, "WeCom")
}

// StartReceiving is a no-op; receiving is handled inside Connect.
func (b *Bot) StartReceiving(ctx context.Context) error { return nil }

// StopReceiving is a no-op; receiving is stopped via Disconnect.
func (b *Bot) StopReceiving(ctx context.Context) error { return nil }

// Close releases resources.
func (b *Bot) Close() error {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	return nil
}

// ---------------------------------------------------------------------------
// sdkEventHandler bridges types.EventHandler → Bot callbacks
// ---------------------------------------------------------------------------

// sdkEventHandler implements types.EventHandler and forwards events to the Bot.
type sdkEventHandler struct {
	bot *Bot
}

func (h *sdkEventHandler) OnMessage(ctx context.Context, msg *types.Message) error {
	h.bot.handleIncomingMessage(ctx, msg)
	return nil
}

func (h *sdkEventHandler) OnReaction(ctx context.Context, reaction *types.Reaction) error {
	return nil // WeCom AI Bot does not emit reaction events
}

func (h *sdkEventHandler) OnEdit(ctx context.Context, msg *types.Message) error {
	return nil // WeCom AI Bot does not emit edit events
}

func (h *sdkEventHandler) OnEvent(ctx context.Context, event *types.Event) {
	if event == nil {
		return
	}
	switch event.EventType {
	case "disconnected":
		h.bot.Logger().Error("WeCom WebSocket disconnected")
		h.bot.UpdateConnected(false)
		h.bot.UpdateReady(false)
		h.bot.EmitDisconnected()
	default:
		h.bot.Logger().Info("WeCom event: type=%s", event.EventType)
	}
}

func (h *sdkEventHandler) OnError(ctx context.Context, err error) {
	h.bot.Logger().Error("WeCom SDK error: %v", err)
	h.bot.EmitError(err)
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// handleIncomingMessage adapts the SDK message and fires it to all registered
// OnMessage listeners on the BaseBot.
func (b *Bot) handleIncomingMessage(ctx context.Context, msg *types.Message) {
	coreMsg, err := b.adapter.AdaptMessage(ctx, msg)
	if err != nil {
		b.Logger().Error("Failed to adapt WeCom message: %v", err)
		return
	}
	b.EmitMessage(*coreMsg)
}

// sendText sends a text message, using stream reply when a context_token is available.
func (b *Bot) sendText(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if err := b.ValidateTextLength(opts.Text); err != nil {
		return nil, err
	}

	contextToken := ""
	if opts.Metadata != nil {
		contextToken, _ = opts.Metadata["context_token"].(string)
	}

	outMsg := &types.OutboundMessage{
		To:           target,
		Text:         opts.Text,
		ContextToken: contextToken,
	}

	result, err := b.sdk.Send(ctx, outMsg)
	if err != nil {
		return nil, core.WrapError(err, core.PlatformWecom, core.ErrPlatformError)
	}
	if !result.OK {
		return nil, core.NewBotError(core.ErrPlatformError, result.Error, false)
	}

	b.UpdateLastActivity()
	return &core.SendResult{
		MessageID: result.MessageID,
		Timestamp: time.Now().Unix(),
	}, nil
}

// sendMedia forwards media send to the SDK (requires wecom_media_id in Metadata).
func (b *Bot) sendMedia(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if len(opts.Media) == 0 {
		return nil, core.NewBotError(core.ErrUnknown, "no media to send", false)
	}

	item := opts.Media[0]
	contextToken := ""
	if opts.Metadata != nil {
		contextToken, _ = opts.Metadata["context_token"].(string)
	}

	outMsg := &types.OutboundMessage{
		To:           target,
		ContentType:  item.Type,
		FileName:     item.Filename,
		ContextToken: contextToken,
		Metadata:     opts.Metadata,
	}

	result, err := b.sdk.SendMedia(ctx, outMsg)
	if err != nil {
		return nil, core.WrapError(err, core.PlatformWecom, core.ErrMediaNotSupported)
	}
	if !result.OK {
		return nil, core.NewBotError(core.ErrPlatformError, result.Error, false)
	}

	b.UpdateLastActivity()
	return &core.SendResult{
		MessageID: result.MessageID,
		Timestamp: time.Now().Unix(),
	}, nil
}
