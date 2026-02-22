package webchat

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// MessageStore defines the interface for message persistence
type MessageStore interface {
	SaveMessage(msg *core.Message) error
	GetMessages(sessionID string, limit int, offset int) ([]*core.Message, error)
	GetMessagesSince(sessionID string, timestamp int64) ([]*core.Message, error)
	GetSession(sessionID string) (*SessionInfo, error)
	CreateOrUpdateSession(sessionID, senderID, senderName string, clientInfo *WebSocketClientInfo) error
	Close() error
}

// SessionInfo represents session metadata stored in database
type SessionInfo struct {
	ID         string
	SenderID   string
	SenderName string
	CreatedAt  int64
	LastActive int64
	ClientInfo *WebSocketClientInfo
	ExpiresAt  int64
}

// MessageRow represents the database storage format of a message
type MessageRow struct {
	ID          string
	SessionID   string
	Timestamp   int64
	SenderID    string
	SenderName  string
	RecipientID string
	ChatType    string
	ContentType string
	ContentData string
	Metadata    string
}

// ToCoreMessage converts MessageRow to core.Message
func (m *MessageRow) ToCoreMessage() (*core.Message, error) {
	var content core.Content
	var err error

	switch m.ContentType {
	case "text":
		content, err = core.NewTextContentFromString(m.ContentData)
	case "media":
		content, err = core.NewMediaContentFromString(m.ContentData)
	default:
		// Fallback to text content
		content = core.NewTextContent(m.ContentData)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse content: %w", err)
	}

	var metadata map[string]any
	if m.Metadata != "" {
		if err := json.Unmarshal([]byte(m.Metadata), &metadata); err != nil {
			metadata = nil
		}
	}

	return &core.Message{
		ID:        m.ID,
		Platform:  core.PlatformWebChat,
		Timestamp: m.Timestamp,
		Sender: core.Sender{
			ID:          m.SenderID,
			DisplayName: m.SenderName,
		},
		Recipient: core.Recipient{
			ID: m.RecipientID,
		},
		ChatType: core.ChatType(m.ChatType),
		Content:  content,
		Metadata: metadata,
	}, nil
}

// SQLiteStore implements MessageStore using SQLite database
type SQLiteStore struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

// NewSQLiteStore creates a new SQLite store at the given path
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Connection string with WAL mode and other optimizations
	dsn := fmt.Sprintf("file:%s?mode=rwc&_journal_mode=WAL&_timeout=5000&_foreign_keys=1&_synchronous=NORMAL", dbPath)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite doesn't support multiple writers
	db.SetMaxIdleConns(1)

	store := &SQLiteStore{
		db:   db,
		path: dbPath,
	}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the database tables if they don't exist
func (s *SQLiteStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		sender_id TEXT NOT NULL,
		sender_name TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		last_active INTEGER NOT NULL,
		client_info TEXT,
		expires_at INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_last_active ON sessions(last_active);
	CREATE INDEX IF NOT EXISTS idx_sessions_sender ON sessions(sender_id);

	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		sender_id TEXT NOT NULL,
		sender_name TEXT NOT NULL,
		recipient_id TEXT NOT NULL,
		chat_type TEXT NOT NULL,
		content_type TEXT NOT NULL,
		content_data TEXT NOT NULL,
		metadata TEXT,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_messages_session_timestamp
		ON messages(session_id, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_messages_timestamp
		ON messages(timestamp DESC);
	`

	_, err := s.db.Exec(schema)
	return err
}

// SaveMessage saves a message to the database
func (s *SQLiteStore) SaveMessage(msg *core.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	contentData, err := json.Marshal(msg.Content)
	if err != nil {
		return fmt.Errorf("failed to marshal content: %w", err)
	}

	var metadata []byte
	if msg.Metadata != nil {
		metadata, err = json.Marshal(msg.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	// Use Recipient.ID as session_id for WebChat
	sessionID := msg.Recipient.ID

	query := `
	INSERT INTO messages (id, session_id, timestamp, sender_id, sender_name,
						  recipient_id, chat_type, content_type, content_data, metadata)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.Exec(query,
		msg.ID,
		sessionID,
		msg.Timestamp,
		msg.Sender.ID,
		msg.Sender.DisplayName,
		msg.Recipient.ID,
		string(msg.ChatType),
		msg.Content.ContentType(),
		string(contentData),
		string(metadata),
	)

	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	// Update session last_active in the same transaction
	_, err = s.db.Exec("UPDATE sessions SET last_active = ? WHERE id = ?",
		time.Now().Unix(), sessionID)

	return err
}

// GetMessages retrieves messages for a session with pagination
func (s *SQLiteStore) GetMessages(sessionID string, limit int, offset int) ([]*core.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, session_id, timestamp, sender_id, sender_name,
		   recipient_id, chat_type, content_type, content_data, metadata
	FROM messages
	WHERE session_id = ?
	ORDER BY timestamp DESC
	LIMIT ? OFFSET ?
	`

	rows, err := s.db.Query(query, sessionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	return s.scanMessages(rows)
}

// GetMessagesSince retrieves messages after a given timestamp
func (s *SQLiteStore) GetMessagesSince(sessionID string, timestamp int64) ([]*core.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, session_id, timestamp, sender_id, sender_name,
		   recipient_id, chat_type, content_type, content_data, metadata
	FROM messages
	WHERE session_id = ? AND timestamp > ?
	ORDER BY timestamp ASC
	`

	rows, err := s.db.Query(query, sessionID, timestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages since: %w", err)
	}
	defer rows.Close()

	return s.scanMessages(rows)
}

// GetSession retrieves session information
func (s *SQLiteStore) GetSession(sessionID string) (*SessionInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, sender_id, sender_name, created_at, last_active, client_info, expires_at
	FROM sessions
	WHERE id = ?
	`

	var info SessionInfo
	var clientInfoJSON sql.NullString

	err := s.db.QueryRow(query, sessionID).Scan(
		&info.ID,
		&info.SenderID,
		&info.SenderName,
		&info.CreatedAt,
		&info.LastActive,
		&clientInfoJSON,
		&info.ExpiresAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	if clientInfoJSON.Valid {
		json.Unmarshal([]byte(clientInfoJSON.String), &info.ClientInfo)
	}

	return &info, nil
}

// CreateOrUpdateSession creates a new session or updates an existing one
func (s *SQLiteStore) CreateOrUpdateSession(sessionID, senderID, senderName string, clientInfo *WebSocketClientInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	var clientInfoJSON []byte
	if clientInfo != nil {
		var err error
		clientInfoJSON, err = json.Marshal(clientInfo)
		if err != nil {
			clientInfoJSON = nil
		}
	}

	query := `
	INSERT INTO sessions (id, sender_id, sender_name, created_at, last_active, client_info)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		last_active = excluded.last_active,
		client_info = excluded.client_info
	`

	_, err := s.db.Exec(query, sessionID, senderID, senderName, now, now, string(clientInfoJSON))
	return err
}

// scanMessages scans message rows from a query result
func (s *SQLiteStore) scanMessages(rows *sql.Rows) ([]*core.Message, error) {
	var messages []*core.Message

	for rows.Next() {
		var msg MessageRow
		err := rows.Scan(
			&msg.ID,
			&msg.SessionID,
			&msg.Timestamp,
			&msg.SenderID,
			&msg.SenderName,
			&msg.RecipientID,
			&msg.ChatType,
			&msg.ContentType,
			&msg.ContentData,
			&msg.Metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message row: %w", err)
		}

		coreMsg, err := msg.ToCoreMessage()
		if err != nil {
			continue // Skip malformed messages
		}

		messages = append(messages, coreMsg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, nil
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// CleanupExpiredSessions removes sessions that have expired
func (s *SQLiteStore) CleanupExpiredSessions() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	result, err := s.db.Exec("DELETE FROM sessions WHERE expires_at > 0 AND expires_at < ?", now)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}

	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// GetMessageCount returns the total number of messages for a session
func (s *SQLiteStore) GetMessageCount(sessionID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id = ?", sessionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}

	return count, nil
}
