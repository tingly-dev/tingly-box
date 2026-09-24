package core

// PlatformDescriptor is the single source of truth for a platform's intrinsic
// metadata: its display name, capabilities, semantic-reaction mapping,
// product-level behavior defaults, and auth configuration (credential type,
// settings-UI category, and the mapping that turns a stored auth-map into an
// AuthConfig).
//
// Adding or changing a platform is done in ONE place — the platformDescriptors
// table below. GetPlatformName, GetPlatformCapabilities, ResolveReaction,
// IsValidPlatform, GetPlatformAuthType, GetPlatformCategory, AuthMappingFor and
// the derived PlatformNames map all read from it, so they can no longer drift
// apart.
//
// Note the split of concerns: this table owns runtime/protocol/auth metadata.
// Settings-UI form rendering (labels, placeholders, secret flags) lives with
// the consuming package in imbot/auth.go, which derives intrinsic fields from
// this table so those cannot drift either.
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
	// Behavior holds the product-level defaults a platform implies. These used
	// to be switch statements scattered across the consuming packages, which
	// meant adding a platform required finding every one of them.
	Behavior PlatformBehavior

	// AuthType is the credential category for this platform: "token", "oauth",
	// "qr", "none". Empty means no auth config is known (the platform may still
	// be valid; auth wiring falls back to a token-based default).
	AuthType string
	// Category is the settings-UI grouping: "im", "enterprise", "business".
	Category string
	// Auth describes how a stored auth map becomes an AuthConfig for this
	// platform. nil for platforms whose credentials arrive from a non-form flow
	// (e.g. Weixin's QR onboarding) or that need no credentials (Tingly); those
	// fall back to the default token mapping in AuthMappingFor.
	Auth *AuthMapping
}

// PlatformBehavior captures the product-level defaults that follow from a
// platform's nature rather than from operator configuration.
//
// Everything here has a safe zero value, so a platform with no entry behaves
// like the conservative default rather than silently misbehaving.
type PlatformBehavior struct {
	// RequiresPairingByDefault marks platforms where possession of the bot
	// token alone grants full DM command access, so a TOFU pairing handshake
	// is enforced unless the operator explicitly opts out.
	RequiresPairingByDefault bool

	// SuppressVerbose marks platforms that cannot carry a running commentary
	// of intermediate progress messages — typically because each outbound
	// message is tied to a single inbound reply context, so follow-ups either
	// fail or arrive detached.
	//
	// Phrased as "suppress" rather than "supports" so the zero value means
	// "verbose is fine", which is true of every platform but the exceptions.
	SuppressVerbose bool
}

// GetPlatformBehavior returns the product-level defaults for a platform. An
// unknown platform gets the zero value, which is the conservative choice for
// every field.
func GetPlatformBehavior(platform Platform) PlatformBehavior {
	if d, ok := platformByID[platform]; ok {
		return d.Behavior
	}
	return PlatformBehavior{}
}

// platformDescriptors is the canonical, ordered list of known platforms.
var platformDescriptors = []PlatformDescriptor{
	{
		ID:          PlatformWhatsApp,
		DisplayName: "WhatsApp",
		AuthType:    "token",
		Category:    "business",
		Auth: &AuthMapping{
			Type:         "token",
			TokenKey:     "token",
			AccountIDKey: "phoneNumberId",
			RequiredKeys: []string{"token"},
		},
		Capabilities: &PlatformCapabilities{
			ChatTypes:      []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes:     []string{"image", "video", "audio", "document", "sticker"},
			Features:       []string{"reactions", "edit", "delete", "readReceipts", "typingIndicator"},
			TextLimit:      4096,
			RateLimit:      60,
			ThinkingRender: ThinkingRenderHidden,
		},
		Reactions: emojiReactions,
	},
	{
		ID:          PlatformTelegram,
		Behavior:    PlatformBehavior{RequiresPairingByDefault: true},
		DisplayName: "Telegram",
		AuthType:    "token",
		Category:    "im",
		Auth:        tokenAuth,
		Capabilities: &PlatformCapabilities{
			ChatTypes:      []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes:     []string{"image", "video", "audio", "document", "sticker", "gif"},
			Features:       []string{"reactions", "edit", "delete", "threads", "polls", "nativeCommands", "inlineKeyboards", "callbackQueries", "messageEditing"},
			TextLimit:      4096,
			RateLimit:      30,
			ThinkingRender: ThinkingRenderDimmed,
		},
		Reactions: emojiReactions,
	},
	{
		ID:          PlatformDiscord,
		Behavior:    PlatformBehavior{RequiresPairingByDefault: true},
		DisplayName: "Discord",
		AuthType:    "token",
		Category:    "im",
		Auth:        tokenAuth,
		Capabilities: &PlatformCapabilities{
			ChatTypes:      []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes:     []string{"image", "video", "audio", "document", "gif"},
			// "components" deliberately absent: the platform's SendMessage
			// never renders core.ActionSet into Discord message components,
			// and there is no inbound InteractionCreate handling either, so
			// claiming interaction support here makes SupportsInteraction()
			// lie — a permission/AskUserQuestion prompt would render as
			// plain text with no buttons AND, because the (false) capability
			// suppresses imprompter.go's text-fallback instructions, no way
			// to answer it at all. Re-add "components" only alongside an
			// actual button-rendering + click-callback implementation.
			Features:       []string{"reactions", "edit", "delete", "threads", "nativeCommands", "mentions", "messageEditing"},
			TextLimit:      2000,
			RateLimit:      50,
			ThinkingRender: ThinkingRenderDimmed,
		},
		Reactions: emojiReactions,
	},
	{
		ID:          PlatformSlack,
		Behavior:    PlatformBehavior{RequiresPairingByDefault: true},
		DisplayName: "Slack",
		AuthType:    "token",
		Category:    "im",
		Auth:        tokenAuth,
		Capabilities: &PlatformCapabilities{
			ChatTypes:      []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes:     []string{"image", "video", "audio", "document"},
			// "blockKit" deliberately absent: SendMessage never renders
			// core.ActionSet into Slack Block Kit buttons, and there is no
			// inbound interactive-message callback handling either — see
			// the matching comment on Discord's Features above for why a
			// false capability here is worse than no capability at all.
			Features:       []string{"reactions", "edit", "delete", "threads", "mentions", "messageEditing"},
			TextLimit:      40000,
			RateLimit:      60,
			ThinkingRender: ThinkingRenderDimmed,
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
		AuthType:    "oauth",
		Category:    "enterprise",
		Auth:        oauthClientAuth,
		Capabilities: &PlatformCapabilities{
			ChatTypes:      []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes:     []string{"image", "video", "audio", "document"},
			Features:       []string{"reactions", "delete", "threads", "nativeCommands", "mentions", "interactiveCards"},
			TextLimit:      40000, // ~150KB request body limit, practical character limit
			RateLimit:      50,
			ThinkingRender: ThinkingRenderDimmed,
		},
		Reactions: larkReactions,
	},
	{
		ID:          PlatformLark,
		DisplayName: "Lark",
		AuthType:    "oauth",
		Category:    "enterprise",
		Auth:        oauthClientAuth,
		Capabilities: &PlatformCapabilities{
			ChatTypes:      []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes:     []string{"image", "video", "audio", "document"},
			Features:       []string{"reactions", "delete", "threads", "nativeCommands", "mentions", "interactiveCards"},
			TextLimit:      40000,
			RateLimit:      50,
			ThinkingRender: ThinkingRenderDimmed,
		},
		Reactions: larkReactions,
	},
	{
		ID:          PlatformDingTalk,
		DisplayName: "DingTalk",
		AuthType:    "oauth",
		Category:    "enterprise",
		Auth:        oauthClientAuth,
		Capabilities: &PlatformCapabilities{
			ChatTypes:      []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes:     []string{"image", "video", "audio", "document"},
			Features:       []string{"reactions", "delete", "threads"},
			TextLimit:      4000,
			RateLimit:      50,
			ThinkingRender: ThinkingRenderDimmed,
		},
		Reactions: emojiReactions,
	},
	{
		// Weixin has no explicit capabilities entry and intentionally falls
		// back to the conservative default (see GetPlatformCapabilities).
		// ThinkingRender therefore defaults to Dimmed via
		// EffectiveThinkingRender() — thinking segments render as a quoted
		// block, the only viable style on this basic platform.
		ID:          PlatformWeixin,
		Behavior:    PlatformBehavior{SuppressVerbose: true},
		DisplayName: "Weixin",
		AuthType:    "qr",
		Category:    "enterprise",
		Auth: &AuthMapping{
			Type:         "qr",
			TokenKey:     "token",
			AccountIDKey: "bot_id",
			AuthDirKey:   "user_id",
			OptionKeys:   []string{"user_id", "base_url"},
			RequiredKeys: []string{"token", "bot_id"},
		},
	},
	{
		ID:          PlatformWecom,
		DisplayName: "WeCom",
		AuthType:    "oauth",
		Category:    "enterprise",
		Auth:        oauthClientAuth,
		Capabilities: &PlatformCapabilities{
			ChatTypes:      []ChatType{ChatTypeDirect, ChatTypeGroup},
			MediaTypes:     []string{"image", "video", "audio", "file"},
			Features:       []string{"streaming"},
			TextLimit:      4000,
			RateLimit:      50,
			ThinkingRender: ThinkingRenderDimmed,
		},
	},
	{
		ID:          PlatformTingly,
		DisplayName: "Tingly",
		AuthType:    "none",
		Category:    "im",
		Auth:        &AuthMapping{Type: "none", TokenKey: "token"},
		Capabilities: &PlatformCapabilities{
			ChatTypes:      []ChatType{ChatTypeDirect, ChatTypeGroup, ChatTypeChannel, ChatTypeThread},
			MediaTypes:     []string{"image", "video", "audio", "document", "sticker", "gif"},
			Features:       []string{"reactions", "edit", "delete", "threads", "polls", "inlineKeyboards", "callbackQueries", "messageEditing", "mentions", "streaming", "interactiveCards"},
			TextLimit:      65536,
			RateLimit:      1000,
			ThinkingRender: ThinkingRenderDimmed,
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

// Shared AuthMapping literals so the platform table reads as data, not inline
// wiring. They describe how a platform's stored auth-map becomes an AuthConfig.

// tokenAuth maps a single "token" auth-map key to AuthConfig.Token.
var tokenAuth = &AuthMapping{Type: "token", TokenKey: "token", RequiredKeys: []string{"token"}}

// oauthClientAuth maps clientId/clientSecret to the OAuth AuthConfig fields.
var oauthClientAuth = &AuthMapping{Type: "oauth", ClientIDKey: "clientId", ClientSecretKey: "clientSecret", RequiredKeys: []string{"clientId", "clientSecret"}}

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

// GetPlatformAuthType returns the credential category for a platform
// ("token", "oauth", "qr", "none"). Empty for unknown platforms.
func GetPlatformAuthType(platform Platform) string {
	if d, ok := platformByID[platform]; ok {
		return d.AuthType
	}
	return ""
}

// GetPlatformCategory returns the settings-UI grouping for a platform
// ("im", "enterprise", "business"). Empty for unknown platforms.
func GetPlatformCategory(platform Platform) string {
	if d, ok := platformByID[platform]; ok {
		return d.Category
	}
	return ""
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
