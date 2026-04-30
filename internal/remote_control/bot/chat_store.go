package bot

import (
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/pkg/jsonstore"
)

// Error definitions
var (
	ErrStoreNotInitialized = errors.New("chat store not initialized")
	ErrChatNotFound        = errors.New("chat not found")
)

// BotSetting represents bot configuration with platform-specific auth
type BotSetting struct {
	UUID          string            `json:"uuid,omitempty"`           // UUID for bot identification
	Name          string            `json:"name,omitempty"`           // User-defined name for the bot
	Token         string            `json:"token,omitempty"`          // Legacy: for backward compatibility
	Platform      string            `json:"platform"`                 // Platform identifier
	AuthType      string            `json:"auth_type"`                // Auth type: token, oauth, qr
	Auth          map[string]string `json:"auth"`                     // Dynamic auth fields based on platform
	ProxyURL      string            `json:"proxy_url,omitempty"`      // Optional proxy URL
	ChatIDLock    string            `json:"chat_id,omitempty"`        // Optional chat ID lock
	BashAllowlist []string          `json:"bash_allowlist,omitempty"` // Optional bash command allowlist
	DefaultCwd    string            `json:"default_cwd,omitempty"`    // Default working directory if no project bound
	Enabled       bool              `json:"enabled"`                  // Whether this bot is enabled

	// Output behavior settings
	Verbose *bool `json:"verbose,omitempty"` // Send intermediate messages (nil = true default)

	// SmartGuide model configuration (required for @tb agent)
	SmartGuideProvider string `json:"smartguide_provider,omitempty"` // Provider UUID
	SmartGuideModel    string `json:"smartguide_model,omitempty"`    // Model identifier

	// RequirePairing enforces a TOFU pairing-code handshake before any DM is
	// processed. Nil is treated as false so legacy bots keep working until the
	// operator opts in. New bots created via the wizard set this to true.
	RequirePairing *bool `json:"require_pairing,omitempty"`

	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// IsRequirePairing reports whether this bot requires per-chat pairing.
func (b BotSetting) IsRequirePairing() bool {
	return b.RequirePairing != nil && *b.RequirePairing
}

// Chat represents all state associated with a chat (direct or group)
type Chat struct {
	ChatID      string `json:"chat_id"`
	Platform    string `json:"platform"`
	ProjectPath string `json:"project_path,omitempty"`
	OwnerID     string `json:"owner_id,omitempty"`

	// Session removed - sessions are now managed by SessionManager with (ChatID, Agent, Project) binding

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

	// Agent state (for smart guide handoff)
	CurrentAgent string `json:"current_agent,omitempty"` // "tingly-box" or "claude"
	AgentState   []byte `json:"agent_state,omitempty"`   // JSON-encoded agent-specific state

	// Chat-level settings
	Verbose *bool `json:"verbose,omitempty"` // Verbose mode: nil=use bot default, true=verbose, false=quiet

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChatStoreInterface defines the interface for chat persistence
// This allows both SQLite-based ChatStore and JSON-based ChatStoreJSON to be used interchangeably
type ChatStoreInterface interface {
	// Close ensures data is persisted before closing
	Close() error

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

	// SetCurrentAgent sets the current agent for a chat
	SetCurrentAgent(chatID, agentType string) error

	// GetCurrentAgent retrieves the current agent for a chat
	GetCurrentAgent(chatID string) (string, error)

	// SetAgentState sets the agent-specific state for a chat
	SetAgentState(chatID string, state []byte) error

	// GetAgentState retrieves the agent-specific state for a chat
	GetAgentState(chatID string) ([]byte, error)

	// ListWhitelistedGroups returns all whitelisted groups
	ListWhitelistedGroups() ([]struct {
		GroupID   string
		Platform  string
		AddedBy   string
		CreatedAt string
	}, error)

	// SetPaired marks a chat as paired with a specific bot UUID and sender.
	// The chat is created if it does not yet exist.
	SetPaired(chatID, platform, botUUID, senderID string) error

	// ClearPaired removes the pairing on a chat. Other state on the chat is
	// preserved.
	ClearPaired(chatID string) error

	// IsChatPaired reports whether the chat is paired with the given bot UUID.
	IsChatPaired(chatID, botUUID string) bool
}

// Ensure ChatStoreJSON implements the interface
var _ ChatStoreInterface = (*ChatStoreJSON)(nil)

// ChatStoreJSON handles unified chat persistence using JSON file storage
type ChatStoreJSON struct {
	store *jsonstore.Store[Chat]
}

func (s *ChatStoreJSON) ensureStore() error {
	if s == nil || s.store == nil {
		return ErrStoreNotInitialized
	}
	return nil
}

func normalizeChat(chat *Chat) error {
	if chat == nil || chat.ChatID == "" {
		return fmt.Errorf("chat_id is required")
	}

	now := time.Now().UTC()
	if chat.CreatedAt.IsZero() {
		chat.CreatedAt = now
	}
	chat.UpdatedAt = now
	if chat.CurrentAgent == "" {
		chat.CurrentAgent = AgentNameTinglyBox
	}
	return nil
}

func (s *ChatStoreJSON) forceSave() {
	if err := s.store.ForceSave(); err != nil {
		logrus.WithError(err).Error("Failed to force save chat store to disk")
	}
}

func (s *ChatStoreJSON) saveChat(chat *Chat) error {
	if err := normalizeChat(chat); err != nil {
		return err
	}
	if err := s.store.Set(chat.ChatID, chat); err != nil {
		return err
	}
	s.forceSave()
	return nil
}

func defaultCurrentAgent(chat *Chat) string {
	if chat == nil || chat.CurrentAgent == "" {
		return AgentNameTinglyBox
	}
	return chat.CurrentAgent
}

// NewChatStoreJSON creates a new JSON-based chat store
func NewChatStoreJSON(filePath string) (*ChatStoreJSON, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path is required")
	}

	store, err := jsonstore.New[Chat](filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create JSON store: %w", err)
	}

	return &ChatStoreJSON{store: store}, nil
}

// Close ensures data is persisted before closing
func (s *ChatStoreJSON) Close() error {
	if err := s.ensureStore(); err != nil {
		return err
	}
	return s.store.Close()
}

// GetChat retrieves a chat by ID
func (s *ChatStoreJSON) GetChat(chatID string) (*Chat, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	return s.store.Get(chatID), nil
}

// GetOrCreateChat gets a chat or creates it if not exists
func (s *ChatStoreJSON) GetOrCreateChat(chatID, platform string) (*Chat, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}

	if chat := s.store.Get(chatID); chat != nil {
		return chat, nil
	}

	now := time.Now().UTC()
	newChat := &Chat{
		ChatID:    chatID,
		Platform:  platform,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.UpsertChat(newChat); err != nil {
		return nil, err
	}

	return newChat, nil
}

// UpsertChat creates or updates a chat
func (s *ChatStoreJSON) UpsertChat(chat *Chat) error {
	if err := s.ensureStore(); err != nil {
		return err
	}
	return s.saveChat(chat)
}

// UpdateChat updates specific fields of a chat
func (s *ChatStoreJSON) UpdateChat(chatID string, fn func(*Chat)) error {
	if err := s.ensureStore(); err != nil {
		return err
	}
	if fn == nil {
		return fmt.Errorf("update function is required")
	}

	var normalizeErr error
	var updated bool
	err := s.store.Update(chatID, func(chat *Chat) *Chat {
		if chat == nil {
			return nil
		}
		fn(chat)
		normalizeErr = normalizeChat(chat)
		updated = true
		return chat
	})
	if err != nil {
		return err
	}
	if normalizeErr != nil {
		return normalizeErr
	}
	if updated {
		s.forceSave()
	}
	return nil
}

// ============== Project Binding ==============

// BindProject binds a project to a chat (creates chat if not exists)
func (s *ChatStoreJSON) BindProject(chatID, platform, projectPath, ownerID string) error {
	chat, err := s.GetOrCreateChat(chatID, platform)
	if err != nil {
		return err
	}

	chat.Platform = platform
	chat.ProjectPath = projectPath
	chat.OwnerID = ownerID
	return s.UpsertChat(chat)
}

// GetProjectPath retrieves the project path for a chat
func (s *ChatStoreJSON) GetProjectPath(chatID string) (string, bool, error) {
	if err := s.ensureStore(); err != nil {
		return "", false, err
	}
	chat := s.store.Get(chatID)
	if chat == nil || chat.ProjectPath == "" {
		return "", false, nil
	}
	return chat.ProjectPath, true, nil
}

// ListChatsByOwner lists all chats owned by a user
func (s *ChatStoreJSON) ListChatsByOwner(ownerID, platform string) ([]*Chat, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}

	items := s.store.List()
	var chats []*Chat
	for _, chat := range items {
		if chat.OwnerID == ownerID && chat.Platform == platform && chat.ProjectPath != "" {
			chats = append(chats, chat)
		}
	}

	return chats, nil
}

// ============== Whitelist ==============

// AddToWhitelist adds a chat to the whitelist
func (s *ChatStoreJSON) AddToWhitelist(chatID, platform, addedBy string) error {
	chat, err := s.GetOrCreateChat(chatID, platform)
	if err != nil {
		return err
	}
	chat.IsWhitelisted = true
	chat.WhitelistedBy = addedBy
	return s.UpsertChat(chat)
}

// RemoveFromWhitelist removes a chat from the whitelist
func (s *ChatStoreJSON) RemoveFromWhitelist(chatID string) error {
	return s.UpdateChat(chatID, func(chat *Chat) {
		chat.IsWhitelisted = false
	})
}

// IsWhitelisted checks if a chat is whitelisted
func (s *ChatStoreJSON) IsWhitelisted(chatID string) bool {
	if s == nil || s.store == nil {
		return false
	}
	chat := s.store.Get(chatID)
	if chat == nil {
		return false
	}
	return chat.IsWhitelisted
}

// ============== Bash CWD ==============

// SetBashCwd sets the bash working directory for a chat
func (s *ChatStoreJSON) SetBashCwd(chatID, cwd string) error {
	return s.UpdateChat(chatID, func(chat *Chat) {
		chat.BashCwd = cwd
	})
}

// GetBashCwd retrieves the bash working directory for a chat
func (s *ChatStoreJSON) GetBashCwd(chatID string) (string, bool, error) {
	if s == nil || s.store == nil {
		return "", false, nil
	}
	chat := s.store.Get(chatID)
	if chat == nil || chat.BashCwd == "" {
		return "", false, nil
	}
	return chat.BashCwd, true, nil
}

// ============== Current Agent ==============

// SetCurrentAgent sets the current agent for a chat
func (s *ChatStoreJSON) SetCurrentAgent(chatID, agentType string) error {
	return s.UpdateChat(chatID, func(chat *Chat) {
		chat.CurrentAgent = agentType
	})
}

// GetCurrentAgent retrieves the current agent for a chat
// Returns "tingly-box" as default (Smart Guide is the entry point)
func (s *ChatStoreJSON) GetCurrentAgent(chatID string) (string, error) {
	if s == nil || s.store == nil {
		return AgentNameTinglyBox, nil
	}
	return defaultCurrentAgent(s.store.Get(chatID)), nil
}

// SetAgentState sets the agent-specific state for a chat
func (s *ChatStoreJSON) SetAgentState(chatID string, state []byte) error {
	return s.UpdateChat(chatID, func(chat *Chat) {
		chat.AgentState = state
	})
}

// GetAgentState retrieves the agent-specific state for a chat
func (s *ChatStoreJSON) GetAgentState(chatID string) ([]byte, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	chat := s.store.Get(chatID)
	if chat == nil {
		return nil, nil
	}
	return chat.AgentState, nil
}

// ListWhitelistedGroups returns all whitelisted groups
func (s *ChatStoreJSON) ListWhitelistedGroups() ([]struct {
	GroupID   string
	Platform  string
	AddedBy   string
	CreatedAt string
}, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}

	items := s.store.List()
	var results []struct {
		GroupID   string
		Platform  string
		AddedBy   string
		CreatedAt string
	}

	for _, chat := range items {
		if chat.IsWhitelisted {
			results = append(results, struct {
				GroupID   string
				Platform  string
				AddedBy   string
				CreatedAt string
			}{
				GroupID:   chat.ChatID,
				Platform:  chat.Platform,
				AddedBy:   chat.WhitelistedBy,
				CreatedAt: chat.CreatedAt.Format(time.RFC3339),
			})
		}
	}

	return results, nil
}

// ============== Pairing (TOFU) ==============

// SetPaired marks the given chat as paired with botUUID/senderID.
func (s *ChatStoreJSON) SetPaired(chatID, platform, botUUID, senderID string) error {
	if chatID == "" || botUUID == "" {
		return fmt.Errorf("chat_id and bot_uuid are required")
	}

	chat, err := s.GetOrCreateChat(chatID, platform)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	chat.IsPaired = true
	chat.PairedBotUUID = botUUID
	chat.PairedSenderID = senderID
	chat.PairedAt = now
	if platform != "" {
		chat.Platform = platform
	}
	return s.UpsertChat(chat)
}

// ClearPaired removes any pairing recorded on the chat.
func (s *ChatStoreJSON) ClearPaired(chatID string) error {
	return s.UpdateChat(chatID, func(chat *Chat) {
		chat.IsPaired = false
		chat.PairedBotUUID = ""
		chat.PairedSenderID = ""
		chat.PairedAt = time.Time{}
	})
}

// IsChatPaired reports whether the chat is paired with the given bot UUID.
func (s *ChatStoreJSON) IsChatPaired(chatID, botUUID string) bool {
	if s == nil || s.store == nil {
		return false
	}
	chat := s.store.Get(chatID)
	if chat == nil {
		return false
	}
	return chat.IsPaired && chat.PairedBotUUID == botUUID
}
