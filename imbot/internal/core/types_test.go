package core

import (
	"testing"
)

func TestPlatformConstants(t *testing.T) {
	tests := []struct {
		name     string
		platform Platform
		want     string
	}{
		{"WhatsApp", PlatformWhatsApp, "whatsapp"},
		{"Telegram", PlatformTelegram, "telegram"},
		{"Discord", PlatformDiscord, "discord"},
		{"Slack", PlatformSlack, "slack"},
		{"GoogleChat", PlatformGoogleChat, "googlechat"},
		{"Signal", PlatformSignal, "signal"},
		{"BlueBubbles", PlatformBlueBubbles, "bluebubbles"},
		{"Feishu", PlatformFeishu, "feishu"},
		{"WebChat", PlatformWebChat, "webchat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(tt.platform); got != tt.want {
				t.Errorf("Platform = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChatTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		chatType ChatType
		want     string
	}{
		{"Direct", ChatTypeDirect, "direct"},
		{"Group", ChatTypeGroup, "group"},
		{"Channel", ChatTypeChannel, "channel"},
		{"Thread", ChatTypeThread, "thread"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(tt.chatType); got != tt.want {
				t.Errorf("ChatType = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseModeConstants(t *testing.T) {
	tests := []struct {
		name      string
		parseMode ParseMode
		want      string
	}{
		{"Markdown", ParseModeMarkdown, "markdown"},
		{"HTML", ParseModeHTML, "html"},
		{"None", ParseModeNone, "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(tt.parseMode); got != tt.want {
				t.Errorf("ParseMode = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorCodeConstants(t *testing.T) {
	tests := []struct {
		name string
		code ErrorCode
		want string
	}{
		{"AuthFailed", ErrAuthFailed, "AUTH_FAILED"},
		{"ConnectionFailed", ErrConnectionFailed, "CONNECTION_FAILED"},
		{"RateLimited", ErrRateLimited, "RATE_LIMITED"},
		{"MessageTooLong", ErrMessageTooLong, "MESSAGE_TOO_LONG"},
		{"InvalidTarget", ErrInvalidTarget, "INVALID_TARGET"},
		{"MediaNotSupported", ErrMediaNotSupported, "MEDIA_NOT_SUPPORTED"},
		{"PlatformError", ErrPlatformError, "PLATFORM_ERROR"},
		{"Timeout", ErrTimeout, "TIMEOUT"},
		{"Unknown", ErrUnknown, "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(tt.code); got != tt.want {
				t.Errorf("ErrorCode = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsValidPlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     bool
	}{
		{"Valid WhatsApp", "whatsapp", true},
		{"Valid Telegram", "telegram", true},
		{"Valid Discord", "discord", true},
		{"Valid Slack", "slack", true},
		{"Invalid", "invalid", false},
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidPlatform(tt.platform); got != tt.want {
				t.Errorf("IsValidPlatform(%v) = %v, want %v", tt.platform, got, tt.want)
			}
		})
	}
}

func TestGetPlatformName(t *testing.T) {
	tests := []struct {
		name     string
		platform Platform
		want     string
	}{
		{"WhatsApp", PlatformWhatsApp, "WhatsApp"},
		{"Telegram", PlatformTelegram, "Telegram"},
		{"Discord", PlatformDiscord, "Discord"},
		{"Slack", PlatformSlack, "Slack"},
		{"Google Chat", PlatformGoogleChat, "Google Chat"},
		{"Signal", PlatformSignal, "Signal"},
		{"BlueBubbles", PlatformBlueBubbles, "BlueBubbles (iMessage)"},
		{"Feishu", PlatformFeishu, "Feishu/Lark"},
		{"WebChat", PlatformWebChat, "WebChat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetPlatformName(tt.platform); got != tt.want {
				t.Errorf("GetPlatformName(%v) = %v, want %v", tt.platform, got, tt.want)
			}
		})
	}
}

func TestNewPlatformInfo(t *testing.T) {
	platform := PlatformTelegram
	name := "Telegram Bot"

	info := NewPlatformInfo(platform, name)

	if info.ID != platform {
		t.Errorf("ID = %v, want %v", info.ID, platform)
	}

	if info.Name != name {
		t.Errorf("Name = %v, want %v", info.Name, name)
	}

	if info.Capabilities == nil {
		t.Error("Capabilities should not be nil")
	}
}

func TestPlatformCapabilities(t *testing.T) {
	tests := []struct {
		name                  string
		platform              Platform
		wantChatTypes         int
		wantFeatures          int
		wantTextLimit         int
		wantSupportsReactions bool
	}{
		{
			name:                  "Telegram",
			platform:              PlatformTelegram,
			wantChatTypes:         4,
			wantFeatures:          6,
			wantTextLimit:         4096,
			wantSupportsReactions: true,
		},
		{
			name:                  "Discord",
			platform:              PlatformDiscord,
			wantChatTypes:         4,
			wantFeatures:          6,
			wantTextLimit:         2000,
			wantSupportsReactions: true,
		},
		{
			name:                  "Slack",
			platform:              PlatformSlack,
			wantChatTypes:         4,
			wantFeatures:          5,
			wantTextLimit:         40000,
			wantSupportsReactions: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := GetPlatformCapabilities(tt.platform)

			if len(caps.ChatTypes) != tt.wantChatTypes {
				t.Errorf("ChatTypes length = %v, want %v", len(caps.ChatTypes), tt.wantChatTypes)
			}

			if len(caps.Features) != tt.wantFeatures {
				t.Errorf("Features length = %v, want %v", len(caps.Features), tt.wantFeatures)
			}

			if caps.TextLimit != tt.wantTextLimit {
				t.Errorf("TextLimit = %v, want %v", caps.TextLimit, tt.wantTextLimit)
			}

			if got := caps.SupportsFeature("reactions"); got != tt.wantSupportsReactions {
				t.Errorf("SupportsFeature(reactions) = %v, want %v", got, tt.wantSupportsReactions)
			}
		})
	}
}

func TestBotStatus_IsHealthy(t *testing.T) {
	tests := []struct {
		name   string
		status BotStatus
		want   bool
	}{
		{
			name: "All healthy",
			status: BotStatus{
				Connected:     true,
				Authenticated: true,
				Ready:         true,
				Error:         "",
			},
			want: true,
		},
		{
			name: "Not connected",
			status: BotStatus{
				Connected:     false,
				Authenticated: true,
				Ready:         true,
			},
			want: false,
		},
		{
			name: "Has error",
			status: BotStatus{
				Connected:     true,
				Authenticated: true,
				Ready:         true,
				Error:         "connection error",
			},
			want: false,
		},
		{
			name: "Not ready",
			status: BotStatus{
				Connected:     true,
				Authenticated: true,
				Ready:         false,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsHealthy(); got != tt.want {
				t.Errorf("BotStatus.IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}
