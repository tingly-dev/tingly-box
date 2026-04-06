// Package weixin provides Weixin platform bot implementation for ImBot.
//
// This package implements the core.Bot interface for Weixin messaging,
// bridging the ImBot platform layer with the Weixin channel plugin.
package weixin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/weixin/message/media"
	"github.com/tingly-dev/weixin/types"
	"github.com/tingly-dev/weixin/wechat"
)

// Bot implements the Weixin platform bot
type Bot struct {
	*core.BaseBot
	*wechat.WechatBot
	accountID string
	account   *wechat.Account
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
	wcConfig := &types.WeChatConfig{
		BaseURL: baseURL,
		BotType: config.GetOptionString("botType", "3"),
	}

	// Create in-memory store for account management
	// This avoids file system storage and allows integration with imbot's database
	store := NewMemoryStore()

	// Create WeChat account from auth config
	wcAccount := &types.WeChatAccount{
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

	// Save account to our store
	if err := store.Save(wcAccount); err != nil {
		return nil, fmt.Errorf("failed to save account to store: %w", err)
	}

	// Initialize plugin with our custom store
	weixinPlugin, err := wechat.NewWechatBotWithStore(wcConfig, store)
	if err != nil {
		return nil, fmt.Errorf("failed to create weixin bot: %w", err)
	}

	// Load account from store
	if err := weixinPlugin.LoadAccount(accountID); err != nil {
		return nil, fmt.Errorf("failed to load account: %w", err)
	}

	// Get the account from plugin
	account := weixinPlugin.Account()

	bot := &Bot{
		BaseBot:   core.NewBaseBot(config),
		WechatBot: weixinPlugin,
		accountID: accountID,
		account:   account,
	}

	// Set platform info
	// Platform info is set in base bot via config.Platform

	return bot, nil
}

// Connect connects to Weixin
func (b *Bot) Connect(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Account is already set in NewBot, just validate it
	if b.account == nil {
		return core.NewAuthFailedError(core.PlatformWeixin, "account not initialized", nil)
	}

	// Check if account is configured
	if !b.account.IsConfigured() {
		return core.NewAuthFailedError(core.PlatformWeixin, "account not configured, please pair first", nil)
	}

	// Check if account is enabled
	if !b.account.IsEnabled() {
		return fmt.Errorf("account is disabled")
	}

	// Initialize adapter for message conversion
	wcAccount := b.account.WeChatAccount()
	b.adapter = NewAdapter(b.BaseBot.Config(), wcAccount)

	// Mark as connected
	b.UpdateConnected(true)
	b.UpdateAuthenticated(true)
	b.EmitConnected()
	b.Logger().Info("Weixin bot connected: account=%s", b.account.ID())

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

	// Disconnect from plugin
	_ = b.WechatBot.Disconnect()

	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()
	b.Logger().Info("Weixin bot disconnected")

	return nil
}

func (b *Bot) IsConnected() bool {
	return b.account != nil
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
	if opts.Text != "" && len(opts.Media) == 0 {
		return b.sendText(ctx, target, opts)
	}

	// Handle media (with optional caption)
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
func (b *Bot) GetAccount() *types.WeChatAccount {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.account == nil {
		return nil
	}
	return b.account.WeChatAccount()
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

// getContextToken gets the context token for a reply
func (b *Bot) getContextToken(target string, metadata map[string]interface{}) string {
	if metadata != nil {
		if ct, ok := metadata["context_token"].(string); ok && ct != "" {
			return ct
		}
	}
	// For new SDK, context token is managed internally
	// Return empty string to let SDK handle it
	return ""
}

// sendText sends a text message using WechatBot.Send()
func (b *Bot) sendText(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	// Validate text length
	if err := b.ValidateTextLength(opts.Text); err != nil {
		return nil, err
	}

	// Build outbound message
	msg := &types.OutboundMessage{
		To:           target,
		Text:         opts.Text,
		ContextToken: b.getContextToken(target, opts.Metadata),
	}

	// Send via WechatBot
	result, err := b.Send(ctx, msg)
	if err != nil {
		return nil, core.WrapError(err, core.PlatformWeixin, core.ErrPlatformError)
	}

	if !result.OK {
		return nil, core.NewBotError(core.ErrPlatformError, result.Error, false)
	}

	b.UpdateLastActivity()
	now := time.Now().Unix()
	return &core.SendResult{
		MessageID: result.MessageID,
		Timestamp: now,
	}, nil
}

// sendMedia sends media messages using WechatBot.SendMedia()
func (b *Bot) sendMedia(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if len(opts.Media) == 0 {
		return nil, core.NewBotError(core.ErrUnknown, "no media to send", false)
	}

	// Process the first media item
	mediaItem := opts.Media[0]

	// Get local file path from URL
	filePath := mediaItem.URL
	if filePath == "" {
		return nil, core.NewBotError(core.ErrMediaNotSupported, "media URL is required", false)
	}

	// Check if file exists locally, if not, try to download it
	localPath := filePath
	cleanupNeeded := false
	if _, err := os.Stat(filePath); err != nil {
		// File doesn't exist, try to download from URL
		tempDir := filepath.Join(os.TempDir(), "tingly-box-weixin")
		downloadedPath, err := media.DownloadRemoteMediaToTemp(ctx, filePath, tempDir)
		if err != nil {
			return nil, core.WrapError(err, core.PlatformWeixin, core.ErrMediaNotSupported)
		}
		localPath = downloadedPath
		cleanupNeeded = true
	}

	// Determine content type
	contentType := mediaItem.Type
	if contentType == "" {
		// Try to detect from file extension
		switch {
		case isImageFile(localPath):
			contentType = "image"
		case isVideoFile(localPath):
			contentType = "video"
		case isAudioFile(localPath):
			contentType = "audio"
		default:
			contentType = "file"
		}
	}

	// Get filename
	fileName := mediaItem.Filename
	if fileName == "" {
		fileName = filepath.Base(localPath)
	}

	// Build outbound message with media
	msg := &types.OutboundMessage{
		To:           target,
		Text:         opts.Text,
		FilePath:     localPath,
		FileName:     fileName,
		ContentType:  contentType,
		ContextToken: b.getContextToken(target, opts.Metadata),
	}

	// Send via WechatBot.SendMedia
	result, err := b.WechatBot.SendMedia(ctx, msg)
	if err != nil {
		if cleanupNeeded {
			_ = os.Remove(localPath)
		}
		return nil, core.WrapError(err, core.PlatformWeixin, core.ErrMediaNotSupported)
	}

	// Clean up temp file
	if cleanupNeeded {
		_ = os.Remove(localPath)
	}

	if !result.OK {
		return nil, core.NewBotError(core.ErrPlatformError, result.Error, false)
	}

	b.UpdateLastActivity()
	now := time.Now().Unix()
	return &core.SendResult{
		MessageID: result.MessageID,
		Timestamp: now,
	}, nil
}

// Helper functions to detect media type from file extension
func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp"
}

func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".mp4" || ext == ".mov" || ext == ".avi" || ext == ".mkv" || ext == ".webm"
}

func isAudioFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".mp3" || ext == ".wav" || ext == ".m4a" || ext == ".aac" || ext == ".ogg"
}

// receiveMessages receives messages via long-polling
func (b *Bot) receiveMessages() {
	defer b.wg.Done()

	// Mark as ready
	b.UpdateReady(true)
	b.EmitReady()
	b.Logger().Info("Weixin bot ready: account=%s", b.accountID)

	var syncBuf string

	for {
		select {
		case <-b.ctx.Done():
			return
		default:
			// Fetch updates using new SDK API
			result, err := b.GetUpdates(b.ctx, syncBuf)
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

			b.Logger().Debug("GetUpdates result: ErrCode=%d, Messages=%d", result.ErrCode, len(result.Messages))

			// Check for session expiration
			if result.ErrCode == -14 { // SessionExpiredErrCode
				b.Logger().Error("Weixin session expired, need to re-authenticate")
				// Emit session expired event
				b.EmitError(core.NewAuthFailedError(core.PlatformWeixin, "session expired", nil))
				return
			}

			// Update sync buffer for next request
			syncBuf = result.SyncBuf

			// Process messages
			if len(result.Messages) > 0 {
				b.Logger().Info("Processing %d messages from Weixin", len(result.Messages))
			}
			for _, msg := range result.Messages {
				b.Logger().Info("Handling message: ID=%s, From=%s, To=%s, Text=%s", msg.MessageID, msg.From, msg.To, msg.Text)
				b.handleMessage(msg)
			}

			// Use long-polling timeout if provided
			if result.LongPollingTimeout > 0 {
				select {
				case <-time.After(time.Duration(result.LongPollingTimeout) * time.Millisecond):
				case <-b.ctx.Done():
					return
				}
			}
		}
	}
}

// handleMessage processes an incoming message
func (b *Bot) handleMessage(msg *types.Message) {
	if msg == nil {
		return
	}

	// Use adapter to convert types message to core message
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
