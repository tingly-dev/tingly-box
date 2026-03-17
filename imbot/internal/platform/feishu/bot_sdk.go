package feishu

import (
	"context"
	"fmt"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/tingly-dev/tingly-box/imbot/internal/builder"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Domain represents the service domain (Feishu or Lark)
type Domain string

const (
	DomainFeishu Domain = "feishu"
	DomainLark   Domain = "lark"
)

// Bot is the Lark SDK-based bot implementation
// Supports both Feishu and Lark platforms via domain configuration
type Bot struct {
	*core.BaseBot
	client  *lark.Client
	domain  Domain
	adapter *Adapter
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

	// Create Lark SDK client with domain-specific base URL
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

// Connect connects to Feishu/Lark using Lark SDK
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
	b.Logger().Info("%s bot connected: domain=%s", b.domain, b.domain)

	// Mark ready for receiving events
	b.UpdateReady(true)
	b.EmitReady()

	return nil
}

// Disconnect disconnects from Feishu/Lark
func (b *Bot) Disconnect(ctx context.Context) error {
	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()
	b.Logger().Info("%s bot disconnected", b.domain)
	return nil
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
	// Validate text length
	if err := b.ValidateTextLength(opts.Text); err != nil {
		return nil, err
	}

	// Check for interactive card (keyboard)
	if replyMarkup, hasKeyboard := opts.Metadata["replyMarkup"]; hasKeyboard {
		return b.sendInteractiveCard(ctx, target, opts, replyMarkup)
	}

	// Regular text message
	var msgType string
	var content string

	if opts.ParseMode == core.ParseModeMarkdown {
		msgType = "post"
		content = fmt.Sprintf(`{
			"post": {
				"zh_cn": {
					"title": "",
					"content": [[{"tag": "text", "text": %q}]]
				}
			}
		}`, opts.Text)
	} else {
		msgType = "text"
		content = fmt.Sprintf(`{"text":%q}`, opts.Text)
	}

	// Use the direct client Post method for sending
	resp, err := b.client.Im.Message.Create(ctx, &larkim.CreateMessageReq{
		Body: &larkim.CreateMessageReqBody{
			ReceiveId: &target,
			MsgType:   &msgType,
			Content:   &content,
		},
	})
	if err != nil {
		return nil, core.WrapError(err, core.Platform(b.domain), core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	messageID := ""
	if resp.Data.MessageId != nil {
		messageID = *resp.Data.MessageId
	}
	return &core.SendResult{
		MessageID: messageID,
		Timestamp: 0,
	}, nil
}

// sendInteractiveCard sends an interactive card with buttons
func (b *Bot) sendInteractiveCard(ctx context.Context, target string, opts *core.SendMessageOptions, replyMarkup interface{}) (*core.SendResult, error) {
	card := b.buildInteractiveCard(opts.Text, replyMarkup)
	cardJson, err := card.String()
	if err != nil {
		return nil, err
	}

	msgType := "interactive"

	resp, err := b.client.Im.Message.Create(ctx, &larkim.CreateMessageReq{
		Body: &larkim.CreateMessageReqBody{
			ReceiveId: &target,
			MsgType:   &msgType,
			Content:   &cardJson,
		},
	})
	if err != nil {
		return nil, core.WrapError(err, core.Platform(b.domain), core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	messageID := ""
	if resp.Data.MessageId != nil {
		messageID = *resp.Data.MessageId
	}
	return &core.SendResult{
		MessageID: messageID,
		Timestamp: 0,
	}, nil
}

// buildInteractiveCard builds a Lark interactive card from text and keyboard markup
func (b *Bot) buildInteractiveCard(text string, replyMarkup interface{}) *larkcard.MessageCard {
	elements := []larkcard.MessageCardElement{
		larkcard.NewMessageCardLarkMd().Content(text),
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

// HandleWebhook handles an incoming webhook event
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

// StartReceiving starts receiving events (no-op for webhook mode)
func (b *Bot) StartReceiving(ctx context.Context) error {
	return nil
}

// StopReceiving stops receiving events (no-op for webhook mode)
func (b *Bot) StopReceiving(ctx context.Context) error {
	return nil
}
