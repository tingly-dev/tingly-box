package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// JSONStore manages chat state using a JSON file
type JSONStore struct {
	mu       sync.RWMutex
	filePath string
	data     *StoreData
	dirty    bool // Track if data needs to be written
}

// StoreData represents the JSON file structure
type StoreData struct {
	Version int                `json:"version"`
	Chats   map[string]*Chat  `json:"chats"` // Key: chat_id
}

// NewJSONStore creates a new JSON-based store
// If filePath doesn't exist, creates empty store
// If filePath exists, loads existing data
func NewJSONStore(filePath string) (*JSONStore, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path is required")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	store := &JSONStore{
		filePath: filePath,
		data: &StoreData{
			Version: 1,
			Chats:   make(map[string]*Chat),
		},
	}

	// Try to load existing data
	if err := store.load(); err != nil {
		// If file doesn't exist, that's ok - we start fresh
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load store: %w", err)
		}
	}

	return store, nil
}

// load reads the JSON file into memory
func (s *JSONStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if file exists
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		s.data = &StoreData{
			Version: 1,
			Chats:   make(map[string]*Chat),
		}
		s.dirty = false
		return nil
	}

	// Read file
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse JSON
	var storeData StoreData
	if err := json.Unmarshal(data, &storeData); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Validate version
	if storeData.Version > 1 {
		return fmt.Errorf("unsupported store version: %d", storeData.Version)
	}

	// Ensure chats map is initialized
	if storeData.Chats == nil {
		storeData.Chats = make(map[string]*Chat)
	}

	s.data = &storeData
	s.dirty = false
	return nil
}

// save writes the current data to disk atomically
func (s *JSONStore) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.dirty {
		return nil // No changes to save
	}

	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write to temporary file
	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		// Clean up temp file on error
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename file: %w", err)
	}

	s.dirty = false
	return nil
}

// Close ensures data is persisted before closing
func (s *JSONStore) Close() error {
	return s.save()
}

// GetChat retrieves a chat by ID
func (s *JSONStore) GetChat(chatID string) (*Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data.Chats == nil {
		return nil, nil
	}

	chat, ok := s.data.Chats[chatID]
	if !ok {
		return nil, nil
	}

	// Return a copy to avoid race conditions
	chatCopy := *chat
	return &chatCopy, nil
}

// UpsertChat creates or updates a chat
func (s *JSONStore) UpsertChat(chat *Chat) error {
	if chat == nil || chat.ChatID == "" {
		return fmt.Errorf("chat_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if chat.CreatedAt.IsZero() {
		chat.CreatedAt = now
	}
	chat.UpdatedAt = now

	// Set default agent if not specified
	if chat.CurrentAgent == "" {
		chat.CurrentAgent = "claude"
	}

	// Store a copy
	chatCopy := *chat
	s.data.Chats[chat.ChatID] = &chatCopy
	s.dirty = true

	return nil
}

// UpdateChat updates specific fields of a chat
func (s *JSONStore) UpdateChat(chatID string, fn func(*Chat)) error {
	if fn == nil {
		return fmt.Errorf("update function is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	chat, ok := s.data.Chats[chatID]
	if !ok {
		return fmt.Errorf("chat not found: %s", chatID)
	}

	// Create a copy for the update
	chatCopy := *chat
	fn(&chatCopy)

	// Update timestamp
	chatCopy.UpdatedAt = time.Now().UTC()

	s.data.Chats[chatID] = &chatCopy
	s.dirty = true

	return nil
}

// GetOrCreateChat gets a chat or creates it if not exists
func (s *JSONStore) GetOrCreateChat(chatID, platform string) (*Chat, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return nil, err
	}
	if chat != nil {
		return chat, nil
	}

	// Create new chat
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

// ListChats returns all chats
func (s *JSONStore) ListChats() ([]*Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data.Chats == nil {
		return nil, nil
	}

	chats := make([]*Chat, 0, len(s.data.Chats))
	for _, chat := range s.data.Chats {
		chatCopy := *chat
		chats = append(chats, &chatCopy)
	}

	return chats, nil
}

// ListChatsByOwner lists all chats owned by a user
func (s *JSONStore) ListChatsByOwner(ownerID, platform string) ([]*Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data.Chats == nil {
		return nil, nil
	}

	var chats []*Chat
	for _, chat := range s.data.Chats {
		if chat.OwnerID == ownerID && chat.Platform == platform && chat.ProjectPath != "" {
			chatCopy := *chat
			chats = append(chats, &chatCopy)
		}
	}

	return chats, nil
}

// GetProjectPath retrieves the project path for a chat
func (s *JSONStore) GetProjectPath(chatID string) (string, bool, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return "", false, err
	}
	if chat == nil || chat.ProjectPath == "" {
		return "", false, nil
	}
	return chat.ProjectPath, true, nil
}

// BindProject binds a project to a chat (creates chat if not exists)
func (s *JSONStore) BindProject(chatID, platform, projectPath, ownerID string) error {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return err
	}
	if chat == nil {
		// Create new chat
		now := time.Now().UTC()
		chat = &Chat{
			ChatID:      chatID,
			Platform:    platform,
			ProjectPath: projectPath,
			OwnerID:     ownerID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		return s.UpsertChat(chat)
	}
	// Update existing chat
	chat.Platform = platform
	chat.ProjectPath = projectPath
	chat.OwnerID = ownerID
	return s.UpsertChat(chat)
}

// SetSession sets the session for a chat (creates chat if not exists)
func (s *JSONStore) SetSession(chatID, sessionID string) error {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return err
	}
	if chat == nil {
		// Create new chat
		now := time.Now().UTC()
		chat = &Chat{
			ChatID:    chatID,
			Platform:  "telegram", // default platform
			SessionID: sessionID,
			CreatedAt: now,
			UpdatedAt: now,
		}
		return s.UpsertChat(chat)
	}
	chat.SessionID = sessionID
	return s.UpsertChat(chat)
}

// GetSession retrieves the session for a chat
func (s *JSONStore) GetSession(chatID string) (string, bool, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return "", false, err
	}
	if chat == nil || chat.SessionID == "" {
		return "", false, nil
	}
	return chat.SessionID, true, nil
}

// AddToWhitelist adds a chat to the whitelist
func (s *JSONStore) AddToWhitelist(chatID, platform, addedBy string) error {
	chat, err := s.GetOrCreateChat(chatID, platform)
	if err != nil {
		return err
	}
	chat.IsWhitelisted = true
	chat.WhitelistedBy = addedBy
	return s.UpsertChat(chat)
}

// RemoveFromWhitelist removes a chat from the whitelist
func (s *JSONStore) RemoveFromWhitelist(chatID string) error {
	return s.UpdateChat(chatID, func(chat *Chat) {
		chat.IsWhitelisted = false
	})
}

// IsWhitelisted checks if a chat is whitelisted
func (s *JSONStore) IsWhitelisted(chatID string) bool {
	chat, err := s.GetChat(chatID)
	if err != nil || chat == nil {
		return false
	}
	return chat.IsWhitelisted
}

// SetBashCwd sets the bash working directory for a chat
func (s *JSONStore) SetBashCwd(chatID, cwd string) error {
	return s.UpdateChat(chatID, func(chat *Chat) {
		chat.BashCwd = cwd
	})
}

// GetBashCwd retrieves the bash working directory for a chat
func (s *JSONStore) GetBashCwd(chatID string) (string, bool, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return "", false, err
	}
	if chat == nil || chat.BashCwd == "" {
		return "", false, nil
	}
	return chat.BashCwd, true, nil
}

// SetCurrentAgent sets the current agent for a chat
func (s *JSONStore) SetCurrentAgent(chatID, agentType string) error {
	return s.UpdateChat(chatID, func(chat *Chat) {
		chat.CurrentAgent = agentType
	})
}

// GetCurrentAgent retrieves the current agent for a chat
// Returns "claude" as default if not set
func (s *JSONStore) GetCurrentAgent(chatID string) (string, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return "", err
	}
	if chat == nil {
		return "claude", nil // Default to Claude Code
	}
	if chat.CurrentAgent == "" {
		return "claude", nil // Default to Claude Code
	}
	return chat.CurrentAgent, nil
}

// SetAgentState sets the agent-specific state for a chat
func (s *JSONStore) SetAgentState(chatID string, state []byte) error {
	return s.UpdateChat(chatID, func(chat *Chat) {
		chat.AgentState = state
	})
}

// GetAgentState retrieves the agent-specific state for a chat
func (s *JSONStore) GetAgentState(chatID string) ([]byte, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, nil
	}
	return chat.AgentState, nil
}

// ListWhitelistedGroups returns all whitelisted groups
func (s *JSONStore) ListWhitelistedGroups() ([]struct {
	GroupID   string
	Platform  string
	AddedBy   string
	CreatedAt string
}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []struct {
		GroupID   string
		Platform  string
		AddedBy   string
		CreatedAt string
	}

	for _, chat := range s.data.Chats {
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
