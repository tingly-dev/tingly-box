package bot

import (
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// BotSetting represents bot configuration with platform-specific auth
type BotSetting struct {
	UUID          string            `json:"uuid,omitempty"`           // UUID for bot identification
	Name          string            `json:"name,omitempty"`           // User-defined name for the bot
	Platform      string            `json:"platform"`                 // Platform identifier
	AuthType      string            `json:"auth_type"`                // Auth type: token, oauth, qr
	Auth          map[string]string `json:"auth"`                     // Dynamic auth fields based on platform
	ProxyURL      string            `json:"proxy_url,omitempty"`      // Optional proxy URL
	ChatIDLock    string            `json:"chat_id_lock,omitempty"`   // Optional chat ID lock (restriction, not a live chat id)
	BashAllowlist []string          `json:"bash_allowlist,omitempty"` // Optional bash command allowlist
	DefaultCwd    string            `json:"default_cwd,omitempty"`    // Default working directory if no project bound
	Enabled       bool              `json:"enabled"`                  // Whether this bot is enabled
	Scenarios     string            `json:"scenarios,omitempty"`      // Raw scenario/mount list (JSON, see remote/binding)

	// DefaultAgent selects which agent configuration serves @cc for this bot:
	// ""/"claude_code" = the main claude_code scenario, "claude_code:<id>" = a
	// Claude Code profile — @cc then routes through the profiled scenario with
	// the profile's unified/separate mode and env overrides, exactly like a
	// local `tingly-box cc --profile <id>` launch.
	DefaultAgent string `json:"default_agent,omitempty"`

	// Output behavior settings
	Verbose *bool `json:"verbose,omitempty"` // Send intermediate messages (nil = true default)

	// SmartGuide model configuration (required for @tb agent)
	SmartGuideProvider string `json:"smartguide_provider,omitempty"` // Provider UUID
	SmartGuideModel    string `json:"smartguide_model,omitempty"`    // Model identifier

	// RequirePairing enforces a TOFU pairing-code handshake before any DM is
	// processed. Tri-state: explicit true/false wins; nil means "platform
	// default" — enforced for token-DM platforms (telegram/discord/slack)
	// where a leaked bot token alone gives full command access, and disabled
	// elsewhere. Operators opt out by setting this to false explicitly.
	RequirePairing *bool `json:"require_pairing,omitempty"`

	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// SettingFromRecord converts a stored db.Settings row into the BotSetting the
// bot runtime consumes. This is the ONE conversion point — the lifecycle and
// the dynamic per-message settings refresh both go through it.
func SettingFromRecord(record db.Settings) BotSetting {
	return BotSetting{
		UUID:               record.UUID,
		Name:               record.Name,
		Platform:           record.Platform,
		AuthType:           record.AuthType,
		Auth:               record.Auth,
		ProxyURL:           record.ProxyURL,
		ChatIDLock:         record.ChatIDLock,
		BashAllowlist:      record.BashAllowlist,
		DefaultCwd:         record.DefaultCwd,
		DefaultAgent:       record.DefaultAgent,
		Enabled:            record.Enabled,
		Scenarios:          record.Scenarios,
		SmartGuideProvider: record.SmartGuideProvider,
		SmartGuideModel:    record.SmartGuideModel,
		RequirePairing:     record.RequirePairing,
	}
}

// IsRequirePairing reports whether this bot requires per-chat pairing.
// When RequirePairing is nil, the answer depends on Platform: token-DM
// platforms default to enforced; OAuth/QR platforms default to off.
func (b BotSetting) IsRequirePairing() bool {
	if b.RequirePairing != nil {
		return *b.RequirePairing
	}
	return PlatformDefaultsRequirePairing(b.Platform)
}

// PlatformDefaultsRequirePairing reports whether a bot on the given platform
// has TOFU pairing enforced when RequirePairing is unset (nil).
//
// The answer comes from imbot's platform descriptor table rather than a switch
// here: which platforms hand out full DM command access to anyone holding the
// bot token is a fact about the platform, and it belongs next to the rest of
// each platform's intrinsic metadata.
func PlatformDefaultsRequirePairing(platform string) bool {
	return imbot.GetPlatformBehavior(imbot.Platform(platform)).RequiresPairingByDefault
}

// Chat represents all state associated with a chat (direct or group).
//
// The struct itself lives in internal/data/db because that is where the store
// backing it lives, and this package already imports db — defining it there
// is what lets the SQLite store implement ChatStoreInterface without an import
// cycle. The alias keeps every caller in this package writing bot.Chat.
//
// Sessions are not part of a Chat: they are managed by SessionManager, keyed
// by the (ChatID, Agent, Project) binding.
type Chat = db.Chat

// ChatStoreInterface defines the interface for chat persistence, keeping the
// bot package independent of where chats are actually stored.
type ChatStoreInterface interface {
	// GetChat retrieves a chat by ID
	GetChat(chatID string) (*Chat, error)

	// GetOrCreateChat gets a chat or creates it if not exists
	GetOrCreateChat(chatID, platform string) (*Chat, error)

	// UpsertChat creates or updates a chat
	UpsertChat(chat *Chat) error

	// UpdateChat updates specific fields of a chat
	UpdateChat(chatID string, fn func(*Chat)) error

	// BindProject binds a project to a chat
	BindProject(chatID, platform, projectPath, ownerID string) error

	// GetProjectPath retrieves the project path for a chat
	GetProjectPath(chatID string) (string, bool, error)

	// ListChatsByOwner lists all chats owned by a user
	ListChatsByOwner(ownerID, platform string) ([]*Chat, error)

	// ListChats returns the chat records this bot can reach on the given
	// platform — i.e. those whose Platform field is set AND equals platform.
	// Records with an empty or mismatched Platform are dropped at the source:
	// the store key has no platform dimension, so an unattributed record
	// cannot be proven to belong to this bot's channel and must not leak into
	// its /chats list. Used by the GET /bots/:bot/chats API so callers of the
	// notify/interact endpoints can discover the channel-native chat_id they
	// must pass in the request body.
	ListChats(platform string) ([]*Chat, error)

	// ListChatProjectPaths returns the MRU project-path history for a chat.
	ListChatProjectPaths(chatID string) ([]string, error)

	// AddToWhitelist adds a chat to the whitelist
	AddToWhitelist(chatID, platform, addedBy string) error

	// RemoveFromWhitelist removes a chat from the whitelist
	RemoveFromWhitelist(chatID string) error

	// IsWhitelisted checks if a chat is whitelisted
	IsWhitelisted(chatID string) bool

	// SetBashCwd sets the bash working directory for a chat
	SetBashCwd(chatID, cwd string) error

	// GetBashCwd retrieves the bash working directory for a chat
	GetBashCwd(chatID string) (string, bool, error)

	// SetCurrentAgent sets the current agent for a chat. Creates the chat
	// row if it doesn't yet exist so that @cc/@tb handoff state persists
	// even on fresh chats that haven't been bound (/cd) or paired (/bind)
	// yet. Pass an empty platform when the caller doesn't have one — the
	// field will be filled in later by BindProject/SetPaired.
	SetCurrentAgent(chatID, platform, agentType string) error

	// GetCurrentAgent retrieves the current agent for a chat
	GetCurrentAgent(chatID string) (string, error)

	// SetPaired marks a chat as paired with a specific bot UUID and sender.
	// The chat is created if it does not yet exist.
	SetPaired(chatID, platform, botUUID, senderID string) error

	// ClearPaired removes the pairing on a chat. Other state on the chat is
	// preserved.
	ClearPaired(chatID string) error

	// IsChatPaired reports whether the chat is paired with the given bot UUID.
	IsChatPaired(chatID, botUUID string) bool
}

// Ensure the SQLite-backed store satisfies the interface. The JSON store this
// replaces is gone: it kept every chat in one file that each holder rewrote
// whole, so concurrent writers erased each other. See .design/remote-storage.md.
var _ ChatStoreInterface = (*db.RemoteChatStore)(nil)
