package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/tingly-dev/tingly-box/imbot/interaction"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// Domain represents the service domain (Feishu or Lark)
type Domain string

const (
	DomainFeishu Domain = "feishu"
	DomainLark   Domain = "lark"
)

// getReceiveIdType determines the receive_id_type for the Feishu/Lark send API
// based on the target ID format.
//
// Feishu/Lark ID prefixes (https://open.feishu.cn/document/server-docs/im-v1/message/create):
//   - ou_xxxx: user open_id   -> "open_id"
//   - on_xxxx: user union_id  -> "union_id"
//   - oc_xxxx: chat id (p2p or group) -> "chat_id"
//   - contains "@": email     -> "email"
//   - otherwise: user_id (no fixed prefix) -> "user_id"
func getReceiveIdType(targetID string) string {
	switch {
	case strings.HasPrefix(targetID, "ou_"):
		return "open_id"
	case strings.HasPrefix(targetID, "on_"):
		return "union_id"
	case strings.HasPrefix(targetID, "oc_"):
		return "chat_id"
	case strings.Contains(targetID, "@"):
		return "email"
	default:
		return "user_id"
	}
}

// Bot is the Lark SDK-based bot implementation
// Supports both Feishu and Lark platforms via domain configuration
// Supports both WebSocket (long connection) and webhook modes
type Bot struct {
	*core.BaseBot
	client      *lark.Client   // HTTP client for sending messages
	wsClient    *larkws.Client // WebSocket client for receiving events
	domain      Domain
	eventCtx    context.Context
	eventCancel context.CancelFunc
}

// NewBot creates a new Feishu/Lark bot using Lark SDK
func NewBot(config *core.Config, domain Domain) (*Bot, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if config.Auth.Type != "oauth" {
		return nil, core.NewAuthFailedError(core.Platform(string(domain)), "requires oauth auth", nil)
	}

	if config.Auth.ClientID == "" || config.Auth.ClientSecret == "" {
		return nil, core.NewAuthFailedError(core.Platform(string(domain)), "app ID and app secret are required", nil)
	}

	// Determine base URL by domain
	baseURL := lark.FeishuBaseUrl
	if domain == DomainLark {
		baseURL = lark.LarkBaseUrl
	}

	// Create Lark SDK HTTP client with domain-specific base URL
	client := lark.NewClient(
		config.Auth.ClientID,
		config.Auth.ClientSecret,
		lark.WithOpenBaseUrl(baseURL),
		lark.WithEnableTokenCache(true),
	)

	return &Bot{
		BaseBot: core.NewBaseBot(config),
		client:  client,
		domain:  domain,
	}, nil
}

// NewFeishuBot creates a Feishu bot (preserved for backward compatibility)
func NewFeishuBot(config *core.Config) (*Bot, error) {
	return NewBot(config, DomainFeishu)
}

// Connect connects to Feishu/Lark using Lark SDK (authentication + start receiving)
func (b *Bot) Connect(ctx context.Context) error {
	// Test authentication via SDK
	_, err := b.client.GetTenantAccessTokenBySelfBuiltApp(ctx, &larkcore.SelfBuiltTenantAccessTokenReq{
		AppID:     b.Config().Auth.ClientID,
		AppSecret: b.Config().Auth.ClientSecret,
	})
	if err != nil {
		return core.NewAuthFailedError(core.Platform(b.domain), "authentication failed", err)
	}

	b.MarkConnected(true)
	b.Logger().Info("%s bot connected (authenticated): domain=%s", b.domain, b.domain)

	// Auto-start receiving messages via WebSocket
	// This makes Connect() fully ready to receive messages, matching the behavior of other platforms
	if err := b.StartReceiving(ctx); err != nil {
		b.Logger().Error("%s failed to start receiving: %v", b.domain, err)
		return core.NewConnectionFailedError(core.Platform(b.domain), "failed to start receiving", false)
	}

	return nil
}

// Disconnect disconnects from Feishu/Lark
func (b *Bot) Disconnect(ctx context.Context) error {
	// Stop receiving first if running
	if b.wsClient != nil {
		_ = b.StopReceiving(ctx)
	}

	b.MarkDisconnected()
	b.Logger().Info("%s bot disconnected", b.domain)
	return nil
}

// StartReceiving starts receiving events via WebSocket long connection
// This is the main method for establishing real-time event listening
func (b *Bot) StartReceiving(ctx context.Context) error {
	// Create event handler that converts Lark events to core.Message
	// OnP2MessageReceiveV1 handles both v1.0 and v2.0 message receive events
	// OnP2CardActionTrigger handles interactive card button clicks (keyboard callbacks)
	// Reaction handlers are registered (as no-ops) so that, when the app subscribes
	// to reaction events, the dispatcher does not log "not found handler" errors.
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(b.handleP2MessageReceiveV1).
		OnP2CardActionTrigger(b.handleCardActionTrigger).
		OnP2MessageReactionCreatedV1(b.handleMessageReactionCreated).
		OnP2MessageReactionDeletedV1(b.handleMessageReactionDeleted)

	// Determine base URL for WebSocket
	wsDomain := lark.FeishuBaseUrl
	if b.domain == DomainLark {
		wsDomain = lark.LarkBaseUrl
	}

	// Create WebSocket client
	wsClient := larkws.NewClient(
		b.Config().Auth.ClientID,
		b.Config().Auth.ClientSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithDomain(wsDomain),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	b.wsClient = wsClient

	// Start WebSocket connection in background.
	// Derive eventCtx from the parent ctx so cancellation of the bot's
	// lifecycle context tears down the WS goroutine even if Disconnect()
	// is bypassed (e.g. panic upstream).
	b.eventCtx, b.eventCancel = context.WithCancel(ctx)

	go func() {
		b.Logger().Info("%s WebSocket connecting...", b.domain)
		if err := wsClient.Start(b.eventCtx); err != nil {
			b.Logger().Error("%s WebSocket error: %v", b.domain, err)
			b.UpdateReady(false)
		}
	}()

	// Wait a moment for connection to establish
	time.Sleep(2 * time.Second)

	b.MarkReady()
	b.Logger().Info("%s WebSocket connected and receiving events", b.domain)

	return nil
}

// StopReceiving stops receiving events via WebSocket
func (b *Bot) StopReceiving(ctx context.Context) error {
	if b.eventCancel != nil {
		b.eventCancel()
		b.eventCancel = nil
	}
	if b.wsClient != nil {
		// Note: larkws.Client doesn't have explicit Close method
		// The context cancellation handles cleanup
		b.wsClient = nil
	}
	b.UpdateReady(false)
	b.Logger().Info("%s WebSocket stopped", b.domain)
	return nil
}

// handleP2MessageReceiveV1 handles P2 message events (v2.0)
func (b *Bot) handleP2MessageReceiveV1(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	b.Logger().Info("%s received P2 message: %+v", b.domain, event)

	// Convert Lark event to core.Message
	coreMsg := b.convertLarkMessageToCore(event)

	// Emit the message
	b.EmitMessage(*coreMsg)

	return nil
}

// handleCardActionTrigger handles interactive card button clicks.
// It converts the callback into a core.Message shaped like the Telegram callback
// flow (is_callback + callback_data metadata) so downstream routers can be shared.
func (b *Bot) handleCardActionTrigger(ctx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
	if event == nil || event.Event == nil || event.Event.Action == nil {
		return nil, nil
	}

	action := event.Event.Action

	// Buttons built by buildInteractiveCard / FeishuCardRenderer store the routing
	// string under the "callback" key of the button value map.
	callbackData, _ := action.Value["callback"].(string)
	if callbackData == "" {
		// Not a routable button (e.g. URL button or form input); nothing to emit.
		return nil, nil
	}

	var openMessageID, openChatID string
	if event.Event.Context != nil {
		openMessageID = event.Event.Context.OpenMessageID
		openChatID = event.Event.Context.OpenChatID
	}

	// Resolve operator identity (the user who clicked the button).
	var senderID, senderUserID string
	if op := event.Event.Operator; op != nil {
		if op.UserID != nil && *op.UserID != "" {
			senderUserID = *op.UserID
		}
		if op.OpenID != "" {
			senderID = op.OpenID
		} else {
			senderID = senderUserID
		}
	}

	// Reply into the chat that hosts the card (chat_id, valid for p2p and group).
	// This matches the reply target chosen by convertLarkMessageToCore, keeping the
	// conversation key stable across messages and button clicks. Fall back to the
	// operator id only if the chat context is somehow absent.
	replyTarget := openChatID
	if replyTarget == "" {
		if senderID != "" {
			replyTarget = senderID
		} else {
			replyTarget = senderUserID
		}
	}

	b.Logger().Info("%s received card action: callback=%s msg=%s chat=%s", b.domain, callbackData, openMessageID, openChatID)

	msg := core.NewMessageBuilder(core.Platform(b.domain)).
		WithID(openMessageID).
		WithTimestamp(time.Now().Unix()).
		WithSender(senderID, "", "").
		WithRecipient(replyTarget, string(core.ChatTypeDirect), "").
		WithContent(core.NewTextContent("callback:" + callbackData)).
		WithMetadata("is_callback", true).
		WithMetadata("callback_data", callbackData).
		WithMetadata("message_id", openMessageID).
		WithMetadata("original_chat_id", openChatID).
		WithMetadata("sender_user_id", senderUserID).
		WithMetadata("feishu_card_token", event.Event.Token).
		Build()

	b.EmitMessage(*msg)

	// Acknowledge the click without mutating the card; remote_control sends a fresh
	// result message rather than editing the original card in place.
	return nil, nil
}

// handleMessageReactionCreated acknowledges reaction-created events. The bot does
// not act on inbound reactions, but registering a handler prevents the SDK
// dispatcher from logging "not found handler" errors for subscribed events.
func (b *Bot) handleMessageReactionCreated(ctx context.Context, event *larkim.P2MessageReactionCreatedV1) error {
	b.Logger().Debug("%s reaction created (ignored)", b.domain)
	return nil
}

// handleMessageReactionDeleted acknowledges reaction-deleted events (no-op).
func (b *Bot) handleMessageReactionDeleted(ctx context.Context, event *larkim.P2MessageReactionDeletedV1) error {
	b.Logger().Debug("%s reaction deleted (ignored)", b.domain)
	return nil
}

// convertLarkMessageToCore converts a Lark P2MessageReceiveV1 event to core.Message
func (b *Bot) convertLarkMessageToCore(event *larkim.P2MessageReceiveV1) *core.Message {
	// Safety check
	if event == nil {
		b.Logger().Error("convertLarkMessageToCore: event is nil")
		// Return a dummy error message
		return core.NewMessageBuilder(core.Platform(b.domain)).
			WithID("error").
			WithTimestamp(time.Now().Unix()).
			WithSender("system", "", "").
			WithContent(core.NewSystemContent("error", map[string]interface{}{"error": "nil event"})).
			Build()
	}

	b.Logger().Debug("Converting Lark message: event=%p, event.Event=%p", event, event.Event)

	// Extract basic information
	var messageID string
	if event.Event != nil && event.Event.Message != nil {
		if event.Event.Message.MessageId != nil {
			messageID = *event.Event.Message.MessageId
		}
		b.Logger().Debug("Message ID: %s", messageID)
	}

	var chatID string
	var replyTarget string // Used for sending replies
	var chatType core.ChatType = core.ChatTypeDirect

	if event.Event != nil && event.Event.Message != nil {
		if event.Event.Message.ChatId != nil {
			chatID = *event.Event.Message.ChatId
		}
		if event.Event.Message.ChatType != nil {
			switch *event.Event.Message.ChatType {
			case "group":
				chatType = core.ChatTypeGroup
			case "channel":
				chatType = core.ChatTypeChannel
			}
		}
		b.Logger().Debug("Chat ID: %s, Type: %s", chatID, chatType)
	}

	var senderID string
	var senderUserID string // Global user_id for cross-app messaging
	if event.Event != nil && event.Event.Sender != nil {
		if event.Event.Sender.SenderId != nil {
			// Prefer user_id (global) over open_id (app-specific)
			if event.Event.Sender.SenderId.UserId != nil {
				senderUserID = *event.Event.Sender.SenderId.UserId
				senderID = senderUserID
			} else if event.Event.Sender.SenderId.OpenId != nil {
				senderID = *event.Event.Sender.SenderId.OpenId
			}
		}
		b.Logger().Debug("Sender ID: %s, Sender UserID: %s", senderID, senderUserID)
	}

	// Reply to the chat that the message belongs to. chat_id (oc_...) is valid for
	// both p2p and group chats and avoids the "open_id cross app" failure that arises
	// when an app-specific open_id is reused as a receive_id. Using chat_id uniformly
	// also keeps the conversation key consistent with card-action callbacks, so
	// per-chat state (bind flow, directory browser) survives button interactions.
	if chatID != "" {
		replyTarget = chatID
	} else if senderUserID != "" {
		replyTarget = senderUserID
	} else {
		replyTarget = senderID
	}

	// Extract message content - Lark Content is JSON string like {"text":"hello"}
	// or a media payload like {"image_key":"..."} / {"file_key":"...","file_name":"..."}.
	var msgType string
	if event.Event != nil && event.Event.Message != nil && event.Event.Message.MessageType != nil {
		msgType = *event.Event.Message.MessageType
	}
	var rawContent string
	var contentMap map[string]interface{}
	if event.Event != nil && event.Event.Message != nil && event.Event.Message.Content != nil {
		rawContent = *event.Event.Message.Content
		b.Logger().Debug("Raw content: %s", rawContent)
		if err := json.Unmarshal([]byte(rawContent), &contentMap); err != nil {
			b.Logger().Warn("Failed to parse content JSON: %v, using raw string", err)
		}
	}

	content := b.buildLarkContent(msgType, contentMap, rawContent, messageID)

	// Build core.Message using the builder
	messageBuilder := core.NewMessageBuilder(core.Platform(b.domain)).
		WithID(messageID).
		WithTimestamp(time.Now().Unix()).
		WithSender(senderID, "", "").
		WithRecipient(replyTarget, string(chatType), "").
		WithContent(content)

	// Add metadata for raw event access and original chat_id
	messageBuilder.WithMetadata("raw_lark_event", event)
	messageBuilder.WithMetadata("original_chat_id", chatID)
	messageBuilder.WithMetadata("chat_type", chatType)
	messageBuilder.WithMetadata("sender_user_id", senderUserID)

	msg := messageBuilder.Build()
	b.Logger().Debug("Built core message: ID=%s, Sender=%s, Type=%s", msg.ID, msg.Sender.ID, msgType)

	return msg
}

// larkResourceURLScheme prefixes media URLs that must be fetched via the Feishu
// resource API rather than plain HTTP. The remote bot's FileStore recognizes it.
const larkResourceURLScheme = "feishu://"

// buildLarkContent converts a parsed Lark message payload into core.Content,
// producing media content (with a feishu:// resource URL) for image/file/audio/
// media messages and text content otherwise.
func (b *Bot) buildLarkContent(msgType string, content map[string]interface{}, rawContent, messageID string) core.Content {
	str := func(k string) string {
		if v, ok := content[k].(string); ok {
			return v
		}
		return ""
	}

	switch msgType {
	case "image":
		if key := str("image_key"); key != "" {
			return core.NewMediaContent([]core.MediaAttachment{
				b.larkResourceAttachment("image", "image", key, "", messageID),
			}, "")
		}
	case "file", "audio", "media":
		if key := str("file_key"); key != "" {
			return core.NewMediaContent([]core.MediaAttachment{
				b.larkResourceAttachment(larkCoreMediaType(msgType), "file", key, str("file_name"), messageID),
			}, "")
		}
	}

	if text := str("text"); text != "" {
		return core.NewTextContent(text)
	}
	return core.NewTextContent(rawContent)
}

// larkResourceAttachment builds a MediaAttachment that the FileStore downloads via
// the Feishu resource API. resType is "image" or "file" per the resource endpoint.
func (b *Bot) larkResourceAttachment(mediaType, resType, fileKey, fileName, messageID string) core.MediaAttachment {
	mimeType := "image/png"
	if resType != "image" {
		mimeType = mimeFromFileName(fileName)
	}
	return core.MediaAttachment{
		Type:     mediaType,
		URL:      larkResourceURLScheme + fileKey,
		MimeType: mimeType,
		Filename: fileName,
		Raw: map[string]interface{}{
			"feishu_message_id": messageID,
			"feishu_file_key":   fileKey,
			"feishu_res_type":   resType,
		},
	}
}

// larkCoreMediaType maps a Lark message type to a core media attachment type.
func larkCoreMediaType(msgType string) string {
	switch msgType {
	case "audio":
		return "audio"
	case "media":
		return "video"
	default:
		return "document"
	}
}

// mimeFromFileName infers a MIME type from a file name's extension.
func mimeFromFileName(name string) string {
	if ext := filepath.Ext(name); ext != "" {
		if mt := mime.TypeByExtension(ext); mt != "" {
			if i := strings.IndexByte(mt, ';'); i >= 0 {
				mt = mt[:i]
			}
			return strings.TrimSpace(mt)
		}
	}
	return "application/octet-stream"
}

// SendMessage sends a message using Lark SDK
func (b *Bot) SendMessage(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	// Handle text/card message
	if opts.Text != "" {
		return b.sendText(ctx, target, opts)
	}

	// Handle media
	if len(opts.Media) > 0 {
		return b.sendMedia(ctx, target, opts)
	}

	return nil, core.NewBotError(core.ErrUnknown, "no content to send", false)
}

// sendText sends a text or interactive card message
func (b *Bot) sendText(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	b.Logger().Debug("sendText called: target=%s, text=%s", target, opts.Text)

	// Safety checks
	if b == nil {
		return nil, fmt.Errorf("bot is nil")
	}
	if b.client == nil {
		return nil, fmt.Errorf("bot client is nil")
	}

	// Validate text length
	if err := b.ValidateTextLength(opts.Text); err != nil {
		return nil, err
	}

	// Check for a pre-rendered interactive card (e.g. action menus). This takes
	// precedence because the card JSON already encodes its own buttons/callbacks.
	if cardJSON, ok := opts.Metadata["card_json"].(string); ok && cardJSON != "" {
		return b.sendRawCard(ctx, target, cardJSON)
	}

	// Check for interactive card (keyboard)
	if replyMarkup, hasKeyboard := opts.Metadata["replyMarkup"]; hasKeyboard {
		return b.sendInteractiveCard(ctx, target, opts, replyMarkup)
	}

	// Regular text message using SDK builder
	var msgType string
	var content string

	if opts.ParseMode == core.ParseModeMarkdown {
		// For Lark/Feishu, markdown in card needs to be wrapped in div element
		// MessageCardLarkMd implements MessageCardText interface
		msgType = "interactive"
		cardJson, err := larkcard.NewMessageCard().
			Elements([]larkcard.MessageCardElement{
				larkcard.NewMessageCardDiv().
					Text(larkcard.NewMessageCardLarkMd().Content(opts.Text)),
			}).
			String()
		if err != nil {
			return nil, fmt.Errorf("failed to build card: %w", err)
		}
		content = cardJson
	} else {
		msgType = "text"
		content = fmt.Sprintf(`{"text":%q}`, opts.Text)
	}

	b.Logger().Debug("Sending message: msgType=%s, target=%s", msgType, target)

	// Check if Im service is available
	if b.client.Im == nil {
		return nil, fmt.Errorf("client.Im is nil - SDK not properly initialized")
	}

	// Use the direct client Post method for sending
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(getReceiveIdType(target)).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(target).
			MsgType(msgType).
			Content(content).
			Build()).
		Build()

	b.Logger().Debug("Sending message request: target=%s, msgType=%s, receiveIdType=%s", target, msgType, getReceiveIdType(target))
	resp, err := b.client.Im.Message.Create(context.Background(), req)

	if err != nil {
		b.Logger().Error("Failed to send message: %v", err)
		return nil, core.WrapError(err, core.Platform(b.domain), core.ErrPlatformError)
	}

	// Check if the API call was successful (code 0)
	if resp.Code != 0 {
		b.Logger().Error("API returned error code: %d, msg: %s", resp.Code, resp.Msg)
		return nil, core.NewBotError(core.ErrPlatformError, fmt.Sprintf("API error: %s", resp.Msg), false)
	}

	b.UpdateLastActivity()
	messageID := b.extractMessageIDFromResponse(resp)
	b.Logger().Info("Message sent successfully: ID=%s", messageID)
	return &core.SendResult{
		MessageID: messageID,
		Timestamp: 0,
	}, nil
}

// createMessage sends a message of the given type/content to the target.
func (b *Bot) createMessage(ctx context.Context, target, msgType, content string) (*core.SendResult, error) {
	if b.client == nil || b.client.Im == nil {
		return nil, fmt.Errorf("client.Im is nil")
	}

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(getReceiveIdType(target)).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(target).
			MsgType(msgType).
			Content(content).
			Build()).
		Build()

	resp, err := b.client.Im.Message.Create(ctx, req)
	if err != nil {
		b.Logger().Error("Failed to send %s message: %v", msgType, err)
		return nil, core.WrapError(err, core.Platform(b.domain), core.ErrPlatformError)
	}
	if resp.Code != 0 {
		b.Logger().Error("API returned error code: %d, msg: %s", resp.Code, resp.Msg)
		return nil, core.NewBotError(core.ErrPlatformError, fmt.Sprintf("API error: %s", resp.Msg), false)
	}

	b.UpdateLastActivity()
	messageID := b.extractMessageIDFromResponse(resp)
	b.Logger().Info("%s message sent successfully: ID=%s", msgType, messageID)
	return &core.SendResult{MessageID: messageID, Timestamp: 0}, nil
}

// sendRawCard sends a pre-serialized interactive card JSON string.
func (b *Bot) sendRawCard(ctx context.Context, target string, cardJSON string) (*core.SendResult, error) {
	return b.createMessage(ctx, target, "interactive", cardJSON)
}

// sendInteractiveCard sends an interactive card with buttons
func (b *Bot) sendInteractiveCard(ctx context.Context, target string, opts *core.SendMessageOptions, replyMarkup interface{}) (*core.SendResult, error) {
	b.Logger().Debug("sendInteractiveCard called: target=%s", target)

	// Safety checks
	if b.client == nil {
		return nil, fmt.Errorf("bot client is nil")
	}
	if b.client.Im == nil {
		return nil, fmt.Errorf("client.Im is nil")
	}

	card := b.buildInteractiveCard(opts.Text, replyMarkup)
	cardJson, err := card.String()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize card: %w", err)
	}

	msgType := "interactive"
	b.Logger().Debug("Sending card: type=%s", msgType)

	// Use SDK builder pattern
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(getReceiveIdType(target)).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(target).
			MsgType(msgType).
			Content(cardJson).
			Build()).
		Build()

	b.Logger().Debug("Sending card request: target=%s, receiveIdType=%s", target, getReceiveIdType(target))
	resp, err := b.client.Im.Message.Create(ctx, req)
	if err != nil {
		b.Logger().Error("Failed to send card: %v", err)
		return nil, core.WrapError(err, core.Platform(b.domain), core.ErrPlatformError)
	}

	// Check if the API call was successful (code 0)
	if resp.Code != 0 {
		b.Logger().Error("API returned error code: %d, msg: %s", resp.Code, resp.Msg)
		return nil, core.NewBotError(core.ErrPlatformError, fmt.Sprintf("API error: %s", resp.Msg), false)
	}

	b.UpdateLastActivity()
	messageID := b.extractMessageIDFromResponse(resp)
	b.Logger().Info("Card sent successfully: ID=%s", messageID)
	return &core.SendResult{
		MessageID: messageID,
		Timestamp: 0,
	}, nil
}

// buildInteractiveCard builds a Lark interactive card from text and keyboard markup
func (b *Bot) buildInteractiveCard(text string, replyMarkup interface{}) *larkcard.MessageCard {
	elements := []larkcard.MessageCardElement{
		larkcard.NewMessageCardDiv().
			Text(larkcard.NewMessageCardLarkMd().Content(text)),
	}

	// Convert keyboard markup to action buttons
	// Try to convert from InlineKeyboardMarkup or map format
	var buttons []larkcard.MessageCardActionElement

	// Handle InlineKeyboardMarkup type
	if kb, ok := replyMarkup.(interaction.InlineKeyboardMarkup); ok {
		for _, row := range kb.InlineKeyboard {
			for _, btn := range row {
				button := larkcard.NewMessageCardEmbedButton().
					Text(larkcard.NewMessageCardPlainText().Content(btn.Text)).
					Type(larkcard.MessageCardButtonTypeDefault).
					Value(map[string]interface{}{
						"callback": btn.CallbackData,
					})
				buttons = append(buttons, button)
			}
		}
	} else if kbMap, ok := replyMarkup.(map[string]interface{}); ok {
		// Handle map format (from JSON unmarshaling)
		if inlineKeyboard, ok := kbMap["inline_keyboard"].([]interface{}); ok {
			for _, row := range inlineKeyboard {
				if rowArray, ok := row.([]interface{}); ok {
					for _, btn := range rowArray {
						if btnMap, ok := btn.(map[string]interface{}); ok {
							buttonText, _ := btnMap["text"].(string)
							callbackData, _ := btnMap["callback_data"].(string)

							button := larkcard.NewMessageCardEmbedButton().
								Text(larkcard.NewMessageCardPlainText().Content(buttonText)).
								Type(larkcard.MessageCardButtonTypeDefault).
								Value(map[string]interface{}{
									"callback": callbackData,
								})
							buttons = append(buttons, button)
						}
					}
				}
			}
		}
	}

	if len(buttons) > 0 {
		layout := larkcard.MessageCardActionLayoutFlow
		action := larkcard.NewMessageCardAction().
			Layout(&layout).
			Actions(buttons)
		elements = append(elements, action)
	}

	wideScreen := true
	return larkcard.NewMessageCard().
		Config(larkcard.NewMessageCardConfig().WideScreenMode(wideScreen)).
		Elements(elements).
		Build()
}

// sendMedia uploads each attachment and sends it as an image or file message.
// Images are sent as "image" messages; everything else as "file" messages.
func (b *Bot) sendMedia(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if len(opts.Media) == 0 {
		return nil, core.NewBotError(core.ErrUnknown, "no media to send", false)
	}
	if b.client == nil || b.client.Im == nil {
		return nil, fmt.Errorf("client.Im is nil")
	}

	var last *core.SendResult
	for _, m := range opts.Media {
		var msgType, content string
		switch m.Type {
		case "image", "sticker", "gif":
			key, err := b.uploadImage(ctx, m)
			if err != nil {
				return nil, err
			}
			msgType, content = "image", fmt.Sprintf(`{"image_key":%q}`, key)
		default:
			key, err := b.uploadFile(ctx, m)
			if err != nil {
				return nil, err
			}
			msgType, content = "file", fmt.Sprintf(`{"file_key":%q}`, key)
		}

		res, err := b.createMessage(ctx, target, msgType, content)
		if err != nil {
			return nil, err
		}
		last = res
	}
	return last, nil
}

// uploadImage uploads an image attachment and returns its image_key.
func (b *Bot) uploadImage(ctx context.Context, m core.MediaAttachment) (string, error) {
	reader, closeFn, err := openMediaReader(ctx, m.URL)
	if err != nil {
		return "", fmt.Errorf("open image %q: %w", m.URL, err)
	}
	defer closeFn()

	req := larkim.NewCreateImageReqBuilder().
		Body(larkim.NewCreateImageReqBodyBuilder().
			ImageType("message").
			Image(reader).
			Build()).
		Build()

	resp, err := b.client.Im.Image.Create(ctx, req)
	if err != nil {
		return "", core.WrapError(err, core.Platform(b.domain), core.ErrPlatformError)
	}
	if !resp.Success() || resp.Data == nil || resp.Data.ImageKey == nil {
		return "", core.NewBotError(core.ErrPlatformError, fmt.Sprintf("upload image failed: code=%d msg=%s", resp.Code, resp.Msg), false)
	}
	return *resp.Data.ImageKey, nil
}

// uploadFile uploads a file attachment and returns its file_key.
func (b *Bot) uploadFile(ctx context.Context, m core.MediaAttachment) (string, error) {
	reader, closeFn, err := openMediaReader(ctx, m.URL)
	if err != nil {
		return "", fmt.Errorf("open file %q: %w", m.URL, err)
	}
	defer closeFn()

	fileName := m.Filename
	if fileName == "" {
		fileName = filepath.Base(m.URL)
	}
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = "file"
	}

	req := larkim.NewCreateFileReqBuilder().
		Body(larkim.NewCreateFileReqBodyBuilder().
			FileType(feishuFileType(fileName)).
			FileName(fileName).
			File(reader).
			Build()).
		Build()

	resp, err := b.client.Im.File.Create(ctx, req)
	if err != nil {
		return "", core.WrapError(err, core.Platform(b.domain), core.ErrPlatformError)
	}
	if !resp.Success() || resp.Data == nil || resp.Data.FileKey == nil {
		return "", core.NewBotError(core.ErrPlatformError, fmt.Sprintf("upload file failed: code=%d msg=%s", resp.Code, resp.Msg), false)
	}
	return *resp.Data.FileKey, nil
}

// feishuFileType maps a file name to a Feishu file_type. The API accepts
// opus/mp4/pdf/doc/xls/ppt and "stream" for anything else.
func feishuFileType(name string) string {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")) {
	case "pdf":
		return "pdf"
	case "doc", "docx":
		return "doc"
	case "xls", "xlsx":
		return "xls"
	case "ppt", "pptx":
		return "ppt"
	case "mp4":
		return "mp4"
	case "opus":
		return "opus"
	default:
		return "stream"
	}
}

// openMediaReader returns a reader for an outbound media URL. It supports local
// file paths (optionally file://) and http(s) URLs. The returned close function
// must always be called.
func openMediaReader(ctx context.Context, mediaURL string) (io.Reader, func(), error) {
	if strings.HasPrefix(mediaURL, "http://") || strings.HasPrefix(mediaURL, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
		if err != nil {
			return nil, func() {}, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, func() {}, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, func() {}, fmt.Errorf("download failed: status %d", resp.StatusCode)
		}
		return resp.Body, func() { resp.Body.Close() }, nil
	}

	path := strings.TrimPrefix(mediaURL, "file://")
	f, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	return f, func() { f.Close() }, nil
}

// extractMessageIDFromResponse extracts the message ID from a Lark API response
// Handles the case where resp.Data is nil even when the API call is successful (code=0)
func (b *Bot) extractMessageIDFromResponse(resp *larkim.CreateMessageResp) string {
	if resp.Code != 0 {
		return ""
	}

	if resp.Data != nil && resp.Data.MessageId != nil {
		messageID := *resp.Data.MessageId
		b.Logger().Debug("Extracted message ID: %s", messageID)
		return messageID
	}

	// For Lark/Feishu, sometimes Data is nil even on success
	// Generate a fallback message ID for tracking
	b.Logger().Warn("Response.Data is nil, but API call succeeded (code=0)")
	return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}

// PlatformInfo returns platform information
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(core.Platform(b.domain), b.domain.DisplayName())
}

// DisplayName returns the display name for the domain
func (d Domain) DisplayName() string {
	switch d {
	case DomainFeishu:
		return "Feishu"
	case DomainLark:
		return "Lark"
	default:
		return string(d)
	}
}

// SendText sends a simple text message
func (b *Bot) SendText(ctx context.Context, target string, text string) (*core.SendResult, error) {
	return b.SendMessage(ctx, target, &core.SendMessageOptions{Text: text})
}

// SendMedia sends media
func (b *Bot) SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error) {
	return b.SendMessage(ctx, target, &core.SendMessageOptions{Media: media})
}

// DownloadMessageResource downloads an image or file attachment from a received
// message via the Feishu resource API. resType must be "image" or "file".
func (b *Bot) DownloadMessageResource(ctx context.Context, messageID, fileKey, resType string) (io.Reader, error) {
	if b.client == nil || b.client.Im == nil {
		return nil, fmt.Errorf("feishu client not initialized")
	}

	req := larkim.NewGetMessageResourceReqBuilder().
		MessageId(messageID).
		FileKey(fileKey).
		Type(resType).
		Build()

	resp, err := b.client.Im.MessageResource.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("feishu download resource: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("feishu download resource failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return resp.File, nil
}

// React reacts to a message using Feishu SDK MessageReaction API.
// emoji should be a platform-specific key (e.g. "THUMBSUP") or a core.Reaction
// resolved via core.ResolveReaction before calling.
func (b *Bot) React(ctx context.Context, messageID string, emoji string) error {
	if b.client == nil {
		return fmt.Errorf("feishu client not initialized")
	}

	req := larkim.NewCreateMessageReactionReqBuilder().
		MessageId(messageID).
		Body(larkim.NewCreateMessageReactionReqBodyBuilder().
			ReactionType(larkim.NewEmojiBuilder().EmojiType(emoji).Build()).
			Build()).
		Build()

	resp, err := b.client.Im.MessageReaction.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("feishu react: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("feishu react failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

// EditMessage edits a previously sent message.
// Feishu only supports editing interactive (card) messages via the Patch API, so the
// new content is wrapped in a markdown card before patching.
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	if b.client == nil || b.client.Im == nil {
		return fmt.Errorf("feishu client not initialized")
	}
	if messageID == "" {
		return core.NewBotError(core.ErrInvalidTarget, "message ID is required", false)
	}

	cardJSON, err := larkcard.NewMessageCard().
		Config(larkcard.NewMessageCardConfig().WideScreenMode(true)).
		Elements([]larkcard.MessageCardElement{
			larkcard.NewMessageCardDiv().
				Text(larkcard.NewMessageCardLarkMd().Content(text)),
		}).
		String()
	if err != nil {
		return fmt.Errorf("failed to build card: %w", err)
	}

	req := larkim.NewPatchMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewPatchMessageReqBodyBuilder().
			Content(cardJSON).
			Build()).
		Build()

	resp, err := b.client.Im.Message.Patch(ctx, req)
	if err != nil {
		return core.WrapError(err, core.Platform(b.domain), core.ErrPlatformError)
	}
	if !resp.Success() {
		return core.NewBotError(core.ErrPlatformError, fmt.Sprintf("API error: %s", resp.Msg), false)
	}

	b.UpdateLastActivity()
	return nil
}

// DeleteMessage recalls a previously sent message by its open_message_id.
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	if b.client == nil || b.client.Im == nil {
		return fmt.Errorf("feishu client not initialized")
	}
	if messageID == "" {
		return core.NewBotError(core.ErrInvalidTarget, "message ID is required", false)
	}

	req := larkim.NewDeleteMessageReqBuilder().MessageId(messageID).Build()
	resp, err := b.client.Im.Message.Delete(ctx, req)
	if err != nil {
		return core.WrapError(err, core.Platform(b.domain), core.ErrPlatformError)
	}
	if !resp.Success() {
		return core.NewBotError(core.ErrPlatformError, fmt.Sprintf("API error: %s", resp.Msg), false)
	}
	return nil
}
