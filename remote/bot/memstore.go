package bot

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"
)

// ErrChatIDRequired is returned when an operation needs a chat ID and got none.
var ErrChatIDRequired = errors.New("chat_id is required")

// MemoryChatStore is a ChatStoreInterface kept in memory. It follows the
// SQLite store's semantics (the host's), so Remote can run its handler and
// manager without a database: in its own tests, and in hosts that do not need
// chats to survive a restart.
type MemoryChatStore struct {
	mu    sync.Mutex
	chats map[string]*Chat
	// last is the latest UpdatedAt handed out; stamps never repeat, so
	// "newest first" is a total order even when writes land within one clock
	// tick.
	last time.Time
}

// NewMemoryChatStore returns an empty store.
func NewMemoryChatStore() *MemoryChatStore {
	return &MemoryChatStore{chats: map[string]*Chat{}}
}

var _ ChatStoreInterface = (*MemoryChatStore)(nil)

func cloneChat(c *Chat) *Chat {
	if c == nil {
		return nil
	}
	out := *c
	out.ProjectHistory = slices.Clone(c.ProjectHistory)
	return &out
}

func (s *MemoryChatStore) normalize(chat *Chat) error {
	if chat == nil || chat.ChatID == "" {
		return ErrChatIDRequired
	}
	now := time.Now().UTC()
	if !now.After(s.last) {
		now = s.last.Add(time.Nanosecond)
	}
	s.last = now
	if chat.CreatedAt.IsZero() {
		chat.CreatedAt = now
	}
	chat.UpdatedAt = now
	return nil
}

func (s *MemoryChatStore) get(chatID string) *Chat { return s.chats[chatID] }

// GetChat returns a copy of the chat; a missing chat is (nil, nil).
func (s *MemoryChatStore) GetChat(chatID string) (*Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneChat(s.get(chatID)), nil
}

// GetOrCreateChat returns the chat, creating an empty one if needed. A chat
// that belongs to another platform is refused rather than returned.
func (s *MemoryChatStore) GetOrCreateChat(chatID, platform string) (*Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.get(chatID); existing != nil {
		if platform != "" && existing.Platform != "" && existing.Platform != platform {
			return nil, fmt.Errorf("chat %q belongs to platform %q, not %q", chatID, existing.Platform, platform)
		}
		return cloneChat(existing), nil
	}
	fresh := &Chat{ChatID: chatID, Platform: platform}
	if err := s.normalize(fresh); err != nil {
		return nil, err
	}
	s.chats[chatID] = fresh
	return cloneChat(fresh), nil
}

// UpsertChat creates or replaces a chat.
func (s *MemoryChatStore) UpsertChat(chat *Chat) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := cloneChat(chat)
	if err := s.normalize(c); err != nil {
		return err
	}
	s.chats[c.ChatID] = c
	return nil
}

// UpdateChat applies fn to an existing chat; a missing chat is a no-op.
func (s *MemoryChatStore) UpdateChat(chatID string, fn func(*Chat)) error {
	if fn == nil {
		return errors.New("update function is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := s.get(chatID)
	if existing == nil {
		return nil
	}
	c := cloneChat(existing)
	fn(c)
	if err := s.normalize(c); err != nil {
		return err
	}
	s.chats[chatID] = c
	return nil
}

// mutate applies fn to a chat, creating it on platform if needed.
func (s *MemoryChatStore) mutate(chatID, platform string, fn func(*Chat)) error {
	if chatID == "" {
		return ErrChatIDRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var c *Chat
	if existing := s.get(chatID); existing != nil {
		c = cloneChat(existing)
		if platform != "" && c.Platform != "" && c.Platform != platform {
			return fmt.Errorf("chat %q belongs to platform %q, not %q", chatID, c.Platform, platform)
		}
	} else {
		c = &Chat{ChatID: chatID, Platform: platform}
	}
	fn(c)
	if err := s.normalize(c); err != nil {
		return err
	}
	s.chats[chatID] = c
	return nil
}

// BindProject binds a project path to a chat and pushes it onto the history.
func (s *MemoryChatStore) BindProject(chatID, platform, projectPath, ownerID string) error {
	return s.mutate(chatID, platform, func(c *Chat) {
		c.Platform = platform
		c.OwnerID = ownerID
		c.PushProjectHistory(projectPath)
	})
}

// RememberFolder pushes a folder onto the history, leaving ProjectPath alone.
func (s *MemoryChatStore) RememberFolder(chatID, platform, folder, ownerID string) error {
	return s.mutate(chatID, platform, func(c *Chat) {
		c.Platform = platform
		c.OwnerID = ownerID
		c.RememberFolder(folder)
	})
}

// GetProjectPath returns the chat's project path.
func (s *MemoryChatStore) GetProjectPath(chatID string) (string, bool, error) {
	chat, _ := s.GetChat(chatID)
	if chat == nil || chat.ProjectPath == "" {
		return "", false, nil
	}
	return chat.ProjectPath, true, nil
}

// ListChatProjectPaths returns the MRU history, falling back to [ProjectPath].
func (s *MemoryChatStore) ListChatProjectPaths(chatID string) ([]string, error) {
	chat, _ := s.GetChat(chatID)
	if chat == nil {
		return nil, nil
	}
	if len(chat.ProjectHistory) > 0 {
		return chat.ProjectHistory, nil
	}
	if chat.ProjectPath != "" {
		return []string{chat.ProjectPath}, nil
	}
	return nil, nil
}

// ListChatsByOwner lists a user's chats on a platform that have a project bound.
func (s *MemoryChatStore) ListChatsByOwner(ownerID, platform string) ([]*Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Chat
	for _, c := range s.chats {
		if c.OwnerID == ownerID && c.Platform == platform && c.ProjectPath != "" {
			out = append(out, cloneChat(c))
		}
	}
	return out, nil
}

// ListChats returns the chats of platform, newest first; disabled ones only
// with includeDisabled.
func (s *MemoryChatStore) ListChats(platform string, includeDisabled bool) ([]*Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Chat
	for _, c := range s.chats {
		if c.Platform != platform || (c.Disabled && !includeDisabled) {
			continue
		}
		out = append(out, cloneChat(c))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].ChatID < out[j].ChatID
	})
	return out, nil
}

// AddToWhitelist whitelists a chat, creating it if needed.
func (s *MemoryChatStore) AddToWhitelist(chatID, platform, addedBy string) error {
	return s.mutate(chatID, platform, func(c *Chat) {
		c.IsWhitelisted = true
		c.WhitelistedBy = addedBy
	})
}

// RemoveFromWhitelist clears the whitelist flag.
func (s *MemoryChatStore) RemoveFromWhitelist(chatID string) error {
	return s.UpdateChat(chatID, func(c *Chat) { c.IsWhitelisted = false })
}

// IsWhitelisted reports the whitelist flag.
func (s *MemoryChatStore) IsWhitelisted(chatID string) bool {
	chat, _ := s.GetChat(chatID)
	return chat != nil && chat.IsWhitelisted
}

// SetPaired marks a chat as paired with a bot and sender.
func (s *MemoryChatStore) SetPaired(chatID, platform, botUUID, senderID string) error {
	if chatID == "" || botUUID == "" {
		return errors.New("chat_id and bot_uuid are required")
	}
	return s.mutate(chatID, platform, func(c *Chat) {
		c.IsPaired = true
		c.PairedBotUUID = botUUID
		c.PairedSenderID = senderID
		c.PairedAt = time.Now().UTC()
		if platform != "" {
			c.Platform = platform
		}
	})
}

// ClearPaired removes the pairing, keeping the rest of the chat.
func (s *MemoryChatStore) ClearPaired(chatID string) error {
	return s.UpdateChat(chatID, func(c *Chat) {
		c.IsPaired = false
		c.PairedBotUUID = ""
		c.PairedSenderID = ""
		c.PairedAt = time.Time{}
	})
}

// IsChatPaired reports whether the chat is paired with the bot.
func (s *MemoryChatStore) IsChatPaired(chatID, botUUID string) bool {
	chat, _ := s.GetChat(chatID)
	return chat != nil && chat.IsPaired && chat.PairedBotUUID == botUUID
}

// DeleteChat removes the chat; a missing chat is a no-op.
func (s *MemoryChatStore) DeleteChat(chatID string) error {
	if chatID == "" {
		return ErrChatIDRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.chats, chatID)
	return nil
}

// SetChatDisabled toggles the blocklist flag; a missing chat is a no-op.
func (s *MemoryChatStore) SetChatDisabled(chatID string, disabled bool) error {
	return s.UpdateChat(chatID, func(c *Chat) {
		c.Disabled = disabled
		if disabled {
			c.DisabledAt = time.Now().UTC()
		} else {
			c.DisabledAt = time.Time{}
		}
	})
}

// IsChatDisabled reports the blocklist flag.
func (s *MemoryChatStore) IsChatDisabled(chatID string) bool {
	chat, _ := s.GetChat(chatID)
	return chat != nil && chat.Disabled
}
