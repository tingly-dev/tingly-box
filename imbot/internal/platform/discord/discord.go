package discord

import (
	"context"
	"fmt"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Bot implements the Discord bot
type Bot struct {
	*core.BaseBot
	adapter *Adapter // Local adapter for message conversion
	session *discordgo.Session
	intents discordgo.Intent
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.RWMutex
}

// NewDiscordBot creates a new Discord bot
func NewDiscordBot(config *core.Config) (*Bot, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if config.Auth.Type != "token" {
		return nil, core.NewAuthFailedError(config.Platform, "discord requires token auth", nil)
	}

	token, err := config.Auth.GetToken()
	if err != nil {
		return nil, core.NewAuthFailedError(config.Platform, "failed to get token", err)
	}

	// Ensure token has Bot prefix
	if !hasBotPrefix(token) {
		token = "Bot " + token
	}

	// Create Discord session with intents
	session, err := discordgo.New(token)
	if err != nil {
		return nil, core.NewAuthFailedError(core.PlatformDiscord, "failed to create discord session", err)
	}

	bot := &Bot{
		BaseBot: core.NewBaseBot(config),
		session: session,
		// Default intents
		intents: discordgo.IntentsGuilds | discordgo.IntentsDirectMessages | discordgo.IntentsGuildMessages | discordgo.IntentMessageContent,
	}

	// Configure intents from options
	if intents, ok := config.Options["intents"].([]interface{}); ok {
		bot.intents = 0
		for _, intent := range intents {
			if intentStr, ok := intent.(string); ok {
				bot.intents |= parseIntent(intentStr)
			}
		}
	}

	// Note: In newer versions of discordgo, intents are set via New(token) with Intent options
	// or through session GatewayManager.Identify. For now, we store intents and use them when needed.

	return bot, nil
}

// Connect connects to Discord
func (b *Bot) Connect(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Initialize adapter
	b.adapter = NewAdapter(b.Config(), b.session)

	// Register handlers
	b.session.AddHandler(b.onMessageCreate)
	b.session.AddHandler(b.onReady)

	// Open connection
	if err := b.session.Open(); err != nil {
		return core.NewConnectionFailedError(core.PlatformDiscord, "failed to open discord connection", true)
	}

	b.UpdateConnected(true)
	b.EmitConnected()
	b.Logger().Info("Discord bot connected: %s", b.session.State.User.Username)

	return nil
}

// Disconnect disconnects from Discord
func (b *Bot) Disconnect(ctx context.Context) error {
	if b.cancel != nil {
		b.cancel()
	}

	// Close session
	if err := b.session.Close(); err != nil {
		b.Logger().Error("Error closing discord session: %v", err)
	}

	b.wg.Wait()

	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()
	b.Logger().Info("Discord bot disconnected")

	return nil
}

// SendMessage sends a message
func (b *Bot) SendMessage(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	// Handle text message
	if opts.Text != "" {
		return b.sendText(ctx, target, opts)
	}

	// Handle media
	if len(opts.Media) > 0 {
		return b.sendMedia(ctx, target, opts)
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

	// Parse channel ID and message ID
	// Discord uses "channelID:messageID" format
	parts := parseDiscordMessageReference(messageID)
	if len(parts) != 2 {
		return core.NewInvalidTargetError(core.PlatformDiscord, messageID, "invalid format, expected channelID:messageID")
	}

	err := b.session.MessageReactionAdd(parts[0], parts[1], emoji)
	if err != nil {
		return core.WrapError(err, core.PlatformDiscord, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return nil
}

// EditMessage edits a message
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	// Parse channel ID and message ID
	parts := parseDiscordMessageReference(messageID)
	if len(parts) != 2 {
		return core.NewInvalidTargetError(core.PlatformDiscord, messageID, "invalid format, expected channelID:messageID")
	}

	_, err := b.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:      parts[1],
		Channel: parts[0],
		Content: &text,
	})
	if err != nil {
		return core.WrapError(err, core.PlatformDiscord, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return nil
}

// DeleteMessage deletes a message
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	// Parse channel ID and message ID
	parts := parseDiscordMessageReference(messageID)
	if len(parts) != 2 {
		return core.NewInvalidTargetError(core.PlatformDiscord, messageID, "invalid format, expected channelID:messageID")
	}

	err := b.session.ChannelMessageDelete(parts[0], parts[1])
	if err != nil {
		return core.WrapError(err, core.PlatformDiscord, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return nil
}

// PlatformInfo returns platform information
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(core.PlatformDiscord, "Discord")
}

// StartReceiving starts receiving messages (already started in Connect)
func (b *Bot) StartReceiving(ctx context.Context) error {
	return nil
}

// StopReceiving stops receiving messages (already handled in Disconnect)
func (b *Bot) StopReceiving(ctx context.Context) error {
	return nil
}

// onReady is called when the bot is ready
func (b *Bot) onReady(s *discordgo.Session, event *discordgo.Ready) {
	b.UpdateReady(true)
	b.EmitReady()
	b.Logger().Info("Discord bot ready")
}

// onMessageCreate handles incoming messages
func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from bots
	if m.Message.Author.Bot {
		return
	}

	// Use adapter to convert platform message to core message
	coreMessage, err := b.adapter.AdaptMessage(b.ctx, m)
	if err != nil {
		b.Logger().Error("Failed to adapt message: %v", err)
		return
	}

	b.EmitMessage(*coreMessage)
}

// sendText sends a text message
func (b *Bot) sendText(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	// Validate text length
	if err := b.ValidateTextLength(opts.Text); err != nil {
		return nil, err
	}

	// Prepare send data
	sendData := &discordgo.MessageSend{
		Content: opts.Text,
	}

	// Set parse mode (Discord doesn't use parse mode like Telegram, markdown is default)
	if opts.ParseMode == core.ParseModeNone {
		// Disable markdown
		// Discord has no way to disable markdown entirely, so we send as is
	}

	// Add reply
	if opts.ReplyTo != "" {
		parts := parseDiscordMessageReference(opts.ReplyTo)
		if len(parts) == 2 {
			sendData.Reference = &discordgo.MessageReference{
				MessageID: parts[1],
			}
		}
	}

	// Send message - ChannelMessageSendComplex for MessageSend struct
	m, err := b.session.ChannelMessageSendComplex(target, sendData)
	if err != nil {
		return nil, core.WrapError(err, core.PlatformDiscord, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return &core.SendResult{
		MessageID: m.ID,
		Timestamp: m.Timestamp.Unix(),
	}, nil
}

// sendMedia sends media
func (b *Bot) sendMedia(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if len(opts.Media) == 0 {
		return nil, core.NewBotError(core.ErrUnknown, "no media to send", false)
	}

	media := opts.Media[0]
	var files []*discordgo.File

	switch media.Type {
	case "image":
		files = append(files, &discordgo.File{
			Name:   "image.png",
			Reader: nil, // In real implementation, you'd download the file
		})
	default:
		return nil, core.NewMediaNotSupportedError(core.PlatformDiscord, media.Type)
	}

	sendData := &discordgo.MessageSend{
		Files: files,
	}

	if opts.Text != "" {
		sendData.Content = opts.Text
	}

	m, err := b.session.ChannelMessageSendComplex(target, sendData)
	if err != nil {
		return nil, core.WrapError(err, core.PlatformDiscord, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return &core.SendResult{
		MessageID: m.ID,
		Timestamp: m.Timestamp.Unix(),
	}, nil
}

// Helper functions

func hasBotPrefix(token string) bool {
	return len(token) > 4 && (token[:4] == "Bot " || token[:4] == "bot ")
}

func parseDiscordMessageReference(ref string) []string {
	// Discord uses "channelID:messageID" format
	// Split by the first colon
	for i := 0; i < len(ref); i++ {
		if ref[i] == ':' {
			return []string{ref[:i], ref[i+1:]}
		}
	}
	// If no colon found, return the ref as channel ID with empty message ID
	return []string{ref, ""}
}

func parseIntent(intent string) discordgo.Intent {
	switch intent {
	case "Guilds":
		return discordgo.IntentsGuilds
	case "GuildMessages":
		return discordgo.IntentsGuildMessages
	case "DirectMessages":
		return discordgo.IntentsDirectMessages
	case "MessageContent":
		return discordgo.IntentMessageContent
	default:
		return 0
	}
}
