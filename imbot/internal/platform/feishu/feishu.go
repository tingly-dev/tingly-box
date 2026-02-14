package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

const (
	// DefaultFeishuAPI is the default Feishu API endpoint
	DefaultFeishuAPI = "https://open.feishu.cn/open-apis"
	// DefaultLarkAPI is the default Lark API endpoint
	DefaultLarkAPI = "https://open.larksuite.com/open-apis"
)

// Bot implements the Feishu/Lark bot
type Bot struct {
	*core.BaseBot
	adapter     *Adapter // Local adapter for message conversion
	client      *http.Client
	apiURL      string
	appID       string
	appSecret   string
	encryptKey  string
	verifyToken string
	tenantKey   string
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	mu          sync.RWMutex
}

// FeishuAPIResponse represents the standard API response structure
type FeishuAPIResponse struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

// SendMessageRequest represents the send message request
type SendMessageRequest struct {
	ReceiveIDType string      `json:"receive_id_type"`
	ReceiveID     string      `json:"receive_id"`
	MsgType       string      `json:"msg_type"`
	Content       interface{} `json:"content"`
}

// TextContent represents text message content for Feishu
type TextContent struct {
	Text string `json:"text"`
}

// PostContent represents rich post message content for Feishu
type PostContent struct {
	Post FeishuPost `json:"post"`
}

// FeishuPost represents a rich post message
type FeishuPost struct {
	ZhCn FeishuPostContent `json:"zh_cn"`
}

// FeishuPostContent represents post content elements
type FeishuPostContent struct {
	Title   string                   `json:"title"`
	Content [][]FeishuContentElement `json:"content"`
}

// FeishuContentElement represents a content element
type FeishuContentElement struct {
	Tag  string `json:"tag"`
	Text string `json:"text,omitempty"`
	Href string `json:"href,omitempty"`
}

// MessageEvent represents an incoming message event
type MessageEvent struct {
	Header EventHeader        `json:"header"`
	Event  MessageEventDetail `json:"event"`
}

// EventHeader represents the event header
type EventHeader struct {
	EventID   string `json:"event_id"`
	Timestamp string `json:"timestamp"`
	Token     string `json:"token"`
	EventType string `json:"event_type"`
}

// MessageEventDetail represents message event details
type MessageEventDetail struct {
	MessageID  string        `json:"message_id"`
	RootID     interface{}   `json:"root_id"`
	ParentID   interface{}   `json:"parent_id"`
	CreateTime string        `json:"create_time"`
	ChatType   string        `json:"chat_type"`
	MsgType    string        `json:"msg_type"`
	Content    interface{}   `json:"content"`
	Mention    MentionDetail `json:"mention"`
	Sender     SenderDetail  `json:"sender"`
	ChatID     string        `json:"chat_id"`
}

// MentionDetail represents mention details
type MentionDetail struct {
	MentionList []MentionItem `json:"mention_list"`
}

// MentionItem represents a mention item
type MentionItem struct {
	ID        string `json:"id"`
	IDType    string `json:"id_type"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	TenantKey string `json:"tenant_key"`
}

// SenderDetail represents sender details
type SenderDetail struct {
	SenderID   string `json:"sender_id"`
	SenderType string `json:"sender_type"`
	TenantKey  string `json:"tenant_key"`
}

// NewFeishuBot creates a new Feishu/Lark bot
func NewFeishuBot(config *core.Config) (*Bot, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if config.Auth.Type != "oauth" {
		return nil, core.NewAuthFailedError(config.Platform, "feishu requires oauth auth", nil)
	}

	if config.Auth.ClientID == "" || config.Auth.ClientSecret == "" {
		return nil, core.NewAuthFailedError(config.Platform, "app ID and app secret are required", nil)
	}

	// Determine domain
	domain := config.GetOptionString("domain", "feishu")
	var apiURL string
	if domain == "lark" {
		apiURL = DefaultLarkAPI
	} else {
		apiURL = DefaultFeishuAPI
	}

	bot := &Bot{
		BaseBot:     core.NewBaseBot(config),
		client:      &http.Client{Timeout: 30 * time.Second},
		apiURL:      apiURL,
		appID:       config.Auth.ClientID,
		appSecret:   config.Auth.ClientSecret,
		encryptKey:  config.GetOptionString("encryptKey", ""),
		verifyToken: config.GetOptionString("verificationToken", ""),
		tenantKey:   config.GetOptionString("tenantKey", ""),
	}

	return bot, nil
}

// Connect connects to Feishu/Lark
func (b *Bot) Connect(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Initialize adapter for message conversion
	b.adapter = NewAdapter(b.Config())

	// Test authentication
	if err := b.authenticate(); err != nil {
		return core.NewAuthFailedError(core.PlatformFeishu, "authentication failed", err)
	}

	b.UpdateConnected(true)
	b.UpdateAuthenticated(true)
	b.EmitConnected()
	b.Logger().Info("Feishu/Lark bot connected: appID=%s", b.appID)

	// Start receiving events
	b.wg.Add(1)
	go b.receiveEvents()

	return nil
}

// Disconnect disconnects from Feishu/Lark
func (b *Bot) Disconnect(ctx context.Context) error {
	if b.cancel != nil {
		b.cancel()
	}

	b.wg.Wait()

	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()
	b.Logger().Info("Feishu/Lark bot disconnected")

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

	// Feishu uses add reaction API
	url := fmt.Sprintf("%s/v1/im/v1/messages/%s/reactions", b.apiURL, messageID)

	reqBody := map[string]interface{}{
		"emoji_type": "unicode",
		"emoji":      emoji,
	}

	// Get tenant access token
	token, err := b.getTenantAccessToken()
	if err != nil {
		return err
	}

	resp, err := b.doRequest(ctx, "POST", url, reqBody, token)
	if err != nil {
		return core.WrapError(err, core.PlatformFeishu, core.ErrPlatformError)
	}
	defer resp.Body.Close()

	b.UpdateLastActivity()
	return nil
}

// EditMessage edits a message
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/v1/im/v1/messages/%s", b.apiURL, messageID)

	reqBody := map[string]interface{}{
		"msg_type": "text",
		"content": map[string]string{
			"text": text,
		},
	}

	token, err := b.getTenantAccessToken()
	if err != nil {
		return err
	}

	resp, err := b.doRequest(ctx, "PATCH", url, reqBody, token)
	if err != nil {
		return core.WrapError(err, core.PlatformFeishu, core.ErrPlatformError)
	}
	defer resp.Body.Close()

	b.UpdateLastActivity()
	return nil
}

// DeleteMessage deletes a message
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/v1/im/v1/messages/%s", b.apiURL, messageID)

	token, err := b.getTenantAccessToken()
	if err != nil {
		return err
	}

	resp, err := b.doRequest(ctx, "DELETE", url, nil, token)
	if err != nil {
		return core.WrapError(err, core.PlatformFeishu, core.ErrPlatformError)
	}
	defer resp.Body.Close()

	b.UpdateLastActivity()
	return nil
}

// PlatformInfo returns platform information
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(core.PlatformFeishu, "Feishu/Lark")
}

// StartReceiving starts receiving messages (already started in Connect)
func (b *Bot) StartReceiving(ctx context.Context) error {
	return nil
}

// StopReceiving stops receiving messages
func (b *Bot) StopReceiving(ctx context.Context) error {
	return nil
}

// authenticate performs authentication and gets access token
func (b *Bot) authenticate() error {
	_, err := b.getTenantAccessToken()
	return err
}

// getTenantAccessToken gets the tenant access token
func (b *Bot) getTenantAccessToken() (string, error) {
	url := fmt.Sprintf("%s/auth/v3/tenant_access_token/internal", b.apiURL)

	reqBody := map[string]string{
		"app_id":     b.appID,
		"app_secret": b.appSecret,
	}

	var result struct {
		Code              int    `json:"code"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}

	resp, err := b.doRequest(context.Background(), "POST", url, reqBody, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Code != 0 {
		return "", fmt.Errorf("authentication failed: %s", result.TenantAccessToken)
	}

	return result.TenantAccessToken, nil
}

// receiveEvents receives events from Feishu/Lark
func (b *Bot) receiveEvents() {
	defer b.wg.Done()

	b.UpdateReady(true)
	b.EmitReady()

	// In a real implementation, you would start a webhook server
	// or use WebSocket to receive events
	// For now, this is a placeholder
	b.Logger().Info("Feishu/Lark event receiver started (webhook mode)")
}

// sendText sends a text message
func (b *Bot) sendText(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	// Validate text length
	if err := b.ValidateTextLength(opts.Text); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v1/im/v1/messages", b.apiURL)

	// Build content based on parse mode
	var content interface{}
	if opts.ParseMode == core.ParseModeMarkdown {
		content = map[string]interface{}{
			"post": map[string]interface{}{
				"zh_cn": map[string]interface{}{
					"title": "",
					"content": [][]map[string]interface{}{
						{
							{
								"tag":  "text",
								"text": opts.Text,
							},
						},
					},
				},
			},
		}
	} else {
		content = map[string]string{
			"text": opts.Text,
		}
	}

	msgType := "text"
	if opts.ParseMode == core.ParseModeMarkdown {
		msgType = "post"
	}

	reqBody := SendMessageRequest{
		ReceiveIDType: "chat_id",
		ReceiveID:     target,
		MsgType:       msgType,
		Content:       content,
	}

	// Add reply if specified
	if opts.ReplyTo != "" {
		reqBody.Content = map[string]interface{}{
			"post": map[string]interface{}{
				"zh_cn": map[string]interface{}{
					"title": "",
					"content": [][]map[string]interface{}{
						{
							{
								"tag":  "a",
								"text": opts.Text,
								"href": "",
							},
						},
					},
				},
				"replyInReply": true,
			},
		}
	}

	token, err := b.getTenantAccessToken()
	if err != nil {
		return nil, err
	}

	resp, err := b.doRequest(ctx, "POST", url, reqBody, token)
	if err != nil {
		return nil, core.WrapError(err, core.PlatformFeishu, core.ErrPlatformError)
	}
	defer resp.Body.Close()

	var result struct {
		Code      int    `json:"code"`
		Msg       string `json:"msg"`
		MessageID string `json:"msg_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, core.NewPlatformError(core.PlatformFeishu, result.Msg, nil, false)
	}

	b.UpdateLastActivity()
	return &core.SendResult{
		MessageID: result.MessageID,
		Timestamp: time.Now().Unix(),
	}, nil
}

// sendMedia sends media
func (b *Bot) sendMedia(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if len(opts.Media) == 0 {
		return nil, core.NewBotError(core.ErrUnknown, "no media to send", false)
	}

	media := opts.Media[0]

	url := fmt.Sprintf("%s/v1/im/v1/messages", b.apiURL)

	var msgType string
	var content interface{}

	switch media.Type {
	case "image":
		msgType = "image"
		content = map[string]string{
			"image_key": media.URL, // In real implementation, you'd upload first
		}
	case "video":
		msgType = "video"
		content = map[string]string{
			"video_key": media.URL,
		}
	case "audio":
		msgType = "audio"
		content = map[string]string{
			"file_key": media.URL,
		}
	case "document":
		msgType = "file"
		content = map[string]string{
			"file_key": media.URL,
		}
	default:
		return nil, core.NewMediaNotSupportedError(core.PlatformFeishu, media.Type)
	}

	reqBody := SendMessageRequest{
		ReceiveIDType: "chat_id",
		ReceiveID:     target,
		MsgType:       msgType,
		Content:       content,
	}

	token, err := b.getTenantAccessToken()
	if err != nil {
		return nil, err
	}

	resp, err := b.doRequest(ctx, "POST", url, reqBody, token)
	if err != nil {
		return nil, core.WrapError(err, core.PlatformFeishu, core.ErrPlatformError)
	}
	defer resp.Body.Close()

	var result struct {
		Code      int    `json:"code"`
		Msg       string `json:"msg"`
		MessageID string `json:"msg_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, core.NewPlatformError(core.PlatformFeishu, result.Msg, nil, false)
	}

	b.UpdateLastActivity()
	return &core.SendResult{
		MessageID: result.MessageID,
		Timestamp: time.Now().Unix(),
	}, nil
}

// doRequest makes an HTTP request to Feishu/Lark API
func (b *Bot) doRequest(ctx context.Context, method, url string, body interface{}, token string) (*http.Response, error) {
	var reqBody io.Reader

	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return b.client.Do(req)
}

// HandleWebhook handles an incoming webhook event
func (b *Bot) HandleWebhook(body []byte) error {
	// Use adapter to convert webhook to core message
	coreMessage, err := b.adapter.AdaptWebhook(b.ctx, body)
	if err != nil {
		b.Logger().Error("Failed to adapt webhook: %v", err)
		return err
	}

	b.EmitMessage(*coreMessage)
	return nil
}

// GetWebhookURL returns the webhook URL for receiving events
func (b *Bot) GetWebhookURL(webhookPath string) string {
	domain := "feishu"
	if b.apiURL == DefaultLarkAPI {
		domain = "lark"
	}

	return fmt.Sprintf("/webhook/%s/%s", domain, webhookPath)
}

// VerifyWebhook verifies webhook signature
func (b *Bot) VerifyWebhook(signature, timestamp, body string) bool {
	// In real implementation, you would verify the HMAC signature
	// using the encryptKey
	return true
}
