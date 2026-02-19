package bot

import (
	"database/sql"
	"fmt"
	"time"
)

// Chat represents all state associated with a chat (direct or group)
type Chat struct {
	ChatID      string `json:"chat_id"`
	Platform    string `json:"platform"`
	ProjectPath string `json:"project_path,omitempty"`
	OwnerID     string `json:"owner_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`

	// Group-specific
	IsWhitelisted bool   `json:"is_whitelisted"`
	WhitelistedBy string `json:"whitelisted_by,omitempty"`

	// Bash state
	BashCwd string `json:"bash_cwd,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChatStore handles unified chat persistence
type ChatStore struct {
	db *sql.DB
}

// NewChatStore creates a new chat store
func NewChatStore(db *sql.DB) (*ChatStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	return &ChatStore{db: db}, nil
}

// InitChatSchema initializes the chat schema
func InitChatSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS remote_coder_chats (
			chat_id TEXT PRIMARY KEY,
			platform TEXT NOT NULL,
			project_path TEXT,
			owner_id TEXT,
			session_id TEXT,
			is_whitelisted INTEGER DEFAULT 0,
			whitelisted_by TEXT,
			bash_cwd TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_chats_platform ON remote_coder_chats(platform);
		CREATE INDEX IF NOT EXISTS idx_chats_owner ON remote_coder_chats(owner_id);
		CREATE INDEX IF NOT EXISTS idx_chats_session ON remote_coder_chats(session_id);
	`)
	return err
}

// GetChat retrieves a chat by ID
func (s *ChatStore) GetChat(chatID string) (*Chat, error) {
	row := s.db.QueryRow(`
		SELECT chat_id, platform, project_path, owner_id, session_id,
			   is_whitelisted, whitelisted_by, bash_cwd, created_at, updated_at
		FROM remote_coder_chats WHERE chat_id = ?
	`, chatID)

	return scanChat(row)
}

// GetOrCreateChat gets a chat or creates it if not exists
func (s *ChatStore) GetOrCreateChat(chatID, platform string) (*Chat, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return nil, err
	}
	if chat != nil {
		return chat, nil
	}

	// Create new chat
	now := time.Now().UTC()
	chat = &Chat{
		ChatID:    chatID,
		Platform:  platform,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = s.db.Exec(`
		INSERT INTO remote_coder_chats (chat_id, platform, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, chat.ChatID, chat.Platform, chat.CreatedAt.Format(time.RFC3339), chat.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	return chat, nil
}

// UpsertChat creates or updates a chat
func (s *ChatStore) UpsertChat(chat *Chat) error {
	if chat.ChatID == "" {
		return fmt.Errorf("chat_id is required")
	}

	now := time.Now().UTC()
	if chat.CreatedAt.IsZero() {
		chat.CreatedAt = now
	}
	chat.UpdatedAt = now

	isWhitelisted := 0
	if chat.IsWhitelisted {
		isWhitelisted = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO remote_coder_chats (chat_id, platform, project_path, owner_id, session_id,
			is_whitelisted, whitelisted_by, bash_cwd, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			platform = excluded.platform,
			project_path = excluded.project_path,
			owner_id = excluded.owner_id,
			session_id = excluded.session_id,
			is_whitelisted = excluded.is_whitelisted,
			whitelisted_by = excluded.whitelisted_by,
			bash_cwd = excluded.bash_cwd,
			updated_at = excluded.updated_at
	`, chat.ChatID, chat.Platform, chat.ProjectPath, chat.OwnerID, chat.SessionID,
		isWhitelisted, chat.WhitelistedBy, chat.BashCwd,
		chat.CreatedAt.Format(time.RFC3339), chat.UpdatedAt.Format(time.RFC3339))

	return err
}

// UpdateChat updates specific fields of a chat
func (s *ChatStore) UpdateChat(chatID string, fn func(*Chat)) error {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return err
	}
	if chat == nil {
		return fmt.Errorf("chat not found: %s", chatID)
	}

	fn(chat)
	return s.UpsertChat(chat)
}

// ============== Project Binding ==============

// BindProject binds a project to a chat (creates chat if not exists)
func (s *ChatStore) BindProject(chatID, platform, projectPath, ownerID string) error {
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

// GetProjectPath retrieves the project path for a chat
func (s *ChatStore) GetProjectPath(chatID string) (string, bool, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return "", false, err
	}
	if chat == nil || chat.ProjectPath == "" {
		return "", false, nil
	}
	return chat.ProjectPath, true, nil
}

// ListChatsByOwner lists all chats owned by a user
func (s *ChatStore) ListChatsByOwner(ownerID, platform string) ([]*Chat, error) {
	rows, err := s.db.Query(`
		SELECT chat_id, platform, project_path, owner_id, session_id,
			   is_whitelisted, whitelisted_by, bash_cwd, created_at, updated_at
		FROM remote_coder_chats
		WHERE owner_id = ? AND platform = ? AND project_path IS NOT NULL
		ORDER BY updated_at DESC
	`, ownerID, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanChats(rows)
}

// ============== Session Mapping ==============

// SetSession sets the session for a chat (creates chat if not exists)
func (s *ChatStore) SetSession(chatID, sessionID string) error {
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
func (s *ChatStore) GetSession(chatID string) (string, bool, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return "", false, err
	}
	if chat == nil || chat.SessionID == "" {
		return "", false, nil
	}
	return chat.SessionID, true, nil
}

// ============== Whitelist ==============

// AddToWhitelist adds a chat to the whitelist
func (s *ChatStore) AddToWhitelist(chatID, platform, addedBy string) error {
	chat, err := s.GetOrCreateChat(chatID, platform)
	if err != nil {
		return err
	}
	chat.IsWhitelisted = true
	chat.WhitelistedBy = addedBy
	return s.UpsertChat(chat)
}

// RemoveFromWhitelist removes a chat from the whitelist
func (s *ChatStore) RemoveFromWhitelist(chatID string) error {
	return s.UpdateChat(chatID, func(chat *Chat) {
		chat.IsWhitelisted = false
	})
}

// IsWhitelisted checks if a chat is whitelisted
func (s *ChatStore) IsWhitelisted(chatID string) bool {
	chat, err := s.GetChat(chatID)
	if err != nil || chat == nil {
		return false
	}
	return chat.IsWhitelisted
}

// ============== Bash CWD ==============

// SetBashCwd sets the bash working directory for a chat
func (s *ChatStore) SetBashCwd(chatID, cwd string) error {
	return s.UpdateChat(chatID, func(chat *Chat) {
		chat.BashCwd = cwd
	})
}

// GetBashCwd retrieves the bash working directory for a chat
func (s *ChatStore) GetBashCwd(chatID string) (string, bool, error) {
	chat, err := s.GetChat(chatID)
	if err != nil {
		return "", false, err
	}
	if chat == nil || chat.BashCwd == "" {
		return "", false, nil
	}
	return chat.BashCwd, true, nil
}

// ============== Helpers ==============

func scanChat(row *sql.Row) (*Chat, error) {
	var chat Chat
	var projectPath, ownerID, sessionID, whitelistedBy, bashCwd sql.NullString
	var createdAt, updatedAt sql.NullString
	var isWhitelisted int

	err := row.Scan(
		&chat.ChatID, &chat.Platform, &projectPath, &ownerID, &sessionID,
		&isWhitelisted, &whitelistedBy, &bashCwd, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	chat.ProjectPath = projectPath.String
	chat.OwnerID = ownerID.String
	chat.SessionID = sessionID.String
	chat.WhitelistedBy = whitelistedBy.String
	chat.BashCwd = bashCwd.String
	chat.IsWhitelisted = isWhitelisted == 1

	if createdAt.Valid {
		chat.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	}
	if updatedAt.Valid {
		chat.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt.String)
	}

	return &chat, nil
}

func scanChats(rows *sql.Rows) ([]*Chat, error) {
	var chats []*Chat
	for rows.Next() {
		var chat Chat
		var projectPath, ownerID, sessionID, whitelistedBy, bashCwd sql.NullString
		var createdAt, updatedAt sql.NullString
		var isWhitelisted int

		err := rows.Scan(
			&chat.ChatID, &chat.Platform, &projectPath, &ownerID, &sessionID,
			&isWhitelisted, &whitelistedBy, &bashCwd, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		chat.ProjectPath = projectPath.String
		chat.OwnerID = ownerID.String
		chat.SessionID = sessionID.String
		chat.WhitelistedBy = whitelistedBy.String
		chat.BashCwd = bashCwd.String
		chat.IsWhitelisted = isWhitelisted == 1

		if createdAt.Valid {
			chat.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
		}
		if updatedAt.Valid {
			chat.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt.String)
		}

		chats = append(chats, &chat)
	}
	return chats, rows.Err()
}
