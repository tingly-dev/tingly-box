package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Bot implements the WhatsApp bot
type Bot struct {
	*core.BaseBot
	client  *http.Client
	apiURL  string
	apiKey  string
	phoneID string
	authDir string
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.RWMutex
}

// WhatsAppAPIResponse represents the WhatsApp Business API response
type WhatsAppAPIResponse struct {
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

// SendMessageResponse represents the send message response
type SendMessageResponse struct {
	MessagingProduct string `json:"messaging_product"`
	Contacts         []struct {
		Input string `json:"input"`
		WaID  string `json:"wa_id"`
	} `json:"contacts"`
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

// MessageEvent represents an incoming webhook event from WhatsApp
type MessageEvent struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumber        string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WaID string `json:"wa_id"`
				} `json:"contacts"`
				Messages []struct {
					From      string `json:"from"`
					ID        string `json:"id"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text,omitempty"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// NewWhatsAppBot creates a new WhatsApp bot
func NewWhatsAppBot(config *core.Config) (*Bot, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// WhatsApp supports QR auth or token auth
	bot := &Bot{
		BaseBot: core.NewBaseBot(config),
		client:  &http.Client{Timeout: 30 * time.Second},
		apiURL:  config.GetOptionString("apiUrl", "https://graph.facebook.com/v20.0"),
		authDir: config.GetOptionString("authDir", "./whatsapp-auth"),
	}

	// Handle different auth types
	if config.Auth.Type == "qr" {
		// QR authentication - would require Baileys integration
		// For now, we'll need an API key from a connected session
		bot.apiKey = config.GetOptionString("apiKey", "")
		if bot.apiKey == "" {
			return nil, core.NewAuthFailedError(config.Platform, "QR auth requires Baileys integration or pre-configured API key", nil)
		}
	} else if config.Auth.Type == "token" {
		bot.apiKey = config.Auth.Token
		if bot.apiKey == "" {
			return nil, core.NewAuthFailedError(config.Platform, "API key/token is required", nil)
		}
	} else {
		return nil, core.NewAuthFailedError(config.Platform, "whatsapp requires QR or token auth", nil)
	}

	bot.phoneID = config.GetOptionString("phoneId", "")

	return bot, nil
}

// Connect connects to WhatsApp
func (b *Bot) Connect(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Test authentication with a simple API call
	if err := b.authenticate(); err != nil {
		return err
	}

	b.UpdateConnected(true)
	b.UpdateAuthenticated(true)
	b.EmitConnected()
	b.Logger().Info("WhatsApp bot connected: phoneID=%s", b.phoneID)

	// Start receiving events (via webhook polling or websocket)
	b.wg.Add(1)
	go b.receiveEvents()

	return nil
}

// Disconnect disconnects from WhatsApp
func (b *Bot) Disconnect(ctx context.Context) error {
	if b.cancel != nil {
		b.cancel()
	}

	b.wg.Wait()

	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()
	b.Logger().Info("WhatsApp bot disconnected")

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

	// WhatsApp uses send endpoint with reaction type
	url := fmt.Sprintf("%s/%s/messages", b.apiURL, b.phoneID)

	reqBody := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                target,
		"type":              "reaction",
		"reaction": map[string]string{
			"message_id": messageID,
			"emoji":      emoji,
		},
	}

	if err := b.doRequest(ctx, "POST", url, reqBody); err != nil {
		return core.WrapError(err, core.PlatformWhatsApp, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return nil
}

// EditMessage edits a message
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	// WhatsApp doesn't have native edit, but we can send a new message
	// In a real implementation, you'd handle this differently
	b.Logger().Debug("WhatsApp doesn't support edit, sending new message instead")
	return nil
}

// DeleteMessage deletes a message
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	// WhatsApp requires deletion via the API
	url := fmt.Sprintf("%s/%s/messages/%s", b.apiURL, b.phoneID, messageID)

	if err := b.doRequest(ctx, "DELETE", url, nil); err != nil {
		return core.WrapError(err, core.PlatformWhatsApp, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	return nil
}

// PlatformInfo returns platform information
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(core.PlatformWhatsApp, "WhatsApp")
}

// StartReceiving starts receiving messages
func (b *Bot) StartReceiving(ctx context.Context) error {
	return nil
}

// StopReceiving stops receiving messages
func (b *Bot) StopReceiving(ctx context.Context) error {
	return nil
}

// authenticate performs authentication test
func (b *Bot) authenticate() error {
	// Test with a simple phone number lookup
	url := fmt.Sprintf("%s/%s/phone_numbers", b.apiURL, b.phoneID)

	if err := b.doRequest(b.ctx, "GET", url, nil); err != nil {
		return core.WrapError(err, core.PlatformWhatsApp, core.ErrAuthFailed)
	}

	return nil
}

// receiveEvents receives events from WhatsApp
func (b *Bot) receiveEvents() {
	defer b.wg.Done()

	b.UpdateReady(true)
	b.EmitReady()

	// WhatsApp primarily uses webhooks
	// In a real implementation, you would:
	// 1. Start a webhook server
	// 2. Poll for events if configured
	// For now, we'll wait for webhook events

	b.Logger().Info("WhatsApp event receiver started (webhook mode)")
}

// sendText sends a text message
func (b *Bot) sendText(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	// Validate and chunk text
	if err := b.ValidateTextLength(opts.Text); err != nil {
		return nil, err
	}

	chunks := b.ChunkText(opts.Text)

	var lastMessageID string
	for _, chunk := range chunks {
		url := fmt.Sprintf("%s/%s/messages", b.apiURL, b.phoneID)

		// Build request body
		reqBody := map[string]interface{}{
			"messaging_product": "whatsapp",
			"recipient": map[string]string{
				"phone_number": target,
			},
			"type": "text",
			"text": map[string]string{
				"body": chunk,
			},
		}

		// Add reply if specified
		if opts.ReplyTo != "" {
			reqBody["context"] = map[string]string{
				"message_id": opts.ReplyTo,
			}
		}

		resp, err := b.sendAPIRequest(ctx, url, reqBody)
		if err != nil {
			return nil, err
		}

		// Parse response to get message ID
		var sendResp SendMessageResponse
		if err := json.Unmarshal(resp, &sendResp); err != nil {
			return nil, core.WrapError(err, core.PlatformWhatsApp, core.ErrPlatformError)
		}

		if len(sendResp.Messages) > 0 {
			lastMessageID = sendResp.Messages[0].ID
		}
	}

	b.UpdateLastActivity()
	return &core.SendResult{
		MessageID: lastMessageID,
		Timestamp: time.Now().Unix(),
	}, nil
}

// sendMedia sends media
func (b *Bot) sendMedia(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if len(opts.Media) == 0 {
		return nil, core.NewBotError(core.ErrUnknown, "no media to send", false)
	}

	media := opts.Media[0]

	url := fmt.Sprintf("%s/%s/media", b.apiURL, b.phoneID)

	// Build request body for media upload
	reqBody := map[string]interface{}{
		"messaging_product": "whatsapp",
		"recipient": map[string]string{
			"phone_number": target,
		},
		"type": "media",
		"media": map[string]string{
			"link": media.URL,
		},
	}

	// Add caption if provided
	if opts.Caption != "" {
		reqBody["text"] = map[string]string{
			"body": opts.Caption,
		}
	}

	resp, err := b.sendAPIRequest(ctx, url, reqBody)
	if err != nil {
		return nil, err
	}

	// Parse response
	var sendResp SendMessageResponse
	if err := json.Unmarshal(resp, &sendResp); err != nil {
		return nil, core.WrapError(err, core.PlatformWhatsApp, core.ErrPlatformError)
	}

	var messageID string
	if len(sendResp.Messages) > 0 {
		messageID = sendResp.Messages[0].ID
	}

	b.UpdateLastActivity()
	return &core.SendResult{
		MessageID: messageID,
		Timestamp: time.Now().Unix(),
	}, nil
}

// doRequest makes an HTTP request to WhatsApp API
func (b *Bot) doRequest(ctx context.Context, method, url string, body interface{}) error {
	var reqBody io.Reader

	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.apiKey)

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiResp WhatsAppAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err == nil && apiResp.Error != nil {
			return fmt.Errorf("API error: %s", apiResp.Error.Message)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return nil
}

// sendAPIRequest sends a request and returns the response body
func (b *Bot) sendAPIRequest(ctx context.Context, url string, body interface{}) ([]byte, error) {
	var reqBody io.Reader

	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.apiKey)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiResp WhatsAppAPIResponse
		body, _ := io.ReadAll(resp.Body)
		if json.Unmarshal(body, &apiResp) == nil && apiResp.Error != nil {
			return nil, fmt.Errorf("API error: %s", apiResp.Error.Message)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return io.ReadAll(resp.Body)
}

// HandleWebhook handles an incoming webhook event from WhatsApp
func (b *Bot) HandleWebhook(body []byte) error {
	var event MessageEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return err
	}

	if event.Object != "whatsapp_business_account" {
		return nil
	}

	// Process all entries and changes
	for _, entry := range event.Entry {
		for _, change := range entry.Changes {
			if change.Field == "messages" {
				b.handleWhatsAppMessages(change.Value.Messages)
			}
		}
	}

	return nil
}

// handleWhatsAppMessages handles incoming WhatsApp messages
func (b *Bot) handleWhatsAppMessages(messages []struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Text      struct {
		Body string `json:"body"`
	} `json:"text,omitempty"`
}) {
	for _, msg := range messages {
		// Only handle text messages for now
		if msg.Type != "text" {
			continue
		}

		// Create sender
		sender := core.Sender{
			ID:  msg.From,
			Raw: make(map[string]interface{}),
		}

		// Extract phone number from wa_id if available
		// Format: 1234567890@s.whatsapp.net
		if idx := findIndex(msg.From, "@"); idx != -1 {
			sender.DisplayName = msg.From[:idx]
		}

		// Create recipient
		recipient := core.Recipient{
			ID:   msg.From,
			Type: "direct",
		}

		// Create content
		content := core.NewTextContent(msg.Text.Body)

		// Parse timestamp
		timestamp := parseWhatsAppTimestamp(msg.Timestamp)

		// Create message
		message := core.Message{
			ID:        msg.ID,
			Platform:  core.PlatformWhatsApp,
			Timestamp: timestamp,
			Sender:    sender,
			Recipient: recipient,
			Content:   content,
			ChatType:  core.ChatTypeDirect,
			Metadata:  make(map[string]interface{}),
		}

		b.EmitMessage(message)
	}
}

// parseWhatsAppTimestamp parses WhatsApp timestamp (Unix in milliseconds)
func parseWhatsAppTimestamp(ts string) int64 {
	// WhatsApp timestamps are Unix milliseconds
	var ms int64
	if _, err := fmt.Sscanf(ts, "%d", &ms); err == nil {
		return ms / 1000
	}
	return 0
}

// findIndex finds the index of a substring in a string
func findIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
