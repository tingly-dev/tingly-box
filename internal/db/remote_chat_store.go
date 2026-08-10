package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tingly-dev/tingly-box/remote/control/bot"
)

// Compile-time proof that this store backs the remote bot runtime's chat
// persistence. Mirrors the assertion in remote_session_store.go.
var _ bot.ChatStoreInterface = (*RemoteChatStore)(nil)

// DefaultChatAgent is the agent a chat is driven by until something hands it
// off — Smart Guide is the entry point.
const DefaultChatAgent = "tingly-box"

// Sentinels for the remote chat store.
var (
	// ErrChatIDRequired is returned when an operation needs a chat ID and got none.
	ErrChatIDRequired = errors.New("chat_id is required")
	// ErrStoreClosed is returned when the store has no usable DB handle.
	ErrStoreClosed = errors.New("chat store not initialized")
)

// RemoteChatStore persists IM chat state in the shared SQLite database.
//
// It replaces a JSON file that every bot loaded into its own memory and
// rewrote whole, which meant concurrent writers silently erased each other —
// in-process (fixed earlier by sharing one store) and across processes (only
// fixed by being here, on one WAL database with row-level updates).
type RemoteChatStore struct {
	db *gorm.DB
}

// NewRemoteChatStore builds a store over an existing DB handle. The handle is
// owned by the StoreManager; Close here does not close it.
func NewRemoteChatStore(db *gorm.DB) *RemoteChatStore {
	return &RemoteChatStore{db: db}
}

func (s *RemoteChatStore) ready() bool { return s != nil && s.db != nil }

// ---------- mapping ----------

func encodeProjectHistory(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	raw, err := json.Marshal(paths)
	if err != nil {
		logrus.WithError(err).Warn("remote chat: dropping unencodable project history")
		return ""
	}
	return string(raw)
}

func decodeProjectHistory(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		logrus.WithError(err).Warn("remote chat: unreadable project history, treating as empty")
		return nil
	}
	return out
}

func toChatRecord(c *bot.Chat) *RemoteChatRecord {
	return &RemoteChatRecord{
		ChatID:         c.ChatID,
		Platform:       c.Platform,
		ProjectPath:    c.ProjectPath,
		OwnerID:        c.OwnerID,
		ProjectHistory: encodeProjectHistory(c.ProjectHistory),
		IsPaired:       c.IsPaired,
		PairedBotUUID:  c.PairedBotUUID,
		PairedSenderID: c.PairedSenderID,
		PairedAt:       c.PairedAt,
		IsWhitelisted:  c.IsWhitelisted,
		WhitelistedBy:  c.WhitelistedBy,
		BashCwd:        c.BashCwd,
		CurrentAgent:   c.CurrentAgent,
		Verbose:        c.Verbose,
		Disabled:       c.Disabled,
		DisabledAt:     c.DisabledAt,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}

func fromChatRecord(r *RemoteChatRecord) *bot.Chat {
	return &bot.Chat{
		ChatID:         r.ChatID,
		Platform:       r.Platform,
		ProjectPath:    r.ProjectPath,
		ProjectHistory: decodeProjectHistory(r.ProjectHistory),
		OwnerID:        r.OwnerID,
		IsPaired:       r.IsPaired,
		PairedBotUUID:  r.PairedBotUUID,
		PairedSenderID: r.PairedSenderID,
		PairedAt:       r.PairedAt,
		IsWhitelisted:  r.IsWhitelisted,
		WhitelistedBy:  r.WhitelistedBy,
		BashCwd:        r.BashCwd,
		CurrentAgent:   r.CurrentAgent,
		Verbose:        r.Verbose,
		Disabled:       r.Disabled,
		DisabledAt:     r.DisabledAt,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

// normalizeChat fills in the invariants every stored chat must satisfy.
func normalizeChat(chat *bot.Chat) error {
	if chat == nil || chat.ChatID == "" {
		return ErrChatIDRequired
	}
	now := time.Now().UTC()
	if chat.CreatedAt.IsZero() {
		chat.CreatedAt = now
	}
	chat.UpdatedAt = now
	if chat.CurrentAgent == "" {
		chat.CurrentAgent = DefaultChatAgent
	}
	return nil
}

// ---------- basic CRUD ----------

// GetChat retrieves a chat by ID. A missing chat is (nil, nil).
func (s *RemoteChatStore) GetChat(chatID string) (*bot.Chat, error) {
	if !s.ready() || chatID == "" {
		return nil, nil
	}
	var rec RemoteChatRecord
	if err := s.db.Where("chat_id = ?", chatID).First(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get chat %s: %w", chatID, err)
	}
	return fromChatRecord(&rec), nil
}

// GetOrCreateChat returns the chat, creating an empty one if it does not exist.
//
// The table is keyed by chat_id alone (no platform dimension), so when an
// existing row's platform differs from the requested platform we refuse rather
// than silently returning (and later overwriting) another platform's chat.
// This is the guard against cross-platform chatID-string collisions leaking
// platform A's chat into platform B.
func (s *RemoteChatStore) GetOrCreateChat(chatID, platform string) (*bot.Chat, error) {
	if !s.ready() {
		return nil, ErrStoreClosed
	}
	existing, err := s.GetChat(chatID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if platform != "" && existing.Platform != "" && existing.Platform != platform {
			return nil, fmt.Errorf("chat %q belongs to platform %q, not %q", chatID, existing.Platform, platform)
		}
		return existing, nil
	}

	now := time.Now().UTC()
	fresh := &bot.Chat{
		ChatID:    chatID,
		Platform:  platform,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.UpsertChat(fresh); err != nil {
		return nil, err
	}
	return fresh, nil
}

// UpsertChat creates or replaces a chat row.
func (s *RemoteChatStore) UpsertChat(chat *bot.Chat) error {
	if !s.ready() {
		return ErrStoreClosed
	}
	if err := normalizeChat(chat); err != nil {
		return err
	}
	return upsertChatRecord(s.db, chat)
}

// ImportChat stores a chat exactly as given, without restamping UpdatedAt.
//
// Migration needs this for the same reason sessions do: UpdatedAt orders
// ListChats, so stamping it on import would make every migrated chat look
// equally and freshly active in the bot's chat list.
func (s *RemoteChatStore) ImportChat(chat *bot.Chat) error {
	if !s.ready() {
		return ErrStoreClosed
	}
	if chat == nil || chat.ChatID == "" {
		return ErrChatIDRequired
	}
	if chat.CurrentAgent == "" {
		chat.CurrentAgent = DefaultChatAgent
	}
	return upsertChatRecord(s.db, chat)
}

// upsertChatRecord writes a chat as a whole row.
//
// Upsert rather than GORM's Updates: struct-based Updates skips zero values, so
// turning a flag back off (RemoveFromWhitelist, ClearPaired) or clearing a
// string would silently not persist.
func upsertChatRecord(tx *gorm.DB, chat *bot.Chat) error {
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}},
		UpdateAll: true,
	}).Create(toChatRecord(chat)).Error; err != nil {
		return fmt.Errorf("upsert chat %s: %w", chat.ChatID, err)
	}
	return nil
}

// UpdateChat applies fn to an existing chat inside a transaction, so a
// concurrent writer cannot interleave between the read and the write.
// A missing chat is a no-op, matching the store this replaces.
func (s *RemoteChatStore) UpdateChat(chatID string, fn func(*bot.Chat)) error {
	if !s.ready() {
		return ErrStoreClosed
	}
	if fn == nil {
		return fmt.Errorf("update function is required")
	}

	// No SELECT ... FOR UPDATE here: SQLite has no row locks, and the write
	// at the end of the transaction is what serializes concurrent updaters
	// (busy_timeout covers the upgrade contention).
	return s.db.Transaction(func(tx *gorm.DB) error {
		var rec RemoteChatRecord
		err := tx.Where("chat_id = ?", chatID).First(&rec).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return fmt.Errorf("load chat %s: %w", chatID, err)
		}

		chat := fromChatRecord(&rec)
		fn(chat)
		if err := normalizeChat(chat); err != nil {
			return err
		}
		return upsertChatRecord(tx, chat)
	})
}

// mutate applies fn to a chat, creating it on the given platform if it does not
// exist yet, in ONE transaction.
//
// Every "ensure the chat exists, then change a field" operation goes through
// here — binding a project, whitelisting, pairing, agent handoff. Doing it as
// GetOrCreateChat followed by UpdateChat, as those used to, cost two
// transactions and read the row twice, and on a fresh chat wrote it twice with
// the first write immediately overwritten.
func (s *RemoteChatStore) mutate(chatID, platform string, fn func(*bot.Chat)) error {
	if !s.ready() {
		return ErrStoreClosed
	}
	if chatID == "" {
		return ErrChatIDRequired
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		var chat *bot.Chat

		var rec RemoteChatRecord
		switch err := tx.Where("chat_id = ?", chatID).First(&rec).Error; {
		case err == nil:
			chat = fromChatRecord(&rec)
			// The table is keyed by chat_id alone, so refuse rather than
			// re-stamp a chat that belongs to another platform — see
			// GetOrCreateChat.
			if platform != "" && chat.Platform != "" && chat.Platform != platform {
				return fmt.Errorf("chat %q belongs to platform %q, not %q", chatID, chat.Platform, platform)
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			chat = &bot.Chat{ChatID: chatID, Platform: platform}
		default:
			return fmt.Errorf("load chat %s: %w", chatID, err)
		}

		fn(chat)
		if err := normalizeChat(chat); err != nil {
			return err
		}
		return upsertChatRecord(tx, chat)
	})
}

// DeleteChat hard-deletes the chat row. All chat state (pairing, whitelist,
// project binding) is gone; a new message from the same chat recreates it
// fresh via the normal auto-create path. Sessions are untouched. Deleting a
// missing chat is a no-op.
func (s *RemoteChatStore) DeleteChat(chatID string) error {
	if !s.ready() {
		return ErrStoreClosed
	}
	if chatID == "" {
		return ErrChatIDRequired
	}
	if err := s.db.Where("chat_id = ?", chatID).Delete(&RemoteChatRecord{}).Error; err != nil {
		return fmt.Errorf("delete chat %s: %w", chatID, err)
	}
	return nil
}

// SetChatDisabled toggles the inbound blocklist flag. A missing chat is a
// no-op (there is nothing to block; the flag would be erased by the next
// auto-create anyway).
func (s *RemoteChatStore) SetChatDisabled(chatID string, disabled bool) error {
	return s.UpdateChat(chatID, func(chat *bot.Chat) {
		chat.Disabled = disabled
		if disabled {
			chat.DisabledAt = time.Now().UTC()
		} else {
			chat.DisabledAt = time.Time{}
		}
	})
}

// IsChatDisabled reports the blocklist flag. Missing chat → false.
func (s *RemoteChatStore) IsChatDisabled(chatID string) bool {
	chat, err := s.GetChat(chatID)
	if err != nil || chat == nil {
		return false
	}
	return chat.Disabled
}

// ---------- project binding ----------

// BindProject binds a project to a chat, creating the chat if needed, and
// pushes the path onto the chat's MRU history.
func (s *RemoteChatStore) BindProject(chatID, platform, projectPath, ownerID string) error {
	return s.mutate(chatID, platform, func(chat *bot.Chat) {
		chat.Platform = platform
		chat.OwnerID = ownerID
		chat.PushProjectHistory(projectPath)
	})
}

// ListChatProjectPaths returns the per-chat MRU list of project paths (newest
// first), falling back to [ProjectPath] for chats with no history yet.
func (s *RemoteChatStore) ListChatProjectPaths(chatID string) ([]string, error) {
	chat, err := s.GetChat(chatID)
	if err != nil || chat == nil {
		return nil, err
	}
	if len(chat.ProjectHistory) > 0 {
		out := make([]string, len(chat.ProjectHistory))
		copy(out, chat.ProjectHistory)
		return out, nil
	}
	if chat.ProjectPath != "" {
		return []string{chat.ProjectPath}, nil
	}
	return nil, nil
}

// GetProjectPath retrieves the project path bound to a chat.
func (s *RemoteChatStore) GetProjectPath(chatID string) (string, bool, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return "", false, err
	}
	if chat == nil || chat.ProjectPath == "" {
		return "", false, nil
	}
	return chat.ProjectPath, true, nil
}

// ListChatsByOwner lists a user's chats on a platform that have a project bound.
func (s *RemoteChatStore) ListChatsByOwner(ownerID, platform string) ([]*bot.Chat, error) {
	if !s.ready() {
		return nil, ErrStoreClosed
	}
	var recs []RemoteChatRecord
	if err := s.db.
		Where("owner_id = ? AND platform = ? AND project_path <> ''", ownerID, platform).
		Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("list chats by owner: %w", err)
	}
	out := make([]*bot.Chat, 0, len(recs))
	for i := range recs {
		out = append(out, fromChatRecord(&recs[i]))
	}
	return out, nil
}

// ListChats returns the chat records this bot can reach on platform — those
// whose Platform field is set AND matches. Empty/mismatched-platform records
// are dropped at the source (see bot.ChatStoreInterface.ListChats for why).
// Ordered newest-first by updated_at, with chat_id as a stable tiebreaker, so
// the most recently active chats surface at the top.
func (s *RemoteChatStore) ListChats(platform string, includeDisabled bool) ([]*bot.Chat, error) {
	if !s.ready() {
		return nil, ErrStoreClosed
	}

	// Literal equality, including for the empty platform: callers always pass
	// a real platform, and "" selecting exactly the unattributed records is
	// the contract the interface documents.
	q := s.db.Where("platform = ?", platform)
	if !includeDisabled {
		// NULL-safe: rows that predate the disabled column carry NULL, which
		// `disabled = false` would silently drop (NULL never compares equal).
		q = q.Where("disabled IS NOT ?", true)
	}
	var recs []RemoteChatRecord
	if err := q.
		Order("updated_at DESC, chat_id ASC").
		Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("list chats for platform %s: %w", platform, err)
	}

	out := make([]*bot.Chat, 0, len(recs))
	for i := range recs {
		out = append(out, fromChatRecord(&recs[i]))
	}
	return out, nil
}

// ---------- whitelist ----------

// AddToWhitelist whitelists a chat, creating it if needed.
func (s *RemoteChatStore) AddToWhitelist(chatID, platform, addedBy string) error {
	return s.mutate(chatID, platform, func(chat *bot.Chat) {
		chat.IsWhitelisted = true
		chat.WhitelistedBy = addedBy
	})
}

// RemoveFromWhitelist clears a chat's whitelist flag.
func (s *RemoteChatStore) RemoveFromWhitelist(chatID string) error {
	return s.UpdateChat(chatID, func(chat *bot.Chat) {
		chat.IsWhitelisted = false
	})
}

// IsWhitelisted reports whether a chat is whitelisted.
func (s *RemoteChatStore) IsWhitelisted(chatID string) bool {
	chat, err := s.GetChat(chatID)
	if err != nil || chat == nil {
		return false
	}
	return chat.IsWhitelisted
}

// ---------- bash cwd ----------

// SetBashCwd sets the bash working directory for a chat.
func (s *RemoteChatStore) SetBashCwd(chatID, cwd string) error {
	return s.UpdateChat(chatID, func(chat *bot.Chat) {
		chat.BashCwd = cwd
	})
}

// GetBashCwd retrieves the bash working directory for a chat.
func (s *RemoteChatStore) GetBashCwd(chatID string) (string, bool, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return "", false, err
	}
	if chat == nil || chat.BashCwd == "" {
		return "", false, nil
	}
	return chat.BashCwd, true, nil
}

// ---------- current agent ----------

// SetCurrentAgent sets the chat's current agent, creating the chat row if it
// doesn't exist yet — without the auto-create, handoff state was silently
// dropped for any chat that hadn't been bound or paired first.
func (s *RemoteChatStore) SetCurrentAgent(chatID, platform, agentType string) error {
	return s.mutate(chatID, platform, func(chat *bot.Chat) {
		chat.CurrentAgent = agentType
	})
}

// GetCurrentAgent retrieves the chat's current agent, defaulting to Smart
// Guide as the entry point.
func (s *RemoteChatStore) GetCurrentAgent(chatID string) (string, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return DefaultChatAgent, err
	}
	if chat == nil || chat.CurrentAgent == "" {
		return DefaultChatAgent, nil
	}
	return chat.CurrentAgent, nil
}

// ---------- pairing (TOFU) ----------

// SetPaired marks a chat as paired with the given bot and sender, creating the
// chat if needed.
func (s *RemoteChatStore) SetPaired(chatID, platform, botUUID, senderID string) error {
	if chatID == "" || botUUID == "" {
		return fmt.Errorf("chat_id and bot_uuid are required")
	}
	return s.mutate(chatID, platform, func(chat *bot.Chat) {
		chat.IsPaired = true
		chat.PairedBotUUID = botUUID
		chat.PairedSenderID = senderID
		chat.PairedAt = time.Now().UTC()
		if platform != "" {
			chat.Platform = platform
		}
	})
}

// ClearPaired removes any pairing recorded on the chat, preserving the rest of
// its state.
func (s *RemoteChatStore) ClearPaired(chatID string) error {
	return s.UpdateChat(chatID, func(chat *bot.Chat) {
		chat.IsPaired = false
		chat.PairedBotUUID = ""
		chat.PairedSenderID = ""
		chat.PairedAt = time.Time{}
	})
}

// IsChatPaired reports whether the chat is paired with the given bot UUID.
func (s *RemoteChatStore) IsChatPaired(chatID, botUUID string) bool {
	chat, err := s.GetChat(chatID)
	if err != nil || chat == nil {
		return false
	}
	return chat.IsPaired && chat.PairedBotUUID == botUUID
}
