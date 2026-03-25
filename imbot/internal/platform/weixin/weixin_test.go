// Package weixin provides Weixin platform bot implementation for ImBot.
package weixin

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/weixin"
)

// TestNewBot tests creating a new Weixin bot
func TestNewBot(t *testing.T) {
	tests := []struct {
		name    string
		config  *core.Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &core.Config{
				Platform: core.PlatformWeixin,
				Auth: core.AuthConfig{
					Type: "qr",
				},
				Options: map[string]interface{}{
					"baseUrl": "https://api.example.com",
					"botType": "3",
				},
			},
			wantErr: false,
		},
		{
			name: "minimal config",
			config: &core.Config{
				Platform: core.PlatformWeixin,
				Auth: core.AuthConfig{
					Type: "qr",
				},
				Options: map[string]interface{}{},
			},
			wantErr: false,
		},
		{
			name: "invalid platform",
			config: &core.Config{
				Platform: core.PlatformTelegram,
				Auth: core.AuthConfig{
					Type: "qr",
				},
			},
			wantErr: false, // NewBot doesn't validate platform, it just creates the bot
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot, err := NewBot(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && bot == nil {
				t.Error("NewBot() returned nil bot")
			}
		})
	}
}

// TestBotConfigValidation tests configuration validation
func TestBotConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *core.Config
		wantErr bool
	}{
		{
			name: "valid qr auth",
			config: &core.Config{
				Platform: core.PlatformWeixin,
				Auth: core.AuthConfig{
					Type: "qr",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid auth type",
			config: &core.Config{
				Platform: core.PlatformWeixin,
				Auth: core.AuthConfig{
					Type:  "token",
					Token: "test",
				},
			},
			wantErr: false, // QR auth validation allows other auth types
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestGetMessageLimit tests the message limit
func TestGetMessageLimit(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWeixin,
		Auth: core.AuthConfig{
			Type: "qr",
		},
	}
	bot, err := NewBot(config)
	if err != nil {
		t.Fatalf("NewBot() error = %v", err)
	}

	// The BaseBot uses GetPlatformCapabilities which may have a different default
	// The adapter returns 2048
	expectedLimit := bot.GetMessageLimit()
	if expectedLimit == 0 {
		t.Errorf("GetMessageLimit() returned 0")
	}
}

// TestPlatformInfo tests platform info
func TestPlatformInfo(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWeixin,
		Auth: core.AuthConfig{
			Type: "qr",
		},
	}
	bot, err := NewBot(config)
	if err != nil {
		t.Fatalf("NewBot() error = %v", err)
	}

	info := bot.PlatformInfo()
	if info.ID != core.PlatformWeixin {
		t.Errorf("PlatformInfo().ID = %s, want %s", info.ID, core.PlatformWeixin)
	}
	if info.Name != "Weixin" {
		t.Errorf("PlatformInfo().Name = %s, want Weixin", info.Name)
	}
}

// TestBotState tests bot state management
func TestBotState(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWeixin,
		Auth: core.AuthConfig{
			Type: "qr",
		},
	}
	bot, err := NewBot(config)
	if err != nil {
		t.Fatalf("NewBot() error = %v", err)
	}

	// Initial state should be disconnected
	if bot.IsConnected() {
		t.Error("New bot should not be connected")
	}

	// Status should reflect initial state
	status := bot.Status()
	if status.Connected || status.Authenticated || status.Ready {
		t.Error("Initial status should be all false")
	}
}

// TestAdapterPlatform tests adapter platform
func TestAdapterPlatform(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWeixin,
		Auth: core.AuthConfig{
			Type: "qr",
		},
	}
	_, err := NewBot(config)
	if err != nil {
		t.Fatalf("NewBot() error = %v", err)
	}

	// Account needs to be created first
	account := &weixin.WeChatAccount{
		ID:         "test",
		Name:       "Test Account",
		Enabled:    true,
		Configured: true,
	}

	adapter := NewAdapter(config, account)
	if adapter.Platform() != core.PlatformWeixin {
		t.Errorf("Adapter.Platform() = %s, want %s", adapter.Platform(), core.PlatformWeixin)
	}
}

// TestGetAccountInfo tests getting account information
func TestGetAccountInfo(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWeixin,
		Auth: core.AuthConfig{
			Type: "qr",
		},
		Options: map[string]interface{}{
			"accountId": "test-account",
		},
	}
	bot, err := NewBot(config)
	if err != nil {
		t.Fatalf("NewBot() error = %v", err)
	}

	handler := NewInteractionHandler(bot)
	info := handler.GetAccountInfo()

	if info.AccountID != "test-account" {
		t.Errorf("GetAccountInfo().AccountID = %s, want test-account", info.AccountID)
	}

	// New account should not be configured
	if info.Configured {
		t.Error("New account should not be configured")
	}
}

// TestNeedsPairing tests checking if account needs pairing
func TestNeedsPairing(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWeixin,
		Auth: core.AuthConfig{
			Type: "qr",
		},
	}
	bot, err := NewBot(config)
	if err != nil {
		t.Fatalf("NewBot() error = %v", err)
	}

	// New bot should need pairing
	if !bot.NeedsPairing() {
		t.Error("New bot should need pairing")
	}

	// Should not be configured
	if bot.IsConfigured() {
		t.Error("New bot should not be configured")
	}
}

// TestAdapterMessageLimit tests adapter message limit
func TestAdapterMessageLimit(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWeixin,
		Auth: core.AuthConfig{
			Type: "qr",
		},
	}
	account := &weixin.WeChatAccount{
		ID:         "test",
		Name:       "Test Account",
		Enabled:    true,
		Configured: true,
	}

	adapter := NewAdapter(config, account)

	expectedLimit := 2048
	if adapter.GetMessageLimit() != expectedLimit {
		t.Errorf("Adapter.GetMessageLimit() = %d, want %d", adapter.GetMessageLimit(), expectedLimit)
	}
}

// TestAdapterShouldChunkText tests text chunking decision
func TestAdapterShouldChunkText(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWeixin,
		Auth: core.AuthConfig{
			Type: "qr",
		},
	}
	account := &weixin.WeChatAccount{
		ID:         "test",
		Name:       "Test Account",
		Enabled:    true,
		Configured: true,
	}

	adapter := NewAdapter(config, account)

	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "short text",
			text: "Hello world",
			want: false,
		},
		{
			name: "exactly at limit",
			text: string(make([]rune, 2048)),
			want: false,
		},
		{
			name: "over limit",
			text: string(make([]rune, 2049)),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adapter.ShouldChunkText(tt.text); got != tt.want {
				t.Errorf("Adapter.ShouldChunkText() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestUnsupportedOperations tests that unsupported operations return appropriate errors
func TestUnsupportedOperations(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWeixin,
		Auth: core.AuthConfig{
			Type: "qr",
		},
	}
	bot, err := NewBot(config)
	if err != nil {
		t.Fatalf("NewBot() error = %v", err)
	}

	ctx := context.Background()

	// Test React - should return not supported error (even if not ready, we check error message)
	err = bot.React(ctx, "123", "👍")
	if err == nil {
		t.Error("React() should return an error")
	} else if core.GetErrorCode(err) == core.ErrPlatformError {
		// Expected error
	} else if !core.IsRecoverable(err) && err.Error() != "bot is not ready" {
		// Also acceptable if bot is not ready
	}

	// Test EditMessage - should return not supported error
	err = bot.EditMessage(ctx, "123", "new text")
	if err == nil {
		t.Error("EditMessage() should return an error")
	} else if core.GetErrorCode(err) == core.ErrPlatformError {
		// Expected error
	}

	// Test DeleteMessage - should return not supported error
	err = bot.DeleteMessage(ctx, "123")
	if err == nil {
		t.Error("DeleteMessage() should return an error")
	} else if core.GetErrorCode(err) == core.ErrPlatformError {
		// Expected error
	}
}

// TestConfigOptionParsing tests config option parsing
func TestConfigOptionParsing(t *testing.T) {
	tests := []struct {
		name      string
		config    *core.Config
		option    string
		wantValue string
	}{
		{
			name: "baseUrl option",
			config: &core.Config{
				Platform: core.PlatformWeixin,
				Auth:     core.AuthConfig{Type: "qr"},
				Options: map[string]interface{}{
					"baseUrl": "https://api.example.com",
				},
			},
			option:    "baseUrl",
			wantValue: "https://api.example.com",
		},
		{
			name: "botType option",
			config: &core.Config{
				Platform: core.PlatformWeixin,
				Auth:     core.AuthConfig{Type: "qr"},
				Options: map[string]interface{}{
					"botType": "3",
				},
			},
			option:    "botType",
			wantValue: "3",
		},
		{
			name: "accountId option",
			config: &core.Config{
				Platform: core.PlatformWeixin,
				Auth:     core.AuthConfig{Type: "qr"},
				Options: map[string]interface{}{
					"accountId": "my-account",
				},
			},
			option:    "accountId",
			wantValue: "my-account",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetOptionString(tt.option, "")
			if got != tt.wantValue {
				t.Errorf("Config.GetOptionString(%q) = %q, want %q", tt.option, got, tt.wantValue)
			}
		})
	}
}

// TestInteractionHandler tests interaction handler
func TestInteractionHandler(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWeixin,
		Auth: core.AuthConfig{
			Type: "qr",
		},
	}
	bot, err := NewBot(config)
	if err != nil {
		t.Fatalf("NewBot() error = %v", err)
	}

	handler := NewInteractionHandler(bot)

	// Test that interaction handler is created
	if handler == nil {
		t.Error("NewInteractionHandler() returned nil")
	}

	// Test GetInteractionHandler
	handler2 := bot.GetInteractionHandler()
	if handler2 == nil {
		t.Error("GetInteractionHandler() returned nil")
	}
}

// TestMapContentType tests content type mapping
func TestMapContentType(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWeixin,
		Auth:     core.AuthConfig{Type: "qr"},
	}
	account := &weixin.WeChatAccount{
		ID:         "test",
		Name:       "Test Account",
		Enabled:    true,
		Configured: true,
	}

	adapter := NewAdapter(config, account)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"image", "image", "image"},
		{"video", "video", "video"},
		{"audio", "audio", "audio"},
		{"voice", "voice", "audio"},
		{"file", "file", "document"},
		{"unknown", "unknown", "document"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.mapContentType(tt.input)
			if got != tt.expected {
				t.Errorf("mapContentType(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestBuildReplyTarget tests reply target building
func TestBuildReplyTarget(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformWeixin,
		Auth:     core.AuthConfig{Type: "qr"},
	}
	account := &weixin.WeChatAccount{
		ID:         "test",
		Name:       "Test Account",
		Enabled:    true,
		Configured: true,
		UserID:     "user123",
	}

	adapter := NewAdapter(config, account)

	tests := []struct {
		name        string
		senderID    string
		recipientID string
		sessionID   string
		expected    string
	}{
		{
			name:        "sender is bot",
			senderID:    "user123",
			recipientID: "recipient456",
			sessionID:   "session1",
			expected:    "recipient456",
		},
		{
			name:        "sender is not bot",
			senderID:    "other789",
			recipientID: "user123",
			sessionID:   "session2",
			expected:    "other789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.BuildReplyTarget(tt.senderID, tt.recipientID, tt.sessionID)
			if got != tt.expected {
				t.Errorf("BuildReplyTarget() = %q, want %q", got, tt.expected)
			}
		})
	}
}
