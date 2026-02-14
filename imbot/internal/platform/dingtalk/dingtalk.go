package dingtalk

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/logger"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

const (
	defaultMaxReconnectAttempts = 5
	defaultReconnectDelay       = 5 * time.Second
)

// Bot implements DingTalk bot using official SDK
type Bot struct {
	*core.BaseBot
	cli        *client.StreamClient
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.RWMutex
	webhookMap map[string]string // conversationID -> webhook URL
}

// NewDingTalkBot creates a new DingTalk bot
func NewDingTalkBot(config *core.Config) (*Bot, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if config.Auth.Type != "oauth" {
		return nil, core.NewAuthFailedError(config.Platform, "dingtalk requires oauth auth (AppKey/AppSecret)", nil)
	}

	clientID := config.Auth.ClientID
	clientSecret := config.Auth.ClientSecret

	if clientID == "" || clientSecret == "" {
		return nil, core.NewAuthFailedError(config.Platform, "AppKey and AppSecret are required", nil)
	}

	bot := &Bot{
		BaseBot:    core.NewBaseBot(config),
		webhookMap: make(map[string]string),
	}

	return bot, nil
}

// Connect connects to DingTalk using Stream Mode
func (b *Bot) Connect(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ctx, b.cancel = context.WithCancel(ctx)

	b.Logger().Info("Connecting to DingTalk...")

	// Get credentials from config
	clientID := b.Config().Auth.ClientID
	clientSecret := b.Config().Auth.ClientSecret

	// Create official SDK stream client
	cli := client.NewStreamClient(
		client.WithAppCredential(client.NewAppCredentialConfig(clientID, clientSecret)),
	)

	// Set up logger for SDK
	sdkLogger := &sdkLoggerWrapper{
		logger: b.Logger(),
	}
	logger.SetLogger(sdkLogger)

	b.cli = cli

	// Register chat bot callback handler
	b.cli.RegisterChatBotCallbackRouter(b.onChatBotMessage)

	// Start the client
	if err := b.cli.Start(b.ctx); err != nil {
		return core.NewConnectionFailedError(core.PlatformDingTalk, "stream connection failed", true)
	}

	b.UpdateConnected(true)
	b.UpdateAuthenticated(true)
	b.EmitConnected()
	b.Logger().Info("DingTalk bot connected")

	// Wait for connection to be ready
	b.wg.Add(1)
	go b.waitForReady()

	return nil
}

// Disconnect disconnects from DingTalk
func (b *Bot) Disconnect(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cancel != nil {
		b.cancel()
	}

	if b.cli != nil {
		b.cli.Close()
	}

	b.wg.Wait()

	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()
	b.Logger().Info("DingTalk bot disconnected")

	return nil
}

// waitForReady waits for the bot to be ready
func (b *Bot) waitForReady() {
	defer b.wg.Done()
	time.Sleep(2 * time.Second)
	b.UpdateReady(true)
	b.EmitReady()
}

// onChatBotMessage handles chat bot callback messages
func (b *Bot) onChatBotMessage(ctx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
	message, err := b.parseChatBotMessage(data)
	if err != nil {
		b.Logger().Error("Failed to parse chat bot message: %v", err)
		return []byte(""), nil
	}

	// Emit message to handlers
	b.EmitMessage(*message)

	// Return empty response (no reply)
	return []byte(""), nil
}

// parseChatBotMessage converts SDK callback data to unified Message
func (b *Bot) parseChatBotMessage(data *chatbot.BotCallbackDataModel) (*core.Message, error) {
	sender := core.Sender{
		ID:          data.SenderStaffId,
		DisplayName: data.SenderNick,
		Raw: map[string]interface{}{
			"staffId":        data.SenderStaffId,
			"conversationId": data.ConversationId,
		},
	}

	recipient := core.Recipient{
		ID:   data.ConversationId,
		Type: getChatTypeString(data.ConversationType),
	}

	var content core.Content
	if data.Text.Content != "" {
		content = core.NewTextContent(data.Text.Content)
	} else {
		content = core.NewTextContent("")
	}

	// Store webhook URL for this conversation
	b.mu.Lock()
	if data.SessionWebhook != "" {
		b.webhookMap[data.ConversationId] = data.SessionWebhook
	}
	b.mu.Unlock()

	message := &core.Message{
		ID:        data.MsgId,
		Platform:  core.PlatformDingTalk,
		Timestamp: time.Now().Unix(),
		Sender:    sender,
		Recipient: recipient,
		Content:   content,
		ChatType:  getChatType(data.ConversationType),
		Metadata: map[string]interface{}{
			"conversationId":    data.ConversationId,
			"conversationType":  data.ConversationType,
			"conversationTitle": data.ConversationTitle,
			"sessionWebhook":    data.SessionWebhook,
			"msgtype":           data.Msgtype,
			"senderId":          data.SenderId,
		},
	}

	return message, nil
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

// React reacts to a message (not supported in current implementation)
func (b *Bot) React(ctx context.Context, messageID string, emoji string) error {
	return core.NewBotError(core.ErrPlatformError, "react not yet implemented for DingTalk", false)
}

// EditMessage edits a message (not supported)
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	return core.NewBotError(core.ErrPlatformError, "edit message not supported on DingTalk", false)
}

// DeleteMessage deletes a message (not supported via stream mode webhook)
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	return core.NewBotError(core.ErrPlatformError, "delete/recall message not supported via stream mode webhook", false)
}

// PlatformInfo returns platform information
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(core.PlatformDingTalk, "DingTalk")
}

// StartReceiving starts receiving messages (already started in Connect)
func (b *Bot) StartReceiving(ctx context.Context) error {
	return nil // Already started in Connect
}

// StopReceiving stops receiving messages (already handled in Disconnect)
func (b *Bot) StopReceiving(ctx context.Context) error {
	return nil // Already handled in Disconnect
}

// sendText sends a text message using SDK's ChatbotReplier
func (b *Bot) sendText(ctx context.Context, conversationID string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	// Get webhook URL for this conversation
	b.mu.RLock()
	webhookURL, ok := b.webhookMap[conversationID]
	b.mu.RUnlock()

	if !ok || webhookURL == "" {
		return nil, core.NewBotError(core.ErrPlatformError, "no webhook URL for conversation; must receive a message first", false)
	}

	// Validate and chunk text if needed
	if err := b.ValidateTextLength(opts.Text); err != nil {
		return nil, err
	}

	chunks := b.ChunkText(opts.Text)

	replier := chatbot.NewChatbotReplier()
	var lastResult *core.SendResult

	for _, chunk := range chunks {
		if err := replier.SimpleReplyText(ctx, webhookURL, []byte(chunk)); err != nil {
			return nil, core.WrapError(err, core.PlatformDingTalk, core.ErrPlatformError)
		}

		lastResult = &core.SendResult{
			MessageID: fmt.Sprintf("msg_%d", time.Now().UnixNano()),
			Timestamp: time.Now().Unix(),
		}
	}

	b.UpdateLastActivity()
	return lastResult, nil
}

// sendMedia sends media using SDK's ChatbotReplier
func (b *Bot) sendMedia(ctx context.Context, conversationID string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	// Get webhook URL for this conversation
	b.mu.RLock()
	webhookURL, ok := b.webhookMap[conversationID]
	b.mu.RUnlock()

	if !ok || webhookURL == "" {
		return nil, core.NewBotError(core.ErrPlatformError, "no webhook URL for conversation; must receive a message first", false)
	}

	if len(opts.Media) == 0 {
		return nil, core.NewBotError(core.ErrUnknown, "no media to send", false)
	}

	media := opts.Media[0]

	// Note: Stream mode webhook doesn't support direct media upload
	// Media needs to be uploaded first via REST API to get media ID
	// then sent using the appropriate message type
	switch media.Type {
	case "image", "video", "audio", "document":
		return nil, core.NewBotError(core.ErrMediaNotSupported, fmt.Sprintf("media type %s requires upload via REST API first", media.Type), false)
	default:
		return nil, core.NewBotError(core.ErrMediaNotSupported, fmt.Sprintf("unsupported media type: %s", media.Type), false)
	}
}

// getChatType converts DingTalk conversation type to core.ChatType
func getChatType(conversationType string) core.ChatType {
	switch conversationType {
	case "1":
		return core.ChatTypeDirect
	case "2":
		return core.ChatTypeGroup
	default:
		return core.ChatTypeDirect
	}
}

// getChatTypeString returns string representation of chat type
func getChatTypeString(conversationType string) string {
	switch conversationType {
	case "1":
		return "direct"
	case "2":
		return "group"
	default:
		return "direct"
	}
}

// sdkLoggerWrapper wraps core.Logger for SDK compatibility
type sdkLoggerWrapper struct {
	logger core.Logger
}

func (l *sdkLoggerWrapper) Debugf(format string, args ...interface{}) {
	l.logger.Debug(format, args...)
}

func (l *sdkLoggerWrapper) Infof(format string, args ...interface{}) {
	l.logger.Info(format, args...)
}

func (l *sdkLoggerWrapper) Warningf(format string, args ...interface{}) {
	l.logger.Warn(format, args...)
}

func (l *sdkLoggerWrapper) Errorf(format string, args ...interface{}) {
	l.logger.Error(format, args...)
}

func (l *sdkLoggerWrapper) Fatalf(format string, args ...interface{}) {
	l.logger.Error(format, args...)
}
