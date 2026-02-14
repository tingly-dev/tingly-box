package core

import (
	"context"
	"time"
)

// Bot represents the interface for all messaging platform bots
type Bot interface {
	// Lifecycle
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	IsConnected() bool

	// Messaging
	SendMessage(ctx context.Context, target string, opts *SendMessageOptions) (*SendResult, error)
	SendText(ctx context.Context, target string, text string) (*SendResult, error)
	SendMedia(ctx context.Context, target string, media []MediaAttachment) (*SendResult, error)

	// Actions
	React(ctx context.Context, messageID string, emoji string) error
	EditMessage(ctx context.Context, messageID string, text string) error
	DeleteMessage(ctx context.Context, messageID string) error

	// State
	Status() *BotStatus
	PlatformInfo() *PlatformInfo

	// Events
	OnMessage(handler func(Message))
	OnError(handler func(error))
	OnConnected(handler func())
	OnDisconnected(handler func())
	OnReady(handler func())

	// Cleanup
	Close() error
}

// SendMessageOptions represents options for sending a message
type SendMessageOptions struct {
	Text      string                 `json:"text,omitempty"`
	Media     []MediaAttachment      `json:"media,omitempty"`
	ReplyTo   string                 `json:"replyTo,omitempty"`
	ThreadID  string                 `json:"threadId,omitempty"`
	ParseMode ParseMode              `json:"parseMode,omitempty"`
	Silent    bool                   `json:"silent,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// SendResult represents the result of sending a message
type SendResult struct {
	MessageID string                 `json:"messageId"`
	Timestamp int64                  `json:"timestamp"`
	Raw       map[string]interface{} `json:"raw,omitempty"`
}

// BotStatus represents the current status of a bot
type BotStatus struct {
	Connected     bool               `json:"connected"`
	Authenticated bool               `json:"authenticated"`
	Ready         bool               `json:"ready"`
	LastActivity  int64              `json:"lastActivity,omitempty"`
	Error         string             `json:"error,omitempty"`
	Connection    *ConnectionDetails `json:"connection,omitempty"`
}

// IsHealthy returns true if the bot is in a healthy state
func (s *BotStatus) IsHealthy() bool {
	return s.Connected && s.Authenticated && s.Ready && s.Error == ""
}

// LastActivityTime returns the last activity as a time.Time
func (s *BotStatus) LastActivityTime() time.Time {
	if s.LastActivity == 0 {
		return time.Time{}
	}
	return time.Unix(s.LastActivity, 0)
}

// PlatformInfo represents information about a platform
type PlatformInfo struct {
	ID           Platform              `json:"id"`
	Name         string                `json:"name"`
	Capabilities *PlatformCapabilities `json:"capabilities"`
}

// NewPlatformInfo creates a new PlatformInfo
func NewPlatformInfo(platform Platform, name string) *PlatformInfo {
	return &PlatformInfo{
		ID:           platform,
		Name:         name,
		Capabilities: GetPlatformCapabilities(platform),
	}
}

// PlatformNames returns the human-readable name for each platform
var PlatformNames = map[Platform]string{
	PlatformWhatsApp:    "WhatsApp",
	PlatformTelegram:    "Telegram",
	PlatformDiscord:     "Discord",
	PlatformSlack:       "Slack",
	PlatformGoogleChat:  "Google Chat",
	PlatformSignal:      "Signal",
	PlatformBlueBubbles: "BlueBubbles (iMessage)",
	PlatformFeishu:      "Feishu/Lark",
	PlatformWebChat:     "WebChat",
	PlatformDingTalk:    "DingTalk",
}

// GetPlatformName returns the human-readable name for a platform
func GetPlatformName(platform Platform) string {
	if name, ok := PlatformNames[platform]; ok {
		return name
	}
	return string(platform)
}

// IsValidPlatform checks if a platform string is valid
func IsValidPlatform(platform string) bool {
	switch Platform(platform) {
	case PlatformWhatsApp, PlatformTelegram, PlatformDiscord,
		PlatformSlack, PlatformGoogleChat, PlatformSignal,
		PlatformBlueBubbles, PlatformFeishu, PlatformWebChat,
		PlatformDingTalk:
		return true
	default:
		return false
	}
}
