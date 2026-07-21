package core

// PlatformDescriptor is the single source of truth for a platform's intrinsic
// metadata: its display name, capabilities, and semantic-reaction mapping.
//
// Adding or changing a platform's name/capabilities/reactions is done in ONE
// place — the platformDescriptors table below. GetPlatformName,
// GetPlatformCapabilities, ResolveReaction, IsValidPlatform and the derived
// PlatformNames map all read from it, so they can no longer drift apart.
//
// Note the split of concerns: this table owns runtime/protocol metadata.
// Settings-UI metadata (auth type, form fields, category) lives with the
// consuming package in imbot/platform.go, which derives its display names from
// GetPlatformName so those cannot drift either.
type PlatformDescriptor struct {
	// ID is the platform identifier.
	ID Platform
	// DisplayName is the human-readable name.
	DisplayName string
	// Capabilities describes what the platform supports. May be nil, in which
	// case GetPlatformCapabilities returns the conservative default — this
	// mirrors the historical behavior for platforms with no explicit entry.
	Capabilities *PlatformCapabilities
	// Reactions maps semantic reaction tokens to platform-specific emoji/keys.
	// nil means ResolveReaction falls back to the token string itself.
	Reactions map[ReactionToken]string
}

// platformDescriptors is the canonical, ordered list of known platforms.
var platformDescriptors = []PlatformDescriptor{
	{
		ID:          PlatformWhatsApp,
		DisplayName: "WhatsApp",
		Capabilities: &PlatformCapabilities{
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes: []string{"image", "video", "audio", "document", "sticker"},
			Features:   []string{"reactions", "edit", "delete", "readReceipts", "typingIndicator"},
			TextLimit:  4096,
			RateLimit:  60,
		},
		Reactions: emojiReactions,
	},
	{
		ID:          PlatformTelegram,
		DisplayName: "Telegram",
		Capabilities: &PlatformCapabilities{
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes: []string{"image", "video", "audio", "document", "sticker", "gif"},
			Features:   []string{"reactions", "edit", "delete", "threads", "polls", "nativeCommands", "inlineKeyboards", "callbackQueries", "messageEditing"},
			TextLimit:  4096,
			RateLimit:  30,
		},
		Reactions: emojiReactions,
	},
	{
		ID:          PlatformDiscord,
		DisplayName: "Discord",
		Capabilities: &PlatformCapabilities{
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes: []string{"image", "video", "audio", "document", "gif"},
			Features:   []string{"reactions", "edit", "delete", "threads", "nativeCommands", "mentions", "components", "messageEditing"},
			TextLimit:  2000,
			RateLimit:  50,
		},
		Reactions: emojiReactions,
	},
	{
		ID:          PlatformSlack,
		DisplayName: "Slack",
		Capabilities: &PlatformCapabilities{
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes: []string{"image", "video", "audio", "document"},
			Features:   []string{"reactions", "edit", "delete", "threads", "mentions", "blockKit", "messageEditing"},
			TextLimit:  40000,
			RateLimit:  60,
		},
		Reactions: map[ReactionToken]string{
			ReactionReceived: "eyes",
			ReactionDone:     "white_check_mark",
			ReactionError:    "x",
			ReactionLike:     "thumbsup",
			ReactionLove:     "heart",
			ReactionLaugh:    "joy",
		},
	},
	{
		ID:          PlatformGoogleChat,
		DisplayName: "Google Chat",
		Capabilities: &PlatformCapabilities{
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeThread},
			MediaTypes: []string{"image", "video"},
			Features:   []string{"reactions", "delete", "threads"},
			TextLimit:  4000,
			RateLimit:  30,
		},
	},
	{
		ID:          PlatformSignal,
		DisplayName: "Signal",
		Capabilities: &PlatformCapabilities{
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes: []string{"image", "video", "audio", "document"},
			Features:   []string{"reactions", "delete", "readReceipts", "typingIndicator"},
			TextLimit:  4096,
			RateLimit:  60,
		},
	},
	{
		ID:          PlatformBlueBubbles,
		DisplayName: "BlueBubbles (iMessage)",
		Capabilities: &PlatformCapabilities{
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes: []string{"image", "video", "audio", "document"},
			Features:   []string{"reactions", "edit", "delete", "readReceipts", "typingIndicator"},
			TextLimit:  4000,
			RateLimit:  60,
		},
	},
	{
		ID:          PlatformFeishu,
		DisplayName: "Feishu",
		Capabilities: &PlatformCapabilities{
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes: []string{"image", "video", "audio", "document"},
			Features:   []string{"reactions", "delete", "threads", "nativeCommands", "mentions", "interactiveCards"},
			TextLimit:  40000, // ~150KB request body limit, practical character limit
			RateLimit:  50,
		},
		Reactions: larkReactions,
	},
	{
		ID:          PlatformLark,
		DisplayName: "Lark",
		Capabilities: &PlatformCapabilities{
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes: []string{"image", "video", "audio", "document"},
			Features:   []string{"reactions", "delete", "threads", "nativeCommands", "mentions", "interactiveCards"},
			TextLimit:  40000,
			RateLimit:  50,
		},
		Reactions: larkReactions,
	},
	{
		ID:          PlatformDingTalk,
		DisplayName: "DingTalk",
		Capabilities: &PlatformCapabilities{
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes: []string{"image", "video", "audio", "document"},
			Features:   []string{"reactions", "delete", "threads"},
			TextLimit:  4000,
			RateLimit:  50,
		},
		Reactions: emojiReactions,
	},
	{
		// Weixin has no explicit capabilities entry and intentionally falls
		// back to the conservative default (see GetPlatformCapabilities).
		ID:          PlatformWeixin,
		DisplayName: "Weixin",
	},
	{
		ID:          PlatformWecom,
		DisplayName: "WeCom",
		Capabilities: &PlatformCapabilities{
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes: []string{"image", "video", "audio", "file"},
			Features:   []string{"streaming"},
			TextLimit:  4000,
			RateLimit:  50,
		},
	},
	{
		ID:          PlatformTingly,
		DisplayName: "Tingly",
		Capabilities: &PlatformCapabilities{
			ChatTypes:  []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes: []string{"image", "video", "audio", "document", "sticker", "gif"},
			Features:   []string{"reactions", "edit", "delete", "threads", "polls", "inlineKeyboards", "callbackQueries", "messageEditing", "mentions", "streaming", "interactiveCards"},
			TextLimit:  65536,
			RateLimit:  1000,
		},
		Reactions: emojiReactions,
	},
}

// emojiReactions is the default unicode-emoji reaction set shared by platforms
// that accept raw emoji (Telegram, Discord, WhatsApp, DingTalk, Tingly).
var emojiReactions = map[ReactionToken]string{
	ReactionReceived: "👨‍💻",
	ReactionDone:     "✅",
	ReactionError:    "❌",
	ReactionLike:     "👍",
	ReactionLove:     "❤️",
	ReactionLaugh:    "😂",
}

// larkReactions is the Feishu/Lark reaction-key set (named keys, not emoji).
var larkReactions = map[ReactionToken]string{
	ReactionReceived: "Get",
	ReactionDone:     "DONE",
	ReactionError:    "CrossMark",
	ReactionLike:     "THUMBSUP",
	ReactionLove:     "HEART",
	ReactionLaugh:    "LOL",
}

// platformByID indexes platformDescriptors for O(1) lookup.
var platformByID = func() map[Platform]*PlatformDescriptor {
	m := make(map[Platform]*PlatformDescriptor, len(platformDescriptors))
	for i := range platformDescriptors {
		d := &platformDescriptors[i]
		m[d.ID] = d
	}
	return m
}()

// defaultPlatformCapabilities is returned for platforms with no explicit entry.
var defaultPlatformCapabilities = &PlatformCapabilities{
	ChatTypes: []ChatType{ChatTypeDirect},
	Features:  []string{},
}

// PlatformNames maps each known platform to its human-readable name.
// Derived from platformDescriptors; do not edit by hand.
var PlatformNames = func() map[Platform]string {
	m := make(map[Platform]string, len(platformDescriptors))
	for _, d := range platformDescriptors {
		m[d.ID] = d.DisplayName
	}
	return m
}()

// GetPlatformName returns the human-readable name for a platform.
func GetPlatformName(platform Platform) string {
	if d, ok := platformByID[platform]; ok {
		return d.DisplayName
	}
	return string(platform)
}

// IsValidPlatform reports whether the platform is a known platform identifier.
func IsValidPlatform(platform string) bool {
	_, ok := platformByID[Platform(platform)]
	return ok
}

// GetPlatformCapabilities returns the capabilities for a given platform,
// falling back to a conservative default for unknown or entry-less platforms.
func GetPlatformCapabilities(platform Platform) *PlatformCapabilities {
	if d, ok := platformByID[platform]; ok && d.Capabilities != nil {
		return d.Capabilities
	}
	return defaultPlatformCapabilities
}

// ResolveReaction returns the platform-specific emoji/key for a semantic
// reaction token, falling back to the token string if no mapping is found.
func ResolveReaction(platform Platform, r ReactionToken) string {
	if d, ok := platformByID[platform]; ok && d.Reactions != nil {
		if v, ok := d.Reactions[r]; ok {
			return v
		}
	}
	return string(r)
}
