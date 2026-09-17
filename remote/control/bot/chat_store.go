package bot

import (
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
)

// BotSetting represents bot configuration with platform-specific auth
type BotSetting struct {
	UUID          string            `json:"uuid,omitempty"`           // UUID for bot identification
	Name          string            `json:"name,omitempty"`           // User-defined name for the bot
	Platform      string            `json:"platform"`                 // Platform identifier
	AuthType      string            `json:"auth_type"`                // Auth type: token, oauth, qr
	Auth          map[string]string `json:"auth"`                     // Dynamic auth fields based on platform
	ProxyURL      string            `json:"proxy_url,omitempty"`      // Optional proxy URL
	ChatIDLock    string            `json:"chat_id_lock,omitempty"`   // Deprecated: retained for settings compatibility; access policy supersedes it.
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

	// PersistentSession opts @cc into a long-lived Claude Code process kept
	// warm across chat turns instead of one process per message. Nil/false
	// is the default — see .design/claude-code.md.
	PersistentSession *bool `json:"persistent_session,omitempty"`

	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// IsPersistentSession reports whether @cc should keep a Claude Code process
// warm across chat turns for this bot. Nil (unset) defaults to false.
func (b BotSetting) IsPersistentSession() bool {
	return b.PersistentSession != nil && *b.PersistentSession
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

// ProjectHistoryCap bounds the per-chat MRU list so storage stays bounded and
// the /project list stays readable.
const ProjectHistoryCap = 20

// Chat is all state associated with an IM chat (direct or group): which
// project it is bound to, whether it is paired or whitelisted, and which
// agent is currently driving it.
//
// This is the remote-owned domain type. The SQLite store in internal/data/db
// implements ChatStoreInterface against it directly (converting to/from its
// own GORM RemoteChatRecord) — same pattern as remote/session, where db
// imports the remote package that owns the domain type. Callers write
// bot.Chat everywhere.
//
// Sessions are not part of a Chat: they are managed by SessionManager, keyed
// by the (ChatID, Agent, Project) binding.
type Chat struct {
	ChatID         string   `json:"chat_id"`
	Platform       string   `json:"platform"`
	ProjectPath    string   `json:"project_path,omitempty"`
	ProjectHistory []string `json:"project_history,omitempty"` // MRU list of paths this chat has bound to
	OwnerID        string   `json:"owner_id,omitempty"`

	// Pairing (TOFU) — applies to direct messages only. Group chats continue
	// to use the IsWhitelisted gate, but the operator who whitelisted the
	// group must themselves be paired in DM with the same bot.
	IsPaired       bool      `json:"is_paired,omitempty"`
	PairedBotUUID  string    `json:"paired_bot_uuid,omitempty"`
	PairedSenderID string    `json:"paired_sender_id,omitempty"`
	PairedAt       time.Time `json:"paired_at,omitempty"`

	// Group-specific
	IsWhitelisted bool   `json:"is_whitelisted"`
	WhitelistedBy string `json:"whitelisted_by,omitempty"`

	// Bash state
	BashCwd string `json:"bash_cwd,omitempty"`

	// ContextToken is the most recent reply-context token seen from this chat
	// (WeChat ilink). Proactive notifications reuse it so the server's prepare
	// step succeeds without an inbound message in the same turn; without it the
	// send fails with ret=-2 / "prepare failed". Refreshed by every inbound
	// message; day-scale lifetime, so persisting it survives bot restarts.
	ContextToken string `json:"context_token,omitempty"`

	// CurrentAgent is which agent is driving the chat ("tingly-box" or "claude").
	CurrentAgent string `json:"current_agent,omitempty"`

	// Chat-level settings
	Verbose *bool `json:"verbose,omitempty"` // Verbose mode: nil=use bot default, true=verbose, false=quiet

	// Disabled is the inbound blocklist flag: a disabled chat's messages are
	// dropped before any handler runs and the chat is excluded from the
	// reachable list. Unlike deletion, the row survives auto-create paths —
	// only an explicit enable clears it.
	Disabled   bool      `json:"disabled,omitempty"`
	DisabledAt time.Time `json:"disabled_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PushProjectHistory sets chat.ProjectPath and prepends it to ProjectHistory
// (deduped, capped). When the chat already had a ProjectPath that wasn't in
// the history yet, it is preserved one slot below so a fresh upgrade keeps
// the previous binding visible.
func (c *Chat) PushProjectHistory(path string) {
	if c == nil || path == "" {
		return
	}
	prior := c.ProjectHistory
	if len(prior) == 0 && c.ProjectPath != "" && c.ProjectPath != path {
		prior = []string{c.ProjectPath}
	}
	c.ProjectPath = path

	out := make([]string, 0, len(prior)+1)
	out = append(out, path)
	for _, p := range prior {
		if p == "" || p == path {
			continue
		}
		out = append(out, p)
		if len(out) >= ProjectHistoryCap {
			break
		}
	}
	c.ProjectHistory = out
}

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
	// its /chats list. Disabled chats are excluded unless includeDisabled is
	// set. Used by the GET /bots/:bot/chats API so callers of the
	// notify/interact endpoints can discover the channel-native chat_id they
	// must pass in the request body.
	ListChats(platform string, includeDisabled bool) ([]*Chat, error)

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

	// DeleteChat hard-deletes the chat row. All chat state (pairing,
	// whitelist, project binding) is gone; a new message from the same chat
	// recreates it fresh via the normal auto-create path. Sessions are
	// untouched. Deleting a missing chat is a no-op.
	DeleteChat(chatID string) error

	// SetChatDisabled toggles the inbound blocklist flag. A disabled chat's
	// messages are dropped before any handler runs and the chat is excluded
	// from the reachable list. The row survives auto-create paths — only an
	// explicit enable clears the flag.
	SetChatDisabled(chatID string, disabled bool) error

	// IsChatDisabled reports the blocklist flag. Missing chat → false.
	IsChatDisabled(chatID string) bool
}

// The SQLite-backed store (internal/data/db.RemoteChatStore) satisfies this
// interface; the compile-time assertion lives on the db side, next to the
// store, so this package does not import db. The JSON store this replaces is
// gone: it kept every chat in one file that each holder rewrote whole, so
// concurrent writers erased each other. See .design/remote-storage.md.
