package relay

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/webchat/protocol"
)

// MessageStore defines the interface for message persistence
type MessageStore interface {
	SaveMessage(sessionID string, msg *protocol.MessageData) error
	GetMessages(sessionID string, limit, offset int) ([]*protocol.MessageData, error)
	GetSession(sessionID string) (*SessionInfo, error)
	CreateOrUpdateSession(sessionID, senderID, senderName string, info *ClientInfo) error
	Close() error
}

// SQLiteStore implements MessageStore using SQLite
type SQLiteStore struct {
	db   *sql.DB
	path string
	mu   sync.Mutex
}

// SessionInfo represents stored session information
type SessionInfo struct {
	ID         string
	SenderID   string
	SenderName string
	ClientInfo *ClientInfo
}

// NewSQLiteStore creates a new SQLite message store
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &SQLiteStore{
		db:   db,
		path: dbPath,
	}

	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// init initializes the database schema
func (s *SQLiteStore) init() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create messages table
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			sender_id TEXT NOT NULL,
			sender_name TEXT,
			text TEXT,
			media TEXT,
			metadata TEXT,
			created_at INTEGER DEFAULT (strftime('%s', 'now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create messages table: %w", err)
	}

	// Create sessions table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			sender_id TEXT NOT NULL,
			sender_name TEXT,
			user_agent TEXT,
			ip_address TEXT,
			connect_time INTEGER,
			updated_at INTEGER DEFAULT (strftime('%s', 'now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create sessions table: %w", err)
	}

	// Create indexes
	_, err = s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_messages_session_timestamp
		ON messages (session_id, timestamp DESC)
	`)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	return nil
}

// SaveMessage saves a message to the store
func (s *SQLiteStore) SaveMessage(sessionID string, msg *protocol.MessageData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	mediaJSON := "[]"
	if len(msg.Media) > 0 {
		// TODO: Properly serialize media to JSON
	}

	metadataJSON := "{}"
	if msg.Metadata != nil {
		// TODO: Properly serialize metadata to JSON
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO messages
		(id, session_id, timestamp, sender_id, sender_name, text, media, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, msg.ID, sessionID, msg.Timestamp, msg.SenderID, msg.SenderName, msg.Text, mediaJSON, metadataJSON)

	if err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}

	return nil
}

// GetMessages retrieves messages for a session
func (s *SQLiteStore) GetMessages(sessionID string, limit, offset int) ([]*protocol.MessageData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT id, timestamp, sender_id, sender_name, text, media, metadata
		FROM messages
		WHERE session_id = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, sessionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []*protocol.MessageData
	for rows.Next() {
		var msg protocol.MessageData
		var mediaJSON, metadataJSON string

		err := rows.Scan(&msg.ID, &msg.Timestamp, &msg.SenderID, &msg.SenderName, &msg.Text, &mediaJSON, &metadataJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}

		// TODO: Parse mediaJSON and metadataJSON
		msg.Media = []protocol.MediaAttachment{}
		msg.Metadata = make(map[string]interface{})

		messages = append(messages, &msg)
	}

	return messages, nil
}

// GetSession retrieves session information
func (s *SQLiteStore) GetSession(sessionID string) (*SessionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var info SessionInfo
	err := s.db.QueryRow(`
		SELECT id, sender_id, sender_name, user_agent, ip_address, connect_time
		FROM sessions
		WHERE id = ?
	`, sessionID).Scan(&info.ID, &info.SenderID, &info.SenderName, &info.ClientInfo.UserAgent, &info.ClientInfo.IPAddress, &info.ClientInfo.ConnectTime)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	return &info, nil
}

// CreateOrUpdateSession creates or updates a session
func (s *SQLiteStore) CreateOrUpdateSession(sessionID, senderID, senderName string, info *ClientInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO sessions (id, sender_id, sender_name, user_agent, ip_address, connect_time)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			sender_id = excluded.sender_id,
			sender_name = excluded.sender_name,
			updated_at = strftime('%s', 'now')
	`, sessionID, senderID, senderName, info.UserAgent, info.IPAddress, info.ConnectTime)

	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
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
