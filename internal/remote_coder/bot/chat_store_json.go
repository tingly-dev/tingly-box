package bot

import (
	"fmt"
)

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

	// SetSession sets the session for a chat
	SetSession(chatID, sessionID string) error

	// GetSession retrieves the session for a chat
	GetSession(chatID string) (string, bool, error)

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
}

// Ensure ChatStoreJSON implements the interface
var _ ChatStoreInterface = (*ChatStoreJSON)(nil)

// ChatStoreJSON handles unified chat persistence using JSON file storage
// This is the new implementation replacing the SQLite-based ChatStore
type ChatStoreJSON struct {
	store *JSONStore
}

// NewChatStoreJSON creates a new JSON-based chat store
func NewChatStoreJSON(filePath string) (*ChatStoreJSON, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path is required")
	}

	store, err := NewJSONStore(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create JSON store: %w", err)
	}

	return &ChatStoreJSON{store: store}, nil
}

// Close ensures data is persisted before closing
func (s *ChatStoreJSON) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Close()
}

// GetChat retrieves a chat by ID
func (s *ChatStoreJSON) GetChat(chatID string) (*Chat, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	return s.store.GetChat(chatID)
}

// GetOrCreateChat gets a chat or creates it if not exists
func (s *ChatStoreJSON) GetOrCreateChat(chatID, platform string) (*Chat, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("chat store is not initialized")
	}
	return s.store.GetOrCreateChat(chatID, platform)
}

// UpsertChat creates or updates a chat
func (s *ChatStoreJSON) UpsertChat(chat *Chat) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("chat store is not initialized")
	}
	return s.store.UpsertChat(chat)
}

// UpdateChat updates specific fields of a chat
func (s *ChatStoreJSON) UpdateChat(chatID string, fn func(*Chat)) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("chat store is not initialized")
	}
	return s.store.UpdateChat(chatID, fn)
}

// ============== Project Binding ==============

// BindProject binds a project to a chat (creates chat if not exists)
func (s *ChatStoreJSON) BindProject(chatID, platform, projectPath, ownerID string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("chat store is not initialized")
	}
	return s.store.BindProject(chatID, platform, projectPath, ownerID)
}

// GetProjectPath retrieves the project path for a chat
func (s *ChatStoreJSON) GetProjectPath(chatID string) (string, bool, error) {
	if s == nil || s.store == nil {
		return "", false, nil
	}
	return s.store.GetProjectPath(chatID)
}

// ListChatsByOwner lists all chats owned by a user
func (s *ChatStoreJSON) ListChatsByOwner(ownerID, platform string) ([]*Chat, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	return s.store.ListChatsByOwner(ownerID, platform)
}

// ============== Session Mapping ==============

// SetSession sets the session for a chat (creates chat if not exists)
func (s *ChatStoreJSON) SetSession(chatID, sessionID string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("chat store is not initialized")
	}
	return s.store.SetSession(chatID, sessionID)
}

// GetSession retrieves the session for a chat
func (s *ChatStoreJSON) GetSession(chatID string) (string, bool, error) {
	if s == nil || s.store == nil {
		return "", false, nil
	}
	return s.store.GetSession(chatID)
}

// ============== Whitelist ==============

// AddToWhitelist adds a chat to the whitelist
func (s *ChatStoreJSON) AddToWhitelist(chatID, platform, addedBy string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("chat store is not initialized")
	}
	return s.store.AddToWhitelist(chatID, platform, addedBy)
}

// RemoveFromWhitelist removes a chat from the whitelist
func (s *ChatStoreJSON) RemoveFromWhitelist(chatID string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("chat store is not initialized")
	}
	return s.store.RemoveFromWhitelist(chatID)
}

// IsWhitelisted checks if a chat is whitelisted
func (s *ChatStoreJSON) IsWhitelisted(chatID string) bool {
	if s == nil || s.store == nil {
		return false
	}
	return s.store.IsWhitelisted(chatID)
}

// ============== Bash CWD ==============

// SetBashCwd sets the bash working directory for a chat
func (s *ChatStoreJSON) SetBashCwd(chatID, cwd string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("chat store is not initialized")
	}
	return s.store.SetBashCwd(chatID, cwd)
}

// GetBashCwd retrieves the bash working directory for a chat
func (s *ChatStoreJSON) GetBashCwd(chatID string) (string, bool, error) {
	if s == nil || s.store == nil {
		return "", false, nil
	}
	return s.store.GetBashCwd(chatID)
}

// ============== Current Agent ==============

// SetCurrentAgent sets the current agent for a chat
func (s *ChatStoreJSON) SetCurrentAgent(chatID, agentType string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("chat store is not initialized")
	}
	return s.store.SetCurrentAgent(chatID, agentType)
}

// GetCurrentAgent retrieves the current agent for a chat
// Returns "claude" as default if not set
func (s *ChatStoreJSON) GetCurrentAgent(chatID string) (string, error) {
	if s == nil || s.store == nil {
		return "claude", nil
	}
	return s.store.GetCurrentAgent(chatID)
}

// SetAgentState sets the agent-specific state for a chat
func (s *ChatStoreJSON) SetAgentState(chatID string, state []byte) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("chat store is not initialized")
	}
	return s.store.SetAgentState(chatID, state)
}

// GetAgentState retrieves the agent-specific state for a chat
func (s *ChatStoreJSON) GetAgentState(chatID string) ([]byte, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	return s.store.GetAgentState(chatID)
}

// ============== Additional Methods for Compatibility ==============

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
	return s.store.ListWhitelistedGroups()
}
