package telegram

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"golang.org/x/net/proxy"
)

// Bot implements the Telegram bot
type Bot struct {
	*core.BaseBot
	api     *tgbotapi.BotAPI
	adapter *Adapter // Local adapter for message conversion
	updates tgbotapi.UpdatesChannel
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.RWMutex
}

// NewTelegramBot creates a new Telegram bot
func NewTelegramBot(config *core.Config) (*Bot, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if config.Auth.Type != "token" {
		return nil, core.NewAuthFailedError(config.Platform, "telegram requires token auth", nil)
	}

	token, err := config.Auth.GetToken()
	if err != nil {
		return nil, core.NewAuthFailedError(config.Platform, "failed to get token", err)
	}

	apiEndpoint := config.GetOptionString("apiURL", tgbotapi.APIEndpoint)
	proxyURL := config.GetOptionString("proxy", "")

	client := &http.Client{}
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, core.NewAuthFailedError(core.PlatformTelegram, "invalid proxy url", err)
		}
		switch strings.ToLower(parsed.Scheme) {
		case "socks5", "socks5h":
			dialer, err := proxy.FromURL(parsed, proxy.Direct)
			if err != nil {
				return nil, core.NewAuthFailedError(core.PlatformTelegram, "invalid socks5 proxy", err)
			}
			client.Transport = &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				},
			}
		case "http", "https":
			client.Transport = &http.Transport{
				Proxy: http.ProxyURL(parsed),
			}
		default:
			return nil, core.NewAuthFailedError(core.PlatformTelegram, "unsupported proxy scheme", fmt.Errorf("%s", parsed.Scheme))
		}
	}

	api, err := tgbotapi.NewBotAPIWithClient(token, apiEndpoint, client)
	if err != nil {
		return nil, core.NewAuthFailedError(core.PlatformTelegram, "failed to create telegram bot", err)
	}

	bot := &Bot{
		BaseBot: core.NewBaseBot(config),
		api:     api,
	}

	// Set debug mode if enabled
	if config.GetOptionBool("debug", false) {
		api.Debug = true
	}

	return bot, nil
}

// Connect connects to Telegram
func (b *Bot) Connect(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Get update timeout
	timeout := b.Config().GetOptionInt("updateTimeout", 30)

	// Set up updates
	u := tgbotapi.NewUpdate(0)
	u.Timeout = timeout

	b.updates = b.api.GetUpdatesChan(u)
	b.UpdateConnected(true)
	b.UpdateAuthenticated(true)
	b.EmitConnected()
	b.Logger().Info("Telegram bot connected: @%s", b.api.Self.UserName)

	// Initialize adapter for message conversion
	b.adapter = NewAdapter(b.Config(), b.api)

	// Start receiving messages
	b.wg.Add(1)
	go b.receiveUpdates()

	return nil
}

// Disconnect disconnects from Telegram
func (b *Bot) Disconnect(ctx context.Context) error {
	if b.cancel != nil {
		b.cancel()
	}

	if b.updates != nil {
		b.api.StopReceivingUpdates()
	}

	b.wg.Wait()

	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()
	b.Logger().Info("Telegram bot disconnected")

	return nil
}

// SendMessage sends a message
func (b *Bot) SendMessage(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	// Parse target as chat ID
	chatID, err := strconv.ParseInt(target, 10, 64)
	if err != nil {
		return nil, core.NewInvalidTargetError(core.PlatformTelegram, target, "invalid chat ID")
	}

	// Handle text message
	if opts.Text != "" {
		return b.sendText(ctx, chatID, opts)
	}

	// Handle media
	if len(opts.Media) > 0 {
		return b.sendMedia(ctx, chatID, opts)
	}

	return nil, core.NewBotError(core.ErrUnknown, "no content to send", false)
}

// SendText sends a text message
func (b *Bot) SendText(ctx context.Context, target string, text string) (*core.SendResult, error) {
	return b.SendMessage(ctx, target, &core.SendMessageOptions{
		Text: text,
	})
}

// SendMedia sends media
func (b *Bot) SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error) {
	return b.SendMessage(ctx, target, &core.SendMessageOptions{
		Media: media,
	})
}

// React reacts to a message
func (b *Bot) React(ctx context.Context, messageID string, emoji string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	// Parse message ID
	_, err := strconv.Atoi(messageID)
	if err != nil {
		return core.NewBotError(core.ErrInvalidTarget, "invalid message ID", false)
	}

	// Get chat ID from context or use a default
	// In a real implementation, you'd need to track chat IDs
	chatID := int64(0) // This would need to be tracked

	// Send reaction (note: Telegram uses setMessageReaction API)
	// For now, we'll send the emoji as a message
	_, err = b.api.Send(tgbotapi.NewMessage(chatID, emoji))
	return err
}

// EditMessage edits a message
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	// Parse message ID and chat ID
	// In a real implementation, you'd need to track these
	// For now, this is a placeholder
	b.Logger().Debug("Edit message: %s", messageID)
	return nil
}

// DeleteMessage deletes a message
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	// Parse message ID and chat ID
	// In a real implementation, you'd need to track these
	b.Logger().Debug("Delete message: %s", messageID)
	return nil
}

// PlatformInfo returns platform information
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(core.PlatformTelegram, "Telegram")
}

// StartReceiving starts receiving messages (already started in Connect)
func (b *Bot) StartReceiving(ctx context.Context) error {
	return nil // Already started in Connect
}

// StopReceiving stops receiving messages (already handled in Disconnect)
func (b *Bot) StopReceiving(ctx context.Context) error {
	return nil // Already handled in Disconnect
}

// receiveUpdates receives and processes updates from Telegram
func (b *Bot) receiveUpdates() {
	defer b.wg.Done()

	b.UpdateReady(true)
	b.EmitReady()

	for {
		select {
		case <-b.ctx.Done():
			return
		case update, ok := <-b.updates:
			if !ok {
				return
			}

			if update.Message != nil {
				b.handleMessage(update.Message)
			} else if update.CallbackQuery != nil {
				b.handleCallbackQuery(update.CallbackQuery)
			}
		}
	}
}

// handleMessage handles an incoming message
func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	// Use adapter to convert platform message to core message
	coreMessage, err := b.adapter.AdaptMessage(b.ctx, msg)
	if err != nil {
		b.Logger().Error("Failed to adapt message: %v", err)
		return
	}

	b.EmitMessage(*coreMessage)
}

// handleCallbackQuery handles a callback query (button click)
func (b *Bot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	b.Logger().Debug("Received callback query from %d: %s", query.From.ID, query.Data)

	// Use adapter to convert callback to core message
	coreMessage, err := b.adapter.AdaptCallback(b.ctx, query)
	if err != nil {
		b.Logger().Error("Failed to adapt callback: %v", err)
		return
	}

	b.EmitMessage(*coreMessage)

	// Answer the callback query to remove loading state
	callbackCfg := tgbotapi.NewCallback(query.ID, "")
	if _, err := b.api.Request(callbackCfg); err != nil {
		b.Logger().Error("Failed to answer callback query: %v", err)
	}
}

// sendText sends a text message
func (b *Bot) sendText(ctx context.Context, chatID int64, opts *core.SendMessageOptions) (*core.SendResult, error) {
	// Validate and chunk text if needed
	if err := b.ValidateTextLength(opts.Text); err != nil {
		return nil, err
	}

	chunks := b.ChunkText(opts.Text)

	var lastResult *core.SendResult
	for _, chunk := range chunks {
		msg := tgbotapi.NewMessage(chatID, chunk)

		// Set parse mode
		if opts.ParseMode != "" {
			switch opts.ParseMode {
			case core.ParseModeMarkdown:
				msg.ParseMode = tgbotapi.ModeMarkdown
			case core.ParseModeHTML:
				msg.ParseMode = tgbotapi.ModeHTML
			}
		}

		// Set reply to
		if opts.ReplyTo != "" {
			if replyToID, err := strconv.Atoi(opts.ReplyTo); err == nil {
				msg.ReplyToMessageID = replyToID
			}
		}

		// Disable notification if silent
		if opts.Silent {
			msg.DisableNotification = true
		}

		// Set reply markup (inline keyboard) from metadata
		if opts.Metadata != nil {
			if markup, ok := opts.Metadata["replyMarkup"]; ok {
				if replyMarkup, ok := markup.(tgbotapi.InlineKeyboardMarkup); ok {
					msg.ReplyMarkup = replyMarkup
				}
			}
		}

		sentMsg, err := b.api.Send(msg)
		if err != nil {
			return nil, core.WrapError(err, core.PlatformTelegram, core.ErrPlatformError)
		}

		lastResult = &core.SendResult{
			MessageID: strconv.Itoa(sentMsg.MessageID),
			Timestamp: int64(sentMsg.Date),
		}
	}

	b.UpdateLastActivity()
	return lastResult, nil
}

// sendMedia sends media
func (b *Bot) sendMedia(ctx context.Context, chatID int64, opts *core.SendMessageOptions) (*core.SendResult, error) {
	// For now, just send the first media item as a photo/document
	if len(opts.Media) == 0 {
		return nil, core.NewBotError(core.ErrUnknown, "no media to send", false)
	}

	media := opts.Media[0]

	var msg tgbotapi.Chattable

	if media.Type == "image" || media.Type == "sticker" {
		// Send as photo
		photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(media.URL))
		if opts.Text != "" {
			photoMsg.Caption = opts.Text
		}
		msg = photoMsg
	} else {
		// Send as document
		docMsg := tgbotapi.NewDocument(chatID, tgbotapi.FileURL(media.URL))
		if opts.Text != "" {
			docMsg.Caption = opts.Text
		}
		msg = docMsg
	}

	sentMsg, err := b.api.Send(msg)
	if err != nil {
		return nil, core.WrapError(err, core.PlatformTelegram, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return &core.SendResult{
		MessageID: strconv.Itoa(sentMsg.MessageID),
		Timestamp: int64(sentMsg.Date),
	}, nil
}
