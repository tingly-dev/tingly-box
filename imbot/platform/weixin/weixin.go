// Package weixin provides Weixin platform bot implementation for ImBot.
//
// This package implements the core.Bot interface for Weixin messaging,
// bridging the ImBot platform layer with the Weixin channel plugin.
package weixin

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/weixin"
	wechatadapters "github.com/tingly-dev/weixin/adapters"
	weixinapi "github.com/tingly-dev/weixin/api"
	"github.com/tingly-dev/weixin/channel"
	"github.com/tingly-dev/weixin/contexttoken"
)

// Bot implements the Weixin platform bot
type Bot struct {
	*core.BaseBot
	plugin    *weixin.Plugin
	accountID string
	account   *weixin.WeChatAccount
	client    *weixinapi.Client
	adapter   *Adapter
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.RWMutex
}

// NewBot creates a new Weixin bot
func NewBot(config *core.Config) (*Bot, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Get Weixin credentials from AuthConfig
	// Token format: "bot_id:token_key" (combined)
	token := config.Auth.Token
	botID := config.Auth.AccountID // This contains bot_id
	userID := config.Auth.AuthDir  // We're reusing AuthDir to store user_id

	// Get base_url from options
	baseURL := config.GetOptionString("baseUrl", "")
	if baseURL == "" {
		baseURL = config.GetOptionString("base_url", "")
	}
	// Default to Weixin's official iLink endpoint
	if baseURL == "" {
		baseURL = "https://ilinkai.weixin.qq.com"
	}

	// Use account ID from bot_id if available, otherwise use default
	accountID := botID
	if accountID == "" {
		accountID = "default"
	}

	// Create Weixin plugin configuration
	wcConfig := &weixin.WeChatConfig{
		BaseURL: baseURL,
		BotType: config.GetOptionString("botType", "3"),
	}

	// Initialize plugin
	plugin := weixin.NewPlugin(wcConfig)

	// Initialize adapters for the plugin
	// This is required to enable all plugin functionality
	wechatadapters.InitPlugin(plugin)

	// Create account directly from auth config (no file storage needed)
	account := &weixin.WeChatAccount{
		ID:          accountID,
		Name:        fmt.Sprintf("Weixin Account %s", accountID),
		BotID:       botID,
		UserID:      userID,
		BotToken:    token,
		BaseURL:     baseURL,
		Enabled:     true,
		Configured:  token != "" && botID != "", // Consider configured if we have credentials
		CreatedAt:   time.Now(),
		LastLoginAt: time.Now(),
	}

	// Save the account to plugin's in-memory account manager
	// This allows the plugin to find the account later
	if err := plugin.Accounts().Save(account); err != nil {
		return nil, fmt.Errorf("failed to save account: %w", err)
	}

	bot := &Bot{
		BaseBot:   core.NewBaseBot(config),
		plugin:    plugin,
		accountID: accountID,
		account:   account, // Set account directly
	}

	// Set platform info
	// Platform info is set in base bot via config.Platform

	return bot, nil
}

// Connect connects to Weixin
func (b *Bot) Connect(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Get or load account
	account, err := b.getAccount()
	if err != nil {
		return core.NewAuthFailedError(core.PlatformWeixin, "failed to get account", err)
	}
	b.account = account

	// Check if account is configured
	if !account.Configured {
		return core.NewAuthFailedError(core.PlatformWeixin, "account not configured, please pair first", nil)
	}

	// Check if account is enabled
	if !account.Enabled {
		return fmt.Errorf("account is disabled")
	}

	// Create API client
	b.client = weixinapi.NewClient(account.BaseURL, account.BotToken)

	// Initialize adapter for message conversion
	b.adapter = NewAdapter(b.Config(), account)

	// Mark as connected
	b.UpdateConnected(true)
	b.UpdateAuthenticated(true)
	b.EmitConnected()
	b.Logger().Info("Weixin bot connected: account=%s", account.ID)

	// Start receiving messages
	b.wg.Add(1)
	go b.receiveMessages()

	return nil
}

// Disconnect disconnects from Weixin
func (b *Bot) Disconnect(ctx context.Context) error {
	if b.cancel != nil {
		b.cancel()
	}

	b.wg.Wait()

	// Stop the channel gateway
	if b.plugin != nil && b.accountID != "" {
		gateway := b.plugin.Gateway()
		if gateway != nil {
			_ = gateway.StopAccount(ctx, b.accountID)
		}
	}

	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()
	b.Logger().Info("Weixin bot disconnected")

	return nil
}

// SendMessage sends a message
func (b *Bot) SendMessage(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	// Ensure we have an account
	if b.account == nil {
		return nil, core.NewBotError(core.ErrConnectionFailed, "not connected", false)
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
	// Weixin doesn't have a native reaction feature
	return core.NewBotError(core.ErrPlatformError, "reactions not supported on Weixin", false)
}

// EditMessage edits a message
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}
	// Weixin doesn't support editing messages after sending
	return core.NewBotError(core.ErrPlatformError, "editing messages not supported on Weixin", false)
}

// DeleteMessage deletes a message
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	if err := b.EnsureReady(); err != nil {
		return err
	}
	// Weixin doesn't support deleting messages via API
	return core.NewBotError(core.ErrPlatformError, "deleting messages not supported on Weixin", false)
}

// PlatformInfo returns platform information
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(core.PlatformWeixin, "Weixin")
}

// StartReceiving starts receiving messages (already started in Connect)
func (b *Bot) StartReceiving(ctx context.Context) error {
	return nil // Already started in Connect
}

// StopReceiving stops receiving messages (already handled in Disconnect)
func (b *Bot) StopReceiving(ctx context.Context) error {
	return nil // Already handled in Disconnect
}

// GetAccount returns the current account
func (b *Bot) GetAccount() *weixin.WeChatAccount {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.account
}

// GetInteractionHandler returns the interaction handler for this bot
func (b *Bot) GetInteractionHandler() *InteractionHandler {
	return NewInteractionHandler(b)
}

// IsConfigured checks if the account is configured
func (b *Bot) IsConfigured() bool {
	account := b.GetAccount()
	return account != nil && account.Configured
}

// NeedsPairing checks if the account needs QR code pairing
func (b *Bot) NeedsPairing() bool {
	account := b.GetAccount()
	return account == nil || !account.Configured || account.BotToken == ""
}

// getAccount loads or creates an account
func (b *Bot) getAccount() (*weixin.WeChatAccount, error) {
	// Try to load existing account
	account, err := b.plugin.Accounts().Get(b.accountID)
	if err == nil {
		return account, nil
	}

	// Account doesn't exist, create a new one
	account = &weixin.WeChatAccount{
		ID:          b.accountID,
		Name:        fmt.Sprintf("Weixin Account %s", b.accountID),
		Enabled:     true,
		Configured:  false,
		BaseURL:     b.Config().GetOptionString("baseUrl", ""),
		CreatedAt:   time.Now(),
		LastLoginAt: time.Now(),
	}

	// Save the new account
	if err := b.plugin.Accounts().Save(account); err != nil {
		return nil, fmt.Errorf("failed to save account: %w", err)
	}

	return account, nil
}

// sendText sends a text message
func (b *Bot) sendText(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	// Validate text length
	if err := b.ValidateTextLength(opts.Text); err != nil {
		return nil, err
	}

	// Check if there's a context token from a reply
	var contextToken string
	if opts.Metadata != nil {
		if ct, ok := opts.Metadata["context_token"].(string); ok {
			contextToken = ct
		}
	}
	// If no context token in metadata, try to get from storage
	if contextToken == "" {
		contextToken = contexttoken.GetContextToken(b.accountID, target)
	}

	// Send via API - use simple text message
	if err := b.client.SendTextMessage(ctx, target, contextToken, opts.Text); err != nil {
		return nil, core.WrapError(err, core.PlatformWeixin, core.ErrPlatformError)
	}

	b.UpdateLastActivity()
	now := time.Now().Unix()
	return &core.SendResult{
		MessageID: fmt.Sprintf("weixin-%d", now),
		Timestamp: now,
	}, nil
}

// sendMedia sends media messages
func (b *Bot) sendMedia(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if len(opts.Media) == 0 {
		return nil, core.NewBotError(core.ErrUnknown, "no media to send", false)
	}

	// For now, send the first media item
	_ = opts.Media[0]

	// TODO: Implement media upload via CDN
	// For now, return an error
	return nil, core.NewBotError(core.ErrMediaNotSupported, "media upload not yet implemented", false)
}

// receiveMessages receives messages via long-polling
func (b *Bot) receiveMessages() {
	defer b.wg.Done()

	// Start the gateway for this account
	gateway := b.plugin.Gateway()
	if gateway == nil {
		b.Logger().Error("Gateway not available")
		return
	}

	if err := gateway.StartAccount(b.ctx, b.accountID); err != nil {
		b.Logger().Error("Failed to start account: %v", err)
		return
	}

	// Mark as ready
	b.UpdateReady(true)
	b.EmitReady()
	b.Logger().Info("Weixin bot ready: account=%s", b.accountID)

	// Use long-poll adapter to receive messages
	longPoll := b.plugin.LongPoll()
	if longPoll == nil {
		b.Logger().Error("LongPoll adapter not available")
		return
	}

	var syncBuf string

	for {
		select {
		case <-b.ctx.Done():
			return
		default:
			// Fetch updates
			req := &channel.GetUpdatesRequest{
				AccountID: b.accountID,
				SyncBuf:   syncBuf,
				TimeoutMs: 30000, // 30 seconds timeout in milliseconds
			}

			result, err := longPoll.GetUpdates(b.ctx, req)
			if err != nil {
				b.Logger().Error("Failed to get updates: %v", err)
				// Wait before retrying
				select {
				case <-time.After(5 * time.Second):
				case <-b.ctx.Done():
					return
				}
				continue
			}

			b.Logger().Debug("GetUpdates result: ErrCode=%d, Messages=%d, SyncBuf=%s", result.ErrCode, len(result.Messages), func() string {
				if len(result.SyncBuf) > 50 {
					return result.SyncBuf[:50] + "..."
				}
				return result.SyncBuf
			}())

			// Check for session expiration
			if result.ErrCode == -14 { // SessionExpiredErrCode from adapters package
				b.Logger().Error("Weixin session expired, need to re-authenticate")
				// Emit session expired event
				b.EmitError(core.NewAuthFailedError(core.PlatformWeixin, "session expired", nil))
				return
			}

			// Update sync buffer for next request
			syncBuf = result.SyncBuf

			// Process messages
			b.Logger().Info("Processing %d messages from Weixin", len(result.Messages))
			for _, msg := range result.Messages {
				b.Logger().Info("Handling message: ID=%s, From=%s, To=%s, Text=%s", msg.MessageID, msg.From, msg.To, msg.Text)
				b.handleMessage(msg)
			}
		}
	}
}

// handleMessage processes an incoming message
func (b *Bot) handleMessage(msg *channel.Message) {
	if msg == nil {
		return
	}

	// Use adapter to convert channel message to core message
	coreMsg, err := b.adapter.AdaptMessage(b.ctx, msg)
	if err != nil {
		b.Logger().Error("Failed to adapt message: %v", err)
		return
	}

	b.EmitMessage(*coreMsg)
}

// Close cleans up resources
func (b *Bot) Close() error {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	return nil
}
