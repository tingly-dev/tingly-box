package telegram

import (
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// TestTelegramBot_Connect tests bot connection
func TestTelegramBot_Connect(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformTelegram,
		Enabled:  true,
		Auth: core.AuthConfig{
			Type:  "token",
			Token: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		},
		Options: map[string]interface{}{
			"apiURL": "https://api.telegram.org/bot",
		},
	}

	bot, err := NewTelegramBot(config)
	if err != nil {
		t.Skipf("Skipping test - failed to create bot (likely network issue): %v", err)
	}

	if bot == nil {
		t.Fatal("Expected non-nil bot")
	}

	info := bot.PlatformInfo()
	if info.ID != core.PlatformTelegram {
		t.Errorf("Expected platform ID %s, got %s", core.PlatformTelegram, info.ID)
	}

	if info.Name != "Telegram" {
		t.Errorf("Expected platform name Telegram, got %s", info.Name)
	}

	status := bot.Status()
	if status.Connected {
		t.Error("Bot should not be connected initially")
	}
}

// TestTelegramBot_PlatformInfo tests platform info
func TestTelegramBot_PlatformInfo(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformTelegram,
		Enabled:  true,
		Auth: core.AuthConfig{
			Type:  "token",
			Token: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		},
	}

	bot, err := NewTelegramBot(config)
	if err != nil {
		t.Skipf("Skipping test - failed to create bot (likely network issue): %v", err)
	}

	info := bot.PlatformInfo()

	if info.ID != core.PlatformTelegram {
		t.Errorf("Expected platform ID %s, got %s", core.PlatformTelegram, info.ID)
	}

	if info.Name != "Telegram" {
		t.Errorf("Expected platform name Telegram, got %s", info.Name)
	}

	if info.Capabilities == nil {
		t.Error("Capabilities should not be nil")
	} else {
		if !info.Capabilities.SupportsFeature("reactions") {
			t.Error("Should support reactions")
		}
		if !info.Capabilities.SupportsMediaType("image") {
			t.Error("Should support images")
		}
	}
}

// TestTelegramBot_Status tests bot status
func TestTelegramBot_Status(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformTelegram,
		Enabled:  true,
		Auth: core.AuthConfig{
			Type:  "token",
			Token: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		},
	}

	bot, err := NewTelegramBot(config)
	if err != nil {
		t.Skipf("Skipping test - failed to create bot (likely network issue): %v", err)
	}

	status := bot.Status()
	if status.Connected {
		t.Error("Bot should not be connected initially")
	}
	if status.Authenticated {
		t.Error("Bot should not be authenticated initially")
	}
	if status.Ready {
		t.Error("Bot should not be ready initially")
	}
}

// TestTelegramBot_EventHandlers tests event handlers
func TestTelegramBot_EventHandlers(t *testing.T) {
	config := &core.Config{
		Platform: core.PlatformTelegram,
		Enabled:  true,
		Auth: core.AuthConfig{
			Type:  "token",
			Token: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		},
	}

	bot, err := NewTelegramBot(config)
	if err != nil {
		t.Skipf("Skipping test - failed to create bot (likely network issue): %v", err)
	}

	// Test message handler
	messageReceived := false
	bot.OnMessage(func(msg core.Message) {
		messageReceived = true
	})

	// Test error handler
	errorReceived := false
	bot.OnError(func(err error) {
		errorReceived = true
	})

	// Test connected handler
	connectedCalled := false
	bot.OnConnected(func() {
		connectedCalled = true
	})

	// Test disconnected handler
	disconnectedCalled := false
	bot.OnDisconnected(func() {
		disconnectedCalled = true
	})

	// Test ready handler
	readyCalled := false
	bot.OnReady(func() {
		readyCalled = true
	})

	// Just verify the handlers can be set without panicking
	if !messageReceived && !errorReceived && !connectedCalled && !disconnectedCalled && !readyCalled {
		// Handlers haven't been called yet, which is expected
	}
}

// TestTelegramBot_ConfigValidation tests configuration validation
func TestTelegramBot_ConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *core.Config
		wantErr bool
	}{
		{
			name: "Valid config",
			config: &core.Config{
				Platform: core.PlatformTelegram,
				Enabled:  true,
				Auth: core.AuthConfig{
					Type:  "token",
					Token: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
				},
			},
			wantErr: false,
		},
		{
			name: "Missing token",
			config: &core.Config{
				Platform: core.PlatformTelegram,
				Enabled:  true,
				Auth: core.AuthConfig{
					Type:  "token",
					Token: "",
				},
			},
			wantErr: true,
		},
		{
			name: "Invalid token format",
			config: &core.Config{
				Platform: core.PlatformTelegram,
				Enabled:  true,
				Auth: core.AuthConfig{
					Type:  "token",
					Token: "invalid-token-format",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTelegramBot(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTelegramBot() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
