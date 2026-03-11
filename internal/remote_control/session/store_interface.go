package session

// SessionStore defines the interface for session persistence
// This allows both SQLite-based MessageStore and JSON-based SessionStoreJSON
type SessionStore interface {
	// Get retrieves a session by ID
	Get(sessionID string) (*Session, error)

	// Set stores a session
	Set(sessionID string, sess *Session) error

	// Delete removes a session
	Delete(sessionID string) error

	// List returns all sessions
	List() []*Session

	// FindByChatAgentProject finds a session by (chatID, agent, project) tuple
	FindByChatAgentProject(chatID, agent, project string) (*Session, error)

	// ListByChat lists all sessions for a given chat ID
	ListByChat(chatID string) ([]*Session, error)

	// Close closes the store
	Close() error
}
