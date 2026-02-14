package core

import "time"

// Platform represents the supported messaging platforms
type Platform string

const (
	PlatformWhatsApp    Platform = "whatsapp"
	PlatformTelegram    Platform = "telegram"
	PlatformDiscord     Platform = "discord"
	PlatformSlack       Platform = "slack"
	PlatformGoogleChat  Platform = "googlechat"
	PlatformSignal      Platform = "signal"
	PlatformBlueBubbles Platform = "bluebubbles"
	PlatformFeishu      Platform = "feishu"
	PlatformWebChat     Platform = "webchat"
	PlatformDingTalk    Platform = "dingtalk"
)

// ChatType represents the type of chat
type ChatType string

const (
	ChatTypeDirect  ChatType = "direct"
	ChatTypeGroup   ChatType = "group"
	ChatTypeChannel ChatType = "channel"
	ChatTypeThread  ChatType = "thread"
)

// ParseMode represents the parse mode for formatted text
type ParseMode string

const (
	ParseModeMarkdown ParseMode = "markdown"
	ParseModeHTML     ParseMode = "html"
	ParseModeNone     ParseMode = "none"
)

// ErrorCode represents error codes
type ErrorCode string

const (
	ErrAuthFailed        ErrorCode = "AUTH_FAILED"
	ErrConnectionFailed  ErrorCode = "CONNECTION_FAILED"
	ErrRateLimited       ErrorCode = "RATE_LIMITED"
	ErrMessageTooLong    ErrorCode = "MESSAGE_TOO_LONG"
	ErrInvalidTarget     ErrorCode = "INVALID_TARGET"
	ErrMediaNotSupported ErrorCode = "MEDIA_NOT_SUPPORTED"
	ErrPlatformError     ErrorCode = "PLATFORM_ERROR"
	ErrTimeout           ErrorCode = "TIMEOUT"
	ErrUnknown           ErrorCode = "UNKNOWN"
)

// Sender represents the message sender
type Sender struct {
	ID          string                 `json:"id"`
	Username    string                 `json:"username,omitempty"`
	DisplayName string                 `json:"displayName,omitempty"`
	Avatar      string                 `json:"avatar,omitempty"`
	Raw         map[string]interface{} `json:"raw,omitempty"`
}

// Recipient represents the message recipient
type Recipient struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // "user", "group", "channel"
	DisplayName string `json:"displayName,omitempty"`
}

// ThreadContext represents thread context for threaded messages
type ThreadContext struct {
	ID              string `json:"id"`
	Title           string `json:"title,omitempty"`
	ParentMessageID string `json:"parentMessageId,omitempty"`
}

// Entity represents a message entity (mention, URL, etc.)
type Entity struct {
	Type   string                 `json:"type"` // "mention", "hashtag", "url", "bold", "italic", "code"
	Offset int                    `json:"offset"`
	Length int                    `json:"length"`
	Data   map[string]interface{} `json:"data,omitempty"`
}

// ConnectionDetails represents connection details
type ConnectionDetails struct {
	Mode              ConnectionMode `json:"mode"`
	URL               string         `json:"url,omitempty"`
	ReconnectAttempts int            `json:"reconnectAttempts,omitempty"`
	ConnectedAt       int64          `json:"connectedAt,omitempty"`
}

// ConnectionMode represents the connection mode
type ConnectionMode string

const (
	ConnectionModePolling   ConnectionMode = "polling"
	ConnectionModeWebSocket ConnectionMode = "websocket"
	ConnectionModeWebhook   ConnectionMode = "webhook"
	ConnectionModeGateway   ConnectionMode = "gateway"
)

// Time returns the connection time as a time.Time
func (c *ConnectionDetails) Time() time.Time {
	if c.ConnectedAt == 0 {
		return time.Time{}
	}
	return time.Unix(c.ConnectedAt, 0)
}

// PlatformCapabilities represents platform capabilities
type PlatformCapabilities struct {
	ChatTypes  []ChatType `json:"chatTypes"`
	MediaTypes []string   `json:"mediaTypes,omitempty"`
	Features   []string   `json:"features"`
	TextLimit  int        `json:"textLimit,omitempty"`
	RateLimit  int        `json:"rateLimit,omitempty"`
}

// SupportsFeature checks if the platform supports a specific feature
func (p *PlatformCapabilities) SupportsFeature(feature string) bool {
	for _, f := range p.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// SupportsMediaType checks if the platform supports a specific media type
func (p *PlatformCapabilities) SupportsMediaType(mediaType string) bool {
	for _, mt := range p.MediaTypes {
		if mt == mediaType {
			return true
		}
	}
	return false
}

// SupportsChatType checks if the platform supports a specific chat type
func (p *PlatformCapabilities) SupportsChatType(chatType ChatType) bool {
	for _, ct := range p.ChatTypes {
		if ct == chatType {
			return true
		}
	}
	return false
}

// GetPlatformCapabilities returns the capabilities for a given platform
func GetPlatformCapabilities(platform Platform) *PlatformCapabilities {
	capabilities := map[Platform]*PlatformCapabilities{
		PlatformWhatsApp: {
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes: []string{"image", "video", "audio", "document", "sticker"},
			Features:   []string{"reactions", "edit", "delete", "readReceipts", "typingIndicator"},
			TextLimit:  4096,
			RateLimit:  60,
		},
		PlatformTelegram: {
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes: []string{"image", "video", "audio", "document", "sticker", "gif"},
			Features:   []string{"reactions", "edit", "delete", "threads", "polls", "nativeCommands"},
			TextLimit:  4096,
			RateLimit:  30,
		},
		PlatformDiscord: {
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes: []string{"image", "video", "audio", "document", "gif"},
			Features:   []string{"reactions", "edit", "delete", "threads", "nativeCommands", "mentions"},
			TextLimit:  2000,
			RateLimit:  50,
		},
		PlatformSlack: {
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes: []string{"image", "video", "audio", "document"},
			Features:   []string{"reactions", "edit", "delete", "threads", "mentions"},
			TextLimit:  40000,
			RateLimit:  60,
		},
		PlatformGoogleChat: {
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeThread},
			MediaTypes: []string{"image", "video"},
			Features:   []string{"reactions", "delete", "threads"},
			TextLimit:  4000,
			RateLimit:  30,
		},
		PlatformSignal: {
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes: []string{"image", "video", "audio", "document"},
			Features:   []string{"reactions", "delete", "readReceipts", "typingIndicator"},
			TextLimit:  4096,
			RateLimit:  60,
		},
		PlatformBlueBubbles: {
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes: []string{"image", "video", "audio", "document"},
			Features:   []string{"reactions", "edit", "delete", "readReceipts", "typingIndicator"},
			TextLimit:  4000,
			RateLimit:  60,
		},
		PlatformFeishu: {
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes: []string{"image", "video", "audio", "document"},
			Features:   []string{"reactions", "delete", "threads", "nativeCommands", "mentions"},
			TextLimit:  4000,
			RateLimit:  50,
		},
		PlatformWebChat: {
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes: []string{"image", "video", "audio", "document", "sticker"},
			Features:   []string{"reactions", "edit", "delete", "threads", "polls"},
			TextLimit:  4096,
			RateLimit:  60,
		},
		PlatformDingTalk: {
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes: []string{"image", "video", "audio", "document"},
			Features:   []string{"reactions", "delete", "threads"},
			TextLimit:  4000,
			RateLimit:  50,
		},
	}

	if caps, ok := capabilities[platform]; ok {
		return caps
	}

	return &PlatformCapabilities{
		ChatTypes: []ChatType{ChatTypeDirect},
		Features:  []string{},
	}
}
