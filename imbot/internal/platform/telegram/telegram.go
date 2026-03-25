package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/markdown"
	"golang.org/x/net/proxy"
)

// Bot implements the Telegram bot
type Bot struct {
	*core.BaseBot
	api          *tgbot.Bot
	adapter      *Adapter // Local adapter for message conversion
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.RWMutex
	messageIDMap map[string]int // chatID -> last message ID
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

	proxyURL := config.GetOptionString("proxy", "")
	debug := config.GetOptionBool("debug", false)

	opts := []tgbot.Option{}

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
			client := &http.Client{
				Transport: &http.Transport{
					DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						return dialer.Dial(network, addr)
					},
				},
			}
			opts = append(opts, tgbot.WithHTTPClient(time.Second*30, client))
		case "http", "https":
			client := &http.Client{
				Transport: &http.Transport{
					Proxy: http.ProxyURL(parsed),
				},
			}
			opts = append(opts, tgbot.WithHTTPClient(time.Second*30, client))
		default:
			return nil, core.NewAuthFailedError(core.PlatformTelegram, "unsupported proxy scheme", fmt.Errorf("%s", parsed.Scheme))
		}
	}

	if debug {
		opts = append(opts, tgbot.WithDebug())
	}

	api, err := tgbot.New(token, opts...)
	if err != nil {
		return nil, core.NewAuthFailedError(core.PlatformTelegram, "failed to create telegram bot", err)
	}

	bot := &Bot{
		BaseBot:      core.NewBaseBot(config),
		api:          api,
		messageIDMap: make(map[string]int),
	}

	return bot, nil
}

// Connect connects to Telegram
func (b *Bot) Connect(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Initialize adapter for message conversion
	b.adapter = NewAdapter(b.Config(), b.api)

	// Register handlers - use MatchTypePrefix with empty pattern to match all messages
	b.api.RegisterHandler(tgbot.HandlerTypeMessageText, "", tgbot.MatchTypePrefix, b.handleMessageUpdate)
	b.api.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "", tgbot.MatchTypePrefix, b.handleCallbackQueryUpdate)

	b.UpdateConnected(true)
	b.UpdateAuthenticated(true)
	b.EmitConnected()

	// Get bot info
	me, err := b.api.GetMe(b.ctx)
	if err != nil {
		b.Logger().Error("Failed to get bot info: %v", err)
	} else {
		b.Logger().Info("Telegram bot connected: @%s", me.Username)
	}

	b.UpdateReady(true)
	b.EmitReady()

	// Start receiving messages
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.api.Start(b.ctx)
	}()

	return nil
}

// handleMessageUpdate handles incoming message updates
func (b *Bot) handleMessageUpdate(ctx context.Context, api *tgbot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	// Store chat ID for reaction
	b.mu.Lock()
	b.messageIDMap[strconv.FormatInt(update.Message.Chat.ID, 10)] = update.Message.ID
	b.mu.Unlock()

	coreMessage, err := b.adapter.AdaptMessage(b.ctx, update.Message)
	if err != nil {
		b.Logger().Error("Failed to adapt message: %v", err)
		return
	}

	b.EmitMessage(*coreMessage)
}

// handleCallbackQueryUpdate handles incoming callback query updates
func (b *Bot) handleCallbackQueryUpdate(ctx context.Context, api *tgbot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	query := update.CallbackQuery
	b.Logger().Debug("Received callback query from %d: %s", query.From.ID, query.Data)

	coreMessage, err := b.adapter.AdaptCallback(b.ctx, query)
	if err != nil {
		b.Logger().Error("Failed to adapt callback: %v", err)
		return
	}

	b.EmitMessage(*coreMessage)

	// Answer the callback query to remove loading state
	_, _ = b.api.AnswerCallbackQuery(b.ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
	})
}

// Disconnect disconnects from Telegram
func (b *Bot) Disconnect(ctx context.Context) error {
	if b.cancel != nil {
		b.cancel()
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

	// Get chat ID from context
	b.mu.RLock()
	var chatID int64
	for k, v := range b.messageIDMap {
		id, _ := strconv.ParseInt(k, 10, 64)
		if v != 0 {
			chatID = id
			break
		}
	}
	b.mu.RUnlock()

	if chatID == 0 {
		return core.NewBotError(core.ErrInvalidTarget, "chat ID not found", false)
	}

	msgID, err := strconv.Atoi(messageID)
	if err != nil {
		return core.NewBotError(core.ErrInvalidTarget, "invalid message ID", false)
	}

	_, err = b.api.SetMessageReaction(b.ctx, &tgbot.SetMessageReactionParams{
		ChatID:    chatID,
		MessageID: msgID,
		Reaction: []models.ReactionType{
			{
				Type: models.ReactionTypeTypeEmoji,
				ReactionTypeEmoji: &models.ReactionTypeEmoji{
					Type:  models.ReactionTypeTypeEmoji,
					Emoji: emoji,
				},
			},
		},
	})
	return err
}

// EditMessage edits a message
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	b.Logger().Debug("Edit message: %s", messageID)
	return nil
}

// EditMessageWithKeyboard edits a message with text and inline keyboard
func (b *Bot) EditMessageWithKeyboard(ctx interface{}, chatID string, messageID string, text string, keyboard interface{}) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	// Parse chat ID
	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return core.NewInvalidTargetError(core.PlatformTelegram, chatID, "invalid chat ID")
	}

	// Parse message ID
	msgIDInt, err := strconv.Atoi(messageID)
	if err != nil {
		return core.NewInvalidTargetError(core.PlatformTelegram, messageID, "invalid message ID")
	}

	params := &tgbot.EditMessageTextParams{
		ChatID:    chatIDInt,
		MessageID: msgIDInt,
		Text:      escapeMarkdownV2(text),   // Apply MarkdownV2 escaping
		ParseMode: models.ParseModeMarkdown, // Use this for MarkdownV2
	}

	// Set keyboard if provided
	if keyboard != nil {
		if kb, ok := keyboard.(models.InlineKeyboardMarkup); ok {
			params.ReplyMarkup = &kb
		}
	}

	_, err = b.api.EditMessageText(b.ctx, params)
	if err != nil {
		return core.WrapError(err, core.PlatformTelegram, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return nil
}

// RemoveMessageKeyboard removes the inline keyboard from a message
func (b *Bot) RemoveMessageKeyboard(ctx interface{}, chatID string, messageID string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	// Parse chat ID
	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return core.NewInvalidTargetError(core.PlatformTelegram, chatID, "invalid chat ID")
	}

	// Parse message ID
	msgIDInt, err := strconv.Atoi(messageID)
	if err != nil {
		return core.NewInvalidTargetError(core.PlatformTelegram, messageID, "invalid message ID")
	}

	// Create edit config with empty inline keyboard to remove existing keyboard
	params := &tgbot.EditMessageReplyMarkupParams{
		ChatID:    chatIDInt,
		MessageID: msgIDInt,
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{},
		},
	}

	_, err = b.api.EditMessageReplyMarkup(b.ctx, params)
	if err != nil {
		return core.WrapError(err, core.PlatformTelegram, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return nil
}

// DeleteMessage deletes a message
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	// Need chat ID and message ID - for now this is a placeholder
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

// sendText sends a text message
func (b *Bot) sendText(ctx context.Context, chatID int64, opts *core.SendMessageOptions) (*core.SendResult, error) {
	var parseMode models.ParseMode
	var text string = opts.Text
	var entities []models.MessageEntity

	// Priority: Entities > ParseMode
	// If entities are provided, use them directly (no parse_mode)
	if len(opts.Entities) > 0 {
		// Convert core.Entity to Telegram MessageEntity
		entities = convertEntitiesToTelegram(opts.Entities)
	} else if opts.ParseMode != "" {
		// Fall back to parse_mode
		switch opts.ParseMode {
		case core.ParseModeMarkdown:
			// 内部迁移：使用新的 markdown entity 模块
			// 如果转换失败，降级到 MarkdownV2 escape
			if result, err := markdown.Convert(text); err == nil {
				text = result.Text
				entities = convertEntitiesToTelegram(markdown.ToIMBotEntities(result.Entities))
			} else {
				// 转换失败，降级到 MarkdownV2 escape
				parseMode = models.ParseModeMarkdown
				text = escapeMarkdownV2(text)
			}
		case core.ParseModeMarkdownLegacy:
			// Legacy: Use MarkdownV1 (backward compatibility)
			parseMode = models.ParseModeMarkdownV1
		case core.ParseModeHTML:
			parseMode = models.ParseModeHTML
		}
	}

	// Chunk text first (handles long messages)
	chunks := b.ChunkText(text)

	// Validate each chunk length
	for _, chunk := range chunks {
		if err := b.ValidateTextLength(chunk); err != nil {
			return nil, err
		}
	}

	var lastResult *core.SendResult
	for i, chunk := range chunks {
		params := &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   chunk,
		}

		// Set entities (if provided)
		if len(entities) > 0 {
			params.Entities = entities
		} else if parseMode != "" {
			// Otherwise use parse_mode
			params.ParseMode = parseMode
		}

		// Set reply to only on first chunk
		if opts.ReplyTo != "" && i == 0 {
			if replyToID, err := strconv.Atoi(opts.ReplyTo); err == nil {
				params.ReplyParameters = &models.ReplyParameters{
					MessageID: replyToID,
				}
			}
		}

		// Disable notification if silent
		if opts.Silent {
			params.DisableNotification = true
		}

		// Set reply markup (inline keyboard) only on last chunk
		if opts.Metadata != nil && i == len(chunks)-1 {
			if markup, ok := opts.Metadata["replyMarkup"]; ok {
				if replyMarkup, ok := markup.(models.InlineKeyboardMarkup); ok {
					params.ReplyMarkup = &replyMarkup
				}
			}
		}

		sentMsg, err := b.api.SendMessage(b.ctx, params)
		if err != nil {
			return nil, core.WrapError(err, core.PlatformTelegram, core.ErrPlatformError)
		}

		lastResult = &core.SendResult{
			MessageID: strconv.Itoa(sentMsg.ID),
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

	var sentMsg *models.Message
	var err error

	if media.Type == "image" || media.Type == "sticker" {
		// Send as photo
		params := &tgbot.SendPhotoParams{
			ChatID: chatID,
			Photo:  &models.InputFileString{Data: media.URL},
		}
		if opts.Text != "" {
			params.Caption = opts.Text
		}
		sentMsg, err = b.api.SendPhoto(b.ctx, params)
	} else {
		// Send as document
		params := &tgbot.SendDocumentParams{
			ChatID:   chatID,
			Document: &models.InputFileString{Data: media.URL},
		}
		if opts.Text != "" {
			params.Caption = opts.Text
		}
		sentMsg, err = b.api.SendDocument(b.ctx, params)
	}

	if err != nil {
		return nil, core.WrapError(err, core.PlatformTelegram, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return &core.SendResult{
		MessageID: strconv.Itoa(sentMsg.ID),
		Timestamp: int64(sentMsg.Date),
	}, nil
}

// ResolveChatID resolves a chat ID from invite link, username, or direct ID
// Returns the chat ID if successful
func (b *Bot) ResolveChatID(input string) (string, error) {
	if err := b.EnsureReady(); err != nil {
		return "", err
	}

	input = strings.TrimSpace(input)

	// Try to parse as direct chat ID (numeric)
	if chatID, err := strconv.ParseInt(input, 10, 64); err == nil {
		return strconv.FormatInt(chatID, 10), nil
	}

	// Handle @username format
	if strings.HasPrefix(input, "@") {
		username := input[1:]
		chat, err := b.api.GetChat(b.ctx, &tgbot.GetChatParams{
			ChatID: username,
		})
		if err != nil {
			return "", fmt.Errorf("failed to get chat by username @%s: %w", username, err)
		}
		return strconv.FormatInt(chat.ID, 10), nil
	}

	// Handle invite links: https://t.me/+hash or https://t.me/joinchat/hash
	// Note: Bot must already be a member of the chat to get its info
	if strings.Contains(input, "t.me/") {
		// Extract the chat identifier from the link
		// For public groups: https://t.me/groupname
		// For private groups: https://t.me/+hash or https://t.me/joinchat/hash

		// Try to extract username from public link
		if !strings.Contains(input, "+") && !strings.Contains(input, "joinchat") {
			// Public link format: https://t.me/username
			parts := strings.Split(input, "t.me/")
			if len(parts) == 2 {
				username := strings.TrimSuffix(parts[1], "/")
				chat, err := b.api.GetChat(b.ctx, &tgbot.GetChatParams{
					ChatID: username,
				})
				if err != nil {
					return "", fmt.Errorf("failed to get chat from link %s: %w", input, err)
				}
				return strconv.FormatInt(chat.ID, 10), nil
			}
		}

		// For private invite links, we need to use the raw API
		// Try to extract the hash and use joinChat API (if bot has permission)
		inviteHash := ""
		if strings.Contains(input, "+") {
			parts := strings.Split(input, "+")
			if len(parts) == 2 {
				inviteHash = strings.TrimSuffix(parts[1], "/")
			}
		} else if strings.Contains(input, "joinchat/") {
			parts := strings.Split(input, "joinchat/")
			if len(parts) == 2 {
				inviteHash = strings.TrimSuffix(parts[1], "/")
			}
		}

		if inviteHash != "" {
			// Note: joinChat requires raw API, skip for now
			// A production implementation would need to use reflection or a wrapper
			return "", fmt.Errorf("private invite links not supported: %s", input)
		}

		return "", fmt.Errorf("could not parse invite link: %s", input)
	}

	return "", fmt.Errorf("invalid input format: %s (expected chat ID, @username, or invite link)", input)
}

// SetCommandList sets the bot's command list (shown in the menu button)
// Accepts either []models.BotCommand or []map[string]string
func (b *Bot) SetCommandList(commands interface{}) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	var botCommands []models.BotCommand

	switch v := commands.(type) {
	case []models.BotCommand:
		botCommands = v
	case []map[string]string:
		// Convert from map format to BotCommand
		botCommands = make([]models.BotCommand, 0, len(v))
		for _, cmd := range v {
			botCommands = append(botCommands, models.BotCommand{
				Command:     cmd["command"],
				Description: cmd["description"],
			})
		}
	default:
		return fmt.Errorf("invalid commands type: %T", commands)
	}

	_, err := b.api.SetMyCommands(b.ctx, &tgbot.SetMyCommandsParams{
		Commands: botCommands,
	})
	return err
}

// SetMenuButton sets the menu button for the bot
// Config can be:
// - map[string]interface{}{"type": "commands"} - Show commands menu
// - map[string]interface{}{"type": "web_app", "text": "Text", "url": "https://..."} - Open web app
// - map[string]interface{}{"type": "default"} - Reset to default
func (b *Bot) SetMenuButton(config interface{}) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	var menuButton models.InputMenuButton

	switch cfg := config.(type) {
	case map[string]interface{}:
		menuType, _ := cfg["type"].(string)
		switch menuType {
		case "commands":
			menuButton = &models.MenuButtonCommands{Type: models.MenuButtonTypeCommands}
		case "web_app":
			text, _ := cfg["text"].(string)
			url, _ := cfg["url"].(string)
			menuButton = &models.MenuButtonWebApp{
				Type:   models.MenuButtonTypeWebApp,
				Text:   text,
				WebApp: models.WebAppInfo{URL: url},
			}
		default:
			menuButton = &models.MenuButtonDefault{Type: models.MenuButtonTypeDefault}
		}
	default:
		menuButton = &models.MenuButtonCommands{Type: models.MenuButtonTypeCommands}
	}

	_, err := b.api.SetChatMenuButton(b.ctx, &tgbot.SetChatMenuButtonParams{
		MenuButton: menuButton,
	})
	return err
}

// GetMenuButton gets the current menu button configuration
func (b *Bot) GetMenuButton() (map[string]interface{}, error) {
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	resp, err := b.api.GetChatMenuButton(b.ctx, &tgbot.GetChatMenuButtonParams{})
	if err != nil {
		return nil, err
	}

	// Convert to map
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}

	var menuButton map[string]interface{}
	if err := json.Unmarshal(data, &menuButton); err != nil {
		return nil, err
	}

	return menuButton, nil
}

// escapeMarkdownV2 escapes special characters for Telegram MarkdownV2
// MarkdownV2 requires escaping: _ * [ ] ( ) ~ ` > # + - = | { } . !
func escapeMarkdownV2(text string) string {
	// Use the library's built-in escape function
	return tgbot.EscapeMarkdown(text)
}

// convertEntitiesToTelegram converts core.Entity to Telegram MessageEntity
func convertEntitiesToTelegram(entities []core.Entity) []models.MessageEntity {
	result := make([]models.MessageEntity, len(entities))
	for i, ent := range entities {
		msgEntity := models.MessageEntity{
			Type:   models.MessageEntityType(ent.Type),
			Offset: ent.Offset,
			Length: ent.Length,
		}

		// Extract optional fields from Data map
		if ent.Data != nil {
			if url, ok := ent.Data["url"].(string); ok {
				msgEntity.URL = url
			}
			if lang, ok := ent.Data["language"].(string); ok {
				msgEntity.Language = lang
			}
			if emojiID, ok := ent.Data["custom_emoji_id"].(string); ok {
				msgEntity.CustomEmojiID = emojiID
			}
		}

		result[i] = msgEntity
	}
	return result
}
