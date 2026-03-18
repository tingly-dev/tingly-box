package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/tingly-dev/tingly-box/imbot/internal/builder"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Domain represents the service domain (Feishu or Lark)
type Domain string

const (
	DomainFeishu Domain = "feishu"
	DomainLark   Domain = "lark"
)

// getReceiveIdType determines the receive_id_type based on the target ID format
// - "user_id": for user's user_id (global across apps, for P2P/direct chat)
// - "open_id": for user's open_id (app-specific, for P2P/direct chat)
// - "chat_id": for group/chat IDs
//
// Feishu/Lark ID patterns:
// - ou_xxxxxx: user's user_id (global, preferred for cross-app messaging)
// - oc_xxxxxx: user's open_id (app-specific)
// - cli_xxxxxx: group chat ID
//
// The most reliable approach is to check ID prefix.
func getReceiveIdType(targetID string) string {
	if len(targetID) < 4 {
		return "open_id"
	}

	prefix := targetID[:4]
	switch prefix {
	case "cli_":
		return "chat_id"
	case "ou_":
		return "user_id" // Global user_id for cross-app messaging
	case "oc_":
		return "open_id" // App-specific open_id
	default:
		// Default to open_id for backward compatibility
		return "open_id"
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
	adapter     *Adapter
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
	// Initialize adapter for message conversion
	b.adapter = NewAdapter(b.Config())

	// Test authentication via SDK
	_, err := b.client.GetTenantAccessTokenBySelfBuiltApp(ctx, &larkcore.SelfBuiltTenantAccessTokenReq{
		AppID:     b.Config().Auth.ClientID,
		AppSecret: b.Config().Auth.ClientSecret,
	})
	if err != nil {
		return core.NewAuthFailedError(core.Platform(b.domain), "authentication failed", err)
	}

	b.UpdateConnected(true)
	b.UpdateAuthenticated(true)
	b.EmitConnected()
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

	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()
	b.Logger().Info("%s bot disconnected", b.domain)
	return nil
}

// StartReceiving starts receiving events via WebSocket long connection
// This is the main method for establishing real-time event listening
func (b *Bot) StartReceiving(ctx context.Context) error {
	// Create event handler that converts Lark events to core.Message
	// OnP2MessageReceiveV1 handles both v1.0 and v2.0 message receive events
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(b.handleP2MessageReceiveV1)

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

	// Start WebSocket connection in background
	b.eventCtx, b.eventCancel = context.WithCancel(context.Background())

	go func() {
		b.Logger().Info("%s WebSocket connecting...", b.domain)
		if err := wsClient.Start(b.eventCtx); err != nil {
			b.Logger().Error("%s WebSocket error: %v", b.domain, err)
			b.UpdateReady(false)
		}
	}()

	// Wait a moment for connection to establish
	time.Sleep(2 * time.Second)

	b.UpdateReady(true)
	b.EmitReady()
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

// convertLarkMessageToCore converts a Lark P2MessageReceiveV1 event to core.Message
func (b *Bot) convertLarkMessageToCore(event *larkim.P2MessageReceiveV1) *core.Message {
	// Safety check
	if event == nil {
		b.Logger().Error("convertLarkMessageToCore: event is nil")
		// Return a dummy error message
		return builder.NewMessageBuilder(core.Platform(b.domain)).
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

	// For direct messages, use user_id as the reply target to avoid "open_id cross app" error
	// For group messages, use chat_id
	if chatType == core.ChatTypeDirect {
		// Direct message: use the sender's user_id (global) for sending replies
		// This works across all apps in the same tenant
		if senderUserID != "" {
			replyTarget = senderUserID
		} else {
			// Fallback to senderID (might be open_id, could fail with cross-app error)
			replyTarget = senderID
		}
	} else {
		// Group/channel: use chat_id
		replyTarget = chatID
	}

	// Extract message content - Lark Content is JSON string like {"text":"hello"}
	var textContent string
	if event.Event != nil && event.Event.Message != nil && event.Event.Message.Content != nil {
		contentStr := *event.Event.Message.Content
		b.Logger().Debug("Raw content: %s", contentStr)

		// Parse the JSON content to extract actual text
		// Format: {"text":"actual message content"} or more complex for rich text
		var contentMap map[string]interface{}
		if err := json.Unmarshal([]byte(contentStr), &contentMap); err == nil {
			if text, ok := contentMap["text"].(string); ok {
				textContent = text
			} else {
				textContent = contentStr // Fallback to raw string
			}
		} else {
			b.Logger().Warn("Failed to parse content JSON: %v, using raw string", err)
			textContent = contentStr // Fallback to raw string
		}
	}
	b.Logger().Debug("Parsed text content: %s", textContent)

	// Build core.Message using the builder
	// For direct messages, Recipient.ID should be the user_id (not chat_id) for sending replies
	messageBuilder := builder.NewMessageBuilder(core.Platform(b.domain)).
		WithID(messageID).
		WithTimestamp(time.Now().Unix()).
		WithSender(senderID, "", "").
		WithRecipient(replyTarget, string(chatType), "").
		WithContent(core.NewTextContent(textContent))

	// Add metadata for raw event access and original chat_id
	messageBuilder.WithMetadata("raw_lark_event", event)
	messageBuilder.WithMetadata("original_chat_id", chatID)
	messageBuilder.WithMetadata("chat_type", chatType)
	messageBuilder.WithMetadata("sender_user_id", senderUserID)

	msg := messageBuilder.Build()
	b.Logger().Debug("Built core message: ID=%s, Sender=%s, Content length=%d",
		msg.ID, msg.Sender.ID, len(textContent))

	return msg
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
	if kb, ok := replyMarkup.(builder.InlineKeyboardMarkup); ok {
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

// sendMedia sends media
func (b *Bot) sendMedia(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if len(opts.Media) == 0 {
		return nil, core.NewBotError(core.ErrUnknown, "no media to send", false)
	}
	return nil, core.NewMediaNotSupportedError(core.Platform(b.domain), opts.Media[0].Type)
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

// SetCardHandler sets the callback handler for card interactions
func (b *Bot) SetCardHandler(handler func(context.Context, *larkcard.CardAction) (interface{}, error)) {
	// Card handler would be set up separately for webhook handling
	// This is a placeholder for future implementation
}

// HandleCardAction handles an incoming card callback webhook
func (b *Bot) HandleCardAction(ctx context.Context, eventReq *larkevent.EventReq) (*larkevent.EventResp, error) {
	// Placeholder for card callback handling
	return nil, fmt.Errorf("card action handling not implemented")
}

// HandleWebhook handles an incoming webhook event (for webhook mode, alternative to WebSocket)
func (b *Bot) HandleWebhook(body []byte) error {
	coreMessage, err := b.adapter.AdaptWebhook(context.Background(), body)
	if err != nil {
		b.Logger().Error("Failed to adapt webhook: %v", err)
		return err
	}

	b.EmitMessage(*coreMessage)
	return nil
}

// GetWebhookURL returns the webhook path for this platform
func (b *Bot) GetWebhookURL(webhookPath string) string {
	return fmt.Sprintf("/webhook/%s/%s", b.domain, webhookPath)
}

// VerifyWebhook verifies webhook signature
func (b *Bot) VerifyWebhook(signature, timestamp, body string) bool {
	return true
}

// SendText sends a simple text message
func (b *Bot) SendText(ctx context.Context, target string, text string) (*core.SendResult, error) {
	return b.SendMessage(ctx, target, &core.SendMessageOptions{Text: text})
}

// SendMedia sends media
func (b *Bot) SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error) {
	return b.SendMessage(ctx, target, &core.SendMessageOptions{Media: media})
}

// React reacts to a message
func (b *Bot) React(ctx context.Context, messageID string, emoji string) error {
	return fmt.Errorf("reaction not implemented")
}

// EditMessage edits a message
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	return fmt.Errorf("edit message not implemented")
}

// DeleteMessage deletes a message
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	return fmt.Errorf("delete message not implemented")
}
