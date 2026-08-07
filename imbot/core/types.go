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
	PlatformLark        Platform = "lark"
	PlatformDingTalk    Platform = "dingtalk"
	PlatformWeixin      Platform = "weixin"
	PlatformWecom       Platform = "wecom"
	PlatformTingly      Platform = "tingly"
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
	ParseModeMarkdown       ParseMode = "markdown"        // Default: MarkdownV2 (modern)
	ParseModeMarkdownLegacy ParseMode = "markdown_legacy" // Legacy: MarkdownV1 (backward compatibility)
	ParseModeHTML           ParseMode = "html"
	ParseModeNone           ParseMode = "none"
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
	// ErrNotSupported marks an operation this platform cannot perform at all,
	// as opposed to one that failed. Callers treat it as "capability absent".
	ErrNotSupported  ErrorCode = "NOT_SUPPORTED"
	ErrPlatformError ErrorCode = "PLATFORM_ERROR"
	ErrTimeout       ErrorCode = "TIMEOUT"
	ErrUnknown       ErrorCode = "UNKNOWN"
	// ErrPanic marks a contained panic in a platform receive loop. It is the
	// bot's "I crashed" signal to whoever owns its lifecycle: unlike a network
	// disconnect it must NOT be answered with an in-place reconnect — the
	// bot's state is suspect and the whole instance should be closed and
	// rebuilt (see core/safego.go and the host's fatal-panic handling).
	ErrPanic ErrorCode = "PANIC"
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

// SegmentKind classifies one ordered piece of a message.
//
// A message body is modeled as an ordered sequence of segments so that
// interleaved content roles — typically body text alternating with model
// reasoning (thinking) — can be expressed faithfully. Segmentation is
// produced upstream (the gateway / stream assembler holds the structured
// content blocks); imbot only consumes the result and renders it.
type SegmentKind string

const (
	// SegmentBody is primary message content.
	SegmentBody SegmentKind = "body"
	// SegmentThinking is model reasoning, rendered collapsed/dimmed per the
	// platform's ThinkingRender capability. Plain text only.
	SegmentThinking SegmentKind = "thinking"
	// Future kinds (quote, code, ...) are added here when actually needed.
)

// Segment is one ordered piece of a message.
//
// A body segment reuses the existing rich-format trio (Text/Entities/ParseMode),
// the same fields SendMessageOptions already carries. A thinking segment is
// plain text — its Text is rendered per the platform's ThinkingRender
// capability, so Entities/ParseMode are ignored for non-body kinds.
type Segment struct {
	Kind      SegmentKind `json:"kind"`
	Text      string      `json:"text"`
	Entities  []Entity    `json:"entities,omitempty"`  // body segments only
	ParseMode ParseMode   `json:"parseMode,omitempty"` // body segments only
}

// ThinkingRender declares how a platform can render a SegmentThinking segment.
type ThinkingRender string

const (
	// ThinkingRenderCollapsed: native collapsible block (telegram rich text,
	// feishu folding section). Falls back to Dimmed if unavailable at runtime.
	ThinkingRenderCollapsed ThinkingRender = "collapsed"
	// ThinkingRenderDimmed: quoted/greyed block inline with the body.
	ThinkingRenderDimmed ThinkingRender = "dimmed"
	// ThinkingRenderHidden: drop by default; surface a "show reasoning" action.
	ThinkingRenderHidden ThinkingRender = "hidden"
	// ThinkingRenderInline: no special handling, append as plain text.
	ThinkingRenderInline ThinkingRender = "inline"
)

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
	ChatTypes      []ChatType      `json:"chatTypes"`
	MediaTypes     []string        `json:"mediaTypes,omitempty"`
	Features       []string        `json:"features"`
	TextLimit      int             `json:"textLimit,omitempty"`
	RateLimit      int             `json:"rateLimit,omitempty"`
	ThinkingRender ThinkingRender  `json:"thinkingRender,omitempty"`
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

// SupportsInteraction checks if the platform supports native interactive elements
// (inline keyboards, cards, components, etc.)
func (p *PlatformCapabilities) SupportsInteraction() bool {
	for _, f := range p.Features {
		switch f {
		case "inlineKeyboards", "interactiveCards", "components", "blockKit":
			return true
		}
	}
	return false
}

// EffectiveThinkingRender returns the platform's thinking-render capability,
// defaulting to ThinkingRenderDimmed when unset. The default keeps thinking
// visually subdued on platforms that have not declared a stronger mode.
func (p *PlatformCapabilities) EffectiveThinkingRender() ThinkingRender {
	if p == nil || p.ThinkingRender == "" {
		return ThinkingRenderDimmed
	}
	return p.ThinkingRender
}

// ReactionToken represents a platform-agnostic semantic reaction token.
// Use these constants instead of raw emoji or platform-specific keys.
type ReactionToken string

const (
	ReactionReceived ReactionToken = "received" // 👨‍💻 — message received, processing
	ReactionDone     ReactionToken = "done"     // ✅ / DONE / CheckMark — task completed successfully
	ReactionError    ReactionToken = "error"    // ❌ / CrossMark — task failed
	ReactionLike     ReactionToken = "like"     // 👍 / THUMBSUP — general approval
	ReactionLove     ReactionToken = "love"     // ❤️ / HEART — love / great
	ReactionLaugh    ReactionToken = "laugh"    // 😂 / LOL — funny
)

// The semantic reaction tokens above are mapped to platform-specific
// emoji/keys by ResolveReaction, backed by the single platform table in
// platforms.go.
//
// Telegram free-reaction emoji set, for reference:
// 👍 👎 ❤ 🔥 🥰 👏 😁 🤔 🤯 😱 🤬 😢 🎉 🤩 🤮 💩 🙏 👌 🕊 🤡 🥱 🥴 😍 🐳 ❤️‍🔥 🌚 🌭 💯 🤣 ⚡ 🍌 🏆 💔 🤨 😐 🍓 🍾 💋 🖕 😈 😴 😭 🤓 👻 👨‍💻 👀 🎃 🙈 😇 😨 🤝 ✍ 🤗 🫡 🎅 🎄 ☃ 💅 🤪 🗿 🆒 💘 🙉 🦄 😘 💊 🙊 😎 👾 🤷‍♂️ 🤷 🤷‍♀️ 😡
