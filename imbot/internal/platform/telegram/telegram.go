package telegram

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Bot implements the Telegram bot
type Bot struct {
	*core.BaseBot
	api     *tgbotapi.BotAPI
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

	api, err := tgbotapi.NewBotAPI(token)
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
	// Determine chat type
	chatType := b.getChatType(msg)

	// Create sender
	sender := core.Sender{
		ID: strconv.FormatInt(msg.From.ID, 10),
	}

	if msg.From.UserName != "" {
		sender.Username = msg.From.UserName
	}

	if msg.From.FirstName != "" || msg.From.LastName != "" {
		sender.DisplayName = fmt.Sprintf("%s %s", msg.From.FirstName, msg.From.LastName)
	}

	// Create recipient
	recipient := core.Recipient{
		ID:   strconv.FormatInt(msg.Chat.ID, 10),
		Type: string(chatType),
	}

	if msg.Chat.Title != "" {
		recipient.DisplayName = msg.Chat.Title
	}

	// Extract content
	var content core.Content
	if msg.Text != "" {
		content = core.NewTextContent(msg.Text)
	} else if msg.Photo != nil && len(msg.Photo) > 0 {
		// Handle photo
		content = b.handlePhoto(msg)
	} else if msg.Document != nil {
		// Handle document
		content = b.handleDocument(msg)
	} else if msg.Sticker != nil {
		// Handle sticker
		content = b.handleSticker(msg)
	} else {
		// Unknown content type
		content = core.NewSystemContent("unknown", nil)
	}

	// Create message
	message := core.Message{
		ID:        strconv.Itoa(msg.MessageID),
		Platform:  core.PlatformTelegram,
		Timestamp: int64(msg.Date),
		Sender:    sender,
		Recipient: recipient,
		Content:   content,
		ChatType:  chatType,
		Metadata:  make(map[string]interface{}),
	}

	// Add thread context if reply
	if msg.ReplyToMessage != nil {
		message.ThreadContext = &core.ThreadContext{
			ID:              strconv.Itoa(msg.ReplyToMessage.MessageID),
			ParentMessageID: strconv.Itoa(msg.ReplyToMessage.MessageID),
		}
	}

	b.EmitMessage(message)
}

// handleCallbackQuery handles a callback query
func (b *Bot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	b.Logger().Debug("Received callback query from %d", query.From.ID)

	// Answer the callback query
	callbackCfg := tgbotapi.NewCallback(query.ID, "")
	if _, err := b.api.Request(callbackCfg); err != nil {
		b.Logger().Error("Failed to answer callback query: %v", err)
	}
}

// getChatType determines the chat type from the message
func (b *Bot) getChatType(msg *tgbotapi.Message) core.ChatType {
	switch msg.Chat.Type {
	case "private":
		return core.ChatTypeDirect
	case "group", "supergroup":
		return core.ChatTypeGroup
	case "channel":
		return core.ChatTypeChannel
	default:
		return core.ChatTypeDirect
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

// handlePhoto handles a photo message
func (b *Bot) handlePhoto(msg *tgbotapi.Message) core.Content {
	photos := msg.Photo
	if len(photos) == 0 {
		return core.NewSystemContent("photo", nil)
	}

	// Get the largest photo
	largest := photos[len(photos)-1]

	media := []core.MediaAttachment{
		{
			Type:   "image",
			URL:    fmt.Sprintf("file://%s", largest.FileID),
			Width:  largest.Width,
			Height: largest.Height,
			Raw:    map[string]interface{}{"fileUniqueId": largest.FileUniqueID},
		},
	}

	caption := msg.Caption
	return core.NewMediaContent(media, caption)
}

// handleDocument handles a document message
func (b *Bot) handleDocument(msg *tgbotapi.Message) core.Content {
	doc := msg.Document

	media := []core.MediaAttachment{
		{
			Type:     "document",
			URL:      fmt.Sprintf("file://%s", doc.FileID),
			MimeType: doc.MimeType,
			Filename: doc.FileName,
			Size:     int64(doc.FileSize),
			Raw:      map[string]interface{}{"fileUniqueId": doc.FileUniqueID},
		},
	}

	caption := msg.Caption
	return core.NewMediaContent(media, caption)
}

// handleSticker handles a sticker message
func (b *Bot) handleSticker(msg *tgbotapi.Message) core.Content {
	sticker := msg.Sticker

	media := []core.MediaAttachment{
		{
			Type:   "sticker",
			URL:    fmt.Sprintf("file://%s", sticker.FileID),
			Width:  sticker.Width,
			Height: sticker.Height,
			Raw:    map[string]interface{}{"fileUniqueId": sticker.FileUniqueID},
		},
	}

	return core.NewMediaContent(media, "")
}
