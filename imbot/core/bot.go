package core

import (
	"context"
	"time"
)

// Bot represents the interface for all messaging platform bots
type Bot interface {
	// Identity
	UUID() string

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

	// Text Processing
	// ChunkText splits text into chunks based on the platform's message limit.
	// Uses smart break-point detection to avoid breaking words in the middle.
	ChunkText(text string) []string
	// ValidateTextLength checks if text is within the platform's message limit.
	ValidateTextLength(text string) error
	// GetMessageLimit returns the message length limit for this bot's platform.
	GetMessageLimit() int

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
	Entities  []Entity               `json:"entities,omitempty"` // Telegram message entities (alternative to ParseMode)
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

// NewPlatformInfo creates a new PlatformInfo with an explicit display name.
// Prefer NewPlatformInfoFor unless the display name is not derivable from the
// platform id (e.g. the Feishu/Lark domain split).
func NewPlatformInfo(platform Platform, name string) *PlatformInfo {
	return &PlatformInfo{
		ID:           platform,
		Name:         name,
		Capabilities: GetPlatformCapabilities(platform),
	}
}

// NewPlatformInfoFor creates a PlatformInfo whose display name is derived from
// the single PlatformNames table, so bots don't hardcode name literals.
func NewPlatformInfoFor(platform Platform) *PlatformInfo {
	return NewPlatformInfo(platform, GetPlatformName(platform))
}

// PlatformNames, GetPlatformName and IsValidPlatform live in platforms.go,
// backed by the single platform-descriptor table.
