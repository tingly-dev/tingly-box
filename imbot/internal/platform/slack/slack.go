package slack

import (
	"context"
	"fmt"
	"sync"

	"github.com/slack-go/slack"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Bot implements the Slack bot
type Bot struct {
	*core.BaseBot
	client   *slack.Client
	rtm      *slack.RTM
	appToken string
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.RWMutex
}

// NewSlackBot creates a new Slack bot
func NewSlackBot(config *core.Config) (*Bot, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if config.Auth.Type != "token" {
		return nil, core.NewAuthFailedError(config.Platform, "slack requires token auth", nil)
	}

	token, err := config.Auth.GetToken()
	if err != nil {
		return nil, core.NewAuthFailedError(config.Platform, "failed to get token", err)
	}

	// Create Slack client
	client := slack.New(token, slack.OptionDebug(false))

	bot := &Bot{
		BaseBot:  core.NewBaseBot(config),
		client:   client,
		appToken: config.GetOptionString("appToken", ""),
	}

	return bot, nil
}

// Connect connects to Slack
func (b *Bot) Connect(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Test authentication
	authResp, err := b.client.AuthTest()
	if err != nil {
		return core.NewAuthFailedError(core.PlatformSlack, "authentication failed", err)
	}

	b.UpdateConnected(true)
	b.UpdateAuthenticated(true)
	b.EmitConnected()
	b.Logger().Info("Slack bot connected: %s", authResp.User)

	// Start RTM or WebSocket
	if err := b.startRTM(); err != nil {
		return err
	}

	return nil
}

// Disconnect disconnects from Slack
func (b *Bot) Disconnect(ctx context.Context) error {
	if b.cancel != nil {
		b.cancel()
	}

	if b.rtm != nil {
		b.rtm.Disconnect()
	}

	b.wg.Wait()

	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()
	b.Logger().Info("Slack bot disconnected")

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

	// Parse channel ID and timestamp from messageID
	// Slack uses "channelID:timestamp" format
	parts := parseSlackMessageReference(messageID)
	if len(parts) != 2 {
		return core.NewInvalidTargetError(core.PlatformSlack, messageID, "invalid format, expected channelID:timestamp")
	}

	// Add reaction - slack-go API: AddReaction(name string, item ItemRef)
	itemRef := slack.NewRefToMessage(parts[0], parts[1])
	err := b.client.AddReaction(emoji, itemRef)
	if err != nil {
		return core.WrapError(err, core.PlatformSlack, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return nil
}

// EditMessage edits a message
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	// Parse channel ID and timestamp
	parts := parseSlackMessageReference(messageID)
	if len(parts) != 2 {
		return core.NewInvalidTargetError(core.PlatformSlack, messageID, "invalid format, expected channelID:timestamp")
	}

	// Parse thread ID if present (for threaded messages)
	var threadTimestamp string
	if threadIDIdx := findIndex(messageID, ":thread:"); threadIDIdx != -1 {
		threadTimestamp = messageID[threadIDIdx+8:]
	}

	options := []slack.MsgOption{}
	if threadTimestamp != "" {
		options = append(options, slack.MsgOptionTS(threadTimestamp))
	}

	_, _, err := b.client.PostMessage(parts[0], slack.MsgOptionText(text, false), options[0])
	if err != nil {
		return core.WrapError(err, core.PlatformSlack, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return nil
}

// DeleteMessage deletes a message
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	// Parse channel ID and timestamp
	parts := parseSlackMessageReference(messageID)
	if len(parts) != 2 {
		return core.NewInvalidTargetError(core.PlatformSlack, messageID, "invalid format, expected channelID:timestamp")
	}

	_, _, err := b.client.DeleteMessage(parts[0], parts[1])
	if err != nil {
		return core.WrapError(err, core.PlatformSlack, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return nil
}

// PlatformInfo returns platform information
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(core.PlatformSlack, "Slack")
}

// StartReceiving starts receiving messages (already started in Connect)
func (b *Bot) StartReceiving(ctx context.Context) error {
	return nil
}

// StopReceiving stops receiving messages (already handled in Disconnect)
func (b *Bot) StopReceiving(ctx context.Context) error {
	return nil
}

// startRTM starts the Real-Time Messaging connection
func (b *Bot) startRTM() error {
	rtm := b.client.NewRTM()

	go b.handleRTMEvents(rtm)

	b.rtm = rtm
	// RTM connects automatically when we call ManageConnection or after starting the goroutine
	// Return nil immediately, the connection will be established in the background
	return nil
}

// handleRTMEvents handles RTM events
func (b *Bot) handleRTMEvents(rtm *slack.RTM) {
	b.wg.Add(1)
	defer b.wg.Done()

	b.UpdateReady(true)
	b.EmitReady()

	for {
		select {
		case <-b.ctx.Done():
			return
		case msg, ok := <-rtm.IncomingEvents:
			if !ok {
				return
			}

			switch data := msg.Data.(type) {
			case *slack.HelloEvent:
				b.Logger().Debug("Slack RTM connected")
			case *slack.MessageEvent:
				b.handleMessage(data)
			case *slack.RTMError:
				b.EmitError(fmt.Errorf("RTM error: %v", data))
			}
		}
	}
}

// handleMessage handles incoming message events
func (b *Bot) handleMessage(event *slack.MessageEvent) {
	// Ignore messages from bots
	if event.BotID != "" || event.SubType == "bot_message" {
		return
	}

	// Determine chat type
	chatType := b.getChatType(event)

	// Get sender info
	sender := core.Sender{
		ID:  event.User,
		Raw: make(map[string]interface{}),
	}

	// Try to get user info
	user, err := b.client.GetUserInfo(event.User)
	if err == nil {
		sender.Username = user.Name
		sender.DisplayName = user.Profile.DisplayName
		if sender.DisplayName == "" {
			sender.DisplayName = user.Profile.RealName
		}
	}

	// Create recipient
	recipient := core.Recipient{
		ID:   event.Channel,
		Type: string(chatType),
	}

	// Get channel info
	channel, err := b.client.GetConversationInfo(&slack.GetConversationInfoInput{
		ChannelID: event.Channel,
	})
	if err == nil && channel.Name != "" {
		recipient.DisplayName = channel.Name
	}

	// Extract content
	var content core.Content

	if event.Text != "" {
		content = core.NewTextContent(event.Text)
	} else if event.Files != nil && len(event.Files) > 0 {
		content = b.handleFiles(event.Files)
	} else if event.Attachments != nil && len(event.Attachments) > 0 {
		content = b.handleAttachments(event.Attachments)
	} else {
		content = core.NewSystemContent("unknown", nil)
	}

	// Create message
	message := core.Message{
		ID:        event.Timestamp,
		Platform:  core.PlatformSlack,
		Timestamp: parseSlackTimestamp(event.Timestamp),
		Sender:    sender,
		Recipient: recipient,
		Content:   content,
		ChatType:  chatType,
		Metadata:  make(map[string]interface{}),
	}

	// Add thread context if threaded message
	if event.ThreadTimestamp != "" {
		message.ThreadContext = &core.ThreadContext{
			ID:              event.ThreadTimestamp,
			ParentMessageID: event.ThreadTimestamp,
		}
	}

	b.EmitMessage(message)
}

// getChatType determines the chat type
func (b *Bot) getChatType(event *slack.MessageEvent) core.ChatType {
	// Try to get conversation info to determine type
	channel, err := b.client.GetConversationInfo(&slack.GetConversationInfoInput{
		ChannelID: event.Channel,
	})
	if err == nil {
		if channel.IsIM {
			return core.ChatTypeDirect
		}
		if channel.IsMpIM {
			return core.ChatTypeDirect
		}
		if channel.IsChannel || channel.IsGroup {
			return core.ChatTypeGroup
		}
	}

	// Fallback to checking channel ID format
	// DMs typically start with 'D', channels with 'C', groups with 'G'
	if len(event.Channel) > 0 {
		switch event.Channel[0] {
		case 'D':
			return core.ChatTypeDirect
		case 'G':
			// Could be MPDM or private channel
			return core.ChatTypeGroup
		}
	}

	return core.ChatTypeGroup
}

// sendText sends a text message
func (b *Bot) sendText(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	// Validate text length
	if err := b.ValidateTextLength(opts.Text); err != nil {
		return nil, err
	}

	// Build message options
	msgOpts := []slack.MsgOption{slack.MsgOptionText(opts.Text, false)}

	// Add parse mode (Slack supports markdown)
	if opts.ParseMode == core.ParseModeMarkdown {
		// Slack uses markdown by default
	} else if opts.ParseMode == core.ParseModeNone {
		// Disable markdown (send as plain text)
		msgOpts = []slack.MsgOption{slack.MsgOptionText(opts.Text, true)}
	}

	// Add reply
	threadTS := ""
	if opts.ReplyTo != "" {
		parts := parseSlackMessageReference(opts.ReplyTo)
		if len(parts) == 2 {
			threadTS = parts[1]
		}
	}

	if threadTS != "" {
		msgOpts = append(msgOpts, slack.MsgOptionTS(threadTS))
	}

	// Add thread ID if specified
	if opts.ThreadID != "" {
		msgOpts = append(msgOpts, slack.MsgOptionTS(opts.ThreadID))
	}

	// Send message
	channel, timestamp, err := b.client.PostMessage(target, msgOpts...)
	if err != nil {
		return nil, core.WrapError(err, core.PlatformSlack, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return &core.SendResult{
		MessageID: channel + ":" + timestamp,
		Timestamp: parseSlackTimestamp(timestamp),
	}, nil
}

// sendMedia sends media
func (b *Bot) sendMedia(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if len(opts.Media) == 0 {
		return nil, core.NewBotError(core.ErrUnknown, "no media to send", false)
	}

	// For now, just handle first media item
	media := opts.Media[0]

	// In a real implementation, you would:
	// 1. Download the media from URL
	// 2. Upload to Slack
	// 3. Get the file URL and send as attachment

	// For now, send as a message with URL
	var text string
	if opts.Text != "" {
		text = opts.Text + "\n" + media.URL
	} else {
		text = media.URL
	}

	channel, timestamp, err := b.client.PostMessage(target, slack.MsgOptionText(text, false))
	if err != nil {
		return nil, core.WrapError(err, core.PlatformSlack, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return &core.SendResult{
		MessageID: channel + ":" + timestamp,
		Timestamp: parseSlackTimestamp(timestamp),
	}, nil
}

// handleFiles handles Slack files
func (b *Bot) handleFiles(files []slack.File) core.Content {
	media := make([]core.MediaAttachment, len(files))

	for i, file := range files {
		mediaType := "document"
		switch file.Mimetype {
		case "image/png", "image/jpeg", "image/gif":
			mediaType = "image"
		case "video/mp4":
			mediaType = "video"
		case "audio/mpeg":
			mediaType = "audio"
		}

		media[i] = core.MediaAttachment{
			Type:     mediaType,
			URL:      file.URLPrivate,
			Filename: file.Name,
			Size:     int64(file.Size),
			Title:    file.Title,
			Raw:      make(map[string]interface{}),
		}
	}

	caption := ""
	if len(files) == 1 && files[0].Title != "" {
		caption = files[0].Title
	}

	return core.NewMediaContent(media, caption)
}

// handleAttachments handles Slack attachments
func (b *Bot) handleAttachments(attachments []slack.Attachment) core.Content {
	// Convert attachments to text
	var text string

	for _, att := range attachments {
		if att.Text != "" {
			text += att.Text + "\n"
		}
		if att.Title != "" {
			text += "*" + att.Title + "*\n"
		}
	}

	if text == "" {
		return core.NewSystemContent("attachment", nil)
	}

	return core.NewTextContent(text)
}

// Helper functions

func parseSlackMessageReference(ref string) []string {
	// Slack uses "channelID:timestamp" format
	// Also handle thread format: "channelID:thread:threadTimestamp"
	if idx := findIndex(ref, ":thread:"); idx != -1 {
		// Thread reference
		return []string{ref[:idx], ref[idx+1:]}
	}

	return splitStringN(ref, ":", 2)
}

func parseSlackTimestamp(ts string) int64 {
	// Slack timestamps are in seconds (with decimal for microseconds)
	if ts == "" {
		return 0
	}

	// Parse as float64 and convert to int64 (seconds)
	var f float64
	if _, err := fmt.Sscanf(ts, "%f", &f); err == nil {
		return int64(f)
	}

	return 0
}

func findIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func splitStringN(s, sep string, n int) []string {
	parts := make([]string, 0, n)
	current := ""
	count := 0

	for i := 0; i < len(s); i++ {
		if count == n-1 {
			parts = append(parts, s[i:])
			break
		}

		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			parts = append(parts, current)
			current = ""
			i += len(sep) - 1
			count++
		} else {
			current += string(s[i])
		}
	}

	if current != "" && count < n {
		parts = append(parts, current)
	}

	// Pad with empty strings if needed
	for len(parts) < n {
		parts = append(parts, "")
	}

	return parts
}
